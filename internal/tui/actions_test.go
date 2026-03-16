package tui_test

import (
	"os"
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

func TestModel_Actions_CopyURI(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// 1. Bucket View Copy
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = pressKey(m, 'j') // hover b1
	
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	// We can't easily check clipboard content here without mocking, 
	// but we can check if the status was updated to indicate success.
	assert.Assert(t, strings.Contains(m.View(), "Copied gs://b1/ to clipboard"))

	// 2. Object View Copy
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover obj1
	
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Assert(t, strings.Contains(m.View(), "Copied gs://b1/obj1 to clipboard"))

	// 3. Multi-select Copy
	m, _ = pressKey(m, ' ') // select obj1
	// Add another object
	objects2 := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: objects2})
	m, _ = pressKey(m, 'j') 
	m, _ = pressKey(m, ' ') // select obj2
	
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Assert(t, strings.Contains(m.View(), "Copied 2 URIs to clipboard"))
}

func TestModel_Actions_Open(t *testing.T) {
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
	// DownloadMsg is returned by openFile
	m, _ = updateModel(m, msg)

	// Verify mock client was called for download
	assert.Equal(t, client.lastDownload.Bucket, "b1")
	assert.Equal(t, client.lastDownload.Object, "obj1")
}

func TestModel_Actions_Edit(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover obj1

	// Press 'e' to edit
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Editing obj1..."))

	// Simulate editor finishing with a modified file
	tempPath := filepath.Join(os.TempDir(), "lazygcs", "b1", "obj1")
	// Create the file so os.Stat doesn't fail in handleEditorFinishedMsg
	_ = os.MkdirAll(filepath.Dir(tempPath), 0750)
	_ = os.WriteFile(tempPath, []byte("updated content"), 0644)

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
	m, _ = updateModel(m, msg)

	// Verify mock client was called for upload
	assert.Equal(t, client.lastUpload.Bucket, "b1")
	assert.Equal(t, client.lastUpload.Object, "obj1")
	assert.Equal(t, client.lastUpload.Src, tempPath)
}
