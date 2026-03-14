package preview

import (
	"context"
)

// TextPreviewer ...
type TextPreviewer struct{}

// Priority ...
func (p *TextPreviewer) Priority() int { return 100 } // Low priority fallback

// CanPreview ...
func (p *TextPreviewer) CanPreview(_ Object) bool {
	return true // Fallback for everything
}

// Preview ...
func (p *TextPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	content, err := client.GetObjectContent(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}

	if IsBinary(content) {
		return "(binary content)", nil
	}

	return content, nil
}

// SetWidth ...
func (p *TextPreviewer) SetWidth(_ int) {}
