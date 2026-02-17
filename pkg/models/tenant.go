package models

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents an organization or team. Every other entity belongs to a tenant.
type Tenant struct {
	ID        uuid.UUID `db:"id"          json:"id"`
	Name      string    `db:"name"        json:"name"`
	LokiOrgID string    `db:"loki_org_id" json:"loki_org_id"`
	CreatedAt time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"  json:"updated_at"`
}
