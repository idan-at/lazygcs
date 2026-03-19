// Package testutil provides common testing utilities for lazygcs.
package testutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/muesli/termenv"
	"gotest.tools/v3/assert"

	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
)

// SetupGCSMock creates a fake GCS server and returns it along with a lazygcs client.
func SetupGCSMock(t *testing.T, initialObjects []fakestorage.Object, port uint16) (*fakestorage.Server, *gcs.Client) {
	t.Helper()

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: initialObjects,
		Host:           "127.0.0.1",
		Port:           port,
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	gcsClient := gcs.NewClient(server.Client())
	return server, gcsClient
}

// CreateConfigFile creates a temporary config file for tests.
func CreateConfigFile(t *testing.T, projects []string, downloadDir string) string {
	t.Helper()
	var quoted []string
	for _, p := range projects {
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	content := fmt.Sprintf("projects = [%s]\ndownload_dir = %q\n", strings.Join(quoted, ", "), downloadDir)

	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(content), 0600)
	assert.NilError(t, err)
	return path
}

// SetupTestApp initializes the full TUI application using a fake GCS server.
func SetupTestApp(t *testing.T, initialObjects []fakestorage.Object, port uint16, projectIDs []string, downloadDir string) *teatest.TestModel {
	t.Helper()

	// Ensure tests produce deterministic colored output regardless of environment
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: initialObjects,
		Host:           "127.0.0.1",
		Port:           port,
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	configPath := CreateConfigFile(t, projectIDs, downloadDir)
	cfg, err := config.Load(configPath)
	assert.NilError(t, err)

	gcsClient := gcs.NewClient(server.Client())
	m := tui.NewModel(cfg.Projects, gcsClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)

	tm := teatest.NewTestModel(t, m)
	t.Cleanup(func() {
		_ = tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	return tm
}

// CreateMockTar creates a tar archive in memory with the given files map (name -> content).
func CreateMockTar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// CreateMockZip creates a zip archive in memory with the given files map (name -> content).
func CreateMockZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// WaitForFile waits until the specified file path exists, up to the given timeout.
func WaitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for file %s", path)
}
