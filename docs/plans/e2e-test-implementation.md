# E2E Test Implementation Plan

## Overview

Implement all E2E test stubs in `src/mnemonic/tests/e2e/` against the live Mnemonic API stack. Tests were generated from the OpenAPI spec (`docs/api/openapi/mnemonic-v1.yaml`) and validate API behavior as a black box.

## Approach

Each domain is implemented iteratively:

1. Dispatch the `go e2e test engineer` agent to implement test stubs for the domain
2. Run tests using `src/mnemonic/tests/run-e2e.sh`
3. If tests fail due to bugs in the source code, dispatch the `go software engineer` agent to fix
4. Repeat until all tests in the domain pass
5. Produce a status report: tests passed, problems found, resolutions

## Running Tests

```bash
# Local dev (starts infra, runs tests, tears down)
./src/mnemonic/tests/run-e2e.sh

# CI (builds test image, runs in docker-compose)
./src/mnemonic/build/build.sh
```

The dev script uses `docker-compose-dev.yaml` which runs databases + API but no test runner container. Tests execute directly from the host via `go test`.

### Environment Variables

| Variable      | Default                 | Description                                 |
| ------------- | ----------------------- | ------------------------------------------- |
| `API_URL`     | `http://localhost:8080` | Mnemonic admin API base URL                 |
| `METRICS_URL` | `http://localhost:9090` | Prometheus metrics endpoint (separate port) |

## Domain Order

Simplest to most complex. Each domain builds confidence in the test infrastructure before tackling harder endpoints.

### 1. Operations (DONE)

**Files:** `operations_test.go`
**Tests:** 12 (11 passing, 1 skipped)
**Endpoints:** `GET /health`, `GET /version`, `GET /metrics`

Skipped: `TestHealthCheck_UnhealthyReturns503` requires stopping a database container mid-test.

### 2. Agents (TODO)

**Files:** `agents_test.go`
**Tests:** 27 stubs
**Endpoints:** `GET /v1/api/agents`, `POST /v1/api/agents`, `GET /v1/api/agents/{name}`, `PUT /v1/api/agents/{name}`, `DELETE /v1/api/agents/{name}`

Key scenarios:

- CRUD happy paths
- Pagination with cursor walking
- Limit bounds (min/max validation)
- Duplicate name conflict (409)
- Validation errors (missing required fields, field length limits)
- Full replacement semantics on PUT (omitted fields reset)
- Delete cascade to pattern associations

### 3. Skills (TODO)

**Files:** `skills_test.go`
**Tests:** 26 stubs
**Endpoints:** `GET /v1/api/skills`, `POST /v1/api/skills`, `GET /v1/api/skills/{name}`, `PUT /v1/api/skills/{name}`, `DELETE /v1/api/skills/{name}`

Key scenarios:

- CRUD happy paths
- Pagination, tag filtering
- Duplicate name conflict (409)
- Validation errors
- Name format validation
- Delete cascade to skill files

### 4. Skill Files (TODO)

**Files:** `skillfiles_test.go`
**Tests:** 27 functions x 3 collections (scripts, references, assets) = 77 subtests
**Endpoints:** CRUD for each of `scripts`, `references`, `assets` under `/v1/api/skills/{name}/`

Key scenarios:

- Upload (POST) with base64 encoding
- File size limits (413)
- File count limits per collection (422)
- Duplicate filename conflict (409)
- Skill not found (404)
- Same filename across different collections (allowed)
- Same filename across different skills (allowed)

### 5. Patterns (TODO)

**Files:** `patterns_test.go`
**Tests:** 71 stubs (plus subtests)
**Endpoints:** `GET /v1/api/patterns`, `POST /v1/api/patterns`, `POST /v1/api/patterns/search`, `GET /v1/api/patterns/{id}`, `PUT /v1/api/patterns/{id}`, `DELETE /v1/api/patterns/{id}`, `GET /v1/api/patterns/{id}/agents`, `PUT /v1/api/patterns/{id}/agents`

Key scenarios:

- CRUD happy paths (create returns 202 Accepted, not 201)
- Pagination with cursor walking, tag filtering, full-text search
- Semantic search with query, threshold, tag/agent filters
- Agent associations (get and set)
- Enrichment lifecycle (pending -> enriched, content change re-triggers)
- Validation errors across all endpoints
- Invalid UUID handling

## Test Infrastructure

### Files

| File                      | Purpose                                                           |
| ------------------------- | ----------------------------------------------------------------- |
| `helpers.go`              | `TestClient` with auth headers, HTTP helpers, assertion utilities |
| `types.go`                | Response types matching OpenAPI spec                              |
| `test-runner.sh`          | Entrypoint for Docker-based CI runs                               |
| `Dockerfile`              | Test runner container image                                       |
| `run-e2e.sh`              | Local dev test script                                             |
| `docker-compose-dev.yaml` | Dev stack (databases + API, no test runner)                       |
| `docker-compose.yaml`     | CI stack (databases + API + test runner)                          |

### Test Helpers

- `NewTestClient(t)` — admin client with auth headers
- `NewReadOnlyTestClient(t)` — developer-only client
- `NewUnauthenticatedClient(t)` — no auth headers
- `AssertStatusCode(t, resp, code)` — status code assertion
- `AssertContentType(t, resp, ct)` — content-type assertion
- `ParseJSON[T](t, resp)` — generic JSON response parser
- `ReadBody(t, resp)` — raw body reader
- `GenerateUniqueName(prefix)` — unique resource name generator

### Notes

- Tests create their own test data; no shared fixtures
- Each test should be independent and idempotent
- Use `GenerateUniqueName` to avoid collisions between test runs
- Pattern create is async (202) — enrichment runs in background with a dummy OpenAI key, so enrichment will fail; tests should account for this
- Auth is not enforced in MVP 1; auth-related tests are excluded
