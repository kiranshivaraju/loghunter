ALTER TABLE error_clusters
    ADD CONSTRAINT uq_error_clusters_tenant_svc_ns_fp
    UNIQUE (tenant_id, service, namespace, fingerprint);
