package main

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/muesli/termenv"
	"google.golang.org/api/iterator"

	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/testutil"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

var ansiRegexp = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

func getLastFrame(bts []byte) string {
	s := ansiRegexp.ReplaceAllString(string(bts), "")
	// Frames start with the header "gs://".
	// We look for the last occurrence of this to isolate the latest frame.
	idx := strings.LastIndex(s, "gs://")
	if idx != -1 {
		return s[idx:]
	}

	// Fallback to "Buckets" if "gs://" is somehow missing
	idx = strings.LastIndex(s, "Buckets")
	if idx != -1 {
		return s[idx:]
	}

	return s
}

func TestListBuckets(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "test-bucket-1",
				Name:       "init",
			},
			Content: []byte("hi"),
		},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)
}

func TestBucketMetadataPreview(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "metadata-bucket",
				Name:       "init",
			},
		},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	// Wait for the bucket to appear in the list
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "metadata-bucket")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Move cursor down to the bucket (index 1, since index 0 is the project)
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "Storage Class:")
		},
		teatest.WithDuration(3*time.Second),
	)

	tm.Send(tea.Quit())
	finalView := tm.FinalModel(t).(*tui.Model).View()
	assert.Assert(t, strings.Contains(finalView, "Storage Class:"), "Final view:\n%s", finalView)
}

func TestDownloadObject(t *testing.T) {
	content := []byte("download test content")
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "test-bucket-1",
				Name:       "file_to_dl.txt",
			},
			Content: content,
		},
	}
	downloadDir := t.TempDir()
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, downloadDir)

	// Wait for bucket
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Move cursor down to first bucket
	tm.Type("j")

	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
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

	// Wait for downloaded to show on the screen
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
	assert.NilError(t, testutil.WaitForFile(expectedPath, 3*time.Second))

	// #nosec G304
	b, err := os.ReadFile(expectedPath)
	assert.NilError(t, err)
	assert.Equal(t, string(b), string(content))
}

func TestDownloadObject_MultiSelect(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "file1.txt"},
			Content:     []byte("content1"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "folder1/file2.txt"},
			Content:     []byte("content2"),
		},
	}
	downloadDir := t.TempDir()
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, downloadDir)

	// Wait for bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "test-bucket-1") }, teatest.WithDuration(3*time.Second))

	// Move cursor down to first bucket
	tm.Type("j")

	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Select folder1/
	tm.Type(" ")

	// Move to file1.txt and select it
	tm.Type("j")
	tm.Type(" ")

	// Download
	tm.Type("d")

	// Wait for download to finish
	expectedPath1 := filepath.Join(downloadDir, "file1.txt")
	expectedPathZip := filepath.Join(downloadDir, "folder1.zip")

	assert.NilError(t, testutil.WaitForFile(expectedPath1, 3*time.Second))
	assert.NilError(t, testutil.WaitForFile(expectedPathZip, 3*time.Second))

	// #nosec G304
	b1, err := os.ReadFile(expectedPath1)
	assert.NilError(t, err)
	assert.Equal(t, string(b1), "content1")

	// Check zip
	r, err := zip.OpenReader(expectedPathZip)
	assert.NilError(t, err)
	defer func() { _ = r.Close() }()

	var foundFile2 bool
	for _, f := range r.File {
		if f.Name == "file2.txt" {
			foundFile2 = true
		}
	}
	assert.Assert(t, foundFile2, "file2.txt should be in the zip")
}

func TestProjectInformationPreview(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-2", Name: "init"}, Content: []byte("hi")},
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: objects,
		Host:           "127.0.0.1",
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	cfgPath := testutil.CreateConfigFile(t, []string{"test-project-1"}, t.TempDir())
	cfg, _ := config.Load(cfgPath)

	realClient := gcs.NewClient(server.Client())
	mockClient := &testutil.MockProjectGCSClient{
		GCSClient: realClient,
		Projects: map[string]*gcs.ProjectMetadata{
			"test-project-1": {
				ProjectID:     "test-project-1",
				Name:          "Test Project",
				ProjectNumber: 123456789,
				CreateTime:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Labels:        map[string]string{"env": "prod"},
				ParentType:    "organization",
				ParentID:      "987654321",
			},
		},
	}

	m := tui.NewModel(cfg.Projects, mockClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)

	tm := teatest.NewTestModel(t, &m)
	m.SetSendMsg(tm.Send)

	// Wait for buckets to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		return strings.Contains(s, "test-bucket-1") && strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(5*time.Second))

	// Force cursor move to trigger project metadata fetch
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})

	// Force a full redraw
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Check if Project Information is displayed correctly
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		hasTitle := strings.Contains(s, "Project Information")
		hasProject := strings.Contains(s, "Project: test-project-1")
		hasName := strings.Contains(s, "Project Name: Test Project")
		hasNumber := strings.Contains(s, "Project Number: 123456789")
		hasCreated := strings.Contains(s, "Created: 2023-01-01 12:00:00")
		hasParent := strings.Contains(s, "Parent: organization (987654321)")
		hasLabel := strings.Contains(s, "env: prod")
		hasTotal := strings.Contains(s, "Total Buckets: 2")
		return hasTitle && hasProject && hasName && hasNumber && hasCreated && hasParent && hasLabel && hasTotal
	}, teatest.WithDuration(5*time.Second))
}

func TestProjectInformationPreview_Error(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: objects,
		Host:           "127.0.0.1",
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	cfgPath := testutil.CreateConfigFile(t, []string{"error-project"}, t.TempDir())
	cfg, _ := config.Load(cfgPath)

	realClient := gcs.NewClient(server.Client())
	mockClient := &testutil.MockProjectGCSClient{
		GCSClient:    realClient,
		ProjectError: fmt.Errorf("project not found"),
	}

	m := tui.NewModel(cfg.Projects, mockClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)

	tm := teatest.NewTestModel(t, &m)
	m.SetSendMsg(tm.Send)
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Add a slight delay for the message queue to process
	time.Sleep(100 * time.Millisecond)

	// Log the raw View() output
	t.Logf("Direct m.View() output:\n%s\n", m.View())

	// Wait for buckets view and ensure the project name and error are visible
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		return strings.Contains(s, "error-project") && strings.Contains(s, "Error: project not found")
	}, teatest.WithDuration(5*time.Second))
}

func TestSearch(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-2", Name: "init"}, Content: []byte("hi")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		return strings.Contains(s, "test-bucket-1") && strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(5*time.Second))

	// Search for test-bucket-1
	tm.Type("/")
	time.Sleep(200 * time.Millisecond) // UI transition to search
	tm.Type("bucket-1")
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Enter to finish search mode

	// Force a full redraw
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Verify only test-bucket-1 is visible
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		parts := strings.Split(s, "q quit")
		if len(parts) < 2 {
			return false
		}
		// The last complete frame is in the second to last part
		lastFrame := parts[len(parts)-2]
		return strings.Contains(lastFrame, "FILTER: bucket-1") && strings.Contains(lastFrame, "test-bucket-1") && !strings.Contains(lastFrame, "test-bucket-2")
	}, teatest.WithDuration(5*time.Second))
}

func TestNavigationUp(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}, Content: []byte("hi")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket b1
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Enter folder1/
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket root
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket list
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "Buckets") }, teatest.WithDuration(3*time.Second))
}

func TestDownloadOverwrite(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"},
			Content:     []byte("new content"),
		},
	}
	downloadDir := t.TempDir()
	filePath := filepath.Join(downloadDir, "file1.txt")
	err := os.WriteFile(filePath, []byte("old content"), 0600)
	assert.NilError(t, err)

	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, downloadDir)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Attempt download
	tm.Type("d")
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for overwrite prompt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "(o)verwrite")
	}, teatest.WithDuration(3*time.Second))

	// Confirm overwrite
	tm.Type("o")

	// Verify content
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// #nosec G304
		b, _ := os.ReadFile(filePath) //nolint:gosec // acceptable in tests
		if string(b) == "new content" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("file was not overwritten with new content")
}

func TestHelpMenu(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "init"}, Content: []byte("hi")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Toggle help
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "HELP") }, teatest.WithDuration(3*time.Second))

	// Toggle help off
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		lastFrame := getLastFrame(bts)
		return !strings.Contains(lastFrame, "HELP")
	}, teatest.WithDuration(3*time.Second))
}

func TestJumpTopBottom(t *testing.T) {
	var objects []fakestorage.Object
	for i := 1; i <= 50; i++ {
		objects = append(objects, fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: fmt.Sprintf("file%02d.txt", i)},
			Content:     []byte("hi"),
		})
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for objects
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file01.txt") }, teatest.WithDuration(3*time.Second))

	// Jump to bottom
	tm.Type("G")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file50.txt")
	}, teatest.WithDuration(2*time.Second))

	// Jump to top
	tm.Type("g")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file01.txt")
	}, teatest.WithDuration(2*time.Second))
}

func TestFastEscape(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/folder2/folder3/file1.txt"}, Content: []byte("hi")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Drill down
	tm.Type("j") // Move to b1
	tm.Type("l") // Enter b1
	// Wait for folder1/
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))
	tm.Type("l") // Enter folder1/
	// Wait for folder2/
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder2/") }, teatest.WithDuration(3*time.Second))
	tm.Type("l") // Enter folder1/
	// Wait for folder3/
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder3/") }, teatest.WithDuration(3*time.Second))
	tm.Type("l") // Enter folder1/

	// Wait for file1.txt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Fast escape to bucket list
	tm.Type("H")

	// Verify we are back at Buckets view
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Buckets") && strings.Contains(string(bts), "b1")
	}, teatest.WithDuration(3*time.Second))
}

func TestDownloadAbortRename(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"}, Content: []byte("content")},
	}
	downloadDir := t.TempDir()
	filePath := filepath.Join(downloadDir, "file1.txt")
	err := os.WriteFile(filePath, []byte("old"), 0600)
	assert.NilError(t, err)

	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, downloadDir)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Navigate to file
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// 1. Test Abort
	tm.Type("d")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "(a)bort") }, teatest.WithDuration(2*time.Second))
	tm.Type("a")
	time.Sleep(100 * time.Millisecond)

	// Verify old content still exists
	b, _ := os.ReadFile(filePath) //nolint:gosec // acceptable in tests
	assert.Equal(t, string(b), "old")

	// 2. Test Rename
	tm.Type("d")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "(r)ename") }, teatest.WithDuration(2*time.Second))
	tm.Type("r")

	// Verify renamed file exists
	renamePath := filepath.Join(downloadDir, "file1_1.txt")
	assert.NilError(t, testutil.WaitForFile(renamePath, 3*time.Second))
	b2, _ := os.ReadFile(renamePath) //nolint:gosec // acceptable in tests
	assert.Equal(t, string(b2), "content")
}

func TestOpen(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"}, Content: []byte("content")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Mock ExecCommand
	originalExec := tui.ExecCommand
	defer func() { tui.ExecCommand = originalExec }()

	var mu sync.Mutex
	var capturedCmd string
	var capturedArgs []string
	tui.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		mu.Lock()
		defer mu.Unlock()
		capturedCmd = name
		capturedArgs = arg
		// Return a command that does nothing
		return exec.Command("true")
	}

	// Navigate to file
	tm.Type("j")
	tm.Type("l")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Open file
	tm.Type("o")

	// Wait for captured command
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		cmd := capturedCmd
		mu.Unlock()
		if cmd != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	finalCmd := capturedCmd
	finalArgs := capturedArgs
	mu.Unlock()

	// Verify command was triggered
	assert.Assert(t, finalCmd == "open" || finalCmd == "xdg-open" || finalCmd == "rundll32")
	assert.Assert(t, len(finalArgs) > 0)
	assert.Assert(t, strings.Contains(finalArgs[len(finalArgs)-1], "file1.txt"))
}

func TestRefresh(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "init"}, Content: []byte("hi")},
	}

	server, client := testutil.SetupGCSMock(t, objects, 0)
	configPath := testutil.CreateConfigFile(t, []string{"p1"}, t.TempDir())
	cfg, _ := config.Load(configPath)

	m := tui.NewModel(cfg.Projects, client, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)
	tm := teatest.NewTestModel(t, &m)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Add new bucket directly to server
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "new-bucket"})

	// Refresh
	tm.Type("R")

	// Wait for new bucket to appear
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "new-bucket")
	}, teatest.WithDuration(5*time.Second))
}

func TestPaging(t *testing.T) {
	var objects []fakestorage.Object
	for i := 1; i <= 100; i++ {
		objects = append(objects, fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: fmt.Sprintf("file%03d.txt", i)},
			Content:     []byte("hi"),
		})
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Navigate to bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30}) // Fixed height for predictable paging

	// Wait for objects
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file001.txt") }, teatest.WithDuration(3*time.Second))

	// Page Down (Ctrl+F)
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlF})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		// With height 30, approx 20 items per page
		return strings.Contains(string(bts), "file02")
	}, teatest.WithDuration(2*time.Second))

	// Page Up (Ctrl+B)
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlB})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file001.txt")
	}, teatest.WithDuration(2*time.Second))

	// Half Page Down (Ctrl+D)
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file01")
	}, teatest.WithDuration(2*time.Second))

	// Half Page Up (Ctrl+U)
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file001.txt")
	}, teatest.WithDuration(2*time.Second))
}

func TestResizePreview(t *testing.T) {
	longText := strings.Repeat("A", 100)
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file.txt"}, Content: []byte(longText)},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file.txt") }, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 350, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), strings.Repeat("A", 100))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 50, Height: 20})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		lastFrame := getLastFrame(bts)
		return !strings.Contains(lastFrame, strings.Repeat("A", 100)) && strings.Contains(lastFrame, "A")
	}, teatest.WithDuration(3*time.Second))
}

func TestEditorFinishedMsg(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file.txt"}, Content: []byte("content")},
	}
	tm, _ := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file.txt") }, teatest.WithDuration(3*time.Second))

	originalExec := tui.ExecCommand
	defer func() { tui.ExecCommand = originalExec }()
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "content") }, teatest.WithDuration(3*time.Second))

	tm.Type("e")
	time.Sleep(500 * time.Millisecond)

	tm.Send(tui.EditorFinishedMsg{Err: fmt.Errorf("editor failed")})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "editor failed")
	}, teatest.WithDuration(3*time.Second))
}

func TestProjectLabelsSorted(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: objects,
		Host:           "127.0.0.1",
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	cfgPath := testutil.CreateConfigFile(t, []string{"test-project-1"}, t.TempDir())
	cfg, _ := config.Load(cfgPath)

	realClient := gcs.NewClient(server.Client())
	mockClient := &testutil.MockProjectGCSClient{
		GCSClient: realClient,
		Projects: map[string]*gcs.ProjectMetadata{
			"test-project-1": {
				ProjectID: "test-project-1",
				Name:      "Test Project",
				Labels: map[string]string{
					"zebra": "final",
					"apple": "first",
					"mango": "middle",
				},
			},
		},
	}

	m := tui.NewModel(cfg.Projects, mockClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)
	m.SetDeterministicSpinner(true)

	tm := teatest.NewTestModel(t, &m)
	m.SetSendMsg(tm.Send)

	// Wait for buckets to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		return strings.Contains(s, "test-bucket-1")
	}, teatest.WithDuration(5*time.Second))

	// Force cursor move to trigger project metadata fetch
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})

	// Force a full redraw
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Check if Labels are sorted: apple, mango, zebra
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := ansiRegexp.ReplaceAllString(string(bts), "")
		appleIdx := strings.Index(s, "apple:")
		mangoIdx := strings.Index(s, "mango:")
		zebraIdx := strings.Index(s, "zebra:")

		return appleIdx != -1 && mangoIdx != -1 && zebraIdx != -1 &&
			appleIdx < mangoIdx && mangoIdx < zebraIdx
	}, teatest.WithDuration(5*time.Second))
}

func TestDeleteObject(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "delete-bucket",
				Name:       "file_to_delete.txt",
			},
			Content: []byte("delete me"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "delete-bucket",
				Name:       "keep_me.txt",
			},
			Content: []byte("keep me"),
		},
	}
	tm, server := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	// Wait for bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "delete-bucket")
	}, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for objects
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file_to_delete.txt")
	}, teatest.WithDuration(3*time.Second))

	// Delete object
	tm.Type("x")

	// Wait for confirmation prompt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "DELETE CONFIRMATION") &&
			strings.Contains(string(bts), "gs://delete-bucket/file_to_delete.txt")
	}, teatest.WithDuration(2*time.Second))

	// Confirm deletion
	tm.Type("y")

	// Wait for "Deleted" message OR for the object to disappear from the list
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		lastFrame := getLastFrame(bts)

		deletedMsg := strings.Contains(lastFrame, "Deleted file_to_delete.txt")
		notInList := !strings.Contains(lastFrame, "📄 file_to_delete.txt")

		return (deletedMsg || notInList) && strings.Contains(lastFrame, "keep_me.txt")
	}, teatest.WithDuration(10*time.Second))

	// Double check with server
	it := server.Client().Bucket("delete-bucket").Objects(context.Background(), nil)
	attrs, err := it.Next()
	assert.NilError(t, err)
	assert.Equal(t, attrs.Name, "keep_me.txt")
}

func TestDeleteBucket(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "keep-bucket",
				Name:       "init",
			},
		},
	}
	tm, server := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	// Create an empty bucket directly on the server
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "bucket-to-delete"})

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "bucket-to-delete") &&
			strings.Contains(string(bts), "keep-bucket")
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for stable state
	time.Sleep(200 * time.Millisecond)

	// Move to bucket-to-delete (it's the first bucket, after the project header)
	// Project is at 0, first bucket at 1
	tm.Type("j")

	// Wait to ensure cursor moved
	time.Sleep(100 * time.Millisecond)

	// Delete bucket
	tm.Type("x")

	// Wait for confirmation prompt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "DELETE CONFIRMATION") &&
			strings.Contains(string(bts), "gs://bucket-to-delete")
	}, teatest.WithDuration(2*time.Second))

	// Confirm deletion
	tm.Type("y")

	// Wait for "Deleted" message OR for the bucket to disappear from the list
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		lastFrame := getLastFrame(bts)

		deletedMsg := strings.Contains(lastFrame, "Deleted bucket-to-delete")
		notInList := !strings.Contains(lastFrame, "📦 bucket-to-delete")

		return (deletedMsg || notInList) && strings.Contains(lastFrame, "keep-bucket")
	}, teatest.WithDuration(10*time.Second))

	// Double check with server
	_, err := server.Client().Bucket("bucket-to-delete").Attrs(context.Background())
	assert.Assert(t, err != nil)
}

func TestDeleteMultiSelect(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "multi-delete-bucket", Name: "file1.txt"}, Content: []byte("c1")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "multi-delete-bucket", Name: "file2.txt"}, Content: []byte("c2")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "multi-delete-bucket", Name: "keep.txt"}, Content: []byte("keep")},
	}
	tm, server := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "multi-delete-bucket")
	}, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	time.Sleep(100 * time.Millisecond)
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Wait for objects
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "file1.txt")
	}, teatest.WithDuration(3*time.Second))

	// Select file1.txt
	tm.Type(" ")
	// Move to file2.txt and select it
	tm.Type("j")
	tm.Type(" ")

	// Delete selected
	tm.Type("x")

	// Wait for confirmation prompt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "DELETE CONFIRMATION") &&
			strings.Contains(string(bts), "2 selected items")
	}, teatest.WithDuration(2*time.Second))

	// Confirm deletion
	tm.Type("y")
	time.Sleep(500 * time.Millisecond)

	// Wait for objects to disappear
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		lastFrame := getLastFrame(bts)

		// Specifically look for the list items being gone in the LATEST frame
		hasFile1 := strings.Contains(lastFrame, "file1.txt")
		hasFile2 := strings.Contains(lastFrame, "file2.txt")
		hasKeep := strings.Contains(lastFrame, "keep.txt")
		return !hasFile1 && !hasFile2 && hasKeep
	}, teatest.WithDuration(10*time.Second))

	// Verify on server
	it := server.Client().Bucket("multi-delete-bucket").Objects(context.Background(), nil)
	attrs, err := it.Next()
	assert.NilError(t, err)
	assert.Equal(t, attrs.Name, "keep.txt")
	_, err = it.Next()
	assert.Equal(t, err, iterator.Done)
}
