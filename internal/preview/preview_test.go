package preview_test

import (
	"context"
	"errors"
	"testing"

	"github.com/idan-at/lazygcs/internal/preview"
	"gotest.tools/v3/assert"
)

type failingPreviewer struct{}

func (p *failingPreviewer) Priority() int                    { return 10 }
func (p *failingPreviewer) CanPreview(_ preview.Object) bool { return true }
func (p *failingPreviewer) Preview(_ context.Context, _ preview.GCSClient, _ preview.Object) (string, error) {
	return "", errors.New("boom")
}
func (p *failingPreviewer) SetWidth(_ int) {}

type successPreviewer struct {
	content string
}

func (p *successPreviewer) Priority() int                    { return 20 }
func (p *successPreviewer) CanPreview(_ preview.Object) bool { return true }
func (p *successPreviewer) Preview(_ context.Context, _ preview.GCSClient, _ preview.Object) (string, error) {
	return p.content, nil
}
func (p *successPreviewer) SetWidth(_ int) {}

func TestRegistry_Fallback(t *testing.T) {
	reg := preview.NewRegistry()
	reg.Register(&failingPreviewer{})
	reg.Register(&successPreviewer{content: "fallback success"})

	content, err := reg.GetPreview(context.Background(), nil, preview.Object{})

	// Should return the fallback content AND the error from the failed previewer
	assert.Equal(t, content, "fallback success")
	assert.Error(t, err, "boom")
}
