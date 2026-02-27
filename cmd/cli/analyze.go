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

type analyzeCmd struct {
	service      string
	namespace    string
	since        string
	levels       string
	noAI         bool
	pollInterval time.Duration
	pollTimeout  time.Duration
	jsonOutput   bool
}

func (cmd *analyzeCmd) registerFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.service, "service", "", "Service name (required)")
	fs.StringVar(&cmd.namespace, "namespace", "default", "Namespace")
	fs.StringVar(&cmd.since, "since", "1h", "Time range (e.g. 30m, 1h, 24h)")
	fs.StringVar(&cmd.levels, "levels", "ERROR,WARN,FATAL,CRITICAL", "Comma-separated log levels")
	fs.BoolVar(&cmd.noAI, "no-ai", false, "Skip AI analysis")
	fs.DurationVar(&cmd.pollInterval, "poll-interval", 2*time.Second, "Polling interval for AI analysis")
	fs.DurationVar(&cmd.pollTimeout, "poll-timeout", 120*time.Second, "Max time to wait for AI analysis")
	fs.BoolVar(&cmd.jsonOutput, "json", false, "Output raw JSON")
}

func (cmd *analyzeCmd) validate() error {
	if cmd.service == "" {
		return fmt.Errorf("--service is required")
	}
	return nil
}

// analyzeResult holds the parsed API response for analysis.
type analyzeResult struct {
	Clusters []clusterResult `json:"clusters"`
	JobID    string          `json:"job_id,omitempty"`
	AIResult *aiResult       `json:"ai_result,omitempty"`
}

type clusterResult struct {
	ID            string `json:"id"`
	Level         string `json:"level"`
	Count         int    `json:"count"`
	SampleMessage string `json:"sample_message"`
	FirstSeenAt   string `json:"first_seen_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

type aiResult struct {
	RootCause       string  `json:"root_cause"`
	Confidence      float64 `json:"confidence"`
	Summary         string  `json:"summary"`
	SuggestedAction string  `json:"suggested_action,omitempty"`
	Provider        string  `json:"provider"`
	Model           string  `json:"model"`
}

func runAnalyze(ctx context.Context, client *apiClient, cmd *analyzeCmd, w io.Writer) (int, error) {
	// Parse time range
	dur, err := time.ParseDuration(cmd.since)
	if err != nil {
		return 1, fmt.Errorf("invalid --since value: %w", err)
	}
	end := time.Now().UTC()
	start := end.Add(-dur)

	levels := strings.Split(cmd.levels, ",")
	for i := range levels {
		levels[i] = strings.TrimSpace(levels[i])
	}

	fmt.Fprintf(w, "[*] Analyzing logs for service=%s...\n", cmd.service)

	// POST /api/v1/analyze to trigger detection
	reqBody := map[string]any{
		"service":    cmd.service,
		"namespace":  cmd.namespace,
		"start":      start.Format(time.RFC3339),
		"end":        end.Format(time.RFC3339),
		"levels":     levels,
		"include_ai": !cmd.noAI,
	}

	data, err := client.post(ctx, "/api/v1/analyze", reqBody)
	if err != nil {
		return 1, fmt.Errorf("analyze request failed: %w", err)
	}

	// Parse the response (could be 202 with job_id or immediate result)
	var resp struct {
		JobID    string `json:"job_id"`
		PollURL  string `json:"poll_url"`
		Clusters []clusterResult
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 1, fmt.Errorf("parsing response: %w", err)
	}

	result := &analyzeResult{
		Clusters: resp.Clusters,
		JobID:    resp.JobID,
	}

	// If we got a job_id and AI is enabled, poll for completion
	if resp.JobID != "" && !cmd.noAI {
		fmt.Fprintf(w, "[*] AI analysis in progress (job: %s)...", resp.JobID)

		aiRes, err := pollJob(ctx, client, resp.JobID, cmd.pollInterval, cmd.pollTimeout, w)
		if err != nil {
			fmt.Fprintf(w, "\n[!] AI analysis failed: %v\n", err)
		} else {
			result.AIResult = aiRes
		}
		fmt.Fprintln(w)
	}

	// Output
	if cmd.jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		formatAnalyzeOutput(w, cmd.service, cmd.since, result)
	}

	// Exit code: 1 if error clusters found, 0 if clean
	if len(result.Clusters) > 0 {
		return 1, nil
	}
	return 0, nil
}

func pollJob(ctx context.Context, client *apiClient, jobID string, interval, timeout time.Duration, w io.Writer) (*aiResult, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timed out after %s", timeout)
		case <-ticker.C:
			fmt.Fprint(w, ".")

			data, err := client.get(ctx, "/api/v1/analyze/"+jobID)
			if err != nil {
				continue
			}

			var pollResp struct {
				Status string   `json:"status"`
				Result aiResult `json:"result"`
			}
			if err := json.Unmarshal(data, &pollResp); err != nil {
				continue
			}

			switch pollResp.Status {
			case "completed":
				return &pollResp.Result, nil
			case "failed":
				return nil, fmt.Errorf("analysis job failed")
			}
		}
	}
}

func formatAnalyzeOutput(w io.Writer, service, since string, result *analyzeResult) {
	fmt.Fprintf(w, "\nLogHunter Analysis — %s (last %s)\n", service, since)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w)

	if len(result.Clusters) == 0 {
		fmt.Fprintln(w, "No error clusters found.")
		return
	}

	fmt.Fprintf(w, "%d error cluster(s) found\n\n", len(result.Clusters))

	for _, c := range result.Clusters {
		fmt.Fprintf(w, "[%s] %s (%d occurrences)\n", c.Level, c.SampleMessage, c.Count)
		fmt.Fprintf(w, "  First seen: %s\n", c.FirstSeenAt)
		fmt.Fprintf(w, "  Last seen:  %s\n", c.LastSeenAt)
	}

	if result.AIResult != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  AI Analysis (confidence: %.0f%%)\n", result.AIResult.Confidence*100)
		fmt.Fprintf(w, "  Root Cause: %s\n", result.AIResult.RootCause)
		if result.AIResult.SuggestedAction != "" {
			fmt.Fprintf(w, "  Suggested Action: %s\n", result.AIResult.SuggestedAction)
		}
	}
}
