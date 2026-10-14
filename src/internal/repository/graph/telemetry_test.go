package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/twistingmercury/mnemonic-api/internal/metrics"
	"github.com/twistingmercury/mnemonic-api/internal/repository/graph"
)

// TestRepository_InstrumentsOperations verifies that a failed operation
// produces an errored client span and is counted in the database metrics.
func TestRepository_InstrumentsOperations(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	reader := sdkmetric.NewManualReader()
	dbMetrics, err := metrics.NewDatabase(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
	require.NoError(t, err)

	session := &mockSessionExecutor{
		executeWriteFn: func(_ context.Context, _ func(runner graph.CypherRunner) (any, error)) (any, error) {
			return nil, errors.New("connection closed")
		},
	}
	repo := graph.ExportNewRepositoryWithMetrics(func(_ context.Context) graph.SessionExecutor {
		return session
	}, dbMetrics)

	require.Error(t, repo.DeletePattern(context.Background(), uuid.New()))

	var span sdktrace.ReadOnlySpan
	for _, s := range recorder.Ended() {
		if s.Name() == "neo4j DeletePattern" {
			span = s
		}
	}
	require.NotNil(t, span, "expected a neo4j DeletePattern span")
	assert.Equal(t, codes.Error, span.Status().Code)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	names := map[string]bool{}
	for _, sm := range data.ScopeMetrics {
		for _, m := range sm.Metrics {
			names[m.Name] = true
		}
	}
	assert.True(t, names["mnemonic.db.query_latency"], "latency should be recorded")
	assert.True(t, names["mnemonic.db.query_errors"], "error should be counted")
}
