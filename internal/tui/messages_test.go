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
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	// Add 600 messages (assuming limit is 500)
	for i := 0; i < 600; i++ {
		_ = m.AddMessage(tui.LevelInfo, fmt.Sprintf("message %d", i), 0, "")
	}

	// Verify that the number of messages is bounded at 500
	assert.Equal(t, len(m.Messages()), 500, "Messages queue should be bounded at 500, got %d", len(m.Messages()))
}

func TestModel_ErrorCount_Tracking(t *testing.T) {
	client := &mockGCSClient{}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	_ = m.AddMessage(tui.LevelError, "error 1", 0, "")
	_ = m.AddMessage(tui.LevelError, "error 2", 0, "")
	assert.Equal(t, m.ErrorCount(), 2, "ErrorCount should be 2")

	_ = m.AddMessage(tui.LevelInfo, "info 1", 0, "")
	assert.Equal(t, m.ErrorCount(), 0, "ErrorCount should reset to 0 after an info message")

	_ = m.AddMessage(tui.LevelError, "error 3", 0, "")
	assert.Equal(t, m.ErrorCount(), 1, "ErrorCount should be 1 after new error")
}

func TestModel_ErrorCount_Bounding(t *testing.T) {
	client := &mockGCSClient{}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	// Add 500 errors
	for i := 0; i < 500; i++ {
		_ = m.AddMessage(tui.LevelError, fmt.Sprintf("error %d", i), 0, "")
	}
	assert.Equal(t, m.ErrorCount(), 500, "ErrorCount should be 500")

	// Add 1 more error, it should still be 500 because one error was pushed out
	_ = m.AddMessage(tui.LevelError, "one more error", 0, "")
	assert.Equal(t, m.ErrorCount(), 500, "ErrorCount should stay at 500 after overflow")

	// Add 1 info message, it should reset to 0
	_ = m.AddMessage(tui.LevelInfo, "info message", 0, "")
	assert.Equal(t, m.ErrorCount(), 0, "ErrorCount should reset to 0 on success")
}

func TestModel_ClearStatusMsg(t *testing.T) {
	client := &mockGCSClient{}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	// Add an initial message
	cmd1 := m.AddMessage(tui.LevelInfo, "first message", 0, "")
	msg1 := m.Messages()[len(m.Messages())-1]

	// The status pill should be visible initially
	assert.Assert(t, !m.HideStatusPill(), "hideStatusPill should be false after adding a message")

	// Add a second message before the first clears
	cmd2 := m.AddMessage(tui.LevelInfo, "second message", 0, "")
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
	assert.Equal(t, len(m.Messages()), 0, "")
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
	// CLEAR code: \x1b_Ga=d,d=A\x1b\\
	assert.Assert(t, strings.HasPrefix(view, "\x1b_Ga=d,d=A\x1b\\"), "View should start with CLEAR code when messages are shown")

	// Verify that DRAW code is absent in the rest of the view
	contentAfterClear := view[len("\x1b_Ga=d,d=A\x1b\\"):]
	assert.Assert(t, !strings.Contains(contentAfterClear, "\x1b_Ga=T"), "View should NOT contain DRAW code after the initial CLEAR code")
	assert.Assert(t, strings.Contains(view, "(image preview hidden)"), "Placeholder text should be present when messages are visible")
}

func TestHelpView_ClearsKittyImages(t *testing.T) {
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

	// Show help
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})

	view := m.View()
	// CLEAR code: \x1b_Ga=d,d=A\x1b\\
	assert.Assert(t, strings.HasPrefix(view, "\x1b_Ga=d,d=A\x1b\\"), "View should start with CLEAR code when help is shown")

	// Verify that DRAW code is absent in the rest of the view
	contentAfterClear := view[len("\x1b_Ga=d,d=A\x1b\\"):]
	assert.Assert(t, !strings.Contains(contentAfterClear, "\x1b_Ga=T"), "View should NOT contain DRAW code after the initial CLEAR code")
	assert.Assert(t, strings.Contains(view, "(image preview hidden)"), "Placeholder text should be present when help is visible")
}

func TestFooterView_HideHelpOnMessage(t *testing.T) {
	client := &mockGCSClient{}
	mModel := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m := &mModel

	// Set width via WindowSizeMsg
	res, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = res.(*tui.Model)

	// Initially, help should be visible in footerView (NORMAL state)
	view := m.View()
	assert.Assert(t, strings.Contains(view, "filter"), "Help hints should be visible initially")
	assert.Assert(t, strings.Contains(view, "select"), "Help hints should be visible initially")

	// Add a message
	_ = m.AddMessage(tui.LevelInfo, "test message", 0, "")

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
