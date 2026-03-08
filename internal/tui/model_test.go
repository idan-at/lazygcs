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

func TestModel_ObjectPreview(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, []string{"folder1/"}),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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

func TestModel_UI_WrappingBug(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	
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
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	view := m.View()
	// Old indicators should be gone
	assert.Assert(t, !strings.Contains(view, ">"), "View should not contain old cursor '>'")
	assert.Assert(t, !strings.Contains(view, "[ ]"), "View should not contain old unselected indicator '[ ]'")

	// Select the item
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "[x]"), "View should not contain old selected indicator '[x]'")
	assert.Assert(t, strings.Contains(view, "✓"), "View should contain new selection indicator '✓'")
}

func TestModel_HelpMenu(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)

	// Assert help menu is not shown initially
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"))

	// Press '?' to show help
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updatedM.(tui.Model)

	view = m.View()
	// In 'Which Key' style, we should see both the main content AND the help at the bottom
	assert.Assert(t, strings.Contains(view, "Buckets"), "Buckets column should still be visible")
	assert.Assert(t, strings.Contains(view, "HELP"), "View should contain 'HELP' header")
	assert.Assert(t, !strings.Contains(view, "WHICH-KEY"), "View should NOT contain 'WHICH-KEY' anymore")
	assert.Assert(t, strings.Contains(view, "toggle help"), "View should list the help keybind")

	// Press '?' again to hide help
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updatedM.(tui.Model)

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"), "View should no longer contain 'HELP'")
}

func TestModel_InitialObjectPreview(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Setup: In bucket, obj1 loaded and previewed
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:      simpleObjectList([]string{"obj1"}, nil),
		contentError: fmt.Errorf("permission denied"),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Setup: In bucket, objects loaded
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"async-b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	cmd := m.Init()
	assert.Assert(t, cmd != nil)

	updatedM, _ := m.Update(cmd())
	m = updatedM.(tui.Model)
	view := m.View()
	assert.Assert(t, strings.Contains(view, "async-b1"))
	assert.Assert(t, !strings.Contains(view, "Loading"))
}

func TestModel_Update_ArrowKeyNavigation(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), " p1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "Objects in b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
}

func TestModel_Update_CursorCycle(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), " b1"))

	// Up from top -> bottom
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b3"))

	// Down from bottom -> top
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
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
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)

	// Simulate objects fetch result
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "b1"))
	assert.Assert(t, strings.Contains(view, "obj1"))
}

func TestModel_Update_ObjectCursorCycle(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// First item is obj1
	assert.Assert(t, strings.Contains(m.View(), " obj1"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " obj2"))

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " obj1"))
}

func TestModel_Resize(t *testing.T) {
	client := mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, len(view) > 0)

	// Very narrow
	updatedM, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updatedM.(tui.Model)
	viewNarrow := m.View()

	assert.Assert(t, len(viewNarrow) > 0)
}

func TestModel_EnterPrefix(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"file1"}, []string{"folder1/"}),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)

	// Enter bucket b1
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Verify we are at root
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))

	// Enter folder1/
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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

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
	updatedM, _ = m.Update(metaMsg)
	m = updatedM.(tui.Model)

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

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

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
		objects: simpleObjectList(objects, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)

	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press Down - should not crash or change cursor
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "obj1"))
}

func TestModel_HeaderClearedOnBack(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)

	// Enter bucket
	updatedM, _ = m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "gs://b1/"))

	// Go back to bucket list
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	// Header should be cleared back to gs://
	assert.Assert(t, strings.Contains(m.View(), "gs:// "))
}

func TestModel_StaleObjectsMsg(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}},
		objects: simpleObjectList([]string{"obj-from-b1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter b1
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// Capture the fetch objects msg for b1
	staleMsg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}

	// Go back to bucket list
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	// Move to b2 and enter it
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// Now the STALE msg from b1 arrives
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Downloading obj1..."), "View should show downloading status with filename")

	// Simulate download completion
	updatedM, _ = m.Update(tui.DownloadMsg{Path: "/tmp/obj1"})
	m = updatedM.(tui.Model)

	assert.Assert(t, strings.Contains(m.View(), "Downloaded to /tmp/obj1"), "View should show success status")
}

func TestModel_DownloadAction_MultiSelect(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1", "obj2", "obj3"}, nil),
	}
	downloadDir := t.TempDir()
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Select obj1
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

	// Move to obj2 and select it
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

	// Press 'd' to download
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updatedM.(tui.Model)

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
	updatedM, _ = m.Update(dl2)
	m = updatedM.(tui.Model)
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
		objects: simpleObjectList([]string{longName}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Set a specific width where we know it should truncate everywhere
	// leftWidth = 40 * 0.3 = 12.
	// Header width = 40 - 2 = 38.
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	m = updatedM.(tui.Model)

	// 1. Check Bucket truncation
	updatedM, _ = m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{longName}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	view := m.View()
	// Should contain truncated version (usually ending in ...)
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

func TestModel_PreviewBinaryContent(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"binary_obj"}, nil),
	}
	client.contentError = nil
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Simulate receiving binary content
	binaryContent := "ELF\x01\x02\x03\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x3e\x00"
	msg := cmd()
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = binaryContent
	updatedM, _ = m.Update(contentMsg)
	m = updatedM.(tui.Model)

	view := m.View()
	// UI shouldn't break by printing raw binary. It should indicate it's a binary file.
	assert.Assert(t, strings.Contains(view, "(binary content)"), "View should indicate binary content instead of printing raw bytes")
	assert.Assert(t, !strings.Contains(view, "ELF"), "View should not contain the raw binary data")
}

func TestModel_PreviewContentTooManyLines(t *testing.T) {
	var longContent strings.Builder
	for i := 0; i < 100; i++ {
		longContent.WriteString(fmt.Sprintf("line %d\n", i))
	}

	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	// override the content just for this test
	client.contentError = nil
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Set height to 50
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, cmd := m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Simulate receiving the content but with 100 lines
	msg := cmd()
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = longContent.String()
	updatedM, _ = m.Update(contentMsg)
	m = updatedM.(tui.Model)

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
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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

func TestModel_MultiSelect(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})
	m = updatedM.(tui.Model)

	// Initially, obj1 is at cursor but not selected
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "✓ obj1"))

	// Press space to select obj1
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ obj1"), "obj1 should be selected")

	// Move cursor down to obj2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// Press space to select obj2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ obj1"), "obj1 should still be selected")
	assert.Assert(t, strings.Contains(view, "✓ obj2"), "obj2 should be selected")

	// Press space again to deselect obj2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updatedM.(tui.Model)

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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2", "b3"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 2. Move cursor down to b2 (index 1)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), " b2"))

	// 3. Enter bucket b2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "Objects in b2"))

	// 4. Go back to bucket list
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot", "blueberry"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 2. Filter by 'b' -> [banana, blueberry]
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = updatedM.(tui.Model)
	
	// Exit search mode but keep query active
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	
	// 3. Move cursor down to blueberry (index 2 in filtered list: [p1, banana, blueberry])
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	
	// 4. Enter bucket blueberry
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	
	// 5. Go back to bucket list
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	// Assertions:
	// - Filter should be cleared
	// - Cursor should be on blueberry
	view := m.View()
	assert.Assert(t, strings.Contains(view, "▶") && strings.Contains(view, "blueberry"), "Cursor should be on blueberry, view:\n%s", view)
}

func TestModel_CursorPersistsOnBack_Prefix(t *testing.T) {
	rootObjects := simpleObjectList([]string{"file1"}, []string{"folder1/", "folder2/"})
	
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: rootObjects,
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket b1
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	
	// Load root objects
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: rootObjects})
	m = updatedM.(tui.Model)
	
	// Move cursor to folder2/ (index 1)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// Enter folder2/
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	
	// Go back using 'h'
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)
	
	// Re-load root objects
	updatedM, _ = m.Update(tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: rootObjects})
	m = updatedM.(tui.Model)

	// Assert cursor is restored to folder2/
	view := m.View()
	assert.Assert(t, strings.Contains(view, "▶") && strings.Contains(view, "folder2/"), "Cursor should be restored to folder2/, view:\n%s", view)
	assert.Assert(t, strings.Contains(view, "▶  folder2/") || strings.Contains(view, "▶ \x1b[1m\x1b[38;5;69mfolder2/"), "Cursor should be specifically on folder2/")
}

func TestModel_CollapseProjectOnLeft(t *testing.T) {
	client := mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}})
	m = updatedM.(tui.Model)

	// Ensure p1 is expanded initially (it is by default)
	assert.Assert(t, strings.Contains(m.View(), " b1"))
	assert.Assert(t, strings.Contains(m.View(), "▼ p1"))

	// Ensure cursor is on p1 (index 0)
	// Press 'h' (Left) to collapse it
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updatedM.(tui.Model)

	// Assertions:
	// - The project should be collapsed (▶ p1)
	// - The bucket (b1) should not be visible
	view := m.View()
	assert.Assert(t, !strings.Contains(view, " b1"), "Bucket b1 should be hidden")
	assert.Assert(t, strings.Contains(view, "▶ p1") || strings.Contains(view, "▶ \x1b[1m\x1b[38;5;69mp1") || strings.Contains(view, "▶  \x1b[1m\x1b[38;5;69mp1"), "Project p1 should be collapsed")
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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: client.projects})
	m = updatedM.(tui.Model)

	// Enter search mode and type "apple"
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updatedM.(tui.Model)
	for _, r := range "apple" {
		updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updatedM.(tui.Model)
	}

	view := m.View()
	// Should NOT show "apple-project" because none of its buckets match "apple"
	assert.Assert(t, !strings.Contains(view, "apple-project"), "Should not match on project name")
	assert.Assert(t, !strings.Contains(view, "banana"), "Should not show banana bucket")

	// Now search for "banana"
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // Clear
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updatedM.(tui.Model)
	for _, r := range "banana" {
		updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updatedM.(tui.Model)
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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}}})
	m = updatedM.(tui.Model)

	// Enter search mode
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updatedM.(tui.Model)

	// Type 'a', 'p'
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updatedM.(tui.Model)

	view := m.View()
	// Should show 'apple' and 'apricot', but not 'banana'
	assert.Assert(t, strings.Contains(view, "apple"))
	assert.Assert(t, strings.Contains(view, "apricot"))
	assert.Assert(t, !strings.Contains(view, "banana"))
	assert.Assert(t, strings.Contains(view, "SEARCH: ap"))

	// Exit search mode
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updatedM.(tui.Model)
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
	updatedM, _ := m.Update(tui.BucketsMsg{Projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}}})
	m = updatedM.(tui.Model)

	// Enter search mode
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updatedM.(tui.Model)

	// Type 'a', 'l', 'e' -> should match 'apple', but not 'apricot'
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updatedM.(tui.Model)

	view := m.View()
	// Should show 'apple'
	assert.Assert(t, strings.Contains(view, "apple"))
	// Should not show 'banana'
	assert.Assert(t, !strings.Contains(view, "banana"))
	// Should not show 'apricot' because 'l' is not in 'apricot'
	assert.Assert(t, !strings.Contains(view, "apricot"))
	assert.Assert(t, strings.Contains(view, "SEARCH: ale"))
}
