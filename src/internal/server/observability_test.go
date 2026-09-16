package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twistingmercury/mnemonic-api/internal/config"
	"github.com/twistingmercury/mnemonic-api/internal/handlers"
	"github.com/twistingmercury/mnemonic-api/internal/handlers/patterns"
	"github.com/twistingmercury/mnemonic-api/internal/middleware"
	"github.com/twistingmercury/mnemonic-api/internal/service"
	searchsvc "github.com/twistingmercury/mnemonic-api/internal/service/search"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type failingSearch struct{ err error }

func (f failingSearch) SearchPatterns(context.Context, searchsvc.SearchOptions) (*searchsvc.SearchResult, error) {
	return nil, f.err
}

func TestProductionHTTPObservability(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
		panic  bool
	}{
		{"internal", 500, errors.New("repository sentinel_failure"), false},
		{"unavailable", 503, fmt.Errorf("%w: sentinel_failure: API returned status 502: provider-secret-body", service.ErrServiceUnavailable), false},
		{"panic", 500, nil, true},
		{"escaped quotes", 500, errors.New(`repository sentinel_failure: "value \"private-escaped-marker\" end"`), false},
		{"escaped single quotes", 500, errors.New(`repository sentinel_failure: 'value \'private-escaped-marker\' end'`), false},
		{"truncated escaped quote", 500, errors.New(`repository sentinel_failure: "value \"private-escaped-marker` + strings.Repeat("x", 5000)), false},
		{"unclosed escaped quote", 500, errors.New(`repository sentinel_failure: "value \"private-escaped-marker`), false},
		{"invalid", 400, service.ErrInvalidInput, false},
		{"missing", 404, service.ErrNotFound, false},
		{"conflict", 409, service.ErrConflict, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			oldTP, oldProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
			otel.SetTracerProvider(tp)
			otel.SetTextMapPropagator(propagation.TraceContext{})
			t.Cleanup(func() {
				otel.SetTracerProvider(oldTP)
				otel.SetTextMapPropagator(oldProp)
				require.NoError(t, tp.Shutdown(context.Background()))
			})
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { require.NoError(t, mp.Shutdown(context.Background())) })
			rm, err := middleware.NewRequestMetrics(mp.Meter("test"))
			require.NoError(t, err)
			var logs bytes.Buffer
			router := setupRouter(zerolog.New(&logs), rm)
			var active trace.SpanContext
			router.Use(func(c *gin.Context) { active = trace.SpanContextFromContext(c.Request.Context()); c.Next() })
			if tc.panic {
				router.GET("/v1/api/patterns/search", func(*gin.Context) { panic("sentinel_failure password=private-panic-secret") })
			} else {
				h := patterns.New(nil, failingSearch{tc.err}, config.VocabularyConfig{})
				h.RegisterRoutes(router.Group("/v1/api"))
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/api/patterns/search?q=private-query-secret", strings.NewReader("private-body-secret"))
			req.Header.Set("Authorization", "Bearer private-header-secret")
			req.Header.Set("X-Request-ID", "request-distinct-123")
			req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "request-distinct-123", w.Header().Get("X-Request-ID"))
			var problem handlers.ProblemDetail
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
			assert.Equal(t, tc.status, problem.Status)
			assert.Equal(t, req.URL.Path, problem.Instance)
			assert.Equal(t, active.TraceID().String(), problem.TraceID)
			assert.Equal(t, "0123456789abcdef0123456789abcdef", problem.TraceID)
			assert.NotEqual(t, w.Header().Get("X-Request-ID"), problem.TraceID)
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			assert.Equal(t, active, spans[0].SpanContext)
			assert.Equal(t, "0123456789abcdef", spans[0].Parent.SpanID().String())
			assert.True(t, spans[0].Parent.IsRemote())
			var log map[string]any
			lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
			require.Len(t, lines, 1)
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &log))
			assert.Equal(t, "request completed", log["message"])
			assert.Equal(t, float64(tc.status), log["status"])
			assert.Equal(t, problem.TraceID, log["trace_id"])
			assert.Equal(t, active.SpanID().String(), log["span_id"])
			assert.Equal(t, "request-distinct-123", log["request_id"])
			assert.Equal(t, "/v1/api/patterns/search", log["route"])
			if tc.status >= 500 {
				assert.Equal(t, "error", log["level"])
				assert.Contains(t, log["error"], "sentinel_failure")
				assert.Equal(t, codes.Error, spans[0].Status.Code)
				require.Len(t, spans[0].Events, 1)
				assert.Equal(t, "exception", spans[0].Events[0].Name)
				assert.Contains(t, fmt.Sprint(spans[0].Events[0].Attributes), "sentinel_failure")
				assert.NotContains(t, w.Body.String(), "sentinel_failure")
				detail := "an unexpected error occurred"
				if tc.status == 503 {
					detail = "service temporarily unavailable"
				}
				assert.Equal(t, detail, problem.Detail)
			} else {
				assert.Equal(t, "info", log["level"])
				assert.NotContains(t, log, "error")
				assert.NotEqual(t, codes.Error, spans[0].Status.Code)
				assert.Empty(t, spans[0].Events)
			}
			for _, secret := range []string{"private-query-secret", "private-body-secret", "private-header-secret", "private-panic-secret", "provider-secret-body", "private-escaped-marker"} {
				assert.NotContains(t, logs.String(), secret)
				assert.NotContains(t, fmt.Sprint(spans), secret)
				assert.NotContains(t, w.Body.String(), secret)
			}
			var data metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(context.Background(), &data))
			assertCompletionMetrics(t, data, tc.status)
			if tc.panic {
				// Dispatch again through this same router and its pooled Gin contexts.
				router.GET("/ready", func(c *gin.Context) { c.String(http.StatusOK, "ready") })
				healthyReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
				healthyReq.Header.Set("X-Request-ID", "healthy-request-456")
				healthy := httptest.NewRecorder()
				router.ServeHTTP(healthy, healthyReq)
				require.Equal(t, http.StatusOK, healthy.Code)
				assert.Equal(t, "ready", healthy.Body.String())
				assert.Equal(t, "healthy-request-456", healthy.Header().Get("X-Request-ID"))
				allSpans := exporter.GetSpans()
				require.Len(t, allSpans, 2)
				successSpan := allSpans[1]
				assert.True(t, successSpan.SpanContext.IsValid())
				assert.Equal(t, active, successSpan.SpanContext)
				assert.NotEqual(t, spans[0].SpanContext.TraceID(), successSpan.SpanContext.TraceID())
				assert.False(t, successSpan.Parent.IsValid())
				assert.Equal(t, codes.Unset, successSpan.Status.Code)
				assert.Empty(t, successSpan.Status.Description)
				assert.Empty(t, successSpan.Events)
				assert.NotContains(t, fmt.Sprint(successSpan), "sentinel_failure")
				allLines := strings.Split(strings.TrimSpace(logs.String()), "\n")
				require.Len(t, allLines, 2)
				var successLog map[string]any
				require.NoError(t, json.Unmarshal([]byte(allLines[1]), &successLog))
				assert.Equal(t, "info", successLog["level"])
				assert.Equal(t, float64(http.StatusOK), successLog["status"])
				assert.Equal(t, "request completed", successLog["message"])
				assert.Equal(t, "/ready", successLog["route"])
				assert.Equal(t, "healthy-request-456", successLog["request_id"])
				assert.Equal(t, active.TraceID().String(), successLog["trace_id"])
				assert.Equal(t, active.SpanID().String(), successLog["span_id"])
				assert.NotContains(t, successLog, "error")
				assert.NotContains(t, allLines[1], "sentinel_failure")
				var after metricdata.ResourceMetrics
				require.NoError(t, reader.Collect(context.Background(), &after))
				assertCompletionMetrics(t, after, http.StatusInternalServerError, "/ready")
			}
		})
	}
}

func assertCompletionMetrics(t *testing.T, data metricdata.ResourceMetrics, status int, successRoutes ...string) {
	t.Helper()
	expected := attribute.NewSet(attribute.String("http.method", "GET"), attribute.String("http.route", "/v1/api/patterns/search"), attribute.String("http.status_code", fmt.Sprint(status)))
	expectedSeries := map[attribute.Distinct]bool{expected.Equivalent(): true}
	for _, route := range successRoutes {
		labels := attribute.NewSet(attribute.String("http.method", "GET"), attribute.String("http.route", route), attribute.String("http.status_code", "200"))
		expectedSeries[labels.Equivalent()] = true
	}
	found := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			found[m.Name] = true
			switch m.Name {
			case "mnemonic.http.request.count":
				assert.Equal(t, "{request}", m.Unit)
				sum := m.Data.(metricdata.Sum[int64])
				require.Len(t, sum.DataPoints, len(expectedSeries))
				for _, point := range sum.DataPoints {
					assert.True(t, expectedSeries[point.Attributes.Equivalent()], "unexpected count series: %v", point.Attributes)
					assert.EqualValues(t, 1, point.Value)
				}
			case "mnemonic.http.request.duration":
				assert.Equal(t, "ms", m.Unit)
				hist := m.Data.(metricdata.Histogram[float64])
				require.Len(t, hist.DataPoints, len(expectedSeries))
				for _, point := range hist.DataPoints {
					assert.True(t, expectedSeries[point.Attributes.Equivalent()], "unexpected duration series: %v", point.Attributes)
					assert.EqualValues(t, 1, point.Count)
					assert.GreaterOrEqual(t, point.Sum, float64(0))
				}
			case "mnemonic.http.request.in_flight":
				sum := m.Data.(metricdata.Sum[int64])
				require.Len(t, sum.DataPoints, 1)
				assert.Zero(t, sum.DataPoints[0].Value)
			}
		}
	}
	assert.Len(t, found, 3)
}

func TestProductionHTTPNoTraceAndSkips(t *testing.T) {
	oldTP := otel.GetTracerProvider()
	// A noop provider supplies no locally active span or invented trace identity.
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(oldTP) })
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, mp.Shutdown(context.Background())) })
	rm, err := middleware.NewRequestMetrics(mp.Meter("test"))
	require.NoError(t, err)
	var logs bytes.Buffer
	router := setupRouter(zerolog.New(&logs), rm)
	for _, path := range []string{"/health", "/metrics"} {
		router.GET(path, func(c *gin.Context) { c.Status(200) })
	}
	for _, path := range []string{"/health", "/metrics"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		assert.Equal(t, 200, w.Code)
	}
	assert.Empty(t, logs.String())
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	assert.Empty(t, data.ScopeMetrics)
	router.GET("/failure", func(c *gin.Context) { handlers.RespondError(c, errors.New("failure")) })
	req := httptest.NewRequest("GET", "/failure", nil)
	req.Header.Set("X-Request-ID", strings.Repeat("x", 129))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var problem handlers.ProblemDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Empty(t, problem.TraceID)
	assert.NotContains(t, w.Body.String(), "traceId")
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.Len(t, w.Header().Get("X-Request-ID"), 36)
	assert.NotContains(t, logs.String(), "trace_id")
}
