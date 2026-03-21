package tui_test

import (
	"fmt"
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
	assert.Assert(t, strings.Contains(m.View(), "Cannot copy multiple files at once"))
}

func TestModel_Actions_CopyURIDirectory(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, []string{"dir1/"})
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	cb := &mockClipboard{}
	m.SetClipboard(cb)

	m = enterBucket(m, projects, "b1", objects)

	// 'k' to hover dir1/ assuming it's above obj1 or first. Actually, simpleObjectList puts prefixes before objects.
	// We need to ensure we hover the prefix. The initial cursor is at 0, which is "dir1/".
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Equal(t, cb.content, "gs://b1/dir1/")
	assert.Assert(t, strings.Contains(m.View(), "Copied gs://b1/dir1/ to clipboard"))
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

	// Resolve the command (which is a batch containing openFile and clearStatusCmd)
	msg := cmd()
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batchMsg {
			if c != nil {
				subMsg := c()
				if foMsg, ok := subMsg.(tui.FileOpenedMsg); ok {
					_, _ = updateModel(m, foMsg)
				}
			}
		}
	}

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
	assert.Assert(t, strings.Contains(m.View(), "[Job #1] Downloading as obj1..."), "View should show downloading status with job number and filename")

	// Simulate download completion
	m, _ = updateModel(m, tui.DownloadMsg{Path: "/tmp/obj1", JobNum: 1})

	assert.Assert(t, strings.Contains(m.View(), "[Job #1] Downloaded to /tmp/obj1"), "View should show success status with job number")
}

func TestModel_DownloadAction_MultiSelect(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2", "obj3"}, nil)
	downloadDir := t.TempDir()
	m, client := setupTestModel(projects, objects, downloadDir)

	// Enter bucket and load objects
	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1
	m, _ = pressKey(m, ' ')

	// Move to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	assert.Assert(t, cmd != nil, "Cmd should not be nil")

	// With concurrent downloads, both items might be queued and started immediately
	msgs := resolveAllFetchCmds(cmd)
	var dlMsgs []tui.DownloadMsg
	for _, msg := range msgs {
		if dl, ok := msg.(tui.DownloadMsg); ok {
			dlMsgs = append(dlMsgs, dl)
		} else {
			m, _ = updateModel(m, msg)
		}
	}
	assert.Assert(t, len(dlMsgs) == 2, "Expected 2 concurrent downloads")

	// Check that messages were logged for both downloads
	found1 := false
	found2 := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "[Job #1] Downloading 1/2 as obj") {
			found1 = true
		}
		if strings.Contains(msg.Text, "[Job #1] Downloading 2/2 as obj") {
			found2 = true
		}
	}
	assert.Assert(t, found1, "Should show progress for obj1")
	assert.Assert(t, found2, "Should show progress for obj2")

	// Finish both downloads
	m, _ = updateModel(m, dlMsgs[0])
	m, cmd = updateModel(m, dlMsgs[1])

	// Update model with the final download result
	assert.Assert(t, cmd != nil, "Expected final completion cmd")

	foundFinish := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Downloaded 2 files") {
			foundFinish = true
		}
	}
	assert.Assert(t, foundFinish, "Should indicate all files downloaded")

	// We expect the paths to be obj1 and obj2 in any order
	paths := map[string]bool{
		filepath.Base(dlMsgs[0].Path): true,
		filepath.Base(dlMsgs[1].Path): true,
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

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	// Enter bucket and load objects
	m = enterBucket(m, projects, "b1", client.objects)

	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Assert that we are prompted for confirmation
	assert.Assert(t, cmd == nil, "Command should NOT be returned to clear the prompt message (persistent prompt)")
	view := m.View()
	assert.Assert(t, strings.Contains(view, "File exists"), "Message should indicate file exists")
	assert.Assert(t, strings.Contains(view, "(o)verwrite"), "Message should present overwrite option")
	assert.Assert(t, strings.Contains(view, "(a)bort"), "Message should present abort option")
	assert.Assert(t, strings.Contains(view, "(r)ename"), "Message should present rename option")

	// Press 'j' which is invalid in the confirm prompt
	m, jCmd := pressKey(m, 'j')
	assert.Assert(t, jCmd != nil, "Pressing an invalid key should return a command")
	msg := jCmd()
	_, ok := msg.(tui.BeepMsg)
	assert.Assert(t, ok, "Expected BeepMsg for invalid key press during prompt")

	// Press 'a' to abort
	m, cmd = pressKey(m, 'a')

	assert.Assert(t, cmd != nil, "A clear status command should be returned after abort")
	assert.Assert(t, strings.Contains(m.View(), "Aborted"), "Message should indicate abortion")
}

func TestModel_DownloadAction_FileExists_Overwrite(t *testing.T) {
	downloadDir := t.TempDir()

	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0600)
	assert.NilError(t, err)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Press 'd' to download
	m, _ = pressKey(m, 'd')

	// Press 'o' to overwrite
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Cmd should be returned for overwrite")
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1..."), "View should show overwriting status")
	assert.Assert(t, strings.Contains(m.View(), "⟳ 1 Tasks"), "Expected task to be tracked in footer")

	msg := resolveFetchCmd(cmd)
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	assert.Equal(t, downloadMsg.Path, existingFile)

	m, _ = updateModel(m, downloadMsg)
	assert.Assert(t, !strings.Contains(m.View(), "⟳ 1 Tasks"), "Expected task to be removed after download completes")
}

func TestModel_DownloadAction_FileExists_Rename(t *testing.T) {
	downloadDir := t.TempDir()

	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0600)
	assert.NilError(t, err)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Press 'd' to download
	m, _ = pressKey(m, 'd')

	// Press 'r' to rename
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	assert.Assert(t, cmd != nil, "Cmd should be returned for rename")

	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1_1..."), "View should show downloading renamed file status")
	assert.Assert(t, strings.Contains(m.View(), "⟳ 1 Tasks"), "Expected task to be tracked in footer")

	msg := resolveFetchCmd(cmd)
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	expectedNewPath := filepath.Join(downloadDir, "obj1_1")
	assert.Equal(t, downloadMsg.Path, expectedNewPath)

	m, _ = updateModel(m, downloadMsg)
	assert.Assert(t, !strings.Contains(m.View(), "⟳ 1 Tasks"), "Expected task to be removed after download completes")
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
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1..."))

	// Move cursor down
	m, _ = pressKey(m, 'j')

	// Status should PERSIST during navigation while downloading
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1..."), "Download status should persist after navigation")

	// Trigger download success
	m, cmd := updateModel(m, tui.DownloadMsg{Path: "/tmp/obj1"})
	assert.Assert(t, strings.Contains(m.View(), "Downloaded to /tmp/obj1"))

	// Command to clear status should be returned
	assert.Assert(t, cmd != nil, "Expected a command to clear the status")

	// Execute the command (simulate timer firing)
	msg := cmd()
	m, _ = updateModel(m, msg)

	// Status should be CLEARED
	assert.Assert(t, !strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "Status should be cleared after timer fires")
	assert.Assert(t, strings.Contains(m.View(), " NORMAL "), "Status should be NORMAL again")
}

func TestModel_Actions_OpenSingleSelectSuccess(t *testing.T) {
	// Mock ExecCommand
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Move cursor to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Move cursor back to obj1 to ensure it operates on the selection, not cursor
	m, _ = pressKey(m, 'k')

	// Press 'o' to open
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Expected cmd to be returned")
	assert.Assert(t, strings.Contains(m.View(), "Opening obj2"), "Expected to open selected file")
}

func TestModel_Actions_EditSingleSelectSuccess(t *testing.T) {
	// Mock ExecCommand
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Move cursor to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Move cursor back to obj1
	m, _ = pressKey(m, 'k')

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	assert.Assert(t, cmd != nil, "Expected cmd to be returned")
	assert.Assert(t, strings.Contains(m.View(), "Opening obj2"), "Expected to edit selected file")
}

func TestModel_Actions_OpenMultiSelectError(t *testing.T) {
	// Mock ExecCommand
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Select obj1
	m, _ = pressKey(m, ' ')
	// Move to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Press 'o' to open
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot open multiple files at once"))
}

func TestModel_Actions_EditMultiSelectError(t *testing.T) {
	// Mock ExecCommand
	oldExec := tui.ExecCommand
	tui.ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	defer func() { tui.ExecCommand = oldExec }()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Select obj1
	m, _ = pressKey(m, ' ')
	// Move to obj2 and select it
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, ' ')

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot edit multiple files at once"))
}

func TestModel_Actions_OpenSingleSelectPrefixError(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList(nil, []string{"prefix1/"})
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Select prefix1/
	m, _ = pressKey(m, ' ')

	// Press 'o' to open
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot open a directory"), "Expected error message about opening a directory")
}

func TestModel_Actions_EditSingleSelectPrefixError(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList(nil, []string{"prefix1/"})
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Select prefix1/
	m, _ = pressKey(m, ' ')

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot edit a directory"), "Expected error message about editing a directory")
}

func TestModel_Actions_OpenHighlightedPrefixError(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList(nil, []string{"prefix1/"})
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Don't select, just let the cursor highlight prefix1/

	// Press 'o' to open
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot open a directory"), "Expected error message about opening a directory")
}

func TestModel_Actions_EditHighlightedPrefixError(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList(nil, []string{"prefix1/"})
	m, _ := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)

	// Don't select, just let the cursor highlight prefix1/

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	assert.Assert(t, cmd != nil, "Expected clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Cannot edit a directory"), "Expected error message about editing a directory")
}

func TestModel_DownloadAction_MultiSelect_FileExists_Abort(t *testing.T) {
	downloadDir := t.TempDir()

	// Create dummy files that already exist
	existingFile1 := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile1, []byte("existing content 1"), 0600)
	assert.NilError(t, err)

	existingFile2 := filepath.Join(downloadDir, "obj2")
	err = os.WriteFile(existingFile2, []byte("existing content 2"), 0600)
	assert.NilError(t, err)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1 and obj2
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj2

	// Press 'd' to download
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Prompt for the first file
	view := m.View()
	assert.Assert(t, strings.Contains(view, "(a)bort"), "Message should present abort option")

	// Press 'a' to abort the first one
	m, _ = pressKey(m, 'a')

	// After aborting the first one, it should prompt for the second file
	foundPrompt := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "(a)bort") && strings.Contains(msg.Text, "obj2") {
			foundPrompt = true
			break
		}
	}
	assert.Assert(t, foundPrompt, "Message should present abort option for the next file")
}

func TestModel_DownloadAction_MultiSelect_FileExists_OverwriteRename(t *testing.T) {
	downloadDir := t.TempDir()

	// Create dummy files that already exist
	existingFile1 := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile1, []byte("existing content 1"), 0600)
	assert.NilError(t, err)

	existingFile2 := filepath.Join(downloadDir, "obj2")
	err = os.WriteFile(existingFile2, []byte("existing content 2"), 0600)
	assert.NilError(t, err)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1 and obj2
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj2

	// Press 'd' to download
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Prompt for the first file (obj1)
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj1"), "Should prompt for obj1")

	// Press 'o' to overwrite obj1
	m, cmd := pressKey(m, 'o')
	assert.Assert(t, cmd != nil, "Should start download task")

	// startDownloadTask returns a Batch of (AddMessage, fetchDownload)
	batch, ok := cmd().(tea.BatchMsg)
	assert.Assert(t, ok, "Expected tea.BatchMsg")

	var dlMsg tui.DownloadMsg
	found := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := c()
		if dm, ok := msg.(tui.DownloadMsg); ok {
			dlMsg = dm
			found = true
			break
		}
	}
	assert.Assert(t, found, "Should have found DownloadMsg in batch")

	res, cmd := m.Update(dlMsg)
	m = res.(tui.Model)
	// cmd should be nil because there's still 1 item in the queue and it prompted (processDownloadQueue returned nil)
	assert.Assert(t, cmd == nil, "Should trigger processDownloadQueue which returns nil as it prompts")

	// Now it should prompt for the second file (obj2)
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj2"), "Should prompt for obj2")

	// Press 'r' to rename obj2
	m, cmd = pressKey(m, 'r')
	assert.Assert(t, cmd != nil, "Should start download task with renamed destination")

	batch, ok = cmd().(tea.BatchMsg)
	assert.Assert(t, ok, "Expected tea.BatchMsg")

	found = false
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := c()
		if dm, ok := msg.(tui.DownloadMsg); ok {
			dlMsg = dm
			found = true
			break
		}
	}
	assert.Assert(t, found, "Should have found DownloadMsg in batch")
	assert.Assert(t, strings.Contains(dlMsg.Path, "obj2_1"), "Should have renamed path: %s", dlMsg.Path)

	res, cmd = m.Update(dlMsg)
	m = res.(tui.Model)
	// downloadQueue is now empty, so it should finish
	assert.Assert(t, cmd != nil, "Should return clearStatusCmd")
	assert.Assert(t, strings.Contains(m.View(), "Downloaded 2 files"), "Should indicate all files downloaded")
}

func TestModel_DownloadAction_MultiSelect_FileExists_RenameError(t *testing.T) {
	downloadDir := t.TempDir()

	// Make the downloadDir read-only to cause rename to fail (autoRename might check permission or fail later)
	// Actually autoRename just checks if file exists, it doesn't do the rename itself.
	// It returns error if it can't find a free suffix after 100 attempts.

	existingFile1 := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile1, []byte("existing content 1"), 0600)
	assert.NilError(t, err)

	existingFile2 := filepath.Join(downloadDir, "obj2")
	err = os.WriteFile(existingFile2, []byte("existing content 2"), 0600)
	assert.NilError(t, err)

	// Create 100 files to make autoRename fail
	for i := 0; i < 101; i++ {
		var name string
		if i == 0 {
			name = "obj1"
		} else {
			name = fmt.Sprintf("obj1_%d", i)
		}
		path := filepath.Join(downloadDir, name)
		_ = os.WriteFile(path, []byte("existing"), 0600)
	}

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1 and obj2
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}) // Select obj2

	// Press 'd' to download
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Press 'r' to rename obj1 (should fail)
	m, _ = pressKey(m, 'r')

	// Should show error and continue to obj2
	foundErr := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Rename failed") {
			foundErr = true
			break
		}
	}
	assert.Assert(t, foundErr, "Should have added a 'Rename failed' message")

	// Since rename failed, it should have triggered processDownloadQueue for next file
	// It returns tea.Batch(cmd, nextCmd)
	// m is already updated by pressKey (which calls handleDownloadConfirmKey)

	// Now it should have prompted for the second file (obj2)
	foundPrompt := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "File exists: obj2") {
			foundPrompt = true
			break
		}
	}
	assert.Assert(t, foundPrompt, "Should have prompted for obj2. Messages: %+v", m.Messages())

	// And the error should also be there
	foundErr = false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Rename failed") {
			foundErr = true
			break
		}
	}
	assert.Assert(t, foundErr, "Should have added a 'Rename failed' message")
}

func TestModel_DownloadAction_MultipleConcurrentBatches(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2", "obj3", "obj4", "obj5", "obj6", "obj7"}, nil)
	downloadDir := t.TempDir()
	m, client := setupTestModel(projects, objects, downloadDir)

	// Enter bucket and load objects
	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1, obj2, obj3, obj4, obj5, obj6
	for i := 0; i < 6; i++ {
		m, _ = pressKey(m, ' ') // Select
		m, _ = pressKey(m, 'j') // Move down
	}

	// Start Batch 1 (Job 1)
	m, cmd1 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Assert(t, cmd1 != nil, "Batch start cmd should not be nil")

	// Because maxConcurrentDownloads is 5, it should spawn exactly 5 downloads immediately
	msgs := resolveAllFetchCmds(cmd1)

	var dlMsgs []tui.DownloadMsg
	for _, msg := range msgs {
		if dl, ok := msg.(tui.DownloadMsg); ok {
			dlMsgs = append(dlMsgs, dl)
		} else {
			m, _ = updateModel(m, msg)
		}
	}
	assert.Assert(t, len(dlMsgs) == 5, "Expected 5 concurrent downloads, got %d", len(dlMsgs))

	// Check the message queue to ensure all 5 starting messages were logged
	messages := m.Messages()
	for i := 1; i <= 5; i++ {
		found := false
		expectedText := fmt.Sprintf("[Job #1] Downloading %d/6 as obj%d...", i, i)
		for _, msg := range messages {
			if strings.Contains(msg.Text, expectedText) {
				found = true
				break
			}
		}
		assert.Assert(t, found, "Missing message: %s", expectedText)
	}

	// obj6 should NOT have a starting message yet since it's queued
	foundObj6 := false
	for _, msg := range messages {
		if strings.Contains(msg.Text, "[Job #1] Downloading 6/6 as obj6...") {
			foundObj6 = true
			break
		}
	}
	assert.Assert(t, !foundObj6, "obj6 should not be started yet")

	// Finish the first download (obj1)
	m, cmd2 := updateModel(m, dlMsgs[0])

	// This should trigger the next item in the queue (obj6)
	nextMsgs := resolveAllFetchCmds(cmd2)
	assert.Assert(t, len(nextMsgs) == 1, "Expected 1 new download to start")

	if dl, ok := nextMsgs[0].(tui.DownloadMsg); ok {
		assert.Assert(t, strings.HasSuffix(dl.Path, "obj6"), "The popped item should be obj6")
	} else {
		t.Fatalf("Expected DownloadMsg, got %T", nextMsgs[0])
	}

	assert.Assert(t, strings.Contains(m.View(), "[Job #1] Downloading 6/6 as obj6"), "Job 1 6th item started concurrently")
}

func TestModel_DownloadAction_ProcessQueueWhileConfirming(t *testing.T) {
	downloadDir := t.TempDir()

	// Pre-create obj2 to trigger "File exists" confirmation later
	_ = os.WriteFile(filepath.Join(downloadDir, "obj2"), []byte("exist"), 0600)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Start two jobs
	// Job 1 (obj1)
	m, _ = pressKey(m, ' ') // Select obj1
	m, cmd1 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Assert(t, cmd1 != nil, "Cmd1 should not be nil")

	msg1 := resolveFetchCmd(cmd1)
	dlMsg1, ok := msg1.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")

	// Job 2 (obj2)
	m, _ = pressKey(m, 'j') // Move to obj2
	m, _ = pressKey(m, ' ') // Select obj2
	m, cmd2 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	// cmd2 triggers processDownloadQueue which returns nil because it enters confirmation state for obj2
	assert.Assert(t, cmd2 == nil, "Cmd2 should be nil because it enters confirmation state")
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj2"), "View should show confirmation prompt")

	// Now finish Job 1 while Job 2 is waiting for confirmation
	m, _ = updateModel(m, dlMsg1)

	// Verify that finishing Job 1 didn't overwrite Job 2's confirmation state
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj2"), "View should STILL show confirmation prompt for obj2, not be interrupted by background task completion")
}

func TestModel_DownloadAction_LostAbortProgressMessage(t *testing.T) {
	downloadDir := t.TempDir()

	// Pre-create obj2 to trigger "File exists" confirmation on the second file
	_ = os.WriteFile(filepath.Join(downloadDir, "obj2"), []byte("exist"), 0600)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)

	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1, obj2
	m, _ = pressKey(m, ' ') // Select obj1
	m, _ = pressKey(m, 'j') // Move to obj2
	m, _ = pressKey(m, ' ') // Select obj2

	// Start Download
	m, cmd1 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Assert(t, cmd1 != nil, "Cmd should start obj1")

	msg := resolveFetchCmd(cmd1)
	dlMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")

	// Finish obj1
	m, cmd2 := updateModel(m, dlMsg)

	// cmd2 should be processing obj2, which exists, so cmd2 is nil, state is confirmation
	assert.Assert(t, cmd2 == nil, "obj2 should prompt for confirmation")
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj2"), "View should show confirmation prompt")

	// Abort obj2
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// We aborted obj2. obj1 succeeded. We should see "Downloaded 1 files" in messages.
	found := false
	for _, logMsg := range m.Messages() {
		if strings.Contains(logMsg.Text, "Downloaded 1 files") {
			found = true
			break
		}
	}
	assert.Assert(t, found, "Should see success message for the 1 completed file even if 1 was aborted")
}

func TestModel_DownloadAction_QueueStallFix(t *testing.T) {
	downloadDir := t.TempDir()

	// Pre-create obj1 so it triggers a prompt
	err := os.WriteFile(filepath.Join(downloadDir, "obj1"), []byte("exists"), 0600)
	assert.NilError(t, err)

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2", "obj3"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)
	m.SetDeterministicSpinner(true)

	// Enter bucket and load objects
	m = enterBucket(m, projects, "b1", client.objects)

	// Select obj1, obj2, obj3
	m, _ = pressKey(m, ' ') // Select obj1
	m, _ = pressKey(m, 'j') // Move to obj2
	m, _ = pressKey(m, ' ') // Select obj2
	m, _ = pressKey(m, 'j') // Move to obj3
	m, _ = pressKey(m, ' ') // Select obj3

	// Start Download Batch
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Assert(t, cmd == nil, "Cmd should be nil because it hit a prompt for obj1")
	assert.Assert(t, strings.Contains(m.View(), "File exists: obj1"), "Should prompt for obj1")

	// Press 'o' to overwrite
	m, cmd2 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	assert.Assert(t, cmd2 != nil, "Cmd should not be nil after choosing overwrite")

	// With the fix, cmd2 should contain commands for obj1 (overwrite) PLUS obj2 and obj3 since they were queued
	msgs := resolveAllFetchCmds(cmd2)
	var dlMsgs []tui.DownloadMsg
	for _, msg := range msgs {
		if dl, ok := msg.(tui.DownloadMsg); ok {
			dlMsgs = append(dlMsgs, dl)
		} else {
			m, _ = updateModel(m, msg)
		}
	}

	assert.Assert(t, len(dlMsgs) == 3, "Expected 3 concurrent downloads to start (obj1 overwritten, plus obj2, obj3 from queue), got %d", len(dlMsgs))

	foundObj2 := false
	foundObj3 := false
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Downloading 2/3 as obj2") {
			foundObj2 = true
		}
		if strings.Contains(msg.Text, "Downloading 3/3 as obj3") {
			foundObj3 = true
		}
	}
	assert.Assert(t, foundObj2, "Should show progress for obj2")
	assert.Assert(t, foundObj3, "Should show progress for obj3")
}

func TestModel_DownloadAction_ActivelyDownloadingPrompt(t *testing.T) {
	downloadDir := t.TempDir()

	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, downloadDir)
	m.SetDeterministicSpinner(true)

	// Enter bucket and load objects
	m = enterBucket(m, projects, "b1", client.objects)

	// First batch: download obj1
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	assert.Assert(t, cmd != nil, "Cmd should not be nil")

	// It's actively downloading now
	// Start a second batch for obj1 while it's still downloading!
	m, cmd2 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Because it's actively downloading, it should prompt with the actively downloading message
	assert.Assert(t, cmd2 == nil, "Cmd should be nil because it hit a prompt for obj1")
	if !strings.Contains(m.View(), "File is actively downloading: obj1") {
		t.Logf("m.View(): %s", m.View())
	}
	assert.Assert(t, strings.Contains(m.View(), "File is actively downloading: obj1"), "Should prompt that it's actively downloading")

	// If the user tries to overwrite, it should reject (BeepCmd)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})

	// A BeepCmd is just a func that returns nil, but we can't easily assert on it,
	// but the state should STILL be viewDownloadConfirm since it rejected the keypress
	assert.Assert(t, strings.Contains(m.View(), "File is actively downloading: obj1"), "Should still be prompting because overwrite was rejected")

	// If the user chooses rename, it should rename it
	m, cmd4 := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	msgs := resolveAllFetchCmds(cmd4)
	assert.Assert(t, len(msgs) > 0, "Should return fetch command")
	if dl, ok := msgs[0].(tui.DownloadMsg); ok {
		assert.Assert(t, strings.HasSuffix(dl.Path, "obj1_1"), "Should have renamed to obj1_1")
	} else {
		t.Fatalf("Expected DownloadMsg, got %T", msgs[0])
	}
}
