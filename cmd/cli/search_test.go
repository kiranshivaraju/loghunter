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

func TestSearchCmd_FlagParsing(t *testing.T) {
	cmd := &searchCmd{}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	cmd.registerFlags(fs)

	args := []string{
		"--service", "payments-api",
		"--namespace", "production",
		"--since", "30m",
		"--until", "5m",
		"--levels", "ERROR,WARN",
		"--keyword", "timeout",
		"--limit", "50",
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
	if cmd.until != "5m" {
		t.Errorf("expected until 5m, got %s", cmd.until)
	}
	if cmd.levels != "ERROR,WARN" {
		t.Errorf("expected levels ERROR,WARN, got %s", cmd.levels)
	}
	if cmd.keyword != "timeout" {
		t.Errorf("expected keyword timeout, got %s", cmd.keyword)
	}
	if cmd.limit != 50 {
		t.Errorf("expected limit 50, got %d", cmd.limit)
	}
	if !cmd.jsonOutput {
		t.Error("expected json to be true")
	}
}

func TestSearchCmd_Defaults(t *testing.T) {
	cmd := &searchCmd{}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
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
	if cmd.levels != "" {
		t.Errorf("expected empty default levels, got %s", cmd.levels)
	}
	if cmd.keyword != "" {
		t.Errorf("expected empty default keyword, got %s", cmd.keyword)
	}
	if cmd.limit != 100 {
		t.Errorf("expected default limit 100, got %d", cmd.limit)
	}
}

func TestSearchCmd_Validate(t *testing.T) {
	cmd := &searchCmd{service: ""}
	if err := cmd.validate(); err == nil {
		t.Error("expected error for empty service")
	}

	cmd.service = "payments-api"
	if err := cmd.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatSearchOutput_NoResults(t *testing.T) {
	var buf bytes.Buffer
	result := &searchResult{Results: nil}
	formatSearchOutput(&buf, "api", "1h", result)

	out := buf.String()
	if !strings.Contains(out, "No log lines found") {
		t.Errorf("expected 'No log lines found' in output, got: %s", out)
	}
}

func TestFormatSearchOutput_WithResults(t *testing.T) {
	var buf bytes.Buffer
	cid := "abc-123"
	result := &searchResult{
		Results: []searchResultLine{
			{
				Timestamp: "2024-01-01T01:47:03Z",
				Level:     "ERROR",
				Message:   "connection timeout",
				ClusterID: &cid,
			},
			{
				Timestamp: "2024-01-01T01:48:00Z",
				Level:     "WARN",
				Message:   "retrying request",
			},
		},
	}
	formatSearchOutput(&buf, "payments-api", "30m", result)

	out := buf.String()
	if !strings.Contains(out, "2 log line(s) returned") {
		t.Errorf("expected line count in output, got: %s", out)
	}
	if !strings.Contains(out, "connection timeout") {
		t.Errorf("expected log message in output")
	}
	if !strings.Contains(out, "[cluster:abc-123]") {
		t.Errorf("expected cluster ID in output")
	}
	if !strings.Contains(out, "retrying request") {
		t.Errorf("expected second log message in output")
	}
}

func TestFormatSearchOutput_CacheHit(t *testing.T) {
	var buf bytes.Buffer
	result := &searchResult{
		Results: []searchResultLine{
			{Timestamp: "2024-01-01T00:00:00Z", Level: "INFO", Message: "test"},
		},
		CacheHit: true,
	}
	formatSearchOutput(&buf, "api", "1h", result)

	out := buf.String()
	if !strings.Contains(out, "served from cache") {
		t.Errorf("expected cache hit message, got: %s", out)
	}
}

func TestRunSearch_JSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"results": []map[string]any{
					{
						"timestamp":  "2024-01-01T00:00:00Z",
						"message":    "timeout",
						"level":      "ERROR",
						"labels":     map[string]string{"service": "api"},
						"cluster_id": "abc-123",
					},
				},
				"query":     `{service="api"}`,
				"cache_hit": false,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &searchCmd{
		service:    "api",
		namespace:  "default",
		since:      "1h",
		until:      "now",
		keyword:    "timeout",
		limit:      100,
		jsonOutput: true,
	}

	var buf bytes.Buffer
	exitCode, err := runSearch(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify valid JSON in output
	output := buf.String()
	jsonStart := strings.Index(output, "{")
	if jsonStart == -1 {
		t.Fatalf("no JSON found in output: %s", output)
	}
	var result searchResult
	if err := json.Unmarshal([]byte(output[jsonStart:]), &result); err != nil {
		t.Fatalf("invalid JSON in output: %v\n%s", err, output)
	}
	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
}

func TestRunSearch_HumanOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"results":   []any{},
				"query":     `{service="api"}`,
				"cache_hit": false,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := newAPIClient(ts.URL, "test-token")
	cmd := &searchCmd{
		service:   "api",
		namespace: "default",
		since:     "1h",
		until:     "now",
		limit:     100,
	}

	var buf bytes.Buffer
	exitCode, err := runSearch(context.Background(), client, cmd, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "No log lines found") {
		t.Errorf("expected no results message, got: %s", buf.String())
	}
}
