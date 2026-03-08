package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"lazygcs/internal/gcs"
)

type viewState int

const (
	viewBuckets viewState = iota
	viewObjects
	viewDownloadConfirm
)

type downloadTask struct {
	bucket   string
	object   string
	dest     string
	isPrefix bool
}

type BucketListItem struct {
	IsProject  bool
	ProjectID  string
	BucketName string
}

// Model maintains the state of the TUI application.
type Model struct {
	client      GCSClient
	projectIDs  []string
	downloadDir string

	// View State
	width          int
	height         int
	state          viewState
	previewContent string
	showHelp       bool

	// Search State
	searchMode  bool
	searchQuery string
	fuzzySearch bool

	// Settings
	showIcons bool

	// Download Confirm State
	pendingDownloadBucket   string
	pendingDownloadObject   string
	pendingDownloadDest     string
	pendingDownloadIsPrefix bool
	downloadQueue           []downloadTask
	downloadTotal           int
	downloadFinished        int

	// Buckets View
	projects          []gcs.ProjectBuckets
	collapsedProjects map[string]struct{}
	cursor            int // used for buckets or objects depending on state
	bucketCursor      int // stores the cursor position in the bucket list

	// Objects View
	currentBucket      string
	currentPrefix      string
	targetPrefixCursor string
	objects            []gcs.ObjectMetadata
	prefixes           []gcs.PrefixMetadata
	selected           map[string]struct{}

	loading bool
	status  string
	err     error
	help    help.Model
}

// NewModel creates a Model initialized with the provided projects and GCS client.
func NewModel(projectIDs []string, client GCSClient, downloadDir string, fuzzySearch bool, showIcons bool) Model {
	return Model{
		projectIDs:        projectIDs,
		client:            client,
		downloadDir:       downloadDir,
		fuzzySearch:       fuzzySearch,
		showIcons:         showIcons,
		width:             120,
		height:            40,
		state:             viewBuckets,
		loading:           true,
		selected:          make(map[string]struct{}),
		collapsedProjects: make(map[string]struct{}),
		help:              help.New(),
	}
}

// Init initializes the application by triggering the first bucket fetch.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		projects, err := m.client.ListBuckets(context.Background(), m.projectIDs)
		return BucketsMsg{Projects: projects, Err: err}
	}
}

func (m Model) fetchObjects() tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix
	return func() tea.Msg {
		list, err := m.client.ListObjects(context.Background(), bucket, prefix)
		return ObjectsMsg{Bucket: bucket, Prefix: prefix, List: list, Err: err}
	}
}

func (m Model) fetchContent(bucketName, objectName string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.GetObjectContent(context.Background(), bucketName, objectName)
		return ContentMsg{ObjectName: objectName, Content: content, Err: err}
	}
}

func (m Model) fetchPrefixMetadata(idx int) tea.Cmd {
	// Ensure we get the correct prefix name from the filtered list, but fetchPrefixMetadata
	// is typically called with an index into the main list.
	// Wait, the index passed to this function might be the filtered index. Let's fix this
	// to take the name directly to avoid issues.
	return func() tea.Msg {
		return nil // Replaced by fetchPrefixMetadataByName
	}
}

func (m Model) fetchPrefixMetadataByName(name string, originalIdx int) tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix
	return func() tea.Msg {
		meta, err := m.client.GetObjectMetadata(context.Background(), bucket, name)
		return MetadataMsg{Bucket: bucket, Prefix: prefix, PrefixIndex: originalIdx, Metadata: meta, Err: err}
	}
}

func (m Model) fetchDownload(bucketName, objectName, dest string, isPrefix bool) tea.Cmd {
	return func() tea.Msg {
		if isPrefix {
			err := m.client.DownloadPrefixAsZip(context.Background(), bucketName, objectName, dest)
			return DownloadMsg{Path: dest, Err: err}
		}
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest)
		return DownloadMsg{Path: dest, Err: err}
	}
}

func (m *Model) resetObjectsState() {
	m.objects = nil
	m.prefixes = nil
	m.cursor = 0
	m.loading = true
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}
	m.selected = make(map[string]struct{})
}

func parentPrefix(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return ""
}

func autoRename(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	for i := 1; ; i++ {
		newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}

func isBinary(s string) bool {
	// A simple heuristic: if it contains a null byte, it's likely binary.
	return strings.ContainsRune(s, '\x00')
}

func fuzzyMatch(query, target string) bool {
	if len(query) == 0 {
		return true
	}
	if len(query) > len(target) {
		return false
	}
	queryRunes := []rune(strings.ToLower(query))
	targetRunes := []rune(strings.ToLower(target))

	i := 0
	for _, r := range targetRunes {
		if r == queryRunes[i] {
			i++
			if i == len(queryRunes) {
				return true
			}
		}
	}
	return false
}

func (m Model) filteredBuckets() []BucketListItem {
	var items []BucketListItem

	if m.state != viewBuckets {
		// Even if not in viewBuckets, we need to generate the list to find activeIdx
		// But if not in viewBuckets, we ignore the search query to show the full tree context
	}

	lowerQuery := strings.ToLower(m.searchQuery)
	isSearchActive := m.searchQuery != "" && m.state == viewBuckets

	for _, p := range m.projects {
		// Determine if the project should be expanded.
		_, isCollapsed := m.collapsedProjects[p.ProjectID]
		isExpanded := !isCollapsed
		if isSearchActive {
			isExpanded = true // Always expand during search to show matches
		}

		// Filter buckets within the project
		var matchingBuckets []string
		for _, b := range p.Buckets {
			if !isSearchActive {
				matchingBuckets = append(matchingBuckets, b)
				continue
			}

			bMatch := false
			if m.fuzzySearch {
				bMatch = fuzzyMatch(lowerQuery, b)
			} else {
				bMatch = strings.Contains(strings.ToLower(b), lowerQuery)
			}

			// Only match against bucket name
			if bMatch {
				matchingBuckets = append(matchingBuckets, b)
			}
		}

		// Add project header only if we're not searching, OR if it has matching buckets.
		// Note: We no longer match against projectMatches.
		if !isSearchActive || len(matchingBuckets) > 0 {
			items = append(items, BucketListItem{
				IsProject: true,
				ProjectID: p.ProjectID,
			})

			if isExpanded {
				for _, b := range matchingBuckets {
					items = append(items, BucketListItem{
						IsProject:  false,
						ProjectID:  p.ProjectID,
						BucketName: b,
					})
				}
			}
		}
	}

	return items
}

func (m Model) filteredObjects() ([]gcs.PrefixMetadata, []gcs.ObjectMetadata, []int) {
	if m.searchQuery == "" || m.state != viewObjects {
		// When no search query or not in objects view, original indices are a straight mapping
		indices := make([]int, len(m.prefixes))
		for i := range m.prefixes {
			indices[i] = i
		}
		return m.prefixes, m.objects, indices
	}

	var filteredPrefixes []gcs.PrefixMetadata
	var filteredObjects []gcs.ObjectMetadata
	var originalPrefixIndices []int

	lowerQuery := strings.ToLower(m.searchQuery)

	for i, p := range m.prefixes {
		match := false
		if m.fuzzySearch {
			match = fuzzyMatch(lowerQuery, p.Name)
		} else {
			match = strings.Contains(strings.ToLower(p.Name), lowerQuery)
		}
		if match {
			filteredPrefixes = append(filteredPrefixes, p)
			originalPrefixIndices = append(originalPrefixIndices, i)
		}
	}
	for _, o := range m.objects {
		match := false
		if m.fuzzySearch {
			match = fuzzyMatch(lowerQuery, o.Name)
		} else {
			match = strings.Contains(strings.ToLower(o.Name), lowerQuery)
		}
		if match {
			filteredObjects = append(filteredObjects, o)
		}
	}
	return filteredPrefixes, filteredObjects, originalPrefixIndices
}

func (m *Model) processDownloadQueue() tea.Cmd {
	if len(m.downloadQueue) == 0 {
		return nil
	}

	task := m.downloadQueue[0]
	m.downloadQueue = m.downloadQueue[1:]

	// Check if file already exists
	if _, err := os.Stat(task.dest); err == nil {
		m.state = viewDownloadConfirm
		m.pendingDownloadBucket = task.bucket
		m.pendingDownloadObject = task.object
		m.pendingDownloadDest = task.dest
		m.pendingDownloadIsPrefix = task.isPrefix
		m.status = fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename?", filepath.Base(task.dest))
		return nil
	}

	if m.downloadTotal > 1 {
		m.status = fmt.Sprintf("Downloading %d/%d: %s...", m.downloadFinished+1, m.downloadTotal, filepath.Base(task.dest))
	} else {
		m.status = fmt.Sprintf("Downloading %s...", filepath.Base(task.dest))
	}
	return m.fetchDownload(task.bucket, task.object, task.dest, task.isPrefix)
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
		m.projects = msg.Projects
		return m, nil

	case ObjectsMsg:
		if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
			return m, nil
		}
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
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
			cmd = m.fetchPrefixMetadataByName(m.prefixes[m.cursor].Name, m.cursor)
		} else if len(m.objects) > 0 {
			cmd = m.fetchContent(m.currentBucket, m.objects[0].Name)
			m.previewContent = "Loading..."
		}
		return m, cmd

	case MetadataMsg:
		if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
			return m, nil
		}
		if msg.Err == nil && msg.PrefixIndex >= 0 && msg.PrefixIndex < len(m.prefixes) {
			m.prefixes[msg.PrefixIndex].Created = msg.Metadata.Created
			m.prefixes[msg.PrefixIndex].Updated = msg.Metadata.Updated
			m.prefixes[msg.PrefixIndex].Owner = msg.Metadata.Owner
		}
		return m, nil

	case ContentMsg:
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
					}
				}
			}
		}
		return m, nil

	case DownloadMsg:
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
			cmd := m.processDownloadQueue()
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if m.showHelp {
			switch {
			case key.Matches(msg, keys.Help), key.Matches(msg, keys.Quit), msg.String() == "esc":
				m.showHelp = false
			}
			return m, nil
		}

		if m.searchMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.searchMode = false
				m.searchQuery = ""
				m.cursor = 0
				return m, nil
			case tea.KeyEnter:
				m.searchMode = false
				return m, nil
			case tea.KeyBackspace, tea.KeyDelete:
				if len(m.searchQuery) > 0 {
					runes := []rune(m.searchQuery)
					m.searchQuery = string(runes[:len(runes)-1])
					m.cursor = 0
				}
				return m, nil
			case tea.KeyRunes, tea.KeySpace:
				m.searchQuery += msg.String()
				m.cursor = 0
				return m, nil
			}
			return m, nil
		}

		if m.state == viewDownloadConfirm {
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
				newDest := autoRename(m.pendingDownloadDest)
				m.status = fmt.Sprintf("Downloading as %s...", filepath.Base(newDest))
				m.state = viewObjects
				return m, m.fetchDownload(m.pendingDownloadBucket, m.pendingDownloadObject, newDest, m.pendingDownloadIsPrefix)
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Help):
			m.showHelp = true
			return m, nil

		case key.Matches(msg, keys.Search):
			m.searchMode = true
			m.searchQuery = ""
			m.cursor = 0
			return m, nil

		case key.Matches(msg, keys.Select):
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

		case key.Matches(msg, keys.Down):
			if !strings.HasPrefix(m.status, "Downloading") {
				m.status = ""
			}

			var itemsCount int
			var currentPrefixes []gcs.PrefixMetadata
			var currentObjects []gcs.ObjectMetadata
			var origIndices []int

			if m.state == viewBuckets {
				itemsCount = len(m.filteredBuckets())
			} else if m.state == viewObjects {
				currentPrefixes, currentObjects, origIndices = m.filteredObjects()
				itemsCount = len(currentObjects) + len(currentPrefixes)
			}

			if itemsCount > 0 {
				oldCursor := m.cursor
				m.cursor = (m.cursor + 1) % itemsCount
				if oldCursor != m.cursor {
					m.previewContent = "" // Reset preview on move
					if m.state == viewObjects {
						if m.cursor < len(currentPrefixes) {
							origIdx := origIndices[m.cursor]
							if m.prefixes[origIdx].Created.IsZero() {
								return m, m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx)
							}
						} else if m.cursor >= len(currentPrefixes) {
							idx := m.cursor - len(currentPrefixes)
							obj := currentObjects[idx]
							m.previewContent = "Loading..."
							return m, m.fetchContent(m.currentBucket, obj.Name)
						}
					}
				}
			}
		case key.Matches(msg, keys.Up):
			if !strings.HasPrefix(m.status, "Downloading") {
				m.status = ""
			}

			var itemsCount int
			var currentPrefixes []gcs.PrefixMetadata
			var currentObjects []gcs.ObjectMetadata
			var origIndices []int

			if m.state == viewBuckets {
				itemsCount = len(m.filteredBuckets())
			} else if m.state == viewObjects {
				currentPrefixes, currentObjects, origIndices = m.filteredObjects()
				itemsCount = len(currentObjects) + len(currentPrefixes)
			}

			if itemsCount > 0 {
				oldCursor := m.cursor
				m.cursor = (m.cursor - 1 + itemsCount) % itemsCount
				if oldCursor != m.cursor {
					m.previewContent = "" // Reset preview on move
					if m.state == viewObjects {
						if m.cursor < len(currentPrefixes) {
							origIdx := origIndices[m.cursor]
							if m.prefixes[origIdx].Created.IsZero() {
								return m, m.fetchPrefixMetadataByName(currentPrefixes[m.cursor].Name, origIdx)
							}
						} else if m.cursor >= len(currentPrefixes) {
							idx := m.cursor - len(currentPrefixes)
							obj := currentObjects[idx]
							m.previewContent = "Loading..."
							return m, m.fetchContent(m.currentBucket, obj.Name)
						}
					}
				}
			}
		case key.Matches(msg, keys.Right):
			if m.state == viewBuckets {
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
					// Note: Since expanding/collapsing can change the absolute index,
					// restoring exact cursor might require matching by bucket name.
					m.bucketCursor = m.cursor

					m.currentPrefix = "" // Reset prefix when entering bucket
					m.state = viewObjects
					m.searchMode = false
					m.searchQuery = ""
					m.resetObjectsState()
					return m, m.fetchObjects()
				}
			} else if m.state == viewObjects {
				m.previewContent = ""
				currentPrefixes, _, _ := m.filteredObjects()
				// Check if selected item is a prefix
				if m.cursor < len(currentPrefixes) {
					m.currentPrefix = currentPrefixes[m.cursor].Name
					m.searchMode = false
					m.searchQuery = ""
					m.resetObjectsState()
					return m, m.fetchObjects()
				}
			}
		case key.Matches(msg, keys.Left):
			if m.state == viewBuckets {
				filtered := m.filteredBuckets()
				if len(filtered) > 0 {
					item := filtered[m.cursor]
					if item.IsProject {
						// Ensure project is collapsed
						m.collapsedProjects[item.ProjectID] = struct{}{}
					} else {
						// Optional: If on a bucket, 'h' could jump back up to the project header
						// Let's implement that since it's a standard tree navigation behavior
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
				m.resetObjectsState()
				return m, m.fetchObjects()
			}
		case key.Matches(msg, keys.Download):
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

					cmd := m.processDownloadQueue()
					return m, cmd
				}
			}
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}
