# Security Standards

## Authentication

**Strategy:** API Key + optional Grafana-proxied identity

### CLI & Direct API Access
- API keys issued per user/team via the LogHunter API server
- Keys are stored as bcrypt hashes in PostgreSQL (never in plaintext)
- Keys are passed as `Authorization: Bearer <api-key>` header
- Keys can be scoped to specific tenants/namespaces and revoked individually

### Grafana Plugin
- The plugin operates within Grafana's session context
- Grafana proxies requests to the LogHunter API server, forwarding the authenticated user identity
- LogHunter trusts Grafana-provided identity headers (`X-Grafana-User`, `X-Grafana-OrgId`) when configured in trusted-proxy mode
- Respects Grafana RBAC: viewers can read analysis, editors can trigger analysis, admins can manage config

---

## Authorization

**Model:** Tenant-scoped access control

- Every API request is scoped to a `tenant_id` (maps to a Loki org/namespace)
- Users can only access tenants they are authorized for
- Authorization is checked on every request at the handler level — no exceptions
- Principle of least privilege: API keys default to read-only unless explicitly granted write permissions

---

## AI Provider Security

This is the highest-sensitivity area of the system. Log data may contain PII, credentials, or business-sensitive information.

### On-Premise Providers (Ollama, vLLM)
- Log data stays within the deployment boundary — no external network calls
- Model inference runs on the same infrastructure as the LogHunter server
- **Default and recommended configuration**

### Cloud Providers (OpenAI, Anthropic)
- Cloud providers are **opt-in only** — disabled by default
- Must be explicitly enabled in config with an API key
- When enabled, an admin-level warning is displayed in the UI: "Log data will be sent to [provider] for analysis"
- Recommend restricting cloud provider usage to non-production or non-sensitive log streams
- API keys stored exclusively as environment variables — never in config files, databases, or logs
- All API key values are masked in log output: `sk-***` truncation applied in `slog` handler

### Provider Config Example
```yaml
ai:
  provider: ollama          # ollama | vllm | openai | anthropic
  ollama:
    base_url: http://ollama:11434
    model: llama3
  vllm:
    base_url: http://vllm:8000
    model: mistralai/Mistral-7B-Instruct-v0.2
  openai:
    api_key: ${OPENAI_API_KEY}  # from environment only
    model: gpt-4o-mini
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}  # from environment only
    model: claude-3-haiku-20240307
```

---

## Data Protection

- **Log data in transit:** All LogHunter API traffic over HTTPS (TLS 1.2+ required)
- **Log data at rest:** Raw log lines are NOT stored in PostgreSQL — only structured analysis results (error cluster metadata, summaries, root cause text) are persisted
- **API keys:** Stored as bcrypt hashes; the raw key is only shown once at creation
- **AI provider API keys:** Environment variables only, never written to disk or database
- **PostgreSQL:** Encrypted connections required (`sslmode=require`); credentials via environment variables

---

## Input Validation

- All Loki query parameters (service name, namespace, label selectors) are validated and sanitized before being interpolated into LogQL templates — prevent LogQL injection
- Time range inputs are validated (max range enforced to prevent runaway queries)
- Free-text search terms are escaped before inclusion in LogQL label matchers
- HTTP request bodies are validated against Go struct schemas with explicit field allowlists
- Rate limiting applied per API key: configurable (default: 60 requests/minute)

---

## Secrets Management

| Secret | Storage Location | Access Method |
|---|---|---|
| Database password | Environment variable | `DATABASE_URL` env var |
| Redis password | Environment variable | `REDIS_URL` env var |
| Loki credentials | Environment variable | `LOKI_USERNAME`, `LOKI_PASSWORD` |
| OpenAI API key | Environment variable | `OPENAI_API_KEY` |
| Anthropic API key | Environment variable | `ANTHROPIC_API_KEY` |
| LogHunter API keys | PostgreSQL (bcrypt hash) | Issued via admin API |

Never commit `.env` files to version control. Provide `.env.example` with placeholder values only.

---

## Security Headers (API Server)

All HTTP responses include:
- `Strict-Transport-Security: max-age=63072000; includeSubDomains`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: default-src 'none'` (API responses are JSON only)

---

## Dependency Management

- Go modules with `go.sum` verification
- `govulncheck` run in CI on every PR to detect known vulnerabilities in dependencies
- Grafana plugin npm packages audited via `npm audit` in CI
- Dependencies pinned to exact versions in `go.mod` and `package-lock.json`
- Dependabot configured for automated security update PRs

---

## Incident Response

If a security issue is detected:
1. Revoke affected API keys immediately via the admin API
2. Rotate any exposed environment-variable secrets
3. Review PostgreSQL audit logs for unauthorized access
4. If cloud AI providers were involved, check provider audit logs for unexpected requests
