package index

import "strings"

// BuildFQN constructs catalog/schema/table from dbt node fields and a public FQN.
// Models typically have database=null and schema without catalog.
// Sources often embed catalog in schema as "ext.acme_marts".
// When includeCatalog is false, FQN is "schema.table".
func BuildFQN(database, schema, alias, name, identifier string, includeCatalog bool) (catalog, sch, table, fqn string) {
	table = alias
	if table == "" {
		table = identifier
	}
	if table == "" {
		table = name
	}
	database = strings.TrimSpace(database)
	schema = strings.TrimSpace(schema)

	if database != "" {
		catalog = database
		sch = schema
	} else if i := strings.IndexByte(schema, '.'); i >= 0 {
		catalog = schema[:i]
		sch = schema[i+1:]
	} else {
		catalog = defaultCatalog
		sch = schema
	}
	if includeCatalog {
		fqn = catalog + "." + sch + "." + table
	} else {
		fqn = sch + "." + table
	}
	return catalog, sch, table, fqn
}

// LayerBonus returns ranking bonus for warehouse layer.
// Only models in default_catalog outside external_catalogs/ get marts/pds (+15) or dds (+8).
// Sources and external catalog models stay at 0 so they rank below warehouse marts/dds on alias ties.
func LayerBonus(catalog, schema, path, resourceType string) int {
	if resourceType != ResourceModel || catalog != defaultCatalog {
		return 0
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if strings.Contains(path, "external_catalogs/") {
		return 0
	}
	s := strings.ToLower(schema + " " + path)
	switch {
	case strings.Contains(s, "_marts") || strings.Contains(s, "/marts/") || strings.Contains(s, "_pds") || strings.Contains(s, "/pds/"):
		return 15
	case strings.Contains(s, "_dds") || strings.Contains(s, "/dds/"):
		return 8
	default:
		return 0
	}
}
