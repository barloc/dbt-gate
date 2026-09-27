# Changelog

## [1.0.2] — 2026-09-27

### Added

- Example Cursor skill: `examples/skills/dbt-gate/SKILL.md`
- `AGENTS.md` for IDE agents

## [1.0.1] — 2026-09-27

### Fixed

- Docker image includes the example fixture so bare `docker run` can serve `/v1/search`
- `config.example.yaml` documents `auth.mode: static`
- Fixture/tests: dbt project id `space` → `acme`; source prefix `hdp_` → `ext_`

## [1.0.0] — 2026-09-27

First public release.

### Added

- Read-only dbt catalog gate over a standard `manifest.json` (HTTP file or URL)
- HTTP API on `:8080`: `POST /v1/search`, `/v1/get_model`, `/v1/lineage`, `/v1/get_exposure`
- Streamable HTTP MCP on `/mcp` with the same four tools
- System endpoints on `:8090`: `/health`, `/metrics` (index freshness)
- `index.include_catalog` (default `true`): FQN as `catalog.schema.table` or `schema.table`
- Model cards include truncated `compiled_sql` from the manifest
- Lexical search ranking with field weights and marts/dds layer bonus (documented in README)
- Local workflow: `make test`, `make run`, `make smoke`, `docker compose`
- Multi-stage Docker image (`barloc/dbt-gate`), Apache-2.0 license

### Notes

- Built around a dbt-StarRocks-style layout; usable with any dbt adapter via `manifest.json`
- No GitHub Actions CI — verify and publish images with Make
- Not supported: dbt Docs v2 Parquet UI, embedded `static_index.html`, dbt Cloud without a downloadable manifest
