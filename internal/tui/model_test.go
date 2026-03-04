package tui_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gotest.tools/v3/assert"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

type mockGCSClient struct {
	buckets      []string
	objects      *gcs.ObjectList
	contentError error // Used to force an error for GetObjectContent
}

func (f mockGCSClient) ListBuckets(ctx context.Context, projectIDs []string) ([]string, error) {
	return f.buckets, nil
}

func (f mockGCSClient) ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error) {
	return f.objects, nil
}

func (f mockGCSClient) GetObjectMetadata(ctx context.Context, bucketName, objectName string) (*gcs.ObjectMetadata, error) {
	// Simple mock: find in prefixes or objects
	if f.objects != nil {
		for _, p := range f.objects.Prefixes {
			if p.Name == objectName {
				return &gcs.ObjectMetadata{Name: p.Name, Updated: p.Updated, Created: p.Created, Owner: p.Owner}, nil
			}
		}
		for _, o := range f.objects.Objects {
			if o.Name == objectName {
				return &o, nil
			}
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f mockGCSClient) GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error) {
	if f.contentError != nil {
		return "", f.contentError
	}
	if f.objects != nil {
		for _, o := range f.objects.Objects {
			if o.Name == objectName {
				// Fake content for testing
				return fmt.Sprintf("content of %s", objectName), nil
			}
		}
	}
	return "", fmt.Errorf("not found")
}

func (f mockGCSClient) DownloadObject(ctx context.Context, bucketName, objectName, destPath string) error {
	return nil
}

// Helper to create simple object list from names
func simpleObjectList(names []string, prefixes []string) *gcs.ObjectList {
	var objects []gcs.ObjectMetadata
	for _, n := range names {
		objects = append(objects, gcs.ObjectMetadata{Name: n})
	}
	var prefs []gcs.PrefixMetadata
	for _, p := range prefixes {
		prefs = append(prefs, gcs.PrefixMetadata{Name: p})
	}
	return &gcs.ObjectList{Objects: objects, Prefixes: prefs}
}

func TestModel_ObjectPreview(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, []string{"folder1/"}),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Move cursor down to obj1 (index 1)
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// Verify we got a command and view shows loading
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading..."))

	// Simulate receiving the content
	msg := cmd()
	updatedM, _ = m.Update(msg)
	m = updatedM.(tui.Model)

	// Verify view shows the content
	view := m.View()
	assert.Assert(t, strings.Contains(view, "content of obj1"))
}

func TestModel_InitialObjectPreview(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// Process ObjectsMsg - this should trigger initial fetchContent
	msg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}
	updatedM, cmd := m.Update(msg)
	m = updatedM.(tui.Model)

	// Verify fetchContent was triggered automatically
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading..."))

	// Simulate receiving the content
	contentMsg := cmd()
	updatedM, _ = m.Update(contentMsg)
	m = updatedM.(tui.Model)

	// Verify view shows the content
	assert.Assert(t, strings.Contains(m.View(), "content of obj1"))
}

func TestModel_CursorNoop_PreviewNotReloaded(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Setup: In bucket, obj1 loaded and previewed
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)
	
	// Process initial fetchContent
	updatedM, _ = m.Update(cmd())
	m = updatedM.(tui.Model)
	
	assert.Assert(t, strings.Contains(m.View(), "content of obj1"))

	// 2. Press 'j' (down). Since there's only 1 item, cursor stays at 0.
	updatedM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// Assertions:
	// - No command should be returned (no new fetch)
	// - View should NOT show "Loading..."
	// - View should still show the content
	assert.Assert(t, cmd == nil, "Pressing 'j' when only one item is present should not trigger a new fetch")
	assert.Assert(t, !strings.Contains(m.View(), "Loading..."), "View should not show 'Loading...' if the cursor didn't move")
	assert.Assert(t, strings.Contains(m.View(), "content of obj1"), "Preview content should still be visible")
}

func TestModel_ObjectPreview_Error(t *testing.T) {
	client := mockGCSClient{
		buckets:      []string{"b1"},
		objects:      simpleObjectList([]string{"obj1"}, nil),
		contentError: fmt.Errorf("permission denied"),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Simulate receiving the error message
	msg := cmd()
	updatedM, _ = m.Update(msg)
	m = updatedM.(tui.Model)

	// Verify view shows the error
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Error: permission denied"))
}

func TestModel_StalePreviewContent(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Setup: In bucket, objects loaded
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// 1. Move to obj1, triggering a fetch. Capture the command.
	_, cmdForObj1 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	msgForObj1 := cmdForObj1()

	// 2. Before the content for obj1 arrives, move to obj2.
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 3. Now, the stale message for obj1 arrives.
	updatedM, _ = m.Update(msgForObj1)
	m = updatedM.(tui.Model)

	// The view should NOT show content for "obj1" because we are on "obj2"
	view := m.View()
	if strings.Contains(view, "content of obj1") {
		t.Fatalf("Bug: Stale preview content for obj1 was displayed while obj2 is selected.")
	}
}

func TestModel_AsyncLoading(t *testing.T) {
	client := mockGCSClient{buckets: []string{"async-b1"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	cmd := m.Init()
	assert.Assert(t, cmd != nil)

	msg := tui.BucketsMsg{Buckets: []string{"async-b1"}}
	updatedM, _ := m.Update(msg)
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "async-b1"))
	assert.Assert(t, !strings.Contains(view, "Loading"))
}

func TestModel_Update_ArrowKeyNavigation(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1", "b2", "b3"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2", "b3"}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1", "b2", "b3"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2", "b3"}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}

func TestModel_Update_CursorCycle(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1", "b2", "b3"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2", "b3"}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}

func TestModel_Update_Quit(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())
}

func TestModel_EnterBucket(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)

	msg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}
	updatedM, _ = m.Update(msg)
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "b1"))
	assert.Assert(t, strings.Contains(view, "obj1"))
	assert.Assert(t, strings.Contains(view, "> obj1"))
}

func TestModel_Update_ObjectCursorCycle(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "> obj1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> obj2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> obj1"))
}

func TestModel_Resize(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1"}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, len(view) > 0)

	updatedM, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updatedM.(tui.Model)
	viewNarrow := m.View()

	assert.Assert(t, len(viewNarrow) > 0)
}

func TestModel_EnterPrefix(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"file1"}, []string{"folder1/"}),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	if !strings.Contains(m.View(), "> folder1/") {
		t.Fatalf("Expected view to contain '> folder1/', but got:\n%q", m.View())
	}

	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)

	// Simulate nested fetch
	nestedObjects := simpleObjectList([]string{"folder1/file2.txt"}, []string{"folder1/sub/"})
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "folder1/", List: nestedObjects})
	m = updatedM.(tui.Model)

	view := m.View()
	// Should show path header
	assert.Assert(t, strings.Contains(view, "gs://b1/folder1/"))

	// Split view into columns to be more precise if possible, but let's just check the objects list part
	// The objects list is the middle column.
	// For now, let's just verify that RELATIVE names are present.
	assert.Assert(t, strings.Contains(view, " file2.txt"))
	assert.Assert(t, strings.Contains(view, " sub/"))
}

func TestModel_SelectObject(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: &gcs.ObjectList{
			Objects: []gcs.ObjectMetadata{{
				Name:        "file1.txt",
				Size:        1024,
				ContentType: "text/plain",
			}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Cursor on file1.txt
	view := m.View()
	if !strings.Contains(view, "1024") {
		t.Fatalf("View should show file size. Got:\n%q", view)
	}
	assert.Assert(t, strings.Contains(view, "text/plain"), "View should show content type")
}

func TestModel_CursorBug_SingleItem(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b"},
		objects: &gcs.ObjectList{
			Prefixes: []gcs.PrefixMetadata{{Name: "folder1/"}},
			Objects:  []gcs.ObjectMetadata{{Name: "file1"}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// 2. Enter folder1/
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// 3. State is Loading. Objects/Prefixes are STALE.
	// Press 'j' (down).
	// Current stale list: folder1/ (0), file1 (1).
	// Cursor moves to 1.
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 4. Assert Bug: Preview pane shows "file1" (stale)
	if strings.Contains(m.View(), "file1") {
		t.Fatalf("Preview pane shows stale data 'file1' during loading!\nView:\n%q", m.View())
	}
}

func TestModel_Pagination_Buckets(t *testing.T) {
	var buckets []string
	for i := 0; i < 50; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := mockGCSClient{buckets: buckets}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: buckets})
	m = updatedM.(tui.Model)

	view := m.View()
	if strings.Contains(view, "bucket-49") {
		t.Fatalf("Expected bucket-49 to be hidden due to pagination, but it was visible.")
	}

	for i := 0; i < 49; i++ {
		updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updatedM.(tui.Model)
	}

	view2 := m.View()
	if !strings.Contains(view2, "bucket-49") {
		t.Fatalf("Expected bucket-49 to be visible after scrolling down, but it wasn't.")
	}
	if strings.Contains(view2, "bucket-00") {
		t.Fatalf("Expected bucket-00 to be hidden after scrolling down, but it was visible.")
	}
}

func TestModel_Pagination_Objects(t *testing.T) {
	var objects []string
	for i := 0; i < 50; i++ {
		objects = append(objects, fmt.Sprintf("obj-%02d", i))
	}
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList(objects, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	view := m.View()
	if strings.Contains(view, "obj-49") {
		t.Fatalf("Expected obj-49 to be hidden due to pagination, but it was visible.")
	}

	for i := 0; i < 49; i++ {
		updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updatedM.(tui.Model)
	}

	view2 := m.View()
	if !strings.Contains(view2, "obj-49") {
		t.Fatalf("Expected obj-49 to be visible after scrolling down, but it wasn't.")
	}
	if strings.Contains(view2, "obj-00") {
		t.Fatalf("Expected obj-00 to be hidden after scrolling down, but it was visible.")
	}
}

func TestModel_SelectPrefix(t *testing.T) {
	now := time.Now()
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: &gcs.ObjectList{
			Prefixes: []gcs.PrefixMetadata{{
				Name:    "folder1/",
				Updated: now,
				Created: now,
			}},
			Objects: []gcs.ObjectMetadata{{
				Name:        "file1.txt",
				Size:        1024,
				ContentType: "text/plain",
			}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	if cmd != nil {
		msg := cmd()
		updatedM, _ = m.Update(msg)
		m = updatedM.(tui.Model)
	}

	// Cursor is on folder1/ by default
	view := m.View()
	if !strings.Contains(view, "Type: Folder") {
		t.Fatalf("View should show Folder type for prefixes. Got:\n%q", view)
	}
	assert.Assert(t, strings.Contains(view, "folder1/"), "View should show folder name")
	assert.Assert(t, strings.Contains(view, "Updated:"), "View should show updated time for folder")
	assert.Assert(t, strings.Contains(view, "Created:"), "View should show created time for folder")
}

func TestModel_HeaderClearedOnBack(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	// Enter bucket
	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	viewInBucket := m.View()
	assert.Assert(t, strings.Contains(viewInBucket, "gs://b1/"), "View should show bucket in header when inside bucket")

	// Go back to bucket list
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	viewInBucketsList := m.View()
	assert.Assert(t, !strings.Contains(viewInBucketsList, "gs://b1/"), "View should not show bucket in header after returning to bucket list")
	assert.Assert(t, strings.Contains(viewInBucketsList, "gs://"), "View should show gs:// in header after returning to bucket list")
}

func TestModel_StaleObjectsMsg(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1", "b2"},
		objects: simpleObjectList([]string{"obj-from-b1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter b1
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2"}})
	m = updatedM.(tui.Model)

	// User enters b1 (this triggers a fetch for b1, but we simulate a delay by NOT applying the msg yet)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// User decides to go back to buckets list before b1 loads
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	// User moves to b2 and enters it
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// At this point, the user is in b2, waiting for its objects.
	// Now, the delayed response for b1 arrives!
	staleMsg := tui.ObjectsMsg{
		List: simpleObjectList([]string{"obj-from-b1"}, nil),
	}
	updatedM, _ = m.Update(staleMsg)
	m = updatedM.(tui.Model)

	// The view should NOT show objects from b1 when we are currently in b2.
	view := m.View()
	if strings.Contains(view, "obj-from-b1") {
		t.Fatalf("Bug: Stale ObjectsMsg from b1 took over the view while user is in b2!\nView:\n%s", view)
	}
}

func TestModel_DownloadAction(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil, "Cmd should be returned for download")
	assert.Assert(t, strings.Contains(m.View(), "Downloading..."), "View should show downloading status")

	// Simulate download completion
	updatedM, _ = m.Update(tui.DownloadMsg{Path: "/tmp/obj1"})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "View should show success status")
}

func TestModel_Truncation(t *testing.T) {
	longName := "this_is_a_very_long_object_name_that_should_be_truncated_to_fit_in_the_column"
	client := mockGCSClient{
		buckets: []string{longName},
		objects: simpleObjectList([]string{longName}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp")
	
	// Set a specific width where we know it should truncate everywhere
	// leftWidth = 40 * 0.3 = 12.
	// Header width = 40 - 2 = 38.
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	m = updatedM.(tui.Model)

	// 1. Check Bucket truncation
	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{longName}})
	m = updatedM.(tui.Model)
	
	view := m.View()
	// Should contain truncated version (usually ending in ...)
	assert.Assert(t, strings.Contains(view, "..."), "View should contain ellipsis for truncated bucket name")
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long bucket name")

	// 2. Check Object truncation
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: longName, Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	view = m.View()
	assert.Assert(t, strings.Contains(view, "..."), "View should contain ellipsis for truncated object name")
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long object name")
}

func TestModel_DownloadAction_FileExists_Abort(t *testing.T) {
	// Create a temp directory for downloads
	downloadDir := t.TempDir()
	
	// Create a dummy file that already exists
	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	assert.NilError(t, err)

	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

	// Assert that we are prompted for confirmation and no command is returned yet
	assert.Assert(t, cmd == nil, "No command should be returned when asking for confirmation")
	assert.Assert(t, strings.Contains(m.View(), "File exists"), "View should indicate file exists")
	assert.Assert(t, strings.Contains(m.View(), "(o)verwrite"), "View should present overwrite option")
	assert.Assert(t, strings.Contains(m.View(), "(a)bort"), "View should present abort option")
	assert.Assert(t, strings.Contains(m.View(), "(r)ename"), "View should present rename option")

	// Press 'a' to abort
	updatedM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd == nil, "No command should be returned after abort")
	assert.Assert(t, strings.Contains(m.View(), "Download aborted"), "View should indicate abortion")
}

func TestModel_DownloadAction_FileExists_Overwrite(t *testing.T) {
	downloadDir := t.TempDir()
	
	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	assert.NilError(t, err)

	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

	// Press 'o' to overwrite
	updatedM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil, "Cmd should be returned for overwrite")
	assert.Assert(t, strings.Contains(m.View(), "Downloading (overwriting)..."), "View should show overwriting status")

	msg := cmd()
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	assert.Equal(t, downloadMsg.Path, existingFile)
}

func TestModel_DownloadAction_FileExists_Rename(t *testing.T) {
	downloadDir := t.TempDir()
	
	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	assert.NilError(t, err)

	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

	// Press 'r' to rename
	updatedM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil, "Cmd should be returned for rename")
	
	msg := cmd()
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	expectedNewPath := filepath.Join(downloadDir, "obj1_1")
	assert.Equal(t, downloadMsg.Path, expectedNewPath)
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1_1..."), "View should show downloading renamed file status")
}
