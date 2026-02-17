# Data Architecture

## Database Choice

**Primary Database:** PostgreSQL 15

**Reasoning:** PostgreSQL handles multi-tenant data isolation cleanly via row-level tenant scoping, supports concurrent writes from multiple API server instances, and provides proper indexing on time-series anomaly baseline data. It is well-supported by the Go ecosystem (`pgx` driver) and integrates naturally with `golang-migrate` for schema management.

**Important note:** LogHunter does NOT store raw log lines in PostgreSQL. Loki is the source of truth for log data. PostgreSQL stores only derived, structured data: analysis results, anomaly baselines, API keys, and configuration. This keeps the database small and avoids replicating Loki's responsibility.

---

## Data Modeling Principles

- Use UUIDs (`uuid_generate_v4()`) for all primary keys
- Every table includes `created_at TIMESTAMPTZ` and `updated_at TIMESTAMPTZ`
- Soft deletes via `deleted_at TIMESTAMPTZ` for user-facing entities (API keys, saved filters)
- Hard deletes for time-bounded operational data (anomaly events older than retention window)
- All tables include a `tenant_id UUID` foreign key — enforced at application layer on every query
- Normalize to 3NF; denormalize only when query performance demands it (with explicit comment explaining why)
- No nullable columns unless the absence of a value is semantically meaningful

---

## Core Entities

### tenants
Represents an organization or team. Maps to a Loki org/namespace scope.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| name | TEXT | Human-readable name |
| loki_org_id | TEXT | Loki `X-Scope-OrgID` header value |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

### api_keys
API keys issued to users/services for CLI and direct API access.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| tenant_id | UUID | FK → tenants |
| name | TEXT | Human label (e.g., "ci-pipeline-key") |
| key_hash | TEXT | bcrypt hash of the raw key |
| scopes | TEXT[] | e.g., `["read:analysis", "write:analysis"]` |
| last_used_at | TIMESTAMPTZ | Nullable |
| deleted_at | TIMESTAMPTZ | Nullable — soft delete = revoked |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

### error_clusters
Deduplicated groups of related error log lines detected in a time window.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| tenant_id | UUID | FK → tenants |
| service | TEXT | Service/app label from Loki |
| namespace | TEXT | Kubernetes namespace or equivalent |
| fingerprint | TEXT | Hash of normalized error message (for deduplication) |
| level | TEXT | ERROR, WARN, FATAL, CRITICAL |
| first_seen_at | TIMESTAMPTZ | Earliest log line in cluster |
| last_seen_at | TIMESTAMPTZ | Latest log line in cluster |
| count | INTEGER | Number of occurrences in window |
| sample_message | TEXT | Representative log line (sanitized) |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

### analysis_results
AI-generated analysis for an error cluster.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| cluster_id | UUID | FK → error_clusters |
| tenant_id | UUID | FK → tenants |
| provider | TEXT | "ollama", "vllm", "openai", "anthropic" |
| model | TEXT | Model name used for this result |
| root_cause | TEXT | AI-generated root cause narrative |
| confidence | FLOAT | 0.0–1.0 |
| summary | TEXT | Plain-language log summary |
| suggested_action | TEXT | Nullable — recommended fix |
| created_at | TIMESTAMPTZ | |

### anomaly_baselines
Rolling statistical baseline per service for anomaly detection.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| tenant_id | UUID | FK → tenants |
| service | TEXT | |
| namespace | TEXT | |
| window_start | TIMESTAMPTZ | Start of baseline window |
| window_end | TIMESTAMPTZ | End of baseline window |
| avg_log_rate | FLOAT | Avg log lines/minute |
| avg_error_rate | FLOAT | Avg error lines/minute |
| p95_error_rate | FLOAT | 95th percentile error rate |
| created_at | TIMESTAMPTZ | |

### anomaly_events
Detected anomalies that exceeded baseline thresholds.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| tenant_id | UUID | FK → tenants |
| service | TEXT | |
| namespace | TEXT | |
| detected_at | TIMESTAMPTZ | When anomaly was detected |
| severity | TEXT | LOW, MEDIUM, HIGH, CRITICAL |
| metric | TEXT | e.g., "error_rate", "log_volume" |
| baseline_value | FLOAT | Expected value |
| observed_value | FLOAT | Actual value |
| deviation_factor | FLOAT | observed / baseline |
| notified | BOOLEAN | Whether notification was sent |
| created_at | TIMESTAMPTZ | |

### saved_filters
User-saved log search filter configurations.

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| tenant_id | UUID | FK → tenants |
| name | TEXT | Display name |
| filter_json | JSONB | Serialized filter (service, namespace, levels, keywords) |
| created_by | TEXT | User identifier |
| deleted_at | TIMESTAMPTZ | Nullable — soft delete |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

*Note: Detailed column constraints and indexes are defined in migration files.*

---

## Redis Usage

| Key Pattern | TTL | Purpose |
|---|---|---|
| `loki:query:<hash>` | 60s (configurable) | Cached Loki query results |
| `baseline:<tenant>:<service>` | 5m | Cached anomaly baseline for fast access |
| `ratelimit:<api_key>` | 60s | Sliding window rate limit counter |
| `job:<job_id>` | 10m | Async AI inference job status |

All Redis keys are namespaced by `tenant_id` to enforce isolation.

---

## Migrations

- Tool: `golang-migrate` (`migrate` CLI + Go library)
- Migration files: `migrations/<timestamp>_<description>.{up,down}.sql`
- Naming: `20240101120000_create_tenants.up.sql`
- Rules:
  - Never modify an existing migration file
  - All schema changes must have both `up` and `down` migrations
  - Migrations run automatically on server startup (dev); run manually in production with approval gate
  - Down migrations must be tested before merging

---

## Backup & Recovery

- Daily automated PostgreSQL dumps via `pg_dump` (configured in Docker Compose)
- 30-day retention of dumps
- Redis is a cache/queue only — data loss is tolerable; no backup required
- Restore procedure tested monthly in staging
- Point-in-time recovery: configure PostgreSQL WAL archiving for production deployments
