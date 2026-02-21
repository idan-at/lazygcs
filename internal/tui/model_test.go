package tui_test

import (
	"context"
	"strings"
	"testing"

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

// Helper to create simple object list from names
func simpleObjectList(names []string, prefixes []string) *gcs.ObjectList {
	var objects []gcs.ObjectMetadata
	for _, n := range names {
		objects = append(objects, gcs.ObjectMetadata{Name: n})
	}
	return &gcs.ObjectList{Objects: objects, Prefixes: prefixes}
}

func TestModel_AsyncLoading(t *testing.T) {
	client := mockGCSClient{buckets: []string{"async-b1"}}
	m := tui.NewModel([]string{"p1"}, client)

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
	m := tui.NewModel([]string{"p1"}, client)
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
	m := tui.NewModel([]string{"p1"}, client)
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
	m := tui.NewModel([]string{"p1"}, client)
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
	m := tui.NewModel([]string{"p1"}, client)

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)

	msg := tui.ObjectsMsg{List: client.objects}
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
	m := tui.NewModel([]string{"p1"}, client)

	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
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
	m := tui.NewModel([]string{"p1"}, client)

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
	m := tui.NewModel([]string{"p1"}, client)
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
	m = updatedM.(tui.Model)

	if !strings.Contains(m.View(), "> folder1/") {
		t.Fatalf("Expected view to contain '> folder1/', but got:\n%q", m.View())
	}

	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)

	// Simulate nested fetch
	nestedObjects := simpleObjectList([]string{"folder1/file2.txt"}, []string{"folder1/sub/"})
	updatedM, _ = m.Update(tui.ObjectsMsg{List: nestedObjects})
	m = updatedM.(tui.Model)

	view := m.View()
	// Should show path header
	assert.Assert(t, strings.Contains(view, "gs://b1/folder1/"))

	// Should show RELATIVE names
	assert.Assert(t, strings.Contains(view, "file2.txt"))
	assert.Assert(t, !strings.Contains(view, "folder1/file2.txt"))

	assert.Assert(t, strings.Contains(view, "sub/"))
	assert.Assert(t, !strings.Contains(view, "folder1/sub/"))
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
	m := tui.NewModel([]string{"p1"}, client)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
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
			Prefixes: []string{"folder1/"},
			Objects:  []gcs.ObjectMetadata{{Name: "file1"}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
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
