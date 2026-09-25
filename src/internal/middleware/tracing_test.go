package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twistingmercury/mnemonic-api/internal/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func init() { gin.SetMode(gin.TestMode) }

func TestTracingMiddleware(t *testing.T) {
	for _, tc := range []struct {
		name, path   string
		custom, skip bool
	}{
		{"regular", "/test", false, false},
		{"health", "/health", false, true},
		{"metrics", "/metrics", false, true},
		{"custom skip", "/custom/skip", true, true},
		{"another custom skip", "/another/skip", true, true},
		{"custom regular", "/api/v1/test", true, false},
		{"custom replaces defaults", "/health", true, false},
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
			router := gin.New()
			if tc.custom {
				router.Use(middleware.TracingMiddlewareWithSkipPaths("test-service", []string{"/custom/skip", "/another/skip"}))
			} else {
				router.Use(middleware.TracingMiddleware("test-service"))
			}
			var active trace.SpanContext
			called := false
			router.GET(tc.path, func(c *gin.Context) {
				called = true
				active = trace.SpanContextFromContext(c.Request.Context())
				c.Status(http.StatusAccepted)
			})
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusAccepted, w.Code)
			require.True(t, called)
			spans := exporter.GetSpans()
			if tc.skip {
				require.Empty(t, spans)
				assert.False(t, active.IsValid())
				return
			}
			require.Len(t, spans, 1)
			assert.True(t, active.IsValid())
			assert.Equal(t, active, spans[0].SpanContext)
			assert.Equal(t, "0123456789abcdef0123456789abcdef", active.TraceID().String())
			assert.Equal(t, "0123456789abcdef", spans[0].Parent.SpanID().String())
			assert.True(t, spans[0].Parent.IsRemote())
			assert.NotEqual(t, spans[0].Parent.SpanID(), active.SpanID())
			assert.Equal(t, trace.SpanKindServer, spans[0].SpanKind)
			assert.Contains(t, spans[0].Name, tc.path)
		})
	}
}
