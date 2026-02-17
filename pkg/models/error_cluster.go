package models

import (
	"time"

	"github.com/google/uuid"
)

// ErrorCluster represents a deduplicated group of related error log lines
// that share the same normalized fingerprint within a service.
type ErrorCluster struct {
	ID            uuid.UUID `db:"id"             json:"id"`
	TenantID      uuid.UUID `db:"tenant_id"      json:"tenant_id"`
	Service       string    `db:"service"        json:"service"`
	Namespace     string    `db:"namespace"      json:"namespace"`
	Fingerprint   string    `db:"fingerprint"    json:"fingerprint"`
	Level         string    `db:"level"          json:"level"`
	FirstSeenAt   time.Time `db:"first_seen_at"  json:"first_seen_at"`
	LastSeenAt    time.Time `db:"last_seen_at"   json:"last_seen_at"`
	Count         int       `db:"count"          json:"count"`
	SampleMessage string    `db:"sample_message" json:"sample_message"`
	CreatedAt     time.Time `db:"created_at"     json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"     json:"updated_at"`
}
