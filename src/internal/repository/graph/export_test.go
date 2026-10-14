package graph

import (
	"context"

	"github.com/twistingmercury/mnemonic-api/internal/metrics"
)

// ExportNewRepositoryWithFactory exposes newRepositoryWithFactory for testing.
var ExportNewRepositoryWithFactory = newRepositoryWithFactory

// ExportNewRepositoryWithHealthCheck creates a Repository with a custom SessionFactory
// and health check function for testing.
var ExportNewRepositoryWithHealthCheck = newRepositoryWithFactory

// newRepositoryWithFactory creates a new Repository with a custom SessionFactory
// and an optional health check function. This is used for unit testing with mocked sessions.
func newRepositoryWithFactory(factory SessionFactory, healthCheckFn func(ctx context.Context) error) Repository {
	if healthCheckFn == nil {
		healthCheckFn = func(_ context.Context) error { return nil }
	}
	return &neo4jRepository{
		factory:       factory,
		healthCheckFn: healthCheckFn,
	}
}

// ExportNewRepositoryWithMetrics creates a Repository with a custom
// SessionFactory that records to dbMetrics.
func ExportNewRepositoryWithMetrics(factory SessionFactory, dbMetrics *metrics.Database) Repository {
	return &neo4jRepository{
		factory:       factory,
		healthCheckFn: func(_ context.Context) error { return nil },
		dbMetrics:     dbMetrics,
	}
}
