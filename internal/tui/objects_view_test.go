package tui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

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
	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	// Simulate receiving the content
	msg2 := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg2)

	// Verify view shows the content
	view := m.View()
	assert.Assert(t, strings.Contains(view, "content of obj1"))
}

func TestModel_InitialObjectPreview(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)

	// Process ObjectsMsg - this should trigger initial fetchContent
	msg := tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects}
	m, cmd := updateModel(m, msg)

	// Verify fetchContent was triggered automatically
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	// Simulate receiving the content
	contentMsg := resolveFetchCmd(cmd)
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
	m, _ = updateModel(m, resolveFetchCmd(cmd))

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
	client := &mockGCSClient{
		projects:     []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:      simpleObjectList([]string{"obj1"}, nil),
		contentError: fmt.Errorf("permission denied"),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving the error message
	msg := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg)

	// Verify view shows the error
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Error: permission denied"))
}

func TestModel_PrefixMetadata_VirtualDirectory(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: &gcs.ObjectList{
			Prefixes: []gcs.PrefixMetadata{{Name: "folder1/"}},
		},
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Should be loading metadata for the prefix
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading metadata..."))

	// Simulate receiving the error message (virtual directory)
	msg := resolveFetchCmd(cmd)
	metaMsg := msg.(tui.MetadataMsg)
	metaMsg.Err = fmt.Errorf("object doesn't exist")
	m, _ = updateModel(m, metaMsg)

	// Verify view shows Folder (Virtual) and no loading indicator
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Folder (Virtual)"))
	assert.Assert(t, !strings.Contains(view, "Loading metadata..."))
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

func TestModel_Update_ObjectCursorCycle(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// First item is obj1
	assert.Assert(t, strings.Contains(m.View(), " obj1"))

	m, _ = pressKeyType(m, tea.KeyUp)
	assert.Assert(t, strings.Contains(m.View(), " obj2"))

	m, _ = pressKeyType(m, tea.KeyDown)
	assert.Assert(t, strings.Contains(m.View(), " obj1"))
}

func TestModel_EnterPrefix(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"file1"}, []string{"folder1/"}),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 50})

	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

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

	// Verify that RELATIVE names are present.
	assert.Assert(t, strings.Contains(view, " file2.txt"))
	assert.Assert(t, strings.Contains(view, " sub/"))
}

func TestModel_SelectObject(t *testing.T) {
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
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	// Should show metadata in preview
	assert.Assert(t, strings.Contains(view, "obj1"))
	assert.Assert(t, strings.Contains(view, "1.0 KB"))
	assert.Assert(t, strings.Contains(view, "text/plain"))
}

func TestModel_SelectPrefix(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b"}}},
		objects: &gcs.ObjectList{
			Prefixes: []gcs.PrefixMetadata{{Name: "folder1/"}},
			Objects:  []gcs.ObjectMetadata{{Name: "file1"}},
		},
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// 1. Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b"}}}, "b", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b", Prefix: "", List: client.objects})

	// Initial fetch for first item (prefix)
	assert.Assert(t, cmd != nil)
	msg := resolveFetchCmd(cmd)
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

func TestModel_Pagination_Objects(t *testing.T) {
	var objects []string
	for i := 0; i < 50; i++ {
		objects = append(objects, fmt.Sprintf("obj-%02d", i))
	}
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList(objects, nil),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 10})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "obj-00"))
	assert.Assert(t, !strings.Contains(view, "obj-49"))
}

func TestModel_StaleObjectsMsg(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}
	objects := simpleObjectList([]string{"obj-from-b1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter b1
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2"}})
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

func TestModel_Truncation(t *testing.T) {
	longName := "this_is_a_very_long_object_name_that_should_be_truncated_to_fit_in_the_column"
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{longName}}},
		objects:  simpleObjectList([]string{longName}, nil),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	// Set a specific width where we know it should truncate everywhere
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 40, Height: 50})

	// 1. Check Bucket truncation
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{longName}})
	m, _ = pressKey(m, 'j')

	view := m.View()
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long bucket name")

	// 2. Check Object truncation
	m, _ = pressKeyType(m, tea.KeyEnter)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: longName, Prefix: "", List: client.objects})

	view = m.View()
	assert.Assert(t, strings.Contains(view, "..."), "View should contain ellipsis for truncated object name")
	assert.Assert(t, !strings.Contains(view, longName), "View should NOT contain the full long object name")
}

func TestModel_PreviewBinaryContent(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"binary_obj"}, nil),
	}
	client.contentError = nil
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving binary content
	binaryContent := "ELF\x01\x02\x03\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x3e\x00"
	msg := resolveFetchCmd(cmd)
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = binaryContent
	m, _ = updateModel(m, contentMsg)

	view := m.View()
	assert.Assert(t, strings.Contains(view, "(binary content)"), "View should indicate binary content instead of printing raw bytes")
	assert.Assert(t, !strings.Contains(view, "ELF"), "View should not contain the raw binary data")
}

func TestModel_PreviewContentTooManyLines(t *testing.T) {
	var longContent strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&longContent, "line %d\n", i)
	}

	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	client.contentError = nil
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", nil)
	m, cmd := updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Simulate receiving the content but with 100 lines
	msg := resolveFetchCmd(cmd)
	contentMsg := msg.(tui.ContentMsg)
	contentMsg.Content = longContent.String()
	m, _ = updateModel(m, contentMsg)

	view := m.View()
	lineCount := strings.Count(view, "\n")
	assert.Assert(t, lineCount <= 50, "View has %d lines, expected <= 50", lineCount)
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
	assert.Assert(t, !strings.Contains(view, "✓ 📄 obj1"))

	// Press space to select obj1
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ 📄 obj1"), "obj1 should be selected")

	// Move cursor down to obj2
	m, _ = pressKey(m, 'j')

	// Press space to select obj2
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ 📄 obj1"), "obj1 should still be selected")
	assert.Assert(t, strings.Contains(view, "✓ 📄 obj2"), "obj2 should be selected")

	// Press space again to deselect obj2
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, strings.Contains(view, "✓ 📄 obj1"), "obj1 should still be selected")
	assert.Assert(t, !strings.Contains(view, "✓ 📄 obj2"), "obj2 should be deselected")
}

func TestModel_UnknownContentTypeFallback(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects: &gcs.ObjectList{
			Objects: []gcs.ObjectMetadata{{
				Name:        "backup.sql.gz",
				Size:        1024,
				ContentType: "", // Empty to trigger fallback
			}},
		},
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Enter bucket
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	// Should show inferred type instead of 'unknown'
	assert.Assert(t, strings.Contains(view, "application/gzip"), "View should infer application/gzip type for .gz extensions")
}

func TestModel_CommonExtensionContentTypeFallback(t *testing.T) {
	testCases := []struct {
		filename      string
		expectedTypes []string
	}{
		{"script.py", []string{"text/x-python", "text/python", "application/x-python-code"}},
		{"main.go", []string{"text/x-go", "text/go"}},
		{"query.sql", []string{"application/x-sql", "application/sql"}},
		{"readme.md", []string{"text/markdown", "text/x-markdown"}},
		{"setup.sh", []string{"application/x-sh", "application/sh", "text/x-sh"}},
		{"config.yaml", []string{"application/x-yaml", "application/yaml", "text/yaml"}},
		{"config.yml", []string{"application/x-yaml", "application/yaml", "text/yaml"}},
		{"unknown.xyz", []string{"unknown"}},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			client := &mockGCSClient{
				projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
				objects: &gcs.ObjectList{
					Objects: []gcs.ObjectMetadata{{
						Name:        tc.filename,
						Size:        1024,
						ContentType: "", // Empty to trigger fallback
					}},
				},
			}
			mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
			m := &mModel
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

			// Enter bucket
			m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

			view := m.View()
			matched := false
			for _, expectedType := range tc.expectedTypes {
				if strings.Contains(view, expectedType) {
					matched = true
					break
				}
			}
			if !matched {
				t.Logf("Failed: Expected one of %v but view is:\n%s", tc.expectedTypes, view)
			}
			assert.Assert(t, matched, "View should infer one of %v types for %s", tc.expectedTypes, tc.filename)
		})
	}
}
