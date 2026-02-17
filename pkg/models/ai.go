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
	ContextLogs []LogLine // Surrounding log lines for context, sorted chronologically
}

// LogLine represents a single log entry from Loki.
type LogLine struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
	Level     string            `json:"level"`
}
