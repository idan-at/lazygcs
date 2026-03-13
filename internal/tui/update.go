package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"lazygcs/internal/gcs"
)

// Update processes terminal messages (key presses, window resizes) and async responses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case BucketsPageMsg:
		return m.handleBucketsPageMsg(msg)
	case ObjectsMsg:
		return m.handleObjectsMsg(msg)
	case ObjectsPageMsg:
		return m.handleObjectsPageMsg(msg)
	case MetadataMsg:
		return m.handleMetadataMsg(msg)
	case ContentMsg:
		return m.handleContentMsg(msg)
	case DownloadMsg:
		return m.handleDownloadMsg(msg)
	case DebouncePreviewMsg:
		if msg.CursorVersion == m.cursorVersion {
			return m, msg.FetchCmd
		}
		return m, nil
	case HoverPrefetchTickMsg:
		if msg.CursorVersion == m.cursorVersion {
			if msg.FetchCmd != nil {
				return m, msg.FetchCmd
			}
		}
		return m, nil
	case HoverPrefetchMsg:
		if msg.Err == nil && msg.List != nil {
			cacheKey := msg.Bucket + "::" + msg.Prefix
			m.listCache[cacheKey] = listCacheEntry{List: msg.List, ExpiresAt: time.Now().Add(5 * time.Minute)}
		}
		return m, nil
	case ClearStatusMsg:
		if !strings.HasPrefix(m.status, "Downloading") {
			m.status = ""
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m Model) handleBucketsPageMsg(msg BucketsPageMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.errorsList = append(m.errorsList, msg.Err)
		m.loading = false
		m.bgJobs--
		if m.bgJobs < 0 { m.bgJobs = 0 }
		delete(m.loadingProjects, msg.ProjectID)
		return m, nil
	}
	
	// Find if project already exists in m.projects
	found := false
	for i, p := range m.projects {
		if p.ProjectID == msg.ProjectID {
			m.projects[i].Buckets = append(m.projects[i].Buckets, msg.Buckets...)
			found = true
			break
		}
	}
	
	if !found {
		// Maintain order from m.projectIDs if possible, or just append
		m.projects = append(m.projects, gcs.ProjectBuckets{
			ProjectID: msg.ProjectID,
			Buckets:   msg.Buckets,
		})
		// Reorder m.projects to match m.projectIDs
		var ordered []gcs.ProjectBuckets
		for _, id := range m.projectIDs {
			for _, p := range m.projects {
				if p.ProjectID == id {
					ordered = append(ordered, p)
					break
				}
			}
		}
		m.projects = ordered
	}

	var cmd tea.Cmd
	if msg.NextToken != "" {
		cmd = m.fetchBucketsPage(msg.ProjectID, msg.NextToken)
	} else {
		// Only stop loading if all projects are fully loaded?
		// For a lazy UI, we can turn off loading immediately, 
		// or maintain a map of loading projects.
		// For simplicity, let's turn it off when we get any page 
		// so the UI feels fast, or wait until all are done.
		// Let's just turn it off immediately so buckets appear ASAP.
		m.loading = false
		m.bgJobs--
		if m.bgJobs < 0 { m.bgJobs = 0 }
		delete(m.loadingProjects, msg.ProjectID)
	}
	// On first successful page, ensure loading screen hides
	m.loading = false
	return m, cmd
}

func (m Model) handleObjectsMsg(msg ObjectsMsg) (tea.Model, tea.Cmd) {
	if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
		m.bgJobs--
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
		return m, nil
	}
	m.loading = false
	m.bgJobs--
	if m.bgJobs < 0 {
		m.bgJobs = 0
	}
	if msg.Err != nil {
		m.errorsList = append(m.errorsList, msg.Err)
		return m, nil
	}

	cacheKey := msg.Bucket + "::" + msg.Prefix
	m.listCache[cacheKey] = listCacheEntry{List: msg.List, ExpiresAt: time.Now().Add(5 * time.Minute)}

	m.objects = msg.List.Objects
	m.prefixes = msg.List.Prefixes
	m.cursor = 0

	// Restore cursor if we just navigated back from a prefix
	if m.targetPrefixCursor != "" {
		for i, p := range m.prefixes {
			if p.Name == m.targetPrefixCursor {
				m.cursor = i
				break
			}
		}
		m.targetPrefixCursor = "" // Clear it after use
	}

	var cmd tea.Cmd
	if len(m.prefixes) > 0 {
		// Fetch metadata for the current cursor (either 0 or restored)
		m, cmd = m.triggerDebounces(m.fetchPrefixMetadataByName(m.prefixes[m.cursor].Name, m.cursor), m.currentBucket, m.prefixes[m.cursor].Name)
	} else if len(m.objects) > 0 {
		m.previewContent = "Loading..."
		m, cmd = m.triggerDebounces(m.fetchContent(m.currentBucket, m.objects[0].Name), "", "")
	}
	return m, cmd
}

func (m Model) handleObjectsPageMsg(msg ObjectsPageMsg) (tea.Model, tea.Cmd) {
	if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
		m.bgJobs--
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
		return m, nil
	}

	if msg.Err != nil {
		m.errorsList = append(m.errorsList, msg.Err)
		m.loading = false
		m.bgJobs--
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
		return m, nil
	}

	isFirstPage := len(m.objects) == 0 && len(m.prefixes) == 0

	m.objects = append(m.objects, msg.List.Objects...)
	m.prefixes = append(m.prefixes, msg.List.Prefixes...)

	var cmd tea.Cmd
	if isFirstPage {
		m.cursor = 0
		// Restore cursor if we just navigated back from a prefix
		if m.targetPrefixCursor != "" {
			for i, p := range m.prefixes {
				if p.Name == m.targetPrefixCursor {
					m.cursor = i
					break
				}
			}
			m.targetPrefixCursor = "" // Clear it after use
		}

		if len(m.prefixes) > 0 {
			// Fetch metadata for the current cursor (either 0 or restored)
			m, cmd = m.triggerDebounces(m.fetchPrefixMetadataByName(m.prefixes[m.cursor].Name, m.cursor), m.currentBucket, m.prefixes[m.cursor].Name)
		} else if len(m.objects) > 0 {
			m.previewContent = "Loading..."
			m, cmd = m.triggerDebounces(m.fetchContent(m.currentBucket, m.objects[0].Name), "", "")
		}
	}

	if msg.NextToken != "" {
		// Still loading more
		var batch []tea.Cmd
		if cmd != nil {
			batch = append(batch, cmd)
		}
		batch = append(batch, m.fetchObjectsPage(msg.Bucket, msg.Prefix, msg.NextToken))
		return m, tea.Batch(batch...)
	}

	// Loading complete
	m.loading = false
	m.bgJobs--
	if m.bgJobs < 0 { m.bgJobs = 0 }
	cacheKey := msg.Bucket + "::" + msg.Prefix
	fullList := &gcs.ObjectList{
		Objects:  m.objects,
		Prefixes: m.prefixes,
	}
	m.listCache[cacheKey] = listCacheEntry{List: fullList, ExpiresAt: time.Now().Add(5 * time.Minute)}

	return m, cmd
}

func (m Model) handleMetadataMsg(msg MetadataMsg) (tea.Model, tea.Cmd) {
	if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
		return m, nil
	}
	if msg.PrefixIndex >= 0 && msg.PrefixIndex < len(m.prefixes) {
		m.prefixes[msg.PrefixIndex].Fetched = true
		m.prefixes[msg.PrefixIndex].Err = msg.Err
		if msg.Err == nil {
			m.prefixes[msg.PrefixIndex].Created = msg.Metadata.Created
			m.prefixes[msg.PrefixIndex].Updated = msg.Metadata.Updated
			m.prefixes[msg.PrefixIndex].Owner = msg.Metadata.Owner

			cacheKey := msg.Bucket + "::" + msg.Metadata.Name
			m.metadataCache[cacheKey] = metadataCacheEntry{Metadata: msg.Metadata, ExpiresAt: time.Now().Add(5 * time.Minute)}
		}
	}
	return m, nil
}

func (m Model) handleContentMsg(msg ContentMsg) (tea.Model, tea.Cmd) {
	// Make sure the content is for the currently selected object (respecting filters)
	if m.state == viewObjects {
		currentPrefixes, currentObjects, _ := m.filteredObjects()
		if m.cursor >= len(currentPrefixes) {
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) && currentObjects[idx].Name == msg.ObjectName {
				if msg.Err != nil {
					m.previewContent = fmt.Sprintf("Error: %v", msg.Err)
				} else {
					m.previewContent = msg.Content
					cacheKey := m.currentBucket + "::" + msg.ObjectName
					m.contentCache[cacheKey] = contentCacheEntry{Content: msg.Content, ExpiresAt: time.Now().Add(5 * time.Minute)}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleDownloadMsg(msg DownloadMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.status = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("Download failed: %v", msg.Err))
	} else {
		m.downloadFinished++
		if len(m.downloadQueue) == 0 && m.downloadTotal > 1 {
			m.status = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("Downloaded %d files", m.downloadTotal))
		} else if len(m.downloadQueue) == 0 {
			m.status = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("Downloaded to %s", msg.Path))
		}
	}

	if len(m.downloadQueue) > 0 {
		m, cmd := m.processDownloadQueue()
		return m, cmd
	}
	return m, clearStatusCmd()
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		switch {
		case key.Matches(msg, keys.Help), key.Matches(msg, keys.Quit), msg.String() == "esc":
			m.showHelp = false
		}
		return m, nil
	}

	if m.showErrors {
		switch {
		case key.Matches(msg, keys.Errors), key.Matches(msg, keys.Quit), msg.String() == "esc":
			m.showErrors = false
		}
		return m, nil
	}

	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	if m.state == viewDownloadConfirm {
		return m.handleDownloadConfirmKey(msg)
	}

	switch {
	case key.Matches(msg, keys.Help):
		m.showHelp = true
		return m, nil

	case key.Matches(msg, keys.Errors):
		if len(m.errorsList) > 0 {
			m.showErrors = true
		}
		return m, nil

	case key.Matches(msg, keys.Search):
		m.searchMode = true
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.cursor = 0
		}
		return m, nil

	case key.Matches(msg, keys.Select):
		return m.handleSelectKey()

	case key.Matches(msg, keys.Down):
		return m.handleDownKey()

	case key.Matches(msg, keys.HalfPageDown):
		return m.handleHalfPageDownKey()

	case key.Matches(msg, keys.Up):
		return m.handleUpKey()

	case key.Matches(msg, keys.HalfPageUp):
		return m.handleHalfPageUpKey()

	case key.Matches(msg, keys.Right):
		return m.handleRightKey()

	case key.Matches(msg, keys.Left):
		return m.handleLeftKey()

	case key.Matches(msg, keys.Download):
		return m.handleDownloadKey()

	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	oldQuery := m.searchQuery
	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m.searchQuery = ""
		m.cursor = 0
	case tea.KeyEnter:
		m.searchMode = false
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.cursor = 0
		}
	case tea.KeyRunes, tea.KeySpace:
		m.searchQuery += msg.String()
		m.cursor = 0
	}

	if oldQuery != m.searchQuery && m.state == viewObjects {
		currentPrefixes, currentObjects, origIndices := m.filteredObjects()
		if m.cursor < len(currentPrefixes) {
			origIdx := origIndices[m.cursor]
			if !m.prefixes[origIdx].Fetched {
				return m.triggerDebounces(m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx), m.currentBucket, currentPrefixes[m.cursor].Name)
			}
			return m.triggerDebounces(nil, m.currentBucket, currentPrefixes[m.cursor].Name)
		} else if m.cursor >= len(currentPrefixes) {
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				obj := currentObjects[idx]
				m.previewContent = "Loading..."
				return m.triggerDebounces(m.fetchContent(m.currentBucket, obj.Name), "", "")
			}
		}
	}
	return m, nil
}

func (m Model) handleDownloadConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "o":
		m.status = "Downloading (overwriting)..."
		m.state = viewObjects
		return m, m.fetchDownload(m.pendingDownloadBucket, m.pendingDownloadObject, m.pendingDownloadDest, m.pendingDownloadIsPrefix)
	case "a", "q", "ctrl+c", "esc":
		m.status = "Download aborted."
		m.state = viewObjects
		m.downloadQueue = nil // Clear the rest of the queue
		return m, nil
	case "r":
		newDest, err := autoRename(m.pendingDownloadDest)
		if err != nil {
			m.status = fmt.Sprintf("Rename failed: %v", err)
			m.state = viewObjects
			m.downloadQueue = nil
			return m, nil
		}
		m.status = fmt.Sprintf("Downloading as %s...", filepath.Base(newDest))
		m.state = viewObjects
		return m, m.fetchDownload(m.pendingDownloadBucket, m.pendingDownloadObject, newDest, m.pendingDownloadIsPrefix)
	}
	return m, nil
}

func (m Model) handleSelectKey() (tea.Model, tea.Cmd) {
	if m.state == viewObjects {
		currentPrefixes, currentObjects, _ := m.filteredObjects()
		if m.cursor < len(currentPrefixes) {
			name := currentPrefixes[m.cursor].Name
			if _, ok := m.selected[name]; ok {
				delete(m.selected, name)
			} else {
				m.selected[name] = struct{}{}
			}
		} else if m.cursor >= len(currentPrefixes) {
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) {
				name := currentObjects[idx].Name
				if _, ok := m.selected[name]; ok {
					delete(m.selected, name)
				} else {
					m.selected[name] = struct{}{}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleDownKey() (tea.Model, tea.Cmd) {
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}

	var itemsCount int
	var currentPrefixes []gcs.PrefixMetadata
	var currentObjects []gcs.ObjectMetadata
	var origIndices []int

	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		currentPrefixes, currentObjects, origIndices = m.filteredObjects()
		itemsCount = len(currentObjects) + len(currentPrefixes)
	}

	if itemsCount > 0 {
		oldCursor := m.cursor
		m.cursor = (m.cursor + 1) % itemsCount
		if oldCursor != m.cursor {
			m.previewContent = "" // Reset preview on move
			switch m.state {
			case viewObjects:
				if m.cursor < len(currentPrefixes) {
					origIdx := origIndices[m.cursor]
					if !m.prefixes[origIdx].Fetched {
						return m.triggerDebounces(m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx), m.currentBucket, currentPrefixes[m.cursor].Name)
					}
					return m.triggerDebounces(nil, m.currentBucket, currentPrefixes[m.cursor].Name)
				} else if m.cursor >= len(currentPrefixes) {
					idx := m.cursor - len(currentPrefixes)
					obj := currentObjects[idx]
					m.previewContent = "Loading..."
					return m.triggerDebounces(m.fetchContent(m.currentBucket, obj.Name), "", "")
				}
			case viewBuckets:
				filtered := m.filteredBuckets()
				if m.cursor < len(filtered) {
					item := filtered[m.cursor]
					if !item.IsProject {
						return m.triggerDebounces(nil, item.BucketName, "")
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleUpKey() (tea.Model, tea.Cmd) {
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}

	var itemsCount int
	var currentPrefixes []gcs.PrefixMetadata
	var currentObjects []gcs.ObjectMetadata
	var origIndices []int

	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		currentPrefixes, currentObjects, origIndices = m.filteredObjects()
		itemsCount = len(currentObjects) + len(currentPrefixes)
	}

	if itemsCount > 0 {
		oldCursor := m.cursor
		m.cursor = (m.cursor - 1 + itemsCount) % itemsCount
		if oldCursor != m.cursor {
			m.previewContent = "" // Reset preview on move
			switch m.state {
			case viewObjects:
				if m.cursor < len(currentPrefixes) {
					origIdx := origIndices[m.cursor]
					if !m.prefixes[origIdx].Fetched {
						return m.triggerDebounces(m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx), m.currentBucket, currentPrefixes[m.cursor].Name)
					}
					return m.triggerDebounces(nil, m.currentBucket, currentPrefixes[m.cursor].Name)
				} else if m.cursor >= len(currentPrefixes) {
					idx := m.cursor - len(currentPrefixes)
					obj := currentObjects[idx]
					m.previewContent = "Loading..."
					return m.triggerDebounces(m.fetchContent(m.currentBucket, obj.Name), "", "")
				}
			case viewBuckets:
				filtered := m.filteredBuckets()
				if m.cursor < len(filtered) {
					item := filtered[m.cursor]
					if !item.IsProject {
						return m.triggerDebounces(nil, item.BucketName, "")
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleHalfPageDownKey() (tea.Model, tea.Cmd) {
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}

	var itemsCount int
	var currentPrefixes []gcs.PrefixMetadata
	var currentObjects []gcs.ObjectMetadata
	var origIndices []int

	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		currentPrefixes, currentObjects, origIndices = m.filteredObjects()
		itemsCount = len(currentObjects) + len(currentPrefixes)
	}

	if itemsCount > 0 {
		offset := m.maxItemsVisible() / 2
		if offset < 1 {
			offset = 1
		}
		oldCursor := m.cursor
		m.cursor += offset
		if m.cursor >= itemsCount {
			m.cursor = itemsCount - 1
		}
		if oldCursor != m.cursor {
			m.previewContent = "" // Reset preview on move
			switch m.state {
			case viewObjects:
				if m.cursor < len(currentPrefixes) {
					origIdx := origIndices[m.cursor]
					if !m.prefixes[origIdx].Fetched {
						return m.triggerDebounces(m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx), m.currentBucket, currentPrefixes[m.cursor].Name)
					}
					return m.triggerDebounces(nil, m.currentBucket, currentPrefixes[m.cursor].Name)
				} else if m.cursor >= len(currentPrefixes) {
					idx := m.cursor - len(currentPrefixes)
					obj := currentObjects[idx]
					m.previewContent = "Loading..."
					return m.triggerDebounces(m.fetchContent(m.currentBucket, obj.Name), "", "")
				}
			case viewBuckets:
				filtered := m.filteredBuckets()
				if m.cursor < len(filtered) {
					item := filtered[m.cursor]
					if !item.IsProject {
						return m.triggerDebounces(nil, item.BucketName, "")
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleHalfPageUpKey() (tea.Model, tea.Cmd) {
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}

	var itemsCount int
	var currentPrefixes []gcs.PrefixMetadata
	var currentObjects []gcs.ObjectMetadata
	var origIndices []int

	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		currentPrefixes, currentObjects, origIndices = m.filteredObjects()
		itemsCount = len(currentObjects) + len(currentPrefixes)
	}

	if itemsCount > 0 {
		offset := m.maxItemsVisible() / 2
		if offset < 1 {
			offset = 1
		}
		oldCursor := m.cursor
		m.cursor -= offset
		if m.cursor < 0 {
			m.cursor = 0
		}
		if oldCursor != m.cursor {
			m.previewContent = "" // Reset preview on move
			switch m.state {
			case viewObjects:
				if m.cursor < len(currentPrefixes) {
					origIdx := origIndices[m.cursor]
					if !m.prefixes[origIdx].Fetched {
						return m.triggerDebounces(m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx), m.currentBucket, currentPrefixes[m.cursor].Name)
					}
					return m.triggerDebounces(nil, m.currentBucket, currentPrefixes[m.cursor].Name)
				} else if m.cursor >= len(currentPrefixes) {
					idx := m.cursor - len(currentPrefixes)
					obj := currentObjects[idx]
					m.previewContent = "Loading..."
					return m.triggerDebounces(m.fetchContent(m.currentBucket, obj.Name), "", "")
				}
			case viewBuckets:
				filtered := m.filteredBuckets()
				if m.cursor < len(filtered) {
					item := filtered[m.cursor]
					if !item.IsProject {
						return m.triggerDebounces(nil, item.BucketName, "")
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleRightKey() (tea.Model, tea.Cmd) {
	switch m.state {
	case viewBuckets:
		filtered := m.filteredBuckets()
		if len(filtered) > 0 {
			item := filtered[m.cursor]

			if item.IsProject {
				// Toggle project expansion
				if _, ok := m.collapsedProjects[item.ProjectID]; ok {
					delete(m.collapsedProjects, item.ProjectID)
				} else {
					m.collapsedProjects[item.ProjectID] = struct{}{}
				}
				// Don't change state, just re-render
				return m, nil
			}

			m.currentBucket = item.BucketName

			// Save the index in the filtered list to restore later.
			m.bucketCursor = m.cursor

			m.currentPrefix = "" // Reset prefix when entering bucket
			m.state = viewObjects
			m.searchMode = false
			m.searchQuery = ""
			m = m.resetObjectsState()
			return m, m.fetchObjects()
		}
	case viewObjects:
		currentPrefixes, _, _ := m.filteredObjects()
		// Check if selected item is a prefix
		if m.cursor < len(currentPrefixes) {
			m.previewContent = ""
			m.currentPrefix = currentPrefixes[m.cursor].Name
			m.searchMode = false
			m.searchQuery = ""
			m = m.resetObjectsState()
			return m, m.fetchObjects()
		}
	}
	return m, nil
}

func (m Model) handleLeftKey() (tea.Model, tea.Cmd) {
	if m.state == viewBuckets {
		filtered := m.filteredBuckets()
		if len(filtered) > 0 {
			item := filtered[m.cursor]
			if item.IsProject {
				// Ensure project is collapsed
				m.collapsedProjects[item.ProjectID] = struct{}{}
			} else {
				// Optional: If on a bucket, 'h' could jump back up to the project header
				for i := m.cursor - 1; i >= 0; i-- {
					if filtered[i].IsProject && filtered[i].ProjectID == item.ProjectID {
						m.cursor = i
						break
					}
				}
			}
			return m, nil
		}
	} else if m.state == viewObjects {
		m.previewContent = ""
		m.searchMode = false
		m.searchQuery = ""
		if m.currentPrefix == "" {
			m.state = viewBuckets

			// Find the bucket in the current filtered view to restore cursor correctly
			filtered := m.filteredBuckets()
			m.cursor = 0
			for i, item := range filtered {
				if !item.IsProject && item.BucketName == m.currentBucket {
					m.cursor = i
					break
				}
			}

			m.currentBucket = ""
			m.loading = false
			return m, nil
		}

		// Save the current prefix so we can highlight it in the parent directory
		m.targetPrefixCursor = m.currentPrefix

		// Go up one level
		m.currentPrefix = parentPrefix(m.currentPrefix)
		m = m.resetObjectsState()
		return m, m.fetchObjects()
	}
	return m, nil
}

func (m Model) handleDownloadKey() (tea.Model, tea.Cmd) {
	if m.state == viewObjects {
		currentPrefixes, currentObjects, _ := m.filteredObjects()

		var toDownload []downloadTask
		if len(m.selected) > 0 {
			// Download all selected objects and prefixes
			for _, p := range m.prefixes {
				if _, ok := m.selected[p.Name]; ok {
					dest := filepath.Join(m.downloadDir, strings.TrimSuffix(filepath.Base(p.Name), "/")+".zip")
					toDownload = append(toDownload, downloadTask{bucket: m.currentBucket, object: p.Name, dest: dest, isPrefix: true})
				}
			}
			for _, obj := range m.objects {
				if _, ok := m.selected[obj.Name]; ok {
					dest := filepath.Join(m.downloadDir, filepath.Base(obj.Name))
					toDownload = append(toDownload, downloadTask{bucket: m.currentBucket, object: obj.Name, dest: dest, isPrefix: false})
				}
			}
		} else {
			// Fallback to downloading the currently highlighted item
			if m.cursor < len(currentPrefixes) {
				name := currentPrefixes[m.cursor].Name
				dest := filepath.Join(m.downloadDir, strings.TrimSuffix(filepath.Base(name), "/")+".zip")
				toDownload = append(toDownload, downloadTask{bucket: m.currentBucket, object: name, dest: dest, isPrefix: true})
			} else if m.cursor >= len(currentPrefixes) {
				idx := m.cursor - len(currentPrefixes)
				if idx < len(currentObjects) {
					name := currentObjects[idx].Name
					dest := filepath.Join(m.downloadDir, filepath.Base(name))
					toDownload = append(toDownload, downloadTask{bucket: m.currentBucket, object: name, dest: dest, isPrefix: false})
				}
			}
		}

		if len(toDownload) > 0 {
			m.downloadTotal = len(toDownload)
			m.downloadFinished = 0

			m.downloadQueue = append(m.downloadQueue, toDownload...)

			// Clear selection after triggering download
			m.selected = make(map[string]struct{})

			m, cmd := m.processDownloadQueue()
			return m, cmd
		}
	}
	return m, nil
}