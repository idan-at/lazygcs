package tui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_Actions_Refresh(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client
	m = enterBucket(m, projects, "b1", objects)

	// Verify we have 1 object
	assert.Equal(t, len(m.Objects()), 1)

	// Press 'r' to refresh
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.Assert(t, cmd != nil)

	// Simulate receipt of refreshed objects (same object list)
	msg := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg)

	// BUG: If duplicated, length will be 2. It should remain 1.
	assert.Equal(t, len(m.Objects()), 1, "Objects should not be duplicated after refresh")
}

type mockClipboard struct {
	content string
}

func (c *mockClipboard) WriteAll(text string) error {
	c.content = text
	return nil
}

func TestModel_Actions_CopyURI(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	cb := &mockClipboard{}
	m.SetClipboard(cb)

	// 1. Bucket View Copy
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = pressKey(m, 'j') // hover b1

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Equal(t, cb.content, "gs://b1/")
	assert.Assert(t, strings.Contains(m.View(), "Copied gs://b1/ to clipboard"))

	// 2. Object View Copy
	m = enterBucket(m, projects, "b1", objects)
	m.SetClipboard(cb)
	m, _ = pressKey(m, 'j') // hover obj1

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Equal(t, cb.content, "gs://b1/obj1")
	assert.Assert(t, strings.Contains(m.View(), "Copied gs://b1/obj1 to clipboard"))

	// 3. Multi-select Copy
	m, _ = pressKey(m, ' ') // select obj1
	// Add another object
	objects2 := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: objects2})
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ') // select obj2

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	// Check content (order depends on map iteration, but both should be there)
	assert.Assert(t, strings.Contains(cb.content, "gs://b1/obj1"))
	assert.Assert(t, strings.Contains(cb.content, "gs://b1/obj2"))
	assert.Assert(t, strings.Contains(m.View(), "Copied 2 URIs to clipboard"))
}

func TestModel_Actions_Open(t *testing.T) {
	// Mock ExecCommand to avoid launching real applications
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover obj1

	// Press 'o' to open
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Opening obj1..."))

	// Resolve the command (which triggers the download and exec)
	msg := cmd()
	// FileOpenedMsg is returned by openFile
	_, _ = updateModel(m, msg)

	// Verify mock client was called for download
	assert.Equal(t, client.lastDownload.Bucket, "b1")
	assert.Equal(t, client.lastDownload.Object, "obj1")
}

func TestModel_Actions_Edit(t *testing.T) {
	// Mock ExecCommand
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover obj1

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Opening obj1..."))

	// Simulate editor finishing with a modified file
	tempPath := filepath.Join(os.TempDir(), "lazygcs", "b1", "obj1")
	// Create the file so os.Stat doesn't fail in handleEditorFinishedMsg
	_ = os.MkdirAll(filepath.Dir(tempPath), 0750)
	_ = os.WriteFile(tempPath, []byte("updated content"), 0600)

	// Original time was likely before now
	originalTime := time.Now().Add(-1 * time.Hour)

	// Send EditorFinishedMsg
	m, cmd = updateModel(m, tui.EditorFinishedMsg{
		TempPath:        tempPath,
		OriginalModTime: originalTime,
		Err:             nil,
	})

	assert.Assert(t, cmd != nil, "Should trigger upload command")
	assert.Assert(t, strings.Contains(m.View(), "Uploading changes to obj1..."))

	// Resolve upload command
	msg := cmd()
	_, _ = updateModel(m, msg)

	// Verify mock client was called for upload
	assert.Equal(t, client.lastUpload.Bucket, "b1")
	assert.Equal(t, client.lastUpload.Object, "obj1")
	assert.Equal(t, client.lastUpload.Src, tempPath)
}

func TestModel_DownloadAction(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Downloading obj1..."), "View should show downloading status with filename")

	// Simulate download completion
	m, _ = updateModel(m, tui.DownloadMsg{Path: "/tmp/obj1"})

	assert.Assert(t, strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "View should show success status")
}

func TestModel_DownloadAction_MultiSelect(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1", "obj2", "obj3"}, nil),
	}
	downloadDir := t.TempDir()
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Select obj1
	m, _ = pressKey(m, ' ')

	// Move to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	assert.Assert(t, cmd != nil, "Cmd should not be nil")
	assert.Assert(t, strings.Contains(m.View(), "Downloading 1/2"), "View should show batch downloading progress")

	// With the queue system, the first command is a single download fetch
	msg := resolveFetchCmd(cmd)
	dl1, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected a tui.DownloadMsg for the first item")

	// Update model with the first download result, which should trigger the next item in the queue
	updatedM, cmd2 := m.Update(dl1)
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd2 != nil, "Expected a second download command to be queued")
	assert.Assert(t, strings.Contains(m.View(), "Downloading 2/2"), "View should show batch downloading progress for the second item")

	msg2 := cmd2()
	dl2, ok2 := msg2.(tui.DownloadMsg)
	assert.Assert(t, ok2, "Expected a tui.DownloadMsg for the second item")

	// Update model with the final download result
	m, _ = updateModel(m, dl2)
	assert.Assert(t, strings.Contains(m.View(), "Downloaded 2 files"), "View should show final batch success message")

	// We expect the paths to be obj1 and obj2 in any order
	paths := map[string]bool{
		filepath.Base(dl1.Path): true,
		filepath.Base(dl2.Path): true,
	}
	assert.Assert(t, paths["obj1"], "obj1 should be downloaded")
	assert.Assert(t, paths["obj2"], "obj2 should be downloaded")

	// Verify the selection is cleared
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "✓ obj1"), "obj1 should no longer be selected")
	assert.Assert(t, !strings.Contains(view, "✓ obj2"), "obj2 should no longer be selected")
}

func TestModel_DownloadAction_FileExists_Abort(t *testing.T) {
	// Create a temp directory for downloads
	downloadDir := t.TempDir()

	// Create a dummy file that already exists
	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0600)
	assert.NilError(t, err)

	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Assert that we are prompted for confirmation and no command is returned yet
	assert.Assert(t, cmd == nil, "No command should be returned when asking for confirmation")
	assert.Assert(t, strings.Contains(m.View(), "File exists"), "View should indicate file exists")
	assert.Assert(t, strings.Contains(m.View(), "(o)verwrite"), "View should present overwrite option")
	assert.Assert(t, strings.Contains(m.View(), "(a)bort"), "View should present abort option")
	assert.Assert(t, strings.Contains(m.View(), "(r)ename"), "View should present rename option")

	// Press 'a' to abort
	m, cmd = pressKey(m, 'a')

	assert.Assert(t, cmd == nil, "No command should be returned after abort")
	assert.Assert(t, strings.Contains(m.View(), "Download aborted"), "View should indicate abortion")
}

func TestModel_DownloadAction_FileExists_Overwrite(t *testing.T) {
	downloadDir := t.TempDir()

	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0600)
	assert.NilError(t, err)

	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Press 'd' to download
	m, _ = pressKey(m, 'd')

	// Press 'o' to overwrite
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Cmd should be returned for overwrite")
	assert.Assert(t, strings.Contains(m.View(), "Downloading (overwriting)..."), "View should show overwriting status")

	msg := resolveFetchCmd(cmd)
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	assert.Equal(t, downloadMsg.Path, existingFile)
}

func TestModel_DownloadAction_FileExists_Rename(t *testing.T) {
	downloadDir := t.TempDir()

	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0600)
	assert.NilError(t, err)

	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Press 'd' to download
	m, _ = pressKey(m, 'd')

	// Press 'r' to rename
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	assert.Assert(t, cmd != nil, "Cmd should be returned for rename")

	msg := resolveFetchCmd(cmd)
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	expectedNewPath := filepath.Join(downloadDir, "obj1_1")
	assert.Equal(t, downloadMsg.Path, expectedNewPath)
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1_1..."), "View should show downloading renamed file status")
}

func TestModel_DownloadStatusAutoClear(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Trigger download status
	m, _ = pressKey(m, 'd')
	assert.Assert(t, strings.Contains(m.View(), "Downloading obj1..."))

	// Move cursor down
	m, _ = pressKey(m, 'j')

	// Status should PERSIST during navigation while downloading
	assert.Assert(t, strings.Contains(m.View(), "Downloading obj1..."), "Download status should persist after navigation")

	// Trigger download success
	m, cmd := updateModel(m, tui.DownloadMsg{Path: "/tmp/obj1"})
	assert.Assert(t, strings.Contains(m.View(), "Downloaded to /tmp/obj1"))

	// Command to clear status should be returned
	assert.Assert(t, cmd != nil, "Expected a command to clear the status")

	// Execute the command (simulate timer firing)
	msg := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg)

	// Status should be CLEARED
	assert.Assert(t, !strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "Status should be cleared after timer fires")
	assert.Assert(t, strings.Contains(m.View(), " NORMAL "), "Status should be NORMAL again")
}
