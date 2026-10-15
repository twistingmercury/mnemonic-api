package rabbitmq_test

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

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

// TestHeaderCarrier_RoundTrip verifies that trace context injected into AMQP
// headers can be extracted again, which is what the consumer relies on.
func TestHeaderCarrier_RoundTrip(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "publish")
	defer span.End()

	headers := amqp.Table{}
	prop := propagation.TraceContext{}
	prop.Inject(ctx, rabbitmq.HeaderCarrier(headers))

	require.Contains(t, headers, "traceparent")

	got := trace.SpanContextFromContext(prop.Extract(context.Background(), rabbitmq.HeaderCarrier(headers)))
	assert.Equal(t, span.SpanContext().TraceID(), got.TraceID())
	assert.Equal(t, span.SpanContext().SpanID(), got.SpanID())
}
