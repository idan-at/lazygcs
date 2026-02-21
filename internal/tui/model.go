package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"lazygcs/internal/gcs"
)

// GCSClient defines the contract for interacting with Google Cloud Storage.
// This interface allows for easy mocking in TUI unit tests.
type GCSClient interface {
	// ListBuckets returns names of buckets in the specified projects.
	ListBuckets(ctx context.Context, projectIDs []string) ([]string, error)
	// ListObjects returns names of objects and common prefixes (folders) in a bucket.
	ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error)
}

// BucketsMsg is sent when bucket listing completes.
type BucketsMsg struct {
	Buckets []string
	Err     error
}

// Model maintains the state of the TUI application.
type Model struct {
	client     GCSClient
	projectIDs []string
	buckets    []string
	cursor     int
	loading    bool
	err        error
}

// NewModel creates a Model initialized with the provided projects and GCS client.
//
// Arguments:
//   - projectIDs: List of projects to scan for buckets initially.
//   - client: Implementation of the GCSClient interface.
func NewModel(projectIDs []string, client GCSClient) Model {
	return Model{
		projectIDs: projectIDs,
		client:     client,
		loading:    true,
	}
}

// Init initializes the application by triggering the first bucket fetch.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		buckets, err := m.client.ListBuckets(context.Background(), m.projectIDs)
		return BucketsMsg{Buckets: buckets, Err: err}
	}
}

// Update processes terminal messages (key presses, window resizes) and async responses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case BucketsMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.buckets = msg.Buckets
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.buckets)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the current state of the application as a string.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\n(press q to quit)", m.err)
	}
	if m.loading {
		return "Loading buckets...\n"
	}

	var s strings.Builder
	for i, bucket := range m.buckets {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %s\n", cursor, bucket))
	}
	return s.String()
}
