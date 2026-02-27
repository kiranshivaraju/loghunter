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
	"time"
)

func TestAnalyzeCmd_FlagParsing(t *testing.T) {
	cmd := &analyzeCmd{}
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	cmd.registerFlags(fs)

	args := []string{
		"--service", "payments-api",
		"--namespace", "production",
		"--since", "30m",
		"--levels", "ERROR,FATAL",
		"--no-ai",
		"--poll-interval", "5s",
		"--poll-timeout", "60s",
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
	if cmd.since != "30m" {
		t.Errorf("expected since 30m, got %s", cmd.since)
	}
	if cmd.levels != "ERROR,FATAL" {
		t.Errorf("expected levels ERROR,FATAL, got %s", cmd.levels)
	}
	if !cmd.noAI {
		t.Error("expected no-ai to be true")
	}
	if cmd.pollInterval != 5*time.Second {
		t.Errorf("expected poll-interval 5s, got %s", cmd.pollInterval)
	}
	if cmd.pollTimeout != 60*time.Second {
		t.Errorf("expected poll-timeout 60s, got %s", cmd.pollTimeout)
	}
	if !cmd.jsonOutput {
		t.Error("expected json to be true")
	}
}

func TestAnalyzeCmd_Defaults(t *testing.T) {
	cmd := &analyzeCmd{}
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	cmd.registerFlags(fs)
	fs.Parse([]string{"--service", "api"})

	if cmd.namespace != "default" {
		t.Errorf("expected default namespace, got %s", cmd.namespace)
	}
	if cmd.since != "1h" {
		t.Errorf("expected default since 1h, got %s", cmd.since)
	}
	if cmd.levels != "ERROR,WARN,FATAL,CRITICAL" {
		t.Errorf("expected default levels, got %s", cmd.levels)
	}
	if cmd.noAI {
		t.Error("expected no-ai to default false")
	}
	if cmd.pollInterval != 2*time.Second {
		t.Errorf("expected default poll-interval 2s, got %s", cmd.pollInterval)
	}
}

func TestAnalyzeCmd_Validate(t *testing.T) {
	cmd := &analyzeCmd{service: ""}
	if err := cmd.validate(); err == nil {
		t.Error("expected error for empty service")
	}

	cmd.service = "payments-api"
	if err := cmd.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatAnalyzeOutput_NoClusters(t *testing.T) {
	var buf bytes.Buffer
	result := &analyzeResult{Clusters: nil}
	formatAnalyzeOutput(&buf, "api", "1h", result)

	out := buf.String()
	if !strings.Contains(out, "No error clusters found") {
		t.Errorf("expected 'No error clusters found' in output, got: %s", out)
	}
}

func TestFormatAnalyzeOutput_WithClusters(t *testing.T) {
	var buf bytes.Buffer
	result := &analyzeResult{
		Clusters: []clusterResult{
			{
				Level:         "ERROR",
				SampleMessage: "connection timeout",
				Count:         47,
				FirstSeenAt:   "01:47:03 UTC",
				LastSeenAt:    "02:03:17 UTC",
			},
		},
	}
	formatAnalyzeOutput(&buf, "payments-api", "30m", result)

	out := buf.String()
	if !strings.Contains(out, "1 error cluster(s) found") {
		t.Errorf("expected cluster count in output, got: %s", out)
	}
	if !strings.Contains(out, "connection timeout") {
		t.Errorf("expected sample message in output")
	}
	if !strings.Contains(out, "47 occurrences") {
		t.Errorf("expected occurrence count in output")
	}
}

func TestFormatAnalyzeOutput_WithAI(t *testing.T) {
	var buf bytes.Buffer
	result := &analyzeResult{
		Clusters: []clusterResult{
			{Level: "ERROR", SampleMessage: "test", Count: 1},
		},
		AIResult: &aiResult{
			RootCause:       "Connection pool exhaustion",
			Confidence:      0.87,
			SuggestedAction: "Increase pool size",
		},
	}
	formatAnalyzeOutput(&buf, "api", "1h", result)

	out := buf.String()
	if !strings.Contains(out, "AI Analysis (confidence: 87%)") {
		t.Errorf("expected AI analysis in output, got: %s", out)
	}
	if !strings.Contains(out, "Connection pool exhaustion") {
		t.Errorf("expected root cause in output")
	}
	if !strings.Contains(out, "Increase pool size") {
		t.Errorf("expected suggested action in output")
	}
}

func TestRunAnalyze_JSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"clusters": []map[string]any{
					{
						"id":             "abc",
						"level":          "ERROR",
						"count":          5,
						"sample_message": "timeout",
						"first_seen_at":  "2024-01-01T00:00:00Z",
						"last_seen_at":   "2024-01-01T01:00:00Z",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &analyzeCmd{
		service:    "api",
		namespace:  "default",
		since:      "1h",
		levels:     "ERROR",
		noAI:       true,
		jsonOutput: true,
	}

	var buf bytes.Buffer
	exitCode, err := runAnalyze(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 (errors found), got %d", exitCode)
	}

	// Verify valid JSON output
	var result analyzeResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		// The output includes the "[*] Analyzing..." line before JSON
		// Extract just the JSON part
		output := buf.String()
		jsonStart := strings.Index(output, "{")
		if jsonStart == -1 {
			t.Fatalf("no JSON found in output: %s", output)
		}
		if err := json.Unmarshal([]byte(output[jsonStart:]), &result); err != nil {
			t.Fatalf("invalid JSON in output: %v\n%s", err, output)
		}
	}
}

func TestRunAnalyze_ExitCode0_NoClusters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"clusters": []any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &analyzeCmd{
		service:   "api",
		namespace: "default",
		since:     "1h",
		levels:    "ERROR",
		noAI:      true,
	}

	var buf bytes.Buffer
	exitCode, err := runAnalyze(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (no errors), got %d", exitCode)
	}
}
