# Testing Strategy

## Testing Philosophy

- **Test-Driven Development (TDD):** Write tests BEFORE implementation — this is enforced by code review
- **Coverage Target:** 80% minimum across all Go packages
- **Testing Pyramid:** Maximize unit tests; integration tests cover critical paths; E2E tests cover key user workflows
- **AI provider tests:** Always test against a mock provider in unit/integration tests — never call real AI APIs in automated tests

---

## Test Types

### Unit Tests
- Test individual functions, methods, and types in isolation
- All external dependencies (Loki client, AI provider, PostgreSQL, Redis) are mocked via interfaces
- Fast: full unit test suite must complete in under 30 seconds
- Framework: Go standard `testing` package + `testify` for assertions and mocks

### Contract Tests
- Verify the LogHunter REST API matches its documented request/response contracts
- Test every endpoint: valid inputs → correct response shape; invalid inputs → correct error codes
- Use `net/http/httptest` to test handlers without a running server
- Critical: ensure the `AIProvider` interface contract is respected by all provider implementations

### Integration Tests
- Test real interactions between components using actual PostgreSQL and Redis instances
- Use Docker Compose `test` profile to spin up dependencies
- Cover: Loki client → parse → cluster → store → retrieve flows
- Test AI provider implementations against local mocks (Ollama mock server)
- Framework: Go `testing` + `testcontainers-go` for ephemeral PostgreSQL/Redis

### Grafana Plugin Tests
- Unit tests for React components using `@testing-library/react`
- API client mock tests to verify correct request construction
- Snapshot tests for UI component rendering

### E2E Tests (Phase 3+)
- Full stack tests: real Loki instance + LogHunter API + CLI
- Cover: end-to-end analysis flow, CLI JSON output parsing
- Run in CI on merge to `main` only (not on every PR — too slow)

---

## Test Organization

```
tests/
├── unit/
│   ├── analysis/
│   │   ├── cluster_test.go       # Error clustering logic
│   │   └── detection_test.go     # Error/warning detection
│   ├── ai/
│   │   ├── ollama_test.go        # Ollama provider unit tests (mocked HTTP)
│   │   ├── openai_test.go        # OpenAI provider unit tests (mocked HTTP)
│   │   └── provider_contract_test.go  # Interface compliance tests
│   ├── loki/
│   │   └── client_test.go        # Loki client with mocked HTTP responses
│   ├── anomaly/
│   │   └── detector_test.go
│   └── api/
│       └── handlers_test.go      # HTTP handler unit tests
├── contract/
│   ├── analyze_test.go           # POST /api/v1/analyze contract
│   ├── clusters_test.go          # GET /api/v1/clusters contract
│   ├── anomalies_test.go
│   └── health_test.go
└── integration/
    ├── analysis_flow_test.go     # Full detection → cluster → store → retrieve
    ├── anomaly_baseline_test.go
    └── api_key_test.go

grafana-plugin/src/__tests__/
├── components/
│   ├── ErrorClusterPanel.test.tsx
│   └── AnalysisSummary.test.tsx
└── api/
    └── client.test.ts
```

---

## Mock Strategy

**AI Provider mock** (`internal/ai/mock/provider.go`):
```go
type MockProvider struct {
    AnalyzeFunc   func(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
    SummarizeFunc func(ctx context.Context, logs []LogLine) (string, error)
}
```
Used in all unit and contract tests. Each test controls the response — no real model required.

**Loki HTTP mock:** Use `httptest.NewServer` with pre-loaded fixture responses (real LogQL response JSON). Fixtures stored in `tests/fixtures/loki/`.

**PostgreSQL:** Use `testcontainers-go` to spin up a real PostgreSQL instance per integration test suite. Schema applied via `golang-migrate`. Tear down after suite.

---

## CI/CD Testing Pipeline

```
PR opened / push to branch:
  1. go vet ./...                    # Static analysis
  2. golangci-lint run               # Lint
  3. go test ./tests/unit/...        # Unit tests (fast)
  4. go test ./tests/contract/...    # Contract tests
  5. go test ./tests/integration/... # Integration tests (Docker required)
  6. go tool cover -func coverage.out # Coverage check (fail if < 80%)
  7. govulncheck ./...               # Vulnerability scan
  8. npm test (grafana-plugin/)      # Plugin unit tests

Merge to main:
  All above + E2E test suite (Phase 3+)
```

CI fails the PR if:
- Any test fails
- Coverage drops below 80%
- `govulncheck` finds a known vulnerability with a fix available
- `golangci-lint` reports new issues

---

## Testing Standards

- Every new exported function must have at least one unit test
- Every new API endpoint must have a contract test covering: happy path, validation error, auth error
- Tests must be deterministic — no `time.Now()` or random values without seeding/mocking
- Use table-driven tests for functions with multiple input/output combinations
- Test names follow: `Test<Function>_<Scenario>` (e.g., `TestCluster_DeduplicatesIdenticalFingerprints`)
- No test should depend on the order of other tests — each test is fully independent
- Test data (fixtures) stored in `tests/fixtures/` — never hardcode long strings in test files

---

## Coverage Requirements

| Package | Minimum Coverage |
|---|---|
| `internal/analysis` | 90% |
| `internal/ai` | 85% |
| `internal/loki` | 85% |
| `internal/api` | 80% |
| `internal/anomaly` | 80% |
| `internal/store` | 75% (DB layer, tested via integration) |
| Overall | 80% |
