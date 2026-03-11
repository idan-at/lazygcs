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
	projects     []gcs.ProjectBuckets
	objects      *gcs.ObjectList
	contentError error // Used to force an error for GetObjectContent
}

func (f mockGCSClient) ListBuckets(ctx context.Context, projectIDs []string) ([]gcs.ProjectBuckets, error) {
	return f.projects, nil
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

func (f mockGCSClient) DownloadPrefixAsZip(ctx context.Context, bucketName, prefix, destZipPath string) error {
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

func updateModel(m tui.Model, msg tea.Msg) (tui.Model, tea.Cmd) {
	updatedM, cmd := m.Update(msg)
	return updatedM.(tui.Model), cmd
}

func setupTestModel(projects []gcs.ProjectBuckets, objects *gcs.ObjectList, downloadDir string) (tui.Model, mockGCSClient) {
	client := mockGCSClient{
		projects: projects,
		objects:  objects,
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 50})
	return m, client
}

func enterBucket(m tui.Model, projects []gcs.ProjectBuckets, bucket string, objects *gcs.ObjectList) tui.Model {
	m, _ = updateModel(m, tui.BucketsMsg{Projects: projects})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)
	if objects != nil {
		m, _ = updateModel(m, tui.ObjectsMsg{Bucket: bucket, Prefix: "", List: objects})
	}
	return m
}

func pressKey(m tui.Model, key rune) (tui.Model, tea.Cmd) {
	return updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
}

func pressKeyType(m tui.Model, keyType tea.KeyType) (tui.Model, tea.Cmd) {
	return updateModel(m, tea.KeyMsg{Type: keyType})
}

func TestModel_ObjectPreview(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, []string{"folder1/"})
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Move cursor down to obj1 (index 1)
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Verify we got a command and view shows loading
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading..."))

	// Simulate receiving the content
	msg := cmd()
	m, _ = updateModel(m, msg)

	// Verify view shows the content
	view := m.View()
	assert.Assert(t, strings.Contains(view, "content of obj1"))
}

func TestModel_UI_WrappingBug(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

	// We check the buckets view first.
	// The view output shouldn't have rows that wrap. We can detect wrapping if the line count
	// is much higher than expected, or we can look for specific lipgloss wrapping artifacts.
	// A simpler way: just check the actual dimensions of the view.
	view := m.View()
	lines := strings.Split(view, "\n")

	// Total height is 50. If it wraps, it might exceed 50 or push the footer out.
	// Or we can just count lines. It should exactly match m.height.
	// We might have some trailing empty lines, but let's check max width of lines.
	for _, line := range lines {
		// removing ansi for accurate visible length is needed, but just checking if it blew up the height is easier.
		_ = line
	}

	assert.Assert(t, len(lines) <= 50, "View height %d exceeded window height 50 due to wrapping", len(lines))
}

func TestModel_ModernSelectionUI(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	// Old indicators should be gone
	assert.Assert(t, !strings.Contains(view, ">"), "View should not contain old cursor '>'")
	assert.Assert(t, !strings.Contains(view, "[ ]"), "View should not contain old unselected indicator '[ ]'")

	// Select the item
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "[x]"), "View should not contain old selected indicator '[x]'")
	assert.Assert(t, strings.Contains(view, "✓"), "View should contain new selection indicator '✓'")
}

func TestModel_HelpMenu(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

	// Assert help menu is not shown initially
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"))

	// Press '?' to show help
	m, _ = pressKey(m, '?')

	view = m.View()
	// In 'Which Key' style, we should see both the main content AND the help at the bottom
	assert.Assert(t, strings.Contains(view, "Buckets"), "Buckets column should still be visible")
	assert.Assert(t, strings.Contains(view, "HELP"), "View should contain 'HELP' header")
	assert.Assert(t, !strings.Contains(view, "WHICH-KEY"), "View should NOT contain 'WHICH-KEY' anymore")
	assert.Assert(t, strings.Contains(view, "toggle help"), "View should list the help keybind")

	// Press '?' again to hide help
	m, _ = pressKey(m, '?')

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"), "View should no longer contain 'HELP'")
}

func TestModel_InitialObjectPreview(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Process ObjectsMsg - this should trigger initial fetchContent
	msg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}
	m, cmd := updateModel(m, msg)

	// Verify fetchContent was triggered automatically
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading..."))

	// Simulate receiving the content
	contentMsg := cmd()
	m, _ = updateModel(m, contentMsg)

	// Verify view shows the content
	assert.Assert(t, strings.Contains(m.View(), "content of obj1"))
}

func TestModel_CursorNoop_PreviewNotReloaded(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// 1. Setup: In bucket, obj1 loaded and previewed
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Process initial fetchContent
	m, _ = updateModel(m, cmd())

	assert.Assert(t, strings.Contains(m.View(), "content of obj1"))

	// 2. Press 'j' (down). Since there's only 1 item, cursor stays at 0.
	m, cmd = pressKey(m, 'j')

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
		projects:     []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:      simpleObjectList([]string{"obj1"}, nil),
		contentError: fmt.Errorf("permission denied"),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving the error message
	msg := cmd()
	m, _ = updateModel(m, msg)

	// Verify view shows the error
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Error: permission denied"))
}

func TestModel_StalePreviewContent(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Setup: In bucket, objects loaded
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// 1. Move to obj1, triggering a fetch. Capture the command.
	_, cmdForObj1 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	msgForObj1 := cmdForObj1()

	// 2. Before the content for obj1 arrives, move to obj2.
	m, _ = pressKey(m, 'j')

	// 3. Now, the stale message for obj1 arrives.
	m, _ = updateModel(m, msgForObj1)

	// The view should NOT show content for "obj1" because we are on "obj2"
	view := m.View()
	if strings.Contains(view, "content of obj1") {
		t.Fatalf("Bug: Stale preview content for obj1 was displayed while obj2 is selected.")
	}
}

func TestModel_AsyncLoading(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"async-b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	cmd := m.Init()
	assert.Assert(t, cmd != nil)

	m, _ = updateModel(m, cmd())
	view := m.View()
	assert.Assert(t, strings.Contains(view, "async-b1"))
	assert.Assert(t, !strings.Contains(view, "Loading"))
}

func TestModel_Update_ArrowKeyNavigation(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})

	assert.Assert(t, strings.Contains(m.View(), " p1"))

	m, _ = pressKey(m, 'j')
	assert.Assert(t, strings.Contains(m.View(), " b1"))

	m, _ = pressKeyType(m, tea.KeyRight)
	assert.Assert(t, strings.Contains(m.View(), "Objects in b1"))

	m, _ = pressKeyType(m, tea.KeyLeft)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	m, _ = pressKey(m, 'j')
	assert.Assert(t, strings.Contains(m.View(), " b2"))

	m, _ = pressKey(m, 'k')
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_CursorCycle(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	// Up from top -> bottom
	m, _ = pressKeyType(m, tea.KeyUp)
	assert.Assert(t, strings.Contains(m.View(), " b3"))

	// Down from bottom -> top
	m, _ = pressKeyType(m, tea.KeyDown)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_Quit(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())
}

func TestModel_EnterBucket(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

	m, _ = pressKey(m, 'j')

	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Assert(t, cmd != nil)

	// Simulate objects fetch result
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	view := m.View()
	assert.Assert(t, strings.Contains(view, "b1"))
	assert.Assert(t, strings.Contains(view, "obj1"))
}

func TestModel_Update_ObjectCursorCycle(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// First item is obj1
	assert.Assert(t, strings.Contains(m.View(), " obj1"))

	m, _ = pressKeyType(m, tea.KeyUp)
	assert.Assert(t, strings.Contains(m.View(), " obj2"))

	m, _ = pressKeyType(m, tea.KeyDown)
	assert.Assert(t, strings.Contains(m.View(), " obj1"))
}

func TestModel_Resize(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client

	view := m.View()
	assert.Assert(t, len(view) > 0)

	// Very narrow
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 20, Height: 10})
	viewNarrow := m.View()

	assert.Assert(t, len(viewNarrow) > 0)
}

func TestModel_EnterPrefix(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"file1"}, []string{"folder1/"}),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 50})

	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

	// Enter bucket b1
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Verify we are at root
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))

	// Enter folder1/
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Assert(t, cmd != nil)

	// Simulate nested fetch
	nestedObjects := simpleObjectList([]string{"folder1/file2.txt"}, []string{"folder1/sub/"})
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder1/", List: nestedObjects})

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

	view := m.View()
	// Should show metadata in preview
	assert.Assert(t, strings.Contains(view, "Name: \x1b[1m\x1b[38;5;15mobj1") || strings.Contains(view, "Name: obj1") || strings.Contains(view, "obj1"))
	// Due to lipgloss styling, direct string matches on "Name: obj1" might fail if ansi codes are present.
	// We'll just check for the presence of the values and keys loosely, or strip ansi if needed.
	// For now, let's just look for the humanized size.
	assert.Assert(t, strings.Contains(view, "1.0 KB"))
	assert.Assert(t, strings.Contains(view, "text/plain"))
}

func TestModel_SelectPrefix(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b"}}},
		objects: &gcs.ObjectList{
			Prefixes: []gcs.PrefixMetadata{{Name: "folder1/"}},
			Objects:  []gcs.ObjectMetadata{{Name: "file1"}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b"}}}, "b", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b", Prefix: "", List: client.objects})

	// Initial fetch for first item (prefix)
	assert.Assert(t, cmd != nil)
	msg := cmd()
	metaMsg := msg.(tui.MetadataMsg)

	// Simulate metadata arrival
	now := time.Now()
	metaMsg.Metadata = &gcs.ObjectMetadata{
		Name:    "folder1/",
		Updated: now,
		Created: now,
		Owner:   "test-user",
	}
	m, _ = updateModel(m, metaMsg)

	view := m.View()
	// Preview should show prefix metadata
	assert.Assert(t, strings.Contains(view, "folder1/"))
	assert.Assert(t, strings.Contains(view, "Folder"))
	assert.Assert(t, strings.Contains(view, "test-user"))
}

func TestModel_Pagination_Buckets(t *testing.T) {
	var buckets []string
	for i := 0; i < 50; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 10})

	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}})
	m, _ = pressKey(m, 'j')

	view := m.View()
	// Should not show all 50 buckets
	assert.Assert(t, strings.Contains(view, "bucket-00"))
	assert.Assert(t, !strings.Contains(view, "bucket-49"))
}

func TestModel_Pagination_Objects(t *testing.T) {
	var objects []string
	for i := 0; i < 50; i++ {
		objects = append(objects, fmt.Sprintf("obj-%02d", i))
	}
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList(objects, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 10})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "obj-00"))
	assert.Assert(t, !strings.Contains(view, "obj-49"))
}

func TestModel_CursorBug_SingleItem(t *testing.T) {
	client := mockGCSClient{
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
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))

	// Go back to bucket list
	m, _ = pressKey(m, 'h')

	// Header should be cleared back to gs://
	assert.Assert(t, strings.Contains(m.View(), "gs:// "))
}

func TestModel_StaleObjectsMsg(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}
	objects := simpleObjectList([]string{"obj-from-b1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter b1
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Capture the fetch objects msg for b1
	staleMsg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}

	// Go back to bucket list
	m, _ = pressKey(m, 'h')

	// Move to b2 and enter it
	m, _ = pressKeyType(m, tea.KeyDown)
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Now the STALE msg from b1 arrives
	m, _ = updateModel(m, staleMsg)

	// The view should NOT show objects from b1 when we are currently in b2.
	view := m.View()
	if strings.Contains(view, "obj-from-b1") {
		t.Fatalf("Bug: Stale ObjectsMsg from b1 took over the view while user is in b2!\nView:\n%s", view)
	}
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
	client := mockGCSClient{
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
	msg := cmd()
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

func TestModel_Truncation(t *testing.T) {
	longName := "this_is_a_very_long_object_name_that_should_be_truncated_to_fit_in_the_column"
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{longName}}},
		objects:  simpleObjectList([]string{longName}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Set a specific width where we know it should truncate everywhere
	// leftWidth = 40 * 0.3 = 12.
	// Header width = 40 - 2 = 38.
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 40, Height: 50})

	// 1. Check Bucket truncation
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{longName}}}})
	m, _ = pressKey(m, 'j')

	view := m.View()
	// Should contain truncated version (usually ending in ...)
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long bucket name")

	// 2. Check Object truncation
	m, _ = pressKeyType(m, tea.KeyEnter)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: longName, Prefix: "", List: client.objects})

	view = m.View()
	assert.Assert(t, strings.Contains(view, "..."), "View should contain ellipsis for truncated object name")
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long object name")
}

func TestModel_PreviewBinaryContent(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"binary_obj"}, nil),
	}
	client.contentError = nil
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving binary content
	binaryContent := "ELF\x01\x02\x03\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x3e\x00"
	msg := cmd()
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = binaryContent
	m, _ = updateModel(m, contentMsg)

	view := m.View()
	// UI shouldn't break by printing raw binary. It should indicate it's a binary file.
	assert.Assert(t, strings.Contains(view, "(binary content)"), "View should indicate binary content instead of printing raw bytes")
	assert.Assert(t, !strings.Contains(view, "ELF"), "View should not contain the raw binary data")
}

func TestModel_PreviewContentTooManyLines(t *testing.T) {
	var longContent strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&longContent, "line %d\n", i)
	}

	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	// override the content just for this test
	client.contentError = nil
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Set height to 50
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving the content but with 100 lines
	msg := cmd()
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = longContent.String()
	m, _ = updateModel(m, contentMsg)

	view := m.View()
	// Total lines should not exceed the window height significantly.
	// Since lipgloss.JoinHorizontal matches heights, if one column is very tall,
	// the whole view will be tall.
	lineCount := strings.Count(view, "\n")
	assert.Assert(t, lineCount <= 50, "View has %d lines, expected <= 50", lineCount)
}

func TestModel_DownloadAction_FileExists_Abort(t *testing.T) {
	// Create a temp directory for downloads
	downloadDir := t.TempDir()

	// Create a dummy file that already exists
	existingFile := filepath.Join(downloadDir, "obj1")
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	assert.NilError(t, err)

	client := mockGCSClient{
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
	err := os.WriteFile(existingFile, []byte("existing content"), 0644)
	assert.NilError(t, err)

	client := mockGCSClient{
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

	msg := cmd()
	downloadMsg, ok := msg.(tui.DownloadMsg)
	assert.Assert(t, ok, "Expected DownloadMsg")
	expectedNewPath := filepath.Join(downloadDir, "obj1_1")
	assert.Equal(t, downloadMsg.Path, expectedNewPath)
	assert.Assert(t, strings.Contains(m.View(), "Downloading as obj1_1..."), "View should show downloading renamed file status")
}

func TestModel_MultiSelect(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Initially, obj1 is at cursor but not selected
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "✓ obj1"))

	// Press space to select obj1
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ obj1"), "obj1 should be selected")

	// Move cursor down to obj2
	m, _ = pressKey(m, 'j')

	// Press space to select obj2
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ obj1"), "obj1 should still be selected")
	assert.Assert(t, strings.Contains(view, "✓ obj2"), "obj2 should be selected")

	// Press space again to deselect obj2
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ obj1"), "obj1 should still be selected")
	assert.Assert(t, !strings.Contains(view, "✓ obj2"), "obj2 should be deselected")
}

func TestModel_CursorPersistsOnBack(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Initial state: Buckets loaded, cursor at 0 (b1)
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})
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
	assert.Assert(t, !strings.Contains(view, "Objects in"), "Should NOT show Objects header")
	assert.Assert(t, strings.Contains(view, " b2"), "Cursor should be on b2, view:\n%s", view)
}

func TestModel_CursorPersistsOnBack_WithFilter(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot", "blueberry"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Initial state: Buckets loaded
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot", "blueberry"}}}})
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
	// - Filter should be cleared
	// - Cursor should be on blueberry
	view := m.View()
	assert.Assert(t, strings.Contains(view, "blueberry"), "Should contain blueberry")
	// Index 3 in full list: p1, apple, banana, apricot, blueberry -> [0, 1, 2, 3, 4]
	assert.Equal(t, m.Cursor(), 4, "Cursor should be on blueberry (index 4)")
}

func TestModel_CursorPersistsOnBack_Prefix(t *testing.T) {
	rootObjects := simpleObjectList([]string{"file1"}, []string{"folder1/", "folder2/"})
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := rootObjects
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket b1
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
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
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})

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

func TestModel_SearchFilter_BucketsOnly(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{
			{ProjectID: "apple-project", Buckets: []string{"banana"}},
		},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsMsg{Projects: client.projects})

	// Enter search mode and type "apple"
	m, _ = pressKey(m, '/')
	for _, r := range "apple" {
		m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := m.View()
	// Should NOT show "apple-project" because none of its buckets match "apple"
	assert.Assert(t, !strings.Contains(view, "apple-project"), "Should not match on project name")
	assert.Assert(t, !strings.Contains(view, "banana"), "Should not show banana bucket")

	// Now search for "banana"
	m, _ = pressKeyType(m, tea.KeyEsc) // Clear
	m, _ = pressKey(m, '/')
	for _, r := range "banana" {
		m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view = m.View()
	// Should show both "apple-project" and "banana"
	assert.Assert(t, strings.Contains(view, "apple-project"), "Should show project header when a bucket matches")
	assert.Assert(t, strings.Contains(view, "banana"), "Should show matching bucket")
}

func TestModel_SearchFilter(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}}})

	// Enter search mode
	m, _ = pressKey(m, '/')

	// Type 'a', 'p'
	m, _ = pressKey(m, 'a')
	m, _ = pressKey(m, 'p')

	view := m.View()
	// Should show 'apple' and 'apricot', but not 'banana'
	assert.Assert(t, strings.Contains(view, "apple"))
	assert.Assert(t, strings.Contains(view, "apricot"))
	assert.Assert(t, !strings.Contains(view, "banana"))
	assert.Assert(t, strings.Contains(view, "SEARCH: ap"))

	// Exit search mode
	m, _ = pressKeyType(m, tea.KeyEsc)
	view = m.View()
	assert.Assert(t, !strings.Contains(view, "SEARCH: ap"))
}

func TestModel_FuzzySearch(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", true, false) // true enables fuzzy search
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}}})

	// Enter search mode
	m, _ = pressKey(m, '/')

	// Type 'a', 'l', 'e' -> should match 'apple', but not 'apricot'
	m, _ = pressKey(m, 'a')
	m, _ = pressKey(m, 'l')
	m, _ = pressKey(m, 'e')

	view := m.View()
	// Should show 'apple'
	assert.Assert(t, strings.Contains(view, "apple"))
	// Should not show 'banana'
	assert.Assert(t, !strings.Contains(view, "banana"))
	// Should not show 'apricot' because 'l' is not in 'apricot'
	assert.Assert(t, !strings.Contains(view, "apricot"))
	assert.Assert(t, strings.Contains(view, "SEARCH: ale"))
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
	msg := cmd()
	m, _ = updateModel(m, msg)

	// Status should be CLEARED
	assert.Assert(t, !strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "Status should be cleared after timer fires")
	assert.Assert(t, strings.Contains(m.View(), " NORMAL "), "Status should be NORMAL again")
}

	func TestModel_LongBucketList_EnterBucket_ObjectsVisible(t *testing.T) {
	var buckets []string
	for i := 0; i < 100; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 20}) // maxVisible = 10
	m, _ = updateModel(m, tui.BucketsMsg{Projects: client.projects})

	// Scroll to bucket-90. 
	// The list has: [p1 (0), bucket-00 (1), ..., bucket-90 (91), ...]
	for i := 0; i < 91; i++ {
		m, _ = pressKeyType(m, tea.KeyDown)
	}

	// Verify we are on bucket-90
	viewBefore := m.View()
	assert.Assert(t, strings.Contains(viewBefore, "bucket-90"))

	// Enter bucket
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Simulate objects arrival
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "bucket-90", Prefix: "", List: client.objects})

	view := m.View()
	// Check if obj1 is visible in the view
	assert.Assert(t, strings.Contains(view, "obj1"), "Objects should be visible even after scrolling deep in buckets list. View:\n%s", view)
	}

	func TestModel_UI_Wrapping_Bug_Detected(t *testing.T) {
	// This test specifically checks if the rendered columns have the expected height.
	// If wrapping occurs, the number of lines will exceed columnHeight.
	m := tui.NewModel([]string{"p1"}, nil, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 20})

	// maxVisible = 10. columnHeight = 12.
	// We'll add 10 buckets.
	var buckets []string
	for i := 0; i < 10; i++ {
		buckets = append(buckets, "a_fairly_long_bucket_name_to_trigger_wrapping")
	}
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}
	m, _ = updateModel(m, tui.BucketsMsg{Projects: projects})

	view := m.View()
	lines := strings.Split(view, "\n")

	// The view height should be around 17-18 lines.
	// Header(3) + Column(12) + Footer(2) = 17.
	// If wrapping occurs in the left column (Buckets), the column height will increase
	// and JoinHorizontal will make the whole mainContent taller.

	t.Logf("Total lines in view: %d", len(lines))
	assert.Assert(t, len(lines) <= 20, "View height %d exceeded terminal height 20. Wrapping likely occurred!", len(lines))
	}
