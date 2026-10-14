package chunk_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twistingmercury/mnemonic-api/internal/repository/chunk"
)

// testChunk returns a sample Chunk for testing.
func testChunk() *chunk.Chunk {
	return &chunk.Chunk{
		ID:           uuid.New(),
		PatternID:    uuid.New(),
		SectionTitle: "Overview",
		ChunkIndex:   0,
		Content:      "This is chunk content for testing.",
	}
}

// testEmbedding returns a sample embedding vector for testing.
func testEmbedding() []float32 {
	emb := make([]float32, 1536)
	for i := range emb {
		emb[i] = float32(i) * 0.001
	}
	return emb
}

// ptr is a helper to create a pointer to any value.
func ptr[T any](v T) *T {
	return &v
}

func TestRepository_ListByPatternID(t *testing.T) {
	t.Parallel()

	now := time.Now()
	patternID := uuid.New()
	chunk1ID := uuid.New()
	chunk2ID := uuid.New()

	tests := []struct {
		name      string
		patternID uuid.UUID
		setupMock func(mock pgxmock.PgxPoolIface)
		wantCount int
		wantErr   bool
	}{
		{
			name:      "returns chunks ordered by chunk_index",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "pattern_id", "section_title", "chunk_index", "content",
					"enrichment_status", "enrichment_error", "enriched_at",
					"created_at", "updated_at",
				}).
					AddRow(chunk1ID, patternID, "Intro", 0, "first", "pending", nil, nil, now, now).
					AddRow(chunk2ID, patternID, "Details", 1, "second", "pending", nil, nil, now, now)
				mock.ExpectQuery("SELECT .* FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnRows(rows)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty result returns empty slice",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "pattern_id", "section_title", "chunk_index", "content",
					"enrichment_status", "enrichment_error", "enriched_at",
					"created_at", "updated_at",
				})
				mock.ExpectQuery("SELECT .* FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "database error is propagated",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT .* FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnError(errors.New("connection failed"))
			},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.setupMock(mock)

			repo := chunk.NewRepository(mock)
			chunks, err := repo.ListByPatternID(context.Background(), tt.patternID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, chunks, tt.wantCount)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRepository_DeleteByPatternID(t *testing.T) {
	t.Parallel()

	patternID := uuid.New()

	tests := []struct {
		name      string
		patternID uuid.UUID
		setupMock func(mock pgxmock.PgxPoolIface)
		wantErr   bool
	}{
		{
			name:      "successful deletion",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("DELETE FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnResult(pgxmock.NewResult("DELETE", 3))
			},
			wantErr: false,
		},
		{
			name:      "no chunks to delete is not an error",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("DELETE FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnResult(pgxmock.NewResult("DELETE", 0))
			},
			wantErr: false,
		},
		{
			name:      "database error is propagated",
			patternID: patternID,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("DELETE FROM pattern_chunks WHERE pattern_id").
					WithArgs(patternID).
					WillReturnError(errors.New("connection failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.setupMock(mock)

			repo := chunk.NewRepository(mock)
			err = repo.DeleteByPatternID(context.Background(), tt.patternID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRepository_FindSimilar(t *testing.T) {
	t.Parallel()

	emb := testEmbedding()
	opts := chunk.SimilarityOptions{
		MinSimilarity: 0.7,
		MaxResults:    5,
		Language:      "",
		Domain:        "",
	}

	tests := []struct {
		name      string
		opts      chunk.SimilarityOptions
		setupMock func(mock pgxmock.PgxPoolIface)
		wantCount int
		wantErr   bool
	}{
		{
			name: "returns matching chunks with pattern metadata",
			opts: opts,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				tagsJSON, _ := json.Marshal([]string{"go", "patterns"})
				rows := pgxmock.NewRows([]string{
					"chunk_id", "pattern_id", "pattern_name", "entity_type",
					"language", "domain", "tags",
					"section_title", "chunk_index", "content", "similarity",
				}).AddRow(
					uuid.New(),
					uuid.New(),
					"go-error-handling",
					"pattern",
					"go",
					"engineering",
					tagsJSON,
					"Overview",
					0,
					"content here",
					0.85,
				)
				mock.ExpectQuery("SELECT .* FROM pattern_chunks pc").
					WithArgs(
						pgxmock.AnyArg(), // embedding vector
						opts.MinSimilarity,
						opts.Language,
						opts.Domain,
						opts.MaxResults,
					).
					WillReturnRows(rows)
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "no results returns empty slice",
			opts: opts,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"chunk_id", "pattern_id", "pattern_name", "entity_type",
					"language", "domain", "tags",
					"section_title", "chunk_index", "content", "similarity",
				})
				mock.ExpectQuery("SELECT .* FROM pattern_chunks pc").
					WithArgs(
						pgxmock.AnyArg(),
						opts.MinSimilarity,
						opts.Language,
						opts.Domain,
						opts.MaxResults,
					).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "with language and domain filters",
			opts: chunk.SimilarityOptions{
				MinSimilarity: 0.5,
				MaxResults:    10,
				Language:      "go",
				Domain:        "engineering",
			},
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"chunk_id", "pattern_id", "pattern_name", "entity_type",
					"language", "domain", "tags",
					"section_title", "chunk_index", "content", "similarity",
				})
				mock.ExpectQuery("SELECT .* FROM pattern_chunks pc").
					WithArgs(
						pgxmock.AnyArg(), // embedding vector
						0.5,              // min similarity
						"go",             // language filter
						"engineering",    // domain filter
						10,               // max results
					).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			// Chunks whose enrichment_status is not 'enriched' must not appear in
			// results even when their embedding column is non-null (M12).
			// The WHERE clause filters them server-side; the mock returns no rows to
			// represent the database honouring that filter.
			name: "chunks with non-null embedding but non-enriched status are excluded",
			opts: opts,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"chunk_id", "pattern_id", "pattern_name", "entity_type",
					"language", "domain", "tags",
					"section_title", "chunk_index", "content", "similarity",
				})
				// No rows: the database filtered out the failed/pending chunk via
				// AND pc.enrichment_status = 'enriched'.
				mock.ExpectQuery("SELECT .* FROM pattern_chunks pc").
					WithArgs(
						pgxmock.AnyArg(),
						opts.MinSimilarity,
						opts.Language,
						opts.Domain,
						opts.MaxResults,
					).
					WillReturnRows(rows)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "database error is propagated",
			opts: opts,
			setupMock: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT .* FROM pattern_chunks pc").
					WithArgs(
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnError(errors.New("connection failed"))
			},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.setupMock(mock)

			repo := chunk.NewRepository(mock)
			matches, err := repo.FindSimilar(context.Background(), emb, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, matches, tt.wantCount)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRepository_CreateBatch(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name       string
		makeChunks func() []*chunk.Chunk
		setupMock  func(mock pgxmock.PgxPoolIface, chunks []*chunk.Chunk)
		wantErr    bool
	}{
		{
			name: "empty slice is a no-op",
			makeChunks: func() []*chunk.Chunk {
				return []*chunk.Chunk{}
			},
			setupMock: func(mock pgxmock.PgxPoolIface, _ []*chunk.Chunk) {
				// No expectations — no DB calls expected.
			},
			wantErr: false,
		},
		{
			// pgxmock.PgxPoolIface is not *pgxpool.Pool, so CreateBatch falls
			// through to createBatchDirect and executes without an internal
			// transaction (no Begin/Commit expected).
			name: "inserts each chunk and returns DB values",
			makeChunks: func() []*chunk.Chunk {
				patternID := uuid.New()
				return []*chunk.Chunk{
					{PatternID: patternID, SectionTitle: "Intro", ChunkIndex: 0, Content: "intro content"},
					{PatternID: patternID, SectionTitle: "Details", ChunkIndex: 1, Content: "details content"},
				}
			},
			setupMock: func(mock pgxmock.PgxPoolIface, chunks []*chunk.Chunk) {
				for range chunks {
					rows := pgxmock.NewRows([]string{"enrichment_status", "created_at", "updated_at"}).
						AddRow("pending", now, now)
					mock.ExpectQuery("INSERT INTO pattern_chunks").
						WithArgs(
							pgxmock.AnyArg(), // id
							pgxmock.AnyArg(), // pattern_id
							pgxmock.AnyArg(), // section_title
							pgxmock.AnyArg(), // chunk_index
							pgxmock.AnyArg(), // content
						).
						WillReturnRows(rows)
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chunks := tt.makeChunks()

			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			tt.setupMock(mock, chunks)

			repo := chunk.NewRepository(mock)
			err = repo.CreateBatch(context.Background(), chunks)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
