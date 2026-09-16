# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Bounded failure diagnostics for unexpected request errors: a single private request-completion log records the status, route template, duration, request ID, active trace and span IDs, and a redacted cause, and the active span records the exception.
- `X-Request-ID` validation. The API echoes identifiers of up to 128 ASCII letters, digits, dots, underscores, or hyphens, and generates a UUID when the header is missing or invalid.

### Changed

- Creating or updating a pattern now writes the pattern, its chunks and one enrichment job per chunk in a single transaction. Previously `Create` used no transaction at all, so a chunk failure returned 500 with the pattern already stored, and retrying the same name returned 409 with no way forward. A failed write now persists nothing and the name stays free.
- A failed enrichment-job insert now fails the request instead of returning success. Previously the response was 2xx and the affected chunks were never embedded, with no row to find them by.
- Enrichment jobs are published to RabbitMQ only after the transaction commits, so a rolled-back write cannot enqueue work for rows that no longer exist. Publication remains best-effort: a queue failure leaves the job rows pending for recovery.
- Error response `traceId` now identifies the active OpenTelemetry trace rather than the incoming `X-Request-ID`, and is omitted when no valid trace context exists. Request identity and trace identity are now separate values.
- Request-completion logging moved from the `otelx` Gin middleware to an in-repository implementation that carries the failure cause.
- `database.postgres.max_open_conns` and `max_idle_conns` are typed `int32`, matching the pool settings they configure. A value too large is now rejected while loading, naming the key, instead of being silently clamped.

### Removed

- In-process TLS support and `server.tls.*` configuration. TLS terminates at the reverse proxy and the API serves plain HTTP behind it, which is what the Docker health probe already assumed.

### Fixed

- A failed enrichment publish now logs the pattern ID alongside the job ID and cause, so a job left pending can be traced back to the pattern it belongs to.
- Panic recovery now runs inside the observability middleware. A recovered panic produces a 500 completion log, an error span, and request count and duration metrics, and returns the in-flight count to zero; previously those frames unwound before the outer recovery wrote the response.

### Security

- Public 503 responses return a stable `service temporarily unavailable` detail instead of the upstream failure text, which could disclose upstream response bodies to API clients.
- Private diagnostics redact upstream response bodies, URLs, quoted values, and credential or content fields, and exclude request bodies and query strings. Gin's default recovery, which prints raw panic values and request headers to stderr, is no longer used.

## [v0.3.4] - 2026-09-09

### Changed

- Updated the API and E2E Go modules and Docker build images to Go 1.27.1.
- Updated runtime and development dependencies.

### Fixed

- Disabled test analysis in golangci-lint with `run.tests: false`; unit tests remain covered by `go test`.
- Moved graph repository and OpenAI embedding test constructors into `export_test.go` so production lint analysis excludes test-only helpers.
- Retained `pgx` 5.10.0 for compatibility with `pgxmock` 4.9.0, which lacks the `TypeMap` method required by `pgx` 5.11.0.

## [v0.3.3] 2026-08-24

### Removed

- The embedded MCP listener, its API configuration, and the API-local MCP implementation dependency.
- API-owned MCP integration tests and test-stack wiring; `mnemonic-mcp` exclusively serves MCP.

## [v0.3.1] - 2026-08-19

### Added

- Local Docker Compose integration for the standalone enrichment service

### Changed

- Updated the Go toolchain and runtime dependencies
- Hardened the Docker build and runtime image, including non-root execution and image metadata
- Unified Docker-first CI so successful main and develop builds publish the image they tested
- Removed obsolete standalone database-test Make targets in favor of the full E2E build gate
- Cleaned Docker build contexts and removed obsolete generated artifacts

### Fixed

- Aligned local Compose services on shared database and RabbitMQ configuration
- Made CI and E2E image selection deterministic and validated the supported Compose configurations

## [v0.2.1]

### Added

- REST Admin API (port 8080) for pattern CRUD and semantic search operations
- Pattern enrichment pipeline: automatic embedding generation and concept extraction via OpenAI LLM
- PostgreSQL data persistence with PGVector support for vector similarity search
- Neo4j backing store for pattern concept relationships and graph traversal
- API-local MCP serving; MCP protocol serving is owned by `mnemonic-mcp`
- OpenTelemetry observability: distributed tracing, metrics collection, and structured logging
- Gin HTTP server framework with middleware for tracing and request metrics
- Configuration management (`internal/config`): layered loading from defaults, files, and environment variables
- Server integration with configurable timeouts, TLS support, and graceful shutdown
- Telemetry package (`internal/telemetry`) with otelx integration for unified OpenTelemetry setup
- Middleware package (`internal/middleware`) with tracing and request metrics for Gin
- Metrics package (`internal/metrics`) with domain-specific counters and histograms for patterns and database operations
- Distributed tracing support via otelgin middleware with trace ID correlation
- Request metrics: operation counts, duration histograms, and in-flight request counters
- Version package (`internal/version`) for build metadata and release information
- Repository layer (`internal/repository`) with PostgreSQL implementation for:
  - Patterns: CRUD, similarity search, and enrichment status tracking
  - Skills: definition management and versioning
  - Skill files: content persistence and metadata
  - Chunks: text segmentation for pattern processing
  - Enrichment jobs: status tracking and result storage
  - Agents: metadata and configuration
  - Graph: Neo4j pattern relationships and concept linkage
- Database schema migrations for all entity types
- Repository error types: domain-specific errors for conflict, not found, validation, and persistence failures
- List options for pagination support in repository queries
- Service layer (`internal/service`) for business logic:
  - Pattern service: enrichment orchestration, search, and lifecycle management
  - Skill and skill file services for capability management
  - Agent service for user/agent tracking
  - Enrichment service for LLM pipeline coordination
  - Search service for semantic similarity queries
- OpenAI integration service (`internal/service/openai`):
  - Embedding generation using text-embedding-3-large model
  - Concept extraction and entity identification via structured chat completions
  - Token usage tracking and error handling
- Health check endpoint for service readiness and dependency status
- Docker multi-stage build for optimized image size
- E2E test suite via Docker Compose:
  - Tests for all API endpoints (agents, skills, skill files, patterns, enrichment operations)
  - REST API integration tests
  - Database and dependency initialization
- GitHub Actions CI/CD workflows for automated testing and image publication
- Comprehensive unit and integration tests with pgxmock for database isolation
- Makefile targets for building, testing, and documentation generation
- Swagger UI at `/swagger/index.html` with a Swagger 2.0 specification
- Build script with cleanup traps for Docker Compose teardown

### Fixed

- E2E test Docker Compose configuration naming (mnemonic_tests container reference)
- CI config and E2E Docker Compose setup for proper service initialization
- E2E test execution and assertions for all API endpoints

### Changed

- API version path structure: `/v1/api/` prefix for all endpoints
- Server startup now initializes telemetry and observability middleware by default
- Configuration validation includes log level and timeout validation with fail-fast error reporting
- Build and CI/CD workflows optimized for mnemonic-api specific requirements
