# Phase 2 Architecture Review — Queue Publishing

[Back to Architectural Decisions](00-architectural-decisions.md) | [Back to Project README](../../README.md)

**Date:** 2026-03-23
**Scope:** `src/internal/queue/`, `src/internal/queue/rabbitmq/`, `src/internal/service/pattern/service.go`, `src/internal/server/server.go`
**Objective:** Replace inline polling enrichment worker with RabbitMQ-backed job dispatch.

---

## Summary

The Phase 2 changes are structurally sound. The dual-write design is the correct trade-off for this system. Two findings require action before this pattern is used in a production environment at scale: the synchronous reconnect sleep inside the HTTP handler (R-003) and the transaction phantom in `updateWithTransaction` (R-007). The remaining items are low risk or acceptable given the stated design constraints.

---

## Findings

### R-001 — Dual write: acceptable trade-off, recovery path needs definition

**Severity: MEDIUM**

The sequence — commit to PostgreSQL, then publish to RabbitMQ — is the correct choice for this system. The pattern write is never held hostage to broker availability, and the `pending` status is a natural recovery signal. ADR-009 codifies this.

The gap is operational: nothing currently monitors pending job age. A job stuck in `pending` for hours or days is silent. Without alerting or a sweep process, the enrichment lag is invisible until a user notices a pattern with stale embeddings.

**Suggested approach:** Add an observability alert on `SELECT COUNT(*) FROM enrichment_jobs WHERE status = 'pending' AND created_at < NOW() - INTERVAL '10 minutes'`. A sweep process that republishes old pending jobs can be deferred to Phase 3 but the alert should ship with Phase 2.

---

### R-002 — Publisher ownership: server.go is the right boundary

**Severity: LOW**

`queue.Publisher` is created in `wireDependencies`, returned to `ListenAndServe`, and closed via `defer pub.Close()`. The service receives it as a dependency; it does not own the lifecycle.

This is correct. The publisher holds a network connection. Lifecycle management belongs at the application boundary (server startup/shutdown), not inside a domain service. The service only needs to call `Publish`; it has no business controlling when the connection closes.

No change needed.

---

### R-003 — Synchronous reconnect sleep inside HTTP handler goroutine

**Severity: HIGH**

`reconnect()` calls `time.Sleep(cfg.ReconnectDelay)` — defaulting to 5 seconds — synchronously inside `Publish`, which is called inside an HTTP request handler goroutine.

Under broker degradation (connection drop, restart), every pattern create or update that triggers a reconnect will stall for 5 seconds before the retry. Go's HTTP server handles each request in its own goroutine, so this does not block other requests, but the affected requests will time out from the client's perspective if the client timeout is shorter than the reconnect delay plus the retry round-trip.

More critically, if the broker is completely down, every publish attempt after the first failure follows the sequence: fail → sleep 5 s → reconnect (fail) → return error → `publishJob` logs and returns. That is still 5 s of latency per request for the reconnect sleep, even though the call path ultimately exits without publishing.

The publish is best-effort and the caller does not propagate the error, so this does not cause data loss. But it is a latency landmine that will surface under any broker instability.

**Suggested approach:** Move the reconnect sleep out of the hot path. The two viable options are:

1. Run reconnect in a background goroutine with a channel-based circuit breaker. If the publisher knows the connection is down, `Publish` returns immediately with an error (fast-fail) rather than sleeping.
2. Reduce `ReconnectDelay` to 0 or a very short value (100–200 ms) and rely on the operating system's TCP connection timeout to bound the dial time. The sleep was likely added to avoid tight reconnect loops, which a short delay still accomplishes.

Option 2 is the lower-effort fix. Option 1 is more correct but requires more structural change to the publisher.

---

### R-004 — Single channel, concurrent safety

**Severity: LOW (current usage); MEDIUM (if usage changes)**

AMQP channels are not safe for concurrent use. `RabbitMQPublisher` uses a single channel (`p.ch`) with no mutex. Under the current usage model — one `patternService` instance, HTTP requests serialized through Gin's goroutine-per-request model where `publishJob` is fire-and-forget from the handler goroutine — concurrent publishes from different request goroutines are possible.

In practice, the serialization guarantee only holds if `publishJob` is the last call before the handler returns and there is no goroutine spawned between them. Looking at the code, `publishJob` is called directly (no `go` keyword), so concurrent HTTP requests will call it concurrently. This is a real concurrency issue, not theoretical.

However, because publish failures are silently swallowed, a channel corruption from concurrent use would manifest as a silent message loss, not a panic. The risk is bounded.

**Suggested approach:** Add a `sync.Mutex` to `RabbitMQPublisher` and hold it around `PublishWithContext` and the reconnect path. This is a one-line struct change and a few lock/unlock calls. It is the minimum correct fix.

---

### R-005 — No dead-letter queue or publisher confirms

**Severity: LOW**

Messages are published without mandatory flag and without publisher confirms. The `mandatory` flag being `false` means a message with no binding (e.g., queue deleted, exchange misconfigured) is silently dropped by the broker. Publisher confirms would let the publisher know the broker received and queued the message.

Given the PostgreSQL safety net, a silently dropped message results in a stuck `pending` job rather than data loss. The risk is acceptable at this scale.

If publisher confirms were added, they would need to be asynchronous (confirm channel) to avoid blocking the HTTP handler, which adds significant complexity. The marginal benefit over the existing DB safety net does not justify that complexity in Phase 2.

No change needed for Phase 2. Revisit if the enricher moves to a model where the DB row is not the recovery source.

---

### R-006 — Interface placement: no import cycle, idiomatic Go

**Severity: LOW (informational)**

`queue.Publisher` is defined in `internal/queue`. The implementation `internal/queue/rabbitmq` imports `internal/queue` to satisfy the interface. There is no import cycle: the parent package does not import the child.

This is idiomatic Go. Defining the interface in the consumer-side package (`internal/queue`) rather than the implementor package means consumers depend on the abstraction, not the concrete type. `patternService` imports `internal/queue` only, with no reference to `internal/queue/rabbitmq`. The wiring in `server.go` is the only place the concrete type appears.

No change needed.

---

### R-007 — Transaction phantom in `updateWithTransaction`

**Severity: HIGH**

`updateWithTransaction` begins a `pgx.Tx` but passes it to none of the repository methods. All four repository calls (`patternRepo.Update`, `patternRepo.SetAgentAssociations`, `chunkRepo.DeleteByPatternID`, `chunkRepo.CreateBatch`) execute against the underlying pool connection, not the transaction. The `tx.Commit()` at the end commits an empty transaction.

This means:
- The four writes are not atomic. A failure between any two steps leaves partial state.
- `tx.Rollback` in the deferred function rolls back the empty transaction, not the writes already committed to the pool.
- The comment in the function states these writes are inside "a single Postgres transaction" — that claim is incorrect.

This is a real defect. The exact consequence depends on which step fails, but the designed invariant (chunk delete + insert as a unit) is violated. A crash between step 3 (delete stale chunks) and step 4 (insert new chunks) leaves the pattern with no chunks and no enrichment jobs.

**Suggested approach:** Each repository method needs a `WithTx(tx pgx.Tx)` variant (or accept a `pgx.Tx` parameter) and `updateWithTransaction` must pass the transaction object to each call. This is a repository layer change, not just a service layer fix.

---

### R-008 — VHost URL encoding with default `/`

**Severity: LOW**

`amqpURL()` produces `amqp://user:pass@host:port//` when `VHost` is `/` (the default). The trailing double-slash is the URL representation for the default vhost.

The `amqp091-go` library parses the AMQP URL per the AMQP URI specification (as documented in its source). The path component after the last `/` is URL-decoded and used as the vhost. For `amqp://host:5672//`, the path is `/`, URL-decoded to `/`, which is the default vhost in RabbitMQ. This is correct behavior and the library handles it.

If an operator configures a named vhost (e.g., `mnemonic`), the URL becomes `amqp://user:pass@host:5672/mnemonic`, which is also correct. No URL-encoding concern arises for vhost names that are valid URI path segments.

The only edge case is a vhost name containing special characters (e.g., spaces, slashes beyond the default). The current `amqpURL()` function does not URL-encode `c.VHost`. For the default `/` case and any alphanumeric vhost name, this is safe. It would be a latent bug only if an operator configures a vhost with URL-special characters — unlikely in practice and easy to document as a constraint.

No change needed for the current use case.

---

## Risk Matrix

| Finding | Severity | Data loss risk | Latency risk | Action required |
|---------|----------|---------------|--------------|-----------------|
| R-001 Pending job accumulation | MEDIUM | No | No | Add observability alert |
| R-002 Publisher ownership | LOW | No | No | No change |
| R-003 Synchronous reconnect sleep | HIGH | No | Yes (5 s/request) | Fix reconnect path |
| R-004 Single channel concurrency | MEDIUM | Low (silent drop) | No | Add mutex |
| R-005 No publisher confirms / DLQ | LOW | No | No | No change in Phase 2 |
| R-006 Interface placement | LOW | No | No | No change |
| R-007 Transaction phantom | HIGH | Yes (partial chunk state) | No | Fix repository tx plumbing |
| R-008 VHost URL encoding | LOW | No | No | Document constraint |

---

## ADRs Added

- **ADR-008**: RabbitMQ for enrichment job dispatch — captures the Phase 2 architectural decision to replace the polling worker with queue publish.
- **ADR-009**: Best-effort publish with PostgreSQL as recovery source — captures the dual-write trade-off and the recovery model.

Both ADRs are appended to [`00-architectural-decisions.md`](00-architectural-decisions.md).

---

## Handoff to Go Software Architect

Two findings require implementation work before Phase 2 ships to production:

**R-007 (HIGH) — Transaction phantom in `updateWithTransaction`:** The repository layer does not accept a `pgx.Tx` parameter. The fix requires adding transactional variants to `patternrepo.Repository` and `chunkrepo.Repository`, then threading the `tx` object through `updateWithTransaction`. This is a repository layer contract change.

**R-003 (HIGH) — Synchronous reconnect sleep:** `RabbitMQPublisher.reconnect()` sleeps inside the HTTP goroutine. The minimum fix is to remove or reduce `ReconnectDelay` from the reconnect hot path. The preferred fix is a circuit-breaker approach that fast-fails publish when the connection is known to be down, with reconnect happening out-of-band.

**R-004 (MEDIUM) — Channel concurrency:** Add `sync.Mutex` to `RabbitMQPublisher` and hold it around `PublishWithContext` and the reconnect path.

**R-001 (MEDIUM) — Pending job observability:** Wire a metric or alerting query on pending job age. This belongs in the observability layer, not the queue package.
