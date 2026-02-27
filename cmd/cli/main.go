package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 1
	}

	subcmd := args[0]
	subArgs := args[1:]

	// Handle top-level flags
	if subcmd == "--help" || subcmd == "-h" {
		printUsage(os.Stdout)
		return 0
	}
	if subcmd == "--version" || subcmd == "-v" {
		fmt.Fprintf(os.Stdout, "loghunter %s\n", version)
		return 0
	}

	switch subcmd {
	case "analyze":
		return handleAnalyze(subArgs)
	case "search":
		return handleSearch(subArgs)
	case "summarize":
		return handleSummarize(subArgs)
	case "config":
		return handleConfig(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subcmd)
		printUsage(os.Stderr)
		return 1
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "LogHunter CLI — AI-powered log debugging tool")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: loghunter <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  analyze     Detect and cluster error patterns in logs")
	fmt.Fprintln(w, "  search      Search log lines with keyword filtering")
	fmt.Fprintln(w, "  summarize   AI-generated summary of recent logs")
	fmt.Fprintln(w, "  config      Manage CLI configuration")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global flags (for analyze, search, summarize):")
	fmt.Fprintln(w, "  --url       API server URL (env: LOGHUNTER_API_URL)")
	fmt.Fprintln(w, "  --token     API token (env: LOGHUNTER_TOKEN)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'loghunter <command> --help' for command-specific flags.")
}

// resolveCredentials resolves API URL and token with precedence: flag > env > config.
func resolveCredentials(fs *flag.FlagSet) (string, string, error) {
	var flagURL, flagToken string

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "url":
			flagURL = f.Value.String()
		case "token":
			flagToken = f.Value.String()
		}
	})

	apiURL := flagURL
	if apiURL == "" {
		apiURL = os.Getenv("LOGHUNTER_API_URL")
	}
	if apiURL == "" {
		cfg, err := loadConfig()
		if err == nil && cfg.APIURL != "" {
			apiURL = cfg.APIURL
		}
	}
	if apiURL == "" {
		return "", "", fmt.Errorf("API URL required: use --url, LOGHUNTER_API_URL, or 'loghunter config set-url'")
	}

	token := flagToken
	if token == "" {
		token = os.Getenv("LOGHUNTER_TOKEN")
	}
	if token == "" {
		cfg, err := loadConfig()
		if err == nil && cfg.Token != "" {
			token = cfg.Token
		}
	}
	if token == "" {
		return "", "", fmt.Errorf("API token required: use --token, LOGHUNTER_TOKEN, or 'loghunter config set-token'")
	}

	return apiURL, token, nil
}

func handleAnalyze(args []string) int {
	cmd := &analyzeCmd{}
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.StringVar(new(string), "url", "", "API server URL")
	fs.StringVar(new(string), "token", "", "API token")
	cmd.registerFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if err := cmd.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	apiURL, token, err := resolveCredentials(fs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := newAPIClient(apiURL, token)
	exitCode, err := runAnalyze(ctx, client, cmd, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return exitCode
}

func handleSearch(args []string) int {
	cmd := &searchCmd{}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.StringVar(new(string), "url", "", "API server URL")
	fs.StringVar(new(string), "token", "", "API token")
	cmd.registerFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if err := cmd.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	apiURL, token, err := resolveCredentials(fs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := newAPIClient(apiURL, token)
	exitCode, err := runSearch(ctx, client, cmd, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return exitCode
}

func handleSummarize(args []string) int {
	cmd := &summarizeCmd{}
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	fs.StringVar(new(string), "url", "", "API server URL")
	fs.StringVar(new(string), "token", "", "API token")
	cmd.registerFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if err := cmd.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	apiURL, token, err := resolveCredentials(fs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := newAPIClient(apiURL, token)
	exitCode, err := runSummarize(ctx, client, cmd, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return exitCode
}

func handleConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: loghunter config <set-url|set-token|show>")
		return 1
	}

	switch args[0] {
	case "set-url":
		if err := runConfigSetURL(args[1:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	case "set-token":
		if err := runConfigSetToken(args[1:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	case "show":
		if err := runConfigShow(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", args[0])
		return 1
	}
	return 0
}
