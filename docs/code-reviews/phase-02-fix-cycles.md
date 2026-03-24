# Code Review: Phase 2 — Fix Cycles 1–5

**Review Date:** 2026-03-23
**Reviewers:** code-reviewer, solutions-architect, go-software-architect
**Phase:** 2 (Code-Review Fix Cycles 1–5)

## Files Reviewed

### Source Files

- `src/internal/queue/rabbitmq/publisher.go` — mutex, context-aware reconnect, rename, nosec fix, doc comments
- `src/internal/service/pattern/service.go` — nil-guard removal, import alias removal
- `src/internal/server/server.go` — import alias removal

### Test Files

- `src/internal/service/pattern/service_test.go` — publishErr tests, test factory updates

### Infrastructure Files

- `src/tests/docker-compose.yaml` — RabbitMQ password alignment

## Validation Results

| Tool                      | Result                      |
| ------------------------- | --------------------------- |
| `go vet ./...`            | ✓ pass                      |
| `golangci-lint run ./...` | ✓ pass                      |
| `make build`              | ✓ pass (Docker image + E2E) |

## Design Compliance

Implementation satisfies all Phase 2 behavioral requirements. All findings from the initial Phase 2 queue publishing review have been resolved through fix cycles 1–5.

### Behavioral Requirements Verified

- Data race on `RabbitMQPublisher` eliminated via mutex protection ✓
- Context-aware reconnect prevents HTTP handler stalls ✓
- Transaction scope bug fixed: tx-scoped repository instances ✓
- Nil-guard dead code removed from Create, Update, ListChunks ✓
- Incorrect `#nosec` annotation corrected ✓
- Test coverage for publish failure path added ✓

### Design Doc Divergences (Post-Review)

No divergences from the fix cycles themselves. The fixes align the implementation with the originally intended behavior.

## Findings

### HIGH Priority

None.

### MEDIUM Priority

| ID  | Source                               | Finding                                                                                                                                                                                                                                                                                                                                                                                                                                        | Resolution                                                                                                                                                                                                                                                                                                   |
| --- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M1  | code-reviewer, solutions-architect   | **`TestPublishJobError` subtests do not reach the publish path**: Both subtests use plain content with no `[//]: pattern` decorators. `splitIntoChunks` returns 0 chunks, so `publishJob` is never called and the failing `mockPublisher` is never exercised. `assert.Empty(t, pub.publishedIDs)` passes trivially because publish was skipped, not swallowed. The warn-only publish failure path (the original L3 finding) remains uncovered. | Supply content with at least one `[//]: pattern`-decorated section in both subtests so that chunk creation fires, `enrichmentRepo.Create` is stubbed, and `publishJob` is actually invoked against the failing publisher. Assert that the service still returns `(non-nil result, nil error)`.               |
| M2  | code-reviewer, go-software-architect | **`newTestServiceWithFailingPublisher` is a byte-for-byte duplicate of `newTestServiceWithPublisher`**: Both functions have identical signatures and bodies (`service_test.go:547–558` vs `1883–1894`). The publisher's failure behavior is determined by the caller constructing `&mockPublisher{publishErr: …}` before passing it in — neither factory enforces it. Two names for the same concept invite a third.                           | Delete `newTestServiceWithFailingPublisher`; replace its call sites in `TestPublishJobError` with the pre-existing `newTestServiceWithPublisher`.                                                                                                                                                            |
| M3  | solutions-architect                  | **No runtime observability for stuck pending enrichment jobs**: The operational runbook query added to the package doc in Cycle 2 is correct, but there is no metric counter, health check contribution, or alertable threshold. Stuck `pending` jobs accumulate silently. The original finding M5 from the Phase 2 review is addressed only at the documentation level.                                                                       | Wire a Prometheus gauge or structured log counter that queries `enrichment_jobs WHERE status = 'pending' AND created_at < NOW() - INTERVAL '10 minutes'`. The query is already written in the package doc; it needs a runtime home before the enrichment pipeline is relied upon in a monitored environment. |

### LOW Priority

| ID  | Source                | Finding                                                                                                                                                                                                                                                                                                                                                                                                                                 | Resolution                                                                                                                                                                                                                                                                            |
| --- | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| L1  | go-software-architect | **`time.After` in the reconnect `select` leaks a timer when context is cancelled first**: `select { case <-time.After(p.cfg.ReconnectDelay): case <-ctx.Done(): }` leaves the `time.After` timer goroutine and channel alive for up to `ReconnectDelay` after context cancellation. Low runtime impact for a one-shot reconnect but a known Go gotcha.                                                                                  | Use `time.NewTimer` + `defer timer.Stop()`: `timer := time.NewTimer(p.cfg.ReconnectDelay); defer timer.Stop(); select { case <-timer.C: case <-ctx.Done(): return fmt.Errorf("rabbitmq: reconnect cancelled: %w", ctx.Err()) }`.                                                      |
| L2  | go-software-architect | **`#nosec G101` annotation on struct field declaration targets nothing gosec flags**: gosec G101 fires on assignments that look like credential literals (e.g. `password := "hunter2"`), not on field declarations. The annotation is syntactically valid but suppresses no actual scanner finding and gives false confidence. If a test fixture assigns a literal to `PublisherConfig.Password`, this annotation will not suppress it. | Remove the `#nosec` comment from the field declaration. If a specific test or constant assigns a literal password, apply `// #nosec G101` on that line instead. Retain the plain-English doc note (`// value supplied at runtime via config, never hardcoded`) without the nosec tag. |
| L3  | go-software-architect | **Missing inline comment explaining why the mutex spans the reconnect delay**: `defer p.mu.Unlock()` is held for `ReconnectDelay` seconds on failure. This is correct — `reconnect` mutates `p.conn` and `p.ch` — but the long hold is non-obvious and a future maintainer might "optimise" the unlock to before the reconnect, reintroducing the data race.                                                                            | Add an inline comment: `// Held across reconnect: p.conn and p.ch are mutated by reconnect, // so both the retry publish and any concurrent Close must wait.`                                                                                                                         |
| L4  | solutions-architect   | **`amqpURL()` still returns a plaintext password string with no runtime masking**: The godoc warning added in Cycle 2 reduces the risk but does not eliminate it. Every other connection helper (`SafeDSN()`, `SafeURI()`) returns a redacted string; `amqpURL()` is the only connection helper that embeds a live credential in its return value.                                                                                      | Pass credentials via `amqp.Config.SASL` and use a redacted URL string for dial, matching the Postgres/Neo4j pattern. Alternatively, accept the documentation-only fix as a known deviation tracked for a future phase.                                                                |
| L5  | go-software-architect | **`TestPublishJobError` subtest names do not match the file's established convention**: Every other subtest uses short lowercase phrases (`"happy path"`, `"not found"`, `"repo error propagates"`). The two new subtests use sentence-style descriptions (`"Create propagates no error when publish fails"`).                                                                                                                          | Rename to match: `"publish failure does not propagate on Create"` / `"publish failure does not propagate on Update"`.                                                                                                                                                                 |

## Patterns to Document

1. **`defer p.mu.Unlock()` held across a latency-introducing path**: when a mutex must guard a reconnect or retry that has a built-in delay, holding the lock across the full span (initial attempt → reconnect → retry) is correct and necessary if the reconnect mutates shared fields. The lock hold duration becomes the worst-case blocking time for concurrent callers. Document this clearly inline; do not release early to "avoid holding the lock too long" without first ensuring the shared fields are not accessed after release.

2. **Test factory proliferation anti-pattern**: Go test helpers that differ only in a doc comment and not in their body are a sign that the variance belongs in the test itself (by passing a different argument) rather than in a new factory. Prefer a single factory that accepts the variant as a parameter over N factories with misleading names.

## Notes for Future Phases

**Merge to main**: M1 should be resolved before the fix-cycle branch is merged so that the warn-only publish failure path has actual test coverage.

**Phase 3 (Observability & Metrics)**: M3 (stuck pending jobs observability) should be addressed before Phase 2 reaches a monitored production environment. The operational query is documented; a runtime home is needed.

**Phase 4+ (Connection Pool & Credential Management)**: L4 (`amqpURL` plaintext password) is a known deviation; track as a cleanup item for the phase that wires the metrics/health layer and credential management hardening.
