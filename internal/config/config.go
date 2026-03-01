package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the LogHunter server.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Loki     LokiConfig
	AI       AIConfig
	Watcher  WatcherConfig
}

type WatcherConfig struct {
	Enabled        bool
	Interval       time.Duration
	LookbackWindow time.Duration
	Services       []string
	Namespace      string
	AutoAnalyze    bool
	SpikeThreshold float64
	MaxServices    int
	LogsLimit      int
}

type ServerConfig struct {
	Port int
	Env  string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL string
}

type LokiConfig struct {
	BaseURL  string
	Username string
	Password string
	OrgID    string
	Timeout  time.Duration
}

type AIConfig struct {
	Provider         string
	InferenceTimeout time.Duration
	Ollama           OllamaConfig
	VLLM             VLLMConfig
	OpenAI           OpenAIConfig
	Anthropic        AnthropicConfig
}

type OllamaConfig struct {
	BaseURL string
	Model   string
}

type VLLMConfig struct {
	BaseURL string
	Model   string
}

type OpenAIConfig struct {
	APIKey string
	Model  string
}

type AnthropicConfig struct {
	APIKey string
	Model  string
}

var validProviders = map[string]bool{
	"ollama":    true,
	"vllm":      true,
	"openai":    true,
	"anthropic": true,
}

// Load reads configuration from environment variables and returns a validated Config.
// Returns an error with a descriptive message if any required value is missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: envInt("LOGHUNTER_PORT", 8080),
			Env:  envString("LOGHUNTER_ENV", "development"),
		},
		Database: DatabaseConfig{
			URL:             os.Getenv("DATABASE_URL"),
			MaxOpenConns:    envInt("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envDuration("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL: os.Getenv("REDIS_URL"),
		},
		Loki: LokiConfig{
			BaseURL:  os.Getenv("LOKI_BASE_URL"),
			Username: os.Getenv("LOKI_USERNAME"),
			Password: os.Getenv("LOKI_PASSWORD"),
			OrgID:    envString("LOKI_ORG_ID", "default"),
			Timeout:  envDuration("LOKI_TIMEOUT", 30*time.Second),
		},
		AI: AIConfig{
			Provider:         os.Getenv("AI_PROVIDER"),
			InferenceTimeout: envDurationSecs("AI_INFERENCE_TIMEOUT_SECS", 60*time.Second),
			Ollama: OllamaConfig{
				BaseURL: envString("OLLAMA_BASE_URL", "http://localhost:11434"),
				Model:   envString("OLLAMA_MODEL", "llama3"),
			},
			VLLM: VLLMConfig{
				BaseURL: envString("VLLM_BASE_URL", "http://localhost:8000"),
				Model:   envString("VLLM_MODEL", ""),
			},
			OpenAI: OpenAIConfig{
				APIKey: os.Getenv("OPENAI_API_KEY"),
				Model:  envString("OPENAI_MODEL", "gpt-4"),
			},
			Anthropic: AnthropicConfig{
				APIKey: os.Getenv("ANTHROPIC_API_KEY"),
				Model:  envString("ANTHROPIC_MODEL", "claude-sonnet-4-5-20250929"),
			},
		},
		Watcher: WatcherConfig{
			Enabled:        envBool("WATCHER_ENABLED", false),
			Interval:       envDurationSecs("WATCHER_INTERVAL_SECS", 60*time.Second),
			LookbackWindow: envDurationSecs("WATCHER_LOOKBACK_SECS", 120*time.Second),
			Services:       envStringSlice("WATCHER_SERVICES"),
			Namespace:      envString("WATCHER_NAMESPACE", "default"),
			AutoAnalyze:    envBool("WATCHER_AUTO_ANALYZE", true),
			SpikeThreshold: envFloat("WATCHER_SPIKE_THRESHOLD", 3.0),
			MaxServices:    envInt("WATCHER_MAX_SERVICES", 50),
			LogsLimit:      envInt("WATCHER_LOGS_LIMIT", 2000),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if c.Redis.URL == "" {
		return fmt.Errorf("REDIS_URL is required")
	}

	if c.Loki.BaseURL == "" {
		return fmt.Errorf("LOKI_BASE_URL is required")
	}
	if !strings.HasPrefix(c.Loki.BaseURL, "http://") && !strings.HasPrefix(c.Loki.BaseURL, "https://") {
		return fmt.Errorf("LOKI_BASE_URL must start with http:// or https://, got %q", c.Loki.BaseURL)
	}

	if c.AI.Provider == "" {
		return fmt.Errorf("AI_PROVIDER is required")
	}
	if !validProviders[c.AI.Provider] {
		return fmt.Errorf("AI_PROVIDER must be one of ollama, vllm, openai, anthropic; got %q", c.AI.Provider)
	}

	if c.AI.Provider == "openai" && c.AI.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required when AI_PROVIDER is openai")
	}
	if c.AI.Provider == "anthropic" && c.AI.Anthropic.APIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required when AI_PROVIDER is anthropic")
	}

	return nil
}

func envString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}

func envDurationSecs(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	secs, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return time.Duration(secs) * time.Second
}

func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func envFloat(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

func envStringSlice(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var result []string
	for _, s := range strings.Split(v, ",") {
		if t := strings.TrimSpace(s); t != "" {
			result = append(result, t)
		}
	}
	return result
}
