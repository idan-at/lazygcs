package main

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"

	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/testutil"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)
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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, downloadDir)

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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, downloadDir)

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

func TestSearch(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-2", Name: "init"}, Content: []byte("hi")},
	}
	tm := testutil.SetupTestApp(t, objects, 0, []string{"test-project-1"}, t.TempDir())

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "test-bucket-1") && strings.Contains(string(bts), "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))

	// Search for test-bucket-1
	tm.Type("/")
	time.Sleep(100 * time.Millisecond) // UI transition to search
	tm.Type("bucket-1")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Enter to finish search mode

	// Force a full redraw
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Verify only test-bucket-1 is visible
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "test-bucket-1") && !strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))
}

func TestNavigationUp(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}, Content: []byte("hi")},
	}
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket b1
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Enter folder1/
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket root
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket list
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
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

	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, downloadDir)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Attempt download
	tm.Type("d")
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Toggle help
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "HELP") }, teatest.WithDuration(3*time.Second))

	// Toggle help off
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return !strings.Contains(string(bts), "HELP") }, teatest.WithDuration(3*time.Second))
}

func TestJumpTopBottom(t *testing.T) {
	var objects []fakestorage.Object
	for i := 1; i <= 50; i++ {
		objects = append(objects, fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: fmt.Sprintf("file%02d.txt", i)},
			Content:     []byte("hi"),
		})
	}
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

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
	tm.Type("l") // Enter folder2/
	// Wait for folder3/
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder3/") }, teatest.WithDuration(3*time.Second))
	tm.Type("l") // Enter folder3/

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

	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, downloadDir)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Navigate to file
	tm.Type("j")
	tm.Type("l")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// 1. Test Abort
	tm.Type("d")
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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Mock ExecCommand
	originalExec := tui.ExecCommand
	defer func() { tui.ExecCommand = originalExec }()

	var capturedCmd string
	var capturedArgs []string
	tui.ExecCommand = func(name string, arg ...string) *exec.Cmd {
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
		if capturedCmd != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify command was triggered
	assert.Assert(t, capturedCmd == "open" || capturedCmd == "xdg-open" || capturedCmd == "rundll32")
	assert.Assert(t, len(capturedArgs) > 0)
	assert.Assert(t, strings.Contains(capturedArgs[len(capturedArgs)-1], "file1.txt"))
}

func TestRefresh(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "init"}, Content: []byte("hi")},
	}

	server, client := testutil.SetupGCSMock(t, objects, 0)
	configPath := testutil.CreateConfigFile(t, []string{"p1"}, t.TempDir())
	cfg, _ := config.Load(configPath)

	m := tui.NewModel(cfg.Projects, client, cfg.DownloadDir, cfg.FuzzySearch, cfg.Icons)
	m.SetDeterministicSpinner(true)
	tm := teatest.NewTestModel(t, m)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Add new bucket directly to server
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "new-bucket"})

	// Refresh
	tm.Type("r")

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
	tm := testutil.SetupTestApp(t, objects, 0, []string{"p1"}, t.TempDir())

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
