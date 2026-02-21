package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Model represents the state of the TUI.
type Model struct {
	buckets []string
	cursor  int
}

// InitialModel creates a new Model with the provided buckets.
func InitialModel(buckets []string) Model {
	return Model{
		buckets: buckets,
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and returns an updated model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
