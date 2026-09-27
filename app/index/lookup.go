package index

import "strings"

// GetModel looks up a node by fqn or unique_id in the manifest (+ allowlist).
func (idx *Index) GetModel(id string, allow Allowlist) (*Node, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	n, ok := idx.ByUID[id]
	if !ok {
		n, ok = idx.ByFQN[id]
	}
	if !ok || n == nil {
		if s, stubOK := idx.Stubs[id]; stubOK {
			if s.Hidden {
				return nil, ErrForbidden
			}
			return nil, ErrNotFound
		}
		for _, s := range idx.Stubs {
			if s.FQN == id {
				if s.Hidden {
					return nil, ErrForbidden
				}
				return nil, ErrNotFound
			}
		}
		return nil, ErrNotFound
	}
	if n.ResourceType == ResourceSource || n.ResourceType == ResourceModel ||
		n.ResourceType == ResourceSnapshot {
		if !allow.Allows(n.CatalogSchema()) {
			return nil, ErrForbidden
		}
		return n, nil
	}
	return nil, ErrNotFound
}

// GetExposure looks up an exposure by unique_id or name.
func (idx *Index) GetExposure(id string) (*Exposure, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	if e, ok := idx.Exposures[id]; ok {
		return e, nil
	}
	for _, e := range idx.Exposures {
		if e.Name == id {
			return e, nil
		}
	}
	if s, ok := idx.Stubs[id]; ok && s.ResourceType == ResourceExposure {
		return nil, ErrForbidden
	}
	return nil, ErrNotFound
}

// LineageNode is one node in a lineage response.
type LineageNode struct {
	UniqueID     string `json:"unique_id"`
	FQN          string `json:"fqn,omitempty"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name,omitempty"`
	Alias        string `json:"alias,omitempty"`
	Path         string `json:"path,omitempty"`
	Hidden       bool   `json:"hidden,omitempty"`
}

// LineageResult holds ancestors/descendants.
type LineageResult struct {
	Root        LineageNode   `json:"root"`
	Ancestors   []LineageNode `json:"ancestors"`
	Descendants []LineageNode `json:"descendants"`
	Depth       int           `json:"depth"`
}

// Lineage walks parent_map / child_map up to depth (default 1, cap 3).
func (idx *Index) Lineage(id string, depth int, allow Allowlist) (*LineageResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	uid := id
	if n, ok := idx.ByFQN[id]; ok {
		uid = n.UniqueID
	} else if e, ok := idx.Exposures[id]; ok {
		uid = e.UniqueID
	} else if _, ok := idx.ByUID[id]; !ok {
		// try exposure name / stub fqn
		found := false
		for _, e := range idx.Exposures {
			if e.Name == id {
				uid = e.UniqueID
				found = true
				break
			}
		}
		if !found {
			for u, s := range idx.Stubs {
				if s.FQN == id || u == id {
					uid = u
					found = true
					break
				}
			}
		}
		if !found {
			return nil, ErrNotFound
		}
	}

	root := idx.lineageCard(uid, allow)
	if root.UniqueID == "" {
		return nil, ErrNotFound
	}

	return &LineageResult{
		Root:        root,
		Ancestors:   idx.walk(uid, idx.ParentMap, depth, allow),
		Descendants: idx.walk(uid, idx.ChildMap, depth, allow),
		Depth:       depth,
	}, nil
}

func (idx *Index) walk(start string, edges map[string][]string, depth int, allow Allowlist) []LineageNode {
	seen := map[string]struct{}{start: {}}
	frontier := []string{start}
	var out []LineageNode
	for d := 0; d < depth; d++ {
		var next []string
		for _, u := range frontier {
			for _, child := range edges[u] {
				if _, ok := seen[child]; ok {
					continue
				}
				seen[child] = struct{}{}
				out = append(out, idx.lineageCard(child, allow))
				next = append(next, child)
			}
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}
	return out
}

func (idx *Index) lineageCard(uid string, allow Allowlist) LineageNode {
	if n, ok := idx.ByUID[uid]; ok {
		hidden := !allow.Allows(n.CatalogSchema())
		if hidden {
			return LineageNode{
				UniqueID:     uid,
				FQN:          n.FQN,
				ResourceType: n.ResourceType,
				Hidden:       true,
			}
		}
		return LineageNode{
			UniqueID:     uid,
			FQN:          n.FQN,
			ResourceType: n.ResourceType,
			Name:         n.Name,
			Alias:        n.Alias,
			Path:         n.Path,
		}
	}
	if e, ok := idx.Exposures[uid]; ok {
		return LineageNode{
			UniqueID:     uid,
			ResourceType: ResourceExposure,
			Name:         e.Name,
		}
	}
	if s, ok := idx.Stubs[uid]; ok {
		return LineageNode{
			UniqueID:     s.UniqueID,
			FQN:          s.FQN,
			ResourceType: s.ResourceType,
			Hidden:       true,
		}
	}
	return LineageNode{UniqueID: uid, Hidden: true, ResourceType: "unknown"}
}

// Errors.
var (
	ErrNotFound  = errCode("not_found")
	ErrForbidden = errCode("forbidden")
)

type errCode string

func (e errCode) Error() string { return string(e) }
