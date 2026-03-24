# Phase 2 Fix Cycles 1–5 — Architecture Review

[Back to Architectural Decisions](00-architectural-decisions.md) | [Back to Project README](../../README.md)

**Date:** 2026-03-23
**Scope:** Cycles 1–5 resolving findings from `docs/code-reviews/phase-02-queue-publishing.md`
**Files reviewed:**

- `src/internal/queue/rabbitmq/publisher.go`
- `src/internal/service/pattern/service.go`
- `src/internal/service/pattern/service_test.go`
- `src/internal/server/server.go`
- `src/tests/docker-compose.yaml`

---

## Summary

Cycles 1–5 correctly resolve all HIGH and most MEDIUM/LOW findings from the Phase 2 code review. The mutex scope is safe and deadlock-free. The nil-guard removal is safe given startup wiring. Two items remain open: observability on stuck pending jobs (M5 / R-001) is addressed only at the documentation level, and `amqpURL()` still embeds the password in the return value without masking (M4).

No new architectural concerns were introduced.

---

## Finding-by-Finding Assessment

### FC-001 — Mutex scope in `Publish` and `Close`

**Severity: PASS**
**Resolves: H2 (data race), R-004**

`Publish` acquires `p.mu` before `PublishWithContext` and holds it through the optional `reconnect` + retry path. `Close` acquires `p.mu` before reading `p.ch` and `p.conn`. Neither method calls back into any function that also acquires `p.mu`.

`reconnect` is called only from within `Publish` while the lock is already held. `reconnect` itself does not acquire the mutex — it operates directly on `p.ch` and `p.conn`. This is correct: taking the lock again from within `Publish` would deadlock because Go's `sync.Mutex` is not reentrant. The invariant holds: all access to the mutable fields `p.ch` and `p.conn` is gated by the caller holding the lock.

No deadlock risk. No data race. Finding resolved.

---

### FC-002 — Context-aware reconnect delay

**Severity: PASS**
**Resolves: M1 (reconnect blocks HTTP goroutine), R-003**

`time.Sleep(p.cfg.ReconnectDelay)` is replaced with:

```go
select {
case <-time.After(p.cfg.ReconnectDelay):
case <-ctx.Done():
    return fmt.Errorf("rabbitmq: reconnect cancelled: %w", ctx.Err())
}
```

The HTTP handler's request context now propagates into the reconnect path. A timed-out or cancelled request aborts the reconnect wait rather than sleeping to completion. The full `ReconnectDelay` is still incurred when the context is live and the broker is down, but this is bounded by the client's timeout and the context propagates correctly.

Finding resolved. The remaining latency under broker failure (sleeping the full `ReconnectDelay`) is acceptable given publish is best-effort and the primary record is already in PostgreSQL.

---

### FC-003 — Type rename and nosec annotation

**Severity: PASS**
**Resolves: L2 (type stutter), M3 (wrong nosec rule)**

`RabbitMQPublisher` renamed to `Publisher`. The concrete type is only referenced in `server.go` via `NewPublisher`, so the impact is local. The nosec annotation is corrected from `G117` to `G101` with a justification comment. Both are mechanical fixes with no behavioral change.

Finding resolved.

---

### FC-004 — Doc comments and amqpURL password warning

**Severity: PARTIAL**
**Resolves: M4 (partially), L6 (VHost constraint), M5 (partially)**

The package doc now includes the operational runbook query for stuck pending jobs. `amqpURL()` now carries a godoc comment: "The returned string embeds a plaintext password and must never be logged or included in error messages." The VHost encoding constraint is documented in `PublisherConfig`.

**Remaining gap (M4):** The warning is documentation-only. `amqpURL()` still returns a string with a plaintext password that any future caller can accidentally log. The `SafeDSN()` / `SafeURI()` pattern used for Postgres and Neo4j returns a redacted string for logging and keeps the full credentials internal. `amqpURL()` does not follow this pattern. The current call site in `NewPublisher` and `reconnect` is correct — neither passes the URL to a logger — but the function is exported-equivalent in its risk surface (the field is unexported but `amqpURL()` is a method on the exported `PublisherConfig`).

This is a LOW risk item. The documentation warning reduces the probability of accidental exposure but does not eliminate it. Closing M4 fully would require removing the string-building method and passing credentials via `amqp.Config.SASL` instead, consistent with R-008's suggestion. That is deferred.

**M5 status:** The package doc runbook note is a useful reference, but there is still no runtime signal — no metric counter, no health check contribution, no alert threshold. The observability gap identified in R-001 remains open at the infrastructure level.

---

### FC-005 — `TestPublishJobError` test coverage

**Severity: PASS**
**Resolves: L3 (publishErr mock field never exercised)**

Two sub-tests are added:

- `Create propagates no error when publish fails`: wires `publishErr: errors.New("queue unavailable")`, calls `Create`, asserts the call succeeds and `pub.publishedIDs` is empty.
- `Update propagates no error when publish fails`: same pattern for `Update`, including full transaction lifecycle expectations.

Both tests exercise the primary behavioral invariant of the best-effort publish path: a queue failure does not fail the caller. The tests do not assert on log output, which is acceptable — the logger is `zerolog.Nop()` and the warning is structural, not behavioral.

One observation: both tests use content without `[//]: pattern` decorators, so no chunks are produced and `Publish` is never actually invoked with a failing publisher. The tests confirm that the zero-chunk path does not call `Publish` at all. Tests that exercise the publish failure with actual chunks (which would call `er.Create` and then `s.publisher.Publish`) would provide stronger coverage of the `publishJob` warn path. This is a LOW gap, not a blocking concern.

Finding resolved at the required level.

---

### FC-006 — Nil-guard removal in `service.go`

**Severity: PASS**
**Resolves: M2 (dead nil-guard branches)**

The `if s.chunkRepo == nil` branches in `Create`, `Update`, and `ListChunks` are removed. `server.go` confirms `chunkRepo` is always wired at startup via `chunkrepo.NewRepository(pgPool)` before `patternsvc.New(...)` is called. The `New` constructor does not accept a nil `chunkRepo`. There is no code path that creates a `patternService` without a real `chunkRepo`.

The removal is safe. The previous nil-guard in `ListChunks` was the most dangerous: it returned an empty slice rather than panicking, which would have silently masked a wiring bug. Removing it means a nil `chunkRepo` now panics on first use — the correct behavior for a programmer error.

`newTestService` in `service_test.go` (line 520) now passes `new(mockChunkRepo)` rather than `nil`, confirming the test helper is updated to match.

Finding resolved.

---

### FC-007 — Test compose credential alignment

**Severity: PASS**
**Resolves: L5 (test compose / dev compose credential inconsistency)**

`src/tests/docker-compose.yaml` now sets `RABBITMQ_DEFAULT_PASS: mnemonic_dev`, matching the dev `docker-compose.yaml`. The `mnemonic_api` container in the test compose sets `MNEMONIC_QUEUE_RABBITMQ_PASSWORD: mnemonic_dev` to match. The previous mismatch (`mnemonic` vs `mnemonic_dev`) would have caused a broker authentication failure in the e2e test environment.

Finding resolved.

---

## Remaining Open Items

| ID         | Severity | Description                                                  | Status                                      |
| ---------- | -------- | ------------------------------------------------------------ | ------------------------------------------- |
| M4         | LOW      | `amqpURL()` returns plaintext password; no runtime masking   | Documentation only; structural fix deferred |
| M5 / R-001 | MEDIUM   | No runtime metric or alert for stuck pending enrichment jobs | Documentation only; operational gap remains |

Neither item blocks Phase 2 from shipping. M5 / R-001 should be addressed before the enrichment pipeline is relied upon in a monitored production environment.

---

## Architectural Integrity Assessment

**H1 (transaction phantom):** Confirmed resolved in a prior cycle. `updateWithTransaction` now constructs `txPatternRepo` and `txChunkRepo` via `WithTx(tx)` (service.go lines 453–454), and all four writes execute against those tx-scoped instances. The `mockPatternRepo.WithTx` and `mockChunkRepo.WithTx` implementations in the test file return `m` (self), which correctly routes test expectations through the same mock for both pool-backed and tx-scoped calls. The transaction is not a phantom.

**Publisher lifecycle:** `pub.Close()` is deferred in `ListenAndServe` after `wireDependencies` returns. The publisher is not closed before the HTTP server shuts down — the errgroup runs first, then the deferred close runs. This is correct ordering.

**Interface boundary:** `patternService` holds `queue.Publisher`, not `*rabbitmq.Publisher`. The concrete type appears only in `wireDependencies`. This boundary is maintained through all five cycles.

**No new concerns introduced.**
