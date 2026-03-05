package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) fullPath() string {
	if m.currentBucket == "" {
		return "gs://"
	}
	return fmt.Sprintf("gs://%s/%s", m.currentBucket, m.currentPrefix)
}

func (m Model) previewView(width int) string {
	var s strings.Builder
	if m.state == viewObjects {
		s.WriteString(lipgloss.NewStyle().Bold(true).Render("Preview") + "\n\n")

		currentPrefixes, currentObjects, _ := m.filteredObjects()

		if m.cursor < len(currentPrefixes) {
			// Selected item is a prefix (folder)
			prefix := currentPrefixes[m.cursor]
			s.WriteString(fmt.Sprintf("Name: %s\n", truncate(prefix.Name, width-6)))
			s.WriteString("Type: Folder\n")
			if !prefix.Created.IsZero() {
				s.WriteString(fmt.Sprintf("Created: %s\n", prefix.Created.Format("2006-01-02 15:04:05")))
			}
			if !prefix.Updated.IsZero() {
				s.WriteString(fmt.Sprintf("Updated: %s\n", prefix.Updated.Format("2006-01-02 15:04:05")))
			}
			if prefix.Owner != "" {
				s.WriteString(fmt.Sprintf("Owner: %s\n", prefix.Owner))
			}
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]
				s.WriteString(fmt.Sprintf("Name: %s\n", truncate(obj.Name, width-6)))
				s.WriteString(fmt.Sprintf("Size: %d bytes\n", obj.Size))
				s.WriteString(fmt.Sprintf("Type: %s\n", obj.ContentType))
				if !obj.Created.IsZero() {
					s.WriteString(fmt.Sprintf("Created: %s\n", obj.Created.Format("2006-01-02 15:04:05")))
				}
				s.WriteString(fmt.Sprintf("Updated: %s\n", obj.Updated.Format("2006-01-02 15:04:05")))
				if obj.Owner != "" {
					s.WriteString(fmt.Sprintf("Owner: %s\n", obj.Owner))
				}
				if m.previewContent != "" {
					s.WriteString("\n---\n")

					if isBinary(m.previewContent) {
						s.WriteString("(binary content)")
					} else {
						// Leave room for the metadata lines and the "..." truncation indicator
						maxContentLines := m.maxItemsVisible() - 12
						if maxContentLines < 1 {
							maxContentLines = 1
						}

						lines := strings.Split(m.previewContent, "\n")
						if len(lines) > maxContentLines {
							s.WriteString(strings.Join(lines[:maxContentLines], "\n"))
							s.WriteString("\n...")
						} else {
							s.WriteString(m.previewContent)
						}
					}
				}
			}
		}
	}
	return s.String()
}

func (m Model) headerView() string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Padding(0, 1).
		Render(truncate(m.fullPath(), m.width-2))
}

func (m Model) footerView() string {
	var s strings.Builder
	if m.status != "" {
		s.WriteString("\n" + m.status)
	}

	if m.searchMode {
		s.WriteString(fmt.Sprintf("\nSearch: %s█\n\n(esc/enter: exit search)", m.searchQuery))
	} else if m.searchQuery != "" {
		s.WriteString(fmt.Sprintf("\nSearch: %s\n\n(/: search, q: quit, h: back, l/enter: select, d: download)", m.searchQuery))
	} else {
		s.WriteString("\n\n(/: search, q: quit, h: back, l/enter: select, d: download)")
	}

	return s.String()
}

func (m Model) maxItemsVisible() int {
	v := m.height - 10
	if v < 1 {
		v = 1
	}
	return v
}

func visibleRange(cursor, totalItems, maxVisible int) (start, end int) {
	if maxVisible <= 0 {
		return 0, 0
	}
	if totalItems <= maxVisible {
		return 0, totalItems
	}

	start = cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end = start + maxVisible
	if end > totalItems {
		end = totalItems
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > maxLen {
		if maxLen > 3 {
			return string(r[:maxLen-3]) + "..."
		}
		return string(r[:maxLen])
	}
	return s
}

func (m Model) objectsView(width int) string {
	var s strings.Builder
	if m.state == viewObjects {
		title := fmt.Sprintf("Objects in %s", m.currentBucket)
		s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate(title, width)) + "\n\n")
		if m.loading {
			s.WriteString("Loading...")
		} else {
			currentPrefixes, currentObjects, _ := m.filteredObjects()
			totalItems := len(currentPrefixes) + len(currentObjects)

			start, end := visibleRange(m.cursor, totalItems, m.maxItemsVisible())

			for i := start; i < end; i++ {
				cursor := " "
				if m.cursor == i {
					cursor = ">"
				}

				var displayItem string
				if i < len(currentPrefixes) {
					displayItem = currentPrefixes[i].Name
				} else {
					displayItem = currentObjects[i-len(currentPrefixes)].Name
				}

				// Display relative path
				displayItem = strings.TrimPrefix(displayItem, m.currentPrefix)
				// Truncate to fit column (account for cursor and padding)
				displayItem = truncate(displayItem, width-2)
				s.WriteString(fmt.Sprintf("%s %s\n", cursor, displayItem))
			}
			if totalItems == 0 {
				s.WriteString("(empty)")
			}
		}
	}
	return s.String()
}

func (m Model) bucketsView(width int) string {
	var s strings.Builder
	s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate("Buckets", width)) + "\n\n")
	if m.state == viewBuckets && m.loading {
		s.WriteString("Loading...")
	} else {
		filtered := m.filteredBuckets()
		start, end := visibleRange(m.cursor, len(filtered), m.maxItemsVisible())
		for i := start; i < end; i++ {
			bucket := filtered[i]
			cursor := " "
			if m.state == viewBuckets && m.cursor == i {
				cursor = ">"
			}
			// Truncate to fit column (account for cursor and padding)
			truncatedBucket := truncate(bucket, width-2)
			s.WriteString(fmt.Sprintf("%s %s\n", cursor, truncatedBucket))
		}
	}
	return s.String()
}

// View renders the current state of the application as a string.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\n(press q to quit)", m.err)
	}

	// Calculate column widths
	// 30% | 35% | 35%
	leftWidth := int(float64(m.width) * 0.3)
	midWidth := int(float64(m.width) * 0.35)
	rightWidth := m.width - leftWidth - midWidth - 6 // account for borders/padding

	leftCol := lipgloss.NewStyle().Width(leftWidth).PaddingRight(2).Render(m.bucketsView(leftWidth))
	midCol := lipgloss.NewStyle().Width(midWidth).PaddingRight(2).Render(m.objectsView(midWidth))
	rightCol := lipgloss.NewStyle().Width(rightWidth).Render(m.previewView(rightWidth))

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, midCol, rightCol)

	return m.headerView() + "\n\n" + mainContent + m.footerView()
}
