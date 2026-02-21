package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"lazygcs/internal/config"
	"gotest.tools/v3/assert"
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

func TestLoad_CLIArgsPrecedence(t *testing.T) {
	t.Setenv("LAZYGCS_PROJECTS", "env-project")
	configFile := createConfigFile(t, `projects = ["file-project"]`)

	cfg, err := config.Load([]string{"cli-project"}, configFile, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"cli-project"})
}

func TestLoad_CLIArgsWithWhitespace(t *testing.T) {
	// Simulate args with spaces (e.g. from shell expansion issues)
	cfg, err := config.Load([]string{" p1 ", "p2 "}, "", nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_LazyGCSProjectsEnvVarPrecedence(t *testing.T) {
	t.Setenv("LAZYGCS_PROJECTS", "p1,p2")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "gcp-project")
	configFile := createConfigFile(t, `projects = ["file-project"]`)

	cfg, err := config.Load(nil, configFile, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_LazyGCSProjectsEnvVarWithSpaces(t *testing.T) {
	t.Setenv("LAZYGCS_PROJECTS", " p1 ,  p2  ")
	cfg, err := config.Load(nil, "", nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_GoogleCloudProjectEnvVarPrecedence(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "gcp-project")
	configFile := createConfigFile(t, `projects = ["file-project"]`)

	cfg, err := config.Load(nil, configFile, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"gcp-project"})
}

func TestLoad_ConfigFile(t *testing.T) {
	configFile := createConfigFile(t, `projects = ["p1", "p2"]`)

	cfg, err := config.Load(nil, configFile, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_ConfigFileWithWhitespace(t *testing.T) {
	configFile := createConfigFile(t, `projects = [" p1 ", " p2 "]`)

	cfg, err := config.Load(nil, configFile, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"p1", "p2"})
}

func TestLoad_Fallback(t *testing.T) {
	mockFallback := func() string {
		return "fallback-project"
	}

	cfg, err := config.Load(nil, "", mockFallback)
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Projects, []string{"fallback-project"})
}
