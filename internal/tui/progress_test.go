package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestProgress_Tracking(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, _ := setupTestModel(projects, objects, t.TempDir())

	// Simulate starting a download task
	taskID := "task-1"
	m, _ = updateModel(m, tui.DownloadProgressMsg{
		TaskID:  taskID,
		Current: 0,
		Total:   1000,
	})

	// Manually inject a task into activeTasks since startDownloadTaskDirectly is not exported
	// or we can just trigger a download action. Let's try to trigger it.
	m = enterBucket(m, projects, "b1", objects)
	// Press 'd' to download
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Find the task ID from activeTasks
	var actualTaskID string
	for id := range m.ActiveTasks() {
		actualTaskID = id
		break
	}
	assert.Assert(t, actualTaskID != "", "Task should be active")

	// Send progress update
	m, _ = updateModel(m, tui.DownloadProgressMsg{
		TaskID:  actualTaskID,
		Current: 500,
		Total:   1000,
	})

	task := m.ActiveTasks()[actualTaskID]
	assert.Equal(t, task.Progress, 50, "Progress should be 50%%")
	assert.Equal(t, task.Current, int64(500))
	assert.Equal(t, task.TotalBytes, int64(1000))
}

func TestProgress_VisibilityThreshold(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, _ := setupTestModel(projects, objects, t.TempDir())

	m = enterBucket(m, projects, "b1", objects)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	var taskID string
	for id := range m.ActiveTasks() {
		taskID = id
		break
	}

	// Send progress immediately - progress bar should NOT be in footer
	m, _ = updateModel(m, tui.DownloadProgressMsg{
		TaskID:  taskID,
		Current: 500,
		Total:   1000,
	})

	// We need a way to mock/control time or just check the view
	// For now, let's assume the task Started time is what matters.
	// Since it just started, it's < ProgressVisibilityThreshold.
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "[====>    ]"), "Progress bar should not be visible in footer within threshold")

	// Update task started time to exceed threshold
	tasks := m.ActiveTasks()
	task := tasks[taskID]
	task.Started = time.Now().Add(-(tui.ProgressVisibilityThreshold + time.Second))
	tasks[taskID] = task

	view = m.View()
	// Now it should be visible
	assert.Assert(t, strings.Contains(view, "50%"), "Progress percentage should be visible in footer after threshold")
}

func TestProgress_StaysVisibleAfterTimeout(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, _ := setupTestModel(projects, objects, t.TempDir())

	m = enterBucket(m, projects, "b1", objects)
	// Press 'd' to download
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Find the task ID and message ID
	var taskID string
	for id := range m.ActiveTasks() {
		taskID = id
		break
	}

	// We need to find the ClearStatusMsg from the batch of commands
	var clearMsg tui.ClearStatusMsg
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if msg := c(); msg != nil {
				if cm, ok := msg.(tui.ClearStatusMsg); ok {
					clearMsg = cm
				}
			}
		}
	}

	assert.Assert(t, clearMsg.ID != "", "Should have received a ClearStatusMsg")

	// Update task to exceed threshold so it's visible
	tasks := m.ActiveTasks()
	task := tasks[taskID]
	task.Started = time.Now().Add(-(tui.ProgressVisibilityThreshold + time.Second))
	task.Progress = 45
	task.TotalBytes = 100
	task.Current = 45
	tasks[taskID] = task

	// Initial check: progress bar should be visible
	assert.Assert(t, strings.Contains(m.View(), "45%"), "Progress should be visible initially")

	// Simulate the clear message arriving
	m, _ = updateModel(m, clearMsg)

	// ASSERTION: Progress should STILL be visible because the task is still active
	assert.Assert(t, strings.Contains(m.View(), "45%"), "Progress should STILL be visible in footer even after ClearStatusMsg if task is active")
}

func TestProgress_AggregateCalculation(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	m, _ := setupTestModel(projects, objects, t.TempDir())

	m = enterBucket(m, projects, "b1", objects)

	// Select both
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})

	// Download both
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	tasks := m.ActiveTasks()
	assert.Equal(t, len(tasks), 2)

	var ids []string
	for id := range tasks {
		ids = append(ids, id)
	}

	// Task 1: 200/1000 (20%)
	m, _ = updateModel(m, tui.DownloadProgressMsg{TaskID: ids[0], Current: 200, Total: 1000})
	// Task 2: 800/1000 (80%)
	m, _ = updateModel(m, tui.DownloadProgressMsg{TaskID: ids[1], Current: 800, Total: 1000})

	// Total: 1000/2000 (50%)
	// We need a way to get aggregate progress, maybe a method on Model
	// For now let's just check the view if we can force > threshold
	for id := range tasks {
		t := tasks[id]
		t.Started = time.Now().Add(-(tui.ProgressVisibilityThreshold + time.Second))
		tasks[id] = t
	}

	view := m.View()
	assert.Assert(t, strings.Contains(view, "50%"), "Aggregate progress should be 50%%")
}

func TestProgress_MessagesViewVisibility(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, _ := setupTestModel(projects, objects, t.TempDir())

	m = enterBucket(m, projects, "b1", objects)
	// Press 'd' to download
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// Find the task ID
	var taskID string
	for id := range m.ActiveTasks() {
		taskID = id
		break
	}

	// Update progress
	m, _ = updateModel(m, tui.DownloadProgressMsg{
		TaskID:  taskID,
		Current: 500,
		Total:   1000,
	})

	// Toggle messages view
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	view := stripAnsi(m.View())
	// Should contain the task name/file and the progress percentage
	assert.Assert(t, strings.Contains(view, "obj1"), "Messages view should contain filename")
	assert.Assert(t, strings.Contains(view, "50%"), "Messages view should contain progress percentage")
	assert.Assert(t, strings.Contains(view, "[====>     ]"), "Messages view should contain progress bar")
}
