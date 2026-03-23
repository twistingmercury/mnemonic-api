# Product Requirements Document: Phase 2 Code Review Fixes

_Gralph processes cycles in this document from top to bottom. Checklist markers are significant: `- [ ]` (open), `- [x]` (complete), `- [~]` (abandoned). Each cycle must be small, independently verifiable, and assigned to exactly one agent._

## Objective

Resolve all actionable findings from the Phase 2 queue-publishing code review. The H1 transaction scope bug was fixed before this loop started. This loop addresses the remaining eleven findings in priority order, leaving the codebase clean before the branch is merged.

## Problem Statement

The Phase 2 code review (`docs/code-reviews/phase-02-queue-publishing.md`) identified a data race, a latency amplifier in the HTTP request path, dead code, a wrong lint suppression comment, missing test coverage, and several minor style issues. None are blocking — the branch builds and passes — but they are real bugs or misleading code that will cause problems later if left unaddressed.

## Success Criteria

- `go test -race ./...` from `src/` exits 0 with no races reported.
- `RabbitMQPublisher` type does not exist (renamed to `Publisher`).
- No `if s.chunkRepo == nil` guard remains in `service.go`.
- No redundant `queue` import alias in `service.go` or `server.go`.
- `publishErr` path in `publishJob` is covered by at least one test.
- `make analyze` exits 0.
- `docker compose -f src/tests/docker-compose.yaml config --quiet` exits 0.

## Scope

### In scope

- `src/internal/queue/rabbitmq/publisher.go` — mutex, ctx-aware reconnect, rename, nosec annotation, doc comments.
- `src/internal/service/pattern/service.go` — remove import alias, remove dead nil-guard branches.
- `src/internal/service/pattern/service_test.go` — publishErr tests, newTestService comment update.
- `src/internal/server/server.go` — remove import alias.
- `src/tests/docker-compose.yaml` — align RabbitMQ password with dev compose.

### Out of scope

- No new packages or interfaces beyond what is already in the repo.
- No E2E test changes.
- No CI workflow changes.
- M5 (stuck-jobs monitoring) and L6 (vhost encoding doc) are embedded as inline doc comments in publisher.go — no separate files.

## Constraints and Decisions

- Go module root: `src/` — all `go` commands run from there.
- `go test -race ./...` required for Cycle 1 (the race fix cycle).
- `make analyze` must pass after any `.go` file is modified.
- Signed commits: `git commit -S`.
- Rename `RabbitMQPublisher` → `Publisher` is safe: `NewPublisher` returns the `queue.Publisher` interface, so the concrete type is never visible outside the `rabbitmq` package.
- Removing nil-guard branches in Cycle 4 requires updating `newTestService` in the same cycle to pass a real `mockChunkRepo` so no test regresses.

## Implementation Plan

- [ ] **Cycle 1 - Publisher mutex and ctx-aware reconnect**: Add a `sync.Mutex` to `RabbitMQPublisher` to eliminate the data race (H2), and replace `time.Sleep` in `reconnect` with a context-aware select so the HTTP goroutine can be cancelled during a reconnect delay (M1).
  - Agent: `go software engineer`
  - Files: `src/internal/queue/rabbitmq/publisher.go`
  - Steps:
    - Add `mu sync.Mutex` field to `RabbitMQPublisher` struct; add `"sync"` to imports.
    - In `Publish`: wrap the entire publish-attempt → reconnect → retry block with `p.mu.Lock()` / `defer p.mu.Unlock()` so concurrent callers cannot interleave on the channel or trigger concurrent reconnects.
    - Change `reconnect` signature to `reconnect(ctx context.Context) error`.
    - In `reconnect`: replace `time.Sleep(p.cfg.ReconnectDelay)` with `select { case <-time.After(p.cfg.ReconnectDelay): case <-ctx.Done(): return fmt.Errorf("rabbitmq: reconnect cancelled: %w", ctx.Err()) }`.
    - Update the `p.reconnect()` call site in `Publish` to `p.reconnect(ctx)`.
    - In `Close`: acquire `p.mu.Lock()` / `defer p.mu.Unlock()` before accessing `p.ch` and `p.conn`.
  - Verify: `cd src && go test -race ./... && make build`
  - Done: Both commands exit 0; `go test -race` reports no data races; `make build` completes successfully including linters, unit tests, and E2E tests.

- [ ] **Cycle 2 - Publisher cleanup**: Rename the concrete type (L2), fix the wrong gosec annotation (M3), add log-safety and operational doc comments (M4, M5, L6), and remove the redundant import aliases from service.go and server.go (L1).
  - Agent: `go software engineer`
  - Files: `src/internal/queue/rabbitmq/publisher.go`, `src/internal/service/pattern/service.go`, `src/internal/server/server.go`
  - Steps:
    - In `publisher.go`: rename `RabbitMQPublisher` to `Publisher` in the struct definition, the `&RabbitMQPublisher{...}` literal in `NewPublisher`, and all method receivers (`func (p *RabbitMQPublisher)` → `func (p *Publisher)`).
    - In `publisher.go`: change `Password string // #nosec G117` to `Password string // #nosec G101 — value supplied via config, not hardcoded in source`.
    - In `publisher.go`: add to `amqpURL()` godoc: `// The returned string embeds a plaintext password and must never be logged or included in error messages.`
    - In `publisher.go`: add to `PublisherConfig` godoc: `// VHost must be URL-safe; names containing special characters must be percent-encoded by the caller before assignment.`
    - In `publisher.go`: add to the package doc (`// Package rabbitmq ...`): `// Operational note: if Publish returns an error the enrichmentjob row remains in PostgreSQL with status "pending". Operators should alert on: SELECT count(*) FROM enrichment_jobs WHERE status = 'pending' AND created_at < NOW() - INTERVAL '10 minutes'.`
    - In `service.go`: change `queue "github.com/twistingmercury/mnemonic-api/internal/queue"` to `"github.com/twistingmercury/mnemonic-api/internal/queue"` (remove alias).
    - In `server.go`: change `queue "github.com/twistingmercury/mnemonic-api/internal/queue"` to `"github.com/twistingmercury/mnemonic-api/internal/queue"` (remove alias).
  - Verify: `make build`
  - Done: `make build` exits 0; no `RabbitMQPublisher` identifier exists in the repo; no `queue "..."` import alias exists in service.go or server.go.

- [ ] **Cycle 3 - publishErr test coverage**: Add tests that exercise the warn-only publish-failure path in `publishJob` and update the `newTestService` comment to document its intentional scope (L3, partial L4).
  - Agent: `go software engineer`
  - Files: `src/internal/service/pattern/service_test.go`
  - Steps:
    - Add a new parallel test function `TestPublishJobError` with two subtests:
      - `Create propagates no error when publish fails`: construct a service via `newTestServiceWithChunkRepo` using a named `pub := &mockPublisher{publishErr: errors.New("queue unavailable")}`; set up mocks for a successful chunk-aware `Create` (pattern repo Create, SetAgentAssociations, chunk repo CreateBatch, enrichment repo Create returning a job with a populated ID); call `svc.Create`; assert result is non-nil and error is nil; assert `pub.publishedIDs` is empty (publish was called but failed).
      - `Update propagates no error when publish fails`: same setup but call `svc.Update`; assert same post-conditions.
    - Update the comment in `newTestService` from `// chunkRepo is nil: chunk creation is skipped during the transitional period.` to `// chunkRepo is nil intentionally: exercises the legacy pattern-level enrichment job path. Tests covering the production chunk-aware path use newTestServiceWithChunkRepo.`
  - Verify: `make build`
  - Done: `make build` exits 0; `TestPublishJobError` subtests exist and pass inside the Docker build stage.

- [ ] **Cycle 4 - Remove dead nil-guard branches**: Delete the unreachable `if s.chunkRepo == nil` guards in `Create`, `Update`, and `ListChunks` (M2), and update `newTestService` to pass a real mock chunk repo so no existing test regresses (L4).
  - Agent: `go software engineer`
  - Files: `src/internal/service/pattern/service.go`, `src/internal/service/pattern/service_test.go`
  - Steps:
    - In `service.go` `Create`: remove the `if s.chunkRepo != nil { ... } else { <legacy-path> }` wrapper; keep only the chunk-aware body (the former `if` branch content) as the unconditional implementation.
    - In `service.go` `Update`: remove the `if s.chunkRepo != nil { updateWithTransaction(...) } else { <legacy-path> }` wrapper; keep only the `updateWithTransaction` call as the unconditional implementation. Remove the entire legacy else-block.
    - In `service.go` `ListChunks`: remove the `if s.chunkRepo == nil { return []*chunkrepo.Chunk{}, nil }` nil guard; the repo is always wired and a nil value is a wiring bug that should surface as a panic.
    - In `service_test.go`: update `newTestService` to pass `new(mockChunkRepo)` instead of `nil` for `chunkRepo`.
    - For every existing test that uses `newTestService` and calls `Create` or `Update` with content that produces PATTERN-decorated chunks (check by inspecting each test's `Content` field — any content containing `[//]: pattern` will trigger chunk creation), add the necessary mock expectations to the `mockChunkRepo` (`cr.On("CreateBatch", ...).Return(nil)`) and `mockEnrichmentRepo` for per-chunk jobs.
    - For tests using plain-text content with no `[//]: pattern` decorators, no chunk repo expectations are needed (`splitIntoChunks` returns empty and `CreateBatch` is not called).
  - Verify: `make build`
  - Done: `make build` exits 0; `grep -r "chunkRepo == nil" src/internal/service/pattern/service.go` returns no matches.

- [ ] **Cycle 5 - Compose alignment and operational doc**: Align the test RabbitMQ credentials with the dev compose file (L5).
  - Agent: `devops engineer`
  - Files: `src/tests/docker-compose.yaml`
  - Steps:
    - Change `RABBITMQ_DEFAULT_PASS: mnemonic` to `RABBITMQ_DEFAULT_PASS: mnemonic_dev`.
    - Change `MNEMONIC_QUEUE_RABBITMQ_PASSWORD: mnemonic` to `MNEMONIC_QUEUE_RABBITMQ_PASSWORD: mnemonic_dev`.
    - The RabbitMQ user (`mnemonic`) and all other settings remain unchanged.
  - Verify: `make build`
  - Done: `make build` exits 0; both password values in `src/tests/docker-compose.yaml` match `docker-compose.yaml`.

## Risks and Mitigations

- Risk: Adding a mutex in Cycle 1 could deadlock if `Close` is called while `Publish` holds the lock.
  - Mitigation: Both `Publish` and `Close` take the same lock; `Publish` releases it via `defer` before returning, so `Close` always eventually acquires it. No recursive lock acquisition is possible.
- Risk: Removing nil-guard branches in Cycle 4 could break tests that relied on the nil chunkRepo path.
  - Mitigation: Cycle 4 steps explicitly require updating `newTestService` and scanning every call site before marking the cycle done; `go test ./...` is the final gate.
- Risk: A cycle Done condition is too vague, causing gralph to loop without completing the item.
  - Mitigation: Every Done is tied to a concrete shell command or grep result, not intent.

## Definition of Done

- `make build` exits 0 (Docker image built, E2E tests passed).
- `grep -r "chunkRepo == nil" src/internal/service/pattern/service.go` returns no matches.
- `grep -r "RabbitMQPublisher" src/` returns no matches.
- `grep -r 'queue "github.com' src/internal/service/pattern/service.go src/internal/server/server.go` returns no matches.
