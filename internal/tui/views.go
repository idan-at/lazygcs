package tui

import (
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/idan-at/lazygcs/internal/gcs"
)

var (
	bucketInfoTitleStyle   = lipgloss.NewStyle().Bold(true)
	bucketInfoProjectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))
	bucketInfoKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))
	bucketInfoValStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4"))
	bucketInfoErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F28FAD"))
	bucketInfoLabelsStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5C2E7"))
	bucketInfoLinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Underline(true)
)

func (m *Model) renderSpinner() string {
	if m.deterministicSpinner {
		return "*"
	}
	return m.spinner.View()
}

// FullPath returns the absolute GCS path of the currently selected or focused item.
func (m *Model) FullPath() string {
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

func (m *Model) renderScrollbar(totalItems, activeIdx int) string {
	maxVisible := m.maxItemsVisible()
	if totalItems <= maxVisible || maxVisible <= 0 {
		return ""
	}

	start, _ := visibleRange(activeIdx, totalItems, maxVisible)

	// Calculate thumb size and position
	// Ensure thumb is at least 1 cell high
	thumbHeight := int(float64(maxVisible) / float64(totalItems) * float64(maxVisible))
	if thumbHeight < 1 {
		thumbHeight = 1
	}

	// Calculate thumb start position
	thumbStart := int(float64(start) / float64(totalItems) * float64(maxVisible))
	// Adjust if it goes out of bounds
	if thumbStart+thumbHeight > maxVisible {
		thumbStart = maxVisible - thumbHeight
	}

	var s strings.Builder
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#414559")) // Dimmed slate
	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")) // Mauve

	for i := 0; i < maxVisible; i++ {
		if i >= thumbStart && i < thumbStart+thumbHeight {
			s.WriteString(thumbStyle.Render("┃"))
		} else {
			s.WriteString(trackStyle.Render("│"))
		}
		if i < maxVisible-1 {
			s.WriteString("\n")
		}
	}
	return s.String()
}

func (m *Model) renderVersionsView(width int) string {
	var s strings.Builder
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("Object Versions") + "\n\n")

	if !m.versioningChecked {
		fmt.Fprintf(&s, "\n%s Loading versions...\n", m.renderSpinner())
		return s.String()
	}

	if !m.isBucketVersioningEnabled {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Render(truncate("Versioning is not enabled for this bucket.", width)))
		return s.String()
	}

	if len(m.objectVersions) == 0 {
		s.WriteString(truncate("No previous versions found.", width))
		return s.String()
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8CAAEE")).Bold(true)
	cellStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4"))
	liveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Bold(true)

	// Column widths
	// Increase widths to use more of the available column space.
	// We'll use absolute values first, then scale down if the column is too narrow.
	genWidth := 25
	sizeWidth := 12
	// Space needed for Generation + spaces + Size + spaces + some space for Updated.
	// 25 + 2 + 12 + 2 + 16 (for YYYY-MM-DD HH:MM) = 57.
	if width < 60 {
		genWidth = int(float64(width) * 0.4)
		sizeWidth = int(float64(width) * 0.2)
	}

	// Ensure at least some minimum widths
	if genWidth < 4 {
		genWidth = 4
	}
	if sizeWidth < 4 {
		sizeWidth = 4
	}

	// Column headers
	fmt.Fprintf(&s, "%s  %s  %s\n", headerStyle.Width(genWidth).Render("Generation"), headerStyle.Width(sizeWidth).Render("Size"), headerStyle.Render("Updated"))
	s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#414559")).Render(strings.Repeat("─", width)) + "\n")

	// Max versions to show
	maxLines := m.maxItemsVisible() - 4
	if maxLines < 1 {
		maxLines = 1
	}

	displayVersions := m.objectVersions
	if len(displayVersions) > maxLines {
		displayVersions = displayVersions[:maxLines]
	}

	for i, v := range displayVersions {
		sizeStr := humanizeSize(v.Size)
		updatedStr := v.Updated.Format("2006-01-02 15:04")

		genStr := fmt.Sprintf("%d", v.Generation)
		var styledGen string
		if i == 0 { // Latest is first
			styledGen = liveStyle.Width(genWidth).Render(truncate(genStr, genWidth))
		} else {
			styledGen = cellStyle.Width(genWidth).Render(truncate(genStr, genWidth))
		}

		fmt.Fprintf(&s, "%s  %s  %s\n", styledGen, cellStyle.Width(sizeWidth).Render(truncate(sizeStr, sizeWidth)), cellStyle.Render(truncate(updatedStr, width-genWidth-sizeWidth-4)))
	}

	if len(m.objectVersions) > maxLines {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Render(truncate(fmt.Sprintf("\n... and %d more", len(m.objectVersions)-maxLines), width)))
	}

	return s.String()
}

func (m *Model) previewView(width int) string {
	var s strings.Builder
	switch m.state {
	case viewObjects, viewDownloadConfirm:
		if m.showVersions {
			s.WriteString(m.renderVersionsView(width))
			if strings.HasPrefix(m.previewContent, "\x1b_G") {
				s.WriteString(clearImagesEsc)
			}
			return s.String()
		}

		title := "Preview"
		if m.showMetadata {
			title = "Metadata"
		}
		s.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n")

		currentPrefixes, currentObjects, _ := m.filteredObjects()

		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))            // Dimmed text
		valStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CDD6F4")) // Bright white

		if m.cursor < len(currentPrefixes) {
			// Selected item is a prefix (folder)
			prefix := currentPrefixes[m.cursor]

			displayName := getDisplayName(prefix.Name, m.currentPrefix)
			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(displayName, width-10)))
			folderURL := fmt.Sprintf("https://console.cloud.google.com/storage/browser/%s/%s;tab=objects?project=%s", m.currentBucket, strings.TrimSuffix(prefix.Name, "/"), m.currentProjectID)
			fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Console Link:"), valStyle.Render(terminalHyperlink(folderURL, bucketInfoLinkStyle.Render("Link"))))

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
					fmt.Fprintf(&s, "\n%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Render(truncate(fmt.Sprintf("Metadata Error: %v", prefix.Err), width-2)))
				}
			}
		} else if m.cursor >= len(currentPrefixes) && len(currentObjects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]

				if m.showMetadata {
					// Detailed Metadata View
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(obj.Name, width-10)))
					objectURL := fmt.Sprintf("https://console.cloud.google.com/storage/browser/_details/%s/%s?project=%s", m.currentBucket, obj.Name, m.currentProjectID)
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Console Link:"), valStyle.Render(terminalHyperlink(objectURL, bucketInfoLinkStyle.Render("Link"))))

					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Bucket:"), valStyle.Render(truncate(obj.Bucket, width-12)))
					s.WriteString("\n")

					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Size:"), valStyle.Render(humanizeSize(obj.Size)))

					storageClass := obj.StorageClass
					if storageClass == "" {
						storageClass = "STANDARD" // Default fallback if missing
					}
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Storage Class:"), valStyle.Render(storageClass))
					s.WriteString("\n")

					if !obj.Created.IsZero() {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(obj.Created.Format(time.RFC3339)))
					}
					if !obj.Updated.IsZero() {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(obj.Updated.Format(time.RFC3339)))
					}
					s.WriteString("\n")

					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Content-Type:"), valStyle.Render(truncate(obj.ContentType, width-18)))
					if obj.ContentEncoding != "" {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Content-Encoding:"), valStyle.Render(truncate(obj.ContentEncoding, width-22)))
					}
					if obj.CacheControl != "" {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Cache-Control:"), valStyle.Render(truncate(obj.CacheControl, width-19)))
					}
					s.WriteString("\n")

					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Generation:"), valStyle.Render(fmt.Sprintf("%d", obj.Generation)))
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Metageneration:"), valStyle.Render(fmt.Sprintf("%d", obj.Metageneration)))
					if obj.ETag != "" {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("ETag:"), valStyle.Render(truncate(obj.ETag, width-10)))
					}
					if len(obj.MD5) > 0 {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("MD5:"), valStyle.Render(fmt.Sprintf("%x", obj.MD5)))
					}
					if obj.CRC32C != 0 {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("CRC32C:"), valStyle.Render(fmt.Sprintf("%x", obj.CRC32C)))
					}

					if len(obj.Metadata) > 0 {
						s.WriteString("\n")
						s.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5C2E7")).Render("Custom Metadata:") + "\n")
						for k, v := range obj.Metadata {
							fmt.Fprintf(&s, "  %s %s\n", keyStyle.Render(k+":"), valStyle.Render(truncate(v, width-len(k)-6)))
						}
					}

					if strings.HasPrefix(m.previewContent, "\x1b_G") {
						s.WriteString(clearImagesEsc)
					}
				} else {
					// Standard Preview
					displayName := getDisplayName(obj.Name, m.currentPrefix)
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Name:"), valStyle.Render(truncate(displayName, width-10)))
					objectURL := fmt.Sprintf("https://console.cloud.google.com/storage/browser/_details/%s/%s?project=%s", m.currentBucket, obj.Name, m.currentProjectID)
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Console Link:"), valStyle.Render(terminalHyperlink(objectURL, bucketInfoLinkStyle.Render("Link"))))

					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Size:"), valStyle.Render(humanizeSize(obj.Size)))

					contentType := obj.ContentType
					if contentType == "" {
						contentType = mime.TypeByExtension(filepath.Ext(obj.Name))
					}
					if contentType == "" {
						ext := strings.ToLower(filepath.Ext(obj.Name))
						switch ext {
						case ".py":
							contentType = "text/x-python"
						case ".go":
							contentType = "text/x-go"
						case ".sql":
							contentType = "application/sql"
						case ".md":
							contentType = "text/markdown"
						case ".sh":
							contentType = "application/x-sh"
						case ".yaml", ".yml":
							contentType = "application/x-yaml"
						default:
							contentType = "unknown"
						}
					}
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Type:"), valStyle.Render(truncate(contentType, width-10)))

					if !obj.Created.IsZero() {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Created:"), valStyle.Render(obj.Created.Format("2006-01-02 15:04:05")))
					}
					fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Updated:"), valStyle.Render(obj.Updated.Format("2006-01-02 15:04:05")))

					if obj.Owner != "" {
						fmt.Fprintf(&s, "%s %s\n", keyStyle.Render("Owner:"), valStyle.Render(truncate(obj.Owner, width-10)))
					}

					if m.previewContent != "" && m.previewContent != clearImagesEsc {
						separator := lipgloss.NewStyle().
							Border(lipgloss.NormalBorder(), true, false, false, false).
							BorderForeground(lipgloss.Color("#414559")).
							Width(width).
							MarginTop(1).
							Render("")

						s.WriteString(separator)
						s.WriteString("\n")

						if m.previewContent == "Loading..." || m.previewContent == clearImagesEsc+"Loading..." {
							fmt.Fprintf(&s, "\n%s Loading preview...\n", m.renderSpinner())
						} else if isBinary(m.previewContent) {
							s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Italic(true).Render("(binary content)"))
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

							if isKitty && (m.showHelp || m.showMessages) {
								// Embed the clear command here so BubbleTea's diff renderer sends it when toggling views.
								s.WriteString(clearImagesEsc)
								s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Italic(true).Render("(image preview hidden)"))
							} else {
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
							}

							if len(allLines) > maxLines {
								s.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Render("..."))
							}
						}
					}
				}
			}
		}
	case viewBuckets:
		filtered := m.filteredBuckets()
		isProject := false
		if m.cursor < len(filtered) && filtered[m.cursor].IsProject {
			isProject = true
		}

		title := "Bucket Information"
		if isProject {
			title = "Project Information"
		}
		s.WriteString(bucketInfoTitleStyle.Render(title) + "\n\n")

		if m.cursor < len(filtered) {
			item := filtered[m.cursor]
			if item.IsProject {
				s.WriteString(bucketInfoProjectStyle.Render("Project: ") + item.ProjectID + "\n")
				projectURL := fmt.Sprintf("https://console.cloud.google.com/welcome?project=%s", item.ProjectID)
				fmt.Fprintf(&s, "%s %s\n\n", bucketInfoKeyStyle.Render("Console Link:"), bucketInfoValStyle.Render(terminalHyperlink(projectURL, bucketInfoLinkStyle.Render("Link"))))

				if m.previewContent == "Loading project info..." || m.previewContent == clearImagesEsc+"Loading project info..." {
					fmt.Fprintf(&s, "\n%s Loading project info...\n", m.renderSpinner())
				} else if strings.HasPrefix(m.previewContent, "Error:") {
					fmt.Fprintf(&s, "\n%s\n", bucketInfoErrorStyle.Render(m.previewContent))
				} else {
					// Find the project to get the bucket count
					for _, p := range m.projects {
						if p.ProjectID == item.ProjectID {
							if cacheEntry, ok := m.projectMetadataCache.Get(item.ProjectID); ok && cacheEntry.Metadata != nil {
								meta := cacheEntry.Metadata
								if meta.Name != "" && meta.Name != item.ProjectID {
									fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Project Name:"), bucketInfoValStyle.Render(meta.Name))
								}
								if meta.ProjectNumber > 0 {
									fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Project Number:"), bucketInfoValStyle.Render(fmt.Sprintf("%d", meta.ProjectNumber)))
								}
								if !meta.CreateTime.IsZero() {
									fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Created:"), bucketInfoValStyle.Render(meta.CreateTime.Format("2006-01-02 15:04:05")))
								}
								if meta.ParentType != "" {
									fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Parent:"), bucketInfoValStyle.Render(fmt.Sprintf("%s (%s)", meta.ParentType, meta.ParentID)))
								}
								if len(meta.Labels) > 0 {
									s.WriteString("\n" + bucketInfoLabelsStyle.Render("Labels:") + "\n")
									// Sort labels
									keys := make([]string, 0, len(meta.Labels))
									for k := range meta.Labels {
										keys = append(keys, k)
									}
									sort.Strings(keys)
									for _, k := range keys {
										fmt.Fprintf(&s, "  %s %s\n", bucketInfoKeyStyle.Render(k+":"), bucketInfoValStyle.Render(truncate(meta.Labels[k], width-len(k)-6)))
									}
								}
							}

							fmt.Fprintf(&s, "\n%s %s\n", bucketInfoKeyStyle.Render("Total Buckets:"), bucketInfoValStyle.Render(fmt.Sprintf("%d", len(p.Buckets))))
							break
						}
					}
				}
			} else {
				fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Bucket:"), bucketInfoValStyle.Render(truncate(item.BucketName, width-10)))
				bucketURL := fmt.Sprintf("https://console.cloud.google.com/storage/browser/%s?project=%s", item.BucketName, item.ProjectID)
				fmt.Fprintf(&s, "%s %s\n\n", bucketInfoKeyStyle.Render("Console Link:"), bucketInfoValStyle.Render(terminalHyperlink(bucketURL, bucketInfoLinkStyle.Render("Link"))))

				if m.previewContent == "Loading..." || m.previewContent == clearImagesEsc+"Loading..." {
					fmt.Fprintf(&s, "\n%s Loading metadata...\n", m.renderSpinner())
				} else if strings.HasPrefix(m.previewContent, "Error:") {
					fmt.Fprintf(&s, "\n%s\n", bucketInfoErrorStyle.Render(m.previewContent))
				} else if cacheEntry, ok := m.bucketMetadataCache.Peek(item.BucketName); ok && time.Now().Before(cacheEntry.ExpiresAt) {
					meta := cacheEntry.Metadata
					if meta != nil {
						fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Location:"), bucketInfoValStyle.Render(meta.Location))

						storageClass := meta.StorageClass
						if storageClass == "" {
							storageClass = "STANDARD"
						}
						fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Storage Class:"), bucketInfoValStyle.Render(storageClass))

						versioning := "Disabled"
						if meta.VersioningEnabled {
							versioning = "Enabled"
						}
						fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Versioning:"), bucketInfoValStyle.Render(versioning))

						if !meta.Created.IsZero() {
							fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Created At:"), bucketInfoValStyle.Render(meta.Created.Format("2006-01-02 15:04:05")))
						}

						if meta.OwnerEntity != "" {
							fmt.Fprintf(&s, "%s %s\n", bucketInfoKeyStyle.Render("Owner:"), bucketInfoValStyle.Render(truncate(meta.OwnerEntity, width-10)))
						}

						if len(cacheEntry.SortedLabels) > 0 {
							s.WriteString("\n" + bucketInfoLabelsStyle.Render("Labels:") + "\n")
							for _, label := range cacheEntry.SortedLabels {
								k, v := label.Key, label.Value
								fmt.Fprintf(&s, "  %s %s\n", bucketInfoKeyStyle.Render(k+":"), bucketInfoValStyle.Render(truncate(v, width-len(k)-6)))
							}
						}
					} else if cacheEntry.Err != nil {
						// Fallback in case the previewContent wasn't set to "Error:" but the cache has an error
						fmt.Fprintf(&s, "\n%s\n", bucketInfoErrorStyle.Render(fmt.Sprintf("Error: %v", cacheEntry.Err)))
					}
				}
			}
		}
	}
	return s.String()
}

func (m *Model) headerView() string {
	path := m.FullPath()
	path = strings.TrimPrefix(path, "gs://")

	var parts []string
	if path != "" {
		parts = strings.Split(strings.TrimSuffix(path, "/"), "/")
	}

	rootPill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#1E1E2E")).
		Background(lipgloss.Color("#8CAAEE")).
		Padding(0, 1).
		Render("gs://")

	breadcrumbs := []string{rootPill}

	for i, part := range parts {
		if part == "" {
			continue
		}
		var pill string
		if i == len(parts)-1 {
			// Last item is active
			pill = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#1E1E2E")).
				Background(lipgloss.Color("#CBA6F7")).
				Padding(0, 1).
				Render(part)
		} else {
			// Intermediate items
			pill = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CDD6F4")).
				Background(lipgloss.Color("#414559")).
				Padding(0, 1).
				Render(part)
		}
		breadcrumbs = append(breadcrumbs, pill)
	}

	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6C7086")).
		Margin(0, 1).
		Render("❯")

	renderedPath := truncate(strings.Join(breadcrumbs, separator), m.width-2)

	return lipgloss.NewStyle().
		Width(m.width).
		Render(renderedPath)
}

func (m *Model) footerView() string {
	// Left side: Status Pill
	statusText := " NORMAL "
	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.Color("#414559")).
		Foreground(lipgloss.Color("#CDD6F4"))

	q := m.bucketSearchQuery
	if m.state == viewObjects || m.state == viewDownloadConfirm {
		q = m.objectSearchQuery
	}

	if m.creationMode {
		prompt := " NEW FILE: "
		if m.state == viewBuckets {
			prompt = " NEW BUCKET: "
		} else if strings.HasSuffix(m.creationQuery, "/") {
			prompt = " NEW DIR: "
		}
		statusText = fmt.Sprintf("%s%s_ ", prompt, m.creationQuery)
		statusStyle = statusStyle.Background(lipgloss.Color("#A6E3A1")).Foreground(lipgloss.Color("#1E1E2E"))
	} else if m.searchMode {
		statusText = fmt.Sprintf(" SEARCH: %s_ ", q)
		statusStyle = statusStyle.Background(lipgloss.Color("#CBA6F7")).Foreground(lipgloss.Color("#1E1E2E"))
	} else if q != "" {
		statusText = fmt.Sprintf(" FILTER: %s ", q)
		statusStyle = statusStyle.Background(lipgloss.Color("#CBA6F7")).Foreground(lipgloss.Color("#1E1E2E"))
	} else if m.bgJobs > len(m.loadingProjects) {
		statusText = " LOADING "
		statusStyle = statusStyle.Background(lipgloss.Color("#8CAAEE")).Foreground(lipgloss.Color("#1E1E2E"))
	}

	pill := statusStyle.Render(statusText)

	// Task Pill
	var tasksPill string
	if len(m.activeTasks) > 0 {
		tasksPill = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("#F9E2AF")).
			Foreground(lipgloss.Color("#1E1E2E")).
			Render(fmt.Sprintf(" ⟳ %d Tasks ", len(m.activeTasks)))
	}

	// Error Pill
	var errorsPill string
	if m.msgQueue.ErrorCount > 0 {
		errorsPill = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("#F38BA8")).
			Foreground(lipgloss.Color("#1E1E2E")).
			Render(fmt.Sprintf(" %d ERRORS ", m.msgQueue.ErrorCount))
	}

	hasMessage := m.state == viewDownloadConfirm || (len(m.msgQueue.Messages()) > 0 && !m.msgQueue.HideStatusPill)

	// Right side: Help hints
	var helpView string
	if !hasMessage {
		m.help.ShowAll = false
		m.help.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#BAC2DE"))
		m.help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
		m.help.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("#414559"))
		helpView = m.help.View(keys)
	}

	// Calculate available width for msgPill
	leftBaseWidth := lipgloss.Width(pill + tasksPill + errorsPill)
	helpWidth := lipgloss.Width(helpView)
	availableWidth := m.width - leftBaseWidth - helpWidth - 4

	var msgPill string
	if m.state == viewDownloadConfirm {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#F9E2AF"))
		icon := getLevelIcon(LevelWarn, m.showNerdIcons)
		var text string
		if m.activeDestinations != nil && m.activeDestinations[m.pendingDownloadDest] {
			text = fmt.Sprintf("File is actively downloading: %s - (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(m.pendingDownloadDest))
		} else {
			text = fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(m.pendingDownloadDest))
		}
		msgPill = style.Render(truncate(icon+text, availableWidth))
	} else {
		// Calculate aggregate progress
		var totalProgress int
		var activeDlCount int
		var totalBytes int64
		var currentBytes int64
		showProgressBar := false

		for _, t := range m.activeTasks {
			if strings.Contains(t.Name, "Downloading") {
				activeDlCount++
				totalBytes += t.TotalBytes
				currentBytes += t.Current
				if time.Since(t.Started) > ProgressVisibilityThreshold {
					showProgressBar = true
				}
			}
		}

		if activeDlCount > 0 && totalBytes > 0 {
			totalProgress = int(float64(currentBytes) / float64(totalBytes) * 100)
		}

		if len(m.msgQueue.Messages()) > 0 && !m.msgQueue.HideStatusPill {
			latest := m.msgQueue.Messages()[len(m.msgQueue.Messages())-1]
			style := lipgloss.NewStyle().Padding(0, 1)
			switch latest.Level {
			case LevelError:
				style = style.Foreground(lipgloss.Color("#F38BA8"))
			case LevelWarn:
				style = style.Foreground(lipgloss.Color("#F9E2AF"))
			default:
				style = style.Foreground(lipgloss.Color("#A6E3A1"))
			}
			icon := getLevelIcon(latest.Level, m.showNerdIcons)
			text := latest.Text

			if showProgressBar && strings.Contains(text, "Downloading") {
				bar := renderProgressBar(10, totalProgress)
				text = fmt.Sprintf("%s %s %d%%", text, bar, totalProgress)
			}
			msgPill = style.Render(truncate(icon+text, availableWidth))
		}
	}

	// Build the final footer bar
	leftSide := lipgloss.JoinHorizontal(lipgloss.Top, pill, tasksPill, errorsPill, msgPill)

	// Fill the gap between left and right
	gapWidth := m.width - lipgloss.Width(leftSide) - lipgloss.Width(helpView)
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := strings.Repeat(" ", gapWidth)

	return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, leftSide, gap, helpView)
}

func (m *Model) maxItemsVisible() int {
	v := m.height - 10
	if m.showMessages || m.showHelp || m.state == viewDeleteConfirm {
		v -= 19 // Space for either help, messages, or delete confirm view
	}
	if v < 1 {
		v = 1
	}
	return v
}

func (m *Model) objectsView(width int) string {
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
					return lipgloss.NewStyle().Bold(true).Render(truncate(fmt.Sprintf("Objects in %s", targetBucket), width)) + "\n\n" +
						fmt.Sprintf("%s Loading...", m.renderSpinner())
				}
			}
		}
	}

	if !showObjects {
		return ""
	}

	title := lipgloss.NewStyle().Bold(true).Render(truncate(fmt.Sprintf("Objects in %s", targetBucket), width))
	totalItems := len(currentPrefixes) + len(currentObjects)

	if m.loading && m.state != viewBuckets && totalItems == 0 {
		return title + "\n\n" + fmt.Sprintf("%s Loading...", m.renderSpinner())
	}

	maxVisible := m.maxItemsVisible()
	displayMaxVisible := maxVisible
	if m.loading && m.state != viewBuckets {
		displayMaxVisible--
		if displayMaxVisible < 1 {
			displayMaxVisible = 1
		}
	}

	startIdx := objCursor
	if startIdx < 0 {
		startIdx = 0
	}

	scrollbar := m.renderScrollbar(totalItems, startIdx)
	listWidth := width
	if scrollbar != "" {
		listWidth -= 2
	}

	start, end := visibleRange(startIdx, totalItems, displayMaxVisible)
	var listBuilder strings.Builder

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
			selectionIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5C2E7")).Render("✓")
		}

		displayItem := getDisplayName(originalName, targetPrefix)
		icon := getIcon(displayItem, isFolder, false, m.showNerdIcons)
		iconColor := getIconColor(displayItem, isFolder, false)

		textStyle := lipgloss.NewStyle()
		isFocused := (m.state != viewBuckets) && (objCursor == i)
		if isFocused {
			textStyle = textStyle.Background(lipgloss.Color("#313244")).Foreground(lipgloss.Color("#CDD6F4")).Bold(true)
		} else if isSelected {
			textStyle = textStyle.Foreground(lipgloss.Color("#F5C2E7")).Bold(true)
		} else {
			textStyle = textStyle.Foreground(lipgloss.Color("#A6ADC8"))
		}

		iconStyle := textStyle.Foreground(lipgloss.Color(iconColor))
		styledIcon := iconStyle.Render(icon)

		truncateLen := listWidth - 2 - lipgloss.Width(icon)
		truncatedItem := truncate(displayItem, truncateLen)
		highlightedItem := highlightMatch(truncatedItem, m.objectSearchQuery, m.fuzzySearch)

		itemContent := fmt.Sprintf("%s %s%s", selectionIndicator, styledIcon, highlightedItem)
		content := textStyle.Width(listWidth).Render(itemContent)
		listBuilder.WriteString(content + "\n")
	}

	if m.loading && m.state != viewBuckets {
		fmt.Fprintf(&listBuilder, "%s Loading...", m.renderSpinner())
	} else if totalItems == 0 {
		listBuilder.WriteString("(empty)")
	}

	list := listBuilder.String()
	var content string
	if scrollbar != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Top, list, " ", scrollbar)
	} else {
		content = list
	}

	return title + "\n\n" + content
}

func (m *Model) bucketsView(width int) string {
	title := lipgloss.NewStyle().Bold(true).Render(truncate("Buckets", width))

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

	maxVisible := m.maxItemsVisible()
	scrollbar := m.renderScrollbar(len(filtered), activeIdx)
	listWidth := width
	if scrollbar != "" {
		listWidth -= 2
	}

	start, end := visibleRange(activeIdx, len(filtered), maxVisible)
	var listBuilder strings.Builder
	for i := start; i < end; i++ {
		item := filtered[i]

		indicator := " "
		if m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket {
			indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Render("●")
		}

		textStyle := lipgloss.NewStyle()
		isFocused := m.state == viewBuckets && m.cursor == i
		isActiveBucket := m.state != viewBuckets && !item.IsProject && item.BucketName == m.currentBucket

		if isFocused {
			textStyle = textStyle.Background(lipgloss.Color("#313244")).Foreground(lipgloss.Color("#CDD6F4")).Bold(true)
		} else if isActiveBucket {
			textStyle = textStyle.Foreground(lipgloss.Color("#CBA6F7"))
		} else {
			textStyle = textStyle.Foreground(lipgloss.Color("#A6ADC8"))
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
				projectStyle = projectStyle.Foreground(lipgloss.Color("#BAC2DE")).Bold(true)
			}

			truncateLen := listWidth - 2 // ▼ + space = 2
			if m.loadingProjects[item.ProjectID] {
				truncateLen -= 2 // space + spinner
			}
			truncatedProject := truncate(item.ProjectID, truncateLen)

			itemContent := fmt.Sprintf("%s%s", icon, truncatedProject)
			if m.loadingProjects[item.ProjectID] {
				itemContent += " " + m.renderSpinner()
			}
			content := projectStyle.Width(listWidth).Render(itemContent)
			listBuilder.WriteString(content + "\n")
		} else {
			// Bucket Item
			icon := getIcon(item.BucketName, false, true, m.showNerdIcons)
			iconColor := getIconColor(item.BucketName, false, true)

			iconStyle := textStyle.Foreground(lipgloss.Color(iconColor))
			styledIcon := iconStyle.Render(icon)

			// Truncate to fit column, account for indentation
			// Offset: 1 (indicator) + 1 (space) + icon width
			truncateLen := listWidth - 2 - lipgloss.Width(icon)
			truncatedBucket := truncate(item.BucketName, truncateLen)
			highlightedBucket := highlightMatch(truncatedBucket, m.bucketSearchQuery, m.fuzzySearch)

			itemContent := fmt.Sprintf("%s %s%s", indicator, styledIcon, highlightedBucket)
			content := textStyle.Width(listWidth).Render(itemContent)
			listBuilder.WriteString(content + "\n")
		}
	}

	list := listBuilder.String()
	var content string
	if scrollbar != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Top, list, " ", scrollbar)
	} else {
		content = list
	}

	return title + "\n\n" + content
}

// View renders the current state of the application as a string.
func (m *Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\n(press q to quit)", m.err)
	}

	// Styles
	columnHeight := m.maxItemsVisible() + 4 // Title + blank line + list items + borders

	activeStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#CBA6F7")). // Modern Mauve/Purple
		Padding(0, 1).
		Height(columnHeight)

	inactiveStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#414559")). // Dimmed slate
		Foreground(lipgloss.Color("#A6ADC8")).       // Dimmed text
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
	if m.state == viewObjects || m.state == viewDownloadConfirm || m.state == viewDeleteConfirm {
		midStyle = activeStyle
	}
	midCol := midStyle.Width(midWidth - 2).Render(m.objectsView(midWidth - 4))

	// Preview column is always "inactive" in terms of focus
	rightCol := inactiveStyle.Width(rightWidth - 2).Render(m.previewView(rightWidth - 4))

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, midCol, rightCol)

	var view string
	if m.showMessages {
		view = m.headerView() + "\n\n" + mainContent + "\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.messagesView())
	} else if m.showHelp {
		view = m.headerView() + "\n\n" + mainContent + "\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.helpView())
	} else if m.state == viewDeleteConfirm {
		view = m.headerView() + "\n\n" + mainContent + "\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.deleteConfirmView())
	} else {
		view = m.headerView() + "\n\n" + mainContent + m.footerView()
	}

	result := view
	// We add the kitty image clear code here to clear out any rendered images
	// since they are drawn over the text otherwise
	if strings.HasPrefix(m.previewContent, "\x1b_G") {
		return clearImagesEsc + result
	}
	return result
}

func (m *Model) helpView() string {
	groups := keys.FullHelp()
	headers := []string{"Navigation", "Pagination", "Actions", "App"}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Bold(true).Width(12)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8")).Width(20)
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8CAAEE")).Bold(true).Underline(true).MarginBottom(1)

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
		Foreground(lipgloss.Color("#8CAAEE")).
		Render("HELP")

	helpGrid := lipgloss.JoinHorizontal(lipgloss.Top,
		cols[0],
		lipgloss.NewStyle().Padding(0, 2).Render(cols[1]),
		lipgloss.NewStyle().Padding(0, 2).Render(cols[2]),
		lipgloss.NewStyle().Padding(0, 2).Render(cols[3]),
	)

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6C7086")).
		MarginTop(1).
		Render("Press esc or q to close")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", helpGrid, footer)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#CBA6F7")).
		Padding(1, 2).
		Height(15).
		Render(content)

	return box
}

func (m *Model) messagesView() string {
	var s strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#CBA6F7")).
		Render(fmt.Sprintf("MESSAGES (%d)", len(m.msgQueue.Messages())))

	s.WriteString(title + "\n\n")

	start := m.msgQueue.MessagesScroll
	end := start + 9
	if end > len(m.msgQueue.Messages()) {
		end = len(m.msgQueue.Messages())
	}

	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6D189"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C890"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E78284"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))

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

		statusIcon := ""
		progressText := ""

		// Find associated task
		var task *Task
		if msg.TaskID != "" {
			if t, ok := m.activeTasks[msg.TaskID]; ok {
				task = &t
			}
		}

		if task != nil {
			statusIcon = "[>] "
			bar := renderProgressBar(10, task.Progress)
			progressText = fmt.Sprintf(" %s %d%% (%s/%s)",
				bar,
				task.Progress,
				humanizeSize(task.Current),
				humanizeSize(task.TotalBytes))
		} else if strings.Contains(msg.Text, "Downloaded") || strings.Contains(msg.Text, "Uploaded") {
			statusIcon = "[✔] "
		} else if msg.Level == LevelError && (strings.Contains(msg.Text, "Download failed") || strings.Contains(msg.Text, "Upload failed")) {
			statusIcon = "[✘] "
		}

		icon := getLevelIcon(msg.Level, m.showNerdIcons)
		fmt.Fprintf(&s, "%s %s %s%s%s\n", textStyle.Render(timeStr), style.Render(icon), style.Render(statusIcon), style.Render(msg.Text), textStyle.Faint(true).Render(progressText))
	}

	keyStyleFooter := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Bold(true)
	descStyleFooter := lipgloss.NewStyle().Foreground(lipgloss.Color("#585B70"))

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
		BorderForeground(lipgloss.Color("#CBA6F7")).
		Padding(1, 2).
		Width(boxWidth).
		Height(15).
		Render(s.String())

	return box
}

func (m *Model) deleteConfirmView() string {
	var s strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F38BA8")).Render("DELETE CONFIRMATION")
	s.WriteString(title + "\n\n")

	var target string
	if m.pendingDeleteIsBucket {
		target = fmt.Sprintf("gs://%s", m.pendingDeleteBucket)
	} else if m.pendingDeletePrefix != "" {
		target = fmt.Sprintf("gs://%s/%s", m.pendingDeleteBucket, m.pendingDeletePrefix)
	} else {
		target = fmt.Sprintf("gs://%s/%s", m.pendingDeleteBucket, m.pendingDeleteObject)
	}

	fmt.Fprintf(&s, "Are you sure you want to delete %s?\n", lipgloss.NewStyle().Bold(true).Render(target))
	s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Render("This action cannot be undone.") + "\n\n")

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Bold(true)
	fmt.Fprintf(&s, "%s delete    %s cancel", keyStyle.Render("y"), keyStyle.Render("n/esc"))

	modalWidth := m.width - 20
	if modalWidth < 60 {
		modalWidth = 60
	}
	if modalWidth > 120 {
		modalWidth = 120
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F38BA8")).
		Padding(1, 4).
		Width(modalWidth).
		Height(15).
		Render(s.String())
}
