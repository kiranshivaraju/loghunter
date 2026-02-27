package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"
)

type summarizeCmd struct {
	service    string
	namespace  string
	since      string
	until      string
	maxLines   int
	jsonOutput bool
}

func (cmd *summarizeCmd) registerFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.service, "service", "", "Service name (required)")
	fs.StringVar(&cmd.namespace, "namespace", "default", "Namespace")
	fs.StringVar(&cmd.since, "since", "1h", "Start of time range (e.g. 30m, 1h, 24h)")
	fs.StringVar(&cmd.until, "until", "now", "End of time range or 'now'")
	fs.IntVar(&cmd.maxLines, "max-lines", 500, "Max log lines to analyze")
	fs.BoolVar(&cmd.jsonOutput, "json", false, "Output raw JSON")
}

func (cmd *summarizeCmd) validate() error {
	if cmd.service == "" {
		return fmt.Errorf("--service is required")
	}
	return nil
}

// summarizeResult holds the parsed API response for summarization.
type summarizeResult struct {
	Summary       string `json:"summary"`
	LinesAnalyzed int    `json:"lines_analyzed"`
	TimeRange     struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"time_range"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func runSummarize(ctx context.Context, client *apiClient, cmd *summarizeCmd, w io.Writer) (int, error) {
	dur, err := time.ParseDuration(cmd.since)
	if err != nil {
		return 1, fmt.Errorf("invalid --since value: %w", err)
	}

	end := time.Now().UTC()
	if cmd.until != "now" {
		untilDur, err := time.ParseDuration(cmd.until)
		if err != nil {
			return 1, fmt.Errorf("invalid --until value: %w", err)
		}
		end = time.Now().UTC().Add(-untilDur)
	}
	start := end.Add(-dur)

	fmt.Fprintf(w, "[*] Summarizing logs for service=%s...\n", cmd.service)

	reqBody := map[string]any{
		"service":   cmd.service,
		"namespace": cmd.namespace,
		"start":     start.Format(time.RFC3339),
		"end":       end.Format(time.RFC3339),
		"max_lines": cmd.maxLines,
	}

	data, err := client.post(ctx, "/api/v1/summarize", reqBody)
	if err != nil {
		return 1, fmt.Errorf("summarize request failed: %w", err)
	}

	var result summarizeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return 1, fmt.Errorf("parsing response: %w", err)
	}

	if cmd.jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		formatSummarizeOutput(w, cmd.service, cmd.since, &result)
	}

	return 0, nil
}

func formatSummarizeOutput(w io.Writer, service, since string, result *summarizeResult) {
	fmt.Fprintf(w, "\nLogHunter Summary — %s (last %s)\n", service, since)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Lines analyzed: %d\n", result.LinesAnalyzed)
	fmt.Fprintf(w, "Provider: %s (%s)\n\n", result.Provider, result.Model)
	fmt.Fprintln(w, result.Summary)
}
