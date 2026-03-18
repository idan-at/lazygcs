package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_SearchFetchesMetadata(t *testing.T) {
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

	// Enter search mode
	m, _ = pressKey(m, '/')

	// Type '2' to filter down to "folder2/"
	_, cmd := pressKey(m, '2')

	// The cmd returned should be the fetchPrefixMetadataByName command
	assert.Assert(t, cmd != nil, "A command should be returned to fetch metadata for the newly focused item")

	// Let's actually execute the command to see if it yields a MetadataMsg
	msg := resolveFetchCmd(cmd)
	_, ok := msg.(tui.MetadataMsg)
	assert.Assert(t, ok, "Expected a MetadataMsg to be returned")
}

func TestModel_SearchFilter(t *testing.T) {
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}})

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
	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}}},
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", true, false) // true enables fuzzy search
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Load buckets
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"apple", "banana", "apricot"}})

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
