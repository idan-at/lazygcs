package main

import (
	"bytes"
	"errors"
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
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, exitError.ExitCode(), 1)
	}
	assert.Check(t, strings.Contains(output, "failed to create GCS client"))
	assert.Check(t, !strings.Contains(output, "failed to load config"))
	assert.Check(t, !strings.Contains(output, "panic:")) // No raw stack trace
}

func TestMain_NoConfig(t *testing.T) {
	// Set an invalid config path to force failure
	assert.NilError(t, os.Setenv("LAZYGCS_CONFIG", "/tmp/non-existent-lazygcs-config.toml"))
	t.Cleanup(func() { _ = os.Unsetenv("LAZYGCS_CONFIG") })

	cmd := exec.Command(binaryPath)
	output, err := cmd.CombinedOutput()

	// Should fail with a config error
	assert.Assert(t, err != nil)
	assert.Check(t, strings.Contains(string(output), "failed to load config"))
}

func TestVersionFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()

	// Should succeed
	assert.NilError(t, err)

	// Ensure the output format is correct
	assert.Assert(t, strings.HasPrefix(string(output), "lazygcs "))
	assert.Assert(t, strings.HasSuffix(string(output), "\n"))
}

func TestHelpFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()

	// Should succeed (returns nil in run() when flag.ErrHelp)
	assert.NilError(t, err)

	// Ensure the output contains usage info
	assert.Assert(t, strings.Contains(string(output), "Usage:"))
	assert.Assert(t, strings.Contains(string(output), "Flags:"))
	assert.Assert(t, strings.Contains(string(output), "Controls:"))
}

func TestInitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	customDownloadDir := filepath.Join(tmpDir, "custom_downloads")

	// Ensure config doesn't exist yet
	_, err := os.Stat(configPath)
	assert.Assert(t, os.IsNotExist(err))

	// Set LAZYGCS_CONFIG to the temp path
	t.Setenv("LAZYGCS_CONFIG", configPath)

	// #nosec G204
	cmd := exec.Command(binaryPath, "init", "--project", "p1", "--project", "p2", "--download-dir", customDownloadDir, "--nerd-icons")
	output, err := cmd.CombinedOutput()

	// Should succeed
	assert.NilError(t, err, "output: "+string(output))

	// Output should confirm creation
	assert.Check(t, strings.Contains(string(output), "Config initialized"))

	// Verify the config file was created
	// #nosec G304
	content, err := os.ReadFile(configPath)
	assert.NilError(t, err)

	configStr := string(content)
	assert.Check(t, strings.Contains(configStr, `projects = ["p1", "p2"]`))
	assert.Check(t, strings.Contains(configStr, fmt.Sprintf(`download_dir = "%s"`, customDownloadDir)))
	assert.Check(t, strings.Contains(configStr, `nerd_icons = true`))
}

func TestInitCommand_NoProjects(t *testing.T) {
	cmd := exec.Command(binaryPath, "init")
	output, err := cmd.CombinedOutput()

	// Should fail
	assert.Assert(t, err != nil, "init should fail without projects")
	assert.Check(t, strings.Contains(string(output), "at least one --project is required"))
}
