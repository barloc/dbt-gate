package index

const (
	ResourceModel    = "model"
	ResourceSource   = "source"
	ResourceExposure = "exposure"
	ResourceSnapshot = "snapshot"

	defaultCatalog = "default_catalog"
	sqlCapBytes    = 8 * 1024
	descHitCap     = 500
)

// Column is a documented column from the manifest.
type Column struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DataType    string `json:"data_type,omitempty"`
}

// Node is a compact catalog node (model / source / snapshot).
type Node struct {
	UniqueID     string
	ResourceType string
	Name         string
	Alias        string
	FQN          string
	Catalog      string
	Schema       string
	Table        string
	Path         string
	Description  string
	Tags         []string
	Owner        string
	Columns      []Column
	CompiledSQL  string
}

// CatalogSchema returns "catalog.schema" for AuthZ allowlists.
func (n *Node) CatalogSchema() string {
	return n.Catalog + "." + n.Schema
}

// Exposure is a compact exposure node.
type Exposure struct {
	UniqueID    string
	Name        string
	Label       string
	Type        string
	URL         string
	Description string
	Tags        []string
	OwnerName   string
	OwnerEmail  string
	DependsOn   []string // unique_ids
}

// Stub is a thin lineage placeholder for lineage graph nodes (allowlist may hide details).
type Stub struct {
	UniqueID     string `json:"unique_id"`
	FQN          string `json:"fqn,omitempty"`
	ResourceType string `json:"resource_type"`
	Hidden       bool   `json:"hidden"`
}

// Meta is loaded-manifest envelope fields.
type Meta struct {
	Product             string
	Checksum            string
	ManifestGeneratedAt string
	ETag                string
}

// Index is an immutable snapshot of the catalog for one product instance.
type Index struct {
	Meta      Meta
	ByUID     map[string]*Node
	ByFQN     map[string]*Node
	Exposures map[string]*Exposure
	// Stubs covers lineage placeholders (pruned nodes drop out after build).
	Stubs     map[string]Stub
	ParentMap map[string][]string
	ChildMap  map[string][]string
	// SearchDocs are models/exposures/sources in the manifest (snapshots opt-in via resource_type).
	SearchDocs []SearchDoc

	includeCatalog bool
}

// Allowlist filters catalog.schema. All=true allows every catalog.schema (open allowlist).
type Allowlist struct {
	All     bool
	Schemas map[string]struct{}
}

// Allows reports whether catalog.schema is permitted.
func (a Allowlist) Allows(catalogSchema string) bool {
	if a.All {
		return true
	}
	if a.Schemas == nil {
		return false
	}
	_, ok := a.Schemas[catalogSchema]
	return ok
}

// SearchDoc is a precomputed search document.
type SearchDoc struct {
	ResourceType string
	UniqueID     string
	FQN          string
	Name         string
	Alias        string
	Label        string
	Path         string
	Description  string
	Tags         []string
	Catalog      string
	Schema       string
	LayerBonus   int
	Columns      []Column
}
