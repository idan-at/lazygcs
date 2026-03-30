package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_HeaderItemCounts(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}
	objects := simpleObjectList([]string{"obj1", "obj2", "obj3"}, []string{"folder1/"})
	m, client := setupTestModel(projects, objects, "/tmp")

	// 1. Initial view: Buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2"}})
	view := m.View()

	// Check Buckets header. Total items: 3 (p1, b1, b2). Cursor at p1 (position 1)
	assert.Assert(t, strings.Contains(view, "Buckets (1/3)"), "Buckets header should show cursor/total. View:\n%s", view)

	// 2. Enter bucket b1
	m = enterBucket(m, projects, "b1", client.objects)
	view = m.View()

	// Check Objects header. Total items: 3 objects + 1 prefix = 4. Cursor at index 0 (pos 1).
	assert.Assert(t, strings.Contains(view, "Objects in b1 (1/4)"), "Objects header should show 1/4. View:\n%s", view)

	// 3. Select one object
	m, _ = pressKey(m, ' ') // Select folder1/ (cursor is at 0)
	view = m.View()
	assert.Assert(t, strings.Contains(view, "Objects in b1 (1/4)"), "Objects header should show 1/4. View:\n%s", view)

	// 4. Select another object
	m, _ = pressKey(m, 'j') // Move to obj1
	m, _ = pressKey(m, ' ') // Select obj1
	view = m.View()
	assert.Assert(t, strings.Contains(view, "Objects in b1 (2/4)"), "Objects header should show 2/4. View:\n%s", view)

	// 5. Filter objects
	m, _ = pressKey(m, '/') // Enter search mode
	m, _ = pressKey(m, 'o')
	m, _ = pressKey(m, 'b')
	m, _ = pressKey(m, 'j')
	m, _ = pressKey(m, '1')
	// Filter matches only obj1 (1 item). Cursor is at index 0 (pos 1).
	view = m.View()
	assert.Assert(t, strings.Contains(view, "Objects in b1 (1/1)"), "Objects header should show 1/1 when filtered. View:\n%s", view)
}

func TestModel_HeaderItemCounts_LongNames(t *testing.T) {
	longBucketName := "a_very_very_very_long_bucket_name_that_exceeds_normal_width"
	longDirName := "a_very_very_very_long_directory_name_that_exceeds_normal_width/"
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{longBucketName}}}
	objects := simpleObjectList([]string{"obj1"}, []string{longDirName})
	m, client := setupTestModel(projects, objects, "/tmp")

	// Set small width to force truncation
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	// 1. Initial view: Buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{longBucketName}})

	// 2. Enter bucket
	m = enterBucket(m, projects, longBucketName, client.objects)

	// 3. Enter long directory
	m, _ = pressKey(m, 'j') // assume we need to enter dir, but let's just check Objects header first
	view := m.View()

	// Check if the count (2/2) is still visible despite truncation
	assert.Assert(t, strings.Contains(view, "(2/2)"), "Objects header should keep the count visible even with long bucket name. View:\n%s", view)
}
