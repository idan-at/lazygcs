package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the application's runtime configuration.
type Config struct {
	// Projects is a list of Google Cloud Project IDs to operate on.
	Projects []string
	// DownloadDir is the directory where files will be downloaded.
	DownloadDir string
}

// tomlConfig represents the structure of the config.toml file.
type tomlConfig struct {
	Projects    []string `toml:"projects"`
	DownloadDir string   `toml:"download_dir"`
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// Load resolves the configuration based on the precedence hierarchy.
//
// It checks sources in the following order (first match wins):
//  1. CLI Arguments: Passed directly to the application.
//  2. Config File: TOML file at the specified path (e.g., ~/.config/lazygcs/config.toml).
//
// Arguments:
//   - args: Command-line arguments (excluding the program name).
//   - configPath: Absolute path to the TOML configuration file.
//
// Returns:
//   - *Config: The resolved configuration.
//   - error: If the config file exists but cannot be parsed.
func Load(args []string, configPath string) (*Config, error) {
	cfg := &Config{
		Projects:    []string{},
		DownloadDir: defaultDownloadDir(),
	}

	if len(args) > 0 {
		cfg.Projects = trimProjects(args)
		// We still try to load DownloadDir from config if it exists
	}

	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			var tc tomlConfig
			if _, err := toml.DecodeFile(configPath, &tc); err != nil {
				return nil, err
			}

			if len(tc.Projects) > 0 && len(cfg.Projects) == 0 {
				cfg.Projects = trimProjects(tc.Projects)
			}
			if tc.DownloadDir != "" {
				cfg.DownloadDir = tc.DownloadDir
			}
		}
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
