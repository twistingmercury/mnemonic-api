package enrichmentjob

import "errors"

// Errors returned by CreatePendingForChunks.
var (
	// ErrJobAlreadyPending is returned when a pending enrichment job already exists for this pattern.
	ErrJobAlreadyPending = errors.New("enrichment job already pending for this pattern")

	// ErrInvalidJobTarget is returned when a job does not target exactly one of pattern_id or chunk_id.
	ErrInvalidJobTarget = errors.New("enrichment job must target exactly one of pattern_id or chunk_id")
)
