package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all E2E tests
	tmpDir, err := os.MkdirTemp("", "lazygcs-e2e-")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binaryPath = filepath.Join(tmpDir, "lazygcs")

	// tests run in the 'tests' directory, so we point to the main.go in the parent dir
	// #nosec G204
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../main.go")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Printf("failed to build binary: %v\noutput:\n%s\n", err, output)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
func TestMain_E2E(t *testing.T) {
	// 1. Setup a valid config file for the binary
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `projects = ["test-project"]
download_dir = "/tmp"
`
	err := os.WriteFile(configPath, []byte(content), 0600)
	assert.NilError(t, err)

	// 2. Set the config environment variable
	assert.NilError(t, os.Setenv("LAZYGCS_CONFIG", configPath))
	t.Cleanup(func() { _ = os.Unsetenv("LAZYGCS_CONFIG") })

	// We want to ensure it doesn't pick up real credentials from the environment or well-known locations
	assert.NilError(t, os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/non-existent-path.json"))
	t.Cleanup(func() { _ = os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS") })

	// Also override HOME to avoid finding gcloud credentials
	origHome := os.Getenv("HOME")
	assert.NilError(t, os.Setenv("HOME", tmpDir))
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })

	// 3. Start the binary
	cmd := exec.Command(binaryPath)

	// We capture stdout/err to see what it printed.
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	// Run it to completion - it should fail fast on GCS client creation
	err = cmd.Run()
	output := combined.String()

	// It should fail with GCS client error because we provided a non-existent credentials path
	assert.Assert(t, err != nil, "should fail due to missing credentials")
	assert.Check(t, strings.Contains(output, "failed to create GCS client"))
	assert.Check(t, !strings.Contains(output, "failed to load config"))
}

func TestMain_NoConfig(t *testing.T) {
	assert.NilError(t, os.Setenv("LAZYGCS_CONFIG", "/tmp/non-existent-lazygcs-config.toml"))
	t.Cleanup(func() { _ = os.Unsetenv("LAZYGCS_CONFIG") })

	cmd := exec.Command(binaryPath)
	err := cmd.Run()

	// Should fail with a config error because default config won't exist in the test environment (unless the user has one)
	// We can't guarantee what err will be, but it should fail.
	assert.Assert(t, err != nil)
}

func TestVersionFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--version")
	err := cmd.Run()

	// Should succeed
	assert.NilError(t, err)
}
