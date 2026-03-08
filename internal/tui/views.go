package tui

import (
	"fmt"
	"path/filepath"
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

		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))           // Dimmed text
		valStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // Bright white

		if m.cursor < len(currentPrefixes) {
			// Selected item is a prefix (folder)
			prefix := currentPrefixes[m.cursor]

			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(prefix.Name, width-6)))
			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render("Folder"))

			if !prefix.Created.IsZero() {
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(prefix.Created.Format("2006-01-02 15:04:05")))
			}
			if !prefix.Updated.IsZero() {
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(prefix.Updated.Format("2006-01-02 15:04:05")))
			}
			if prefix.Owner != "" {
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(prefix.Owner))
			}
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(obj.Name, width-6)))
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Size:"), valStyle.Render(humanizeSize(obj.Size)))

				contentType := obj.ContentType
				if contentType == "" {
					contentType = "unknown"
				}
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render(contentType))

				if !obj.Created.IsZero() {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(obj.Created.Format("2006-01-02 15:04:05")))
				}
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(obj.Updated.Format("2006-01-02 15:04:05")))

				if obj.Owner != "" {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(obj.Owner))
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
	s.WriteString("\n") // Spacer

	// Left side: Status Pill
	statusText := " NORMAL "
	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("250"))

	if m.searchMode {
		statusText = fmt.Sprintf(" SEARCH: %s█ ", m.searchQuery)
		statusStyle = statusStyle.Background(lipgloss.Color("69")).Foreground(lipgloss.Color("15"))
	} else if m.searchQuery != "" {
		statusText = fmt.Sprintf(" FILTER: %s ", m.searchQuery)
		statusStyle = statusStyle.Background(lipgloss.Color("61")).Foreground(lipgloss.Color("15"))
	} else if m.status != "" {
		statusText = fmt.Sprintf(" %s ", m.status)
		statusStyle = statusStyle.Background(lipgloss.Color("130")).Foreground(lipgloss.Color("15"))
	}

	pill := statusStyle.Render(statusText)

	// Right side: Help
	m.help.ShowAll = false
	helpView := m.help.View(keys)

	// Combine
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, pill, "  ", helpView))

	return s.String()
}

func (m Model) maxItemsVisible() int {
	v := m.height - 10
	if m.showHelp {
		v -= 6 // Make columns shorter when help is shown at the bottom
	}
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

func getIcon(name string, isFolder bool, isBucket bool) string {
	if isBucket {
		return "🪣 " // Bucket icon
	}
	if isFolder {
		return " " // Folder icon
	}

	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return "󰟓 "
	case ".md":
		return " "
	case ".json":
		return " "
	case ".txt":
		return " "
	case ".csv":
		return "󰈙 "
	case ".yaml", ".yml", ".toml":
		return " "
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return " "
	case ".pdf":
		return " "
	case ".zip", ".tar", ".gz", ".tgz":
		return " "
	case ".sh", ".bash", ".zsh":
		return " "
	case ".py":
		return " "
	case ".js", ".ts", ".jsx", ".tsx":
		return " "
	case ".html", ".htm":
		return " "
	case ".css":
		return " "
	default:
		return " " // Default file icon
	}
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
				var isFolder bool
				if i < len(currentPrefixes) {
					originalName = currentPrefixes[i].Name
					isFolder = true
				} else {
					originalName = currentObjects[i-len(currentPrefixes)].Name
					isFolder = false
				}

				// Check if selected
				var isSelected bool
				if m.selected != nil {
					_, isSelected = m.selected[originalName]
				}

				selectionIndicator := " "
				if isSelected {
					selectionIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("✓")
				}

				displayItem := originalName
				// Display relative path
				displayItem = strings.TrimPrefix(displayItem, m.currentPrefix)

				icon := ""
				if m.showIcons {
					icon = getIcon(displayItem, isFolder, false)
				}

				cursorIndicator := " "
				if m.cursor == i {
					cursorIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true).Render("▶")
				}

				textStyle := lipgloss.NewStyle()
				if m.cursor == i {
					textStyle = textStyle.Foreground(lipgloss.Color("69")).Bold(true)
				} else if isSelected {
					textStyle = textStyle.Foreground(lipgloss.Color("212")).Bold(true)
				} else {
					textStyle = textStyle.Foreground(lipgloss.Color("250"))
				}

				// Truncate to fit column (account for cursor, selection indicator, optional icon, and padding)
				truncateLen := width - 4
				if m.showIcons {
					truncateLen -= 2 // Icon + space
				}
				truncatedItem := truncate(displayItem, truncateLen)
				content := fmt.Sprintf("%s%s %s%s", cursorIndicator, selectionIndicator, icon, textStyle.Render(truncatedItem))

				s.WriteString(content + "\n")
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
			for i, item := range filtered {
				if !item.IsProject && item.BucketName == m.currentBucket {
					activeIdx = i
					break
				}
			}
		}

		start, end := visibleRange(activeIdx, len(filtered), m.maxItemsVisible())
		for i := start; i < end; i++ {
			item := filtered[i]

			cursorIndicator := " "
			if m.state == viewBuckets && m.cursor == i {
				cursorIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true).Render("▶")
			}

			indicator := " "
			if m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket {
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render("●")
			}

			textStyle := lipgloss.NewStyle()
			if m.state == viewBuckets && m.cursor == i {
				textStyle = textStyle.Foreground(lipgloss.Color("69")).Bold(true)
			} else if m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket {
				textStyle = textStyle.Foreground(lipgloss.Color("69"))
			} else {
				textStyle = textStyle.Foreground(lipgloss.Color("250"))
			}

			if item.IsProject {
				// Project Header
				icon := "▼ "
				if _, collapsed := m.collapsedProjects[item.ProjectID]; collapsed {
					icon = "▶ "
				}

				// Make project titles bold and a different color
				projectStyle := textStyle
				if m.state == viewBuckets && m.cursor != i {
					projectStyle = projectStyle.Foreground(lipgloss.Color("246")).Bold(true)
				} else {
					projectStyle = projectStyle.Bold(true)
				}

				truncateLen := width - 4
				truncatedProject := truncate(item.ProjectID, truncateLen)
				content := fmt.Sprintf("%s%s%s", cursorIndicator, icon, projectStyle.Render(truncatedProject))
				s.WriteString(content + "\n")
			} else {
				// Bucket Item
				icon := ""
				if m.showIcons {
					icon = getIcon(item.BucketName, false, true)
				}

				// Truncate to fit column, account for indentation
				truncateLen := width - 6
				if m.showIcons {
					truncateLen -= 2
				}
				truncatedBucket := truncate(item.BucketName, truncateLen)
				content := fmt.Sprintf("%s%s  %s%s", cursorIndicator, indicator, icon, textStyle.Render(truncatedBucket))

				s.WriteString(content + "\n")
			}
		}
	}
	return s.String()
}

// View renders the current state of the application as a string.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\n(press q to quit)", m.err)
	}

	// Styles
	columnHeight := m.maxItemsVisible() + 2 // Title + blank line + list items

	activeStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("69")). // Google Blue-ish
		Padding(0, 1).
		Height(columnHeight)

	inactiveStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")). // Dimmed gray
		Foreground(lipgloss.Color("244")).       // Dimmed text
		Padding(0, 1).
		Height(columnHeight)

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

	view := m.headerView() + "\n\n" + mainContent

	if m.showHelp {
		view += "\n" + m.helpView()
	} else {
		view += m.footerView()
	}

	return view
}

func (m Model) helpView() string {
	m.help.ShowAll = true
	helpText := m.help.View(keys)

	helpStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true, false, false, false).
		BorderForeground(lipgloss.Color("69")).
		Padding(1, 1).
		Width(m.width)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Render("HELP")

	content := lipgloss.JoinVertical(lipgloss.Left, titleStyle, helpText)
	return helpStyle.Render(content)
}
