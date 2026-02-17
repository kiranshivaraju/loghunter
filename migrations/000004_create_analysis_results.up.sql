CREATE TABLE analysis_results (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id       UUID        NOT NULL REFERENCES error_clusters(id),
    tenant_id        UUID        NOT NULL REFERENCES tenants(id),
    job_id           UUID        NOT NULL UNIQUE,
    provider         VARCHAR(64)  NOT NULL,
    model            VARCHAR(255) NOT NULL,
    root_cause       TEXT         NOT NULL,
    confidence       FLOAT        NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
    summary          TEXT         NOT NULL,
    suggested_action TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_analysis_results_cluster_id ON analysis_results(cluster_id);
CREATE INDEX idx_analysis_results_job_id     ON analysis_results(job_id);
CREATE INDEX idx_analysis_results_tenant_id  ON analysis_results(tenant_id);
