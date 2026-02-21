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

// FallbackFunc is a function that returns a fallback project ID.
type FallbackFunc func() string

// Load resolves the configuration based on the precedence hierarchy:
// 1. CLI Arguments
// 2. LAZYGCS_PROJECTS env var
// 3. GOOGLE_CLOUD_PROJECT env var
// 4. Config File (TOML)
// 5. Fallback Provider
func Load(args []string, configPath string, fallback FallbackFunc) (*Config, error) {
	// 1. CLI Arguments
	if len(args) > 0 {
		return &Config{Projects: args}, nil
	}

	// 2. LAZYGCS_PROJECTS env var
	if p := os.Getenv("LAZYGCS_PROJECTS"); p != "" {
		projects := strings.Split(p, ",")
		return &Config{Projects: projects}, nil
	}

	// 3. GOOGLE_CLOUD_PROJECT env var
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return &Config{Projects: []string{p}}, nil
	}

	// 4. Config File
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			var tc tomlConfig
			if _, err := toml.DecodeFile(configPath, &tc); err == nil {
				if len(tc.Projects) > 0 {
					return &Config{Projects: tc.Projects}, nil
				}
			}
		}
	}

	// 5. Fallback
	if fallback != nil {
		if p := fallback(); p != "" {
			return &Config{Projects: []string{p}}, nil
		}
	}

	return &Config{Projects: []string{}}, nil
}
