package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"

	"lazygcs/internal/config"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all E2E tests
	tmpDir, err := os.MkdirTemp("", "lazygcs-e2e-")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "lazygcs")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "main.go")
	if err := buildCmd.Run(); err != nil {
		fmt.Printf("failed to build binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestMain_NoConfig(t *testing.T) {
	// Run without config (pointing to a non-existent file via env)
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "LAZYGCS_CONFIG=/tmp/non-existent-lazygcs-config.toml")
	output, err := cmd.CombinedOutput()

	// Should fail
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(string(output), "Failed to load config"))
}

func TestMain_EmptyProjects(t *testing.T) {
	// Create config with empty projects
	configPath := createConfigFile(t, []string{}, t.TempDir())

	// Run with config
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "LAZYGCS_CONFIG="+configPath)
	output, err := cmd.CombinedOutput()

	// Should fail
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(string(output), "No project IDs found in config file"))
}

func createConfigFile(t *testing.T, projects []string, downloadDir string) string {
	t.Helper()
	var quoted []string
	for _, p := range projects {
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	content := fmt.Sprintf("projects = [%s]\ndownload_dir = %q\n", strings.Join(quoted, ", "), downloadDir)

	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(content), 0644)
	assert.NilError(t, err)
	return path
}

func setupTestApp(t *testing.T, server *fakestorage.Server, projectIDs []string, downloadDir string) *teatest.TestModel {
	t.Helper()

	configPath := createConfigFile(t, projectIDs, downloadDir)
	cfg, err := config.Load(configPath)
	assert.NilError(t, err)

	gcsClient := gcs.NewClient(server.Client())
	m := tui.NewModel(cfg.Projects, gcsClient, cfg.DownloadDir, cfg.FuzzySearch)

	tm := teatest.NewTestModel(t, m)
	return tm
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for file %s", path)
}

func TestListBuckets(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "test-bucket-1",
					Name:       "init",
				},
				Content: []byte("hi"),
			},
		},
		Host:   "127.0.0.1",
		Port:   8081,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	tm := setupTestApp(t, server, []string{"test-project-1"}, t.TempDir())
	t.Cleanup(func() {
		tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)
}

func TestDownloadObject_E2E(t *testing.T) {
	content := []byte("download test content")
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "test-bucket-1",
					Name:       "file_to_dl.txt",
				},
				Content: content,
			},
		},
		Host:   "127.0.0.1",
		Port:   8088,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	downloadDir := t.TempDir()

	tm := setupTestApp(t, server, []string{"test-project-1"}, downloadDir)
	t.Cleanup(func() {
		tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	// Wait for bucket
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Enter bucket
	tm.Type("l")

	// Wait for object
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "file_to_dl.txt")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Download object
	tm.Type("d")

	// Wait for downloaded to show on the screen just in case we need to give it more time or see what it looks like
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "Downloaded to")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Check file was downloaded
	expectedPath := filepath.Join(downloadDir, "file_to_dl.txt")
	assert.NilError(t, waitForFile(expectedPath, 3*time.Second))

	b, err := os.ReadFile(expectedPath)
	assert.NilError(t, err)
	assert.Equal(t, string(b), string(content))
}

func TestPreviewObject_E2E(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"},
				Content:     []byte("content1"),
			},
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file2.txt"},
				Content:     []byte("content2"),
			},
		},
		Host:   "127.0.0.1",
		Port:   8089,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	tm := setupTestApp(t, server, []string{"test-project-1"}, t.TempDir())
	t.Cleanup(func() {
		tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	// Enter bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") })
	tm.Type("l")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") })

	// Move to second file and check for its preview
	tm.Type("j")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "content2")
	}, teatest.WithDuration(3*time.Second))
}

func TestDownloadObject_E2E_MultiSelect(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "file1.txt"},
				Content:     []byte("content1"),
			},
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "file2.txt"},
				Content:     []byte("content2"),
			},
		},
		Host:   "127.0.0.1",
		Port:   8090,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	downloadDir := t.TempDir()

	tm := setupTestApp(t, server, []string{"test-project-1"}, downloadDir)
	t.Cleanup(func() {
		tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	// Wait for bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "test-bucket-1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Select file1
	tm.Type(" ")
	
	// Move to file2 and select it
	tm.Type("j")
	tm.Type(" ")

	// Download
	tm.Type("d")

	// Wait for download to finish
	expectedPath1 := filepath.Join(downloadDir, "file1.txt")
	expectedPath2 := filepath.Join(downloadDir, "file2.txt")
	
	assert.NilError(t, waitForFile(expectedPath1, 3*time.Second))
	assert.NilError(t, waitForFile(expectedPath2, 3*time.Second))

	b1, err := os.ReadFile(expectedPath1)
	assert.NilError(t, err)
	assert.Equal(t, string(b1), "content1")

	b2, err := os.ReadFile(expectedPath2)
	assert.NilError(t, err)
	assert.Equal(t, string(b2), "content2")
}
