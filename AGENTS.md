# Agent notes (dbt-gate)

Read-only dbt catalog gate: indexes a `manifest.json`, exposes HTTP `/v1` + MCP `/mcp`.

## Run locally

```bash
make test
make up          # or: make run
make smoke
```

Ports: API/MCP `:8080`, health/metrics `:8090`.

MCP: `http://127.0.0.1:8080/mcp` — tools `search`, `get_model`, `lineage`, `get_exposure`.

## Config

- `index.file` or `index.url` (raw `manifest.json` required)
- `index.include_catalog` (default `true` → `catalog.schema.table`)

Example skill for Cursor: [`examples/skills/dbt-gate/SKILL.md`](examples/skills/dbt-gate/SKILL.md).

## Do not

- Treat this service as a SQL runner
- Assume dbt Docs v2 / Parquet / Cloud Explore URLs work without a downloadable manifest
