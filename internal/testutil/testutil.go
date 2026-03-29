// Package testutil provides common testing utilities for lazygcs.
package testutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
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

// MockProjectGCSClient wraps a real GCS client but mocks the project metadata calls.
type MockProjectGCSClient struct {
	tui.GCSClient
	Projects     map[string]*gcs.ProjectMetadata
	ProjectError error
}

// GetProjectMetadata returns mocked project metadata or a default if not found.
func (m *MockProjectGCSClient) GetProjectMetadata(_ context.Context, projectID string) (*gcs.ProjectMetadata, error) {
	if m.ProjectError != nil {
		return nil, m.ProjectError
	}
	if p, ok := m.Projects[projectID]; ok {
		return p, nil
	}
	// Return a default mock to avoid CRM service initialization errors in tests.
	return &gcs.ProjectMetadata{
		ProjectID: projectID,
		Name:      "Mock Project " + projectID,
	}, nil
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
func SetupTestApp(t *testing.T, initialObjects []fakestorage.Object, port uint16, projectIDs []string, downloadDir string) (*teatest.TestModel, *fakestorage.Server) {
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
	mockClient := &MockProjectGCSClient{GCSClient: gcsClient}
	m := tui.NewModel(cfg.Projects, mockClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)

	tm := teatest.NewTestModel(t, &m)
	// Set the message sender so async updates work in teatest
	m.SetSendMsg(tm.Send)
	t.Cleanup(func() {
		_ = tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	return tm, server
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
