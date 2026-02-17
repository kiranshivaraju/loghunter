// Package models contains shared data models used across the LogHunter codebase.
package models

import (
	"context"
	"time"
)

// AIProvider is the core interface that all AI integrations must implement.
// Never call specific AI providers directly — always inject this interface.
type AIProvider interface {
	// Analyze performs root cause analysis on an error cluster.
	Analyze(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
	// Summarize condenses a stream of log lines into a plain-language summary.
	Summarize(ctx context.Context, logs []LogLine) (string, error)
	// Name returns the provider identifier (e.g., "ollama", "openai").
	Name() string
}

// AnalysisRequest is the input to an AI analysis operation.
type AnalysisRequest struct {
	Cluster     ErrorCluster
	ContextLogs []LogLine // Surrounding log lines for context
}

// AnalysisResult is the output of an AI analysis operation.
type AnalysisResult struct {
	RootCause       string    `json:"root_cause"`
	Confidence      float64   `json:"confidence"` // 0.0–1.0
	Summary         string    `json:"summary"`
	SuggestedAction string    `json:"suggested_action,omitempty"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"created_at"`
}

// ErrorCluster represents a deduplicated group of related error log lines.
type ErrorCluster struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Service       string    `json:"service"`
	Namespace     string    `json:"namespace"`
	Fingerprint   string    `json:"fingerprint"`
	Level         string    `json:"level"` // ERROR, WARN, FATAL, CRITICAL
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	Count         int       `json:"count"`
	SampleMessage string    `json:"sample_message"`
}

// LogLine represents a single log entry from Loki.
type LogLine struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
	Level     string            `json:"level"`
}
