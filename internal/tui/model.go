package tui

import (
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/preview"
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
	jobNum   int
}

// BucketListItem ...
type BucketListItem struct {
	IsProject  bool
	ProjectID  string
	BucketName string
}

const maxConcurrentDownloads = 5

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
	showMetadata   bool
	showVersions   bool

	// Search State
	searchMode        bool
	bucketSearchQuery string
	objectSearchQuery string
	fuzzySearch       bool

	// Creation State
	creationMode  bool
	creationQuery string

	// Settings
	showNerdIcons bool

	// Download Confirm State
	activeDownloads         int
	activeDestinations      map[string]bool
	pendingDownloadBucket   string
	pendingDownloadObject   string
	pendingDownloadDest     string
	pendingDownloadIsPrefix bool
	pendingDownloadJobNum   int
	downloadQueue           []downloadTask
	jobProgress             map[int]*JobProgress

	// Buckets View
	projects            []gcs.ProjectBuckets
	collapsedProjects   map[string]struct{}
	cursor              int // used for buckets or objects depending on state
	bucketCursor        int // stores the cursor position in the bucket list
	cursorVersion       int // used for debouncing preview requests
	bucketMetadataCache *LRUCache[string, bucketMetadataCacheEntry]

	// Objects View
	currentBucket             string
	currentPrefix             string
	targetPrefixCursor        string
	objects                   []gcs.ObjectMetadata
	prefixes                  []gcs.PrefixMetadata
	selected                  map[string]struct{}
	objectVersions            []gcs.ObjectMetadata
	isBucketVersioningEnabled bool
	versioningChecked         bool

	loading         bool
	loadingProjects map[string]bool
	bgJobs          int
	activeTasks     map[string]Task
	nextJobNum      int
	showMessages    bool
	msgQueue        *MessageQueue
	err             error
	help            help.Model
	spinner         spinner.Model
	previewRegistry *preview.Registry
	clipboard       ClipboardWriter
	sendMsg         func(tea.Msg)

	// Test settings
	deterministicSpinner bool

	// Caches
	listCache             map[string]listCacheEntry
	contentCache          map[string]contentCacheEntry
	metadataCache         map[string]metadataCacheEntry
	bucketVersioningCache map[string]bool
}

type listCacheEntry struct {
	List      *gcs.ObjectList
	ExpiresAt time.Time
}

type contentCacheEntry struct {
	Content   string
	ExpiresAt time.Time
}

type metadataCacheEntry struct {
	Metadata  *gcs.ObjectMetadata
	ExpiresAt time.Time
}

type bucketMetadataCacheEntry struct {
	Metadata     *gcs.BucketMetadata
	SortedLabels []Label
	Err          error
	ExpiresAt    time.Time
}

// Label represents a key-value pair for metadata labels.
type Label struct {
	Key   string
	Value string
}

// RealClipboard implements ClipboardWriter using the system clipboard.
type RealClipboard struct{}

// WriteAll writes the given text to the system clipboard.
func (c *RealClipboard) WriteAll(text string) error {
	return clipboard.WriteAll(text)
}

// NewModel creates a Model initialized with the provided projects and GCS client.
func NewModel(projectIDs []string, client GCSClient, downloadDir string, fuzzySearch bool, showNerdIcons bool) Model {
	return NewModelWithSender(projectIDs, client, downloadDir, fuzzySearch, showNerdIcons, nil)
}

// NewModelWithSender creates a Model initialized with a message sender for async updates.
func NewModelWithSender(projectIDs []string, client GCSClient, downloadDir string, fuzzySearch bool, showNerdIcons bool, sendMsg func(tea.Msg)) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7"))

	loadingProjects := make(map[string]bool)
	for _, id := range projectIDs {
		loadingProjects[id] = true
	}

	reg := preview.NewDefaultRegistry()

	return Model{
		projectIDs:            projectIDs,
		client:                client,
		downloadDir:           downloadDir,
		fuzzySearch:           fuzzySearch,
		showNerdIcons:         showNerdIcons,
		width:                 120,
		height:                40,
		state:                 viewBuckets,
		loading:               true,
		loadingProjects:       loadingProjects,
		bgJobs:                len(projectIDs),
		nextJobNum:            1,
		jobProgress:           make(map[int]*JobProgress),
		msgQueue:              NewMessageQueue(),
		activeTasks:           make(map[string]Task),
		activeDestinations:    make(map[string]bool),
		selected:              make(map[string]struct{}),
		collapsedProjects:     make(map[string]struct{}),
		bucketMetadataCache:   NewLRUCache[string, bucketMetadataCacheEntry](256),
		help:                  help.New(),
		spinner:               s,
		previewRegistry:       reg,
		clipboard:             &RealClipboard{},
		sendMsg:               sendMsg,
		deterministicSpinner:  false,
		listCache:             make(map[string]listCacheEntry),
		contentCache:          make(map[string]contentCacheEntry),
		metadataCache:         make(map[string]metadataCacheEntry),
		bucketVersioningCache: make(map[string]bool),
	}
}

// SetDeterministicSpinner sets the spinner to a fixed state for testing.
func (m *Model) SetDeterministicSpinner(v bool) {
	m.deterministicSpinner = v
}

// SetClipboard sets the clipboard writer for testing.
func (m *Model) SetClipboard(c ClipboardWriter) {
	m.clipboard = c
}

// SetSendMsg sets the message sender for testing or async updates.
func (m *Model) SetSendMsg(s func(tea.Msg)) {
	m.sendMsg = s
}

// Cursor returns the current cursor position.
func (m *Model) Cursor() int {
	return m.cursor
}

// Objects returns the current list of objects.
func (m *Model) Objects() []gcs.ObjectMetadata {
	return m.objects
}

// Prefixes returns the current list of prefixes.
func (m *Model) Prefixes() []gcs.PrefixMetadata {
	return m.prefixes
}

// Messages returns the current list of log messages.
func (m *Model) Messages() []LogMessage {
	return m.msgQueue.Messages()
}

// ErrorCount returns the number of log messages with LevelError.
func (m *Model) ErrorCount() int {
	return m.msgQueue.ErrorCount
}

// HideStatusPill returns whether the status pill is currently hidden.
func (m *Model) HideStatusPill() bool {
	return m.msgQueue.HideStatusPill
}

// ShowMessages returns whether the messages view is currently shown.
func (m *Model) ShowMessages() bool {
	return m.showMessages
}

// ShowVersions returns whether the versions view is currently shown.
func (m *Model) ShowVersions() bool {
	return m.showVersions
}

// ShowMetadata returns whether the metadata view is currently shown.
func (m *Model) ShowMetadata() bool {
	return m.showMetadata
}

// ObjectVersions returns the list of versions for the current object.
func (m *Model) ObjectVersions() []gcs.ObjectMetadata {
	return m.objectVersions
}

// ActiveTasks returns the current active background tasks.
func (m *Model) ActiveTasks() map[string]Task {
	return m.activeTasks
}

// AddMessage appends a new message and returns a command to clear it from the status bar after a delay.
func (m *Model) AddMessage(level MsgLevel, text string, jobNum int, taskID string) tea.Cmd {
	return m.msgQueue.AddMessage(level, text, jobNum, taskID)
}

func (m *Model) resetObjectsState() *Model {
	m.objects = nil
	m.prefixes = nil
	m.cursor = 0
	m.loading = true
	m.bgJobs++
	// Removed status clear logic
	m.selected = make(map[string]struct{})
	return m
}

func (m *Model) withCurrentSearchQuery(q string) *Model {
	if m.state == viewBuckets {
		m.bucketSearchQuery = q
	} else {
		m.objectSearchQuery = q
	}
	return m
}

func (m *Model) currentSearchQuery() string {
	if m.state == viewBuckets {
		return m.bucketSearchQuery
	}
	return m.objectSearchQuery
}

func (m *Model) filteredBuckets() []BucketListItem {
	var items []BucketListItem

	lowerQuery := strings.ToLower(m.bucketSearchQuery)
	isSearchActive := m.bucketSearchQuery != ""

	for _, projectID := range m.projectIDs {
		// Find project data if it exists
		var p *gcs.ProjectBuckets
		for i := range m.projects {
			if m.projects[i].ProjectID == projectID {
				p = &m.projects[i]
				break
			}
		}

		// Determine if the project should be expanded.
		_, isCollapsed := m.collapsedProjects[projectID]
		isExpanded := !isCollapsed
		if isSearchActive {
			isExpanded = true // Always expand during search to show matches
		}

		// Filter buckets within the project
		var matchingBuckets []string
		if p != nil {
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
		}

		// Add project header only if we're not searching, OR if it has matching buckets.
		if !isSearchActive || len(matchingBuckets) > 0 {
			items = append(items, BucketListItem{
				IsProject: true,
				ProjectID: projectID,
			})

			if isExpanded {
				for _, b := range matchingBuckets {
					items = append(items, BucketListItem{
						IsProject:  false,
						ProjectID:  projectID,
						BucketName: b,
					})
				}
			}
		}
	}

	return items
}

func (m *Model) filteredObjects() ([]gcs.PrefixMetadata, []gcs.ObjectMetadata, []int) {
	if m.objectSearchQuery == "" || m.state != viewObjects {
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

	lowerQuery := strings.ToLower(m.objectSearchQuery)

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
