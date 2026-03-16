// Package config provides functionality for config.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the application's runtime configuration.
type Config struct {
	// Projects is a list of Google Cloud Project IDs to operate on.
	Projects []string `toml:"projects"`
	// DownloadDir is the directory where files will be downloaded.
	DownloadDir string `toml:"download_dir"`
	// FuzzySearch enables fuzzy matching for filtering lists.
	FuzzySearch bool `toml:"fuzzy_search"`
	// Icons enables rendering of Nerd Font icons next to items.
	Icons bool `toml:"icons"`
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// DefaultPath ...
func DefaultPath() (string, error) {
	configPath := os.Getenv("LAZYGCS_CONFIG")
	if configPath != "" {
		return configPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "lazygcs", "config.toml"), nil
}

// Load resolves the configuration from the specified TOML file.
//
// Arguments:
//   - configPath: Absolute path to the TOML configuration file. If empty, uses DefaultPath().
//
// Returns:
//   - *Config: The resolved configuration.
//   - error: If the config file does not exist, or cannot be parsed.
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(configPath); err != nil {
		return nil, err
	}

	cfg := &Config{
		Projects:    []string{},
		DownloadDir: defaultDownloadDir(),
		FuzzySearch: true,
	}

	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return nil, err
	}

	if len(cfg.Projects) > 0 {
		cfg.Projects = trimProjects(cfg.Projects)
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = defaultDownloadDir()
	}

	return cfg, nil
}

// trimProjects trims whitespace from each project ID and filters out empty strings.
func trimProjects(raw []string) []string {
	var clean []string
	for _, p := range raw {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			if trimmed != p {
				log.Printf("Warning: project ID %q was trimmed to %q", p, trimmed)
			}
			clean = append(clean, trimmed)
		}
	}
	return clean
}
