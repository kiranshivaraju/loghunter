package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configDirName = ".loghunter"
const configFileName = "config.yaml"

type cliConfig struct {
	APIURL string `yaml:"api_url"`
	Token  string `yaml:"token"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, configDirName), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func loadConfig() (*cliConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &cliConfig{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg cliConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func saveConfig(cfg *cliConfig) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	path := filepath.Join(dir, configFileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func runConfigSetURL(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: loghunter config set-url <url>")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	cfg.APIURL = args[0]
	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Fprintf(w, "API URL set to %s\n", args[0])
	return nil
}

func runConfigSetToken(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: loghunter config set-token <token>")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	cfg.Token = args[0]
	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Fprintln(w, "Token saved.")
	return nil
}

func runConfigShow(w io.Writer) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	path, _ := configPath()
	fmt.Fprintf(w, "Config file: %s\n\n", path)

	if cfg.APIURL != "" {
		fmt.Fprintf(w, "api_url: %s\n", cfg.APIURL)
	} else {
		fmt.Fprintln(w, "api_url: (not set)")
	}

	if cfg.Token != "" {
		// Show only first 8 chars
		masked := cfg.Token
		if len(masked) > 8 {
			masked = masked[:8] + "..."
		}
		fmt.Fprintf(w, "token:   %s\n", masked)
	} else {
		fmt.Fprintln(w, "token:   (not set)")
	}

	return nil
}
