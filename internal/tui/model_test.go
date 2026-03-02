package tui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gotest.tools/v3/assert"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

type mockGCSClient struct {
	buckets []string
	objects *gcs.ObjectList
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
