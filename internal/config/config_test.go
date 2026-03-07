package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"lazygcs/internal/config"
)

// Helper to create a temp config file
func createConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	err := os.WriteFile(path, []byte(content), 0644)
	assert.NilError(t, err)
	return path
}

func TestLoad_NoFile(t *testing.T) {
	_, err := config.Load("non-existent.toml")
	assert.Assert(t, err != nil)
}

func TestLoad_ConfigFile(t *testing.T) {
	configFile := createConfigFile(t, `projects = ["p1", "p2"]`)

	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_ConfigFileWithWhitespace(t *testing.T) {
	configFile := createConfigFile(t, `projects = [" p1 ", " p2 "]`)

	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_DefaultDownloadDir(t *testing.T) {
	configFile := createConfigFile(t, `projects = ["p1"]`)
	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	home, err := os.UserHomeDir()
	assert.NilError(t, err)
	expectedDir := filepath.Join(home, "Downloads")

	assert.Equal(t, cfg.DownloadDir, expectedDir)
}

func TestLoad_OverrideDownloadDir(t *testing.T) {
	configFile := createConfigFile(t, `download_dir = "/tmp/custom_downloads"`)

	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	assert.Equal(t, cfg.DownloadDir, "/tmp/custom_downloads")
}

func TestLoad_DefaultFuzzySearch(t *testing.T) {
	configFile := createConfigFile(t, `projects = ["p1"]`)
	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	assert.Equal(t, cfg.FuzzySearch, false)
}

func TestLoad_OverrideFuzzySearch(t *testing.T) {
	configFile := createConfigFile(t, `
projects = ["p1"]
fuzzy_search = true
`)

	cfg, err := config.Load(configFile)
	assert.NilError(t, err)

	assert.Equal(t, cfg.FuzzySearch, true)
}
