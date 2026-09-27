# dbt-gate

Read-only **dbt catalog gate** for IDE agents: search models/sources/exposures, fetch model cards (including compiled SQL), and lineage.

Works with any dbt project that produces a standard `manifest.json`. There is no warehouse client and no StarRocks-specific runtime — StarRocks is just one common deployment shape.

License: [Apache-2.0](LICENSE).

## Features

- HTTP `/v1` tools: `search`, `get_model`, `lineage`, `get_exposure`
- Streamable HTTP MCP on `/mcp` (same four tools)
- Index from local file or any HTTP URL to raw `manifest.json` (Artifactory, S3, classic dbt docs host, CI artifact)
- Optional three-level FQN (`catalog.schema.table`) or two-level (`schema.table`)

Not supported: dbt Docs v2 / Fusion Parquet UI, `static_index.html` embeds, dbt Cloud Explore without a downloadable manifest.

## Quick start

```bash
make test
make run          # uses config.example.yaml + fixture manifest
# in another terminal:
make smoke
```

Or with Docker Compose:

```bash
make up
make smoke
make down
```

Cursor MCP (local):

```json
{
  "mcpServers": {
    "dbt-gate": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

| Port | Role |
| --- | --- |
| `:8090` | `/health`, `/metrics` |
| `:8080` | API `/v1/*` and MCP `/mcp` |

## Configuration

See [`config.example.yaml`](config.example.yaml).

```yaml
product: example

index:
  file: "path/to/manifest.json"          # or:
  # url: "https://docs.example.com/manifest.json"
  refresh_interval: 5m
  include_catalog: true                  # false → FQN as schema.table

search:
  max_hits: 20

auth:
  mode: disabled                         # or static (see example)
```

`index.url` / `index.file`: one is required. Classic dbt Docs (Core 1.x) that serves the whole `target/` directory usually exposes `/manifest.json` next to `index.html`.

`get_model` returns `compiled_sql` from the manifest (truncated to 8 KiB), not the raw `.sql` file from git.

## Search ranking

Lexical AND over whitespace-separated tokens. For each token the best field match wins; scores sum across tokens, then a layer bonus is added.

| Match | Score |
| --- | ---: |
| Exact FQN | 100 |
| Exact alias / name | 80 |
| FQN segment | 55 |
| Exact column name | 40 |
| Substring in name / path / column | 25 |
| Description | 12 |
| Exposure label | 10 |
| Tags / column description | 8 |

Layer bonus (models in `default_catalog` outside `external_catalogs/` only):

| Layer heuristic | Bonus |
| --- | ---: |
| marts / pds (schema or path) | +15 |
| dds | +8 |

Tie-break: shorter alias, then FQN lexicographic order.

## Docker Hub

```bash
docker login
make docker-push                 # barloc/dbt-gate:latest
make docker-push VERSION=v0.1.0  # also tags VERSION
```

Image: [`barloc/dbt-gate`](https://hub.docker.com/r/barloc/dbt-gate).

## Make targets

| Target | Action |
| --- | --- |
| `make test` | `go test ./...` |
| `make run` | build + run with `config.example.yaml` |
| `make smoke` | curl health + `/v1/search` |
| `make up` / `make down` | docker compose |
| `make docker-build` / `make docker-push` | local image publish |
