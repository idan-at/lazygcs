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
