package index

import (
	"sort"
	"strings"
	"unicode"
)

// SearchRequest is the search tool input.
type SearchRequest struct {
	Query        string
	ResourceType string // model|exposure|source|column|""
	Limit        int
	Allow        Allowlist
}

// SearchHit is one ranked result.
type SearchHit struct {
	ResourceType string         `json:"resource_type"`
	FQN          string         `json:"fqn,omitempty"`
	UniqueID     string         `json:"unique_id"`
	Name         string         `json:"name,omitempty"`
	Alias        string         `json:"alias,omitempty"`
	Label        string         `json:"label,omitempty"`
	Path         string         `json:"path,omitempty"`
	Description  string         `json:"description,omitempty"`
	Matched      map[string]any `json:"matched,omitempty"`
	Score        int            `json:"-"`
}

// Search runs lexical AND substring search with ranking.
func (idx *Index) Search(req SearchRequest) ([]SearchHit, error) {
	q := strings.TrimSpace(req.Query)
	if len([]rune(q)) < 2 {
		return nil, ErrInvalidQuery
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 20 {
		limit = 20
	}
	tokens := Tokenize(q)
	if len(tokens) == 0 {
		return nil, ErrInvalidQuery
	}

	wantRT := strings.ToLower(strings.TrimSpace(req.ResourceType))
	hits := make([]SearchHit, 0, 32)

	for _, doc := range idx.SearchDocs {
		if !docAllowed(doc, wantRT, req.Allow) {
			continue
		}
		hit, ok := scoreDoc(doc, tokens, wantRT)
		if !ok {
			continue
		}
		hits = append(hits, hit)
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		ai, aj := hits[i].Alias, hits[j].Alias
		if len(ai) != len(aj) {
			return len(ai) < len(aj)
		}
		return hits[i].FQN < hits[j].FQN
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func docAllowed(doc SearchDoc, wantRT string, allow Allowlist) bool {
	switch wantRT {
	case "":
		// default: model + exposure + source (column matches still score models).
		// Snapshots remain opt-in via resource_type.
		if doc.ResourceType != ResourceModel && doc.ResourceType != ResourceExposure && doc.ResourceType != ResourceSource {
			return false
		}
	case "column":
		if doc.ResourceType != ResourceModel {
			return false
		}
	case ResourceModel:
		if doc.ResourceType != ResourceModel {
			return false
		}
	case ResourceExposure:
		if doc.ResourceType != ResourceExposure {
			return false
		}
	case ResourceSource:
		if doc.ResourceType != ResourceSource {
			return false
		}
	case ResourceSnapshot:
		if doc.ResourceType != wantRT {
			return false
		}
	default:
		if doc.ResourceType != wantRT {
			return false
		}
	}
	if doc.ResourceType == ResourceExposure {
		return true
	}
	cs := doc.Catalog + "." + doc.Schema
	return allow.Allows(cs)
}

func scoreDoc(doc SearchDoc, tokens []string, wantRT string) (SearchHit, bool) {
	fieldsL := lowerDoc(doc)
	matchedFields := map[string]struct{}{}
	var matchedCols []string
	score := 0
	columnOnly := wantRT == "column"

	for _, tok := range tokens {
		best, tokHit, colHit, cols := scoreToken(doc, fieldsL, tok, matchedFields)
		matchedCols = appendUniqueAll(matchedCols, cols)
		if columnOnly && !colHit {
			return SearchHit{}, false
		}
		if !tokHit {
			return SearchHit{}, false
		}
		score += best
	}

	score += doc.LayerBonus
	return buildHit(doc, matchedFields, matchedCols, score), true
}

type docLower struct {
	fqn, name, alias, path, desc, label, tags string
}

func lowerDoc(doc SearchDoc) docLower {
	return docLower{
		fqn:   strings.ToLower(doc.FQN),
		name:  strings.ToLower(doc.Name),
		alias: strings.ToLower(doc.Alias),
		path:  strings.ToLower(doc.Path),
		desc:  strings.ToLower(doc.Description),
		label: strings.ToLower(doc.Label),
		tags:  strings.ToLower(strings.Join(doc.Tags, " ")),
	}
}

func scoreToken(doc SearchDoc, low docLower, tok string, matchedFields map[string]struct{}) (best int, tokHit, colHit bool, cols []string) {
	if low.fqn == tok {
		best = max(best, 100)
		matchedFields["fqn"] = struct{}{}
		tokHit = true
	} else if strings.HasPrefix(low.fqn, tok+".") || strings.Contains(low.fqn, "."+tok+".") || strings.HasSuffix(low.fqn, "."+tok) {
		best = max(best, 55)
		matchedFields["fqn"] = struct{}{}
		tokHit = true
	}
	if low.alias == tok || low.name == tok {
		best = max(best, 80)
		matchedFields["alias"] = struct{}{}
		tokHit = true
	}
	if strings.Contains(low.fqn, tok) || strings.Contains(low.name, tok) || strings.Contains(low.path, tok) {
		best = max(best, 25)
		matchedFields["path"] = struct{}{}
		tokHit = true
	}
	if strings.Contains(low.desc, tok) {
		best = max(best, 12)
		matchedFields["description"] = struct{}{}
		tokHit = true
	}
	if doc.ResourceType == ResourceExposure && strings.Contains(low.label, tok) {
		best = max(best, 10)
		matchedFields["label"] = struct{}{}
		tokHit = true
	}
	if strings.Contains(low.tags, tok) {
		best = max(best, 8)
		matchedFields["tags"] = struct{}{}
		tokHit = true
	}

	for _, c := range doc.Columns {
		cn := strings.ToLower(c.Name)
		cd := strings.ToLower(c.Description)
		switch {
		case cn == tok:
			best = max(best, 40)
			matchedFields["column"] = struct{}{}
			cols = appendUnique(cols, c.Name)
			tokHit, colHit = true, true
		case strings.Contains(cd, tok):
			best = max(best, 8)
			matchedFields["column"] = struct{}{}
			cols = appendUnique(cols, c.Name)
			tokHit, colHit = true, true
		case strings.Contains(cn, tok):
			best = max(best, 25)
			matchedFields["column"] = struct{}{}
			cols = appendUnique(cols, c.Name)
			tokHit, colHit = true, true
		}
	}
	return best, tokHit, colHit, cols
}

func buildHit(doc SearchDoc, matchedFields map[string]struct{}, matchedCols []string, score int) SearchHit {
	fields := make([]string, 0, len(matchedFields))
	for f := range matchedFields {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	matched := map[string]any{"fields": fields}
	if len(matchedCols) > 0 {
		matched["column"] = matchedCols[0]
		if len(matchedCols) > 1 {
			matched["columns"] = matchedCols
		}
	}
	return SearchHit{
		ResourceType: doc.ResourceType,
		FQN:          doc.FQN,
		UniqueID:     doc.UniqueID,
		Name:         doc.Name,
		Alias:        doc.Alias,
		Label:        doc.Label,
		Path:         doc.Path,
		Description:  TruncateDesc(doc.Description),
		Matched:      matched,
		Score:        score,
	}
}

func appendUniqueAll(dst, add []string) []string {
	for _, v := range add {
		dst = appendUnique(dst, v)
	}
	return dst
}

// Tokenize splits query into lowercase tokens; keeps original substrings with _ and .
func Tokenize(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}
	for _, r := range q {
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	// Also add underscore/dot-split parts as additional AND tokens? Plan says:
	// "_ and . are separators AND part of original string (search both users and acme_marts.users)".
	// Interpretation: query "acme_marts.users" is one token for FQN match; whitespace splits AND tokens.
	// So we do NOT split on _/. for AND — only whitespace. Good.
	return tokens
}

func appendUnique(ss []string, v string) []string {
	for _, s := range ss {
		if s == v {
			return ss
		}
	}
	return append(ss, v)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ErrInvalidQuery is returned for empty / too-short queries.
var ErrInvalidQuery = errInvalidQuery{}

type errInvalidQuery struct{}

func (errInvalidQuery) Error() string { return "query must be at least 2 characters" }
