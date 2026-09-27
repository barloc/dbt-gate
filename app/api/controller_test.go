package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/barloc/dbt-gate/app/gate"
)

func TestUnitStatusFor(t *testing.T) {
	require.Equal(t, http.StatusOK, statusFor(gate.Envelope{OK: true}))
	require.Equal(t, http.StatusBadRequest, statusFor(gate.Envelope{Error: &gate.Error{Code: "invalid_argument"}}))
	require.Equal(t, http.StatusUnauthorized, statusFor(gate.Envelope{Error: &gate.Error{Code: "unauthorized"}}))
	require.Equal(t, http.StatusForbidden, statusFor(gate.Envelope{Error: &gate.Error{Code: "forbidden"}}))
	require.Equal(t, http.StatusNotFound, statusFor(gate.Envelope{Error: &gate.Error{Code: "not_found"}}))
	require.Equal(t, http.StatusServiceUnavailable, statusFor(gate.Envelope{Error: &gate.Error{Code: "index_unavailable"}}))
	require.Equal(t, http.StatusInternalServerError, statusFor(gate.Envelope{Error: &gate.Error{Code: "internal"}}))
	require.Equal(t, http.StatusInternalServerError, statusFor(gate.Envelope{}))
}
