package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"lazygcs/internal/gcs"
)

// GCSClient defines the contract for interacting with Google Cloud Storage.
// This interface allows for easy mocking in TUI unit tests.
type GCSClient interface {
	// ListBuckets returns names of buckets grouped by project.
	ListBuckets(ctx context.Context, projectIDs []string) ([]gcs.ProjectBuckets, error)
	// ListObjects returns names of objects and common prefixes (folders) in a bucket.
	ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error)
	// GetObjectMetadata returns full metadata for a specific object or directory stub.
	GetObjectMetadata(ctx context.Context, bucketName, objectName string) (*gcs.ObjectMetadata, error)
	// GetObjectContent returns the first 1KB of content for a specific object.
	GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error)
	// DownloadObject downloads the content of a GCS object to a local file.
	DownloadObject(ctx context.Context, bucketName, objectName, destPath string) error
	// DownloadPrefixAsZip downloads all objects under a prefix into a local zip file.
	DownloadPrefixAsZip(ctx context.Context, bucketName, prefix, destZipPath string) error
}

// BucketsMsg is sent when bucket listing completes.
type BucketsMsg struct {
	Projects []gcs.ProjectBuckets
	Err      error
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

// ClearStatusMsg is sent to clear the status bar.
type ClearStatusMsg struct{}

// DebouncePreviewMsg is sent after a delay to trigger a preview fetch.
type DebouncePreviewMsg struct {
	CursorVersion int
	FetchCmd      tea.Cmd
}
