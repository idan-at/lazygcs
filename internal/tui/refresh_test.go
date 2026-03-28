package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_Actions_RefreshBuckets(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// 1. Initial load
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	// Select bucket b1 (index 1 after project header)
	m, _ = pressKey(m, 'j')

	// Press 'R' to refresh
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	assert.Assert(t, cmd != nil)

	// In Buckets view, "Loading..." should appear in the preview pane
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Bucket Information"), "View should show Bucket Information")
	assert.Assert(t, strings.Contains(view, "Loading..."), "View should show 'Loading...' in the preview pane")
}

func TestModel_Actions_RefreshProjects(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// 1. Initial load
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	// Project p1 is at cursor 0
	assert.Equal(t, m.Cursor(), 0)

	// Press 'R' to refresh
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	assert.Assert(t, cmd != nil)

	// "Loading project info..." should appear in the preview pane
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Project Information"), "View should show Project Information")
	assert.Assert(t, strings.Contains(view, "Loading project info..."), "View should show 'Loading project info...' in the preview pane")
}

func TestModel_Actions_RefreshObjectsPreservesCursor(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2", "obj3"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client
	m = enterBucket(m, projects, "b1", objects)

	// Move cursor to obj2 (index 1)
	m, _ = pressKey(m, 'j')
	assert.Equal(t, m.Cursor(), 1)

	// Press 'R' to refresh
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})

	// m.Objects() should be cleared (but it's public so we can check it)
	assert.Equal(t, len(m.Objects()), 0)

	// Simulate receipt of refreshed objects
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: objects})

	// Cursor should be restored to 1 (obj2)
	assert.Equal(t, m.Cursor(), 1, "Cursor should be preserved after refresh")
}

func TestModel_Actions_RefreshVersions(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client
	m = enterBucket(m, projects, "b1", objects)

	// Press 'V' to show versions
	m, _ = pressKey(m, 'v')
	assert.Assert(t, m.ShowVersions())

	// Press 'R' to refresh
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})

	// Objects should be cleared
	assert.Equal(t, len(m.Objects()), 0)

	// Simulate receipt of refreshed objects
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: objects})

	// In version view, "Loading versions..." should appear in the preview pane
	// after the list is refreshed but before version data arrives.
	view := m.View()
	assert.Assert(t, strings.Contains(view, "Loading versions..."), "View should show 'Loading versions...'")
}

func TestModel_Actions_RefreshBucketsPreservesCursor(t *testing.T) {
	projects := []gcs.ProjectBuckets{
		{ProjectID: "p1", Buckets: []string{"b1", "b2"}},
	}
	objects := simpleObjectList(nil, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")

	// 1. Initial load
	// List: [p1, b1, b2]
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2"}})

	// Select b2 (index 2)
	m, _ = pressKey(m, 'j') // to b1
	m, _ = pressKey(m, 'j') // to b2
	assert.Equal(t, m.Cursor(), 2)

	// 2. Press 'R' to refresh
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})

	// 3. Simulate receipt of refreshed buckets with a new bucket 'b0' added before b1
	// New list for p1: [b0, b1, b2]
	// Expected full list: [p1, b0, b1, b2]
	// b2 should now be at index 3.
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b0", "b1", "b2"}})

	// Cursor should be restored to 3 (b2)
	assert.Equal(t, m.Cursor(), 3, "Cursor should follow the bucket name 'b2', not the index")
}
