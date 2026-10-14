package enrichmentjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twistingmercury/mnemonic-api/internal/repository/enrichmentjob"
)

// testJob returns a sample enrichment job for testing.
func testJob() *enrichmentjob.Job {
	pid := uuid.New()
	return &enrichmentjob.Job{
		ID:           uuid.New(),
		PatternID:    &pid,
		Status:       enrichmentjob.StatusPending,
		Attempts:     0,
		MaxAttempts:  3,
		ScheduledFor: time.Now(),
	}
}

func TestRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name      string
		job       *enrichmentjob.Job
		setupMock func(mock pgxmock.PgxPoolIface)
		wantErr   error
	}{
		{
			name: "successful creation - pattern-based job",
			job:  testJob(),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"scheduled_for", "created_at", "updated_at"}).
					AddRow(now, now, now)
				mock.ExpectQuery("INSERT INTO enrichment_jobs").
					WithArgs(
						pgxmock.AnyArg(), // id
						pgxmock.AnyArg(), // pattern_id
						pgxmock.AnyArg(), // chunk_id
						"pending",
						0,                // attempts
						3,                // max_attempts
						pgxmock.AnyArg(), // last_error
						pgxmock.AnyArg(), // scheduled_for
						pgxmock.AnyArg(), // started_at
						pgxmock.AnyArg(), // completed_at
					).
					WillReturnRows(rows)
			},
			wantErr: nil,
		},
		{
			name: "successful creation - chunk-based job",
			job: func() *enrichmentjob.Job {
				cid := uuid.New()
				return &enrichmentjob.Job{
					ID:          uuid.New(),
					PatternID:   nil,
					ChunkID:     &cid,
					Status:      enrichmentjob.StatusPending,
					Attempts:    0,
					MaxAttempts: 3,
				}
			}(),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"scheduled_for", "created_at", "updated_at"}).
					AddRow(now, now, now)
				mock.ExpectQuery("INSERT INTO enrichment_jobs").
					WithArgs(
						pgxmock.AnyArg(), // id
						pgxmock.AnyArg(), // pattern_id (nil)
						pgxmock.AnyArg(), // chunk_id
						"pending",
						0,                // attempts
						3,                // max_attempts
						pgxmock.AnyArg(), // last_error
						pgxmock.AnyArg(), // scheduled_for
						pgxmock.AnyArg(), // started_at
						pgxmock.AnyArg(), // completed_at
					).
					WillReturnRows(rows)
			},
			wantErr: nil,
		},
		{
			name: "pattern not found returns ErrPatternNotFound",
			job:  testJob(),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("INSERT INTO enrichment_jobs").
					WithArgs(
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnError(&pgconn.PgError{Code: "23503"})
			},
			wantErr: enrichmentjob.ErrPatternNotFound,
		},
		{
			name: "check violation returns ErrInvalidJobTarget",
			job:  testJob(),
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("INSERT INTO enrichment_jobs").
					WithArgs(
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnError(&pgconn.PgError{Code: "23514"})
			},
			wantErr: enrichmentjob.ErrInvalidJobTarget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.setupMock(mock)

			repo := enrichmentjob.NewRepository(mock)
			err = repo.Create(context.Background(), tt.job)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.False(t, tt.job.CreatedAt.IsZero())
				assert.False(t, tt.job.UpdatedAt.IsZero())
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRepository_Create_GeneratesUUID(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	pid := uuid.New()
	job := &enrichmentjob.Job{
		// ID is not set - should be generated
		PatternID: &pid,
	}

	rows := pgxmock.NewRows([]string{"scheduled_for", "created_at", "updated_at"}).
		AddRow(now, now, now)
	mock.ExpectQuery("INSERT INTO enrichment_jobs").
		WithArgs(
			pgxmock.AnyArg(), // generated id
			pgxmock.AnyArg(), // pattern_id
			pgxmock.AnyArg(), // chunk_id
			"pending",        // default status
			0,                // default attempts
			3,                // default max_attempts
			pgxmock.AnyArg(),
			pgxmock.AnyArg(),
			pgxmock.AnyArg(),
			pgxmock.AnyArg(),
		).
		WillReturnRows(rows)

	repo := enrichmentjob.NewRepository(mock)
	err = repo.Create(context.Background(), job)

	assert.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, job.ID)
	assert.Equal(t, enrichmentjob.StatusPending, job.Status)
	assert.Equal(t, 3, job.MaxAttempts)
	assert.NoError(t, mock.ExpectationsWereMet())
}
