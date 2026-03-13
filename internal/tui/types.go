package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
)

// GCSClient defines the contract for interacting with Google Cloud Storage.
// This interface allows for easy mocking in TUI unit tests.
type GCSClient interface {
	// ListBucketsPage retrieves a specific page of buckets for a given project.
	ListBucketsPage(ctx context.Context, projectID, pageToken string, pageSize int) ([]string, string, error)
	// ListObjects returns names of objects and common prefixes (folders) in a bucket.
	ListObjects(ctx context.Context, bucketName, prefix string) (*gcs.ObjectList, error)
	// ListObjectsPage retrieves a specific page of object names and common prefixes (folders).
	ListObjectsPage(ctx context.Context, bucketName, prefix, pageToken string, pageSize int) (*gcs.ObjectList, string, error)
	// GetObjectMetadata returns full metadata for a specific object or directory stub.
	GetObjectMetadata(ctx context.Context, bucketName, objectName string) (*gcs.ObjectMetadata, error)
	// GetObjectContent returns the first 1KB of content for a specific object.
	GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error)
	// DownloadObject downloads the content of a GCS object to a local file.
	DownloadObject(ctx context.Context, bucketName, objectName, destPath string) error
	// DownloadPrefixAsZip downloads all objects under a prefix into a local zip file.
	DownloadPrefixAsZip(ctx context.Context, bucketName, prefix, destZipPath string) error
	// NewReader returns a sequential reader for an object.
	NewReader(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
	// NewReaderAt returns an io.ReaderAt for an object.
	NewReaderAt(ctx context.Context, bucketName, objectName string) io.ReaderAt
}

// BucketsPageMsg is sent for progressive loading of project buckets.
type BucketsPageMsg struct {
	ProjectID string
	Buckets   []string
	NextToken string
	Err       error
}

// ObjectsMsg is sent when object listing completes.
type ObjectsMsg struct {
	Bucket string
	Prefix string
	List   *gcs.ObjectList
	Err    error
}

// ObjectsPageMsg is sent for progressive loading of large buckets.
type ObjectsPageMsg struct {
	Bucket    string
	Prefix    string
	List      *gcs.ObjectList
	NextToken string
	Err       error
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

// HoverPrefetchTickMsg is sent after a delay to trigger a background prefetch.
type HoverPrefetchTickMsg struct {
	CursorVersion int
	FetchCmd      tea.Cmd
}

// HoverPrefetchMsg is sent when a background prefetch completes.
type HoverPrefetchMsg struct {
	Bucket string
	Prefix string
	List   *gcs.ObjectList
	Err    error
}
