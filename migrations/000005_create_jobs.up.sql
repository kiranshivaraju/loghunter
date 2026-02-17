CREATE TABLE jobs (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL REFERENCES tenants(id),
    type           VARCHAR(64)  NOT NULL DEFAULT 'analysis',
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending','running','completed','failed')),
    cluster_id     UUID         REFERENCES error_clusters(id),
    error_message  TEXT,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_tenant_status ON jobs(tenant_id, status, created_at DESC);
CREATE INDEX idx_jobs_cluster_id    ON jobs(cluster_id);
