package tui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idan-at/lazygcs/internal/gcs"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_AddMessage_Bounding(t *testing.T) {
	client := &mockGCSClient{}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Add 600 messages (assuming limit is 500)
	for i := 0; i < 600; i++ {
		_ = m.AddMessage(tui.LevelInfo, fmt.Sprintf("message %d", i))
	}

	// Verify that the number of messages is bounded at 500
	assert.Equal(t, len(m.Messages()), 500, "Messages queue should be bounded at 500, got %d", len(m.Messages()))
}

func TestModel_ErrorCount_Tracking(t *testing.T) {
	client := &mockGCSClient{}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	_ = m.AddMessage(tui.LevelInfo, "info 1")
	_ = m.AddMessage(tui.LevelError, "error 1")
	_ = m.AddMessage(tui.LevelWarn, "warn 1")
	_ = m.AddMessage(tui.LevelError, "error 2")

	// This method ErrorCount doesn't exist yet
	assert.Equal(t, m.ErrorCount(), 2, "ErrorCount should be 2")
}

func TestModel_ErrorCount_Bounding(t *testing.T) {
	client := &mockGCSClient{}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Add 500 errors
	for i := 0; i < 500; i++ {
		_ = m.AddMessage(tui.LevelError, fmt.Sprintf("error %d", i))
	}
	assert.Equal(t, m.ErrorCount(), 500, "ErrorCount should be 500")

	// Add 1 more error, it should still be 500 because one error was pushed out
	_ = m.AddMessage(tui.LevelError, "one more error")
	assert.Equal(t, m.ErrorCount(), 500, "ErrorCount should stay at 500 after overflow")

	// Add 1 info message, it should push out an error
	_ = m.AddMessage(tui.LevelInfo, "info message")
	assert.Equal(t, m.ErrorCount(), 499, "ErrorCount should decrease to 499 as error is pushed out by info")
}

func TestModel_ClearStatusMsg(t *testing.T) {
	client := &mockGCSClient{}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Add an initial message
	cmd1 := m.AddMessage(tui.LevelInfo, "first message")
	msg1 := m.Messages()[len(m.Messages())-1]

	// The status pill should be visible initially
	assert.Assert(t, !m.HideStatusPill(), "hideStatusPill should be false after adding a message")

	// Add a second message before the first clears
	cmd2 := m.AddMessage(tui.LevelInfo, "second message")
	msg2 := m.Messages()[len(m.Messages())-1]

	// Simulate the first command's timer firing
	tick1 := cmd1()
	clearMsg1, ok := tick1.(tui.ClearStatusMsg)
	assert.Assert(t, ok, "Expected ClearStatusMsg")
	assert.Equal(t, clearMsg1.ID, msg1.ID, "ClearStatusMsg should have the ID of the first message")

	m, _ = updateModel(m, clearMsg1)

	// The status pill should STILL be visible because a newer message was added
	assert.Assert(t, !m.HideStatusPill(), "hideStatusPill should remain false because a newer message exists")

	// Simulate the second command's timer firing
	tick2 := cmd2()
	clearMsg2, ok := tick2.(tui.ClearStatusMsg)
	assert.Assert(t, ok, "Expected ClearStatusMsg")
	assert.Equal(t, clearMsg2.ID, msg2.ID, "ClearStatusMsg should have the ID of the second message")

	m, _ = updateModel(m, clearMsg2)

	// The status pill should now be hidden
	assert.Assert(t, m.HideStatusPill(), "hideStatusPill should be true after the latest message clears")
}

func TestMessagesView_ToggleWithNoMessages(t *testing.T) {
	m, _ := setupTestModel(nil, nil, "/tmp")

	// Ensure there are no messages
	assert.Equal(t, len(m.Messages()), 0)
	assert.Equal(t, m.ShowMessages(), false)

	// Press 'm'
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	// Verify that showMessages is true (currently it will be false because of the bug)
	assert.Equal(t, m.ShowMessages(), true, "Messages view should be shown even if there are no messages")
}

func TestMessagesView_ClearsKittyImages(t *testing.T) {
	m, client := setupTestModel(
		[]gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		simpleObjectList([]string{"obj1"}, nil),
		"/tmp",
	)

	// Navigate to object to set preview content
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	// Simulate receiving a kitty image for the object preview
	m, _ = updateModel(m, tui.ContentMsg{
		ObjectName: "obj1",
		Content:    "\x1b_Ga=T,f=100;AAAA\x1b\\",
	})

	// Show messages
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	view := m.View()
	assert.Assert(t, strings.HasPrefix(view, "\x1b_Ga=d,d=A\x1b\\"), "View should clear kitty images when messages are shown")
}

func TestFooterView_HideHelpOnMessage(t *testing.T) {
	client := &mockGCSClient{}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)

	// Set width via WindowSizeMsg
	mModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = mModel.(tui.Model)

	// Initially, help should be visible in footerView (NORMAL state)
	view := m.View()
	assert.Assert(t, strings.Contains(view, "filter"), "Help hints should be visible initially")
	assert.Assert(t, strings.Contains(view, "select"), "Help hints should be visible initially")

	// Add a message
	_ = m.AddMessage(tui.LevelInfo, "test message")

	// Now help should be hidden
	view = m.View()
	assert.Assert(t, strings.Contains(view, "test message"), "Message should be visible")
	assert.Assert(t, !strings.Contains(view, "filter"), "Help hints should be hidden when a message is shown")
	assert.Assert(t, !strings.Contains(view, "select"), "Help hints should be hidden when a message is shown")
}

func TestFooterView_HideHelpOnDownloadConfirm(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	downloadDir := t.TempDir()
	m, client := setupTestModel(projects, objects, downloadDir)
	_ = client

	// 1. Navigate to object view
	m = enterBucket(m, projects, "b1", objects)

	// 2. Create a file with the same name on disk to trigger confirm
	err := os.WriteFile(filepath.Join(downloadDir, "obj1"), []byte("exists"), 0600)
	assert.NilError(t, err)

	// 3. Press 'd' to download, should trigger confirm
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// 4. Verify help is hidden
	view := m.View()
	assert.Assert(t, strings.Contains(view, "File exists"), "Should show confirmation message")
	assert.Assert(t, !strings.Contains(view, "filter"), "Help hints should be hidden in viewDownloadConfirm state")
	assert.Assert(t, !strings.Contains(view, "select"), "Help hints should be hidden in viewDownloadConfirm state")
}
