package config

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the application's runtime configuration.
type Config struct {
	Projects []string
}

// tomlConfig represents the structure of the config.toml file.
type tomlConfig struct {
	Projects []string `toml:"projects"`
}

// Load resolves the configuration based on the precedence hierarchy:
// 1. CLI Arguments
// 2. Config File (TOML)
func Load(args []string, configPath string) (*Config, error) {
	// 1. CLI Arguments
	if len(args) > 0 {
		return &Config{Projects: trimProjects(args)}, nil
	}

	// 2. Config File
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			var tc tomlConfig
			if _, err := toml.DecodeFile(configPath, &tc); err == nil {
				if len(tc.Projects) > 0 {
					return &Config{Projects: trimProjects(tc.Projects)}, nil
				}
			}
		}
	}

	return &Config{Projects: []string{}}, nil
}

// trimProjects trims whitespace from each project ID and filters out empty strings.
func trimProjects(raw []string) []string {
	var clean []string
	for _, p := range raw {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean
}
