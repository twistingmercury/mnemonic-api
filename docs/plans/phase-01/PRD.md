# Product Requirements Document: MVP 2 Part 1 — Extract REST API

*Gralph processes cycles in this document from top to bottom. Checklist markers are significant: `- [ ]` (open), `- [x]` (complete), `- [~]` (abandoned). Each cycle must be small, independently verifiable, and assigned to exactly one agent.*

## Objective

Extract the Admin REST API from the monolithic `mnemonic` binary into a standalone `mnemonic-api`
binary and Docker image. The MCP server and enrichment worker remain in the existing `mnemonic`
binary. Each binary gets its own Docker image and service in docker-compose.

## Problem Statement

All three runtime components (Admin REST API, MCP server, enrichment worker) run in one process.
This prevents independent scaling, deployment, and restart of each component — a prerequisite
for the MVP 2 architecture where the enricher moves to RabbitMQ-based processing in Part 2.

## Success Criteria

- `go build ./cmd/api/...` and `go build ./cmd/main/...` both compile without error.
- `go test ./...` passes with no regressions.
- `docker compose up` brings up `dev_api` (REST, port 8080) and `dev_mcp` (MCP+enricher, port 8081) as separate healthy containers.
- `curl -f localhost:8080/health` returns 200.
- `curl -f localhost:8081/health` returns 200.
- CI workflow builds both images, runs unit tests, runs E2E tests, and pushes both images to GHCR on push to `develop` or `main`.

## Scope

### In scope

- Refactor `internal/server/server.go`: add `ListenAndServeAPI`, `wireAPIDependencies`, rename existing `ListenAndServe` → `ListenAndServeMCPEnricher`.
- Add `/health` route to the MCP HTTP server so the `dev_mcp` container has a health probe.
- New `cmd/api/main.go` and `cmd/api/main_test.go` for the REST API binary.
- Update `cmd/main/main.go` to call `ListenAndServeMCPEnricher`.
- Modify `build/Dockerfile` to build both binaries with separate final stages.
- Update `build/build.sh` to build and tag both images.
- Update `docker-compose.yaml` to split `dev_api` into `dev_api` (REST) and `dev_mcp` (MCP+enricher).

### Out of scope

- Extracting the enricher into its own binary (Part 2).
- Introducing RabbitMQ (Part 2).
- Authentication or multi-user support.
- E2E test updates (tests run against port 8080 and 8081 which are unchanged externally).

## Constraints and Decisions

- Go 1.26.1, module path `github.com/twistingmercury/mnemonic-api`.
- New REST API image name: `ghcr.io/twistingmercury/mnemonic-api`, tag `latest-dev`.
- Existing MCP+enricher image name stays `ghcr.io/twistingmercury/mnemonic`, tag `latest-dev`.
- Single `Dockerfile` with two final stages (`mnemonic` and `mnemonic-api`), built with `--target`.
- MCP+enricher health probe: `/health` endpoint added to the MCP HTTP server mux (not a new port).
- `wireAPIDependencies` does NOT wire extractionSvc or enrichWorker; it DOES wire embeddingSvc (needed by searchSvc for query embedding).
- Metrics port for `dev_mcp` container: expose on `9091:9090` to avoid collision with `dev_api`.

## Implementation Plan

- [ ] **Cycle 1 - Server refactoring**: Add `ListenAndServeAPI` + `wireAPIDependencies` to `server.go`, rename `ListenAndServe` → `ListenAndServeMCPEnricher`, and add `/health` route to the MCP HTTP server.
  - Agent: `go software engineer`
  - Files: `src/mnemonic/internal/server/server.go`, `src/mnemonic/internal/mcpserver/server.go`
  - Steps:
    - Add `wireAPIDependencies(pgPool, neo4jDriver, cfg, logger) (Services, error)` — wires all repos + embeddingSvc + agentSvc/skillSvc/skillFileSvc/searchSvc/patternSvc; no extractionSvc or enrichWorker.
    - Add `ListenAndServeAPI(cfg)` — initializes telemetry, opens DBs, wires API deps, builds Gin router, runs single `runHTTPServer` goroutine; no MCP, no enricher.
    - Rename `ListenAndServe` → `ListenAndServeMCPEnricher`; remove the `runHTTPServer` goroutine for the REST API from it.
    - In `mcpserver/server.go` (or wherever the MCP HTTP server is assembled), register a `/health` handler backed by the existing `health` package HTTP handler.
  - Verify: `cd src/mnemonic && go build ./internal/... && go test ./internal/server/... ./internal/mcpserver/...`
  - Done: Both `go build` and `go test` exit 0 with no errors.

- [ ] **Cycle 2 - REST API entrypoint**: Create the `cmd/api` binary (main.go + main_test.go) and update `cmd/main/main.go` to call the renamed function.
  - Agent: `go software engineer`
  - Files: `src/mnemonic/cmd/api/main.go`, `src/mnemonic/cmd/api/main_test.go`, `src/mnemonic/cmd/main/main.go`
  - Steps:
    - Create `cmd/api/main.go` mirroring `cmd/main/main.go` structure: same `--version`/`--health` flags, swagger annotations for port 8080, calls `server.ListenAndServeAPI(cfg)`.
    - Create `cmd/api/main_test.go` mirroring `cmd/main/main_test.go`.
    - In `cmd/main/main.go`, change the `server.ListenAndServe(cfg)` call to `server.ListenAndServeMCPEnricher(cfg)`.
    - In `cmd/main/main.go`, update `checkHealth()` to probe the MCP health endpoint: read `mcp.port` (not `server.port`) so `--health` checks port 8081.
  - Verify: `cd src/mnemonic && go build ./cmd/api/... && go build ./cmd/main/... && go test ./cmd/...`
  - Done: Both binaries compile and `go test ./cmd/...` exits 0.

- [ ] **Cycle 3 - Full test suite**: Confirm no regressions across the entire module after the server refactor and new entrypoint.
  - Agent: `go software engineer`
  - Files: any files requiring minor fixes to restore passing tests
  - Steps:
    - Run `go test ./...` from `src/mnemonic`.
    - Fix any compilation or test failures caused by the renamed function or new wiring.
  - Verify: `cd src/mnemonic && go test ./...`
  - Done: `go test ./...` exits 0 with all packages passing.

- [ ] **Cycle 4 - Dockerfile split**: Modify `build/Dockerfile` to build both binaries and produce two final stages.
  - Agent: `devops engineer`
  - Files: `src/mnemonic/build/Dockerfile`
  - Steps:
    - In the build stage, add a second `go build` command: `go build -o /out/mnemonic-api ./cmd/api/...` alongside the existing `go build -o /out/mnemonic ./cmd/main/...`.
    - Rename the existing final stage to `AS mnemonic`; update COPY to `/out/mnemonic`.
    - Add a new final stage `AS mnemonic-api` (FROM scratch); COPY `/out/mnemonic-api`, EXPOSE 8080, ENTRYPOINT `["/mnemonic-api"]`.
  - Verify: `docker build --target mnemonic -t mnemonic-test -f src/mnemonic/build/Dockerfile src/mnemonic && docker build --target mnemonic-api -t mnemonic-api-test -f src/mnemonic/build/Dockerfile src/mnemonic`
  - Done: Both `docker build` commands exit 0 and produce runnable images.

- [ ] **Cycle 5 - Build script**: Update `build/build.sh` to build and tag both Docker images.
  - Agent: `shell script engineer`
  - Files: `src/mnemonic/build/build.sh`
  - Steps:
    - Add a `build_api_image()` function that calls `docker build --target mnemonic-api` with tag `ghcr.io/twistingmercury/mnemonic-api:{version}` and `latest-dev`.
    - Rename or update the existing image build function to use `--target mnemonic` for the `mnemonic` image.
    - Call both build functions from the main build entrypoint.
  - Verify: `shellcheck src/mnemonic/build/build.sh && bash -n src/mnemonic/build/build.sh`
  - Done: `shellcheck` and `bash -n` both exit 0.

- [ ] **Cycle 6 - Docker Compose split**: Split the `dev_api` service in `docker-compose.yaml` into `dev_api` (REST API) and `dev_mcp` (MCP + enricher).
  - Agent: `devops engineer`
  - Files: `docker-compose.yaml`
  - Steps:
    - Rename existing `dev_api` → `dev_mcp`; update image to `ghcr.io/twistingmercury/mnemonic:latest-dev`; remove port 8080 mapping; keep 8081; change metrics port mapping to `9091:9090`; update healthcheck to `CMD /mnemonic --health` (probes MCP health on 8081).
    - Add new `dev_api` service: image `ghcr.io/twistingmercury/mnemonic-api:latest-dev`; ports `8080:8080`, `9090:9090`; healthcheck `CMD /mnemonic-api --health`; same DB/OpenAI env vars; depends_on migrate + dev_neo4j + dev_postgres.
  - Verify: `docker compose config --quiet`
  - Done: `docker compose config` exits 0 (valid YAML, all service references resolve).

- [ ] **Cycle 7 - CI workflow update**: Update `.github/workflows/mnemonic-ci.yaml` to run unit tests, run E2E tests, build both images (via build.sh), and push both `mnemonic` and `mnemonic-api` images to GHCR.
  - Agent: `devops engineer`
  - Files: `.github/workflows/mnemonic-ci.yaml`
  - Steps:
    - Add `actions/setup-go@v5` step (go-version: `1.26.1`) before the build script step; set `cache: true` and `cache-dependency-path: src/mnemonic/go.sum`.
    - Add a "Run unit tests" step after Go setup and before the build script: `cd src/mnemonic && go test ./...`.
    - The existing "Run build script" step calls `./build/build.sh`, which after Cycle 5 builds both images tagged `latest-dev` locally — no change needed to this step.
    - Add an "Run E2E tests" step after the build script and before GHCR login: run `src/mnemonic/tests/run-e2e.sh`; set `working-directory: .` (the script manages docker compose lifecycle itself). Add `OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}` to its `env` block.
    - In the "Push image tags" step, add push commands for `ghcr.io/twistingmercury/mnemonic-api` mirroring the existing `mnemonic` push logic: on `main` push `latest` and `${VERSION}`; on other branches push `latest-dev` and `${VERSION}-dev`.
  - Verify: `python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/mnemonic-ci.yaml')); print('YAML valid')" && actionlint .github/workflows/mnemonic-ci.yaml`
  - Done: `python3` YAML parse and `actionlint` both exit 0 with no errors.

## Risks and Mitigations

- Risk: `searchSvc` (used by both REST API and MCP server) requires `embeddingSvc`; forgetting to include it in `wireAPIDependencies` causes panics.
  - Mitigation: `wireAPIDependencies` explicitly includes `embeddingSvc`; Cycle 1 verify includes a build check.
- Risk: The `--health` flag in `cmd/main` probes the REST API port by default; after split it must probe the MCP health endpoint.
  - Mitigation: Cycle 2 explicitly updates `checkHealth()` to read `mcp.port`.
- Risk: Cycle Done condition is too vague, causing gralph to loop.
  - Mitigation: Every Verify is a concrete shell command exiting 0.

## Definition of Done

- `cd src/mnemonic && go test ./...` exits 0.
- `docker compose config --quiet` exits 0.
- `docker compose up -d && sleep 15 && curl -sf localhost:8080/health && curl -sf localhost:8081/health && docker compose down` exits 0.
- `actionlint .github/workflows/mnemonic-ci.yaml` exits 0.
