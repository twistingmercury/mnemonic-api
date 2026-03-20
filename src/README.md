# Mnemonic

> **Maturity Level**: Emerging - MVP foundation in progress

Mnemonic is the backend server for ACE (Agent Coordination Engine), providing deterministic routing and dynamic pattern retrieval via REST API.

## Overview

Mnemonic is a stateless Go service that manages:

- Agent definitions and system prompts (via AgentRepository with PostgreSQL storage)
- Routing rules for prompt-to-agent matching
- Pattern storage with semantic search (via PGVector and Neo4j)
- Background enrichment of patterns with LLM-extracted metadata

See the [System Architecture documentation](/docs/architecture/03-system-architecture.md#mnemonic) for detailed design.

### Current Implementation Status

Phase 5 of the MVP implementation is complete:

- **Database Layer**: PostgreSQL schema for agents with migrations
- **Repository Layer**: AgentRepository interface with pgx implementation and comprehensive error handling
- **Testing**: Full unit test coverage for agent repository operations

## Configuration

Mnemonic uses layered configuration loading:

1. **Built-in defaults** - Safe defaults for all settings
2. **Configuration file** - YAML file searched in:
   - `/etc/mnemonic/config.yaml`
   - `./config.yaml` (current directory)
3. **Environment variables** - Override any setting with `MNEMONIC_` prefix

Example environment variable: `MNEMONIC_SERVER_PORT=9090`

See the [Configuration Reference](/docs/design/mnemonic_service/configuration.md) for complete details on all available settings.

## Building Locally

To build Mnemonic locally:

```bash
cd src/mnemonic
./build/build.sh
```

The build script runs the same steps locally and in CI:

- Builds the Docker image with embedded build metadata
- Runs end-to-end tests against Postgres and Neo4j via Docker Compose

## API Documentation

Mnemonic exposes an interactive Swagger 2.0 UI from the running service.

**Regenerate docs locally:**

```bash
cd src/mnemonic
make docs-swagger
```

Once the service is running, the Swagger UI is available at `http://localhost:8080/swagger/index.html` by default. Override the port with `MNEMONIC_SERVER_PORT`.

> **Contributors:** Run `make docs-swagger` locally to preview docs before committing. Generated files are not committed to the repository.

## CI/CD

Mnemonic has automated CI/CD workflows:

- **CI** (`/.github/workflows/mnemonic-ci.yaml`) - Builds, tests, creates artifact
- **CD** (`/.github/workflows/mnemonic-cd.yaml`) - Pushes Docker image to registry

Manual builds trigger the entire test suite and verify all integration points.

## Documentation

- [API Specification](/docs/design/mnemonic_service/api-specification.md)
- [Pattern Processing](/docs/design/mnemonic_service/pattern-processing.md)
- [Routing Engine](/docs/design/mnemonic_service/routing-engine.md)
- [Configuration Reference](/docs/design/mnemonic_service/configuration.md)
