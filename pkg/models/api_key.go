package models

import (
	"time"

	"github.com/google/uuid"
)

// APIKey represents an authentication key for CLI and API access.
// Raw keys are shown once at creation; only the bcrypt hash is stored.
type APIKey struct {
	ID         uuid.UUID  `db:"id"           json:"id"`
	TenantID   uuid.UUID  `db:"tenant_id"    json:"tenant_id"`
	Name       string     `db:"name"         json:"name"`
	KeyHash    string     `db:"key_hash"     json:"-"`
	KeyPrefix  string     `db:"key_prefix"   json:"key_prefix"`
	Scopes     []string   `db:"scopes"       json:"scopes"`
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at,omitempty"`
	DeletedAt  *time.Time `db:"deleted_at"   json:"-"`
	CreatedAt  time.Time  `db:"created_at"   json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"   json:"updated_at"`
}
