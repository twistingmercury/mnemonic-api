//go:build integration

package graph_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twistingmercury/mnemonic-api/internal/repository/graph"
)

const (
	// testIntegrationPatternPrefix is used to identify test-created patterns for cleanup.
	testIntegrationPatternPrefix = "test-integration-pattern-"

	// testIntegrationConceptPrefix is used to identify test-created concepts for cleanup.
	testIntegrationConceptPrefix = "test-integration-concept-"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func neo4jDatabase() string { return envOr("NEO4J_DATABASE", "neo4j") }

// setupNeo4j connects to the test Neo4j instance and returns the driver and a
// graph.Repository, skipping the test if Neo4j is unavailable. Test data is
// removed before and after the test.
func setupNeo4j(t *testing.T) (neo4j.DriverWithContext, graph.Repository) {
	t.Helper()

	uri := envOr("NEO4J_URI", "bolt://localhost:7688")
	user := envOr("NEO4J_USER", "neo4j")
	password := envOr("NEO4J_PASSWORD", "mnemonic_dev")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		t.Skipf("skipping integration test: unable to create Neo4j driver: %v", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		t.Skipf("skipping integration test: Neo4j connectivity check failed: %v", err)
	}

	cleanupNeo4jTestData(t, driver)
	t.Cleanup(func() {
		cleanupNeo4jTestData(t, driver)
		_ = driver.Close(context.Background())
	})

	return driver, graph.NewRepository(driver, neo4jDatabase())
}

// cypher runs a statement directly against the test database.
func cypher(t *testing.T, driver neo4j.DriverWithContext, query string, params map[string]any) []*neo4j.Record {
	t.Helper()
	res, err := neo4j.ExecuteQuery(context.Background(), driver, query, params,
		neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(neo4jDatabase()))
	require.NoError(t, err)
	return res.Records
}

// cleanupNeo4jTestData removes all test-created nodes and relationships.
func cleanupNeo4jTestData(t *testing.T, driver neo4j.DriverWithContext) {
	t.Helper()
	cypher(t, driver, `MATCH (p:Pattern) WHERE p.name STARTS WITH $prefix DETACH DELETE p`,
		map[string]any{"prefix": testIntegrationPatternPrefix})
	cypher(t, driver, `MATCH (c:Concept) WHERE c.name STARTS WITH $prefix DETACH DELETE c`,
		map[string]any{"prefix": testIntegrationConceptPrefix})
}

// seedPattern creates a Pattern node linked to the named concepts, standing in
// for the enricher. Concept names get the test prefix.
func seedPattern(t *testing.T, driver neo4j.DriverWithContext, suffix string, concepts ...string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	names := make([]string, len(concepts))
	for i, c := range concepts {
		names[i] = testIntegrationConceptPrefix + c
	}
	cypher(t, driver,
		`CREATE (p:Pattern {id: $id, name: $name})
		 WITH p
		 UNWIND $concepts AS cname
		 MERGE (c:Concept {name: cname})
		 ON CREATE SET c.type = 'technology'
		 MERGE (c)-[:MENTIONED_IN]->(p)`,
		map[string]any{"id": id.String(), "name": testIntegrationPatternPrefix + suffix, "concepts": names})
	return id
}

// relate creates a RELATED_TO edge between two patterns, as the enricher would.
func relate(t *testing.T, driver neo4j.DriverWithContext, a, b uuid.UUID, similarity float64) {
	t.Helper()
	cypher(t, driver,
		`MATCH (a:Pattern {id: $a}), (b:Pattern {id: $b})
		 CREATE (a)-[:RELATED_TO {similarity: $sim}]->(b)`,
		map[string]any{"a": a.String(), "b": b.String(), "sim": similarity})
}

func count(t *testing.T, driver neo4j.DriverWithContext, query string, params map[string]any) int64 {
	t.Helper()
	recs := cypher(t, driver, query, params)
	require.Len(t, recs, 1)
	n, _, err := neo4j.GetRecordValue[int64](recs[0], "cnt")
	require.NoError(t, err)
	return n
}

func countConceptsByPrefix(t *testing.T, driver neo4j.DriverWithContext, prefix string) int64 {
	return count(t, driver, `MATCH (c:Concept) WHERE c.name STARTS WITH $prefix RETURN count(c) AS cnt`,
		map[string]any{"prefix": prefix})
}

func countConceptRelationships(t *testing.T, driver neo4j.DriverWithContext, patternID uuid.UUID) int64 {
	return count(t, driver, `MATCH (c:Concept)-[:MENTIONED_IN]->(p:Pattern {id: $id}) RETURN count(c) AS cnt`,
		map[string]any{"id": patternID.String()})
}

func patternExists(t *testing.T, driver neo4j.DriverWithContext, patternID uuid.UUID) bool {
	return count(t, driver, `MATCH (p:Pattern {id: $id}) RETURN count(p) AS cnt`,
		map[string]any{"id": patternID.String()}) > 0
}

// --- Tests ---

func TestIntegration_DeletePattern(t *testing.T) {
	driver, repo := setupNeo4j(t)
	ctx := context.Background()

	t.Run("removes pattern and its concept relationships", func(t *testing.T) {
		id := seedPattern(t, driver, "delete-with-concepts", "del-c1", "del-c2")
		require.Equal(t, int64(2), countConceptRelationships(t, driver, id))

		require.NoError(t, repo.DeletePattern(ctx, id))

		assert.False(t, patternExists(t, driver, id))
		assert.Equal(t, int64(0), countConceptRelationships(t, driver, id))
	})

	t.Run("nonexistent pattern is not an error", func(t *testing.T) {
		assert.NoError(t, repo.DeletePattern(ctx, uuid.New()))
	})
}

func TestIntegration_FindRelatedPatterns(t *testing.T) {
	driver, repo := setupNeo4j(t)
	ctx := context.Background()

	// pA shares 3 concepts with pB and 1 with pC; pD shares none.
	pA := seedPattern(t, driver, "related-a", "go", "concurrency", "channels", "error-handling")
	pB := seedPattern(t, driver, "related-b", "go", "concurrency", "channels")
	pC := seedPattern(t, driver, "related-c", "go", "testing")
	pD := seedPattern(t, driver, "related-unique", "python", "django")
	relate(t, driver, pA, pB, 0.75)
	relate(t, driver, pC, pA, 0.25) // edge direction must not matter

	t.Run("orders by similarity with shared concepts", func(t *testing.T) {
		results, err := repo.FindRelatedPatterns(ctx, pA, 10)
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, pB, results[0].ID)
		assert.Equal(t, testIntegrationPatternPrefix+"related-b", results[0].Name)
		assert.Equal(t, 3, results[0].SharedConcepts)
		assert.InDelta(t, 0.75, results[0].Similarity, 0.001)
		assert.ElementsMatch(t, []string{
			testIntegrationConceptPrefix + "go",
			testIntegrationConceptPrefix + "concurrency",
			testIntegrationConceptPrefix + "channels",
		}, results[0].ConceptNames)

		assert.Equal(t, pC, results[1].ID)
		assert.Equal(t, 1, results[1].SharedConcepts)
	})

	t.Run("pattern with no edges has no results", func(t *testing.T) {
		results, err := repo.FindRelatedPatterns(ctx, pD, 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("respects limit", func(t *testing.T) {
		results, err := repo.FindRelatedPatterns(ctx, pA, 1)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, pB, results[0].ID)
	})

	t.Run("nonexistent pattern has no results", func(t *testing.T) {
		results, err := repo.FindRelatedPatterns(ctx, uuid.New(), 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestIntegration_GetPatternConcepts(t *testing.T) {
	driver, repo := setupNeo4j(t)
	ctx := context.Background()

	t.Run("returns concepts ordered by name", func(t *testing.T) {
		id := seedPattern(t, driver, "get-concepts", "gamma", "alpha", "beta")

		concepts, err := repo.GetPatternConcepts(ctx, id)
		require.NoError(t, err)
		require.Len(t, concepts, 3)
		assert.Equal(t, testIntegrationConceptPrefix+"alpha", concepts[0].Name)
		assert.Equal(t, testIntegrationConceptPrefix+"beta", concepts[1].Name)
		assert.Equal(t, testIntegrationConceptPrefix+"gamma", concepts[2].Name)
		for _, c := range concepts {
			assert.Equal(t, "technology", c.Type)
		}
	})

	t.Run("pattern with no concepts", func(t *testing.T) {
		id := seedPattern(t, driver, "get-concepts-empty")

		concepts, err := repo.GetPatternConcepts(ctx, id)
		require.NoError(t, err)
		assert.Empty(t, concepts)
	})

	t.Run("nonexistent pattern", func(t *testing.T) {
		concepts, err := repo.GetPatternConcepts(ctx, uuid.New())
		require.NoError(t, err)
		assert.Empty(t, concepts)
	})
}

func TestIntegration_CleanupOrphanedConcepts(t *testing.T) {
	driver, repo := setupNeo4j(t)
	ctx := context.Background()

	orphanSource := seedPattern(t, driver, "orphan-source", "orphan-a", "orphan-b", "orphan-c")
	seedPattern(t, driver, "active-concepts", "active-x", "active-y")
	require.NoError(t, repo.DeletePattern(ctx, orphanSource))

	deleted, err := repo.CleanupOrphanedConcepts(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(3))

	assert.Equal(t, int64(0), countConceptsByPrefix(t, driver, testIntegrationConceptPrefix+"orphan-"),
		"orphaned concepts should be removed")
	assert.Equal(t, int64(2), countConceptsByPrefix(t, driver, testIntegrationConceptPrefix+"active-"),
		"concepts still mentioned should be kept")

	deleted, err = repo.CleanupOrphanedConcepts(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted, "second run should find nothing")
}

func TestIntegration_ContextCancellation(t *testing.T) {
	_, repo := setupNeo4j(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, repo.DeletePattern(ctx, uuid.New()))

	_, err := repo.FindRelatedPatterns(ctx, uuid.New(), 10)
	assert.Error(t, err)

	_, err = repo.GetPatternConcepts(ctx, uuid.New())
	assert.Error(t, err)

	_, err = repo.CleanupOrphanedConcepts(ctx)
	assert.Error(t, err)
}
