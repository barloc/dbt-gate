package gate_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/barloc/dbt-gate/app/gate"
	"github.com/barloc/dbt-gate/app/index"
)

func TestUnitSearchEnvelope(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "index", "testdata", "manifest.mini.json"))
	require.NoError(t, err)

	store := index.NewTestStore("example")
	require.NoError(t, store.LoadBytes(body, "sha1mini"))

	svc := gate.NewService(store, slog.Default())
	env := svc.Search(context.Background(), gate.SearchRequest{Query: "users"})
	require.True(t, env.OK)
	require.Equal(t, "example", env.Product)
	require.Equal(t, "sha1mini", env.Checksum)
	require.NotEmpty(t, env.ManifestGeneratedAt)

	env = svc.Search(context.Background(), gate.SearchRequest{Query: "x"})
	require.False(t, env.OK)
	require.Equal(t, "invalid_argument", env.Error.Code)

	env = svc.GetModel(context.Background(), gate.GetModelRequest{FQN: "default_catalog.acme_marts.users"})
	require.True(t, env.OK)

	env = svc.GetExposure(context.Background(), gate.GetExposureRequest{Name: "users_dashboard"})
	require.True(t, env.OK)
}

func TestUnitIndexUnavailable(t *testing.T) {
	store := index.NewTestStore("example")
	svc := gate.NewService(store, slog.Default())

	env := svc.Search(context.Background(), gate.SearchRequest{Query: "users"})
	require.False(t, env.OK)
	require.Equal(t, "index_unavailable", env.Error.Code)
	require.Equal(t, "example", env.Product)
	require.Empty(t, env.Checksum)

	env = svc.GetModel(context.Background(), gate.GetModelRequest{FQN: "default_catalog.acme_marts.users"})
	require.False(t, env.OK)
	require.Equal(t, "index_unavailable", env.Error.Code)
}
