package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	err := os.WriteFile(configPath, []byte(content), 0644)
	assert.NilError(t, err)

	// 2. Set the config environment variable
	assert.NilError(t, os.Setenv("LAZYGCS_CONFIG", configPath))
	t.Cleanup(func() { _ = os.Unsetenv("LAZYGCS_CONFIG") })
	// Also need to set google credentials or the client will fail in true e2e.
	// We'll set a dummy value to bypass the missing credentials error and test
	// that it successfully started the process (it will fail to fetch, but that's okay for verifying the binary works)
	assert.NilError(t, os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/dev/null"))
	t.Cleanup(func() { _ = os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS") })

	// 3. Start the binary
	cmd := exec.Command(binaryPath)
	
	// We capture stdout to see what it printed before we kill it or it exits.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	assert.NilError(t, err, "binary should start successfully")

	// 4. Wait a tiny bit and kill it (since it's a TUI that blocks)
	time.Sleep(500 * time.Millisecond)
	err = cmd.Process.Signal(os.Interrupt)
	assert.NilError(t, err, "should be able to interrupt process")

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for process to exit
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit in time")
	}
	
	output := stdout.String()
	// As long as it started running BubbleTea (which clears screen etc) or failed cleanly on GCS auth, it's a pass.
	// Since we passed /dev/null for creds, it will likely return a known error:
	assert.Assert(t, err != nil)
	assert.Check(t, !strings.Contains(output, "failed to load config"))
	assert.Assert(t, strings.Contains(output, "no project IDs found") == false)
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