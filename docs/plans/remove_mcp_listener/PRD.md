# Product Requirements Document: Remove the Embedded MCP Listener from mnemonic-api

*Gralph processes cycles in this document from top to bottom. Checklist markers are significant: `- [ ]` (open), `- [x]` (complete), `- [~]` (abandoned). Each cycle must be small, independently verifiable, and assigned to exactly one agent.*

## Objective

Make `mnemonic-api` an Admin REST API only. It must not construct, configure, listen for, or expose Model Context Protocol endpoints; `mnemonic-mcp` remains the sole MCP service. Preserve the API's operational endpoints and REST semantic search.

## Problem Statement

`mnemonic-api` currently starts an MCP HTTP server on its configured MCP port in addition to its Admin API listener. This duplicates `mnemonic-mcp`, leaves two independently deployable MCP implementations, and makes configuration, test, and deployment ownership ambiguous.

## Success Criteria

- `mnemonic-api` starts only its Admin API/operations HTTP listener and has no `/mcp` handler or MCP-port listener.
- The API has no MCP configuration, SDK dependency, internal MCP-server package, or API-owned MCP test suite.
- REST pattern CRUD/search, health, version, Swagger, metrics, and RabbitMQ publishing remain functional.
- MCP end-to-end coverage is owned by `mnemonic-mcp` or `mnemonic-integration-tests`, never by `mnemonic-api`.
- API and source-of-truth documentation identify `mnemonic-mcp` as the sole MCP owner.

## Scope

### In scope

- Remove the embedded MCP runtime and all MCP-only code/configuration from `mnemonic-api`.
- Remove or relocate API-owned MCP E2E tests and compose wiring.
- Update affected API packaging/readme text and the Mnemonic source-of-truth topology documentation.
- Verify the API's remaining REST and operational behavior.

### Out of scope

- Changes to MCP tool behavior, schemas, or transport in `mnemonic-mcp`.
- Changes to the Enricher's expected health/version/metrics endpoints.
- Broader remediation from the cross-repository SoT audit.
- Adding authentication or changing REST API semantics.

## Constraints and Decisions

- `mnemonic-mcp` is the sole owner of Streamable HTTP MCP and port 8081.
- `mnemonic-api` continues to own REST API routes, `/health`, `/version`, Swagger, metrics, and enrichment-job publishing.
- REST semantic search continues to use `SearchService`; do not remove its OpenAI, PostgreSQL/PGVector, or Neo4j dependencies merely because the embedded MCP server is removed.
- Use `go_software_engineer` for API implementation, `go_e2e_test_engineer` for E2E ownership, and `technical_writer` for documentation. These identifiers are exact registered agent names.
- Keep source-repository and `mnemonic-docs` commits separate; each commit must be made from its owning repository.

## Implementation Plan

- [x] **Cycle 1 - Remove API MCP E2E ownership**: Remove the duplicate API-owned MCP E2E suite and its compose wiring before removing the listener; `mnemonic-mcp` remains the existing MCP E2E owner.
  - Agent: `go_e2e_test_engineer`
  - Files: `src/tests/docker-compose.yaml`, `src/tests/run-e2e.sh`, `src/tests/e2e/mcp/`, `src/tests/e2e/helpers/helpers.go`, `src/tests/e2e/helpers/types.go`
  - Steps:
    - Remove `MNEMONIC_MCP_PORT`, `MCP_URL`, MCP test package, and MCP-only test helpers from the API E2E suite.
    - Confirm the existing MCP E2E suite in `mnemonic-mcp` remains the canonical owner of MCP protocol coverage.
  - Verify: `cd mnemonic-api && make build && cd ../mnemonic-mcp && make build`
  - Done: Both full builds pass; API E2E has no MCP URL/configuration or MCP test package.

- [x] **Cycle 2 - Stop API MCP serving**: Remove MCP construction and listener startup from the API server while retaining the Admin API lifecycle.
  - Agent: `go_software_engineer`
  - Files: `src/internal/server/server.go`, `src/internal/server/server_test.go`
  - Steps:
    - Remove MCP-server construction and the MCP errgroup task from `ListenAndServe`.
    - Remove `runMCPServer`, its shutdown constant, and MCP-only server logging.
    - Simplify dependency wiring to return only REST services and the queue publisher.
    - Add or update a server-level test proving the API does not construct an MCP listener.
  - Verify: `cd mnemonic-api && make build`
  - Done: The full build passes and `server.go` contains no MCP HTTP-server construction or startup path.

- [ ] **Cycle 3 - Remove API MCP configuration**: Delete MCP configuration state and validation so API startup cannot accept or use MCP listener settings.
  - Agent: `go_software_engineer`
  - Files: `src/internal/config/config.go`, `src/internal/config/defaults.go`, `src/internal/config/config_test.go`
  - Steps:
    - Remove `MCPConfig` and the top-level MCP field from the API configuration model.
    - Remove MCP defaults, environment binding, validation, address helpers, and port-conflict validation.
    - Update configuration tests to assert the remaining API configuration contract.
  - Verify: `cd mnemonic-api && make build`
  - Done: The full build passes and `rg -n 'MCPConfig|mcp\\.' mnemonic-api/src/internal/config` returns no matches.

- [x] **Cycle 4 - Remove MCP implementation dependency**: Remove the API-local MCP package, its tests, and its direct Go SDK dependency.
  - Agent: `go_software_engineer`
  - Files: `src/internal/mcpserver/`, `src/go.mod`, `src/go.sum`, `src/internal/service/doc.go`, `src/internal/service/search/service.go`
  - Steps:
    - Delete the API-local `internal/mcpserver` package and its unit tests.
    - Remove the Model Context Protocol SDK from the API module and tidy module metadata.
    - Revise API-local service comments so they describe REST responsibilities without claiming MCP ownership.
  - Verify: `cd mnemonic-api && make build`
  - Done: The full build passes, `go.mod` has no Model Context Protocol SDK dependency, and no API production package imports `internal/mcpserver`.

- [x] **Cycle 5 - Align deployment and documentation**: Remove API MCP claims from deployment metadata and record `mnemonic-mcp` as the sole owner in source-of-truth documentation.
  - Agent: `technical_writer`
  - Files: `mnemonic-api/README.md`, `mnemonic-api/src/build/Dockerfile`, `mnemonic-api/CHANGELOG.md`, `mnemonic-docs/README.md`, `mnemonic-docs/docs/architecture/system/02-system-architecture.md`, `mnemonic-docs/docs/code_reviews/cross_repository_sot_audit_v01.md`
  - Steps:
    - Remove API port-8081, MCP endpoint, and MCP-tool claims from README, image metadata, and current changelog language where it describes present behavior.
    - State that `mnemonic-mcp` exclusively serves MCP while API operations endpoints remain expected.
    - Mark the audit's duplicate-MCP finding reconciled with implementation evidence after the code change is complete.
  - Verify: `cd mnemonic-api && make build && git -C mnemonic-api diff --check && git -C mnemonic-docs diff --check`
  - Done: The API full build and both whitespace checks pass; no API documentation claims that it serves MCP.

- [x] **Cycle 6 - Prove the API-only network boundary**: Run the final API verification and confirm no API MCP listener remains.
  - Agent: `go_e2e_test_engineer`
  - Files: `mnemonic-api/src/tests/docker-compose.yaml`, `mnemonic-api/src/tests/run-e2e.sh`, `mnemonic-api/src/tests/e2e/api/`
  - Steps:
    - Run the supported API unit and Docker E2E paths.
    - Start the API test stack and verify health, version, REST API, Swagger, and metrics are available from the API container.
    - Add or update a black-box assertion that port 8081 and `/mcp` are unavailable from `mnemonic_api`.
  - Verify: `cd mnemonic-api && make build`
  - Done: The full build passes and E2E evidence shows the API exposes only its approved REST/operational surface.

## Risks and Mitigations

- Risk: Removing MCP wiring accidentally removes shared semantic-search dependencies used by REST search.
  - Mitigation: Preserve `SearchService` and validate REST search in Cycle 6.
- Risk: Deleting API E2E tests reduces MCP coverage temporarily.
  - Mitigation: Cycle 4 requires a designated new owner and a passing MCP suite before completion.
- Risk: Shared working tree changes cross repository boundaries.
  - Mitigation: Inspect status before every cycle and commit each repository independently.
- Risk: A Docker test stack retains an old API image that still listens on 8081.
  - Mitigation: Build/tag the tested API image in the supported test path and assert the listener boundary in Cycle 6.

## Definition of Done

- Every completed cycle's `make build` verification exits 0.
- The API E2E component of `make build` passes without `MCP_URL` or `MNEMONIC_MCP_PORT` configured for `mnemonic_api`.
- The MCP E2E suite passes against `mnemonic-mcp`.
- `rg -n 'internal/mcpserver|MCPConfig|MNEMONIC_MCP|NewMCPHTTPServer|runMCPServer' mnemonic-api/src` returns no production-code matches.
- The API and source-of-truth docs name `mnemonic-mcp` as the only MCP server.
