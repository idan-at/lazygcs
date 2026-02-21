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
		objects: &gcs.ObjectList{Objects: []string{"obj1", "obj2"}},
	}
	m := tui.NewModel([]string{"p1"}, client)

	// 1. Load buckets
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)

	// 2. Select b1 and press Enter
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// 3. Assert loading state for objects
	assert.Assert(t, cmd != nil)

	// 4. Simulate ObjectsMsg (Fetch complete)
	msg := tui.ObjectsMsg{List: &gcs.ObjectList{Objects: []string{"obj1", "obj2"}}}
	updatedM, _ = m.Update(msg)
	m = updatedM.(tui.Model)

	// 5. Assert View
	view := m.View()
	assert.Assert(t, strings.Contains(view, "b1"))
	assert.Assert(t, strings.Contains(view, "obj1"))
	assert.Assert(t, strings.Contains(view, "> obj1"))
}

func TestModel_Update_ObjectCursorCycle(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: &gcs.ObjectList{Objects: []string{"obj1", "obj2"}},
	}
	m := tui.NewModel([]string{"p1"}, client)

	// 1. Load buckets and enter b1
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
	m = updatedM.(tui.Model)

	// 2. Initial object state: cursor at obj1
	assert.Assert(t, strings.Contains(m.View(), "> obj1"))

	// 3. Cycle Up -> obj2
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> obj2"))

	// 4. Cycle Down -> obj1
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updatedM.(tui.Model)
	assert.Assert(t, strings.Contains(m.View(), "> obj1"))
}

func TestModel_Resize(t *testing.T) {
	client := mockGCSClient{buckets: []string{"b1"}}
	m := tui.NewModel([]string{"p1"}, client)
	
	// 1. Initial size (default 0x0)
	// We can't easily assert on exact string length without knowing lipgloss internals,
	// but we can assert that Update processes the message without error.
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)
	
	// 2. Assert View doesn't panic and returns content
	view := m.View()
	assert.Assert(t, len(view) > 0)
	
	// 3. Resize to something narrow
	updatedM, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	m = updatedM.(tui.Model)
	viewNarrow := m.View()
	
	assert.Assert(t, len(viewNarrow) > 0)
	// Ideally we'd check if the rendered width matches 20, but ANSI codes make len() unreliable for display width.
	// We are satisfied here that the Resize message is handled.
}

func TestModel_EnterPrefix(t *testing.T) {
	client := mockGCSClient{
		buckets: []string{"b1"},
		objects: &gcs.ObjectList{
			Objects:  []string{"file1"},
			Prefixes: []string{"folder1/"},
		},
	}
	m := tui.NewModel([]string{"p1"}, client)
	updatedM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updatedM.(tui.Model)
	
	// 1. Enter b1
	updatedM, _ = m.Update(tui.BucketsMsg{Buckets: []string{"b1"}})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)
	updatedM, _ = m.Update(tui.ObjectsMsg{List: client.objects})
	m = updatedM.(tui.Model)

	// 2. Cursor should be on "folder1/" (first item)
	// View logic puts prefixes before objects
	if !strings.Contains(m.View(), "> folder1/") {
		t.Fatalf("Expected view to contain '> folder1/', but got:\n%q", m.View())
	}

	// 3. Press Enter on folder1/
	updatedM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedM.(tui.Model)

	// 4. Assert loading state and correct command
	assert.Assert(t, cmd != nil)
	assert.Assert(t, strings.Contains(m.View(), "Loading"))
}
