package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type searchCmd struct {
	service    string
	namespace  string
	since      string
	until      string
	levels     string
	keyword    string
	limit      int
	jsonOutput bool
}

func (cmd *searchCmd) registerFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.service, "service", "", "Service name (required)")
	fs.StringVar(&cmd.namespace, "namespace", "default", "Namespace")
	fs.StringVar(&cmd.since, "since", "1h", "Start of time range (e.g. 30m, 1h, 24h)")
	fs.StringVar(&cmd.until, "until", "now", "End of time range (e.g. 30m, 1h) or 'now'")
	fs.StringVar(&cmd.levels, "levels", "", "Comma-separated log levels to filter")
	fs.StringVar(&cmd.keyword, "keyword", "", "Search keyword")
	fs.IntVar(&cmd.limit, "limit", 100, "Max number of results")
	fs.BoolVar(&cmd.jsonOutput, "json", false, "Output raw JSON")
}

func (cmd *searchCmd) validate() error {
	if cmd.service == "" {
		return fmt.Errorf("--service is required")
	}
	return nil
}

// searchResult holds the parsed API response for search.
type searchResult struct {
	Results  []searchResultLine `json:"results"`
	Query    string             `json:"query"`
	CacheHit bool              `json:"cache_hit"`
}

type searchResultLine struct {
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Labels    map[string]string `json:"labels"`
	ClusterID *string           `json:"cluster_id,omitempty"`
}

func runSearch(ctx context.Context, client *apiClient, cmd *searchCmd, w io.Writer) (int, error) {
	// Parse time range
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

	var levels []string
	if cmd.levels != "" {
		levels = strings.Split(cmd.levels, ",")
		for i := range levels {
			levels[i] = strings.TrimSpace(levels[i])
		}
	}

	fmt.Fprintf(w, "[*] Searching logs for service=%s...\n", cmd.service)

	reqBody := map[string]any{
		"service":   cmd.service,
		"namespace": cmd.namespace,
		"start":     start.Format(time.RFC3339),
		"end":       end.Format(time.RFC3339),
		"keyword":   cmd.keyword,
		"limit":     cmd.limit,
	}
	if len(levels) > 0 {
		reqBody["levels"] = levels
	}

	data, err := client.post(ctx, "/api/v1/search", reqBody)
	if err != nil {
		return 1, fmt.Errorf("search request failed: %w", err)
	}

	var result searchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return 1, fmt.Errorf("parsing response: %w", err)
	}

	if cmd.jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		formatSearchOutput(w, cmd.service, cmd.since, &result)
	}

	return 0, nil
}

func formatSearchOutput(w io.Writer, service, since string, result *searchResult) {
	fmt.Fprintf(w, "\nLogHunter Search — %s (last %s)\n", service, since)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w)

	if len(result.Results) == 0 {
		fmt.Fprintln(w, "No log lines found.")
		return
	}

	fmt.Fprintf(w, "%d log line(s) returned\n\n", len(result.Results))

	for _, line := range result.Results {
		ts := line.Timestamp
		// Try to format nicely
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			ts = t.Format("15:04:05.000 UTC")
		}

		clusterInfo := ""
		if line.ClusterID != nil {
			clusterInfo = fmt.Sprintf(" [cluster:%s]", *line.ClusterID)
		}
		fmt.Fprintf(w, "[%s] [%s] %s%s\n", ts, line.Level, line.Message, clusterInfo)
	}

	if result.CacheHit {
		fmt.Fprintln(w, "\n(served from cache)")
	}
}
