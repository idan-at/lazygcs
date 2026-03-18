package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_Navigation_TopBottom(t *testing.T) {
	var buckets []string
	for i := 0; i < 50; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: buckets})

	// 1. Initial cursor at 0 (p1 header)
	assert.Equal(t, m.Cursor(), 0)

	// 2. Press 'G' to go to bottom
	m, _ = pressKey(m, 'G')
	// The list has: p1 header + 50 buckets = 51 items. Bottom is index 50.
	assert.Equal(t, m.Cursor(), 50, "Cursor should be at the last item (bucket-49)")

	// 3. Press 'g' to go to top
	m, _ = pressKey(m, 'g')
	assert.Equal(t, m.Cursor(), 0, "Cursor should be back at the first item (p1 header)")
}

func TestModel_Navigation_PageUpDown(t *testing.T) {
	var buckets []string
	for i := 0; i < 100; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}
	// setupTestModel sets height to 50. columnHeight will be height - header(3) - footer(2) = 45.
	// Page jump should be roughly 45.
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: buckets})

	assert.Equal(t, m.Cursor(), 0)

	// 1. Press Ctrl+f (Page Down)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyCtrlF})
	// It should move down by viewport height.
	assert.Assert(t, m.Cursor() > 20, "Cursor should have moved down significantly")

	currentCursor := m.Cursor()

	// 2. Press Ctrl+b (Page Up)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyCtrlB})
	assert.Assert(t, m.Cursor() < currentCursor, "Cursor should have moved up")
	assert.Equal(t, m.Cursor(), 0, "One page up from ~45 should bring us back to 0")
}

func TestModel_Navigation_JumpToRoot(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// 1. Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Verify we are deep (header shows object path)
	m, _ = pressKey(m, 'j') // hover obj1
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/obj1"), "Should show full object path in header")

	// 2. Press 'H' to jump to root
	m, _ = pressKey(m, 'H')

	// Assertions:
	// - Header should be back to bucket level
	assert.Assert(t, !strings.Contains(m.View(), "gs://b1/obj1"), "Header should no longer show object path")
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"), "Header should show bucket root")
}

func TestModel_Update_ArrowKeyNavigation(t *testing.T) {
	client := &mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}})

	assert.Assert(t, strings.Contains(m.View(), " p1"))

	m, _ = pressKey(m, 'j')
	assert.Assert(t, strings.Contains(m.View(), " b1"))

	m, _ = pressKeyType(m, tea.KeyRight)
	assert.Assert(t, strings.Contains(m.View(), "Objects in b1"))

	m, _ = pressKeyType(m, tea.KeyLeft)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	client := &mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}})

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	m, _ = pressKey(m, 'j')
	assert.Assert(t, strings.Contains(m.View(), " b2"))

	m, _ = pressKey(m, 'k')
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_HalfPageNavigation(t *testing.T) {
	var buckets []string
	for i := 0; i < 50; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: buckets})

	// Initial view should show bucket-00
	view := m.View()
	assert.Assert(t, strings.Contains(view, "bucket-00"))
	assert.Assert(t, !strings.Contains(view, "bucket-20")) // Verify it's not showing everything

	// Press Ctrl+D twice to move down significantly
	m, _ = pressKeyType(m, tea.KeyCtrlD)
	m, _ = pressKeyType(m, tea.KeyCtrlD)

	viewDown := m.View()
	assert.Assert(t, !strings.Contains(viewDown, "bucket-00"))
	// We expect the view to have scrolled down

	// Press Ctrl+U twice to move back up
	m, _ = pressKeyType(m, tea.KeyCtrlU)
	m, _ = pressKeyType(m, tea.KeyCtrlU)

	viewUp := m.View()
	assert.Assert(t, strings.Contains(viewUp, "bucket-00"))
}

func TestModel_Update_CursorCycle(t *testing.T) {
	client := &mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}})

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	// Up from top -> bottom
	m, _ = pressKeyType(m, tea.KeyUp)
	assert.Assert(t, strings.Contains(m.View(), " b3"))

	// Down from bottom -> top
	m, _ = pressKeyType(m, tea.KeyDown)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_CursorBug_SingleItem(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: &gcs.ObjectList{
			Objects: []gcs.ObjectMetadata{{
				Name:        "obj1",
				Size:        1024,
				ContentType: "text/plain",
			}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Press Down - should not crash or change cursor
	m, _ = pressKeyType(m, tea.KeyDown)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "obj1"))
}

func TestModel_HeaderClearedOnBack(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))

	// Go back to bucket list
	m, _ = pressKey(m, 'h')

	// Header should update to reflect the currently focused bucket
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))
}

func TestModel_CursorPersistsOnBack(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Initial state: Buckets loaded, cursor at 0 (b1)
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}})
	m, _ = pressKey(m, 'j')

	// 2. Move cursor down to b2 (index 1)
	m, _ = pressKey(m, 'j')
	assert.Assert(t, strings.Contains(m.View(), " b2"))

	// 3. Enter bucket b2
	m, _ = pressKeyType(m, tea.KeyEnter)
	assert.Assert(t, strings.Contains(m.View(), "Objects in b2"))

	// 4. Go back to bucket list
	m, _ = pressKey(m, 'h')

	// Assertions:
	// - Should be back in buckets view (has "Buckets" header and no "Objects in")
	// - Cursor should still be on b2
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Buckets"), "Should show Buckets header")
	assert.Assert(t, strings.Contains(view, " b2"), "Cursor should be on b2, view:\n%s", view)
}

func TestModel_CursorPersistsOnBack_WithFilter(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot", "blueberry"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Initial state: Buckets loaded
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot", "blueberry"}})
	m, _ = pressKey(m, 'j')

	// 2. Filter by 'b' -> [banana, blueberry]
	m, _ = pressKey(m, '/')
	m, _ = pressKey(m, 'b')

	// Exit search mode but keep query active
	m, _ = pressKeyType(m, tea.KeyEnter)

	// 3. Move cursor down to blueberry (index 2 in filtered list: [p1, banana, blueberry])
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, 'j')

	// 4. Enter bucket blueberry
	m, _ = pressKeyType(m, tea.KeyEnter)

	// 5. Go back to bucket list
	m, _ = pressKey(m, 'h')

	// Assertions:
	// - Filter should STILL be active
	// - Cursor should be on blueberry in the filtered list
	view := m.View()
	assert.Assert(t, strings.Contains(view, "blueberry"), "Should contain blueberry")

	// Since filter 'b' is active, the list is: [p1, banana, blueberry]
	// blueberry is at index 2
	assert.Equal(t, m.Cursor(), 2, "Cursor should be on blueberry (index 2 in filtered list)")
}

func TestModel_CursorPersistsOnBack_Prefix(t *testing.T) {
	rootObjects := simpleObjectList([]string{"file1"}, []string{"folder1/", "folder2/"})
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := rootObjects
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket b1
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Load root objects
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: rootObjects})

	// Move cursor to folder2/ (index 1)
	m, _ = pressKey(m, 'j')

	// Enter folder2/
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Go back using 'h'
	m, _ = pressKey(m, 'h')

	// Re-load root objects
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: rootObjects})

	// Assert cursor is restored to folder2/
	assert.Equal(t, m.Cursor(), 1, "Cursor should be restored to folder2/ (index 1)")
	assert.Assert(t, strings.Contains(m.View(), "folder2/"), "View should contain folder2/")
}

func TestModel_CollapseProjectOnLeft(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	// Ensure p1 is expanded initially (it is by default)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
	assert.Assert(t, strings.Contains(m.View(), "▼ p1"))

	// Ensure cursor is on p1 (index 0)
	// Press 'h' (Left) to collapse it
	m, _ = pressKey(m, 'h')

	// Assertions:
	// - The project should be collapsed (▶ p1)
	// - The bucket (b1) should not be visible
	view := m.View()
	assert.Assert(t, !strings.Contains(view, " b1"), "Bucket b1 should be hidden")
	assert.Assert(t, strings.Contains(view, "▶") && strings.Contains(view, "p1"), "Project p1 should be collapsed")
}
