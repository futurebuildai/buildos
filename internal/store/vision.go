package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VisionVerification represents a row in the vision_verifications table.
type VisionVerification struct {
	ID                uuid.UUID       `json:"id"`
	OrgID             uuid.UUID       `json:"org_id"`
	TaskID            uuid.UUID       `json:"task_id"`
	PhotoURL          string          `json:"photo_url"`
	ExpectedProgress  int             `json:"expected_progress"`
	EstimatedProgress int             `json:"estimated_progress"`
	Confidence        float64         `json:"confidence"`
	Notes             string          `json:"notes"`
	Issues            json.RawMessage `json:"issues"`
	RequiresReview    bool            `json:"requires_review"`
}

// VisionStore provides raw SQL access to the vision_verifications table.
type VisionStore struct {
	pool *pgxpool.Pool
}

// NewVisionStore creates a new VisionStore.
func NewVisionStore(pool *pgxpool.Pool) *VisionStore {
	return &VisionStore{pool: pool}
}

// SaveVerification inserts a new vision verification record.
// All queries are org-scoped via the org_id column.
func (s *VisionStore) SaveVerification(ctx context.Context, v *VisionVerification) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vision_verifications (
			org_id, task_id, photo_url,
			expected_progress, estimated_progress, confidence,
			notes, issues, requires_review
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		v.OrgID, v.TaskID, v.PhotoURL,
		v.ExpectedProgress, v.EstimatedProgress, v.Confidence,
		v.Notes, v.Issues, v.RequiresReview,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("saving vision verification: %w", err)
	}
	return id, nil
}
