package graph

import "context"

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
