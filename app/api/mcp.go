package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/barloc/dbt-gate/app/gate"
)

func (c *Controller) buildMCPHandler() http.Handler {
	s := server.NewMCPServer("dbt-gate", "1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Lexical search over the instance dbt manifest catalog"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("resource_type", mcp.Description("Optional filter: model, source, exposure, snapshot")),
		mcp.WithNumber("limit", mcp.Description("Max hits (capped by server)")),
	), c.mcpSearch)

	s.AddTool(mcp.NewTool("get_model",
		mcp.WithDescription("Get a model (or source/snapshot) card by fqn, unique_id, or id"),
		mcp.WithString("fqn", mcp.Description("Fully-qualified name catalog.schema.table")),
		mcp.WithString("unique_id", mcp.Description("dbt unique_id")),
		mcp.WithString("id", mcp.Description("Alias for fqn or unique_id")),
	), c.mcpGetModel)

	s.AddTool(mcp.NewTool("lineage",
		mcp.WithDescription("Ancestors and descendants for a model/source in the manifest"),
		mcp.WithString("fqn", mcp.Description("Fully-qualified name catalog.schema.table")),
		mcp.WithString("unique_id", mcp.Description("dbt unique_id")),
		mcp.WithString("id", mcp.Description("Alias for fqn or unique_id")),
		mcp.WithNumber("depth", mcp.Description("Traversal depth (default 1, max 3)")),
	), c.mcpLineage)

	s.AddTool(mcp.NewTool("get_exposure",
		mcp.WithDescription("Get an exposure by unique_id, name, or id"),
		mcp.WithString("unique_id", mcp.Description("dbt unique_id")),
		mcp.WithString("name", mcp.Description("Exposure name")),
		mcp.WithString("id", mcp.Description("Alias for unique_id or name")),
	), c.mcpGetExposure)

	return server.NewStreamableHTTPServer(s,
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return r.Context()
		}),
	)
}

func (c *Controller) mcpSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.Search(ctx, gate.SearchRequest{
		Query:        argString(args, "query"),
		ResourceType: argString(args, "resource_type"),
		Limit:        argInt(args, "limit"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpGetModel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.GetModel(ctx, gate.GetModelRequest{
		FQN:      argString(args, "fqn"),
		UniqueID: argString(args, "unique_id"),
		ID:       argString(args, "id"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpLineage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.Lineage(ctx, gate.LineageRequest{
		FQN:      argString(args, "fqn"),
		UniqueID: argString(args, "unique_id"),
		ID:       argString(args, "id"),
		Depth:    argInt(args, "depth"),
	})
	return mcpResult(env), nil
}

func (c *Controller) mcpGetExposure(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	env := c.gate.GetExposure(ctx, gate.GetExposureRequest{
		UniqueID: argString(args, "unique_id"),
		Name:     argString(args, "name"),
		ID:       argString(args, "id"),
	})
	return mcpResult(env), nil
}

func mcpResult(env gate.Envelope) *mcp.CallToolResult {
	b, err := json.Marshal(env)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	if !env.OK {
		return mcp.NewToolResultError(string(b))
	}
	return mcp.NewToolResultText(string(b))
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func argInt(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	default:
		return 0
	}
}
