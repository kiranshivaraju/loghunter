# System Architecture

## Overview

LogHunter is a multi-component system built around a central **Go API server** that orchestrates log retrieval from Loki, AI inference, and result delivery across three integration surfaces: a Grafana plugin, a Grafana Alloy extension, and a standalone CLI. All AI inference is performed via a pluggable AI provider layer supporting local runtimes (Ollama, vLLM) and cloud APIs (OpenAI, Anthropic) — the provider is configurable per deployment.

---

## Tech Stack

### Backend (Core API Server)
- **Language:** Go 1.22+
- **HTTP Framework:** `net/http` + `chi` router
- **Database:** PostgreSQL 15 (analysis results, baselines, config, multi-tenant data)
- **Cache:** Redis (query result caching, anomaly baseline caching)
- **Queue:** Redis + background goroutines (async AI inference jobs)
- **Log Client:** Loki HTTP API (LogQL queries via `http.Client`)

### AI Provider Layer
- **Local (on-premise):**
  - Ollama (recommended default — `llama3`, `mistral`, `codellama`)
  - vLLM (high-throughput GPU inference server)
- **Cloud (optional, user-configured):**
  - OpenAI API (`gpt-4o`, `gpt-4o-mini`)
  - Anthropic API (`claude-3-5-sonnet`, `claude-3-haiku`)
- **Abstraction:** Single `AIProvider` interface — all providers implement the same contract; no log data is ever sent to cloud providers unless explicitly configured

### Grafana Plugin (Frontend)
- **Language:** TypeScript
- **Framework:** React 18 + Grafana Plugin SDK (`@grafana/ui`, `@grafana/data`)
- **Build Tool:** Webpack (via Grafana plugin scaffold)
- **Communication:** REST calls to LogHunter API server

### Grafana Alloy Extension
- **Language:** Go
- **API:** Grafana Alloy component API (River config language)
- **Function:** Intercepts log batches, annotates errors/anomalies, forwards enriched logs to Loki

### CLI Tool
- **Language:** Go (single binary, cross-platform)
- **Distribution:** Pre-built binaries + `go install`
- **Output formats:** Human-readable (default), JSON (`--json`), YAML (`--yaml`)

### Infrastructure
- **Container:** Docker
- **Orchestration:** Docker Compose (dev + production single-node)
- **CI/CD:** GitHub Actions
- **Migrations:** `golang-migrate`

---

## Architecture Diagram

```
                        ┌─────────────────────────────────────────┐
                        │            User Interfaces               │
                        │                                          │
                        │  ┌──────────────┐  ┌────────────────┐  │
                        │  │ Grafana      │  │  CLI Tool      │  │
                        │  │ Plugin       │  │  (Go binary)   │  │
                        │  │ (React/TS)   │  │                │  │
                        │  └──────┬───────┘  └───────┬────────┘  │
                        └─────────┼─────────────────-┼───────────┘
                                  │  REST API         │  REST API
                                  ▼                   ▼
                    ┌─────────────────────────────────────────────┐
                    │          LogHunter API Server (Go)           │
                    │                                              │
                    │  ┌───────────┐  ┌──────────┐  ┌─────────┐ │
                    │  │  Log      │  │   AI     │  │Anomaly  │ │
                    │  │  Hunter   │  │ Analysis │  │Detector │ │
                    │  │  Engine   │  │ Service  │  │         │ │
                    │  └─────┬─────┘  └────┬─────┘  └────┬────┘ │
                    └────────┼─────────────┼──────────────┼───────┘
                             │             │              │
               ┌─────────────┘             │              └──────────────┐
               ▼                           ▼                             ▼
  ┌────────────────────┐    ┌──────────────────────────┐   ┌────────────────────┐
  │   Grafana / Loki   │    │   AI Provider Layer      │   │   PostgreSQL       │
  │   (LogQL HTTP API) │    │                          │   │   (results,        │
  │                    │    │  ┌──────┐ ┌──────┐       │   │    baselines,      │
  └────────────────────┘    │  │Ollama│ │ vLLM │       │   │    config)         │
                            │  └──────┘ └──────┘       │   └────────────────────┘
                            │  ┌────────┐ ┌─────────┐  │
                            │  │OpenAI  │ │Anthropic│  │   ┌────────────────────┐
                            │  │(opt.)  │ │ (opt.)  │  │   │   Redis            │
                            │  └────────┘ └─────────┘  │   │   (cache, queues)  │
                            └──────────────────────────┘   └────────────────────┘

  ┌──────────────────────────────────────────────────┐
  │        Grafana Alloy Extension (Go)              │
  │  (pipeline-level log tagging, runs in Alloy)     │
  │  → annotates logs before they reach Loki         │
  └──────────────────────────────────────────────────┘
```

---

## Component Responsibilities

### LogHunter API Server (Go)
- Exposes REST API consumed by Grafana plugin and CLI
- Orchestrates Loki queries via LogQL
- Runs error/warning detection and clustering logic
- Dispatches AI inference jobs to the AI Provider Layer
- Stores analysis results and anomaly baselines in PostgreSQL
- Caches frequent Loki query results in Redis

### Log Hunter Engine
- Translates user requests (service, namespace, time range) into LogQL queries
- Parses and clusters log lines by error type and stack trace fingerprint
- Deduplicates repeated errors within a time window
- Outputs structured `ErrorCluster` objects consumed by AI Analysis Service

### AI Analysis Service
- Accepts `ErrorCluster` + surrounding context log lines
- Routes inference request to the configured AI provider
- Returns structured `AnalysisResult` (root cause, confidence, summary, suggested action)
- Provider selection is runtime-configurable — no code change needed to switch providers

### AI Provider Layer
Implements a single `AIProvider` interface:
```go
type AIProvider interface {
    Analyze(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
    Summarize(ctx context.Context, logs []LogLine) (string, error)
}
```
Concrete implementations:
- `OllamaProvider` — local Ollama server
- `VLLMProvider` — local vLLM server
- `OpenAIProvider` — OpenAI API (optional, requires API key config)
- `AnthropicProvider` — Anthropic API (optional, requires API key config)

### Anomaly Detector
- Maintains rolling baseline of log volume, error rate, and pattern frequency per service
- Runs on a configurable interval (default: 1 minute)
- Compares current window against baseline; emits anomaly events when thresholds exceeded
- Anomaly events trigger notifications (Phase 3) and are stored in PostgreSQL

### Grafana Plugin (React/TypeScript)
- Renders LogHunter analysis results within a Grafana panel
- Syncs time range with Grafana's global time picker
- Provides service/namespace dropdown populated from Loki labels
- Displays error clusters, AI summaries, and root cause analysis

### Grafana Alloy Extension (Go)
- Runs as a native Alloy component in the collection pipeline
- Inspects each log batch for error-level entries
- Appends structured labels: `loghunter_severity`, `loghunter_cluster_id`, `loghunter_analyzed`
- Forwards enriched log stream to Loki — enables instant filtering in downstream queries

### CLI Tool (Go binary)
- Single cross-platform binary; no runtime dependencies
- Authenticates with LogHunter API server (API key or token)
- Supports: `analyze`, `search`, `summary`, `anomalies`, `config` subcommands
- Machine-readable output (`--json`) for CI/CD pipeline integration

---

## Design Decisions

### Why Go for the backend?
Go is the natural choice: the Alloy extension must be written in Go (Alloy component API), and sharing a language with the CLI and backend eliminates context-switching, enables shared packages (Loki client, AI provider interface, data models), and produces small, self-contained binaries with low resource overhead — ideal for a self-hosted tool running alongside an existing Grafana stack.

### Why a pluggable AI Provider interface?
Teams have different constraints: some are air-gapped and require Ollama or vLLM; others are comfortable using OpenAI or Anthropic for non-sensitive environments. A single interface with multiple provider implementations satisfies all use cases without feature branching, and allows teams to switch providers via config without code changes.

### Why PostgreSQL over SQLite?
PostgreSQL supports multi-tenant workloads, concurrent writes from multiple API server instances, and proper indexing on time-series anomaly baseline data. SQLite would limit horizontal scaling and concurrent access. PostgreSQL runs alongside the existing Loki/Grafana stack trivially via Docker Compose.

### Why Redis for caching?
Loki queries can be expensive for large time ranges. Redis caches query results (TTL: configurable, default 60s) and serves as the job queue for async AI inference, preventing blocking on slow model inference during high-traffic incident windows.

---

## Scalability Considerations

- **Phase 1 (single node):** Docker Compose with one API server instance — sufficient for team-scale usage
- **Phase 2 (horizontal):** API server is stateless (all state in PostgreSQL/Redis); scale by adding instances behind a load balancer
- **AI inference bottleneck:** Long-running AI jobs are dispatched asynchronously; the API server returns a job ID immediately and clients poll or receive a webhook callback
- **Loki query volume:** Redis caching reduces repeated identical queries; rate limiting prevents runaway scans

## Security Architecture

- API server exposes endpoints over HTTPS (TLS termination at reverse proxy/nginx)
- Authentication: API key (CLI/plugin) or Grafana-proxied identity for plugin
- All AI provider API keys stored as environment variables — never in config files or code
- No log data is transmitted to cloud AI providers unless explicitly configured by the admin
- Multi-tenant data isolation enforced at the PostgreSQL row level (tenant_id on all tables)
- Input validation on all Loki label/query parameters to prevent LogQL injection
