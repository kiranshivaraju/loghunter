package models

import (
	"time"

	"github.com/google/uuid"
)

// AnalysisResult holds AI-generated analysis output for a specific error cluster.
type AnalysisResult struct {
	ID              uuid.UUID `db:"id"               json:"id"`
	ClusterID       uuid.UUID `db:"cluster_id"       json:"cluster_id"`
	TenantID        uuid.UUID `db:"tenant_id"        json:"tenant_id"`
	JobID           uuid.UUID `db:"job_id"           json:"job_id"`
	Provider        string    `db:"provider"         json:"provider"`
	Model           string    `db:"model"            json:"model"`
	RootCause       string    `db:"root_cause"       json:"root_cause"`
	Confidence      float64   `db:"confidence"       json:"confidence"`
	Summary         string    `db:"summary"          json:"summary"`
	SuggestedAction *string   `db:"suggested_action" json:"suggested_action,omitempty"`
	CreatedAt       time.Time `db:"created_at"       json:"created_at"`
}
