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

// Load resolves the configuration from the specified TOML file.
//
// Arguments:
//   - configPath: Absolute path to the TOML configuration file.
//
// Returns:
//   - *Config: The resolved configuration.
//   - error: If the config file does not exist, or cannot be parsed.
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		return nil, os.ErrNotExist
	}

	if _, err := os.Stat(configPath); err != nil {
		return nil, err
	}

	cfg := &Config{
		Projects:    []string{},
		DownloadDir: defaultDownloadDir(),
	}

	var tc tomlConfig
	if _, err := toml.DecodeFile(configPath, &tc); err != nil {
		return nil, err
	}

	if len(tc.Projects) > 0 {
		cfg.Projects = trimProjects(tc.Projects)
	}
	if tc.DownloadDir != "" {
		cfg.DownloadDir = tc.DownloadDir
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
