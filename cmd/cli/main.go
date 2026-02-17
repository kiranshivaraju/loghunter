// Package main is the entrypoint for the LogHunter CLI tool.
package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO: wire CLI commands (analyze, search, summary, anomalies, config)
	fmt.Fprintln(os.Stderr, "loghunter CLI — not yet implemented")
	os.Exit(1)
}
