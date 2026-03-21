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

func TestModel_ProjectSpecificLoading(t *testing.T) {
	client := &mockGCSClient{}
	// Two projects: p1 will load immediately, p2 will stay loading
	mModel := tui.NewModel([]string{"p1", "p2"}, client, "/tmp", false, false)
	m := &mModel

	// Initially both should have spinners and be visible
	view := m.View()
	assert.Assert(t, strings.Contains(view, "p1"))
	assert.Assert(t, strings.Contains(view, "p2"))

	// Let's simulate p1 finishing
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	view = m.View()
	assert.Assert(t, strings.Contains(view, "b1")) // p1 loaded
	assert.Assert(t, strings.Contains(view, "p2")) // p2 still there
}

func TestModel_AsyncLoading(t *testing.T) {
	client := &mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"async-b1"}}}}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	assert.Assert(t, strings.Contains(m.View(), "p1"))

	cmd := m.Init()
	assert.Assert(t, cmd != nil)

	msg := resolveFetchCmd(cmd)
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batchMsg {
			if c != nil {
				m, _ = updateModel(m, c())
			}
		}
	} else {
		m, _ = updateModel(m, msg)
	}

	view := m.View()
	assert.Assert(t, strings.Contains(view, "async-b1"))
	assert.Assert(t, !strings.Contains(view, "Loading"))
}

func TestModel_Pagination_Buckets(t *testing.T) {
	var buckets []string
	for i := 0; i < 50; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}},
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 10})

	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: buckets})
	m, _ = pressKey(m, 'j')

	view := m.View()
	// Should not show all 50 buckets
	assert.Assert(t, strings.Contains(view, "bucket-00"))
	assert.Assert(t, !strings.Contains(view, "bucket-49"))
}

func TestModel_LongBucketList_EnterBucket_ObjectsVisible(t *testing.T) {
	var buckets []string
	for i := 0; i < 100; i++ {
		buckets = append(buckets, fmt.Sprintf("bucket-%02d", i))
	}
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 20}) // maxVisible = 10
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: client.projects[0].ProjectID, Buckets: client.projects[0].Buckets})

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

func TestModel_EnterBucket(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1", "obj2"}, nil),
	}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	m, _ = pressKey(m, 'j')

	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Assert(t, cmd != nil)

	// Simulate objects fetch result
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	view := m.View()
	assert.Assert(t, strings.Contains(view, "b1"))
	assert.Assert(t, strings.Contains(view, "obj1"))
}

func TestModel_SearchFilter_BucketsOnly(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{
			{ProjectID: "apple-project", Buckets: []string{"banana"}},
		},
	}
	mModel := tui.NewModel([]string{"apple-project"}, client, "/tmp", false, false)
	m := &mModel
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: client.projects[0].ProjectID, Buckets: client.projects[0].Buckets})

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
