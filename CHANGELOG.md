# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
