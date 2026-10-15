package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Repository defines the knowledge-graph operations the API needs. Writing
// concepts and RELATED_TO edges is the enricher's job.
type Repository interface {
	// DeletePattern removes a Pattern node and all its relationships from the graph.
	DeletePattern(ctx context.Context, patternID uuid.UUID) error

	// GetPatternConcepts returns all concepts linked to a pattern via MENTIONED_IN relationships.
	GetPatternConcepts(ctx context.Context, patternID uuid.UUID) ([]Concept, error)

	// FindRelatedPatterns finds patterns related to the given pattern using pre-computed RELATED_TO edges.
	// Results include similarity scores and shared concept names.
	FindRelatedPatterns(ctx context.Context, patternID uuid.UUID, limit int) ([]RelatedPattern, error)

	// CleanupOrphanedConcepts removes concept nodes with no MENTIONED_IN relationships.
	CleanupOrphanedConcepts(ctx context.Context) (int64, error)
}

// neo4jRepository is a Neo4j implementation of Repository.
type neo4jRepository struct {
	driver   neo4j.DriverWithContext
	database string
}

// NewRepository creates a new Neo4j-backed Repository.
func NewRepository(driver neo4j.DriverWithContext, database string) Repository {
	return &neo4jRepository{driver: driver, database: database}
}

var errNilPatternID = errors.New("patternID must not be nil UUID")

// query runs one auto-committed, retried Cypher statement and returns its records.
func (r *neo4jRepository) query(ctx context.Context, write bool, cypher string, params map[string]any) ([]*neo4j.Record, error) {
	routing := neo4j.ExecuteQueryWithReadersRouting()
	if write {
		routing = neo4j.ExecuteQueryWithWritersRouting()
	}
	res, err := neo4j.ExecuteQuery(ctx, r.driver, cypher, params, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(r.database), routing)
	if err != nil {
		return nil, err
	}
	return res.Records, nil
}

// DeletePattern removes a Pattern node and all its relationships from the graph.
func (r *neo4jRepository) DeletePattern(ctx context.Context, patternID uuid.UUID) error {
	if patternID == uuid.Nil {
		return errNilPatternID
	}

	_, err := r.query(ctx, true,
		"MATCH (p:Pattern {id: $patternId}) DETACH DELETE p",
		map[string]any{"patternId": patternID.String()},
	)
	if err != nil {
		return fmt.Errorf("deleting pattern %s: %w", patternID, err)
	}
	return nil
}

// GetPatternConcepts returns all concepts linked to a pattern via MENTIONED_IN relationships.
func (r *neo4jRepository) GetPatternConcepts(ctx context.Context, patternID uuid.UUID) ([]Concept, error) {
	if patternID == uuid.Nil {
		return nil, errNilPatternID
	}

	records, err := r.query(ctx, false,
		`MATCH (c:Concept)-[:MENTIONED_IN]->(p:Pattern {id: $patternId})
		 RETURN c.name AS name, c.type AS type
		 ORDER BY c.name`,
		map[string]any{"patternId": patternID.String()},
	)
	if err != nil {
		return nil, fmt.Errorf("getting concepts for pattern %s: %w", patternID, err)
	}

	concepts := make([]Concept, 0, len(records))
	for _, rec := range records {
		name, _, err := neo4j.GetRecordValue[string](rec, "name")
		if err != nil {
			return nil, fmt.Errorf("getting concepts for pattern %s: %w", patternID, err)
		}
		typ, _, err := neo4j.GetRecordValue[string](rec, "type")
		if err != nil {
			return nil, fmt.Errorf("getting concepts for pattern %s: %w", patternID, err)
		}
		concepts = append(concepts, Concept{Name: name, Type: typ})
	}
	return concepts, nil
}

// FindRelatedPatterns finds patterns related to the given pattern using pre-computed RELATED_TO edges.
// Results include similarity scores and shared concept names.
func (r *neo4jRepository) FindRelatedPatterns(ctx context.Context, patternID uuid.UUID, limit int) ([]RelatedPattern, error) {
	if patternID == uuid.Nil {
		return nil, errNilPatternID
	}

	records, err := r.query(ctx, false,
		`MATCH (p1:Pattern {id: $patternId})-[r:RELATED_TO]-(p2:Pattern)
		 WITH p1, p2, r.similarity AS similarity
		 OPTIONAL MATCH (p1)<-[:MENTIONED_IN]-(c:Concept)-[:MENTIONED_IN]->(p2)
		 WITH p2, similarity, collect(c.name) AS conceptNames, count(c) AS sharedConcepts
		 ORDER BY similarity DESC
		 LIMIT $limit
		 RETURN p2.id AS id, p2.name AS name, sharedConcepts, similarity, conceptNames`,
		map[string]any{"patternId": patternID.String(), "limit": limit},
	)
	if err != nil {
		return nil, fmt.Errorf("finding related patterns for %s: %w", patternID, err)
	}

	patterns := make([]RelatedPattern, 0, len(records))
	for _, rec := range records {
		rp, err := toRelatedPattern(rec)
		if err != nil {
			return nil, fmt.Errorf("finding related patterns for %s: %w", patternID, err)
		}
		patterns = append(patterns, rp)
	}
	return patterns, nil
}

func toRelatedPattern(rec *neo4j.Record) (RelatedPattern, error) {
	idStr, _, err := neo4j.GetRecordValue[string](rec, "id")
	if err != nil {
		return RelatedPattern{}, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return RelatedPattern{}, fmt.Errorf("parsing pattern ID %q: %w", idStr, err)
	}
	name, _, err := neo4j.GetRecordValue[string](rec, "name")
	if err != nil {
		return RelatedPattern{}, err
	}
	shared, _, err := neo4j.GetRecordValue[int64](rec, "sharedConcepts")
	if err != nil {
		return RelatedPattern{}, err
	}
	similarity, _, err := neo4j.GetRecordValue[float64](rec, "similarity")
	if err != nil {
		return RelatedPattern{}, err
	}
	rawNames, _, err := neo4j.GetRecordValue[[]any](rec, "conceptNames")
	if err != nil {
		return RelatedPattern{}, err
	}
	names := make([]string, 0, len(rawNames))
	for _, v := range rawNames {
		s, ok := v.(string)
		if !ok {
			return RelatedPattern{}, fmt.Errorf("unexpected type for concept name element: %T", v)
		}
		names = append(names, s)
	}

	return RelatedPattern{
		ID:             id,
		Name:           name,
		SharedConcepts: int(shared),
		Similarity:     similarity,
		ConceptNames:   names,
	}, nil
}

// CleanupOrphanedConcepts removes concept nodes with no MENTIONED_IN relationships.
func (r *neo4jRepository) CleanupOrphanedConcepts(ctx context.Context) (int64, error) {
	records, err := r.query(ctx, true,
		`MATCH (c:Concept)
		 WHERE NOT (c)-[:MENTIONED_IN]->()
		 DELETE c
		 RETURN count(c) AS deletedCount`,
		nil,
	)
	if err != nil {
		return 0, fmt.Errorf("cleaning up orphaned concepts: %w", err)
	}
	if len(records) != 1 {
		return 0, fmt.Errorf("cleaning up orphaned concepts: expected 1 record, got %d", len(records))
	}
	deleted, _, err := neo4j.GetRecordValue[int64](records[0], "deletedCount")
	if err != nil {
		return 0, fmt.Errorf("cleaning up orphaned concepts: %w", err)
	}
	return deleted, nil
}
