package preview

import (
	"context"
	"strings"

)

type TextPreviewer struct{}

func (p *TextPreviewer) Priority() int { return 100 } // Low priority fallback

func (p *TextPreviewer) CanPreview(obj Object) bool {
	return true // Fallback for everything
}

func (p *TextPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	content, err := client.GetObjectContent(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}

	if isBinary(content) {
		return "(binary content)", nil
	}

	return content, nil
}

func (p *TextPreviewer) SetWidth(width int) {}

func isBinary(s string) bool {
	return strings.ContainsRune(s, '\x00')
}
