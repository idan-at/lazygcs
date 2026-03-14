package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
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
}

// BucketListItem ...
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
	showErrors     bool

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
	cursorVersion     int // used for debouncing preview requests

	// Objects View
	currentBucket      string
	currentPrefix      string
	targetPrefixCursor string
	objects            []gcs.ObjectMetadata
	prefixes           []gcs.PrefixMetadata
	selected           map[string]struct{}

	loading         bool
	loadingProjects map[string]bool
	bgJobs          int
	status          string
	err             error
	errorsList      []error
	help            help.Model
	spinner         spinner.Model
	previewRegistry *preview.Registry

	// Caches
	listCache     map[string]listCacheEntry
	contentCache  map[string]contentCacheEntry
	metadataCache map[string]metadataCacheEntry
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

// NewModel creates a Model initialized with the provided projects and GCS client.
func NewModel(projectIDs []string, client GCSClient, downloadDir string, fuzzySearch bool, showIcons bool) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

	loadingProjects := make(map[string]bool)
	for _, id := range projectIDs {
		loadingProjects[id] = true
	}

	reg := preview.NewRegistry()
	reg.Register(&preview.ImagePreviewer{})
	reg.Register(&preview.DataPreviewer{})
	reg.Register(&preview.ConfigPreviewer{})
	reg.Register(&preview.PDFPreviewer{})
	reg.Register(&preview.LogPreviewer{})
	reg.Register(&preview.ZipPreviewer{})
	reg.Register(&preview.TarPreviewer{})
	reg.Register(preview.NewMarkdownPreviewer(50))
	reg.Register(&preview.CodePreviewer{})
	reg.Register(&preview.TextPreviewer{}) // Fallback last

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
		loadingProjects:   loadingProjects,
		bgJobs:            len(projectIDs),
		selected:          make(map[string]struct{}),
		collapsedProjects: make(map[string]struct{}),
		help:              help.New(),
		spinner:           s,
		previewRegistry:   reg,
		listCache:         make(map[string]listCacheEntry),
		contentCache:      make(map[string]contentCacheEntry),
		metadataCache:     make(map[string]metadataCacheEntry),
	}
}

// Cursor returns the current cursor position.
func (m Model) Cursor() int {
	return m.cursor
}

func (m Model) resetObjectsState() Model {
	m.objects = nil
	m.prefixes = nil
	m.cursor = 0
	m.loading = true
	m.bgJobs++
	if !strings.HasPrefix(m.status, "Downloading") {
		m.status = ""
	}
	m.selected = make(map[string]struct{})
	return m
}

func (m Model) filteredBuckets() []BucketListItem {
	var items []BucketListItem

	lowerQuery := strings.ToLower(m.searchQuery)
	isSearchActive := m.searchQuery != "" && m.state == viewBuckets

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
