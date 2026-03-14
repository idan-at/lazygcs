package preview

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
)

// MarkdownPreviewer ...
type MarkdownPreviewer struct {
	width int
}

// NewMarkdownPreviewer ...
func NewMarkdownPreviewer(width int) *MarkdownPreviewer {
	return &MarkdownPreviewer{width: width}
}

// Priority ...
func (p *MarkdownPreviewer) Priority() int { return 50 }

// CanPreview ...
func (p *MarkdownPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	return ext == ".md" || ext == ".markdown" || obj.ContentType == "text/markdown"
}

// Preview ...
func (p *MarkdownPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	// Fetch more for markdown to get a better rendering, but still limit it.
	// We'll use NewReader and read a chunk since GetObjectContent is fixed at 1KB.
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	// Read up to 50KB
	limit := int64(50 * 1024)
	if obj.Size < limit {
		limit = obj.Size
	}
	buf := make([]byte, limit)
	n, err := io.ReadFull(rc, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	content := string(buf[:n])

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(p.width),
	)
	if err != nil {
		return content, nil // Fallback to raw markdown if glamour fails
	}

	out, err := r.Render(content)
	if err != nil {
		return content, nil
	}

	return out, nil
}

// SetWidth ...
func (p *MarkdownPreviewer) SetWidth(width int) {
	p.width = width
}
