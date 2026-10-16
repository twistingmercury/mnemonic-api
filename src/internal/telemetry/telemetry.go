package telemetry

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/twistingmercury/mnemonic-api/internal/config"
	"github.com/twistingmercury/mnemonic-api/internal/metrics"
	"github.com/twistingmercury/mnemonic-api/internal/version"
	"github.com/twistingmercury/otelx/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Telemetry wraps the otelx.Telemetry with application-specific helpers.
type Telemetry struct {
	otel            *otelx.Telemetry
	metricsRegistry *metrics.Registry
}

// Initialize creates and configures the telemetry system using otelx.
// It sets up logging, metrics, and tracing based on the provided configuration.
// Returns an error if telemetry initialization fails.
func Initialize(ctx context.Context, cfg *config.MnemonicConfig) (*Telemetry, error) {
	otelCfg, err := buildConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build telemetry config: %w", err)
	}

	tel, err := otelx.Initialize(ctx, otelCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// otelx installs its meter provider globally when metrics are enabled;
	// otherwise the global meter is a no-op.
	registry, err := metrics.NewRegistry(otel.Meter("mnemonic"))
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics registry: %w", err)
	}

	return &Telemetry{
		otel:            tel,
		metricsRegistry: registry,
	}, nil
}

// buildConfig maps the application configuration onto an otelx.Config.
// The log level and tracing endpoint are already checked by config validation.
// Returns an error if the trace exporter cannot be created.
func buildConfig(ctx context.Context, cfg *config.MnemonicConfig) (otelx.Config, error) {
	otelCfg := otelx.Config{
		ServiceName:    version.Service,
		ServiceVersion: version.Version(),
		Environment:    getEnvironment(),
		LogLevel:       cfg.Logging.Level,
	}

	if cfg.Observability.Metrics.Enabled {
		otelCfg.MetricsAddr = fmt.Sprintf(":%d", cfg.Observability.Metrics.Port)
		otelCfg.MetricsPath = cfg.Observability.Metrics.Path
	}

	if cfg.Observability.Tracing.Enabled {
		otelCfg.Tracing = true
		otelCfg.Sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Observability.Tracing.SampleRate))

		// otelx v2 reads the OTLP endpoint from OTEL_EXPORTER_OTLP_* variables.
		// Build the exporter here so the mnemonic config keeps working.
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Observability.Tracing.Endpoint)}
		if cfg.Observability.Tracing.OTLPInsecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err := otlptracegrpc.New(ctx, opts...)
		if err != nil {
			return otelx.Config{}, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}
		otelCfg.TraceExporter = exporter
	}

	return otelCfg, nil
}

// getEnvironment determines the environment from the MNEMONIC_ENV environment variable.
// Returns "development" if the environment variable is not set.
func getEnvironment() string {
	if env := os.Getenv("MNEMONIC_ENV"); env != "" {
		return env
	}
	return "development"
}

// Shutdown gracefully shuts down telemetry, flushing pending data.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return t.otel.Shutdown(ctx)
}

// Logger returns the zerolog logger with trace correlation support.
func (t *Telemetry) Logger() zerolog.Logger {
	return t.otel.Logger
}

// MetricsRegistry returns the metrics registry for handler instrumentation.
func (t *Telemetry) MetricsRegistry() *metrics.Registry {
	return t.metricsRegistry
}

// Meter returns an OpenTelemetry meter for creating metrics.
func (t *Telemetry) Meter(name string) metric.Meter {
	return otel.Meter(name)
}
