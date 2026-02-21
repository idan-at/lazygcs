package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// GCSClient defines the interface for fetching GCS data.
type GCSClient interface {
	ListBuckets(ctx context.Context, projectIDs []string) ([]string, error)
	ListObjects(ctx context.Context, bucketName, prefix string) (objects []string, prefixes []string, err error)
}

// BucketsMsg is the message sent when buckets have been fetched.
type BucketsMsg struct {
	Buckets []string
	Err     error
}

// Model represents the state of the TUI.
type Model struct {
	client     GCSClient
	projectIDs []string
	buckets    []string
	cursor     int
	loading    bool
	err        error
}

// NewModel initializes a model with a GCS client and project IDs.
func NewModel(projectIDs []string, client GCSClient) Model {
	return Model{
		projectIDs: projectIDs,
		client:     client,
		loading:    true,
	}
}

// Init triggers the bucket fetching command.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		buckets, err := m.client.ListBuckets(context.Background(), m.projectIDs)
		return BucketsMsg{Buckets: buckets, Err: err}
	}
}

// Update handles messages.
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

// View renders the TUI.
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
