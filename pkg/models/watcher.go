package models

import (
	"time"

	"github.com/google/uuid"
)

// WatcherFinding records a single detection event from the watcher background loop.
type WatcherFinding struct {
	ID                uuid.UUID  `json:"id"`
	TenantID          uuid.UUID  `json:"tenant_id"`
	ClusterID         uuid.UUID  `json:"cluster_id"`
	Service           string     `json:"service"`
	Namespace         string     `json:"namespace"`
	Kind              string     `json:"kind"`
	CurrentCount      int        `json:"current_count"`
	PrevCount         int        `json:"prev_count"`
	AnalysisTriggered bool       `json:"analysis_triggered"`
	JobID             *uuid.UUID `json:"job_id,omitempty"`
	DetectedAt        time.Time  `json:"detected_at"`
}
