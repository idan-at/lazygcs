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
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		s.WriteString(lipgloss.NewStyle().Bold(true).Render("Preview") + "\n\n")

		currentPrefixes, currentObjects, _ := m.filteredObjects()

		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")) // Dimmed text
		valStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // Bright white

		if m.cursor < len(currentPrefixes) {
			// Selected item is a prefix (folder)
			prefix := currentPrefixes[m.cursor]
			
			s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(prefix.Name, width-6))))
			s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Type:"), valStyle.Render("Folder")))
			
			if !prefix.Created.IsZero() {
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Created:"), valStyle.Render(prefix.Created.Format("2006-01-02 15:04:05"))))
			}
			if !prefix.Updated.IsZero() {
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(prefix.Updated.Format("2006-01-02 15:04:05"))))
			}
			if prefix.Owner != "" {
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(prefix.Owner)))
			}
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(obj.Name, width-6))))
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Size:"), valStyle.Render(humanizeSize(obj.Size))))
				
				contentType := obj.ContentType
				if contentType == "" {
					contentType = "unknown"
				}
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Type:"), valStyle.Render(contentType)))
				
				if !obj.Created.IsZero() {
					s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Created:"), valStyle.Render(obj.Created.Format("2006-01-02 15:04:05"))))
				}
				s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(obj.Updated.Format("2006-01-02 15:04:05"))))
				
				if obj.Owner != "" {
					s.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(obj.Owner)))
				}
				
				if m.previewContent != "" {
					separator := lipgloss.NewStyle().
						Foreground(lipgloss.Color("240")).
						Render(strings.Repeat("─", width))
					
					s.WriteString("\n" + separator + "\n")

					if isBinary(m.previewContent) {
						s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true).Render("(binary content)"))
					} else {
						// Leave room for the metadata lines and the "..." truncation indicator
						maxContentLines := m.maxItemsVisible() - 14 // adjusted for the new border
						if maxContentLines < 1 {
							maxContentLines = 1
						}

						lines := strings.Split(m.previewContent, "\n")
						if len(lines) > maxContentLines {
							s.WriteString(strings.Join(lines[:maxContentLines], "\n"))
							s.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("..."))
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

func humanizeSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (m Model) objectsView(width int) string {
	var s strings.Builder
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		title := fmt.Sprintf("Objects in %s", m.currentBucket)
		s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate(title, width)) + "\n\n")
		if m.loading {
			s.WriteString("Loading...")
		} else {
			currentPrefixes, currentObjects, _ := m.filteredObjects()
			totalItems := len(currentPrefixes) + len(currentObjects)

			start, end := visibleRange(m.cursor, totalItems, m.maxItemsVisible())

			for i := start; i < end; i++ {
				var originalName string
				if i < len(currentPrefixes) {
					originalName = currentPrefixes[i].Name
				} else {
					originalName = currentObjects[i-len(currentPrefixes)].Name
				}

				// Check if selected
				var isSelected bool
				if m.selected != nil {
					_, isSelected = m.selected[originalName]
				}

				selectionIndicator := " "
				if isSelected {
					selectionIndicator = "✓"
				}

				displayItem := originalName
				// Display relative path
				displayItem = strings.TrimPrefix(displayItem, m.currentPrefix)
				
				// Styles
				rowStyle := lipgloss.NewStyle().Width(width)
				if m.cursor == i {
					rowStyle = rowStyle.Background(lipgloss.Color("69")).Foreground(lipgloss.Color("15"))
				}
				
				textStyle := lipgloss.NewStyle()
				if isSelected {
					textStyle = textStyle.Foreground(lipgloss.Color("212")).Bold(true)
				} else if m.cursor != i {
					// Dim unselected items if not under cursor
					textStyle = textStyle.Foreground(lipgloss.Color("250"))
				}

				// Truncate to fit column (account for selection indicator and padding)
				truncatedItem := truncate(displayItem, width-4)
				content := fmt.Sprintf("%s %s", selectionIndicator, textStyle.Render(truncatedItem))
				
				s.WriteString(rowStyle.Render(content) + "\n")
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
		
		// Determine the active index for the buckets list
		activeIdx := m.cursor
		if m.state != viewBuckets {
			// Find the index of the current bucket to keep it in view
			activeIdx = 0
			for i, b := range filtered {
				if b == m.currentBucket {
					activeIdx = i
					break
				}
			}
		}

		start, end := visibleRange(activeIdx, len(filtered), m.maxItemsVisible())
		for i := start; i < end; i++ {
			bucket := filtered[i]
			
			rowStyle := lipgloss.NewStyle().Width(width)
			if m.state == viewBuckets && m.cursor == i {
				rowStyle = rowStyle.Background(lipgloss.Color("69")).Foreground(lipgloss.Color("15"))
			}

			indicator := " "
			if m.state != viewBuckets && bucket == m.currentBucket {
				indicator = "*" 
			}

			// Truncate to fit column
			truncatedBucket := truncate(bucket, width-4)
			content := fmt.Sprintf("%s %s", indicator, truncatedBucket)
			
			s.WriteString(rowStyle.Render(content) + "\n")
		}
	}
	return s.String()
}

// View renders the current state of the application as a string.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\n(press q to quit)", m.err)
	}

	if m.showHelp {
		return m.helpView()
	}

	// Styles
	activeStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("69")). // Google Blue-ish
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")). // Dimmed gray
		Foreground(lipgloss.Color("244")).      // Dimmed text
		Padding(0, 1)

	// Calculate column widths
	// 30% | 35% | 35%
	totalWidth := m.width
	leftWidth := int(float64(totalWidth) * 0.3)
	midWidth := int(float64(totalWidth) * 0.35)
	rightWidth := totalWidth - leftWidth - midWidth

	// Apply styles and render columns
	leftStyle := inactiveStyle
	if m.state == viewBuckets {
		leftStyle = activeStyle
	}
	leftCol := leftStyle.Width(leftWidth - 4).Render(m.bucketsView(leftWidth - 4))

	midStyle := inactiveStyle
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		midStyle = activeStyle
	}
	midCol := midStyle.Width(midWidth - 4).Render(m.objectsView(midWidth - 4))

	// Preview column is always "inactive" in terms of focus
	rightCol := inactiveStyle.Width(rightWidth - 4).Render(m.previewView(rightWidth - 4))

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, midCol, rightCol)

	return m.headerView() + "\n\n" + mainContent + m.footerView()
}

func (m Model) helpView() string {
	helpText := `
	Help Menu
	---------

	Navigation:
	j / ↓       Move cursor down
	k / ↑       Move cursor up
	l / Enter / → Enter a bucket or directory
	h / ←       Go back to the parent directory or bucket list

	Actions:
	space       Toggle selection of the highlighted item (Multi-select)
	d           Download the currently highlighted item (or all selected items)
	/           Start searching/filtering the current column
	esc / Enter Exit search mode
	?           Toggle Help (this menu)
	q / Ctrl+c  Quit the application
	`

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(helpText)
}
