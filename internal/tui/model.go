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

// GCSClient defines the contract for interacting with Google Cloud Storage.
// This interface allows for easy mocking in TUI unit tests.
type GCSClient interface {
	// ListBuckets returns names of buckets in the specified projects.
	ListBuckets(ctx context.Context, projectIDs []string) ([]string, error)
	// ListObjects returns names of objects and common prefixes (folders) in a bucket.
	ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error)
	// GetObjectMetadata returns full metadata for a specific object or directory stub.
	GetObjectMetadata(ctx context.Context, bucketName, objectName string) (*gcs.ObjectMetadata, error)
	// GetObjectContent returns the first 1KB of content for a specific object.
	GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error)
	// DownloadObject downloads the content of a GCS object to a local file.
	DownloadObject(ctx context.Context, bucketName, objectName, destPath string) error
}

// BucketsMsg is sent when bucket listing completes.
type BucketsMsg struct {
	Buckets []string
	Err     error
}

// ObjectsMsg is sent when object listing completes.
type ObjectsMsg struct {
	Bucket string
	Prefix string
	List   *gcs.ObjectList
	Err    error
}

// MetadataMsg is sent when on-demand metadata fetching completes.
type MetadataMsg struct {
	Bucket      string
	Prefix      string
	PrefixIndex int
	Metadata    *gcs.ObjectMetadata
	Err         error
}

// ContentMsg is sent when on-demand content fetching completes.
type ContentMsg struct {
	ObjectName string
	Content    string
	Err        error
}

// DownloadMsg is sent when a download operation completes.
type DownloadMsg struct {
	Path string
	Err  error
}

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

	loading bool
	status  string
	err     error
}

// NewModel creates a Model initialized with the provided projects and GCS client.
//
// Arguments:
//   - projectIDs: List of projects to scan for buckets initially.
//   - client: Implementation of the GCSClient interface.
//   - downloadDir: Local directory where files will be downloaded.
func NewModel(projectIDs []string, client GCSClient, downloadDir string) Model {
	return Model{
		projectIDs:  projectIDs,
		client:      client,
		downloadDir: downloadDir,
		width:       120,
		height:      40,
		state:       viewBuckets,
		loading:     true,
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
	bucket := m.currentBucket
	prefix := m.currentPrefix
	name := m.prefixes[idx].Name
	return func() tea.Msg {
		meta, err := m.client.GetObjectMetadata(context.Background(), bucket, name)
		return MetadataMsg{Bucket: bucket, Prefix: prefix, PrefixIndex: idx, Metadata: meta, Err: err}
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
		// After listing, if the first item in the list is a prefix,
		// fetch its metadata. If it's an object, fetch its content for preview.
		if len(m.prefixes) > 0 {
			cmd = m.fetchPrefixMetadata(0)
		} else if len(m.objects) > 0 {
			cmd = m.fetchContent(m.currentBucket, m.objects[0].Name)
			m.previewContent = "Loading..."
		}
		return m, cmd

	case MetadataMsg:
		if m.state != viewObjects || msg.Bucket != m.currentBucket || msg.Prefix != m.currentPrefix {
			return m, nil
		}
		if msg.Err == nil && msg.PrefixIndex < len(m.prefixes) {
			m.prefixes[msg.PrefixIndex].Created = msg.Metadata.Created
			m.prefixes[msg.PrefixIndex].Updated = msg.Metadata.Updated
			m.prefixes[msg.PrefixIndex].Owner = msg.Metadata.Owner
		}
		return m, nil

	case ContentMsg:
		// Make sure the content is for the currently selected object
		if m.state == viewObjects && m.cursor >= len(m.prefixes) {
			idx := m.cursor - len(m.prefixes)
			if idx < len(m.objects) && m.objects[idx].Name == msg.ObjectName {
				if msg.Err != nil {
					m.previewContent = fmt.Sprintf("Error: %v", msg.Err)
				} else {
					m.previewContent = msg.Content
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
		case "j", "down":
			m.status = ""
			itemsCount := len(m.buckets)
			if m.state == viewObjects {
				itemsCount = len(m.objects) + len(m.prefixes)
			}
			if itemsCount > 0 {
				oldCursor := m.cursor
				m.cursor = (m.cursor + 1) % itemsCount
				if oldCursor != m.cursor {
					m.previewContent = "" // Reset preview on move
					if m.state == viewObjects {
						if m.cursor < len(m.prefixes) && m.prefixes[m.cursor].Created.IsZero() {
							return m, m.fetchPrefixMetadata(m.cursor)
						} else if m.cursor >= len(m.prefixes) {
							idx := m.cursor - len(m.prefixes)
							obj := m.objects[idx]
							m.previewContent = "Loading..."
							return m, m.fetchContent(m.currentBucket, obj.Name)
						}
					}
				}
			}
		case "k", "up":
			m.status = ""
			itemsCount := len(m.buckets)
			if m.state == viewObjects {
				itemsCount = len(m.objects) + len(m.prefixes)
			}
			if itemsCount > 0 {
				oldCursor := m.cursor
				m.cursor = (m.cursor - 1 + itemsCount) % itemsCount
				if oldCursor != m.cursor {
					m.previewContent = "" // Reset preview on move
					if m.state == viewObjects {
						if m.cursor < len(m.prefixes) && m.prefixes[m.cursor].Created.IsZero() {
							return m, m.fetchPrefixMetadata(m.cursor)
						} else if m.cursor >= len(m.prefixes) {
							idx := m.cursor - len(m.prefixes)
							obj := m.objects[idx]
							m.previewContent = "Loading..."
							return m, m.fetchContent(m.currentBucket, obj.Name)
						}
					}
				}
			}
		case "l", "right", "enter":
			if m.state == viewBuckets && len(m.buckets) > 0 {
				m.currentBucket = m.buckets[m.cursor]
				m.currentPrefix = "" // Reset prefix when entering bucket
				m.state = viewObjects
				m.resetObjectsState()
				return m, m.fetchObjects()
			} else if m.state == viewObjects {
				m.previewContent = ""
				// Check if selected item is a prefix
				if m.cursor < len(m.prefixes) {
					m.currentPrefix = m.prefixes[m.cursor].Name
					m.resetObjectsState()
					return m, m.fetchObjects()
				}
			}
		case "h", "left":
			if m.state == viewObjects {
				m.previewContent = ""
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
			if m.state == viewObjects && m.cursor >= len(m.prefixes) {
				idx := m.cursor - len(m.prefixes)
				if idx < len(m.objects) {
					obj := m.objects[idx]
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
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

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

		if m.cursor < len(m.prefixes) {
			// Selected item is a prefix (folder)
			prefix := m.prefixes[m.cursor]
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
		} else if m.cursor >= len(m.prefixes) && len(m.objects) > 0 {
			// Selected item is an object (not a prefix)
			idx := m.cursor - len(m.prefixes)
			if idx < len(m.objects) {
				obj := m.objects[idx]
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
					s.WriteString(m.previewContent)
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
	statusLine := ""
	if m.status != "" {
		statusLine = "\n" + m.status
	}
	return statusLine + "\n\n(q: quit, h: back, l/enter: select, d: download)"
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
			// Combine prefixes (strings) and objects (structs) for display
			// We iterate through them separately or build a unified list of display strings
			totalItems := len(m.prefixes) + len(m.objects)

			start, end := visibleRange(m.cursor, totalItems, m.maxItemsVisible())

			for i := start; i < end; i++ {
				cursor := " "
				if m.cursor == i {
					cursor = ">"
				}

				var displayItem string
				if i < len(m.prefixes) {
					displayItem = m.prefixes[i].Name
				} else {
					displayItem = m.objects[i-len(m.prefixes)].Name
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
		start, end := visibleRange(m.cursor, len(m.buckets), m.maxItemsVisible())
		for i := start; i < end; i++ {
			bucket := m.buckets[i]
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
