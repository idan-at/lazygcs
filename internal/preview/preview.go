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

type Registry struct {
	previewers []Previewer
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(p Previewer) {
	r.previewers = append(r.previewers, p)
	sort.Slice(r.previewers, func(i, j int) bool {
		return r.previewers[i].Priority() < r.previewers[j].Priority()
	})
}

func (r *Registry) SetWidth(width int) {
	for _, p := range r.previewers {
		p.SetWidth(width)
	}
}

func (r *Registry) GetPreview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	for _, p := range r.previewers {
		if p.CanPreview(obj) {
			return p.Preview(ctx, client, obj)
		}
	}
	return "(no preview available)", nil
}

func IsBinary(s string) bool {
	return strings.ContainsRune(s, '\x00')
}
