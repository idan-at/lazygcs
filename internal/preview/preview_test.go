package preview_test

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"lazygcs/internal/preview"
)

type failingPreviewer struct{}

func (p *failingPreviewer) Priority() int            { return 10 }
func (p *failingPreviewer) CanPreview(obj preview.Object) bool { return true }
func (p *failingPreviewer) Preview(ctx context.Context, client preview.GCSClient, obj preview.Object) (string, error) {
	return "", errors.New("boom")
}
func (p *failingPreviewer) SetWidth(width int) {}

type successPreviewer struct {
	content string
}

func (p *successPreviewer) Priority() int            { return 20 }
func (p *successPreviewer) CanPreview(obj preview.Object) bool { return true }
func (p *successPreviewer) Preview(ctx context.Context, client preview.GCSClient, obj preview.Object) (string, error) {
	return p.content, nil
}
func (p *successPreviewer) SetWidth(width int) {}

func TestRegistry_Fallback(t *testing.T) {
	reg := preview.NewRegistry()
	reg.Register(&failingPreviewer{})
	reg.Register(&successPreviewer{content: "fallback success"})

	content, err := reg.GetPreview(context.Background(), nil, preview.Object{})
	
	// Should return the fallback content AND the error from the failed previewer
	assert.Equal(t, content, "fallback success")
	assert.Error(t, err, "boom")
}
