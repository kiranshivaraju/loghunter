package shared

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/kiranshivaraju/loghunter/pkg/models"
)

var analyzeTemplate = template.Must(template.New("analyze").Parse(`You are a log analysis expert. Analyze the following error logs and provide:
1. Root cause (2-3 sentences)
2. Confidence score (0.0-1.0)
3. Brief summary of the incident (3-5 sentences)
4. Suggested corrective action (1-2 sentences, optional)

Respond in JSON format:
{
  "root_cause": "...",
  "confidence": 0.85,
  "summary": "...",
  "suggested_action": "..."
}

Error cluster ({{.Count}} occurrences):
{{.SampleMessage}}

Context logs (surrounding lines):
{{range .ContextLogs}}[{{.Timestamp}}] {{.Level}}: {{.Message}}
{{end}}`))

var summarizeTemplate = template.Must(template.New("summarize").Parse(`Summarize the following log stream in 3-5 sentences. Focus on what happened, when it happened, and any notable errors or patterns. Be concise and factual.

Log stream ({{.LineCount}} lines):
{{range .Logs}}[{{.Timestamp}}] {{.Level}}: {{.Message}}
{{end}}`))

// BuildAnalyzePrompt renders the analysis prompt for the given request.
func BuildAnalyzePrompt(req models.AnalysisRequest) (string, error) {
	var buf bytes.Buffer
	err := analyzeTemplate.Execute(&buf, struct {
		Count         int
		SampleMessage string
		ContextLogs   []models.LogLine
	}{
		Count:         req.Cluster.Count,
		SampleMessage: req.Cluster.SampleMessage,
		ContextLogs:   req.ContextLogs,
	})
	if err != nil {
		return "", fmt.Errorf("rendering analyze prompt: %w", err)
	}
	return buf.String(), nil
}

// BuildSummarizePrompt renders the summarize prompt for the given logs.
func BuildSummarizePrompt(logs []models.LogLine) (string, error) {
	var buf bytes.Buffer
	err := summarizeTemplate.Execute(&buf, struct {
		LineCount int
		Logs      []models.LogLine
	}{
		LineCount: len(logs),
		Logs:      logs,
	})
	if err != nil {
		return "", fmt.Errorf("rendering summarize prompt: %w", err)
	}
	return buf.String(), nil
}

// AnalysisJSON is the JSON structure expected from AI providers for analysis responses.
type AnalysisJSON struct {
	RootCause       string  `json:"root_cause"`
	Confidence      float64 `json:"confidence"`
	Summary         string  `json:"summary"`
	SuggestedAction string  `json:"suggested_action"`
}

// ToResult converts an AnalysisJSON into a models.AnalysisResult with validation.
// Confidence is clamped to [0.0, 1.0] and string fields are trimmed.
func (a *AnalysisJSON) ToResult(provider, model string) models.AnalysisResult {
	confidence := a.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	rootCause := strings.TrimSpace(a.RootCause)
	summary := strings.TrimSpace(a.Summary)
	var suggestedAction *string
	if action := strings.TrimSpace(a.SuggestedAction); action != "" {
		suggestedAction = &action
	}

	return models.AnalysisResult{
		Provider:        provider,
		Model:           model,
		RootCause:       rootCause,
		Confidence:      confidence,
		Summary:         summary,
		SuggestedAction: suggestedAction,
	}
}
