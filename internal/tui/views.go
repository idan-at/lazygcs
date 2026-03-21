package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/idan-at/lazygcs/internal/gcs"
)

func (m Model) renderSpinner() string {
	if m.deterministicSpinner {
		return "*"
	}
	return m.spinner.View()
}

func (m Model) fullPath() string {
	if m.state == viewBuckets {
		filtered := m.filteredBuckets()
		if m.cursor < len(filtered) && !filtered[m.cursor].IsProject {
			return "gs://" + filtered[m.cursor].BucketName + "/"
		}
		return "gs://"
	}

	if m.currentBucket == "" {
		return "gs://"
	}

	path := fmt.Sprintf("gs://%s/%s", m.currentBucket, m.currentPrefix)

	if m.state == viewObjects || m.state == viewDownloadConfirm {
		currentPrefixes, currentObjects, _ := m.filteredObjects()
		if m.cursor < len(currentPrefixes) {
			path = "gs://" + m.currentBucket + "/" + currentPrefixes[m.cursor].Name
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				path = "gs://" + m.currentBucket + "/" + currentObjects[idx].Name
			}
		}
	}

	return path
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

			displayName := getDisplayName(prefix.Name, m.currentPrefix)
			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(displayName, width-10)))
			if !prefix.Fetched {
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render("Folder"))
				fmt.Fprintf(&s, "\n%s Loading metadata...\n", m.renderSpinner())
			} else {
				folderType := "Folder"
				if prefix.Err != nil {
					folderType = "Folder (Virtual)"
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
				displayName := getDisplayName(obj.Name, m.currentPrefix)
				fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(displayName, width-10)))
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

				if m.previewContent != "" && m.previewContent != "\x1b_Ga=d,d=A\x1b\\" {
					separator := lipgloss.NewStyle().
						Border(lipgloss.NormalBorder(), true, false, false, false).
						BorderForeground(lipgloss.Color("240")).
						Width(width).
						MarginTop(1).
						Render("")

					s.WriteString(separator)
					s.WriteString("\n")

					if m.previewContent == "Loading..." || m.previewContent == "\x1b_Ga=d,d=A\x1b\\Loading..." {
						fmt.Fprintf(&s, "\n%s Loading preview...\n", m.renderSpinner())
					} else if isBinary(m.previewContent) {
						s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true).Render("(binary content)"))
					} else {
						// Calculate how many lines we can actually show
						// We want the ENTIRE previewView content to fit in m.maxItemsVisible() + 2
						// Preview Title (2) + Metadata (~6) + Separator (2) = ~10 lines
						maxLines := m.maxItemsVisible() - 8
						if maxLines < 1 {
							maxLines = 1
						}

						allLines := strings.Split(m.previewContent, "\n")
						displayLines := allLines
						if len(displayLines) > maxLines {
							displayLines = displayLines[:maxLines]
						}

						isKitty := strings.HasPrefix(m.previewContent, "\x1b_G")

						for i, line := range displayLines {
							if isKitty {
								s.WriteString(line)
							} else {
								s.WriteString(truncate(line, width-2))
							}
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

	q := m.bucketSearchQuery
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		q = m.objectSearchQuery
	}

	if m.searchMode {
		statusText = fmt.Sprintf(" SEARCH: %s█ ", q)
		statusStyle = statusStyle.Background(lipgloss.Color("69")).Foreground(lipgloss.Color("15"))
	} else if q != "" {
		statusText = fmt.Sprintf(" FILTER: %s ", q)
		statusStyle = statusStyle.Background(lipgloss.Color("61")).Foreground(lipgloss.Color("15"))
	} else if m.bgJobs > len(m.loadingProjects) {
		statusText = m.renderSpinner()
		statusStyle = lipgloss.NewStyle().Padding(0, 1)
	}

	pill := statusStyle.Render(statusText)

	var tasksPill string
	if len(m.activeTasks) > 0 {
		tasksPill = " " + lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("130")).
			Foreground(lipgloss.Color("15")).
			Render(fmt.Sprintf("⟳ %d Tasks", len(m.activeTasks)))
	}

	var errorsPill string
	if m.msgQueue.ErrorCount > 0 {
		errorsPill = " " + lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("196")). // Red background
			Foreground(lipgloss.Color("15")).
			Render(fmt.Sprintf("%d ERRORS", m.msgQueue.ErrorCount))
	}

	// Right side: Help hints
	m.help.ShowAll = false
	m.help.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	m.help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpView := m.help.View(keys)

	// Calculate available width for msgPill
	rightPadding := 1
	leftBase := pill + tasksPill + errorsPill
	availableWidth := m.width - lipgloss.Width(leftBase) - lipgloss.Width(helpView) - rightPadding - 2

	var msgPill string
	if m.state == viewDownloadConfirm {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("214")).Faint(true)
		icon := getLevelIcon(LevelWarn, m.showNerdIcons)
		var text string
		if m.activeDestinations != nil && m.activeDestinations[m.pendingDownloadDest] {
			text = fmt.Sprintf("File is actively downloading: %s - (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(m.pendingDownloadDest))
		} else {
			text = fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(m.pendingDownloadDest))
		}
		if lipgloss.Width(icon+text) > availableWidth {
			text = truncate(text, availableWidth-lipgloss.Width(icon))
		}
		msgPill = " " + style.Render(icon+text)
	} else if len(m.msgQueue.Messages()) > 0 && !m.msgQueue.HideStatusPill {
		latest := m.msgQueue.Messages()[len(m.msgQueue.Messages())-1]
		style := lipgloss.NewStyle().Padding(0, 1).Faint(true)
		switch latest.Level {
		case LevelError:
			style = style.Foreground(lipgloss.Color("196"))
		case LevelWarn:
			style = style.Foreground(lipgloss.Color("214"))
		default:
			style = style.Foreground(lipgloss.Color("42"))
		}
		icon := getLevelIcon(latest.Level, m.showNerdIcons)
		text := latest.Text
		if lipgloss.Width(icon+text) > availableWidth {
			text = truncate(text, availableWidth-lipgloss.Width(icon))
		}
		msgPill = " " + style.Render(icon+text)
	}

	// Build the ribbon
	leftContent := pill + tasksPill + msgPill + errorsPill
	gapWidth := m.width - lipgloss.Width(leftContent) - lipgloss.Width(helpView) - rightPadding
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := strings.Repeat(" ", gapWidth)

	return "\n" + leftContent + gap + helpView + strings.Repeat(" ", rightPadding)
}

func (m Model) maxItemsVisible() int {
	v := m.height - 10
	if m.showHelp {
		v -= 12 // Space for the shorter, wider help box
	}
	if v < 1 {
		v = 1
	}
	return v
}

func (m Model) objectsView(width int) string {
	var s strings.Builder

	var targetBucket string
	var targetPrefix string
	var currentPrefixes []gcs.PrefixMetadata
	var currentObjects []gcs.ObjectMetadata
	var showObjects bool
	objCursor := -1

	switch m.state {
	case viewObjects, viewDownloadConfirm:
		targetBucket = m.currentBucket
		targetPrefix = m.currentPrefix
		currentPrefixes, currentObjects, _ = m.filteredObjects()
		showObjects = true
		objCursor = m.cursor
	case viewBuckets:
		filtered := m.filteredBuckets()
		if m.cursor < len(filtered) {
			item := filtered[m.cursor]
			if !item.IsProject {
				targetBucket = item.BucketName
				targetPrefix = ""
				cacheKey := targetBucket + "::"
				if cached, ok := m.listCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
					currentPrefixes = cached.List.Prefixes
					currentObjects = cached.List.Objects
					showObjects = true
				} else {
					s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate(fmt.Sprintf("Objects in %s", targetBucket), width)) + "\n\n")
					fmt.Fprintf(&s, "%s Loading...", m.renderSpinner())
					return s.String()
				}
			}
		}
	}

	if showObjects {
		title := fmt.Sprintf("Objects in %s", targetBucket)
		s.WriteString(lipgloss.NewStyle().Bold(true).Render(truncate(title, width)) + "\n\n")

		totalItems := len(currentPrefixes) + len(currentObjects)

		if m.loading && m.state != viewBuckets && totalItems == 0 {
			fmt.Fprintf(&s, "%s Loading...", m.renderSpinner())
		} else {
			maxVisible := m.maxItemsVisible()
			if m.loading && m.state != viewBuckets {
				maxVisible--
				if maxVisible < 1 {
					maxVisible = 1
				}
			}

			startIdx := objCursor
			if startIdx < 0 {
				startIdx = 0
			}
			start, end := visibleRange(startIdx, totalItems, maxVisible)

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

				displayItem := getDisplayName(originalName, targetPrefix)

				icon := getIcon(displayItem, isFolder, false, m.showNerdIcons)

				textStyle := lipgloss.NewStyle()
				isFocused := (m.state != viewBuckets) && (objCursor == i)
				if isFocused {
					textStyle = textStyle.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15")).Bold(true)
				} else if isSelected {
					textStyle = textStyle.Foreground(lipgloss.Color("212")).Bold(true)
				} else {
					textStyle = textStyle.Foreground(lipgloss.Color("250"))
				}

				// Truncate to fit column (account for selection indicator, optional icon, and padding)
				// Offset: 1 (indicator) + 1 (space) + icon width
				truncateLen := width - 2 - lipgloss.Width(icon)
				truncatedItem := truncate(displayItem, truncateLen)

				itemContent := fmt.Sprintf("%s %s%s", selectionIndicator, icon, truncatedItem)
				content := textStyle.Width(width).Render(itemContent)

				s.WriteString(content + "\n")
			}
			if totalItems == 0 {
				s.WriteString("(empty)")
			} else if m.loading && m.state != viewBuckets {
				fmt.Fprintf(&s, "%s Loading...", m.renderSpinner())
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

			truncateLen := width - 2 // ▼ + space = 2
			if m.loadingProjects[item.ProjectID] {
				truncateLen -= 2 // space + spinner
			}
			truncatedProject := truncate(item.ProjectID, truncateLen)

			itemContent := fmt.Sprintf("%s%s", icon, truncatedProject)
			if m.loadingProjects[item.ProjectID] {
				itemContent += " " + m.renderSpinner()
			}
			content := projectStyle.Width(width).Render(itemContent)
			s.WriteString(content + "\n")
		} else {
			// Bucket Item
			icon := getIcon(item.BucketName, false, true, m.showNerdIcons)

			// Truncate to fit column, account for indentation
			// Offset: 1 (indicator) + 1 (space) + icon width
			truncateLen := width - 2 - lipgloss.Width(icon)
			truncatedBucket := truncate(item.BucketName, truncateLen)

			itemContent := fmt.Sprintf("%s %s%s", indicator, icon, truncatedBucket)
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

	var view string
	if m.showHelp {
		view = m.headerView() + "\n\n" + mainContent + "\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.helpView())
	} else {
		view = m.headerView() + "\n\n" + mainContent + m.footerView()
	}

	result := view
	if m.showMessages {
		// Use lipgloss.Place to center the errors modal.
		result = lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			m.messagesView(),
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(lipgloss.Color("236")),
		)
		// We add the kitty image clear code here to clear out any rendered images
		// since they are drawn over the text otherwise
		if strings.HasPrefix(m.previewContent, "\x1b_G") {
			return "\x1b_Ga=d,d=A\x1b\\" + result
		}
		return result
	}

	if strings.HasPrefix(m.previewContent, "\x1b_Ga=d,d=A\x1b\\") {
		return "\x1b_Ga=d,d=A\x1b\\" + result
	}
	return result
}

func (m Model) helpView() string {
	groups := keys.FullHelp()
	headers := []string{"Navigation", "Pagination", "Actions", "App"}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true).Width(12)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(20)
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Underline(true).MarginBottom(1)

	cols := make([]string, len(groups))
	for i, group := range groups {
		var colBuilder strings.Builder
		colBuilder.WriteString(headerStyle.Render(headers[i]) + "\n")
		for _, b := range group {
			help := b.Help()
			colBuilder.WriteString(keyStyle.Render(help.Key) + descStyle.Render(help.Desc) + "\n")
		}
		cols[i] = strings.TrimSpace(colBuilder.String())
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Render("HELP")

	helpGrid := lipgloss.JoinHorizontal(lipgloss.Top,
		cols[0],
		lipgloss.NewStyle().Padding(0, 2).Render(cols[1]),
		lipgloss.NewStyle().Padding(0, 2).Render(cols[2]),
		lipgloss.NewStyle().Padding(0, 2).Render(cols[3]),
	)

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(1).
		Render("Press esc or q to close")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", helpGrid, footer)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("69")).
		Padding(1, 2).
		Render(content)

	return box
}

func (m Model) messagesView() string {
	var s strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69")).
		Render(fmt.Sprintf("MESSAGES (%d)", len(m.msgQueue.Messages())))

	s.WriteString(title + "\n\n")

	start := m.msgQueue.MessagesScroll
	end := start + 15
	if end > len(m.msgQueue.Messages()) {
		end = len(m.msgQueue.Messages())
	}

	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	for i := start; i < end; i++ {
		msg := m.msgQueue.Messages()[i]
		timeStr := msg.Timestamp.Format("15:04:05")
		if m.deterministicSpinner {
			timeStr = "12:00:00"
		}

		style := infoStyle
		switch msg.Level {
		case LevelWarn:
			style = warnStyle
		case LevelError:
			style = errStyle
		}

		icon := getLevelIcon(msg.Level, m.showNerdIcons)
		fmt.Fprintf(&s, "%s %s %s\n", textStyle.Render(timeStr), style.Render(icon), textStyle.Render(msg.Text))
	}

	keyStyleFooter := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	descStyleFooter := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	line1 := descStyleFooter.Render("Use ") + keyStyleFooter.Render("j/k") + descStyleFooter.Render(" to scroll")
	line2 := keyStyleFooter.Render("esc") + descStyleFooter.Render(" or ") + keyStyleFooter.Render("q") + descStyleFooter.Render(" to close")

	footer := lipgloss.NewStyle().MarginTop(1).Render(line1 + "\n" + line2)

	s.WriteString("\n" + footer)

	boxWidth := m.width - 10
	if boxWidth < 50 {
		boxWidth = 50
	}
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
		if boxWidth < 10 {
			boxWidth = 10
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("69")).
		Padding(1, 2).
		Width(boxWidth).
		Render(s.String())

	return box
}
