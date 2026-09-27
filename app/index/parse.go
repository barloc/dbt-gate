package index

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type rawTop struct {
	Metadata  rawMeta             `json:"metadata"`
	Nodes     json.RawMessage     `json:"nodes"`
	Sources   json.RawMessage     `json:"sources"`
	Exposures json.RawMessage     `json:"exposures"`
	ParentMap map[string][]string `json:"parent_map"`
	ChildMap  map[string][]string `json:"child_map"`
}

type rawMeta struct {
	GeneratedAt string `json:"generated_at"`
}

type rawNode struct {
	UniqueID         string               `json:"unique_id"`
	ResourceType     string               `json:"resource_type"`
	Name             string               `json:"name"`
	Alias            string               `json:"alias"`
	Identifier       string               `json:"identifier"`
	Database         *string              `json:"database"`
	Schema           string               `json:"schema"`
	OriginalFilePath string               `json:"original_file_path"`
	Path             string               `json:"path"`
	Description      string               `json:"description"`
	Tags             []string             `json:"tags"`
	CompiledCode     string               `json:"compiled_code"`
	CompiledSQL      string               `json:"compiled_sql"`
	Columns          map[string]rawColumn `json:"columns"`
	Meta             map[string]any       `json:"meta"`
	DependsOn        rawDepends           `json:"depends_on"`
	Owner            rawOwner             `json:"owner"`
	Label            string               `json:"label"`
	Type             string               `json:"type"`
	URL              string               `json:"url"`
	Fqn              []string             `json:"fqn"`
}

type rawColumn struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DataType    string `json:"data_type"`
}

type rawDepends struct {
	Nodes []string `json:"nodes"`
}

type rawOwner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Build parses a manifest.json body into an Index (full manifest; product is instance metadata only).
func Build(body []byte, product, checksum, etag string, includeCatalog bool) (*Index, error) {
	var top rawTop
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("index: parse top: %w", err)
	}

	idx := &Index{
		Meta: Meta{
			Product:             product,
			Checksum:            checksum,
			ManifestGeneratedAt: top.Metadata.GeneratedAt,
			ETag:                etag,
		},
		includeCatalog: includeCatalog,
		ByUID:          make(map[string]*Node),
		ByFQN:          make(map[string]*Node),
		Exposures:      make(map[string]*Exposure),
		Stubs:          make(map[string]Stub),
		ParentMap:      top.ParentMap,
		ChildMap:       top.ChildMap,
	}
	if idx.ParentMap == nil {
		idx.ParentMap = map[string][]string{}
	}
	if idx.ChildMap == nil {
		idx.ChildMap = map[string][]string{}
	}

	if err := idx.loadNodes(top.Nodes); err != nil {
		return nil, err
	}
	if err := idx.loadSources(top.Sources); err != nil {
		return nil, err
	}
	if err := idx.loadExposures(top.Exposures); err != nil {
		return nil, err
	}
	idx.pruneLineageGraph(3)
	idx.buildSearchDocs()
	return idx, nil
}

func (idx *Index) loadNodes(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var nodes map[string]rawNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return fmt.Errorf("index: parse nodes: %w", err)
	}
	for uid, rn := range nodes {
		rt := rn.ResourceType
		switch rt {
		case ResourceModel, ResourceSnapshot:
			// keep; seeds dropped entirely (not stored)
		default:
			// drop seeds, tests, operations, etc.
			continue
		}
		path := rn.OriginalFilePath
		if path == "" {
			path = rn.Path
		}
		db := ""
		if rn.Database != nil {
			db = *rn.Database
		}
		catalog, sch, table, fqn := BuildFQN(db, rn.Schema, rn.Alias, rn.Name, rn.Identifier, idx.includeCatalog)
		n := &Node{
			UniqueID:     uid,
			ResourceType: rt,
			Name:         rn.Name,
			Alias:        firstNonEmpty(rn.Alias, table),
			FQN:          fqn,
			Catalog:      catalog,
			Schema:       sch,
			Table:        table,
			Path:         path,
			Description:  rn.Description,
			Tags:         rn.Tags,
			Columns:      mapColumns(rn.Columns),
			CompiledSQL:  truncateSQL(firstNonEmpty(rn.CompiledCode, rn.CompiledSQL)),
		}
		idx.Stubs[uid] = Stub{UniqueID: uid, FQN: fqn, ResourceType: rt, Hidden: false}
		idx.ByUID[uid] = n
		if prev, exists := idx.ByFQN[fqn]; exists && !preferFQNOwner(n, prev) {
			continue
		}
		idx.ByFQN[fqn] = n
	}
	return nil
}

func (idx *Index) loadSources(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var sources map[string]rawNode
	if err := json.Unmarshal(raw, &sources); err != nil {
		return fmt.Errorf("index: parse sources: %w", err)
	}
	for uid, rn := range sources {
		path := rn.OriginalFilePath
		if path == "" {
			path = rn.Path
		}
		db := ""
		if rn.Database != nil {
			db = *rn.Database
		}
		catalog, sch, table, fqn := BuildFQN(db, rn.Schema, rn.Alias, rn.Name, rn.Identifier, idx.includeCatalog)
		n := &Node{
			UniqueID:     uid,
			ResourceType: ResourceSource,
			Name:         rn.Name,
			Alias:        table,
			FQN:          fqn,
			Catalog:      catalog,
			Schema:       sch,
			Table:        table,
			Path:         path,
			Description:  rn.Description,
			Tags:         rn.Tags,
			Columns:      mapColumns(rn.Columns),
		}
		idx.Stubs[uid] = Stub{UniqueID: uid, FQN: fqn, ResourceType: ResourceSource, Hidden: false}
		idx.ByUID[uid] = n
		// Prefer model FQN if collision (same table in default_catalog vs already set).
		if _, exists := idx.ByFQN[fqn]; !exists {
			idx.ByFQN[fqn] = n
		}
	}
	return nil
}

func (idx *Index) loadExposures(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var exposures map[string]rawNode
	if err := json.Unmarshal(raw, &exposures); err != nil {
		return fmt.Errorf("index: parse exposures: %w", err)
	}
	for uid, rn := range exposures {
		deps := rn.DependsOn.Nodes
		hasIndexedDep := false
		for _, d := range deps {
			if _, ok := idx.ByUID[d]; ok {
				hasIndexedDep = true
				break
			}
		}
		e := &Exposure{
			UniqueID:    uid,
			Name:        rn.Name,
			Label:       rn.Label,
			Type:        rn.Type,
			URL:         rn.URL,
			Description: rn.Description,
			Tags:        rn.Tags,
			OwnerName:   rn.Owner.Name,
			OwnerEmail:  rn.Owner.Email,
			DependsOn:   deps,
		}
		idx.Stubs[uid] = Stub{UniqueID: uid, ResourceType: ResourceExposure, Hidden: !hasIndexedDep}
		if !hasIndexedDep {
			continue
		}
		idx.Exposures[uid] = e
	}
	return nil
}

func (idx *Index) buildSearchDocs() {
	docs := make([]SearchDoc, 0, len(idx.ByUID)+len(idx.Exposures))
	for _, n := range idx.ByUID {
		docs = append(docs, SearchDoc{
			ResourceType: n.ResourceType,
			UniqueID:     n.UniqueID,
			FQN:          n.FQN,
			Name:         n.Name,
			Alias:        n.Alias,
			Path:         n.Path,
			Description:  n.Description,
			Tags:         n.Tags,
			Catalog:      n.Catalog,
			Schema:       n.Schema,
			LayerBonus:   LayerBonus(n.Catalog, n.Schema, n.Path, n.ResourceType),
			Columns:      n.Columns,
		})
	}
	for _, e := range idx.Exposures {
		docs = append(docs, SearchDoc{
			ResourceType: ResourceExposure,
			UniqueID:     e.UniqueID,
			Name:         e.Name,
			Alias:        e.Name,
			Label:        e.Label,
			Description:  e.Description,
			Tags:         e.Tags,
		})
	}
	idx.SearchDocs = docs
}

func mapColumns(in map[string]rawColumn) []Column {
	if len(in) == 0 {
		return nil
	}
	out := make([]Column, 0, len(in))
	for name, c := range in {
		n := c.Name
		if n == "" {
			n = name
		}
		out = append(out, Column{
			Name:        n,
			Description: c.Description,
			DataType:    c.DataType,
		})
	}
	return out
}

func truncateSQL(s string) string {
	return truncateBytes(s, sqlCapBytes)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// TruncateDesc shortens description for search hits (UTF-8 safe).
func TruncateDesc(s string) string {
	return truncateBytes(s, descHitCap)
}

// truncateBytes cuts s to at most maxBytes without splitting a UTF-8 rune.
func truncateBytes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// preferFQNOwner picks a stable ByFQN winner on collision (model > snapshot, then unique_id).
func preferFQNOwner(cand, prev *Node) bool {
	if cand == nil {
		return false
	}
	if prev == nil {
		return true
	}
	cr, pr := fqnRank(cand.ResourceType), fqnRank(prev.ResourceType)
	if cr != pr {
		return cr > pr
	}
	return cand.UniqueID < prev.UniqueID
}

func fqnRank(rt string) int {
	switch rt {
	case ResourceModel:
		return 2
	case ResourceSnapshot:
		return 1
	default:
		return 0
	}
}

// pruneLineageGraph keeps parent/child/stubs within maxDepth of indexed nodes.
func (idx *Index) pruneLineageGraph(maxDepth int) {
	if maxDepth < 0 {
		maxDepth = 0
	}
	keep := make(map[string]struct{}, len(idx.ByUID)+len(idx.Exposures))
	frontier := make([]string, 0, len(idx.ByUID)+len(idx.Exposures))
	for uid := range idx.ByUID {
		keep[uid] = struct{}{}
		frontier = append(frontier, uid)
	}
	for uid := range idx.Exposures {
		if _, ok := keep[uid]; ok {
			continue
		}
		keep[uid] = struct{}{}
		frontier = append(frontier, uid)
	}
	for d := 0; d < maxDepth; d++ {
		if len(frontier) == 0 {
			break
		}
		next := make([]string, 0, len(frontier))
		for _, u := range frontier {
			for _, n := range idx.ParentMap[u] {
				if _, ok := keep[n]; ok {
					continue
				}
				keep[n] = struct{}{}
				next = append(next, n)
			}
			for _, n := range idx.ChildMap[u] {
				if _, ok := keep[n]; ok {
					continue
				}
				keep[n] = struct{}{}
				next = append(next, n)
			}
		}
		frontier = next
	}

	idx.ParentMap = filterEdgeMap(idx.ParentMap, keep)
	idx.ChildMap = filterEdgeMap(idx.ChildMap, keep)
	for uid := range idx.Stubs {
		if _, ok := keep[uid]; !ok {
			delete(idx.Stubs, uid)
		}
	}
}

func filterEdgeMap(m map[string][]string, keep map[string]struct{}) map[string][]string {
	if len(m) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(keep))
	for u := range keep {
		edges := m[u]
		if len(edges) == 0 {
			continue
		}
		filtered := make([]string, 0, len(edges))
		for _, n := range edges {
			if _, ok := keep[n]; ok {
				filtered = append(filtered, n)
			}
		}
		if len(filtered) > 0 {
			out[u] = filtered
		}
	}
	return out
}
