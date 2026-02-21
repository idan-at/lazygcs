package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// ObjectsMsg is sent when object listing completes.
type ObjectsMsg struct {
	List *gcs.ObjectList
	Err  error
}

type viewState int

const (
	viewBuckets viewState = iota
	viewObjects
)

// Model maintains the state of the TUI application.
type Model struct {
	client     GCSClient
	projectIDs []string

	// View State
	width  int
	height int
	state  viewState

	// Buckets View
	buckets []string
	cursor  int // used for buckets or objects depending on state

	// Objects View
	currentBucket string
	objects       []string
	prefixes      []string

	loading bool
	err     error
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
		state:      viewBuckets,
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

func (m Model) fetchObjects() tea.Cmd {
	return func() tea.Msg {
		list, err := m.client.ListObjects(context.Background(), m.currentBucket, "")
		return ObjectsMsg{List: list, Err: err}
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

	case ObjectsMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.objects = msg.List.Objects
		m.prefixes = msg.List.Prefixes
		m.cursor = 0
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			itemsCount := len(m.buckets)
			if m.state == viewObjects {
				itemsCount = len(m.objects) + len(m.prefixes)
			}
			if itemsCount > 0 {
				m.cursor = (m.cursor + 1) % itemsCount
			}
		case "k", "up":
			itemsCount := len(m.buckets)
			if m.state == viewObjects {
				itemsCount = len(m.objects) + len(m.prefixes)
			}
			if itemsCount > 0 {
				m.cursor = (m.cursor - 1 + itemsCount) % itemsCount
			}
		case "l", "enter":
			if m.state == viewBuckets && len(m.buckets) > 0 {
				m.currentBucket = m.buckets[m.cursor]
				m.state = viewObjects
				m.loading = true
				return m, m.fetchObjects()
			}
		case "h":
			if m.state == viewObjects {
				m.state = viewBuckets
				m.cursor = 0 // for now, reset cursor when going back
				return m, nil
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

	var leftCol, rightCol string

	// Calculate column widths
	leftWidth := int(float64(m.width) * 0.3)
	rightWidth := m.width - leftWidth - 4 // account for padding/border

	// Left Column: Buckets
	var lb strings.Builder
	lb.WriteString(lipgloss.NewStyle().Bold(true).Render("Buckets") + "\n\n")
	if m.state == viewBuckets && m.loading {
		lb.WriteString("Loading...")
	} else {
		for i, bucket := range m.buckets {
			cursor := " "
			if m.state == viewBuckets && m.cursor == i {
				cursor = ">"
			}
			lb.WriteString(fmt.Sprintf("%s %s\n", cursor, bucket))
		}
	}
	leftCol = lb.String()

	// Right Column: Objects
	var rb strings.Builder
	if m.state == viewObjects {
		rb.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Objects in %s", m.currentBucket)) + "\n\n")
		if m.loading {
			rb.WriteString("Loading...")
		} else {
			allItems := append(m.prefixes, m.objects...)
			for i, item := range allItems {
				cursor := " "
				if m.cursor == i {
					cursor = ">"
				}
				rb.WriteString(fmt.Sprintf("%s %s\n", cursor, item))
			}
			if len(allItems) == 0 {
				rb.WriteString("(empty)")
			}
		}
	}
	rightCol = rb.String()

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).PaddingRight(2).Render(leftCol),
		lipgloss.NewStyle().Width(rightWidth).Render(rightCol),
	) + "\n\n(q: quit, h: back, l/enter: select)"
}
