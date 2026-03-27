package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"gotest.tools/v3/assert"
)

func TestToggleVersions(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"file1.txt"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")

	// 1. Navigate to object view and highlight file1.txt
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover file1.txt

	// 2. Press 'v' to toggle versions
	m, _ = pressKey(m, 'v')
	assert.Assert(t, m.ShowVersions(), "ShowVersions should be true after pressing 'v'")

	// 3. Press 'v' again to toggle off
	m, _ = pressKey(m, 'v')
	assert.Assert(t, !m.ShowVersions(), "ShowVersions should be false after pressing 'v' again")

	// 4. Test mutual exclusivity with Metadata ('i')
	// Toggle versions on
	m, _ = pressKey(m, 'v')
	assert.Assert(t, m.ShowVersions(), "ShowVersions should be true")
	assert.Assert(t, !m.ShowMetadata(), "ShowMetadata should be false")

	// Press 'i' to show metadata
	m, _ = pressKey(m, 'i')
	assert.Assert(t, !m.ShowVersions(), "ShowVersions should be false when metadata is shown")
	assert.Assert(t, m.ShowMetadata(), "ShowMetadata should be true")

	// Press 'v' to show versions again
	m, _ = pressKey(m, 'v')
	assert.Assert(t, m.ShowVersions(), "ShowVersions should be true")
	assert.Assert(t, !m.ShowMetadata(), "ShowMetadata should be false when versions are shown")
}

func TestVersioningNotSupported(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"file1.txt"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	client.versioningDisabled = true

	// 1. Navigate to object view and highlight file1.txt
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'j') // hover file1.txt

	// 2. Press 'v' to toggle versions
	m, cmd := pressKey(m, 'v')
	assert.Assert(t, m.ShowVersions(), "ShowVersions should be true")

	// Verify it shows loading first
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Loading versions..."), "Should show loading spinner initially")

	// 3. Resolve the fetch command
	msg := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg)

	// Verify it shows "not enabled" message
	view = m.View()
	assert.Assert(t, strings.Contains(view, "Versioning is not enabled for this bucket."), "Should show versioning not enabled message")
}

func TestVersionsView_Trimming(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}

	// Create a large number of versions (e.g., 50)
	var objects []gcs.ObjectMetadata
	for i := 0; i < 50; i++ {
		objects = append(objects, gcs.ObjectMetadata{
			Name:       "file1.txt",
			Bucket:     "b1",
			Generation: int64(i),
			Size:       1024,
			Updated:    time.Now(),
		})
	}

	m, client := setupTestModel(projects, &gcs.ObjectList{Objects: []gcs.ObjectMetadata{{Name: "file1.txt"}}}, "/tmp")
	client.mockVersions = objects

	// Set a reasonable height, maxVisible will be roughly height - 10 - 19 = 20 - 29 < 0? No, let's use 40.
	// height 40 -> maxVisible = 40 - 10 = 30.
	// maxLines for versions = maxVisible - 4 = 26.
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// 1. Navigate to object view and highlight file1.txt
	m = enterBucket(m, projects, "b1", &gcs.ObjectList{Objects: []gcs.ObjectMetadata{{Name: "file1.txt"}}})
	m, _ = pressKey(m, 'j') // hover file1.txt

	// 2. Press 'v' and resolve
	m, cmd := pressKey(m, 'v')
	msg := resolveFetchCmd(cmd)
	m, _ = updateModel(m, msg)

	// 3. Verify View contains the "... and X more" message
	view := m.View()

	// We have 50 items. maxVisible is (40-10) = 30. maxLines for versions is 30-4 = 26.
	// So 50 - 26 = 24 items should be truncated.
	// The exact text should be "... and 24 more". We'll just check for "... and " to be robust.
	assert.Assert(t, strings.Contains(view, "... and "), "Should vertically truncate the list of versions and show the '... and X more' message")
}
