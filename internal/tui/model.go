package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Search State
	searchMode  bool
	searchQuery string

	// Download Confirm State
	pendingDownloadBucket string
	pendingDownloadObject string
	pendingDownloadDest   string

	// Buckets View
	buckets []string
	cursor  int // used for buckets or objects depending on state

	// Objects View
	currentBucket string
	currentPrefix string
	objects       []gcs.ObjectMetadata
	prefixes      []gcs.PrefixMetadata
	selected      map[string]struct{}

	loading bool
	status  string
	err     error
}

// NewModel creates a Model initialized with the provided projects and GCS client.
func NewModel(projectIDs []string, client GCSClient, downloadDir string) Model {
	return Model{
		projectIDs:  projectIDs,
		client:      client,
		downloadDir: downloadDir,
		width:       120,
		height:      40,
		state:       viewBuckets,
		loading:     true,
		selected:    make(map[string]struct{}),
	}
}

// Init initializes the application by triggering the first bucket fetch.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		buckets, err := m.client.ListBuckets(context.Background(), m.projectIDs)
		return BucketsMsg{Buckets: buckets, Err: err}
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


func (m Model) fetchDownload(bucketName, objectName, dest string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest)
		return DownloadMsg{Path: dest, Err: err}
	}
}

func (m *Model) resetObjectsState() {
	m.objects = nil
	m.prefixes = nil
	m.cursor = 0
	m.loading = true
	m.status = ""
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

func (m Model) filteredBuckets() []string {
	if m.searchQuery == "" || m.state != viewBuckets {
		return m.buckets
	}
	var filtered []string
	lowerQuery := strings.ToLower(m.searchQuery)
	for _, b := range m.buckets {
		if strings.Contains(strings.ToLower(b), lowerQuery) {
			filtered = append(filtered, b)
		}
	}
	return filtered
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
		if strings.Contains(strings.ToLower(p.Name), lowerQuery) {
			filteredPrefixes = append(filteredPrefixes, p)
			originalPrefixIndices = append(originalPrefixIndices, i)
		}
	}
	for _, o := range m.objects {
		if strings.Contains(strings.ToLower(o.Name), lowerQuery) {
			filteredObjects = append(filteredObjects, o)
		}
	}
	return filteredPrefixes, filteredObjects, originalPrefixIndices
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
		m.buckets = msg.Buckets
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

		var cmd tea.Cmd
		if len(m.prefixes) > 0 {
			cmd = m.fetchPrefixMetadataByName(m.prefixes[0].Name, 0)
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
			m.status = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("Downloaded to %s", msg.Path))
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
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
				return m, m.fetchDownload(m.pendingDownloadBucket, m.pendingDownloadObject, m.pendingDownloadDest)
			case "a", "q", "ctrl+c", "esc":
				m.status = "Download aborted."
				m.state = viewObjects
				return m, nil
			case "r":
				newDest := autoRename(m.pendingDownloadDest)
				m.status = fmt.Sprintf("Downloading as %s...", filepath.Base(newDest))
				m.state = viewObjects
				return m, m.fetchDownload(m.pendingDownloadBucket, m.pendingDownloadObject, newDest)
			}
			return m, nil
		}

		switch msg.String() {
		case "/":
			m.searchMode = true
			m.searchQuery = ""
			m.cursor = 0
			return m, nil

		case " ":
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

		case "j", "down":			m.status = ""
			
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
		case "k", "up":
			m.status = ""
			
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
		case "l", "right", "enter":
			if m.state == viewBuckets {
				filtered := m.filteredBuckets()
				if len(filtered) > 0 {
					m.currentBucket = filtered[m.cursor]
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
		case "h", "left":
			if m.state == viewObjects {
				m.previewContent = ""
				m.searchMode = false
				m.searchQuery = ""
				if m.currentPrefix == "" {
					m.state = viewBuckets
					m.currentBucket = ""
					m.cursor = 0 // for now, reset cursor when going back
					return m, nil
				}
				// Go up one level
				m.currentPrefix = parentPrefix(m.currentPrefix)
				m.resetObjectsState()
				return m, m.fetchObjects()
			}
		case "d":
			if m.state == viewObjects {
				currentPrefixes, currentObjects, _ := m.filteredObjects()
				if m.cursor >= len(currentPrefixes) {
					idx := m.cursor - len(currentPrefixes)
					if idx < len(currentObjects) {
						obj := currentObjects[idx]
						dest := filepath.Join(m.downloadDir, filepath.Base(obj.Name))
						
						// Check if file already exists
						if _, err := os.Stat(dest); err == nil {
							m.state = viewDownloadConfirm
							m.pendingDownloadBucket = m.currentBucket
							m.pendingDownloadObject = obj.Name
							m.pendingDownloadDest = dest
							m.status = fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename?", filepath.Base(dest))
							return m, nil
						}

						m.status = "Downloading..."
						return m, m.fetchDownload(m.currentBucket, obj.Name, dest)
					}
				}
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}
