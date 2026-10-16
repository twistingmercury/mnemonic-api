// Package middleware provides Gin middleware for request completion logging.
// It writes one bounded, redacted summary per request, correlated with the
// OpenTelemetry span that otelgin starts, with configurable path exclusions.
//
// Documentation:
//   - Architecture: docs/architecture/07-observability-architecture.md (Metrics (Prometheus) > Application Metrics)
//   - Architecture: docs/architecture/07-observability-architecture.md (Distributed Tracing (Jaeger) > Trace Structure)
//   - Design: docs/design/observability-implementation.md (Request Metrics Middleware, Tracing Middleware)
package middleware
