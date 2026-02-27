package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSummarizeCmd_FlagParsing(t *testing.T) {
	cmd := &summarizeCmd{}
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	cmd.registerFlags(fs)

	args := []string{
		"--service", "payments-api",
		"--namespace", "production",
		"--since", "2h",
		"--until", "10m",
		"--max-lines", "200",
		"--json",
	}

	if err := fs.Parse(args); err != nil {
		t.Fatalf("flag parse error: %v", err)
	}

	if cmd.service != "payments-api" {
		t.Errorf("expected service payments-api, got %s", cmd.service)
	}
	if cmd.namespace != "production" {
		t.Errorf("expected namespace production, got %s", cmd.namespace)
	}
	if cmd.since != "2h" {
		t.Errorf("expected since 2h, got %s", cmd.since)
	}
	if cmd.until != "10m" {
		t.Errorf("expected until 10m, got %s", cmd.until)
	}
	if cmd.maxLines != 200 {
		t.Errorf("expected max-lines 200, got %d", cmd.maxLines)
	}
	if !cmd.jsonOutput {
		t.Error("expected json to be true")
	}
}

func TestSummarizeCmd_Defaults(t *testing.T) {
	cmd := &summarizeCmd{}
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	cmd.registerFlags(fs)
	fs.Parse([]string{"--service", "api"})

	if cmd.namespace != "default" {
		t.Errorf("expected default namespace, got %s", cmd.namespace)
	}
	if cmd.since != "1h" {
		t.Errorf("expected default since 1h, got %s", cmd.since)
	}
	if cmd.until != "now" {
		t.Errorf("expected default until now, got %s", cmd.until)
	}
	if cmd.maxLines != 500 {
		t.Errorf("expected default max-lines 500, got %d", cmd.maxLines)
	}
}

func TestSummarizeCmd_Validate(t *testing.T) {
	cmd := &summarizeCmd{service: ""}
	if err := cmd.validate(); err == nil {
		t.Error("expected error for empty service")
	}

	cmd.service = "payments-api"
	if err := cmd.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatSummarizeOutput(t *testing.T) {
	var buf bytes.Buffer
	result := &summarizeResult{
		Summary:       "Multiple connection timeout errors detected from database pool.",
		LinesAnalyzed: 347,
		Provider:      "ollama",
		Model:         "llama3",
	}
	result.TimeRange.From = "2024-01-01T00:00:00Z"
	result.TimeRange.To = "2024-01-01T01:00:00Z"

	formatSummarizeOutput(&buf, "payments-api", "1h", result)

	out := buf.String()
	if !strings.Contains(out, "LogHunter Summary") {
		t.Errorf("expected header in output, got: %s", out)
	}
	if !strings.Contains(out, "Lines analyzed: 347") {
		t.Errorf("expected lines analyzed in output")
	}
	if !strings.Contains(out, "ollama (llama3)") {
		t.Errorf("expected provider/model in output")
	}
	if !strings.Contains(out, "connection timeout") {
		t.Errorf("expected summary text in output")
	}
}

func TestRunSummarize_JSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"summary":        "High error rate detected",
				"lines_analyzed": 100,
				"time_range": map[string]string{
					"from": "2024-01-01T00:00:00Z",
					"to":   "2024-01-01T01:00:00Z",
				},
				"provider": "ollama",
				"model":    "llama3",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &summarizeCmd{
		service:    "api",
		namespace:  "default",
		since:      "1h",
		until:      "now",
		maxLines:   500,
		jsonOutput: true,
	}

	var buf bytes.Buffer
	exitCode, err := runSummarize(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	if jsonStart == -1 {
		t.Fatalf("no JSON found in output: %s", output)
	}
	var result summarizeResult
	if err := json.Unmarshal([]byte(output[jsonStart:]), &result); err != nil {
		t.Fatalf("invalid JSON in output: %v\n%s", err, output)
	}
	if result.Summary != "High error rate detected" {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
	if result.Provider != "ollama" {
		t.Errorf("unexpected provider: %s", result.Provider)
	}
}

func TestRunSummarize_HumanOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"summary":        "All clear, no anomalies detected.",
				"lines_analyzed": 50,
				"time_range": map[string]string{
					"from": "2024-01-01T00:00:00Z",
					"to":   "2024-01-01T01:00:00Z",
				},
				"provider": "openai",
				"model":    "gpt-4",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &summarizeCmd{
		service:   "api",
		namespace: "default",
		since:     "1h",
		until:     "now",
		maxLines:  500,
	}

	var buf bytes.Buffer
	exitCode, err := runSummarize(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	out := buf.String()
	if !strings.Contains(out, "All clear") {
		t.Errorf("expected summary in output, got: %s", out)
	}
	if !strings.Contains(out, "openai (gpt-4)") {
		t.Errorf("expected provider info in output, got: %s", out)
	}
}
