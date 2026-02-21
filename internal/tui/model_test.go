package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gotest.tools/v3/assert"
	"lazygcs/internal/tui"
)

func TestModel_View(t *testing.T) {
	m := tui.InitialModel([]string{"bucket-1", "bucket-2"})

	view := m.View()

	assert.Assert(t, strings.Contains(view, "bucket-1"))
	assert.Assert(t, strings.Contains(view, "bucket-2"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	m := tui.InitialModel([]string{"b1", "b2", "b3"})

	// 1. Initial state: View should imply selection (e.g., "> b1") or at least just list items for now.
	// But our test expects "> b1", so we must implement that.
	view := m.View()
	// If View doesn't show cursor yet, this will fail, which is good.
	if !strings.Contains(view, "> b1") {
		t.Fatalf("Expected initial view to show selection on b1. Got:\n%s", view)
	}

	// 2. Press 'j' (down)
	updatedM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 3. Assert cursor moved to b2
	view = m.View()
	assert.Assert(t, !strings.Contains(view, "> b1"))
	assert.Assert(t, strings.Contains(view, "> b2"))

	// 4. Press 'k' (up)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)

	// 5. Assert cursor back at b1
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}
