package vectordb

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twistingmercury/mnemonic-api/internal/config"
)

// NewPostgresPool creates a pgxpool.Pool configured from the provided PostgresConfig.
// The caller is responsible for calling pool.Close() when done.
func NewPostgresPool(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	poolCfg.MaxConns = safeIntToInt32(cfg.MaxOpenConns)
	poolCfg.MinConns = safeIntToInt32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return pool, nil
}

// safeIntToInt32 converts an int to int32 with clamping to prevent overflow.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) // #nosec G115 -- bounds checked above
}
