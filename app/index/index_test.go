package index_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/barloc/dbt-gate/app/index"
)

func loadMini(t *testing.T) *index.Index {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	require.NoError(t, err)
	idx, err := index.Build(body, "example", "sha1mini", "sha1mini", true)
	require.NoError(t, err)
	return idx
}

func TestUnitBuildFQN(t *testing.T) {
	cat, sch, table, fqn := index.BuildFQN("", "acme_marts", "users", "acme_marts_users", "", true)
	require.Equal(t, "default_catalog", cat)
	require.Equal(t, "acme_marts", sch)
	require.Equal(t, "users", table)
	require.Equal(t, "default_catalog.acme_marts.users", fqn)

	cat, sch, table, fqn = index.BuildFQN("", "lakehouse.acme_marts", "", "users", "users", true)
	require.Equal(t, "lakehouse", cat)
	require.Equal(t, "acme_marts", sch)
	require.Equal(t, "users", table)
	require.Equal(t, "lakehouse.acme_marts.users", fqn)
}

func TestUnitBuildFQNWithoutCatalog(t *testing.T) {
	_, sch, table, fqn := index.BuildFQN("", "acme_marts", "users", "acme_marts_users", "", false)
	require.Equal(t, "acme_marts", sch)
	require.Equal(t, "users", table)
	require.Equal(t, "acme_marts.users", fqn)
}

func TestUnitBuildIndexesFullManifest(t *testing.T) {
	idx := loadMini(t)
	require.Contains(t, idx.ByFQN, "default_catalog.acme_marts.users")
	require.Contains(t, idx.ByUID, "model.acme.acme_dds_users")
	require.Contains(t, idx.ByFQN, "lakehouse.acme_marts.users")
	require.Contains(t, idx.Exposures, "exposure.acme.users_dashboard")
	require.Equal(t, "2026-08-21T12:00:00.000000Z", idx.Meta.ManifestGeneratedAt)
	require.Equal(t, "example", idx.Meta.Product)
}

func TestUnitSearchUsersRanksMarts(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query: "users",
		Limit: 20,
		Allow: index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "default_catalog.acme_marts.users", hits[0].FQN)

	martsIdx, sourceIdx, ddsIdx, mysqlIdx := -1, -1, -1, -1
	for i, h := range hits {
		switch h.FQN {
		case "default_catalog.acme_marts.users":
			martsIdx = i
		case "lakehouse.acme_marts.users":
			sourceIdx = i
		case "default_catalog.acme_dds.users":
			ddsIdx = i
		case "default_catalog.acme_mysql_union.users":
			mysqlIdx = i
		}
	}
	require.Equal(t, 0, martsIdx, "marts must be first")
	require.GreaterOrEqual(t, sourceIdx, 0, "source must appear in default search")
	require.GreaterOrEqual(t, ddsIdx, 0, "dds must appear")
	require.GreaterOrEqual(t, mysqlIdx, 0, "mysql external must appear")
	require.Less(t, martsIdx, ddsIdx, "marts before dds")
	require.Less(t, ddsIdx, sourceIdx, "dds before source")
	require.Less(t, ddsIdx, mysqlIdx, "dds before mysql")
}

func TestUnitSearchExactFQNSource(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query: "lakehouse.acme_marts.users",
		Allow: index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "lakehouse.acme_marts.users", hits[0].FQN)
	require.Equal(t, "source", hits[0].ResourceType)
}

func TestUnitLayerBonus(t *testing.T) {
	tests := []struct {
		name         string
		catalog      string
		schema       string
		path         string
		resourceType string
		want         int
	}{
		{
			name:         "marts model",
			catalog:      "default_catalog",
			schema:       "acme_marts",
			path:         "models/marts/users/acme_marts_users.sql",
			resourceType: "model",
			want:         15,
		},
		{
			name:         "dds model",
			catalog:      "default_catalog",
			schema:       "acme_dds",
			path:         "models/dds/users/acme_dds_users.sql",
			resourceType: "model",
			want:         8,
		},
		{
			name:         "source with _marts in schema",
			catalog:      "lakehouse",
			schema:       "acme_marts",
			path:         "models/external_catalogs/lakehouse/acme_marts/sources.yml",
			resourceType: "source",
			want:         0,
		},
		{
			name:         "external_catalogs model",
			catalog:      "default_catalog",
			schema:       "acme_mysql_union",
			path:         "models/external_catalogs/acme_mysql_shards/acme_mysql_shards_users.sql",
			resourceType: "model",
			want:         0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, index.LayerBonus(tt.catalog, tt.schema, tt.path, tt.resourceType))
		})
	}
}

func TestUnitSearchExactFQN(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query: "default_catalog.acme_marts.users",
		Allow: index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "default_catalog.acme_marts.users", hits[0].FQN)
}

func TestUnitSearchPhoneColumn(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query: "phone",
		Allow: index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "default_catalog.acme_marts.users", hits[0].FQN)
}

func TestUnitSearchExposure(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query:        "dashboard",
		ResourceType: "exposure",
		Allow:        index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "exposure", hits[0].ResourceType)
}

func TestUnitSearchSourceOptIn(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query:        "users",
		ResourceType: "source",
		Allow:        index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "lakehouse.acme_marts.users", hits[0].FQN)
}

func TestUnitSearchAND(t *testing.T) {
	idx := loadMini(t)
	hits, err := idx.Search(index.SearchRequest{
		Query: "acme_marts users",
		Allow: index.Allowlist{All: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	require.Equal(t, "default_catalog.acme_marts.users", hits[0].FQN)
}

func TestUnitSearchInvalidQuery(t *testing.T) {
	idx := loadMini(t)
	_, err := idx.Search(index.SearchRequest{Query: "a", Allow: index.Allowlist{All: true}})
	require.ErrorIs(t, err, index.ErrInvalidQuery)
}

func TestUnitGetModelAndLineage(t *testing.T) {
	idx := loadMini(t)
	n, err := idx.GetModel("default_catalog.acme_marts.users", index.Allowlist{All: true})
	require.NoError(t, err)
	require.Equal(t, "models/marts/users/acme_marts_users.sql", n.Path)
	require.Contains(t, n.CompiledSQL, "select")

	lin, err := idx.Lineage(n.UniqueID, 1, index.Allowlist{All: true})
	require.NoError(t, err)
	require.NotEmpty(t, lin.Ancestors)
}

func TestUnitTruncateUTF8(t *testing.T) {
	s := "абвгд" // 2 bytes per rune in UTF-8
	out := index.TruncateDesc(s)
	require.True(t, utf8.ValidString(out))

	// Force cut inside multi-byte sequence via internal path: use long Cyrillic.
	long := strings.Repeat("я", 300) // 600 bytes
	cut := index.TruncateDesc(long)
	require.True(t, utf8.ValidString(cut))
	require.LessOrEqual(t, len(cut), 500)
}

func TestUnitSeedsNotStored(t *testing.T) {
	body := []byte(`{
	  "metadata": {"generated_at": "2026-01-01T00:00:00Z"},
	  "nodes": {
	    "seed.acme.acme_seed_users": {
	      "unique_id": "seed.acme.acme_seed_users",
	      "resource_type": "seed",
	      "name": "acme_seed_users",
	      "alias": "users",
	      "schema": "acme_seeds",
	      "original_file_path": "models/acme/seeds/users.csv",
	      "description": "seed users"
	    },
	    "model.acme.acme_marts_users": {
	      "unique_id": "model.acme.acme_marts_users",
	      "resource_type": "model",
	      "name": "acme_marts_users",
	      "alias": "users",
	      "schema": "acme_marts",
	      "original_file_path": "models/acme/marts/users.sql",
	      "description": "mart users"
	    }
	  },
	  "sources": {},
	  "exposures": {},
	  "parent_map": {},
	  "child_map": {}
	}`)
	idx, err := index.Build(body, "example", "x", "x", true)
	require.NoError(t, err)
	require.NotContains(t, idx.ByUID, "seed.acme.acme_seed_users")
	require.NotContains(t, idx.Stubs, "seed.acme.acme_seed_users")
	for _, doc := range idx.SearchDocs {
		require.NotEqual(t, "seed", doc.ResourceType)
	}
}

func TestUnitFQNCollisionPrefersModel(t *testing.T) {
	body := []byte(`{
	  "metadata": {"generated_at": "2026-01-01T00:00:00Z"},
	  "nodes": {
	    "snapshot.acme.acme_users_snap": {
	      "unique_id": "snapshot.acme.acme_users_snap",
	      "resource_type": "snapshot",
	      "name": "acme_users_snap",
	      "alias": "users",
	      "schema": "acme_marts",
	      "original_file_path": "models/acme/snapshots/users.sql"
	    },
	    "model.acme.acme_marts_users": {
	      "unique_id": "model.acme.acme_marts_users",
	      "resource_type": "model",
	      "name": "acme_marts_users",
	      "alias": "users",
	      "schema": "acme_marts",
	      "original_file_path": "models/acme/marts/users.sql"
	    }
	  },
	  "sources": {},
	  "exposures": {},
	  "parent_map": {},
	  "child_map": {}
	}`)
	idx, err := index.Build(body, "example", "x", "x", true)
	require.NoError(t, err)
	n := idx.ByFQN["default_catalog.acme_marts.users"]
	require.NotNil(t, n)
	require.Equal(t, "model", n.ResourceType)
	require.Equal(t, "model.acme.acme_marts_users", n.UniqueID)
}

func TestUnitPruneKeepsIndexedLineage(t *testing.T) {
	idx := loadMini(t)
	require.Contains(t, idx.Stubs, "model.acme.acme_dds_users")
	require.Contains(t, idx.ParentMap, "model.acme.acme_marts_users")
}

func TestUnitOrphanModelIndexedInManifest(t *testing.T) {
	body := []byte(`{
	  "metadata": {"generated_at": "2026-01-01T00:00:00Z"},
	  "nodes": {
	    "model.acme.acme_marts_users": {
	      "unique_id": "model.acme.acme_marts_users",
	      "resource_type": "model",
	      "name": "acme_marts_users",
	      "alias": "users",
	      "schema": "acme_marts",
	      "original_file_path": "models/marts/users/acme_marts_users.sql"
	    },
	    "model.acme.orphan_unrelated": {
	      "unique_id": "model.acme.orphan_unrelated",
	      "resource_type": "model",
	      "name": "orphan_unrelated",
	      "alias": "orphan",
	      "schema": "acme_misc",
	      "original_file_path": "models/misc/orphan_unrelated.sql"
	    }
	  },
	  "sources": {},
	  "exposures": {},
	  "parent_map": {},
	  "child_map": {}
	}`)
	idx, err := index.Build(body, "example", "x", "x", true)
	require.NoError(t, err)
	require.Contains(t, idx.ByUID, "model.acme.orphan_unrelated")
	_, err = idx.GetModel("default_catalog.acme_misc.orphan", index.Allowlist{All: true})
	require.NoError(t, err)
}

func TestUnitFetchFileETag(t *testing.T) {
	path := filepath.Join("testdata", "manifest.mini.json")
	f := index.Fetcher{File: path}
	res, err := f.HeadOrGet("")
	require.NoError(t, err)
	require.False(t, res.NotModified)
	require.NotEmpty(t, res.Body)

	res2, err := f.HeadOrGet(res.ETag)
	require.NoError(t, err)
	require.True(t, res2.NotModified)
}
