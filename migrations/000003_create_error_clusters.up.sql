CREATE TABLE error_clusters (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID         NOT NULL REFERENCES tenants(id),
    service        VARCHAR(255) NOT NULL,
    namespace      VARCHAR(255) NOT NULL DEFAULT 'default',
    fingerprint    VARCHAR(64)  NOT NULL,
    level          VARCHAR(16)  NOT NULL CHECK (level IN ('ERROR','WARN','FATAL','CRITICAL')),
    first_seen_at  TIMESTAMPTZ  NOT NULL,
    last_seen_at   TIMESTAMPTZ  NOT NULL,
    count          INTEGER      NOT NULL DEFAULT 1,
    sample_message TEXT         NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_error_clusters_tenant_service  ON error_clusters(tenant_id, service, last_seen_at DESC);
CREATE INDEX idx_error_clusters_fingerprint     ON error_clusters(tenant_id, fingerprint, first_seen_at DESC);
CREATE INDEX idx_error_clusters_level           ON error_clusters(tenant_id, level);
CREATE INDEX idx_error_clusters_time            ON error_clusters(first_seen_at, last_seen_at);
