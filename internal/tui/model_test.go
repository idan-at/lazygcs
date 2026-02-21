package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gotest.tools/v3/assert"
	"lazygcs/internal/tui"
)

type mockGCSClient struct {
	buckets []string
}

func (f mockGCSClient) ListBuckets(ctx context.Context, projectIDs []string) ([]string, error) {
	return f.buckets, nil
}

func (f mockGCSClient) ListObjects(ctx context.Context, bucketName, prefix string) ([]string, []string, error) {
	return nil, nil, nil
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
	client := mockGCSClient{buckets: []string{"b1", "b2"}}
	m := tui.NewModel([]string{"p1"}, client)
	updatedM, _ := m.Update(tui.BucketsMsg{Buckets: []string{"b1", "b2"}})
	m = updatedM.(tui.Model)

	// 1. Initial state: cursor at b1
	assert.Assert(t, strings.Contains(m.View(), "> b1"))

	// 2. Press 'j' (down)
	updatedM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updatedM.(tui.Model)

	// 3. Assert cursor moved to b2
	assert.Assert(t, strings.Contains(m.View(), "> b2"))
}
