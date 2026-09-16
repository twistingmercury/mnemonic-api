package middleware_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twistingmercury/mnemonic-api/internal/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRequestMetrics(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	rm, err := middleware.NewRequestMetrics(provider.Meter("test"))
	require.NoError(t, err)
	require.NotNil(t, rm)
	router := gin.New()
	router.Use(rm.MiddlewareWithSkipPaths([]string{"/health", "/metrics"}))
	collect := func() map[string]metricdata.Metrics {
		var data metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(context.Background(), &data))
		result := map[string]metricdata.Metrics{}
		for _, scope := range data.ScopeMetrics {
			for _, m := range scope.Metrics {
				result[m.Name] = m
			}
		}
		return result
	}
	handler := func(status int) gin.HandlerFunc {
		return func(c *gin.Context) {
			metrics := collect()
			sum := metrics["mnemonic.http.request.in_flight"].Data.(metricdata.Sum[int64])
			require.Len(t, sum.DataPoints, 1)
			assert.EqualValues(t, 1, sum.DataPoints[0].Value)
			c.Status(status)
		}
	}
	router.POST("/items/:id", handler(201))
	router.PUT("/items/:id", handler(200))
	router.DELETE("/items/:id", handler(204))
	router.GET("/failure", handler(503))
	for _, path := range []string{"/health", "/metrics"} {
		router.GET(path, func(c *gin.Context) { c.Status(200) })
	}
	request := func(method, path string, status int) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		assert.Equal(t, status, w.Code)
	}
	request("GET", "/health", 200)
	request("GET", "/metrics", 200)
	assert.Empty(t, collect(), "skip paths must produce zero observations")
	request("POST", "/items/one", 201)
	request("POST", "/items/two", 201)
	request("PUT", "/items/one", 200)
	request("DELETE", "/items/one", 204)
	request("GET", "/failure", 503)
	request("GET", "/not-registered", 404)
	expected := map[attribute.Distinct]int64{}
	for _, tc := range []struct {
		method, route, status string
		count                 int64
	}{
		{"POST", "/items/:id", "201", 2}, {"PUT", "/items/:id", "200", 1}, {"DELETE", "/items/:id", "204", 1}, {"GET", "/failure", "503", 1}, {"GET", "unknown", "404", 1},
	} {
		attrs := attribute.NewSet(attribute.String("http.method", tc.method), attribute.String("http.route", tc.route), attribute.String("http.status_code", tc.status))
		expected[attrs.Equivalent()] = tc.count
	}
	metrics := collect()
	require.Len(t, metrics, 3)
	count := metrics["mnemonic.http.request.count"]
	assert.Equal(t, "{request}", count.Unit)
	sum := count.Data.(metricdata.Sum[int64])
	assert.True(t, sum.IsMonotonic)
	require.Len(t, sum.DataPoints, len(expected))
	for _, point := range sum.DataPoints {
		want, ok := expected[point.Attributes.Equivalent()]
		require.True(t, ok, "unexpected labels: %v", point.Attributes)
		assert.Equal(t, want, point.Value)
	}
	duration := metrics["mnemonic.http.request.duration"]
	assert.Equal(t, "ms", duration.Unit)
	hist := duration.Data.(metricdata.Histogram[float64])
	require.Len(t, hist.DataPoints, len(expected))
	for _, point := range hist.DataPoints {
		want, ok := expected[point.Attributes.Equivalent()]
		require.True(t, ok, "unexpected labels: %v", point.Attributes)
		assert.Equal(t, uint64(want), point.Count)
		assert.GreaterOrEqual(t, point.Sum, float64(0))
		var buckets uint64
		for _, n := range point.BucketCounts {
			buckets += n
		}
		assert.Equal(t, point.Count, buckets)
		assert.Equal(t, []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000}, point.Bounds)
	}
	flight := metrics["mnemonic.http.request.in_flight"]
	assert.Equal(t, "{request}", flight.Unit)
	inflight := flight.Data.(metricdata.Sum[int64])
	assert.False(t, inflight.IsMonotonic)
	require.Len(t, inflight.DataPoints, 1)
	assert.Zero(t, inflight.DataPoints[0].Value)
	assert.Zero(t, inflight.DataPoints[0].Attributes.Len())
}
