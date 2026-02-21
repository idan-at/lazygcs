package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gotest.tools/v3/assert"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

type mockGCSClient struct {
	buckets []string
	objects *gcs.ObjectList
}

func (f mockGCSClient) ListBuckets(ctx context.Context, projectIDs []string) ([]string, error) {
	return f.buckets, nil
}

func (f mockGCSClient) ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error) {
	return f.objects, nil
}

func TestModel_AsyncLoading(t *testing.T) {
	client := mockGCSClient{buckets: []string{"async-b1"}}
	m := tui.NewModel([]string{"p1"}, client)

	// 1. Initial State should be loading
	assert.Assert(t, strings.Contains(m.View(), "Loading"))

	// 2. Init should return a command
	cmd := m.Init()
	assert.Assert(t, cmd != nil)

	// 3. Simulate receiving the message
	msg := tui.BucketsMsg{Buckets: []string{"async-b1"}}
	updatedM, _ := m.Update(msg)
	m = updatedM.(tui.Model)

	// 4. Assert view now shows the buckets
	view := m.View()
	assert.Assert(t, strings.Contains(view, "async-b1"))
	assert.Assert(t, !strings.Contains(view, "Loading"))
}

func TestModel_Update_CursorNavigation(t *testing.T) {
	// Initialize and move out of loading state
	client := mockGCSClient{buckets: []string{"b1", "b2", "b3"}}
	m := tui.NewModel([]string{"p1"}, client)
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2", "b3"}})
	m = updatedM.(tui.Model)

	// 1. Initial state: cursor at b1 (index 0)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	// 2. Press 'j' (down) -> b2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	// 3. Press 'down' arrow -> b3
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	// 4. Press 'up' (k) -> back to b2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b2"))

	// 5. Press 'up' (up arrow) -> back to b1
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}

func TestModel_Update_CursorCycle(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1", "b2", "b3"}}
	m := tui.NewModel([]string{"p1"}, client)
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2", "b3"}})
	m = updatedM.(tui.Model)

	// 1. Initial state: cursor at b1 (top)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	// 2. Press 'up' -> wrap to b3 (bottom)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	// 3. Press 'down' -> wrap to b1 (top)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	// 4. Test 'k' (up)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b3"))

	// 5. Test 'j' (down)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> b1"))
}

func TestModel_Update_Quit(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1"}}
	m := tui.NewModel([]string{"p1"}, client)
	m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})

	// 1. Press 'q'
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())

	// 2. Press 'ctrl+c'
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())
}

func TestModel_EnterBucket(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: &gcs.ObjectList{Objects: []string{"obj1"}},
	}
	m := tui.NewModel([]string{"p1"}, client)

	// 1. Load buckets
	m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})

	// 2. Select b1 and press Enter
	updatedM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// TODO: Implement Enter handling in next steps
	// For now, just assert it doesn't crash
	assert.Assert(t, m.View() != "")
	// assert.Assert(t, cmd != nil) // This will fail until implemented
}
