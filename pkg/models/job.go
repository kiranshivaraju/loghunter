package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
)

// Job tracks async AI inference jobs. The API returns a job_id on POST /api/v1/analyze;
// the client polls GET /api/v1/analyze/{job_id} until status is completed or failed.
type Job struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	TenantID     uuid.UUID  `db:"tenant_id"     json:"tenant_id"`
	Type         string     `db:"type"          json:"type"`
	Status       string     `db:"status"        json:"status"`
	ClusterID    *uuid.UUID `db:"cluster_id"    json:"cluster_id,omitempty"`
	ErrorMessage *string    `db:"error_message" json:"error_message,omitempty"`
	StartedAt    *time.Time `db:"started_at"    json:"started_at,omitempty"`
	CompletedAt  *time.Time `db:"completed_at"  json:"completed_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"    json:"updated_at"`
}
