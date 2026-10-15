package enrichmentjob

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twistingmercury/mnemonic-api/internal/repository"
)

// Repository defines data access operations for enrichment jobs.
type Repository interface {
	// Create stores a new enrichment job.
	Create(ctx context.Context, job *Job) error

	// WithTx returns a Repository that executes all operations within db.
	// Use this to enlist the repository in a caller-owned transaction.
	WithTx(db repository.DBTX) Repository
}

// pgxRepository is a PostgreSQL implementation of Repository using pgx.
type pgxRepository struct {
	db repository.DBTX
}

// NewRepository creates a new PostgreSQL-backed Repository.
func NewRepository(db repository.DBTX) Repository {
	return &pgxRepository{db: db}
}

// Create stores a new enrichment job in the database.
// Uses SQL now() for timestamps to ensure consistency with database time.
// Uses COALESCE to handle scheduled_for - if not provided, defaults to now().
func (r *pgxRepository) Create(ctx context.Context, job *Job) error {
	// Generate UUID if not provided
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}

	// Set defaults
	if job.Status == "" {
		job.Status = StatusPending
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = DefaultMaxAttempts
	}

	// Use COALESCE to default scheduled_for to now() if not set
	query := `
		INSERT INTO enrichment_jobs (
			id, pattern_id, chunk_id, status, attempts, max_attempts,
			last_error, scheduled_for, started_at, completed_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, now()), $9, $10, now(), now())
		RETURNING scheduled_for, created_at, updated_at
	`

	// Handle nil scheduled_for for COALESCE
	var scheduledForArg any
	if job.ScheduledFor.IsZero() {
		scheduledForArg = nil
	} else {
		scheduledForArg = job.ScheduledFor
	}

	err := r.db.QueryRow(ctx, query,
		job.ID,
		job.PatternID,
		job.ChunkID,
		string(job.Status),
		job.Attempts,
		job.MaxAttempts,
		job.LastError,
		scheduledForArg,
		job.StartedAt,
		job.CompletedAt,
	).Scan(&job.ScheduledFor, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case repository.PgErrCodeUniqueViolation:
				return ErrJobAlreadyPending
			case repository.PgErrCodeForeignKeyViolation:
				return ErrPatternNotFound
			case repository.PgErrCodeCheckViolation:
				return ErrInvalidJobTarget
			}
		}
		return fmt.Errorf("create enrichment job: %w", err)
	}

	return nil
}

// WithTx returns a Repository that executes all operations within db.
func (r *pgxRepository) WithTx(db repository.DBTX) Repository {
	return &pgxRepository{db: db}
}
