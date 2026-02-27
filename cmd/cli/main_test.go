package main

import (
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	code := run(nil)
	if code != 1 {
		t.Errorf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRun_Help(t *testing.T) {
	code := run([]string{"--help"})
	if code != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", code)
	}
}

func TestRun_Version(t *testing.T) {
	code := run([]string{"--version"})
	if code != 0 {
		t.Errorf("expected exit code 0 for --version, got %d", code)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code := run([]string{"foobar"})
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", code)
	}
}

func TestRun_AnalyzeMissingService(t *testing.T) {
	code := run([]string{"analyze"})
	if code != 1 {
		t.Errorf("expected exit code 1 for missing --service, got %d", code)
	}
}

func TestRun_SearchMissingService(t *testing.T) {
	code := run([]string{"search"})
	if code != 1 {
		t.Errorf("expected exit code 1 for missing --service, got %d", code)
	}
}

func TestRun_SummarizeMissingService(t *testing.T) {
	code := run([]string{"summarize"})
	if code != 1 {
		t.Errorf("expected exit code 1 for missing --service, got %d", code)
	}
}

func TestRun_ConfigNoSubcommand(t *testing.T) {
	code := run([]string{"config"})
	if code != 1 {
		t.Errorf("expected exit code 1 for config with no subcommand, got %d", code)
	}
}

func TestRun_ConfigUnknownSubcommand(t *testing.T) {
	code := run([]string{"config", "badcmd"})
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown config subcommand, got %d", code)
	}
}
