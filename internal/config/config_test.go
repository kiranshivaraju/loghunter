package config_test

import (
	"testing"
	"time"

	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setEnv is a helper that sets environment variables for a test and restores them after.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// validEnv returns the minimum set of valid environment variables.
func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":   "postgres://user:pass@localhost:5432/loghunter?sslmode=disable",
		"REDIS_URL":      "redis://localhost:6379",
		"LOKI_BASE_URL":  "http://localhost:3100",
		"AI_PROVIDER":    "ollama",
		"OLLAMA_BASE_URL": "http://localhost:11434",
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, "postgres://user:pass@localhost:5432/loghunter?sslmode=disable", cfg.Database.URL)
	assert.Equal(t, "redis://localhost:6379", cfg.Redis.URL)
	assert.Equal(t, "http://localhost:3100", cfg.Loki.BaseURL)
	assert.Equal(t, "ollama", cfg.AI.Provider)
}

func TestLoad_CustomPort(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("LOGHUNTER_PORT", "9090")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
}

func TestLoad_CustomEnv(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("LOGHUNTER_ENV", "production")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "production", cfg.Server.Env)
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	env := validEnv()
	delete(env, "DATABASE_URL")
	setEnv(t, env)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_EmptyDatabaseURL(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_MissingRedisURL(t *testing.T) {
	env := validEnv()
	delete(env, "REDIS_URL")
	setEnv(t, env)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_URL")
}

func TestLoad_MissingLokiBaseURL(t *testing.T) {
	env := validEnv()
	delete(env, "LOKI_BASE_URL")
	setEnv(t, env)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOKI_BASE_URL")
}

func TestLoad_InvalidLokiBaseURL(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("LOKI_BASE_URL", "not-a-valid-url")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOKI_BASE_URL")
}

func TestLoad_LokiBaseURLMustStartWithHTTP(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("LOKI_BASE_URL", "ftp://localhost:3100")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOKI_BASE_URL")
}

func TestLoad_MissingAIProvider(t *testing.T) {
	env := validEnv()
	delete(env, "AI_PROVIDER")
	setEnv(t, env)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI_PROVIDER")
}

func TestLoad_InvalidAIProvider(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("AI_PROVIDER", "invalid-provider")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI_PROVIDER")
}

func TestLoad_AllValidAIProviders(t *testing.T) {
	providers := []string{"ollama", "vllm", "openai", "anthropic"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			env := validEnv()
			env["AI_PROVIDER"] = provider

			switch provider {
			case "openai":
				env["OPENAI_API_KEY"] = "sk-test-key"
			case "anthropic":
				env["ANTHROPIC_API_KEY"] = "sk-ant-test-key"
			}
			setEnv(t, env)

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, provider, cfg.AI.Provider)
		})
	}
}

func TestLoad_OpenAIProviderMissingAPIKey(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("AI_PROVIDER", "openai")
	// No OPENAI_API_KEY set

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestLoad_AnthropicProviderMissingAPIKey(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("AI_PROVIDER", "anthropic")
	// No ANTHROPIC_API_KEY set

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}

func TestLoad_ExtraConfigIsHarmless(t *testing.T) {
	// Ollama selected but Anthropic key also set → valid (extra config is harmless)
	setEnv(t, validEnv())
	t.Setenv("AI_PROVIDER", "ollama")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-extra-key")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "ollama", cfg.AI.Provider)
}

func TestLoad_DatabaseDefaults(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
	assert.Equal(t, 5, cfg.Database.MaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.Database.ConnMaxLifetime)
}

func TestLoad_LokiDefaults(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "default", cfg.Loki.OrgID)
	assert.Equal(t, 30*time.Second, cfg.Loki.Timeout)
}

func TestLoad_AIDefaults(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 60*time.Second, cfg.AI.InferenceTimeout)
}

func TestLoad_LokiHTTPSURL(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("LOKI_BASE_URL", "https://loki.example.com")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "https://loki.example.com", cfg.Loki.BaseURL)
}

func TestLoad_OllamaConfig(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("OLLAMA_BASE_URL", "http://ollama:11434")
	t.Setenv("OLLAMA_MODEL", "llama3")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "http://ollama:11434", cfg.AI.Ollama.BaseURL)
	assert.Equal(t, "llama3", cfg.AI.Ollama.Model)
}

func TestLoad_VLLMConfig(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("AI_PROVIDER", "vllm")
	t.Setenv("VLLM_BASE_URL", "http://vllm:8000")
	t.Setenv("VLLM_MODEL", "mistral-7b")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "vllm", cfg.AI.Provider)
	assert.Equal(t, "http://vllm:8000", cfg.AI.VLLM.BaseURL)
	assert.Equal(t, "mistral-7b", cfg.AI.VLLM.Model)
}

func TestLoad_CustomInferenceTimeout(t *testing.T) {
	setEnv(t, validEnv())
	t.Setenv("AI_INFERENCE_TIMEOUT_SECS", "120")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 120*time.Second, cfg.AI.InferenceTimeout)
}
