# Product Requirements Document: Phase 2 — Queue Publishing

*Gralph processes cycles in this document from top to bottom. Checklist markers are significant: `- [ ]` (open), `- [x]` (complete), `- [~]` (abandoned). Each cycle must be small, independently verifiable, and assigned to exactly one agent.*

## Objective

Replace `mnemonic-api`'s internal polling enrichment worker with RabbitMQ queue publishing. After a pattern is created or updated, the API writes an `enrichmentjob` row to PostgreSQL and publishes `{"job_id": "<uuid>"}` to the `enrichment-jobs` queue. The standalone `mnemonic-enricher` service picks up that message and processes it — the API no longer runs enrichment inline. All dead code from the removed worker is deleted from the repository.

## Problem Statement

`mnemonic-api` currently embeds a polling enrichment worker that claims jobs from PostgreSQL and processes them inline (OpenAI embeddings, Neo4j graph sync). This duplicates all the logic in `mnemonic-enricher`, forces the API process to hold OpenAI and graph credentials it does not need, and creates a race condition when both services run simultaneously. Offloading to the queue-based enricher eliminates the duplication and cleanly separates concerns.

## Success Criteria

- `go build ./...` and `go test ./...` pass from `src/` with no regressions.
- `make analyze` passes with no errors.
- `POST /v1/api/patterns` creates a pattern, writes a row to `enrichmentjob` with status `pending`, and publishes `{"job_id": "<uuid>"}` to the `enrichment-jobs` RabbitMQ queue.
- The internal polling enrichment worker goroutine, the enrichment service package, the enricher worker package, and the OpenAI extraction service are deleted from the repository.
- E2E tests do not poll for enrichment completion (enrichment is fully async; no enricher runs during E2E).
- `make build` exits 0.

## Scope

### In scope

- `QueueConfig` / `RabbitMQConfig` types added to `internal/config`.
- Queue viper defaults registered in `SetDefaults`.
- `internal/queue/queue.go` — `Publisher` interface.
- `internal/queue/rabbitmq/publisher.go` — RabbitMQ publisher with reconnect.
- `github.com/rabbitmq/amqp091-go` added to `go.mod`.
- `patternService` updated to accept and use a `queue.Publisher`.
- `server.go` wired with publisher, enrichment worker goroutine removed.
- Dead code deleted: `internal/enricher/`, `internal/service/enrichment/`, `internal/service/openai/extraction.go` and its tests.
- E2E patterns tests updated to accept `pending` enrichment status without polling.
- RabbitMQ added to `docker-compose.yaml` and `src/tests/docker-compose.yaml`.

### Out of scope

- Publisher unit tests requiring an AMQP mock; integration coverage comes from `make build`.
- Authentication or multi-user support.
- CI workflow changes.

## Constraints and Decisions

- Go 1.26+, module root `src/`, module path `github.com/twistingmercury/mnemonic-api`.
- AMQP library: `github.com/rabbitmq/amqp091-go` (already used in `mnemonic-enricher`).
- Message format: `{"job_id": "<uuid>"}` — matches `mnemonic-enricher`'s `jobMessage` struct.
- Queue defaults: provider `rabbitmq`, queue `enrichment-jobs`, host `localhost`, port `5672`, vhost `/`, user `guest`, reconnect delay `5s` — identical to enricher defaults.
- Publishing is best-effort: if `Publish` fails, log a warning and do not fail the HTTP request. The job row persists in PostgreSQL; a re-PUT of the pattern will create new jobs and re-publish.
- `NewPublisher` dials RabbitMQ on construction and returns an error if unreachable; the server exits with a clear message rather than starting in a broken state.
- RabbitMQ Docker image: `rabbitmq:3-management-alpine`; healthcheck via `rabbitmq-diagnostics ping`.
- Env var prefix: `MNEMONIC_` — viper maps `MNEMONIC_QUEUE_RABBITMQ_HOST` → `queue.rabbitmq.host`.
- `make analyze` runs `goimports`, `golangci-lint run`, `govulncheck`, and `gosec` — must pass after any Go source change.

## Implementation Plan

- [x] **Cycle 1 - Queue config types and defaults**: Add `QueueConfig` and `RabbitMQConfig` types to `config.go`, add queue constants to `defaults.go`, and register viper defaults in `SetDefaults`.
  - Agent: `go software engineer`
  - Files: `src/internal/config/config.go`, `src/internal/config/defaults.go`
  - Steps:
    - In `config.go`, add `Queue QueueConfig \`mapstructure:"queue"\`` field to `MnemonicConfig`.
    - In `config.go`, add `QueueConfig` struct with `Provider string \`mapstructure:"provider"\`` and `RabbitMQ RabbitMQConfig \`mapstructure:"rabbitmq"\`` fields.
    - In `config.go`, add `RabbitMQConfig` struct with fields: `Host string`, `Port int`, `User string`, `Password string` (`// #nosec G117`), `VHost string`, `Queue string`, `ReconnectDelay time.Duration` — all with `mapstructure` tags.
    - In `defaults.go`, add constants: `DefaultQueueProvider = "rabbitmq"`, `DefaultRabbitMQHost = "localhost"`, `DefaultRabbitMQPort = 5672`, `DefaultRabbitMQUser = "guest"`, `DefaultRabbitMQVHost = "/"`, `DefaultRabbitMQQueue = "enrichment-jobs"`, `DefaultRabbitMQReconnectDelay = 5 * time.Second`.
    - In `config.go`'s `SetDefaults` function, add viper defaults for: `queue.provider`, `queue.rabbitmq.host`, `queue.rabbitmq.port`, `queue.rabbitmq.user`, `queue.rabbitmq.password` (empty string), `queue.rabbitmq.vhost`, `queue.rabbitmq.queue`, `queue.rabbitmq.reconnect_delay`.
  - Verify: `cd src && go build ./internal/config/... && go test ./internal/config/...`
  - Done: Both commands exit 0 with no errors or test failures.

- [x] **Cycle 2 - Publisher package**: Add `github.com/rabbitmq/amqp091-go` to `go.mod` and create the `Publisher` interface and RabbitMQ publisher implementation.
  - Agent: `go software engineer`
  - Files: `src/internal/queue/queue.go`, `src/internal/queue/rabbitmq/publisher.go`, `src/go.mod`, `src/go.sum`
  - Steps:
    - Run `cd src && go get github.com/rabbitmq/amqp091-go` to add the dependency and update `go.sum`.
    - Create `src/internal/queue/queue.go`: package `queue`; define `Publisher` interface with `Publish(ctx context.Context, jobID uuid.UUID) error` and `Close() error`.
    - Create `src/internal/queue/rabbitmq/publisher.go`: package `rabbitmq`; define `PublisherConfig` struct (fields: `Host`, `Port int`, `User`, `Password`, `VHost`, `Queue string`, `ReconnectDelay time.Duration`); define `RabbitMQPublisher` struct holding config, conn, and channel; implement `NewPublisher(cfg PublisherConfig) (queue.Publisher, error)` that dials (building `amqp://user:pass@host:port/vhost` URL), opens a channel, and declares the queue as durable — return error on dial or declare failure; implement `Publish` that marshals `{"job_id": "<uuid>"}` as JSON, calls `ch.PublishWithContext` targeting the queue with a persistent delivery mode, and on error calls an internal `reconnect` helper then retries once before returning the error; implement `reconnect` that re-dials and re-declares the queue (matching the pattern in `mnemonic-enricher/src/internal/queue/rabbitmq/subscriber.go`); implement `Close` that closes channel then connection, joining errors.
  - Verify: `cd src && go build ./internal/queue/...`
  - Done: `go build` exits 0 with no compilation errors.

- [x] **Cycle 3 - Pattern service publishing**: Inject `queue.Publisher` into `patternService` and publish each enrichment job ID after the DB row is created.
  - Agent: `go software engineer`
  - Files: `src/internal/service/pattern/service.go`, `src/internal/service/pattern/service_test.go`, `src/internal/service/pattern/split_chunks_test.go`
  - Steps:
    - Add `publisher queue.Publisher` field to the `patternService` struct.
    - Add `publisher queue.Publisher` as the last parameter before `logger` in `New()`; assign to struct field.
    - Add a private `publishJob(ctx context.Context, jobID uuid.UUID)` method: call `s.publisher.Publish(ctx, jobID)`; on non-nil error log a warning via `s.logger` and return without propagating.
    - In `Create`, after each successful `s.enrichmentRepo.Create(ctx, &job)` call (both chunk path and legacy path), call `s.publishJob(ctx, job.ID)`.
    - In `Update`, after each successful `s.enrichmentRepo.Create(ctx, &job)` call (both chunk path and legacy path), call `s.publishJob(ctx, job.ID)`.
    - In `service_test.go`, define a `mockPublisher` type in the test package implementing `queue.Publisher` with a `publishedIDs []uuid.UUID` field and configurable `publishErr error`; update every `New(...)` call site in tests to pass a `&mockPublisher{}`.
    - Verify `split_chunks_test.go` does not call `New()` directly; if it does, update accordingly.
  - Verify: `cd src && go build ./internal/service/pattern/... && go test ./internal/service/pattern/...`
  - Done: Both commands exit 0 — note this verify is intentionally scoped to the pattern package; server.go's call site is updated in Cycle 4.

- [x] **Cycle 4 - Server wiring, worker removal, and dead code deletion**: Wire the RabbitMQ publisher in `server.go`, remove the internal enrichment worker, and delete all packages that become dead code.
  - Agent: `go software engineer`
  - Files: `src/internal/server/server.go`, `src/internal/enricher/enrichment.go`, `src/internal/enricher/enrichment_test.go`, `src/internal/enricher/doc.go`, `src/internal/service/enrichment/service.go`, `src/internal/service/enrichment/service_test.go`, `src/internal/service/openai/extraction.go`, `src/internal/service/openai/extraction_test.go`, `src/internal/service/openai/export_test.go`
  - Steps:
    - In `server.go`, add imports for `"github.com/twistingmercury/mnemonic-api/internal/queue/rabbitmq"` and `queue "github.com/twistingmercury/mnemonic-api/internal/queue"`.
    - In `wireDependencies`, construct the publisher: `pub, err := rabbitmq.NewPublisher(rabbitmq.PublisherConfig{...})` using `cfg.Queue.RabbitMQ` fields; return the error if construction fails.
    - Pass `pub` to `patternsvc.New(...)` as the publisher argument.
    - Remove `enrichmentSvc` construction (`enrichmentsvc.New(...)`) and its import.
    - Remove `extractionSvc` construction (`openaisvc.NewExtractionService(...)`) — keep `embeddingSvc` (still used by `searchSvc`).
    - Remove `*enricher.Worker` from `wireDependencies` return values and update the function signature.
    - In `ListenAndServe`, remove the `enrichWorker.Run(gCtx)` goroutine.
    - In `ListenAndServe`, add `defer pub.Close()` after the publisher is wired (thread it through from `wireDependencies` return).
    - Remove unused imports from `server.go`: the `enricher` package import and `enrichmentsvc` import.
    - Delete `src/internal/enricher/enrichment.go`, `src/internal/enricher/enrichment_test.go`, `src/internal/enricher/doc.go` (entire package).
    - Delete `src/internal/service/enrichment/service.go`, `src/internal/service/enrichment/service_test.go` (entire package).
    - Delete `src/internal/service/openai/extraction.go`, `src/internal/service/openai/extraction_test.go`.
    - Inspect `src/internal/service/openai/export_test.go`: if it only exports symbols for extraction tests, delete it; if it also exports for embedding tests, remove only the extraction-related exports.
  - Verify: `cd src && go build ./... && go test ./...`
  - Done: Both `go build` and `go test` exit 0 with no errors or test failures; the deleted packages no longer exist on disk.

- [x] **Cycle 5 - E2E test updates**: Update E2E patterns tests to remove enrichment-status polling and accept `pending` as the terminal state in the test environment.
  - Agent: `go e2e test engineer`
  - Files: `src/tests/e2e/api/patterns_test.go`
  - Steps:
    - Locate the test that polls enrichment status (currently around line 2335: `// Poll until enrichment completes`); remove the polling loop and replace with a single GET after creation; assert `enrichment_status == "pending"` (the expected state when no enricher is running).
    - Locate `TestGetPatternEnrichedAt` (around line 2360); update to assert `enrichment_status == "pending"` and `enriched_at == ""` rather than polling or checking for enriched state.
    - Scan the rest of `patterns_test.go` for any other assertions that expect `enrichment_status` to be `"enriched"` or `"failed"` as a definitive outcome (not as one of a valid set); update those to accept `"pending"`.
    - Do not change assertions that already accept `"pending"` as one of `{pending, enriched, failed}` — those are already correct.
    - Do not modify any other E2E test files.
  - Verify: `cd src/tests/e2e && go build ./...`
  - Done: `go build` exits 0; no test file imports or calls removed enrichment packages; updated tests no longer contain polling loops for enrichment status.

- [x] **Cycle 6 - Docker Compose and final validation**: Add RabbitMQ to both compose files, wire env vars, and validate with `make build`.
  - Agent: `devops engineer`
  - Files: `docker-compose.yaml`, `src/tests/docker-compose.yaml`
  - Steps:
    - In `docker-compose.yaml`, add `dev_rabbitmq` service: image `rabbitmq:3-management-alpine`, restart `unless-stopped`, ports `5672:5672` and `15672:15672`, healthcheck `rabbitmq-diagnostics ping` (interval 10s, timeout 5s, retries 5), network `dev_network`.
    - In `docker-compose.yaml`, add `dev_rabbitmq` to `dev_api`'s `depends_on` (condition: `service_healthy`) and add env var `MNEMONIC_QUEUE_RABBITMQ_HOST: dev_rabbitmq`.
    - In `src/tests/docker-compose.yaml`, add `rabbitmq` service with the same image and healthcheck configuration, no port exposure needed, network `mnemonic-network`.
    - In `src/tests/docker-compose.yaml`, add `rabbitmq` to `mnemonic_api`'s `depends_on` (condition: `service_healthy`) and add env var `MNEMONIC_QUEUE_RABBITMQ_HOST: rabbitmq`.
  - Verify: `docker compose -f docker-compose.yaml config --quiet && docker compose -f src/tests/docker-compose.yaml config --quiet && make build`
  - Done: All three commands exit 0; `make build` completes successfully including E2E tests.

## Risks and Mitigations

- Risk: Changing `patternsvc.New()` signature in Cycle 3 breaks `server.go` compilation until Cycle 4 updates the call site.
  - Mitigation: Cycle 3 verify is scoped to `./internal/service/pattern/...` only; Cycle 4 verify runs `./...` to catch the integrated build.
- Risk: `export_test.go` may export symbols used by both extraction and embedding tests — deleting it blindly would break embedding tests.
  - Mitigation: Cycle 4 steps include inspecting the file before deleting; only extraction-related exports are removed.
- Risk: E2E polling test times out waiting for enrichment that never arrives (no enricher in E2E), causing flaky 10-second waits.
  - Mitigation: Cycle 5 removes the polling loop and asserts `pending` directly.
- Risk: `make build` fails in Cycle 6 because RabbitMQ is not healthy before the API container starts.
  - Mitigation: `depends_on` with `condition: service_healthy` and the RabbitMQ healthcheck guarantee ordering.
- Risk: A cycle Done condition is too vague, causing gralph to loop without completing.
  - Mitigation: Every Done is tied to a concrete shell command exiting 0.

## Definition of Done

- `cd src && go build ./... && go test ./...` exits 0.
- `make analyze` exits 0 (goimports, golangci-lint, govulncheck, gosec).
- `docker compose -f docker-compose.yaml config --quiet` exits 0.
- `docker compose -f src/tests/docker-compose.yaml config --quiet` exits 0.
- `make build` exits 0 (Docker image built, E2E tests passed).
- The `internal/enricher`, `internal/service/enrichment`, and `internal/service/openai/extraction*` files do not exist in the repository.
