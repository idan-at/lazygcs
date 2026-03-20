package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
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
	case FileOpenedMsg:
		if msg.Err != nil {
			statusCmd := m.AddMessage(LevelError, fmt.Sprintf("Error opening file: %v", msg.Err))
			return m, statusCmd
		}
		// The status is already set to "Opening...", which gets cleared naturally or we can clear it.
		return m, nil
	case EditorFinishedMsg:
		return m.handleEditorFinishedMsg(msg)
	case UploadMsg:
		return m.handleUploadMsg(msg)
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
		m.msgQueue.ClearStatusPill(msg.ID)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.previewRegistry.SetWidth(msg.Width / 3) // Approx width of preview col
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m Model) handleBucketsPageMsg(msg BucketsPageMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		cmd := m.AddMessage(LevelError, msg.Err.Error())
		m.loading = false
		m.bgJobs--
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
		delete(m.loadingProjects, msg.ProjectID)
		return m, cmd
	}

	// Find if project already exists in m.projects
	found := false
	for i, p := range m.projects {
		if p.ProjectID == msg.ProjectID {
			m.projects[i].Buckets = append(m.projects[i].Buckets, msg.Buckets...)
			sort.Strings(m.projects[i].Buckets)
			found = true
			break
		}
	}

	if !found {
		buckets := msg.Buckets
		sort.Strings(buckets)
		// Maintain order from m.projectIDs if possible, or just append
		m.projects = append(m.projects, gcs.ProjectBuckets{
			ProjectID: msg.ProjectID,
			Buckets:   buckets,
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

	// Always sort projects by their appearance in m.projectIDs or alphabetically
	// Since we already have the ordered logic above, we can just ensure m.projects
	// stays consistent.
	sort.Slice(m.projects, func(i, j int) bool {
		// Try to match the order in m.projectIDs
		iIdx, jIdx := -1, -1
		for idx, id := range m.projectIDs {
			if id == m.projects[i].ProjectID {
				iIdx = idx
			}
			if id == m.projects[j].ProjectID {
				jIdx = idx
			}
		}
		if iIdx != -1 && jIdx != -1 {
			return iIdx < jIdx
		}
		return m.projects[i].ProjectID < m.projects[j].ProjectID
	})

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
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
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
		cmd := m.AddMessage(LevelError, msg.Err.Error())
		return m, cmd
	}

	cacheKey := msg.Bucket + "::" + msg.Prefix
	m.listCache[cacheKey] = listCacheEntry{List: msg.List, ExpiresAt: time.Now().Add(5 * time.Minute)}

	m.objects = msg.List.Objects
	sort.Slice(m.objects, func(i, j int) bool {
		return m.objects[i].Name < m.objects[j].Name
	})
	m.prefixes = msg.List.Prefixes
	sort.Slice(m.prefixes, func(i, j int) bool {
		return m.prefixes[i].Name < m.prefixes[j].Name
	})
	m.cursor = 0

	// Restore cursor if we just navigated back from a prefix or refreshed an object
	if m.targetPrefixCursor != "" {
		found := false
		for i, p := range m.prefixes {
			if p.Name == m.targetPrefixCursor {
				m.cursor = i
				found = true
				break
			}
		}
		if !found {
			for i, o := range m.objects {
				if o.Name == m.targetPrefixCursor {
					m.cursor = len(m.prefixes) + i
					break
				}
			}
		}
		m.targetPrefixCursor = "" // Clear it after use
	}

	var cmd tea.Cmd
	if m.cursor < len(m.prefixes) {
		// Fetch metadata for the current cursor (either 0 or restored)
		m, cmd = m.triggerDebounces(m.fetchPrefixMetadataByName(m.prefixes[m.cursor].Name, m.cursor), m.currentBucket, m.prefixes[m.cursor].Name)
	} else if m.cursor-len(m.prefixes) < len(m.objects) {
		m.previewContent = "\x1b_Ga=d,d=A\x1b\\Loading..."
		m, cmd = m.triggerDebounces(m.fetchContent(m.objects[m.cursor-len(m.prefixes)]), "", "")
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
		cmd := m.AddMessage(LevelError, msg.Err.Error())
		m.loading = false
		m.bgJobs--
		if m.bgJobs < 0 {
			m.bgJobs = 0
		}
		return m, cmd
	}

	isFirstPage := len(m.objects) == 0 && len(m.prefixes) == 0

	m.objects = append(m.objects, msg.List.Objects...)
	sort.Slice(m.objects, func(i, j int) bool {
		return m.objects[i].Name < m.objects[j].Name
	})
	m.prefixes = append(m.prefixes, msg.List.Prefixes...)
	sort.Slice(m.prefixes, func(i, j int) bool {
		return m.prefixes[i].Name < m.prefixes[j].Name
	})

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
			m.previewContent = "\x1b_Ga=d,d=A\x1b\\Loading..."
			m, cmd = m.triggerDebounces(m.fetchContent(m.objects[0]), "", "")
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
	if m.bgJobs < 0 {
		m.bgJobs = 0
	}
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
	var cmd tea.Cmd
	// Make sure the content is for the currently selected object (respecting filters)
	if m.state == viewObjects {
		currentPrefixes, currentObjects, _ := m.filteredObjects()
		if m.cursor >= len(currentPrefixes) {
			idx := m.cursor - len(currentPrefixes)
			if idx < len(currentObjects) && currentObjects[idx].Name == msg.ObjectName {
				if msg.Err != nil {
					if msg.Content == "" {
						m.previewContent = fmt.Sprintf("Error: %v", msg.Err)
					} else {
						m.previewContent = msg.Content // Show whatever content we got (fallback)
					}
					cmd = m.AddMessage(LevelError, msg.Err.Error())
				} else {
					m.previewContent = msg.Content
					cacheKey := m.currentBucket + "::" + msg.ObjectName
					m.contentCache[cacheKey] = contentCacheEntry{Content: msg.Content, ExpiresAt: time.Now().Add(5 * time.Minute)}
				}
			}
		}
	}
	return m, cmd
}

func (m Model) handleDownloadMsg(msg DownloadMsg) (tea.Model, tea.Cmd) {
	delete(m.activeTasks, msg.TaskID)
	var cmd tea.Cmd
	if msg.Err != nil {
		cmd = m.AddMessage(LevelError, fmt.Sprintf("Download failed: %v", msg.Err))
	} else {
		m.downloadFinished++
		if len(m.downloadQueue) == 0 && m.downloadTotal > 1 {
			cmd = m.AddMessage(LevelInfo, fmt.Sprintf("Downloaded %d files", m.downloadTotal))
		} else if len(m.downloadQueue) == 0 {
			cmd = m.AddMessage(LevelInfo, fmt.Sprintf("Downloaded to %s", msg.Path))
		}
	}

	if len(m.downloadQueue) > 0 {
		m, nextCmd := m.processDownloadQueue()
		return m, tea.Batch(cmd, nextCmd)
	}
	return m, cmd
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		switch {
		case key.Matches(msg, keys.Help), key.Matches(msg, keys.Quit), key.Matches(msg, keys.Esc):
			m.showHelp = false
		}
		return m, nil
	}

	if m.showMessages {
		switch {
		case key.Matches(msg, keys.Messages), key.Matches(msg, keys.Quit), key.Matches(msg, keys.Esc):
			m.showMessages = false
		case key.Matches(msg, keys.Up):
			m.msgQueue.MessagesScroll--
			if m.msgQueue.MessagesScroll < 0 {
				m.msgQueue.MessagesScroll = 0
			}
		case key.Matches(msg, keys.Down):
			m.msgQueue.MessagesScroll++
			maxScroll := len(m.msgQueue.Messages()) - 15
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.msgQueue.MessagesScroll > maxScroll {
				m.msgQueue.MessagesScroll = maxScroll
			}
		case key.Matches(msg, keys.PageUp), key.Matches(msg, keys.HalfPageUp):
			m.msgQueue.MessagesScroll -= 15
			if m.msgQueue.MessagesScroll < 0 {
				m.msgQueue.MessagesScroll = 0
			}
		case key.Matches(msg, keys.PageDown), key.Matches(msg, keys.HalfPageDown):
			m.msgQueue.MessagesScroll += 15
			maxScroll := len(m.msgQueue.Messages()) - 15
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.msgQueue.MessagesScroll > maxScroll {
				m.msgQueue.MessagesScroll = maxScroll
			}
		case key.Matches(msg, keys.Top):
			m.msgQueue.MessagesScroll = 0
		case key.Matches(msg, keys.Bottom):
			maxScroll := len(m.msgQueue.Messages()) - 15
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.msgQueue.MessagesScroll = maxScroll
		}
		return m, nil
	}

	if key.Matches(msg, keys.Esc) && !m.searchMode {
		if m.state == viewObjects && m.objectSearchQuery != "" {
			m.objectSearchQuery = ""
			m.cursor = 0
			return m, nil
		} else if m.state == viewObjects && m.bucketSearchQuery != "" {
			m.bucketSearchQuery = ""
			return m, nil
		} else if m.state == viewBuckets && m.bucketSearchQuery != "" {
			m.bucketSearchQuery = ""
			m.cursor = 0
			return m, nil
		}
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

	case key.Matches(msg, keys.Messages):
		m.showMessages = true
		m.msgQueue.MessagesScroll = len(m.msgQueue.Messages()) - 15
		if m.msgQueue.MessagesScroll < 0 {
			m.msgQueue.MessagesScroll = 0
		}
		return m, nil

	case key.Matches(msg, keys.Search):
		m.searchMode = true
		if m.currentSearchQuery() != "" {
			m = m.withCurrentSearchQuery("")
			m.cursor = 0
		}
		return m, nil

	case key.Matches(msg, keys.Select):
		return m.handleSelectKey()

	case key.Matches(msg, keys.Top):
		return m.handleTopKey()

	case key.Matches(msg, keys.Bottom):
		return m.handleBottomKey()

	case key.Matches(msg, keys.Down):
		return m.handleDownKey()

	case key.Matches(msg, keys.PageDown):
		return m.handlePageDownKey()

	case key.Matches(msg, keys.HalfPageDown):
		return m.handleHalfPageDownKey()

	case key.Matches(msg, keys.Up):
		return m.handleUpKey()

	case key.Matches(msg, keys.PageUp):
		return m.handlePageUpKey()

	case key.Matches(msg, keys.HalfPageUp):
		return m.handleHalfPageUpKey()

	case key.Matches(msg, keys.Right):
		return m.handleRightKey()

	case key.Matches(msg, keys.Left):
		return m.handleLeftKey()

	case key.Matches(msg, keys.Home):
		return m.handleHomeKey()

	case key.Matches(msg, keys.Download):
		return m.handleDownloadKey()

	case key.Matches(msg, keys.Open):
		return m.handleOpenKey()

	case key.Matches(msg, keys.Edit):
		return m.handleEditKey()

	case key.Matches(msg, keys.Copy):
		return m.handleCopyKey()

	case key.Matches(msg, keys.Refresh):
		return m.handleRefreshKey(false)

	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	oldQuery := m.currentSearchQuery()
	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m = m.withCurrentSearchQuery("")
		m.cursor = 0
	case tea.KeyEnter:
		m.searchMode = false
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		q := m.currentSearchQuery()
		if len(q) > 0 {
			runes := []rune(q)
			m = m.withCurrentSearchQuery(string(runes[:len(runes)-1]))
			m.cursor = 0
		}
	case tea.KeyRunes, tea.KeySpace:
		m = m.withCurrentSearchQuery(m.currentSearchQuery() + msg.String())
		m.cursor = 0
	}

	if oldQuery != m.currentSearchQuery() && m.state == viewObjects {
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
				m.previewContent = "\x1b_Ga=d,d=A\x1b\\Loading..."
				return m.triggerDebounces(m.fetchContent(obj), "", "")
			}
		}
	}
	return m, nil
}

func (m Model) handleDownloadConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "o":
		m.state = viewObjects
		return m.startDownloadTask(m.pendingDownloadDest)
	case "a", "q", "ctrl+c", "esc":
		cmd := m.AddMessage(LevelInfo, "Download aborted.")
		m.state = viewObjects
		m.downloadQueue = nil // Clear the rest of the queue
		return m, cmd
	case "r":
		newDest, err := autoRename(m.pendingDownloadDest)
		if err != nil {
			cmd := m.AddMessage(LevelError, fmt.Sprintf("Rename failed: %v", err))
			m.state = viewObjects
			m.downloadQueue = nil
			return m, cmd
		}
		m.state = viewObjects
		return m.startDownloadTask(newDest)
	default:
		return m, BeepCmd
	}
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

func (m Model) finalizeCursorMove(oldCursor int) (tea.Model, tea.Cmd) {
	if oldCursor == m.cursor {
		return m, nil
	}

	m.previewContent = "\x1b_Ga=d,d=A\x1b\\" // Reset preview on move
	switch m.state {
	case viewObjects:
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
				m.previewContent = "\x1b_Ga=d,d=A\x1b\\Loading..."
				return m.triggerDebounces(m.fetchContent(obj), "", "")
			}
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
	return m, nil
}

func (m Model) handleDownKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
	}

	if itemsCount > 0 {
		oldCursor := m.cursor
		m.cursor = (m.cursor + 1) % itemsCount
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handleUpKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
	}

	if itemsCount > 0 {
		oldCursor := m.cursor
		m.cursor = (m.cursor - 1 + itemsCount) % itemsCount
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handleHalfPageDownKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
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
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handleHalfPageUpKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
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
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handlePageDownKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
	}

	if itemsCount > 0 {
		offset := m.maxItemsVisible()
		if offset < 1 {
			offset = 1
		}
		oldCursor := m.cursor
		m.cursor += offset
		if m.cursor >= itemsCount {
			m.cursor = itemsCount - 1
		}
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handlePageUpKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
	}

	if itemsCount > 0 {
		offset := m.maxItemsVisible()
		if offset < 1 {
			offset = 1
		}
		oldCursor := m.cursor
		m.cursor -= offset
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handleTopKey() (tea.Model, tea.Cmd) {

	oldCursor := m.cursor
	m.cursor = 0
	return m.finalizeCursorMove(oldCursor)
}

func (m Model) handleBottomKey() (tea.Model, tea.Cmd) {

	var itemsCount int
	switch m.state {
	case viewBuckets:
		itemsCount = len(m.filteredBuckets())
	case viewObjects:
		cp, co, _ := m.filteredObjects()
		itemsCount = len(cp) + len(co)
	}

	if itemsCount > 0 {
		oldCursor := m.cursor
		m.cursor = itemsCount - 1
		return m.finalizeCursorMove(oldCursor)
	}
	return m, nil
}

func (m Model) handleRefreshKey(silent bool) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if !silent {
		cmd = m.AddMessage(LevelInfo, "Refreshing...")
	}

	switch m.state {
	case viewBuckets:
		m.projects = nil
		m.bgJobs = len(m.projectIDs)
		m.loadingProjects = make(map[string]bool)
		for _, id := range m.projectIDs {
			m.loadingProjects[id] = true
		}
		var cmds []tea.Cmd
		cmds = append(cmds, cmd)
		for _, pID := range m.projectIDs {
			cmds = append(cmds, m.fetchBucketsPage(pID, ""))
		}
		return m, tea.Batch(cmds...)
	case viewObjects:
		m.loading = true
		m.bgJobs++
		m.objects = nil
		m.prefixes = nil
		// Invalidate cache for current prefix
		cacheKey := m.currentBucket + "::" + m.currentPrefix
		delete(m.listCache, cacheKey)
		return m, tea.Batch(cmd, m.fetchObjects())
	}
	return m, cmd
}

func (m Model) handleCopyKey() (tea.Model, tea.Cmd) {
	var uris []string

	switch m.state {
	case viewBuckets:
		filtered := m.filteredBuckets()
		if m.cursor < len(filtered) {
			item := filtered[m.cursor]
			if !item.IsProject {
				uris = append(uris, "gs://"+item.BucketName+"/")
			}
		}
	case viewObjects:
		currentPrefixes, currentObjects, _ := m.filteredObjects()

		if len(m.selected) > 1 {
			statusCmd := m.AddMessage(LevelError, "Cannot copy multiple files at once")
			return m, statusCmd
		}

		// If there is a single selection, copy it
		if len(m.selected) == 1 {
			for name := range m.selected {
				uris = append(uris, "gs://"+m.currentBucket+"/"+name)
			}
		} else {
			// Copy the hovered item
			if m.cursor < len(currentPrefixes) {
				uris = append(uris, "gs://"+m.currentBucket+"/"+currentPrefixes[m.cursor].Name)
			} else if m.cursor >= len(currentPrefixes) {
				idx := m.cursor - len(currentPrefixes)
				if idx < len(currentObjects) {
					uris = append(uris, "gs://"+m.currentBucket+"/"+currentObjects[idx].Name)
				}
			}
		}
	}

	if len(uris) == 0 {
		return m, nil
	}

	content := strings.Join(uris, "\n")
	err := m.clipboard.WriteAll(content)
	if err != nil {
		cmd := m.AddMessage(LevelInfo, fmt.Sprintf("Clipboard error: %v", err))
		return m, cmd
	}

	var cmd tea.Cmd
	if len(uris) == 1 {
		cmd = m.AddMessage(LevelInfo, fmt.Sprintf("Copied %s to clipboard", uris[0]))
	} else {
		cmd = m.AddMessage(LevelInfo, fmt.Sprintf("Copied %d URIs to clipboard", len(uris)))
	}

	return m, cmd
}

func (m Model) handleHomeKey() (tea.Model, tea.Cmd) {
	if m.state == viewBuckets {
		return m, nil
	}
	m.state = viewBuckets
	m.currentBucket = ""
	m.currentPrefix = ""
	m.objects = nil
	m.prefixes = nil
	m.previewContent = ""
	m.cursor = m.bucketCursor
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
			m.objectSearchQuery = ""
			m = m.resetObjectsState()
			return m, m.fetchObjects()
		}
	case viewObjects:
		currentPrefixes, _, _ := m.filteredObjects()
		// Check if selected item is a prefix
		if m.cursor < len(currentPrefixes) {
			m.previewContent = "\x1b_Ga=d,d=A\x1b\\"
			m.currentPrefix = currentPrefixes[m.cursor].Name
			m.searchMode = false
			m.objectSearchQuery = ""
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
		m.previewContent = "\x1b_Ga=d,d=A\x1b\\"
		m.searchMode = false
		m.objectSearchQuery = ""
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

type targetError int

const (
	targetErrNone targetError = iota
	targetErrMultiple
	targetErrDirectory
	targetErrNotSelected
)

func (m Model) getSingleTargetObject() (string, targetError) {
	if len(m.selected) > 1 {
		return "", targetErrMultiple
	}

	if len(m.selected) == 1 {
		var targetName string
		for name := range m.selected {
			targetName = name
		}
		if strings.HasSuffix(targetName, "/") {
			return "", targetErrDirectory
		}
		return targetName, targetErrNone
	}

	currentPrefixes, currentObjects, _ := m.filteredObjects()
	if m.cursor < len(currentPrefixes) {
		return "", targetErrDirectory
	}
	if m.cursor >= len(currentPrefixes)+len(currentObjects) {
		return "", targetErrNotSelected
	}
	return currentObjects[m.cursor-len(currentPrefixes)].Name, targetErrNone
}

func (m Model) handleOpenKey() (tea.Model, tea.Cmd) {
	if m.state != viewObjects {
		return m, nil
	}

	targetName, err := m.getSingleTargetObject()
	switch err {
	case targetErrMultiple:
		statusCmd := m.AddMessage(LevelError, "Cannot open multiple files at once")
		return m, statusCmd
	case targetErrDirectory:
		statusCmd := m.AddMessage(LevelError, "Cannot open a directory")
		return m, statusCmd
	case targetErrNotSelected:
		return m, nil
	}

	statusCmd := m.AddMessage(LevelInfo, fmt.Sprintf("Opening %s...", filepath.Base(targetName)))
	return m, tea.Batch(m.openFile(m.currentBucket, targetName), statusCmd)
}

func (m Model) handleEditKey() (tea.Model, tea.Cmd) {
	if m.state != viewObjects {
		return m, nil
	}

	targetName, err := m.getSingleTargetObject()
	switch err {
	case targetErrMultiple:
		statusCmd := m.AddMessage(LevelError, "Cannot edit multiple files at once")
		return m, statusCmd
	case targetErrDirectory:
		statusCmd := m.AddMessage(LevelError, "Cannot edit a directory")
		return m, statusCmd
	case targetErrNotSelected:
		return m, nil
	}

	cmd := m.AddMessage(LevelInfo, fmt.Sprintf("Opening %s...", filepath.Base(targetName)))
	// Save target name for re-upload
	m.targetPrefixCursor = targetName
	return m, tea.Batch(cmd, m.editFile(m.currentBucket, targetName))
}

func (m Model) handleEditorFinishedMsg(msg EditorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		statusCmd := m.AddMessage(LevelInfo, fmt.Sprintf("Editor error: %v", msg.Err))
		return m, statusCmd
	}

	info, err := os.Stat(msg.TempPath)
	if err != nil {
		statusCmd := m.AddMessage(LevelError, fmt.Sprintf("Error checking file: %v", err))
		return m, statusCmd
	}

	if info.ModTime().After(msg.OriginalModTime) {
		cmd := m.AddMessage(LevelInfo, fmt.Sprintf("Uploading changes to %s...", filepath.Base(msg.TempPath)))
		// m.targetPrefixCursor stores the object name from handleEditKey
		return m, tea.Batch(cmd, m.uploadFile(m.currentBucket, m.targetPrefixCursor, msg.TempPath))
	}

	statusCmd := m.AddMessage(LevelInfo, "No changes made")
	return m, statusCmd
}

func (m Model) handleUploadMsg(msg UploadMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.Err != nil {
		cmd = m.AddMessage(LevelError, fmt.Sprintf("Upload failed: %v", msg.Err))
	} else {
		cmd = m.AddMessage(LevelInfo, fmt.Sprintf("Uploaded %s", filepath.Base(msg.ObjectName)))
		cacheKey := m.currentBucket + "::" + msg.ObjectName
		delete(m.contentCache, cacheKey)
		delete(m.metadataCache, cacheKey)
		// Refresh view to show updated metadata (size/time)
		var refreshCmd tea.Cmd
		var newModel tea.Model
		newModel, refreshCmd = m.handleRefreshKey(true)
		return newModel, tea.Batch(cmd, refreshCmd)
	}
	return m, cmd
}
