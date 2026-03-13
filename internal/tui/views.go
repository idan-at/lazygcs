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

		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))           // Dimmed text
		valStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // Bright white

		if m.cursor < len(currentPrefixes) {
			// Selected item is a prefix (folder)
			prefix := currentPrefixes[m.cursor]

			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(prefix.Name, width-10)))
			if !prefix.Fetched {
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render("Folder"))
				fmt.Fprintf(&s, "\n%s Loading metadata...\n", m.spinner.View())
			} else {
				folderType := "Folder"
				if prefix.Err != nil {
					folderType = "Virtual Directory"
				}
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render(folderType))

				if !prefix.Created.IsZero() {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(prefix.Created.Format("2006-01-02 15:04:05")))
				}
				if !prefix.Updated.IsZero() {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(prefix.Updated.Format("2006-01-02 15:04:05")))
				}
				if prefix.Owner != "" {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(truncate(prefix.Owner, width-10)))
				}
				if prefix.Err != nil && !strings.Contains(prefix.Err.Error(), "object doesn't exist") && !strings.Contains(prefix.Err.Error(), "not found") {
					fmt.Fprintf(&s, "\n%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Render(truncate(fmt.Sprintf("Metadata Error: %v", prefix.Err), width-2)))
				}
			}
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(obj.Name, width-10)))
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Size:"), valStyle.Render(humanizeSize(obj.Size)))

				contentType := obj.ContentType
				if contentType == "" {
					contentType = "unknown"
				}
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render(truncate(contentType, width-10)))

				if !obj.Created.IsZero() {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(obj.Created.Format("2006-01-02 15:04:05")))
				}
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(obj.Updated.Format("2006-01-02 15:04:05")))

				if obj.Owner != "" {
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(truncate(obj.Owner, width-10)))
				}

				if m.previewContent != "" {
					separator := lipgloss.NewStyle().
						Border(lipgloss.NormalBorder(), true, false, false, false).
						BorderForeground(lipgloss.Color("240")).
						Width(width).
						MarginTop(1).
						Render("")

					s.WriteString(separator)
					s.WriteString("\n")

					if m.previewContent == "Loading..." {
						fmt.Fprintf(&s, "\n%s Loading preview...\n", m.spinner.View())
					} else if isBinary(m.previewContent) {
						s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true).Render("(binary content)"))
					} else {
						// Calculate how many lines we can actually show
						// We want the ENTIRE previewView content to fit in m.maxItemsVisible() + 2
						// Preview Title (2) + Metadata (~6) + Separator (2) = ~10 lines
						maxLines := m.maxItemsVisible() - 8
						if maxLines < 1 { maxLines = 1 }

						allLines := strings.Split(m.previewContent, "\n")
						displayLines := allLines
						if len(displayLines) > maxLines {
							displayLines = displayLines[:maxLines]
						}

						for i, line := range displayLines {
							s.WriteString(truncate(line, width-2))
							if i < len(displayLines)-1 {
								s.WriteString("\n")
							}
						}

						if len(allLines) > maxLines {
							s.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("..."))
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
	// Left side: Status Pill
	statusText := " NORMAL "
	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
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
	} else if m.bgJobs > len(m.loadingProjects) {
		statusText = m.spinner.View()
		statusStyle = lipgloss.NewStyle().Padding(0, 1)
	}

	pill := statusStyle.Render(statusText)

	var errorsPill string
	if len(m.errorsList) > 0 {
		errorsPill = " " + lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("196")). // Red background
			Foreground(lipgloss.Color("15")).
			Render(fmt.Sprintf("%d ERRORS", len(m.errorsList)))
	}

	// Right side: Help hints
	m.help.ShowAll = false
	m.help.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	m.help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpView := m.help.View(keys)

	// Build the ribbon
	rightPadding := 1
	gapWidth := m.width - lipgloss.Width(pill) - lipgloss.Width(errorsPill) - lipgloss.Width(helpView) - rightPadding
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := strings.Repeat(" ", gapWidth)

	return "\n" + pill + errorsPill + gap + helpView + strings.Repeat(" ", rightPadding)
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

func (m Model) objectsView(width int) string {
	var s strings.Builder
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		title := fmt.Sprintf("Objects in %s", m.currentBucket)
		s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate(title, width)) + "\n\n")

		currentPrefixes, currentObjects, _ := m.filteredObjects()
		totalItems := len(currentPrefixes) + len(currentObjects)

		if m.loading && totalItems == 0 {
			fmt.Fprintf(&s, "%s Loading...", m.spinner.View())
		} else {
			maxVisible := m.maxItemsVisible()
			if m.loading {
				maxVisible--
				if maxVisible < 1 {
					maxVisible = 1
				}
			}

			start, end := visibleRange(m.cursor, totalItems, maxVisible)

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

				textStyle := lipgloss.NewStyle()
				isFocused := m.cursor == i
				if isFocused {
					textStyle = textStyle.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15")).Bold(true)
				} else if isSelected {
					textStyle = textStyle.Foreground(lipgloss.Color("212")).Bold(true)
				} else {
					textStyle = textStyle.Foreground(lipgloss.Color("250"))
				}

				// Truncate to fit column (account for selection indicator, optional icon, and padding)
				truncateLen := width - 3
				if m.showIcons {
					truncateLen -= 2 // Icon + space
				}
				truncatedItem := truncate(displayItem, truncateLen)
				
				itemContent := fmt.Sprintf("%s %s%s", selectionIndicator, icon, truncatedItem)
				content := textStyle.Width(width).Render(itemContent)

				s.WriteString(content + "\n")
			}
			if totalItems == 0 {
				s.WriteString("(empty)")
			} else if m.loading {
				fmt.Fprintf(&s, "%s Loading...", m.spinner.View())
			}
		}
	}
	return s.String()
}

func (m Model) bucketsView(width int) string {
	var s strings.Builder
	s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate("Buckets", width)) + "\n\n")
	
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

		indicator := " "
		if m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket {
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render("●")
		}

		textStyle := lipgloss.NewStyle()
		isFocused := m.state == viewBuckets && m.cursor == i
		isActiveBucket := m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket

		if isFocused {
			textStyle = textStyle.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15")).Bold(true)
		} else if isActiveBucket {
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
			if !isFocused {
				projectStyle = projectStyle.Foreground(lipgloss.Color("246")).Bold(true)
			}

			truncateLen := width - 3
			if m.loadingProjects[item.ProjectID] {
				truncateLen -= 2 // space + spinner
			}
			truncatedProject := truncate(item.ProjectID, truncateLen)
			
			itemContent := fmt.Sprintf("%s%s", icon, truncatedProject)
			if m.loadingProjects[item.ProjectID] {
				itemContent += " " + m.spinner.View()
			}
			content := projectStyle.Width(width).Render(itemContent)
			s.WriteString(content + "\n")
		} else {
			// Bucket Item
			icon := ""
			if m.showIcons {
				icon = getIcon(item.BucketName, false, true)
			}

			// Truncate to fit column, account for indentation
			truncateLen := width - 5
			if m.showIcons {
				truncateLen -= 2
			}
			truncatedBucket := truncate(item.BucketName, truncateLen)
			
			itemContent := fmt.Sprintf("%s  %s%s", indicator, icon, truncatedBucket)
			content := textStyle.Width(width).Render(itemContent)
			s.WriteString(content + "\n")
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
	columnHeight := m.maxItemsVisible() + 4 // Title + blank line + list items + borders

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
	// 25% | 30% | 45%
	totalWidth := m.width
	leftWidth := int(float64(totalWidth) * 0.25)
	midWidth := int(float64(totalWidth) * 0.30)
	rightWidth := totalWidth - leftWidth - midWidth

	// Apply styles and render columns
	leftStyle := inactiveStyle
	if m.state == viewBuckets {
		leftStyle = activeStyle
	}
	// Subtract 2 from width to account for borders
	leftCol := leftStyle.Width(leftWidth - 2).Render(m.bucketsView(leftWidth - 4))

	midStyle := inactiveStyle
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		midStyle = activeStyle
	}
	midCol := midStyle.Width(midWidth - 2).Render(m.objectsView(midWidth - 4))

	// Preview column is always "inactive" in terms of focus
	rightCol := inactiveStyle.Width(rightWidth - 2).Render(m.previewView(rightWidth - 4))

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, midCol, rightCol)

	view := m.headerView() + "\n\n" + mainContent

	if m.showHelp {
		view += "\n" + m.helpView()
	} else {
		view += m.footerView()
	}

	if m.showErrors {
		// Use lipgloss.Place to center the errors modal.
		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			m.errorsView(),
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("236")),
		)
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

func (m Model) errorsView() string {
	var s strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("204")).
		Render(fmt.Sprintf("ERRORS (%d)", len(m.errorsList)))
	
	s.WriteString(title + "\n\n")

	// Limit to last 10 errors to avoid huge modals
	start := 0
	if len(m.errorsList) > 10 {
		start = len(m.errorsList) - 10
	}

	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	for i := start; i < len(m.errorsList); i++ {
		s.WriteString("• " + errStyle.Render(m.errorsList[i].Error()) + "\n")
	}

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(1).
		Render("Press esc or q to close")
	
	s.WriteString(footer)

	boxWidth := m.width / 2
	if boxWidth < 50 {
		boxWidth = 50
	}
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("204")).
		Padding(1, 2).
		Width(boxWidth).
		Render(s.String())

	return box
}