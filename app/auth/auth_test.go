package auth_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/barloc/dbt-gate/app/auth"
)

func TestUnitDisabledIdentity(t *testing.T) {
	var src auth.DisabledIdentity
	id, err := src.FromRequest(&http.Request{})
	require.NoError(t, err)
	require.Equal(t, "anonymous", id.Sub)
}

func TestUnitStaticIdentity(t *testing.T) {
	src := auth.StaticIdentity{Identity: auth.Identity{Sub: "u1", Email: "u@x"}}
	id, err := src.FromRequest(&http.Request{})
	require.NoError(t, err)
	require.Equal(t, "u1", id.Sub)
}
