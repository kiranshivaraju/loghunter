CREATE TABLE watcher_findings (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL REFERENCES tenants(id),
    cluster_id         UUID        NOT NULL REFERENCES error_clusters(id),
    service            VARCHAR(255) NOT NULL,
    namespace          VARCHAR(255) NOT NULL,
    kind               VARCHAR(16)  NOT NULL CHECK (kind IN ('new', 'spike')),
    current_count      INTEGER      NOT NULL,
    prev_count         INTEGER      NOT NULL DEFAULT 0,
    analysis_triggered BOOLEAN      NOT NULL DEFAULT FALSE,
    job_id             UUID,
    detected_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watcher_findings_tenant_time ON watcher_findings(tenant_id, detected_at DESC);
