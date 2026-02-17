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
│   ├── anomaly/        # Anomaly detection and baselines
│   ├── store/          # PostgreSQL data access layer
│   ├── cache/          # Redis cache layer
│   └── config/         # Configuration loading
├── pkg/
│   ├── models/         # Shared data models (AIProvider interface, etc.)
│   ├── logql/          # Shared LogQL utilities
│   └── notify/         # Notification senders
├── alloy/              # Grafana Alloy extension component
├── migrations/         # SQL migration files
├── grafana-plugin/     # Grafana plugin (TypeScript/React)
├── tests/
│   ├── unit/           # Unit tests
│   ├── contract/       # API contract tests
│   ├── integration/    # Integration tests
│   └── fixtures/       # Test data fixtures
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
