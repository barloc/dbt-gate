---
name: dbt-gate
description: >-
  Read-only dbt catalog via dbt-gate MCP/HTTP: search models/sources/exposures,
  get_model (incl. compiled_sql), lineage, get_exposure. Cite fqn + checksum from
  the envelope. Point MCP at a running dbt-gate instance; no warehouse queries.
---

# dbt-gate (example Cursor skill)

Use this skill when the user asks about dbt models, lineage, exposures, or
compiled SQL from a **dbt-gate** instance (local Docker/binary or a deployed URL).

Upstream project: [github.com/barloc/dbt-gate](https://github.com/barloc/dbt-gate).

## Install (Cursor)

1. Copy or symlink this directory into Cursor skills, e.g.:

```bash
mkdir -p ~/.cursor/skills
ln -sfn /path/to/dbt-gate/examples/skills/dbt-gate ~/.cursor/skills/dbt-gate
```

2. Register MCP (Streamable HTTP) in `~/.cursor/mcp.json` against a running gate:

```json
{
  "mcpServers": {
    "dbt-gate": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

For a remote deployment, replace the URL (one MCP entry = one product/instance).

3. Reload the Cursor window. Tools: `search`, `get_model`, `lineage`, `get_exposure`.

Local smoke without MCP:

```bash
# from the dbt-gate repo
make up && make smoke
# or: docker run -p 8080:8080 -p 8090:8090 barloc/dbt-gate:latest
```

## Hard rules

- **Catalog only** — dbt-gate does not execute SQL against the warehouse.
- Prefer MCP tools over inventing FQNs. If `search` misses, say not found (do not guess neighbors).
- Cite **`fqn`** (or `unique_id`) and envelope **`checksum`** / `manifest_generated_at` when quoting a model.
- `get_model` returns **compiled** SQL from the manifest (may be truncated); not the raw `.sql` from git.
- One MCP URL = one indexed manifest (one product instance).
- FQN shape: `catalog.schema.table` by default; some deployments use `schema.table` (`include_catalog: false`).

## Suggested flow

1. `search` with a short query (`users`, table alias, column name).
2. Pick the best hit (prefer marts over dds/sources when scores tie — server already ranks).
3. `get_model` for description, columns, `compiled_sql`, path.
4. `lineage` when the user asks upstream/downstream.
5. `get_exposure` for dashboards/apps wired in the manifest.

## HTTP equivalent

Same tools as JSON POST on the API port (default `:8080`):

| Tool | Method |
| --- | --- |
| `search` | `POST /v1/search` |
| `get_model` | `POST /v1/get_model` |
| `lineage` | `POST /v1/lineage` |
| `get_exposure` | `POST /v1/get_exposure` |

Example:

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"users"}'
```
