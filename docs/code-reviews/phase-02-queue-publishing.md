# Code Review: Phase 2 — Queue Publishing

**Review Date:** 2026-03-23
**Reviewers:** code-reviewer, solutions-architect, go-software-architect
**Phase:** 2 (Queue Publishing — replace inline enrichment worker with RabbitMQ publishing)

## Files Reviewed

### Source Files

- `src/internal/queue/queue.go` — Publisher interface
- `src/internal/queue/rabbitmq/publisher.go` — RabbitMQ implementation (dial, declare, publish, reconnect, close)
- `src/internal/service/pattern/service.go` — publisher injection, publishJob helper, Create/Update call sites
- `src/internal/server/server.go` — wireDependencies wiring, defer pub.Close(), worker removal

### Infrastructure Files

- `docker-compose.yaml` — dev_rabbitmq service added
- `src/tests/docker-compose.yaml` — rabbitmq service added

### Test Files

- `src/internal/service/pattern/service_test.go` — mockPublisher added

## Validation Results

| Tool             | Result                      |
| ---------------- | --------------------------- |
| `go build ./...` | ✓ pass                      |
| `go vet ./...`   | ✓ pass                      |
| `go test ./...`  | ✓ pass                      |
| `goimports`      | ✓ pass                      |
| `golangci-lint`  | ✓ pass                      |
| `govulncheck`    | ✓ pass                      |
| `gosec`          | ✓ pass                      |
| `make build`     | ✓ pass (Docker image + E2E) |

## Design Compliance

Implementation satisfies all Phase 2 behavioral requirements from `docs/plans/phase-02/PRD.md`.

### Behavioral Requirements Verified

- `POST /v1/api/patterns` creates a pattern, writes `enrichmentjob` row with status `pending`, publishes `{"job_id": "<uuid>"}` ✓
- Internal enrichment worker goroutine removed ✓
- `internal/enricher/`, `internal/service/enrichment/`, `internal/service/openai/extraction*` deleted ✓
- E2E tests assert `enrichment_status == "pending"` without polling ✓
- `make build` exits 0 ✓

### Design Doc Divergences (Post-Review)

None identified.

## Findings

### HIGH Priority

| ID  | Source                      | Finding                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | Resolution                                                                                                                                                                        |
| --- | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| H1  | all three agents            | **Transaction scope bug**: `updateWithTransaction` (`service.go:483–535`) opens a `pgx.Tx` but every subsequent call (`s.patternRepo.Update`, `s.patternRepo.SetAgentAssociations`, `s.chunkRepo.DeleteByPatternID`, `s.chunkRepo.CreateBatch`) goes through the pool-backed repo instances, not the transaction. `tx.Commit()` commits an empty transaction. The four writes are NOT atomic. A crash between chunk delete (step 3) and chunk insert (step 4) leaves the pattern with no chunks and no enrichment jobs — the exact scenario the transaction was designed to prevent. | Construct tx-scoped repo variants (e.g. `patternrepo.NewRepository(tx)`, `chunkrepo.NewRepository(tx)`) inside `updateWithTransaction` and pass those to the four mutating calls. |
| H2  | code-reviewer, go-architect | **Data race on `RabbitMQPublisher`**: `p.conn` and `p.ch` are mutated by `reconnect()` (called from `Publish`) with no mutex. Concurrent HTTP handlers calling `publishJob` simultaneously can race on these fields. `Close()` also reads both fields and can overlap with an in-flight `Publish`.                                                                                                                                                                                                                                                                                   | Add a `sync.Mutex` to `RabbitMQPublisher`; lock across the full `Publish` critical section (marshal → publish attempt → optional reconnect + retry) and in `Close`.               |

### MEDIUM Priority

| ID  | Source                      | Finding                                                                                                                                                                                                                                                                                                                                                                                                                    | Resolution                                                                                                                                                                                                                                                                                                        |
| --- | --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| M1  | all three agents            | **`reconnect()` blocks HTTP goroutine for up to 5s and ignores `ctx`**: `time.Sleep(p.cfg.ReconnectDelay)` runs synchronously in the publish path. Under broker instability, every pattern create/update stalls for the full reconnect delay. The `ctx` deadline passed by the HTTP handler is ignored during the sleep, so the retry `PublishWithContext` call may immediately fail with a context error after a 5s wait. | Replace `time.Sleep` with a context-aware select: `select { case <-time.After(p.cfg.ReconnectDelay): case <-ctx.Done(): return ctx.Err() }`. Thread `ctx` into `reconnect`. Or, since publish is best-effort, skip the reconnect entirely on the publish path and let a background goroutine manage reconnection. |
| M2  | code-reviewer, go-architect | **`chunkRepo` nil-guard is dead code; stale comment**: `server.go:192` always wires a real `chunkRepo`. The nil-guard branches in `Create` (line 272), `Update` (line 404), and `ListChunks` (line 674) are never taken in production. The constructor comment at line 153–154 ("chunkRepo may be nil during the transitional period…Task 9") is factually wrong.                                                          | Remove the dead nil-guard branches and the stale comment. The `ListChunks` nil guard is especially misleading: it silently returns an empty slice rather than propagating a wiring bug.                                                                                                                           |
| M3  | code-reviewer, go-architect | **`#nosec G117` is the wrong gosec rule**: G117 is unrelated to credentials ("subprocess launched with variable" or "weak random"). The rule for credential fields is **G101**. The annotation suppresses nothing and misleads readers.                                                                                                                                                                                    | Run `gosec -include=G101 ./internal/queue/rabbitmq/...` to verify which rule fires. Correct to `// #nosec G101` with a justification comment, or remove entirely if no rule fires.                                                                                                                                |
| M4  | code-reviewer, go-architect | **`amqpURL()` embeds plaintext password in a string**: No active leak today (the URL is passed to `amqp.Dial` and errors wrap only the opaque error). However, any future caller who includes `cfg.amqpURL()` in an error message or log field will expose the password — unlike the Postgres `SafeDSN()` and Neo4j `SafeURI()` pattern used elsewhere.                                                                    | Document in `amqpURL()` godoc that the return value contains a plaintext password and must never be logged. Alternatively, pass credentials via `amqp.Config.SASL` and use a redacted URL for the dial call, consistent with how PG/Neo4j connections are handled.                                                |
| M5  | solutions-architect         | **No observability on stuck `pending` jobs**: A failed publish leaves an `enrichmentjob` row as `pending` forever. The recovery path (re-PUT) is correct by design (ADR-009), but there is no alerting. Stuck jobs accumulate silently and will not be noticed until a user reports missing enrichment.                                                                                                                    | Add a monitoring query: `enrichment_jobs WHERE status = 'pending' AND created_at < NOW() - INTERVAL '10 minutes'`. Wire into the health/metrics layer or document as an operational runbook item.                                                                                                                 |

### LOW Priority

| ID  | Source              | Finding                                                                                                                                                                                                                                                                   | Resolution                                                                                                                                                                                                                                   |
| --- | ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| L1  | go-architect        | **Redundant import alias**: `queue "github.com/twistingmercury/mnemonic-api/internal/queue"` in `service.go:15` and `server.go:24` — the alias `queue` is identical to the package name and adds no disambiguation.                                                       | Remove the alias; the import resolves identically without it.                                                                                                                                                                                |
| L2  | go-architect        | **Type stutters with package name**: `rabbitmq.RabbitMQPublisher` — callers read "RabbitMQ" twice.                                                                                                                                                                        | Rename to `rabbitmq.Publisher`. Low-impact since `NewPublisher` returns the `queue.Publisher` interface and the concrete type is only referenced internally.                                                                                 |
| L3  | code-reviewer       | **`publishErr` mock field never exercised**: `mockPublisher.publishErr` is defined but no test sets it to a non-nil value. The warn-only path through `publishJob` (the primary new behavior in Phase 2) has zero test coverage.                                          | Add a test that sets `publishErr`, calls `Create` or `Update`, asserts the call still succeeds (best-effort), and optionally asserts the warning was logged (use the logger-hook pattern from `TestCreate_ChunkJobFailuresSummarisedInLog`). |
| L4  | code-reviewer       | **`newTestService` defaults to nil `chunkRepo` — most tests bypass the production code path**: `newTestService` (used by `TestCreate`, `TestUpdate`, association tests, etc.) passes `nil` for `chunkRepo`, exercising the legacy path that no longer runs in production. | Make `newTestServiceWithChunkRepo` the default factory now that Task 9 is complete, or add a note to each test that uses the nil-chunkRepo path explaining the intentional scope.                                                            |
| L5  | code-reviewer       | **Test compose credential inconsistency**: `src/tests/docker-compose.yaml` uses `RABBITMQ_DEFAULT_PASS: mnemonic`; `docker-compose.yaml` uses `mnemonic_dev`. Harmless for ephemeral containers but could mask credential-configuration bugs.                             | Align to the same value or use a shared variable pattern.                                                                                                                                                                                    |
| L6  | solutions-architect | **VHost URL encoding edge case**: `/` vhost produces `amqp://user:pass@host:5672//`. `amqp091-go` handles this correctly per the AMQP URI spec, so no bug. However, a vhost name containing URL-special characters would silently produce a malformed URL.                | Document in `amqpURL()` that vhost values must be URL-safe, or percent-encode the vhost before interpolation.                                                                                                                                |

## Agreements Across All Three Reviewers

| Finding                                            | Agents                       |
| -------------------------------------------------- | ---------------------------- |
| H1 — transaction scope bug                         | all three                    |
| M1 — reconnect blocks HTTP goroutine / ignores ctx | all three                    |
| H2 — data race on publisher                        | code-reviewer + go-architect |
| M3 — wrong `#nosec` rule                           | code-reviewer + go-architect |
| M4 — amqpURL password risk                         | code-reviewer + go-architect |
| M2 — dead nil-guard / stale comment                | code-reviewer + go-architect |

## Patterns to Document

1. **Best-effort side-effect pattern**: a void helper method that calls a fallible external operation, logs a structured warning on failure with a recovery statement ("job remains pending in postgres"), and never propagates the error to the caller. Invariant: the primary record already exists in the source-of-truth store, so the side effect is safe to lose. See `publishJob` and `syncNeo4j` in `service/pattern/service.go`.

2. **Dual-write with DB safety net**: write to PostgreSQL first (durable), then publish to the message queue (best-effort). The job row in PostgreSQL is the recovery mechanism if the publish fails. This avoids distributed transactions at the cost of requiring an operational runbook for stuck jobs.

3. **Transaction scoping with pgx**: when multiple repository calls must be atomic, construct tx-bound repo instances (`repo.New(tx)`) inside the transaction boundary rather than passing `pgx.Tx` as a parameter to existing methods. Each repository should have a constructor that accepts either `*pgxpool.Pool` or `pgx.Tx` via a `DBTX` interface.

## Notes for Future Phases

**Phase 3** (if any): H1 (transaction scope bug) must be resolved before any feature that depends on atomic chunk+job consistency — the current transaction is a no-op. Consider this a blocking defect if chunks and jobs are expected to be created atomically.

**Operational**: M5 (stuck pending jobs) should be addressed before Phase 2 goes to production. The recovery path is correct by design but invisible without monitoring.
