# Speckit Constitution — LogHunter

This document provides authoritative guidelines and defaults for AI-assisted development on the LogHunter project. It minimizes questions during implementation by establishing clear, pre-approved patterns.

---

## Project Context

LogHunter is an AI-powered log debugging tool for teams using Grafana Loki. It automatically surfaces errors and warnings from Loki log streams, provides AI-generated root cause analysis and summaries, and detects anomalies — all accessible via a Grafana plugin, a Grafana Alloy pipeline extension, and a standalone CLI binary.

**Three components share the same codebase:**
1. **API Server** (Go) — Core backend, REST API, orchestrates Loki queries and AI inference
2. **Alloy Extension** (Go) — Grafana Alloy component for pipeline-level log tagging
3. **CLI** (Go binary) — Command-line interface for terminal and CI/CD usage
4. **Grafana Plugin** (TypeScript/React) — In-Grafana UI panel

---

## Technical Stack

| Layer | Technology |
|---|---|
| Backend language | Go 1.22+ |
| HTTP router | `chi` |
| Database | PostgreSQL 15 (`pgx` driver) |
| Cache / Queue | Redis |
| Migrations | `golang-migrate` |
| AI providers | Ollama, vLLM, OpenAI, Anthropic (pluggable interface) |
| Grafana plugin | React 18 + TypeScript + Grafana Plugin SDK |
| Alloy extension | Go (Alloy component API) |
| CLI | Go (single binary) |
| Container | Docker + Docker Compose |
| Test framework | Go `testing` + `testify` |
| Lint | `golangci-lint` |

---

## AI Provider Defaults

**This is the most critical architectural pattern. Always follow it.**

All AI integrations must go through the `AIProvider` interface in `pkg/models/ai.go`. Never call Ollama, OpenAI, Anthropic, or vLLM directly from handlers or services — always inject the interface.

```go
type AIProvider interface {
    Analyze(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
    Summarize(ctx context.Context, logs []LogLine) (string, error)
    Name() string
}
```

Provider implementations live in `internal/ai/<provider>/`. The active provider is selected at startup from `AIConfig.Provider` (`"ollama"` | `"vllm"` | `"openai"` | `"anthropic"`).

When writing AI-related features:
- Accept `AIProvider` as a constructor parameter (dependency injection)
- Use `internal/ai/mock/` for all tests — never call real providers in tests
- Fail gracefully: if AI analysis fails, return the raw `ErrorCluster` without AI annotation; never block log display

---

## API Defaults

- All endpoints under `/api/v1/`
- Always return `Content-Type: application/json`
- Success (single): `{ "data": { ... } }`
- Success (collection): `{ "data": [...], "meta": { "page", "limit", "total", "has_next" } }`
- Error: `{ "error": { "code": "SCREAMING_SNAKE_CASE", "message": "...", "details": {} } }`
- Auth: `Authorization: Bearer <api-key>` on all endpoints except `/api/v1/health`
- Async operations return `202 Accepted` with `{ "data": { "job_id": "...", "poll_url": "..." } }`
- Rate limit response headers on every request: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`

Standard HTTP status codes (see `api-strategy.md` for full table). Common ones:
- Validation error → `400` with code `VALIDATION_ERROR`
- Bad/missing token → `401` with code `INVALID_TOKEN`
- Loki down → `502` with code `LOKI_UNREACHABLE`
- AI provider down → `502` with code `AI_PROVIDER_UNAVAILABLE`

---

## Database Defaults

- UUIDs for all primary keys (`uuid_generate_v4()`)
- Every table has `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` and `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- Every table has `tenant_id UUID NOT NULL` — enforced on every query
- Soft deletes for user-facing entities: `deleted_at TIMESTAMPTZ` (nullable)
- Hard deletes for time-bounded operational data (anomaly events beyond retention)
- All schema changes via `golang-migrate` migration files in `migrations/`
- Migration naming: `<timestamp>_<description>.{up,down}.sql`
- Never modify existing migration files — always add new ones

---

## Coding Standards (Go)

- `gofmt` and `golangci-lint` — all code must pass before PR merge
- `context.Context` is always the first parameter on functions performing I/O
- Errors wrapped: `fmt.Errorf("operation description: %w", err)`
- Structured logging via `slog` with consistent field names (`service`, `tenant_id`, `duration_ms`, `error`)
- Never log raw log line content (may contain PII)
- Constructor pattern: `New<Type>(deps...) (*Type, error)`
- Shared types in `pkg/models/` — never duplicate model definitions
- Max function length: 50 lines. Decompose if exceeded.

---

## Testing Defaults

- **ALWAYS write tests BEFORE implementation (TDD)** — non-negotiable
- Framework: Go `testing` + `testify`
- Unit tests: mock all external dependencies via interfaces
- Integration tests: use `testcontainers-go` for real PostgreSQL/Redis
- AI providers: always use `internal/ai/mock/` — never call real providers in tests
- Loki: use `httptest.NewServer` with fixture JSON responses from `tests/fixtures/loki/`
- Coverage minimum: 80% overall; 90% for `internal/analysis`
- Test naming: `Test<Function>_<Scenario>`
- Table-driven tests for multi-input functions
- Every new API endpoint needs a contract test covering: happy path, validation error, auth error

---

## Security Defaults

- API keys stored as bcrypt hashes in PostgreSQL — never store raw keys
- Cloud AI provider API keys (OpenAI, Anthropic) from environment variables ONLY — never in config files, code, or database
- All AI provider API key values masked in log output
- Loki query parameters validated and escaped before LogQL interpolation (prevent LogQL injection)
- Rate limiting applied per API key via Redis sliding window
- Multi-tenant isolation: `tenant_id` filter on every database query — no exceptions
- HTTPS required for all API server traffic (TLS at proxy layer)

---

## Configuration Defaults

All configuration loaded from environment variables or YAML config. The `internal/config` package is the single source of truth. Never hardcode URLs, credentials, or provider names outside `internal/config`.

Key environment variables:
```
LOGHUNTER_PORT=8080
DATABASE_URL=postgres://...
REDIS_URL=redis://...
LOKI_BASE_URL=http://loki:3100
LOKI_USERNAME=
LOKI_PASSWORD=
AI_PROVIDER=ollama              # ollama | vllm | openai | anthropic
OLLAMA_BASE_URL=http://ollama:11434
OLLAMA_MODEL=llama3
VLLM_BASE_URL=http://vllm:8000
VLLM_MODEL=mistralai/Mistral-7B-Instruct-v0.2
OPENAI_API_KEY=                 # optional, cloud provider
ANTHROPIC_API_KEY=              # optional, cloud provider
```

---

## Decision-Making Guidelines

When implementation choices arise, apply in order:

1. **DRY first** — if logic already exists anywhere in the codebase, reuse it; never duplicate
2. **Use the AIProvider interface** — never bypass it for "speed"; it is the core abstraction
3. **Choose simplicity** — the simpler solution that meets requirements is always preferred
4. **Security over convenience** — never shortcut auth, tenant isolation, or input validation
5. **Follow existing patterns** — new code should look like surrounding code (same error handling style, same logging style)
6. **When genuinely uncertain, ASK** — do not invent requirements or make major architectural decisions without checking

---

## Features to Avoid Unless Explicitly Requested

- Do NOT add social login / OAuth
- Do NOT add email verification flows
- Do NOT add a web UI / dashboard beyond the Grafana plugin
- Do NOT introduce new dependencies without justification in the PR description
- Do NOT store raw log lines in PostgreSQL (Loki is the source of truth)
- Do NOT call cloud AI providers (OpenAI/Anthropic) by default — they must be explicitly configured

---

## What to Always Include

- Input validation on all API request parameters
- Structured error handling with wrapped errors and `slog` logging
- `context.Context` propagation through all I/O calls
- Tests (unit + contract minimum; integration for database/Loki interaction)
- Tenant ID scoping on every database query
- Graceful degradation when AI provider is unavailable
- Godoc comments on all exported types and functions
