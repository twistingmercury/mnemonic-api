# mnemonic-api

> **Maturity Level**: Emerging - Admin REST API under active development
> **Version**: v0.2.0

---

## Table of Contents

- [Usage](#usage)
- [How it works](#how-it-works)
- [Key Considerations](#key-considerations)
- [Development Considerations](#development-considerations)
- [Versioning](#versioning)

## Usage

Mnemonic-api is a REST Admin API for managing the Mnemonic knowledge graph — primarily patterns.

```bash
# Store a new pattern
curl -X POST http://localhost:8080/v1/api/patterns \
  -H "Content-Type: application/json" \
  -d '{
    "name": "go-error-wrapping",
    "description": "Pattern for wrapping errors with context",
    "content": "Use fmt.Errorf with %w for error chains...",
    "tags": ["go", "error-handling"]
  }'

# Search patterns semantically
curl -X GET "http://localhost:8080/v1/api/patterns/search?q=error+handling&limit=5"
```

See the [API Specification](docs/openapi/mnemonic-v1.yaml) for complete endpoint documentation, or browse the Swagger UI at `http://localhost:8080/swagger/index.html` when the service is running.

## How it works

Mnemonic-api is a Go service that exposes a REST Admin API (port 8080) backed by PostgreSQL, PGVector, and Neo4j.

- **Patterns** are stored with vector embeddings (PGVector) for semantic search and concept relationships (Neo4j) for graph traversal.
- **Pattern enrichment** automatically extracts embeddings and concepts via an LLM pipeline. See [Pattern Processing](docs/design/pattern-processing.md).

## Key Considerations

- **Scope**: REST Admin API only — no MCP server, no routing engine.
- **MVP scope**: Local deployment via Docker Compose, single-user trusted environment, no authentication.
- **Swagger UI**: Available at `http://localhost:8080/swagger/index.html` while the service is running.
- **Post-MVP**: Multi-user authentication, rate limiting, and remote access are out of scope for Phase 1.

## Development Considerations

### Quick Start

Clone and build:

```bash
git clone https://github.com/twistingmercury/mnemonic-api.git
cd mnemonic-api/src
./build/build.sh
```

This builds the Docker image (`ghcr.io/twistingmercury/mnemonic-api`) and runs E2E tests via Docker Compose. Requires Go 1.26+, Docker 27+, and Docker Compose 2.32+.

### Configuration

Mnemonic-api uses layered configuration:

1. **Built-in defaults** — safe defaults for all settings
2. **Config file** — `config.yaml` searched in `/etc/mnemonic/` or the current directory
3. **Environment variables** — `MNEMONIC_` prefix overrides any setting (e.g. `MNEMONIC_SERVER_PORT=9090`)

See the [Configuration Reference](docs/design/configuration.md) for all available settings.

### Swagger docs

Regenerate Swagger docs locally before committing (run from `src/`):

```bash
make docs-swagger
```

### Testing

Unit tests (run from `src/`):

```bash
go test ./...
```

E2E tests (requires Docker, run from `src/`):

```bash
./build/build.sh
```

### Versioning

This project follows [Semantic Versioning 2.0.0](https://semver.org/).

Version is determined from git tags:

```bash
git describe --tags --always
```

No releases published yet. See [CHANGELOG.md](CHANGELOG.md) for development progress.
