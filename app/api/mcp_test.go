package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/barloc/dbt-gate/app/api"
	"github.com/barloc/dbt-gate/app/auth"
	"github.com/barloc/dbt-gate/app/gate"
	"github.com/barloc/dbt-gate/app/index"
)

func TestUnitMCPToolsListAndSearch(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "index", "testdata", "manifest.mini.json"))
	require.NoError(t, err)

	store := index.NewTestStore("example")
	require.NoError(t, store.LoadBytes(body, "sha1mini"))
	svc := gate.NewService(store, slog.Default())
	ctrl := api.NewController(svc, auth.DisabledIdentity{}, slog.Default())
	h := ctrl.HandlerForTest()

	sessionID := mcpInitialize(t, h)

	listResp := mcpPost(t, h, sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	require.Nil(t, listResp["error"], "tools/list error: %v", listResp["error"])

	result, ok := listResp["result"].(map[string]any)
	require.True(t, ok)
	tools, ok := result["tools"].([]any)
	require.True(t, ok)
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool, isMap := raw.(map[string]any)
		require.True(t, isMap)
		name, hasName := tool["name"].(string)
		require.True(t, hasName)
		names = append(names, name)
	}
	require.ElementsMatch(t, []string{"search", "get_model", "lineage", "get_exposure"}, names)

	callResp := mcpPost(t, h, sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search",
			"arguments": map[string]any{
				"query": "users",
			},
		},
	})
	env := mcpEnvelope(t, callResp)
	require.True(t, env.OK)
	require.Equal(t, "example", env.Product)
	data, ok := env.Data.(map[string]any)
	require.True(t, ok)
	hits, ok := data["hits"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, hits)

	modelResp := mcpPost(t, h, sessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "get_model",
			"arguments": map[string]any{
				"fqn": "default_catalog.acme_marts.users",
			},
		},
	})
	modelEnv := mcpEnvelope(t, modelResp)
	require.True(t, modelEnv.OK)
	modelData, ok := modelEnv.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "default_catalog.acme_marts.users", modelData["fqn"])
}

func mcpEnvelope(t *testing.T, callResp map[string]any) gate.Envelope {
	t.Helper()
	require.Nil(t, callResp["error"], "tools/call error: %v", callResp["error"])

	callResult, ok := callResp["result"].(map[string]any)
	require.True(t, ok)
	require.NotEqual(t, true, callResult["isError"])

	content, ok := callResult["content"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, content)
	textItem, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, ok := textItem["text"].(string)
	require.True(t, ok)

	var env gate.Envelope
	require.NoError(t, json.Unmarshal([]byte(text), &env))
	return env
}

func mcpInitialize(t *testing.T, h http.Handler) string {
	t.Helper()
	resp := mcpPost(t, h, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "dbt-gate-test",
				"version": "1.0.0",
			},
		},
	})
	require.Nil(t, resp["error"], "initialize error: %v", resp["error"])

	sessionID := resp["_session_id"].(string)
	require.NotEmpty(t, sessionID)

	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	return sessionID
}

func mcpPost(t *testing.T, h http.Handler, sessionID string, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), "body=%s", body)

	if sid := rr.Header().Get("Mcp-Session-Id"); sid != "" {
		out["_session_id"] = sid
	}
	return out
}
