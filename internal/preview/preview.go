package preview

import (
	"context"
	"io"
	"sort"
	"strings"
)

// GCSClient defines the methods required from the GCS client for previewing.
type GCSClient interface {
	GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error)
	NewReader(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
	NewReaderAt(ctx context.Context, bucketName, objectName string) io.ReaderAt
}

// Object is a subset of gcs.ObjectMetadata needed for previewing.
type Object struct {
	Bucket      string
	Name        string
	Size        int64
	ContentType string
}

// Previewer ...
type Previewer interface {
	// Priority defines the order of evaluation (lower is higher priority).
	Priority() int
	// CanPreview determines if this previewer should handle the object.
	CanPreview(obj Object) bool
	// Preview returns the fully rendered ANSI string.
	Preview(ctx context.Context, client GCSClient, obj Object) (string, error)
	// SetWidth updates the preferred rendering width.
	SetWidth(width int)
}

// Registry ...
type Registry struct {
	previewers []Previewer
}

// NewRegistry ...
func NewRegistry() *Registry {
	return &Registry{}
}

// Register ...
func (r *Registry) Register(p Previewer) {
	r.previewers = append(r.previewers, p)
	sort.Slice(r.previewers, func(i, j int) bool {
		return r.previewers[i].Priority() < r.previewers[j].Priority()
	})
}

// SetWidth ...
func (r *Registry) SetWidth(width int) {
	for _, p := range r.previewers {
		p.SetWidth(width)
	}
}

// GetPreview ...
func (r *Registry) GetPreview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	var lastErr error
	for _, p := range r.previewers {
		if p.CanPreview(obj) {
			content, err := p.Preview(ctx, client, obj)
			if err == nil {
				return content, nil
			}
			lastErr = err // Record error and try next previewer
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "(no preview available)", nil
}

// IsBinary ...
func IsBinary(s string) bool {
	return strings.ContainsRune(s, '\x00')
}
