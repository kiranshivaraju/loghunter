# Design Principles & Coding Standards

## Core Philosophy

**DRY (Don't Repeat Yourself)** is a fundamental, non-negotiable design constraint. Any logic that appears more than once must be extracted into a shared function, package, or type. This applies across all components: API server, CLI, Alloy extension, and Grafana plugin.

---

## Code Organization

### Go (API Server, CLI, Alloy Extension)

```
loghunter/
├── cmd/
│   ├── server/          # API server entrypoint
│   └── cli/             # CLI binary entrypoint
├── internal/
│   ├── api/             # HTTP handlers and routing
│   ├── loki/            # Loki client and LogQL query builder
│   ├── ai/              # AI provider interface + implementations
│   │   ├── provider.go  # AIProvider interface (shared contract)
│   │   ├── ollama/      # Ollama implementation
│   │   ├── vllm/        # vLLM implementation
│   │   ├── openai/      # OpenAI implementation
│   │   └── anthropic/   # Anthropic implementation
│   ├── analysis/        # Error clustering, detection logic
│   ├── anomaly/         # Anomaly detection and baseline management
│   ├── store/           # PostgreSQL data access layer
│   ├── cache/           # Redis cache layer
│   └── config/          # Configuration loading and validation
├── pkg/
│   ├── models/          # Shared data models (ErrorCluster, AnalysisResult, etc.)
│   ├── logql/           # Shared LogQL utilities
│   └── notify/          # Notification senders (Slack, PagerDuty, webhook)
├── alloy/               # Grafana Alloy extension component
├── migrations/          # SQL migration files
└── grafana-plugin/      # Grafana plugin (TypeScript/React)
    ├── src/
    │   ├── components/  # React UI components
    │   ├── api/         # API client (calls LogHunter API server)
    │   └── types/       # TypeScript type definitions
    └── package.json
```

**Key rule:** `internal/` packages are not importable by external code. `pkg/` packages are shared across the CLI, API server, and Alloy extension. Never duplicate model definitions — all shared types live in `pkg/models`.

---

## Coding Standards

### Go
- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) and `gofmt` formatting
- Use `golangci-lint` with the project's `.golangci.yml` config
- Maximum function length: 50 lines (break into smaller functions if exceeded)
- All exported functions and types must have godoc comments
- Use structured logging (`slog` standard library) with consistent field names
- Context (`context.Context`) must be the first parameter on all functions that perform I/O
- Errors must be wrapped with `fmt.Errorf("operation: %w", err)` to preserve stack context
- Never use `panic` in production code paths; return errors explicitly

### Naming Conventions (Go)
- Packages: short, lowercase, no underscores (`analysis`, `loki`, `store`)
- Interfaces: noun or noun phrase, not verb (`AIProvider`, not `AIProvides`)
- Constructors: `New<Type>(...)` pattern (`NewOllamaProvider(...)`)
- Config structs: `<Component>Config` (`LokiConfig`, `AIConfig`)
- Constants: `UPPER_SNAKE_CASE` for package-level, `camelCase` for local

### TypeScript (Grafana Plugin)
- Use TypeScript strict mode (`"strict": true`)
- ESLint + Prettier enforced via pre-commit hooks
- All React components are functional (no class components)
- Props interfaces named `<ComponentName>Props`
- API types generated from or mirroring `pkg/models` Go types
- No `any` types — use `unknown` and narrow explicitly

---

## The AIProvider Interface Contract

This is the most critical shared abstraction in the codebase. All AI integrations must implement it without exception:

```go
// pkg/models/ai.go
type AIProvider interface {
    Analyze(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
    Summarize(ctx context.Context, logs []LogLine) (string, error)
    Name() string
}
```

Adding a new AI provider means creating a new package under `internal/ai/` that implements this interface — nothing else changes. The API server and CLI are completely decoupled from provider specifics.

---

## Error Handling

- **Go:** Return errors; never swallow them silently. Log at the boundary where the error is handled (not at every intermediate level).
- **HTTP API:** Map internal errors to structured JSON responses with appropriate HTTP status codes (see `api-strategy.md`).
- **AI provider errors:** Fail gracefully — if AI analysis fails, return the raw error cluster without AI annotation; never block log display on AI availability.
- **Loki connectivity errors:** Surface as a clear user-facing error with actionable message (e.g., "Cannot reach Loki at http://loki:3100 — check your config").

---

## Configuration

All configuration is loaded from environment variables and/or a YAML config file. The `internal/config` package is the single source of truth:

```go
type Config struct {
    Server   ServerConfig
    Loki     LokiConfig
    AI       AIConfig       // includes provider selection + provider-specific settings
    Database DatabaseConfig
    Cache    RedisConfig
}
```

**Never hardcode URLs, credentials, or provider names outside of `internal/config`.** The `AIConfig.Provider` field (`"ollama"`, `"vllm"`, `"openai"`, `"anthropic"`) determines which implementation is instantiated at startup.

---

## Logging Standards

Use Go's standard `slog` package with structured fields:

```go
slog.Info("analysis complete",
    "service", req.Service,
    "error_clusters", len(clusters),
    "provider", provider.Name(),
    "duration_ms", elapsed.Milliseconds(),
)
```

Log levels:
- `Debug`: Detailed internal state (disabled in production by default)
- `Info`: Normal operational events (request received, analysis complete)
- `Warn`: Recoverable issues (AI provider slow, cache miss rate high)
- `Error`: Failures that require attention (Loki unreachable, DB write failed)

Never log raw log line content at `Info` or above — log content may contain sensitive data.

---

## Code Review Standards

- All changes via Pull Requests — no direct commits to `main`
- At least 1 approval required before merge
- All tests must pass and coverage must not decrease
- No new `//nolint` directives without a comment explaining why
- Reviewer checklist:
  - [ ] Does this duplicate existing logic? (DRY check)
  - [ ] Are errors handled and wrapped correctly?
  - [ ] Is the AI provider interface respected (no provider-specific code outside `internal/ai/<provider>`)?
  - [ ] Are tests included?
  - [ ] Is sensitive data (API keys, log content) excluded from logs?
