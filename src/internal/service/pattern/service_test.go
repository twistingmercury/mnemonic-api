package pattern_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twistingmercury/mnemonic-api/internal/repository"
	chunkrepo "github.com/twistingmercury/mnemonic-api/internal/repository/chunk"
	enrichmentrepo "github.com/twistingmercury/mnemonic-api/internal/repository/enrichmentjob"
	graphrepo "github.com/twistingmercury/mnemonic-api/internal/repository/graph"
	patternrepo "github.com/twistingmercury/mnemonic-api/internal/repository/pattern"
	"github.com/twistingmercury/mnemonic-api/internal/service"
	patternsvc "github.com/twistingmercury/mnemonic-api/internal/service/pattern"
)

// ---------- Mock: patternrepo.Repository ----------

// mockPatternRepo distinguishes writes issued inside a transaction from writes
// issued against the pool. WithTx hands back a separate mock, so a service that
// mutates through the outer repository finds no expectation and fails the test.
type mockPatternRepo struct {
	mock.Mock
	txRepo *mockPatternRepo
}

// tx returns the repository WithTx will hand back, so a test can set
// expectations on it before the service runs.
func (m *mockPatternRepo) tx() *mockPatternRepo {
	if m.txRepo == nil {
		m.txRepo = &mockPatternRepo{}
	}
	return m.txRepo
}

func (m *mockPatternRepo) Create(ctx context.Context, pattern *patternrepo.Pattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *mockPatternRepo) Get(ctx context.Context, id uuid.UUID) (*patternrepo.Pattern, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*patternrepo.Pattern), args.Error(1)
}

func (m *mockPatternRepo) GetByName(ctx context.Context, name string) (*patternrepo.Pattern, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*patternrepo.Pattern), args.Error(1)
}

func (m *mockPatternRepo) Update(ctx context.Context, pattern *patternrepo.Pattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *mockPatternRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockPatternRepo) List(ctx context.Context, filter patternrepo.Filter, opts repository.ListOptions) ([]*patternrepo.Pattern, int64, error) {
	args := m.Called(ctx, filter, opts)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*patternrepo.Pattern), args.Get(1).(int64), args.Error(2)
}

func (m *mockPatternRepo) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	args := m.Called(ctx, id, embedding)
	return args.Error(0)
}

func (m *mockPatternRepo) UpdateEnrichmentStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string) error {
	args := m.Called(ctx, id, status, errMsg)
	return args.Error(0)
}

func (m *mockPatternRepo) FindSimilar(ctx context.Context, embedding []float32, opts patternrepo.SimilarityOptions) ([]*patternrepo.Match, error) {
	args := m.Called(ctx, embedding, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*patternrepo.Match), args.Error(1)
}

func (m *mockPatternRepo) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockPatternRepo) WithTx(_ repository.DBTX) patternrepo.Repository {
	return m.tx()
}

// ---------- Mock: enrichmentrepo.Repository ----------

type mockEnrichmentRepo struct {
	mock.Mock
	txRepo *mockEnrichmentRepo
}

func (m *mockEnrichmentRepo) Create(ctx context.Context, job *enrichmentrepo.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *mockEnrichmentRepo) Get(ctx context.Context, id uuid.UUID) (*enrichmentrepo.Job, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*enrichmentrepo.Job), args.Error(1)
}

func (m *mockEnrichmentRepo) GetByPatternID(ctx context.Context, patternID uuid.UUID) (*enrichmentrepo.Job, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*enrichmentrepo.Job), args.Error(1)
}

func (m *mockEnrichmentRepo) ClaimPending(ctx context.Context) (*enrichmentrepo.Job, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*enrichmentrepo.Job), args.Error(1)
}

func (m *mockEnrichmentRepo) MarkProcessing(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockEnrichmentRepo) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockEnrichmentRepo) MarkFailed(ctx context.Context, id uuid.UUID, err error, retryDelay time.Duration) error {
	args := m.Called(ctx, id, err, retryDelay)
	return args.Error(0)
}

func (m *mockEnrichmentRepo) ReclaimStale(ctx context.Context, timeout time.Duration) (int64, error) {
	args := m.Called(ctx, timeout)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockEnrichmentRepo) List(ctx context.Context, filter enrichmentrepo.Filter, opts repository.ListOptions) ([]*enrichmentrepo.Job, int64, error) {
	args := m.Called(ctx, filter, opts)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*enrichmentrepo.Job), args.Get(1).(int64), args.Error(2)
}

func (m *mockEnrichmentRepo) DeleteCompleted(ctx context.Context, retention time.Duration) (int64, error) {
	args := m.Called(ctx, retention)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockEnrichmentRepo) DeleteFailed(ctx context.Context, retention time.Duration) (int64, error) {
	args := m.Called(ctx, retention)
	return args.Get(0).(int64), args.Error(1)
}

// ---------- Mock: graphrepo.Repository ----------

func (m *mockEnrichmentRepo) WithTx(_ repository.DBTX) enrichmentrepo.Repository {
	return m.tx()
}

// tx returns the repository WithTx will hand back. See mockPatternRepo.tx.
func (m *mockEnrichmentRepo) tx() *mockEnrichmentRepo {
	if m.txRepo == nil {
		m.txRepo = &mockEnrichmentRepo{}
	}
	return m.txRepo
}

type mockGraphRepo struct {
	mock.Mock
}

func (m *mockGraphRepo) SyncPattern(ctx context.Context, pattern *graphrepo.Pattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *mockGraphRepo) DeletePattern(ctx context.Context, patternID uuid.UUID) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *mockGraphRepo) SyncConcepts(ctx context.Context, patternID uuid.UUID, concepts []graphrepo.Concept) error {
	args := m.Called(ctx, patternID, concepts)
	return args.Error(0)
}

func (m *mockGraphRepo) ComputeRelatedToEdges(ctx context.Context, patternID uuid.UUID, minSimilarity float64) error {
	args := m.Called(ctx, patternID, minSimilarity)
	return args.Error(0)
}

func (m *mockGraphRepo) GetPatternConcepts(ctx context.Context, patternID uuid.UUID) ([]graphrepo.Concept, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]graphrepo.Concept), args.Error(1)
}

func (m *mockGraphRepo) FindRelatedPatterns(ctx context.Context, patternID uuid.UUID, limit int) ([]graphrepo.RelatedPattern, error) {
	args := m.Called(ctx, patternID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]graphrepo.RelatedPattern), args.Error(1)
}

func (m *mockGraphRepo) CleanupOrphanedConcepts(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockGraphRepo) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// ---------- Mock: repository.TxBeginner ----------

type mockTxBeginner struct {
	mock.Mock
}

func (m *mockTxBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Tx), args.Error(1)
}

// ---------- Mock: pgx.Tx ----------

// mockPgxTx implements pgx.Tx for unit-testing the transaction lifecycle
// (Begin/Commit/Rollback) inside the service layer. The DBTX methods (Exec,
// Query, QueryRow) are never invoked during service-layer tests because mock
// repository implementations intercept all data-access calls before they
// reach the underlying connection; those methods therefore panic to catch
// accidental use.
type mockPgxTx struct {
	mock.Mock
}

func (m *mockPgxTx) Begin(ctx context.Context) (pgx.Tx, error) {
	panic("mockPgxTx.Begin called unexpectedly")
}

func (m *mockPgxTx) Commit(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPgxTx) Rollback(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPgxTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	panic("mockPgxTx.Exec called unexpectedly")
}

func (m *mockPgxTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	panic("mockPgxTx.Query called unexpectedly")
}

func (m *mockPgxTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	panic("mockPgxTx.QueryRow called unexpectedly")
}

func (m *mockPgxTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	panic("mockPgxTx.CopyFrom called unexpectedly")
}

func (m *mockPgxTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	panic("mockPgxTx.SendBatch called unexpectedly")
}

func (m *mockPgxTx) LargeObjects() pgx.LargeObjects {
	panic("mockPgxTx.LargeObjects called unexpectedly")
}

func (m *mockPgxTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	panic("mockPgxTx.Prepare called unexpectedly")
}

func (m *mockPgxTx) Conn() *pgx.Conn {
	panic("mockPgxTx.Conn called unexpectedly")
}

// ---------- Mock: chunkrepo.Repository ----------

type mockChunkRepo struct {
	mock.Mock
	txRepo *mockChunkRepo
}

func (m *mockChunkRepo) Create(ctx context.Context, c *chunkrepo.Chunk) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}

func (m *mockChunkRepo) CreateBatch(ctx context.Context, chunks []*chunkrepo.Chunk) error {
	args := m.Called(ctx, chunks)
	// Assign IDs to chunks so that enrichment job creation can use them.
	if args.Error(0) == nil {
		for _, c := range chunks {
			if c.ID == (uuid.UUID{}) {
				c.ID = uuid.New()
			}
		}
	}
	return args.Error(0)
}

func (m *mockChunkRepo) Get(ctx context.Context, id uuid.UUID) (*chunkrepo.Chunk, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*chunkrepo.Chunk), args.Error(1)
}

func (m *mockChunkRepo) ListByPatternID(ctx context.Context, patternID uuid.UUID) ([]*chunkrepo.Chunk, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*chunkrepo.Chunk), args.Error(1)
}

func (m *mockChunkRepo) DeleteByPatternID(ctx context.Context, patternID uuid.UUID) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *mockChunkRepo) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	args := m.Called(ctx, id, embedding)
	return args.Error(0)
}

func (m *mockChunkRepo) UpdateEnrichmentStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string) error {
	args := m.Called(ctx, id, status, errMsg)
	return args.Error(0)
}

func (m *mockChunkRepo) FindSimilar(ctx context.Context, embedding []float32, opts chunkrepo.SimilarityOptions) ([]*chunkrepo.Match, error) {
	args := m.Called(ctx, embedding, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*chunkrepo.Match), args.Error(1)
}

func (m *mockChunkRepo) AllEnrichedForPattern(ctx context.Context, patternID uuid.UUID) (bool, error) {
	args := m.Called(ctx, patternID)
	return args.Bool(0), args.Error(1)
}

func (m *mockChunkRepo) AnyFailedForPattern(ctx context.Context, patternID uuid.UUID) (bool, error) {
	args := m.Called(ctx, patternID)
	return args.Bool(0), args.Error(1)
}

func (m *mockChunkRepo) WithTx(_ repository.DBTX) chunkrepo.Repository {
	return m.tx()
}

// tx returns the repository WithTx will hand back. See mockPatternRepo.tx.
func (m *mockChunkRepo) tx() *mockChunkRepo {
	if m.txRepo == nil {
		m.txRepo = &mockChunkRepo{}
	}
	return m.txRepo
}

// ---------- Mock: queue.Publisher ----------

type mockPublisher struct {
	// attemptedIDs records every call, publishedIDs only the successful ones.
	// Without the first, a test cannot distinguish a failed publish from a
	// publish that was never attempted.
	attemptedIDs []uuid.UUID
	publishedIDs []uuid.UUID
	publishErr   error
}

func (m *mockPublisher) Publish(_ context.Context, jobID uuid.UUID) error {
	m.attemptedIDs = append(m.attemptedIDs, jobID)
	if m.publishErr != nil {
		return m.publishErr
	}
	m.publishedIDs = append(m.publishedIDs, jobID)
	return nil
}

func (m *mockPublisher) Close() error { return nil }

// ---------- Helpers ----------

var (
	testPatternID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testRelatedID = uuid.MustParse("44444444-4444-4444-4444-444444444444")
)

func newTestService(
	pr *mockPatternRepo,
	er *mockEnrichmentRepo,
	gr *mockGraphRepo,
	tb *mockTxBeginner,
) patternsvc.Service {
	logger := zerolog.Nop()
	// chunkRepo is wired: exercises the chunk-aware enrichment job path.
	// Plain content (no [//]: pattern decorators) produces no chunks; CreateBatch is not called.
	return patternsvc.New(pr, er, gr, tb, new(mockChunkRepo), &mockPublisher{}, logger)
}

func newTestServiceWithChunkRepo(
	pr *mockPatternRepo,
	er *mockEnrichmentRepo,
	gr *mockGraphRepo,
	tb *mockTxBeginner,
	cr *mockChunkRepo,
) patternsvc.Service {
	logger := zerolog.Nop()
	return patternsvc.New(pr, er, gr, tb, cr, &mockPublisher{}, logger)
}

func newTestServiceWithChunkRepoAndLogger(
	pr *mockPatternRepo,
	er *mockEnrichmentRepo,
	gr *mockGraphRepo,
	tb *mockTxBeginner,
	cr *mockChunkRepo,
	logger zerolog.Logger,
) patternsvc.Service {
	return patternsvc.New(pr, er, gr, tb, cr, &mockPublisher{}, logger)
}

func newTestServiceWithPublisher(
	pr *mockPatternRepo,
	er *mockEnrichmentRepo,
	gr *mockGraphRepo,
	tb *mockTxBeginner,
	cr *mockChunkRepo,
	pub *mockPublisher,
) patternsvc.Service {
	logger := zerolog.Nop()
	return patternsvc.New(pr, er, gr, tb, cr, pub, logger)
}

func testCreateInput() patternsvc.CreateInput {
	desc := "A test pattern"
	return patternsvc.CreateInput{
		Name:        "go-error-handling",
		Description: &desc,
		Content:     "Always handle errors explicitly.",
		Tags:        []string{"golang", "best-practices"},
	}
}

func testUpdateInput() patternsvc.UpdateInput {
	desc := "Updated description"
	return patternsvc.UpdateInput{
		Name:        "go-error-handling-v2",
		Description: &desc,
		Content:     "Updated content.",
		Tags:        []string{"golang", "errors"},
	}
}

func testPattern() *patternrepo.Pattern {
	desc := "A test pattern"
	return &patternrepo.Pattern{
		ID:               testPatternID,
		Name:             "go-error-handling",
		Description:      &desc,
		Content:          "Always handle errors explicitly.",
		Tags:             []string{"golang", "best-practices"},
		EnrichmentStatus: "pending",
	}
}

func enrichedPattern() *patternrepo.Pattern {
	p := testPattern()
	p.EnrichmentStatus = "enriched"
	return p
}

// expectCommittedTx wires a transaction that begins and commits cleanly. The
// deferred Rollback still runs and is a no-op once committed, so it is
// permitted but not required.
func expectCommittedTx(tb *mockTxBeginner) *mockPgxTx {
	tx := new(mockPgxTx)
	tb.On("Begin", mock.Anything).Return(tx, nil)
	tx.On("Commit", mock.Anything).Return(nil)
	tx.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed).Maybe()
	return tx
}

// expectRolledBackTx wires a transaction that begins and must never commit.
// Commit carries no expectation, so a service that commits anyway fails here.
func expectRolledBackTx(tb *mockTxBeginner) *mockPgxTx {
	tx := new(mockPgxTx)
	tb.On("Begin", mock.Anything).Return(tx, nil)
	tx.On("Rollback", mock.Anything).Return(nil)
	return tx
}

// ---------- Create ----------

func TestCreate(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		expectCommittedTx(tb)
		// The write must go through the transaction, not the pool.
		pr.tx().On("Create", mock.Anything, mock.MatchedBy(func(p *patternrepo.Pattern) bool {
			return p.Name == "go-error-handling" && p.Content == "Always handle errors explicitly."
		})).Run(func(args mock.Arguments) {
			p := args.Get(1).(*patternrepo.Pattern)
			p.ID = testPatternID
			p.EnrichmentStatus = "pending"
		}).Return(nil)

		result, err := svc.Create(context.Background(), testCreateInput())

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "go-error-handling", result.Name)
		assert.Equal(t, testPatternID, result.ID)
		assert.Equal(t, "pending", result.EnrichmentStatus)

		pr.tx().AssertExpectations(t)
		tb.AssertExpectations(t)
		gr.AssertExpectations(t)
	})

	t.Run("pattern name conflict returns service.ErrConflict", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		expectRolledBackTx(tb)
		pr.tx().On("Create", mock.Anything, mock.Anything).Return(patternrepo.ErrNameExists)

		result, err := svc.Create(context.Background(), testCreateInput())

		assert.Nil(t, result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrConflict), "expected service.ErrConflict, got: %v", err)

		er.AssertNotCalled(t, "Create")
		er.tx().AssertNotCalled(t, "Create")
		tb.AssertExpectations(t)
	})

	t.Run("begin failure never touches a repository", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		tb.On("Begin", mock.Anything).Return(nil, errors.New("pool exhausted"))

		result, err := svc.Create(context.Background(), testCreateInput())

		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pool exhausted")
		pr.AssertNotCalled(t, "Create")
		pr.tx().AssertNotCalled(t, "Create")
		tb.AssertExpectations(t)
	})
}

// ---------- Get ----------

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)

		result, err := svc.Get(context.Background(), testPatternID)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, testPatternID, result.ID)
		assert.Equal(t, "go-error-handling", result.Name)

		pr.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(nil, patternrepo.ErrNotFound)

		result, err := svc.Get(context.Background(), testPatternID)

		assert.Nil(t, result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrNotFound), "expected service.ErrNotFound, got: %v", err)
	})
}

// ---------- GetWithGraph ----------

func TestGetWithGraph(t *testing.T) {
	t.Parallel()

	t.Run("enriched pattern with graph context", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(enrichedPattern(), nil)

		gr.On("FindRelatedPatterns", mock.Anything, testPatternID, 10).Return([]graphrepo.RelatedPattern{
			{
				ID:             testRelatedID,
				Name:           "go-concurrency",
				SharedConcepts: 2,
				Similarity:     0.75,
				ConceptNames:   []string{"goroutines", "channels"},
			},
		}, nil)

		gr.On("GetPatternConcepts", mock.Anything, testPatternID).Return([]graphrepo.Concept{
			{Name: "error-handling", Type: "practice"},
			{Name: "golang", Type: "technology"},
		}, nil)

		pattern, graphCtx, err := svc.GetWithGraph(context.Background(), testPatternID)

		require.NoError(t, err)
		require.NotNil(t, pattern)
		require.NotNil(t, graphCtx)

		assert.Len(t, graphCtx.RelatedPatterns, 1)
		assert.Equal(t, testRelatedID, graphCtx.RelatedPatterns[0].ID)
		assert.Equal(t, "go-concurrency", graphCtx.RelatedPatterns[0].Name)
		assert.Equal(t, "RELATED_TO", graphCtx.RelatedPatterns[0].Relationship)
		assert.InDelta(t, 0.75, graphCtx.RelatedPatterns[0].Similarity, 0.001)
		assert.Equal(t, []string{"goroutines", "channels"}, graphCtx.RelatedPatterns[0].SharedConcepts)

		assert.Len(t, graphCtx.Concepts, 2)
		assert.Equal(t, "error-handling", graphCtx.Concepts[0].Name)
		assert.Equal(t, "practice", graphCtx.Concepts[0].Type)

		pr.AssertExpectations(t)
		gr.AssertExpectations(t)
	})

	t.Run("pattern not enriched returns nil graph context", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil) // status = "pending"

		pattern, graphCtx, err := svc.GetWithGraph(context.Background(), testPatternID)

		require.NoError(t, err)
		require.NotNil(t, pattern)
		assert.Nil(t, graphCtx)

		gr.AssertNotCalled(t, "FindRelatedPatterns")
		gr.AssertNotCalled(t, "GetPatternConcepts")
	})

	t.Run("neo4j failure returns nil graph context", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(enrichedPattern(), nil)
		gr.On("FindRelatedPatterns", mock.Anything, testPatternID, 10).
			Return(nil, errors.New("neo4j connection refused"))

		pattern, graphCtx, err := svc.GetWithGraph(context.Background(), testPatternID)

		require.NoError(t, err, "neo4j failure should degrade gracefully")
		require.NotNil(t, pattern)
		assert.Nil(t, graphCtx)
	})
}

// ---------- Update ----------

func TestUpdate(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := testUpdateInput()

		// Get existing.
		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)

		// Transaction lifecycle.
		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Commit", mock.Anything).Return(nil)
		tx.On("Rollback", mock.Anything).Return(nil)

		// Update.
		pr.tx().On("Update", mock.Anything, mock.MatchedBy(func(p *patternrepo.Pattern) bool {
			return p.ID == testPatternID && p.Name == "go-error-handling-v2" && p.Content == "Updated content."
		})).Return(nil)

		// Delete stale chunks ("Updated content." has no [//]: pattern sections → 0 chunks).
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)

		// No CreateBatch call (0 chunks).
		// No er.tx().On("Create") call (0 chunks → no per-chunk jobs).

		result, err := svc.Update(context.Background(), testPatternID, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, testPatternID, result.ID)
		assert.Equal(t, "go-error-handling-v2", result.Name)

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
		pr.AssertExpectations(t)
		cr.AssertExpectations(t)
		gr.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Get", mock.Anything, testPatternID).Return(nil, patternrepo.ErrNotFound)

		result, err := svc.Update(context.Background(), testPatternID, testUpdateInput())

		assert.Nil(t, result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrNotFound), "expected service.ErrNotFound, got: %v", err)

		pr.AssertNotCalled(t, "Update")
	})

	t.Run("chunk-aware path deletes stale chunks, creates new chunks, and enqueues per-chunk jobs", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "[//]: pattern\n## Section One\nNew content.\n\n[//]: pattern\n## Section Two\nMore new content.",
			Tags:    []string{"golang", "errors"},
		}

		// Transaction lifecycle.
		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Commit", mock.Anything).Return(nil)
		// Rollback is always called by defer; pgx guarantees it is a no-op
		// after a successful Commit.
		tx.On("Rollback", mock.Anything).Return(nil)

		// Get existing.
		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)

		// Update.
		pr.tx().On("Update", mock.Anything, mock.MatchedBy(func(p *patternrepo.Pattern) bool {
			return p.ID == testPatternID && p.Name == "go-error-handling-v2"
		})).Return(nil)

		// Chunk-aware path: delete stale chunks.
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)

		// Create new chunks (content has 2 PATTERN sections → 2 chunks).
		cr.tx().On("CreateBatch", mock.Anything, mock.MatchedBy(func(chunks []*chunkrepo.Chunk) bool {
			return len(chunks) == 2
		})).Return(nil)

		// Per-chunk enrichment jobs: expect 2 calls, each with a ChunkID set.
		er.tx().On("Create", mock.Anything, mock.MatchedBy(func(j *enrichmentrepo.Job) bool {
			// Status is left unset; enrichmentjob.Create defaults it to pending.
			return j.ChunkID != nil && j.PatternID == nil
		})).Return(nil).Times(2)

		result, err := svc.Update(context.Background(), testPatternID, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, testPatternID, result.ID)
		assert.Equal(t, "go-error-handling-v2", result.Name)

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
		pr.AssertExpectations(t)
		cr.AssertExpectations(t)
		er.AssertExpectations(t)
	})

	t.Run("chunk-aware path: delete stale chunks error rolls back transaction and propagates error", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "## Section\nContent.",
		}

		// Transaction lifecycle: Begin succeeds, Rollback is called by defer.
		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Rollback", mock.Anything).Return(nil)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		pr.tx().On("Update", mock.Anything, mock.Anything).Return(nil)
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).
			Return(errors.New("delete failed"))

		result, err := svc.Update(context.Background(), testPatternID, input)

		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete stale chunks")

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
		tx.AssertNotCalled(t, "Commit")
		cr.AssertNotCalled(t, "CreateBatch")
		er.AssertNotCalled(t, "Create")
	})

	t.Run("transaction commit path: all steps succeed, Commit called, Rollback is deferred no-op", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "[//]: pattern\n## Section One\nNew content.\n\n[//]: pattern\n## Section Two\nMore new content.",
		}

		// Transaction lifecycle: full commit path.
		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Commit", mock.Anything).Return(nil)
		// Rollback is called by defer even after a successful Commit; pgx
		// makes it a no-op.
		tx.On("Rollback", mock.Anything).Return(nil)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		pr.tx().On("Update", mock.Anything, mock.Anything).Return(nil)
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)
		cr.tx().On("CreateBatch", mock.Anything, mock.MatchedBy(func(chunks []*chunkrepo.Chunk) bool {
			return len(chunks) == 2
		})).Return(nil)

		er.tx().On("Create", mock.Anything, mock.MatchedBy(func(j *enrichmentrepo.Job) bool {
			// Status is left unset; enrichmentjob.Create defaults it to pending.
			return j.ChunkID != nil && j.PatternID == nil
		})).Return(nil).Times(2)

		result, err := svc.Update(context.Background(), testPatternID, input)

		require.NoError(t, err)
		require.NotNil(t, result)

		tx.AssertCalled(t, "Commit", mock.Anything)
		tx.AssertCalled(t, "Rollback", mock.Anything)

		// The writes went through the transaction, not the pool. A service that
		// opened a transaction and then mutated via the original repositories
		// would satisfy the lifecycle assertions above but fail these.
		cr.tx().AssertCalled(t, "CreateBatch", mock.Anything, mock.Anything)
		pr.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		cr.AssertNotCalled(t, "CreateBatch", mock.Anything, mock.Anything)
		cr.AssertNotCalled(t, "DeleteByPatternID", mock.Anything, mock.Anything)
		er.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
		pr.AssertExpectations(t)
		pr.tx().AssertExpectations(t)
		cr.tx().AssertExpectations(t)
		er.tx().AssertExpectations(t)
	})

	t.Run("transaction rollback path: CreateBatch fails, Commit NOT called", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "[//]: pattern\n## Section One\nNew content.",
		}

		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Rollback", mock.Anything).Return(nil)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		pr.tx().On("Update", mock.Anything, mock.Anything).Return(nil)
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)
		cr.tx().On("CreateBatch", mock.Anything, mock.Anything).
			Return(errors.New("db error"))

		result, err := svc.Update(context.Background(), testPatternID, input)

		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chunk batch")

		tx.AssertNotCalled(t, "Commit")
		tx.AssertCalled(t, "Rollback", mock.Anything)

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
	})

	t.Run("transaction rollback path: Update fails, later steps NOT called", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "[//]: pattern\n## Section One\nNew content.",
		}

		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Rollback", mock.Anything).Return(nil)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		pr.tx().On("Update", mock.Anything, mock.Anything).Return(errors.New("update failed"))

		result, err := svc.Update(context.Background(), testPatternID, input)

		assert.Nil(t, result)
		require.Error(t, err)

		tx.AssertNotCalled(t, "Commit")
		tx.AssertCalled(t, "Rollback", mock.Anything)
		cr.AssertNotCalled(t, "DeleteByPatternID")
		cr.AssertNotCalled(t, "CreateBatch")

		tb.AssertExpectations(t)
		tx.AssertExpectations(t)
	})

	t.Run("publishJob called for each chunk after transaction commits", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		tx := new(mockPgxTx)
		pub := &mockPublisher{}
		svc := newTestServiceWithPublisher(pr, er, gr, tb, cr, pub)

		input := patternsvc.UpdateInput{
			Name:    "go-error-handling-v2",
			Content: "[//]: pattern\n## Section One\nNew content.\n\n[//]: pattern\n## Section Two\nMore new content.",
		}

		tb.On("Begin", mock.Anything).Return(tx, nil)
		tx.On("Commit", mock.Anything).Return(nil)
		tx.On("Rollback", mock.Anything).Return(nil)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		pr.tx().On("Update", mock.Anything, mock.Anything).Return(nil)
		cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)
		cr.tx().On("CreateBatch", mock.Anything, mock.MatchedBy(func(chunks []*chunkrepo.Chunk) bool {
			return len(chunks) == 2
		})).Return(nil)

		er.tx().On("Create", mock.Anything, mock.MatchedBy(func(j *enrichmentrepo.Job) bool {
			return j.ChunkID != nil
		})).Run(func(args mock.Arguments) {
			j := args.Get(1).(*enrichmentrepo.Job)
			j.ID = uuid.New()
		}).Return(nil).Times(2)

		result, err := svc.Update(context.Background(), testPatternID, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, pub.publishedIDs, 2, "expected publishJob called once per chunk")
	})
}

// ---------- Chunk enrichment job failure summary warnings ----------

// decoratedContent produces two chunks, and therefore two enrichment jobs.
const decoratedContent = "[//]: pattern\n## Section One\nContent one.\n\n[//]: pattern\n## Section Two\nContent two."

// TestCreate_JobFailureRollsBackEverything replaces an earlier test that
// asserted job-insert failures were swallowed and summarised in a warning.
// Required job rows now live in the same transaction as the pattern and its
// chunks, so a failure there must surface and persist nothing.
func TestCreate_JobFailureRollsBackEverything(t *testing.T) {
	t.Parallel()

	pr := new(mockPatternRepo)
	er := new(mockEnrichmentRepo)
	gr := new(mockGraphRepo)
	tb := new(mockTxBeginner)
	cr := new(mockChunkRepo)
	pub := &mockPublisher{}
	svc := patternsvc.New(pr, er, gr, tb, cr, pub, zerolog.Nop())

	expectRolledBackTx(tb)
	pr.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*patternrepo.Pattern).ID = testPatternID
	}).Return(nil)
	cr.tx().On("CreateBatch", mock.Anything, mock.Anything).Return(nil)
	er.tx().On("Create", mock.Anything, mock.Anything).Return(errors.New("jobs table unavailable"))

	result, err := svc.Create(context.Background(), patternsvc.CreateInput{
		Name:    "go-error-handling",
		Content: decoratedContent,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "jobs table unavailable")
	// Nothing may be published for a transaction that never committed.
	assert.Empty(t, pub.publishedIDs)
	tb.AssertExpectations(t)
	pr.tx().AssertExpectations(t)
}

// TestUpdate_JobFailureRollsBackEverything is the Update counterpart. It
// matters more than the Create case: Update has already deleted the old chunks
// inside this transaction, cascading their jobs away, so committing without
// the new jobs would leave the pattern with no route back to enriched.
func TestUpdate_JobFailureRollsBackEverything(t *testing.T) {
	t.Parallel()

	pr := new(mockPatternRepo)
	er := new(mockEnrichmentRepo)
	gr := new(mockGraphRepo)
	tb := new(mockTxBeginner)
	cr := new(mockChunkRepo)
	pub := &mockPublisher{}
	svc := patternsvc.New(pr, er, gr, tb, cr, pub, zerolog.Nop())

	expectRolledBackTx(tb)
	pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
	pr.tx().On("Update", mock.Anything, mock.Anything).Return(nil)
	cr.tx().On("DeleteByPatternID", mock.Anything, testPatternID).Return(nil)
	cr.tx().On("CreateBatch", mock.Anything, mock.Anything).Return(nil)
	er.tx().On("Create", mock.Anything, mock.Anything).Return(errors.New("jobs table unavailable"))

	result, err := svc.Update(context.Background(), testPatternID, patternsvc.UpdateInput{
		Name:    "go-error-handling-v2",
		Content: decoratedContent,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Empty(t, pub.publishedIDs)
	tb.AssertExpectations(t)
	cr.tx().AssertExpectations(t)
}

// TestCreate_CommitFailurePublishesNothing pins the ordering that makes
// best-effort publication safe: job IDs are collected inside the transaction
// but must not reach the queue unless the commit succeeds, or the enricher
// would claim rows that were rolled back.
func TestCreate_CommitFailurePublishesNothing(t *testing.T) {
	t.Parallel()

	pr := new(mockPatternRepo)
	er := new(mockEnrichmentRepo)
	gr := new(mockGraphRepo)
	tb := new(mockTxBeginner)
	cr := new(mockChunkRepo)
	pub := &mockPublisher{}
	svc := patternsvc.New(pr, er, gr, tb, cr, pub, zerolog.Nop())

	tx := new(mockPgxTx)
	tb.On("Begin", mock.Anything).Return(tx, nil)
	tx.On("Commit", mock.Anything).Return(errors.New("connection lost"))
	tx.On("Rollback", mock.Anything).Return(nil)

	pr.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*patternrepo.Pattern).ID = testPatternID
	}).Return(nil)
	cr.tx().On("CreateBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		for i, c := range args.Get(1).([]*chunkrepo.Chunk) {
			c.ID = uuid.New()
			_ = i
		}
	}).Return(nil)
	er.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*enrichmentrepo.Job).ID = uuid.New()
	}).Return(nil)

	result, err := svc.Create(context.Background(), patternsvc.CreateInput{
		Name:    "go-error-handling",
		Content: decoratedContent,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "connection lost")
	assert.Empty(t, pub.publishedIDs, "a rolled-back transaction must publish nothing")
	tx.AssertExpectations(t)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("happy path with neo4j cleanup", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Delete", mock.Anything, testPatternID).Return(nil)
		gr.On("DeletePattern", mock.Anything, testPatternID).Return(nil)
		gr.On("CleanupOrphanedConcepts", mock.Anything).Return(int64(2), nil)

		err := svc.Delete(context.Background(), testPatternID)

		require.NoError(t, err)

		pr.AssertExpectations(t)
		gr.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Delete", mock.Anything, testPatternID).Return(patternrepo.ErrNotFound)

		err := svc.Delete(context.Background(), testPatternID)

		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrNotFound), "expected service.ErrNotFound, got: %v", err)

		gr.AssertNotCalled(t, "DeletePattern")
		gr.AssertNotCalled(t, "CleanupOrphanedConcepts")
	})
}

// ---------- List ----------

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		patterns := []*patternrepo.Pattern{
			{ID: testPatternID, Name: "pattern-a"},
			{ID: testRelatedID, Name: "pattern-b"},
		}
		filter := patternrepo.Filter{Tags: []string{"golang"}}

		pr.On("List", mock.Anything, filter, repository.ListOptions{
			Offset: 0,
			Limit:  10,
		}).Return(patterns, int64(2), nil)

		result, total, err := svc.List(context.Background(), filter, patternsvc.ListOptions{
			Offset: 0,
			Limit:  10,
		})

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), total)

		pr.AssertExpectations(t)
	})
}

// ---------- FindRelated ----------

func TestFindRelated(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Exists", mock.Anything, testPatternID).Return(true, nil)

		gr.On("FindRelatedPatterns", mock.Anything, testPatternID, 5).Return([]graphrepo.RelatedPattern{
			{
				ID:             testRelatedID,
				Name:           "go-concurrency",
				SharedConcepts: 3,
				Similarity:     0.8,
				ConceptNames:   []string{"goroutines", "channels", "select"},
			},
		}, nil)

		results, err := svc.FindRelated(context.Background(), testPatternID, 5)

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, testRelatedID, results[0].ID)
		assert.Equal(t, "go-concurrency", results[0].Name)
		assert.Equal(t, "RELATED_TO", results[0].Relationship)
		assert.InDelta(t, 0.8, results[0].Similarity, 0.001)
		assert.Equal(t, []string{"goroutines", "channels", "select"}, results[0].SharedConcepts)

		pr.AssertExpectations(t)
		gr.AssertExpectations(t)
	})

	t.Run("pattern not found", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		svc := newTestService(pr, er, gr, tb)

		pr.On("Exists", mock.Anything, testPatternID).Return(false, nil)

		results, err := svc.FindRelated(context.Background(), testPatternID, 5)

		assert.Nil(t, results)
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrNotFound), "expected service.ErrNotFound, got: %v", err)

		gr.AssertNotCalled(t, "FindRelatedPatterns")
	})
}

// ---------- TestCreate_ChunksContent ----------

// TestCreate_ChunksContent follows one create through the whole chain and ties
// the identities together: each chunk gets its own job, and exactly those job
// IDs reach the queue, in order, only after the commit. Checking counts alone
// would not catch a service that published the wrong job for a chunk.
func TestCreate_ChunksContent(t *testing.T) {
	t.Parallel()

	pr := new(mockPatternRepo)
	er := new(mockEnrichmentRepo)
	gr := new(mockGraphRepo)
	tb := new(mockTxBeginner)
	cr := new(mockChunkRepo)
	pub := &mockPublisher{}
	svc := patternsvc.New(pr, er, gr, tb, cr, pub, zerolog.Nop())

	chunkIDs := []uuid.UUID{
		uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}
	jobIDs := []uuid.UUID{
		uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
		uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002"),
	}
	// Records which chunk each job was created for, so the published IDs can be
	// traced back to their chunks rather than merely counted.
	jobForChunk := map[uuid.UUID]uuid.UUID{}

	desc := "A chunked pattern"
	input := patternsvc.CreateInput{
		Name:        "chunked-pattern",
		Description: &desc,
		Content:     decoratedContent,
		Tags:        []string{"test"},
		EntityType:  "go-pattern",
		Language:    "go",
		Domain:      "backend",
	}

	expectCommittedTx(tb)

	pr.tx().On("Create", mock.Anything, mock.MatchedBy(func(p *patternrepo.Pattern) bool {
		return p.Name == "chunked-pattern" && p.EntityType == "go-pattern" && p.Language == "go"
	})).Run(func(args mock.Arguments) {
		p := args.Get(1).(*patternrepo.Pattern)
		p.ID = testPatternID
		p.EnrichmentStatus = "pending"
	}).Return(nil)

	var created []*chunkrepo.Chunk
	cr.tx().On("CreateBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		created = args.Get(1).([]*chunkrepo.Chunk)
		// Stand in for the RETURNING clause that assigns real IDs.
		for i, c := range created {
			c.ID = chunkIDs[i]
		}
	}).Return(nil)

	var jobSeq int
	er.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		job := args.Get(1).(*enrichmentrepo.Job)
		require.NotNil(t, job.ChunkID, "every job must name its chunk")
		job.ID = jobIDs[jobSeq]
		jobForChunk[*job.ChunkID] = job.ID
		jobSeq++
	}).Return(nil).Times(2)

	result, err := svc.Create(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, testPatternID, result.ID)

	require.Len(t, created, 2)
	assert.Equal(t, "Section One", created[0].SectionTitle)
	assert.Equal(t, "Section Two", created[1].SectionTitle)
	for i, c := range created {
		assert.Equal(t, i, c.ChunkIndex)
		assert.Equal(t, testPatternID, c.PatternID)
		assert.NotEmpty(t, c.Content)
	}

	// One job per chunk, each naming its own chunk.
	assert.Equal(t, map[uuid.UUID]uuid.UUID{
		chunkIDs[0]: jobIDs[0],
		chunkIDs[1]: jobIDs[1],
	}, jobForChunk)

	// Exactly those jobs are published, in chunk order, after the commit.
	assert.Equal(t, jobIDs, pub.publishedIDs)

	pr.tx().AssertExpectations(t)
	cr.tx().AssertExpectations(t)
	er.tx().AssertExpectations(t)
	tb.AssertExpectations(t)
}

func TestListChunks(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns chunks for existing pattern", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		expectedChunks := []*chunkrepo.Chunk{
			{ChunkIndex: 0, SectionTitle: "Overview", EnrichmentStatus: "pending"},
			{ChunkIndex: 1, SectionTitle: "Philosophy", EnrichmentStatus: "enriched"},
		}
		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		cr.On("ListByPatternID", mock.Anything, testPatternID).Return(expectedChunks, nil)

		result, err := svc.ListChunks(context.Background(), testPatternID)

		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, 0, result[0].ChunkIndex)
		assert.Equal(t, "Overview", result[0].SectionTitle)
		assert.Equal(t, "pending", result[0].EnrichmentStatus)
		assert.Equal(t, 1, result[1].ChunkIndex)
		assert.Equal(t, "Philosophy", result[1].SectionTitle)

		pr.AssertExpectations(t)
		cr.AssertExpectations(t)
	})

	t.Run("pattern not found returns service.ErrNotFound", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		pr.On("Get", mock.Anything, testPatternID).Return(nil, patternrepo.ErrNotFound)

		result, err := svc.ListChunks(context.Background(), testPatternID)

		assert.Nil(t, result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrNotFound), "expected service.ErrNotFound, got: %v", err)

		cr.AssertNotCalled(t, "ListByPatternID")
	})

	t.Run("returns empty slice when pattern has no chunks", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		cr.On("ListByPatternID", mock.Anything, testPatternID).Return([]*chunkrepo.Chunk{}, nil)

		result, err := svc.ListChunks(context.Background(), testPatternID)

		require.NoError(t, err)
		assert.Empty(t, result)

		pr.AssertExpectations(t)
		cr.AssertExpectations(t)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		pr := new(mockPatternRepo)
		er := new(mockEnrichmentRepo)
		gr := new(mockGraphRepo)
		tb := new(mockTxBeginner)
		cr := new(mockChunkRepo)
		svc := newTestServiceWithChunkRepo(pr, er, gr, tb, cr)

		pr.On("Get", mock.Anything, testPatternID).Return(testPattern(), nil)
		cr.On("ListByPatternID", mock.Anything, testPatternID).Return(nil, errors.New("db error"))

		result, err := svc.ListChunks(context.Background(), testPatternID)

		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")

		pr.AssertExpectations(t)
		cr.AssertExpectations(t)
	})
}

// ---------- PublishJob error ----------

// newTestServiceWithFailingPublisher wires a service with the given chunkRepo
// and a publisher that always fails.
func newTestServiceWithFailingPublisher(
	pr *mockPatternRepo,
	er *mockEnrichmentRepo,
	gr *mockGraphRepo,
	tb *mockTxBeginner,
	cr *mockChunkRepo,
	pub *mockPublisher,
) patternsvc.Service {
	logger := zerolog.Nop()
	return patternsvc.New(pr, er, gr, tb, cr, pub, logger)
}

// TestPublishJobError covers the best-effort half of the design: the rows are
// committed, so a queue failure must not fail the request, but it must leave an
// actionable trace. The earlier version of this test used undecorated content,
// which produces no chunks and therefore no jobs — it asserted an empty publish
// list that would have stayed empty with publication deleted outright.
func TestPublishJobError(t *testing.T) {
	t.Parallel()

	pr := new(mockPatternRepo)
	er := new(mockEnrichmentRepo)
	gr := new(mockGraphRepo)
	tb := new(mockTxBeginner)
	cr := new(mockChunkRepo)
	pub := &mockPublisher{publishErr: errors.New("queue unavailable")}

	var logs bytes.Buffer
	svc := patternsvc.New(pr, er, gr, tb, cr, pub, zerolog.New(&logs))

	jobIDs := []uuid.UUID{
		uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
		uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002"),
	}

	expectCommittedTx(tb)
	pr.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*patternrepo.Pattern).ID = testPatternID
	}).Return(nil)
	cr.tx().On("CreateBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		for i, c := range args.Get(1).([]*chunkrepo.Chunk) {
			c.ID = uuid.New()
			_ = i
		}
	}).Return(nil)
	var seq int
	er.tx().On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args.Get(1).(*enrichmentrepo.Job).ID = jobIDs[seq]
		seq++
	}).Return(nil).Times(2)

	result, err := svc.Create(context.Background(), patternsvc.CreateInput{
		Name:    "chunked-pattern",
		Content: decoratedContent,
	})

	// The write is durable, so the request succeeds.
	require.NoError(t, err)
	require.NotNil(t, result)

	// Publication was attempted for both committed jobs and both failed.
	assert.Equal(t, jobIDs, pub.attemptedIDs)
	assert.Empty(t, pub.publishedIDs)

	// Each failure names the job, its pattern and the cause as structured
	// fields, so the pending row can be located and re-published. A message
	// that only says publication failed is not actionable.
	out := logs.String()
	assert.Equal(t, 2, strings.Count(out, `"level":"warn"`))
	assert.Equal(t, 2, strings.Count(out, `"pattern_id":"`+testPatternID.String()+`"`))
	for _, id := range jobIDs {
		assert.Contains(t, out, `"job_id":"`+id.String()+`"`)
	}
	assert.Contains(t, out, "queue unavailable")
}
