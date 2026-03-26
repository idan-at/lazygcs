package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_View_CreationPrompt(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	m, _ := setupTestModel(projects, nil, "/tmp")

	// Must send BucketsPageMsg to populate buckets in model
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	// 1. Bucket creation prompt
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	view := m.View()
	assert.Assert(t, strings.Contains(view, "NEW BUCKET:"), "Should show NEW BUCKET prompt in buckets view")

	// 2. File creation prompt
	m, _ = setupTestModel(projects, nil, "/tmp")
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // Move to b1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyEnter})                     // Enter b1
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	view = m.View()
	assert.Assert(t, strings.Contains(view, "NEW FILE:"), "Should show NEW FILE prompt in objects view")

	// 3. Dir creation prompt (suffix /)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	view = m.View()
	assert.Assert(t, strings.Contains(view, "NEW DIR:"), "Should show NEW DIR prompt when name ends with /")
}
