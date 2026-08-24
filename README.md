# mnemonic-api

> **Maturity Level**: Emerging - interfaces and local workflows are still evolving
> **Version**: v0.3.3

---

## Table of Contents

- [Usage](#usage)
- [How it works](#how-it-works)
- [Key Considerations](#key-considerations)
- [Development Considerations](#development-considerations)
  - [Quick Start](#quick-start)
  - [Building & running](#building--running)
  - [Configuration](#configuration)
  - [Testing](#testing)
  - [API documentation](#api-documentation)
  - [Versioning](#versioning)

## Usage

Mnemonic-api manages reusable knowledge-graph patterns through an Admin REST API. It does not expose an MCP endpoint or listener; the separate `mnemonic-mcp` service exclusively serves the read-only Model Context Protocol (MCP) interface.

With the service running directly on its default ports, create and search patterns through the REST API:

```bash
curl --request POST http://localhost:8080/v1/api/patterns \
  --header "Content-Type: application/json" \
  --data '{
    "name": "go-error-wrapping",
    "description": "Pattern for wrapping errors with context",
    "content": "Use fmt.Errorf with %w to preserve the error chain.",
    "tags": ["go", "error-handling"],
    "entity_type": "go-pattern",
    "language": "go",
    "domain": "backend"
  }'

curl --get http://localhost:8080/v1/api/patterns/search \
  --data-urlencode "q=error handling" \
  --data-urlencode "limit=5"
```

The generated [Swagger 2.0 specification](src/docs/swagger/swagger.yaml) documents the complete REST API. Swagger UI is available at `http://localhost:8080/swagger/index.html`.

## How it works

The Go process runs only the REST API on port 8080. It connects to PostgreSQL with PGVector for pattern and chunk storage, Neo4j for graph relationships, RabbitMQ for enrichment jobs, and OpenAI for query embeddings.

- Pattern mutations are handled by the REST API under `/v1/api/patterns`.
- Semantic searches embed the query and rank enriched pattern chunks by vector similarity.
- Health is exposed at `/health`; Prometheus metrics use a separate listener on port 9090 by default.

## Key Considerations

- The current API deployment model assumes a trusted environment and does not authenticate REST requests.
- Pattern creation is asynchronous and returns `202 Accepted`; semantic results become available after enrichment completes.
- PostgreSQL, Neo4j, RabbitMQ, and an OpenAI API key are required for a working runtime.
- Direct execution uses ports 8080 and 9090. The root Docker Compose stack publishes the Admin API on port 3000; the separate `dev_mcp` service owns its MCP listener.
- Configuration and API contracts may change while the project remains at the Emerging maturity level.

## Development Considerations

### Quick Start

Prerequisites are Go 1.26.6, Docker, Docker Compose v2, and Git.

```bash
git clone https://github.com/twistingmercury/mnemonic-api.git
cd mnemonic-api
export MNEMONIC_OPENAI_API_KEY="your-api-key"
make build
```

`make build` builds `ghcr.io/twistingmercury/mnemonic-api`, runs its unit tests in the Docker build, and executes the Docker Compose E2E suite.

### Building & running

Run all commands from the repository root:

```bash
make help
make build
make start
```

`make start` launches the root [Docker Compose stack](docker-compose.yaml) and publishes mnemonic-api at `http://localhost:3000`. The `dev_api` service requires the locally built mnemonic-api image (`pull_policy: never`); `dev_mcp` and `dev_enricher` use normal registry behavior. `make stop` removes the Compose volumes, deletes the local `migrate/migrate:latest` image, and prunes unused Docker data.

### Configuration

Configuration precedence is built-in defaults, an optional YAML file, then `MNEMONIC_` environment variables. Set `MNEMONIC_CONFIG_FILE` to choose a file explicitly; otherwise the service checks `/etc/mnemonic/config.yaml` and `./config.yaml`.

Nested keys use underscores in environment variables. For example, `server.port` becomes `MNEMONIC_SERVER_PORT`, and the OpenAI credential is `MNEMONIC_OPENAI_API_KEY`. This API has no `mcp.*` configuration; MCP settings belong to `mnemonic-mcp`.

### Testing

The root Makefile provides the supported test entry points:

```bash
make tests-unit       # Unit tests with coverage
make tests-bench      # Internal package benchmarks
make build            # Image build plus the full E2E suite
```

The E2E suite uses the pre-migrated PostgreSQL and Neo4j images and requires Docker.

### API documentation

Regenerate the tracked Swagger files after changing routes or schemas:

```bash
make docs-swagger
```

### Versioning

This project follows [Semantic Versioning 2.0.0](https://semver.org/). Build metadata derives the version from the latest reachable Git tag:

```bash
git describe --tags --abbrev=0
```

See [CHANGELOG.md](CHANGELOG.md) for development history.
