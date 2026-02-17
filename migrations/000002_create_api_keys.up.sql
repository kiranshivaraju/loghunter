CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL REFERENCES tenants(id),
    name         VARCHAR(255) NOT NULL,
    key_hash     VARCHAR(255) NOT NULL,
    key_prefix   VARCHAR(8)   NOT NULL,
    scopes       TEXT[]       NOT NULL DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_api_keys_key_prefix  ON api_keys(key_prefix);
CREATE INDEX idx_api_keys_tenant_id   ON api_keys(tenant_id);
CREATE INDEX idx_api_keys_deleted_at  ON api_keys(deleted_at) WHERE deleted_at IS NULL;
