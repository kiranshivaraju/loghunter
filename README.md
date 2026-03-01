# LogHunter

AI-powered log debugging tool for teams using Grafana Loki. LogHunter automatically surfaces errors and warnings, provides AI-driven root cause analysis and log summarization, and detects anomalies — accessible via a Grafana plugin, a Grafana Alloy extension, and a standalone CLI.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend API | Go 1.22+ (`chi` router) |
| Database | PostgreSQL 15 |
| Cache / Queue | Redis |
| AI Runtime | Ollama / vLLM (local) or OpenAI / Anthropic (optional) |
| Grafana Plugin | React 18 + TypeScript + Grafana Plugin SDK |
| Alloy Extension | Go (Alloy component API) |
| CLI | Go (single binary) |
| Container | Docker + Docker Compose |

## Getting Started

### Prerequisites

- Go 1.22+
- Docker + Docker Compose
- Node.js 20+ (for Grafana plugin development)
- An existing Grafana + Loki stack

### Local Development Setup

```bash
# Clone the repository
git clone https://github.com/kiranshivaraju/loghunter.git
cd loghunter

# Copy environment config
cp .env.example .env
# Edit .env with your Loki URL and AI provider settings

# Start dependencies (PostgreSQL, Redis)
docker compose up -d postgres redis

# Run database migrations
go run ./cmd/migrate up

# Start the API server
go run ./cmd/server

# In another terminal, build the CLI
go build -o bin/loghunter ./cmd/cli
```

### Running Tests

```bash
# Unit tests (fast, no external dependencies)
go test ./tests/unit/...

# Contract tests
go test ./tests/contract/...

# Integration tests (requires running PostgreSQL + Redis)
go test ./tests/integration/...

# All tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# Grafana plugin tests
cd grafana-plugin && npm test
```

### AI Provider Configuration

Set `AI_PROVIDER` in your `.env` file:

```bash
# Local (on-premise, recommended)
AI_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=llama3

# OR: vLLM
AI_PROVIDER=vllm
VLLM_BASE_URL=http://localhost:8000
VLLM_MODEL=mistralai/Mistral-7B-Instruct-v0.2

# Cloud (optional — logs will be sent to external provider)
AI_PROVIDER=openai
OPENAI_API_KEY=sk-...

AI_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
```

## Watcher — Proactive Log Monitoring

The Watcher is a background loop that continuously monitors your services for errors — so you don't have to go hunting for them manually.

**How it works:**

1. Every 60 seconds, the Watcher queries Loki for recent error/warn logs across your services
2. Groups identical errors into clusters using fingerprinting (normalizes timestamps, UUIDs, hex addresses, then hashes)
3. Detects **new** error clusters (never seen before) and **spikes** (error count jumped 3x+)
4. Automatically triggers AI root cause analysis on new/spiking clusters
5. Results are available via `GET /api/v1/watcher/status` — by the time you check, the answer is already there

**Enable it:**

```bash
WATCHER_ENABLED=true
WATCHER_INTERVAL_SECS=30              # poll every 30s (default: 60)
WATCHER_SERVICES=payment-service,auth-service  # or omit to auto-discover from Loki
WATCHER_AUTO_ANALYZE=true             # auto-trigger AI analysis (default: true)
WATCHER_SPIKE_THRESHOLD=3.0           # 3x count increase = spike (default: 3.0)
```

**All Watcher config:**

| Env Var | Default | Description |
|---------|---------|-------------|
| `WATCHER_ENABLED` | `false` | Enable the background watcher |
| `WATCHER_INTERVAL_SECS` | `60` | Seconds between poll cycles |
| `WATCHER_LOOKBACK_SECS` | `120` | How far back each poll looks |
| `WATCHER_SERVICES` | *(empty)* | Comma-separated service list, or empty for auto-discovery |
| `WATCHER_NAMESPACE` | `default` | Namespace filter for Loki queries |
| `WATCHER_AUTO_ANALYZE` | `true` | Auto-trigger AI analysis on findings |
| `WATCHER_SPIKE_THRESHOLD` | `3.0` | Count ratio that constitutes a spike |
| `WATCHER_MAX_SERVICES` | `50` | Cap on auto-discovered services |
| `WATCHER_LOGS_LIMIT` | `2000` | Max log lines per service per poll |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check (public) |
| `GET` | `/api/v1/watcher/status` | Watcher status and recent findings |
| `POST` | `/api/v1/search` | Search logs by keyword, service, time range |
| `GET` | `/api/v1/clusters` | List error clusters |
| `GET` | `/api/v1/clusters/{id}` | Get cluster details |
| `POST` | `/api/v1/analyze` | Trigger AI analysis on a cluster |
| `GET` | `/api/v1/analyze/{jobID}` | Poll analysis job status |
| `POST` | `/api/v1/summarize` | AI-powered log summary |
| `POST` | `/api/v1/admin/keys` | Create API key (admin) |
| `GET` | `/api/v1/admin/keys` | List API keys (admin) |
| `DELETE` | `/api/v1/admin/keys/{id}` | Revoke API key (admin) |

## Project Structure

```
.
├── cmd/
│   ├── server/         # API server entrypoint
│   └── cli/            # CLI binary entrypoint
├── internal/
│   ├── api/            # HTTP handlers and routing
│   ├── loki/           # Loki client and LogQL query builder
│   ├── ai/             # AI provider implementations
│   │   ├── ollama/
│   │   ├── vllm/
│   │   ├── openai/
│   │   ├── anthropic/
│   │   └── mock/       # Test mock provider
│   ├── analysis/       # Error clustering and detection
│   ├── watcher/        # Proactive log monitoring background loop
│   ├── store/          # PostgreSQL data access layer
│   ├── cache/          # Redis cache layer
│   └── config/         # Configuration loading
├── pkg/
│   ├── models/         # Shared data models (AIProvider interface, etc.)
│   └── logql/          # Shared LogQL utilities
├── migrations/         # SQL migration files
├── product/            # Product documentation (PRD, tech docs)
├── sprints/            # Sprint documentation
└── .prodkit/           # ProdKit framework config
```

## Contributing

All code changes require:
1. Tests written BEFORE implementation (TDD)
2. Unit + contract tests minimum; integration tests for DB/Loki interactions
3. 80% test coverage minimum (90% for `internal/analysis`)
4. `golangci-lint` passing with no new issues
5. Code review approval via Pull Request — no direct commits to `main`

See `product/tech-docs/design-principles.md` for coding standards.

## Documentation

- [Product Requirements Document](product/prd.md)
- [System Architecture](product/tech-docs/architecture.md)
- [Design Principles](product/tech-docs/design-principles.md)
- [Security Standards](product/tech-docs/security.md)
- [Data Architecture](product/tech-docs/data-architecture.md)
- [API Strategy](product/tech-docs/api-strategy.md)
- [Testing Strategy](product/tech-docs/testing-strategy.md)

## License

MIT
