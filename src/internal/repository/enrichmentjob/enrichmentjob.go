package enrichmentjob

import (
	"time"

	"github.com/google/uuid"
)

// Job represents a background enrichment task stored in the database.
type Job struct {
	// ID is the unique identifier for the job.
	ID uuid.UUID `db:"id"`

	// PatternID references the pattern to enrich. Nil for chunk-based jobs.
	PatternID *uuid.UUID `db:"pattern_id"`

	// ChunkID references the chunk to enrich. Nil for legacy pattern-level jobs.
	ChunkID *uuid.UUID `db:"chunk_id"`

	// Status is the job processing state.
	// Valid values: pending, processing, completed, failed
	Status JobStatus `db:"status"`

	// Attempts is the number of times processing has been attempted.
	Attempts int `db:"attempts"`

	// MaxAttempts is the maximum number of retries before giving up.
	MaxAttempts int `db:"max_attempts"`

	// LastError is the error message from the most recent failed attempt.
	LastError *string `db:"last_error"`

	// ScheduledFor is when the job should be processed (supports delayed retry).
	ScheduledFor time.Time `db:"scheduled_for"`

	// StartedAt is the timestamp when processing began.
	StartedAt *time.Time `db:"started_at"`

	// CompletedAt is the timestamp when processing finished successfully.
	CompletedAt *time.Time `db:"completed_at"`

	// CreatedAt is the timestamp when the job was created.
	CreatedAt time.Time `db:"created_at"`

	// UpdatedAt is the timestamp when the job was last modified.
	UpdatedAt time.Time `db:"updated_at"`
}

// JobStatus represents the valid job status values.
type JobStatus string

const (
	// StatusPending indicates the job is awaiting processing.
	StatusPending JobStatus = "pending"

	// StatusProcessing indicates the job is currently being processed by a worker.
	StatusProcessing JobStatus = "processing"

	// StatusCompleted indicates the job finished successfully.
	StatusCompleted JobStatus = "completed"

	// StatusFailed indicates processing failed (see LastError).
	StatusFailed JobStatus = "failed"
)

// DefaultMaxAttempts is the default maximum number of attempts for a job.
const DefaultMaxAttempts = 3
