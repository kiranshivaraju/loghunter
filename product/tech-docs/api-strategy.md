# API Design Strategy

## API Style

**Style:** REST over HTTP/JSON

The LogHunter API server exposes a REST API consumed by:
- The Grafana plugin (browser, via Grafana proxy)
- The CLI tool (Go HTTP client)
- External integrations and CI/CD pipelines

---

## Versioning

- Version prefix in URL: `/api/v1/`
- Breaking changes require a new major version (`/api/v2/`)
- Non-breaking additions (new optional fields, new endpoints) do not require a version bump
- Deprecation notices communicated via response headers: `Deprecation: true`, `Sunset: <date>`
- Old versions supported for a minimum of 3 months after deprecation notice

---

## Authentication

All endpoints require authentication via API key:

```
Authorization: Bearer <api-key>
```

The Grafana plugin sends requests through Grafana's data source proxy, which appends the configured API key automatically. CLI users pass the key via `--token` flag or `LOGHUNTER_TOKEN` environment variable.

---

## Endpoint Conventions

- Resource-based URLs (nouns, not verbs)
- Use HTTP methods semantically:
  - `GET` — Read/query (safe, idempotent)
  - `POST` — Trigger analysis, create resources
  - `PATCH` — Partial update
  - `DELETE` — Revoke/remove resources
- All endpoints return `application/json`
- Tenant is resolved from the API key — never passed as a URL parameter

---

## Core Endpoints

### Analysis

```
POST   /api/v1/analyze
```
Trigger error/warning detection and AI analysis for a service + time range. Returns a job ID for async polling or a result if the cache is warm.

```
GET    /api/v1/analyze/{job_id}
```
Poll async analysis job status and retrieve result when complete.

```
GET    /api/v1/clusters
```
List recent error clusters for the authenticated tenant. Supports filtering by service, namespace, level, and time range.

```
GET    /api/v1/clusters/{cluster_id}
```
Retrieve a specific error cluster with its full AI analysis result.

### Summaries

```
POST   /api/v1/summarize
```
Request a plain-language summary of a log stream for a given service + time range.

### Anomalies

```
GET    /api/v1/anomalies
```
List detected anomaly events. Filterable by service, severity, and time range.

```
GET    /api/v1/anomalies/{anomaly_id}
```
Retrieve a specific anomaly event with baseline comparison data.

### Search

```
POST   /api/v1/search
```
Execute a smart log search (translates user-friendly filters to LogQL, returns matching log lines + cluster groupings).

### Saved Filters

```
GET    /api/v1/filters
POST   /api/v1/filters
DELETE /api/v1/filters/{filter_id}
```
Manage saved search filter configurations per tenant.

### Admin

```
POST   /api/v1/admin/keys
DELETE /api/v1/admin/keys/{key_id}
GET    /api/v1/admin/keys
```
Manage API keys (admin-scoped keys only).

```
GET    /api/v1/health
```
Health check endpoint — unauthenticated. Returns server status, Loki connectivity, AI provider status, and DB connectivity.

---

## Request/Response Format

**Content-Type:** `application/json`

### Analysis Request
```json
{
  "service": "payments-api",
  "namespace": "production",
  "since": "30m",
  "levels": ["ERROR", "FATAL"],
  "include_ai": true
}
```

### Success Response (single resource)
```json
{
  "data": {
    "cluster_id": "018e2a4b-...",
    "service": "payments-api",
    "level": "ERROR",
    "count": 47,
    "first_seen_at": "2026-02-17T01:47:03Z",
    "sample_message": "connection pool exhausted: max_open_conns=10",
    "analysis": {
      "root_cause": "Database connection pool exhausted due to high request volume combined with slow query execution.",
      "confidence": 0.87,
      "summary": "The payments-api experienced 47 connection pool exhaustion errors between 01:47 and 02:03 UTC...",
      "suggested_action": "Increase DB_POOL_SIZE from 10 to 25 and investigate slow queries in the payments service."
    }
  }
}
```

### Success Response (collection)
```json
{
  "data": [...],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 84,
    "has_next": true
  }
}
```

### Async Job Response (202 Accepted)
```json
{
  "data": {
    "job_id": "018e2a4b-...",
    "status": "pending",
    "poll_url": "/api/v1/analyze/018e2a4b-..."
  }
}
```

### Error Response
```json
{
  "error": {
    "code": "LOKI_UNREACHABLE",
    "message": "Cannot reach Loki at http://loki:3100. Check your LOKI_BASE_URL configuration.",
    "details": {
      "loki_url": "http://loki:3100",
      "upstream_error": "connection refused"
    }
  }
}
```

---

## HTTP Status Codes

| Code | Meaning | When Used |
|---|---|---|
| 200 | OK | Successful GET / PATCH |
| 201 | Created | Resource created (API key, saved filter) |
| 202 | Accepted | Async job queued (analysis, summarization) |
| 400 | Bad Request | Validation error in request body/params |
| 401 | Unauthorized | Missing or invalid API key |
| 403 | Forbidden | Valid key but insufficient scope |
| 404 | Not Found | Resource doesn't exist or not accessible to tenant |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Unexpected server error |
| 502 | Bad Gateway | Loki or AI provider unreachable |
| 504 | Gateway Timeout | AI inference took too long |

---

## Pagination

```
GET /api/v1/clusters?page=1&limit=20&service=payments-api
```

Response always includes `meta` for collections:
```json
{
  "data": [...],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 84,
    "has_next": true
  }
}
```

Default limit: 20. Maximum limit: 100.

---

## Error Codes

Error codes are machine-readable strings in `SCREAMING_SNAKE_CASE`:

| Code | Meaning |
|---|---|
| `VALIDATION_ERROR` | Request body/params failed validation |
| `INVALID_TIME_RANGE` | Time range is invalid or exceeds max |
| `LOKI_UNREACHABLE` | Cannot connect to Loki |
| `LOKI_QUERY_ERROR` | Loki returned an error for the LogQL query |
| `AI_PROVIDER_UNAVAILABLE` | AI provider is down or misconfigured |
| `AI_INFERENCE_TIMEOUT` | AI model took too long to respond |
| `RATE_LIMIT_EXCEEDED` | Too many requests from this API key |
| `TENANT_NOT_FOUND` | API key's tenant does not exist |
| `RESOURCE_NOT_FOUND` | Requested resource does not exist |
| `INSUFFICIENT_SCOPE` | API key lacks permission for this action |

---

## Rate Limiting

- Applied per API key using a sliding window counter in Redis
- Default: 60 requests/minute per key (configurable per key)
- Response headers on every request:
  - `X-RateLimit-Limit: 60`
  - `X-RateLimit-Remaining: 43`
  - `X-RateLimit-Reset: 1708128060`
- On limit exceeded: `429 Too Many Requests` with `Retry-After` header
