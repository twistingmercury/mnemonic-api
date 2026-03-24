# Phase 2 Code Review — Remaining Items Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve the two open items from `docs/architecture/review-phase-02-fix-cycles-1-5.md`: eliminate the plaintext-password URL string (M4) and add a runtime publish-failure counter (M5/R-001).

**Architecture:** M4 replaces `amqp.Dial(url-with-credentials)` with `amqp.DialConfig(url-without-credentials, Config{SASL: ..., Vhost: ...})` so the password is never embedded in a Go string. M5 adds a `mnemonic.queue.publish_failures_total` counter inside the publisher, incremented after all retry attempts are exhausted, wired via a meter passed to `NewPublisher`.

**Tech Stack:** Go 1.26, `github.com/rabbitmq/amqp091-go v1.10.0` (`amqp.DialConfig`, `amqp.PlainAuth`, `amqp.Config`), OpenTelemetry Metrics SDK (`go.opentelemetry.io/otel/metric`).

---

## File Map

| Action | File                                                       | Purpose                                                         |
| ------ | ---------------------------------------------------------- | --------------------------------------------------------------- |
| Modify | `src/internal/queue/rabbitmq/publisher.go`                 | Replace `amqpURL()` with SASL dial; add publish-failure counter |
| Create | `src/internal/queue/rabbitmq/export_test.go`               | Expose internal counter constructor for unit testing            |
| Create | `src/internal/queue/rabbitmq/publisher_test.go`            | Unit tests for the failure counter                              |
| Modify | `src/internal/server/server.go`                            | Pass `tel.Meter("mnemonic/queue")` to `NewPublisher`            |

No new packages. No changes to the pattern service, metrics registry, or test compose file.

---

## Background: What Each Call Site Does Today

`publisher.go` has two dial call sites and one helper:

```go
// amqpURL builds the AMQP connection URL from the config fields.
// The returned string embeds a plaintext password and must never be logged.
func (c PublisherConfig) amqpURL() string {
    return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", c.User, c.Password, c.Host, c.Port, c.VHost)
}

// In NewPublisher:
conn, err := amqp.Dial(cfg.amqpURL())

// In reconnect:
conn, err := amqp.Dial(p.cfg.amqpURL())
```

After M4, both call sites become:

```go
conn, err := amqp.DialConfig(
    fmt.Sprintf("amqp://%s:%d/", cfg.Host, cfg.Port), // no credentials in URL
    amqp.Config{
        SASL:  []amqp.Authentication{&amqp.PlainAuth{Username: cfg.User, Password: cfg.Password}}, // #nosec G101 — value supplied via config, not hardcoded in source
        Vhost: cfg.VHost,
    },
)
```

`amqpURL()` is deleted entirely.

**Why the URL path `/` is safe:** When `config.Vhost` is set to any non-empty string, `DialConfig` uses that value directly in the AMQP connection handshake and the URL path is ignored entirely. The trailing `/` in `amqp://host:port/` is therefore harmless — `amqp://host:port` would behave identically. The invariant is: non-empty `config.Vhost` always wins over the URL path.

---

## Task 1 — Replace amqpURL with SASL dial (M4)

**Files:**
- Modify: `src/internal/queue/rabbitmq/publisher.go`

The change is a drop-in replacement at both call sites. There is no unit-testable behavior change visible from outside (both paths connect to a live broker), so verification uses `make analyze` (linters + gosec) and `make build` (E2E against a real broker).

- [ ] **Step 1: Read `publisher.go` in full before touching it**

  ```bash
  cat src/internal/queue/rabbitmq/publisher.go
  ```
  Confirm the two `amqp.Dial(cfg.amqpURL())` call sites and the `amqpURL()` method.

- [ ] **Step 2: Replace the dial call in `NewPublisher`**

  In `NewPublisher`, change:
  ```go
  conn, err := amqp.Dial(cfg.amqpURL())
  ```
  to:
  ```go
  conn, err := amqp.DialConfig(
      fmt.Sprintf("amqp://%s:%d/", cfg.Host, cfg.Port),
      amqp.Config{
          SASL:  []amqp.Authentication{&amqp.PlainAuth{Username: cfg.User, Password: cfg.Password}}, // #nosec G101 — value supplied via config, not hardcoded in source
          Vhost: cfg.VHost,
      },
  )
  ```

  The `// #nosec G101` comment is **required** on the `SASL` line. `gosec` will flag
  `PlainAuth{..., Password: cfg.Password}` as a potential hardcoded credential (G101)
  at the struct literal site. The existing annotation on the `Password` field declaration
  in `PublisherConfig` does not suppress usage sites. Without this comment `make analyze`
  will fail.

- [ ] **Step 3: Replace the dial call in `reconnect`**

  In `reconnect`, change:
  ```go
  conn, err := amqp.Dial(p.cfg.amqpURL())
  ```
  to:
  ```go
  conn, err := amqp.DialConfig(
      fmt.Sprintf("amqp://%s:%d/", p.cfg.Host, p.cfg.Port),
      amqp.Config{
          SASL:  []amqp.Authentication{&amqp.PlainAuth{Username: p.cfg.User, Password: p.cfg.Password}}, // #nosec G101 — value supplied via config, not hardcoded in source
          Vhost: p.cfg.VHost,
      },
  )
  ```

  Same `// #nosec G101` annotation required here for the same reason.

- [ ] **Step 4: Delete `amqpURL()` and update the godoc**

  Remove the entire `amqpURL()` method (the `// amqpURL builds...` comment through the closing `}`).

  Update the `PublisherConfig` godoc:
  ```go
  // PublisherConfig holds the connection parameters for the RabbitMQ publisher.
  // VHost must be URL-safe; names containing special characters must be percent-encoded
  // by the caller before assignment. Credentials are passed via SASL and never embedded
  // in a URL string.
  ```

- [ ] **Step 5: Build and lint**

  ```bash
  cd src && go build ./internal/queue/...
  ```
  Expected: no output. Then:
  ```bash
  make analyze
  ```
  Expected: exits 0. If `gosec` reports G101 at either `PlainAuth` literal, add the `// #nosec G101` comment to that specific line and rerun.

- [ ] **Step 6: Tidy the module**

  ```bash
  cd src && go mod tidy
  ```
  `amqp091-go` is currently listed as `// indirect` in `go.mod`. After this change it remains a direct import — `go mod tidy` may promote it to a direct dependency (removes the `// indirect` comment). If so, stage `go.mod` and `go.sum` in the next step.

- [ ] **Step 7: Commit**

  Stage all modified files:
  ```bash
  git add src/internal/queue/rabbitmq/publisher.go src/go.mod src/go.sum
  git commit -S -m "M4 - Replace amqpURL with SASL dial; password never in URL string"
  ```

---

## Task 2 — Publish-failure counter (M5/R-001)

**Files:**
- Modify: `src/internal/queue/rabbitmq/publisher.go`
- Create: `src/internal/queue/rabbitmq/export_test.go`
- Create: `src/internal/queue/rabbitmq/publisher_test.go`
- Modify: `src/internal/server/server.go`

### 2a — Add the counter field and wire the meter

- [ ] **Step 1: Extract counter creation to a package-level function**

  In `publisher.go`, add `"go.opentelemetry.io/otel/metric"` to the imports, then add
  this unexported function **before** `NewPublisher`:

  ```go
  // newPublishFailuresCounter creates the publish_failures_total counter instrument.
  // It is a separate function so tests can invoke it directly without a live broker.
  func newPublishFailuresCounter(meter metric.Meter) (metric.Int64Counter, error) {
      return meter.Int64Counter(
          "mnemonic.queue.publish_failures_total",
          metric.WithDescription("Total number of times Publish failed after all retry attempts"),
          metric.WithUnit("{failure}"),
      )
  }
  ```

- [ ] **Step 2: Add `publishFailures` field to `Publisher` and update `NewPublisher`**

  Extend the struct:
  ```go
  type Publisher struct {
      cfg             PublisherConfig
      conn            *amqp.Connection
      ch              *amqp.Channel
      mu              sync.Mutex
      publishFailures metric.Int64Counter
  }
  ```

  Update `NewPublisher` signature and body — create the counter first, dial second:
  ```go
  // NewPublisher dials the broker, opens a channel, and declares the destination
  // queue. It returns an error if any step fails, cleaning up partial resources
  // before returning. The meter is used to record publish failure counts.
  func NewPublisher(cfg PublisherConfig, meter metric.Meter) (queue.Publisher, error) {
      counter, err := newPublishFailuresCounter(meter)
      if err != nil {
          return nil, fmt.Errorf("rabbitmq: create publish failures counter: %w", err)
      }

      conn, err := amqp.DialConfig(
          fmt.Sprintf("amqp://%s:%d/", cfg.Host, cfg.Port),
          amqp.Config{
              SASL:  []amqp.Authentication{&amqp.PlainAuth{Username: cfg.User, Password: cfg.Password}}, // #nosec G101 — value supplied via config, not hardcoded in source
              Vhost: cfg.VHost,
          },
      )
      if err != nil {
          return nil, fmt.Errorf("rabbitmq: dial: %w", err)
      }

      ch, err := conn.Channel()
      if err != nil {
          _ = conn.Close()
          return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
      }

      if _, err = ch.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
          _ = ch.Close()
          _ = conn.Close()
          return nil, fmt.Errorf("rabbitmq: declare queue %q: %w", cfg.Queue, err)
      }

      return &Publisher{cfg: cfg, conn: conn, ch: ch, publishFailures: counter}, nil
  }
  ```

- [ ] **Step 3: Increment the counter in `Publish` on terminal failure**

  In `Publish`, update the two failure-return paths:

  ```go
  if err = p.ch.PublishWithContext(ctx, "", p.cfg.Queue, false, false, msg); err != nil {
      if reconnErr := p.reconnect(ctx); reconnErr != nil {
          p.publishFailures.Add(ctx, 1)
          return fmt.Errorf("rabbitmq: publish failed and reconnect failed: %w", errors.Join(err, reconnErr))
      }
      if retryErr := p.ch.PublishWithContext(ctx, "", p.cfg.Queue, false, false, msg); retryErr != nil {
          p.publishFailures.Add(ctx, 1)
          return fmt.Errorf("rabbitmq: publish retry: %w", retryErr)
      }
  }
  ```

  Only terminal failures (where all retry attempts are exhausted) increment the counter.
  A transient error that succeeds on retry does NOT increment.

- [ ] **Step 4: Build the package**

  ```bash
  cd src && go build ./internal/queue/...
  ```
  Expected: no output. Fix any compile errors before continuing.

### 2b — Update the call site in `server.go`

- [ ] **Step 5: Pass the meter to `NewPublisher` in `wireDependencies`**

  `wireDependencies` currently has no access to the meter. The meter comes from
  `telemetry.Telemetry`, which is initialized before `wireDependencies` is called.
  There is exactly one call site for `wireDependencies` in `ListenAndServe`.

  Update the `wireDependencies` signature to accept a meter:
  ```go
  func wireDependencies(
      pgPool *pgxpool.Pool,
      neo4jDriver neo4j.DriverWithContext,
      cfg *config.MnemonicConfig,
      logger zerolog.Logger,
      meter metric.Meter,
  ) (Services, mcpserver.ToolDependencies, queue.Publisher, error) {
  ```

  Update the `NewPublisher` call inside `wireDependencies`:
  ```go
  pub, err := rabbitmq.NewPublisher(rabbitmq.PublisherConfig{
      Host:           cfg.Queue.RabbitMQ.Host,
      Port:           cfg.Queue.RabbitMQ.Port,
      User:           cfg.Queue.RabbitMQ.User,
      Password:       cfg.Queue.RabbitMQ.Password,
      VHost:          cfg.Queue.RabbitMQ.VHost,
      Queue:          cfg.Queue.RabbitMQ.Queue,
      ReconnectDelay: cfg.Queue.RabbitMQ.ReconnectDelay,
  }, meter)
  ```

  Update the call site in `ListenAndServe`:
  ```go
  svc, toolDeps, pub, err := wireDependencies(pgPool, neo4jDriver, cfg, logger, tel.Meter("mnemonic/queue"))
  ```

  Add `"go.opentelemetry.io/otel/metric"` to `server.go`'s imports if `goimports` does
  not add it automatically.

- [ ] **Step 6: Build both packages**

  ```bash
  cd src && go build ./internal/server/... ./internal/queue/...
  ```
  Expected: no output.

### 2c — Write tests

The test strategy follows the pattern established in `src/internal/metrics/database_test.go`:
create the instrument, call a record method once (so the OTel ManualReader has data),
then Collect and assert the metric name is present. OTel's ManualReader only exports
metrics that have had at least one observation, so a test that never calls `Add` would
silently pass even if the counter was never created.

- [ ] **Step 7: Create `export_test.go` to expose the counter constructor**

  Create `src/internal/queue/rabbitmq/export_test.go`:

  ```go
  package rabbitmq

  // NewPublishFailuresCounter exposes newPublishFailuresCounter for unit testing.
  var NewPublishFailuresCounter = newPublishFailuresCounter
  ```

  This file is in `package rabbitmq` (not `rabbitmq_test`) so it has access to the
  unexported function. The `_test.go` suffix means it is only compiled during `go test`.

- [ ] **Step 8: Create `publisher_test.go` with a meaningful counter test**

  Create `src/internal/queue/rabbitmq/publisher_test.go`:

  ```go
  package rabbitmq_test

  import (
      "context"
      "testing"

      "github.com/stretchr/testify/assert"
      "github.com/stretchr/testify/require"
      sdkmetric "go.opentelemetry.io/otel/sdk/metric"
      "go.opentelemetry.io/otel/sdk/metric/metricdata"

      "github.com/twistingmercury/mnemonic-api/internal/queue/rabbitmq"
  )

  // TestPublishFailuresCounter verifies that newPublishFailuresCounter creates a
  // working Int64Counter that is observable via the OTel ManualReader.
  // It calls Add once so the ManualReader actually has data to collect.
  func TestPublishFailuresCounter(t *testing.T) {
      reader := sdkmetric.NewManualReader()
      provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
      meter := provider.Meter("test")

      counter, err := rabbitmq.NewPublishFailuresCounter(meter)
      require.NoError(t, err)
      require.NotNil(t, counter)

      // Increment once so the ManualReader has an observation to export.
      ctx := context.Background()
      counter.Add(ctx, 1)

      var data metricdata.ResourceMetrics
      err = reader.Collect(ctx, &data)
      require.NoError(t, err)

      assert.NotEmpty(t, data.ScopeMetrics)
      found := false
      for _, sm := range data.ScopeMetrics {
          for _, m := range sm.Metrics {
              if m.Name == "mnemonic.queue.publish_failures_total" {
                  found = true
              }
          }
      }
      assert.True(t, found, "publish_failures_total metric should be recorded")
  }
  ```

- [ ] **Step 9: Run the new test**

  ```bash
  cd src && go test ./internal/queue/rabbitmq/... -v -run TestPublishFailuresCounter
  ```
  Expected: PASS with `found == true`.

- [ ] **Step 10: Run all unit tests**

  ```bash
  cd src && go test ./...
  ```
  Expected: all pass.

### 2d — Lint, verify, and commit

- [ ] **Step 11: Run linters**

  ```bash
  make analyze
  ```
  Expected: exits 0. Fix any gosec or golangci-lint findings before continuing.

- [ ] **Step 12: Full build**

  ```bash
  make build
  ```
  Expected: exits 0 (Docker image built, E2E tests passed against a real broker).

- [ ] **Step 13: Commit**

  ```bash
  git add src/internal/queue/rabbitmq/publisher.go \
          src/internal/queue/rabbitmq/export_test.go \
          src/internal/queue/rabbitmq/publisher_test.go \
          src/internal/server/server.go
  git commit -S -m "M5/R-001 - Add publish_failures_total counter to Publisher"
  ```

---

## Definition of Done

- `make analyze` exits 0.
- `make build` exits 0.
- `grep -r "amqpURL\|amqp\.Dial(" src/internal/queue/rabbitmq/publisher.go` returns no matches.
- `grep "publish_failures_total" src/internal/queue/rabbitmq/publisher.go` returns a match.
- `grep "mnemonic/queue" src/internal/server/server.go` returns a match.
- `go test ./internal/queue/rabbitmq/... -v -run TestPublishFailuresCounter` passes with `found == true`.

---

## Risks

| Risk | Mitigation |
|------|------------|
| `gosec` flags `PlainAuth{Password: cfg.Password}` as G101 at the struct literal | Both `amqp.DialConfig` call sites carry `// #nosec G101 — value supplied via config, not hardcoded in source` on the SASL line. Steps 2 and 3 of Task 1 make this explicit. `make analyze` is run before `make build` to catch this before the Docker build. |
| `amqp.DialConfig` with `amqp.Config{Vhost: cfg.VHost}` behaves differently from `amqp.Dial("amqp://.../%2F")` for the default vhost | When `config.Vhost` is any non-empty string, `DialConfig` uses it directly in the AMQP handshake and ignores the URL path entirely. The invariant is: non-empty `config.Vhost` always takes precedence. The default VHost `"/"` is passed as-is without URL-encoding — this is correct for the AMQP protocol. E2E tests in `make build` catch any regression. |
| `NewPublisher` signature change breaks the compiler | The only call site is `wireDependencies` in `server.go`. Step 5 of Task 2 updates it. `go build ./internal/server/...` in Step 6 confirms the fix. |
| OTel counter creation failure blocks startup | `NewPublisher` propagates the error to `wireDependencies` → `ListenAndServe`. This is correct: a misconfigured meter should surface at startup. |
| `amqp091-go` `// indirect` annotation in `go.mod` becomes stale | `go mod tidy` in Task 1 Step 6 corrects the annotation. |
