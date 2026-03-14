package preview

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"strings"
	"testing"
)

type mockGCSClientForImage struct {
	content []byte
}

func (m *mockGCSClientForImage) GetObjectContent(_ context.Context, _, _ string) (string, error) {
	return string(m.content), nil
}

func (m *mockGCSClientForImage) NewReader(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(string(m.content))), nil
}

func (m *mockGCSClientForImage) NewReaderAt(_ context.Context, _, _ string) io.ReaderAt {
	return strings.NewReader(string(m.content))
}

func TestImagePreviewer_CanPreview(t *testing.T) {
	p := &ImagePreviewer{}
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{"o.png", "image/png", true},
		{"o.jpg", "image/jpeg", true},
		{"o.gif", "application/octet-stream", true}, // Extension based
		{"o.webp", "", true},                        // Extension based
		{"o.svg", "image/svg+xml", true},            // SVG
		{"o.svg", "", true},                         // SVG extension based
		{"o.txt", "text/plain", false},
		{"o.pdf", "application/pdf", false},
	}

	for _, tt := range tests {
		obj := Object{Name: tt.name, ContentType: tt.contentType}
		if p.CanPreview(obj) != tt.expected {
			t.Errorf("CanPreview(%s, %s) = %v, want %v", tt.name, tt.contentType, !tt.expected, tt.expected)
		}
	}
}

func TestImagePreviewer_Preview(t *testing.T) {
	// Create a simple 2x2 red PNG
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	pr, pw := io.Pipe()
	go func() {
		_ = png.Encode(pw, img)
		_ = pw.Close()
	}()
	content, _ := io.ReadAll(pr)

	client := &mockGCSClientForImage{content: content}
	p := &ImagePreviewer{}
	p.SetWidth(80)

	obj := Object{Bucket: "b", Name: "o.png", ContentType: "image/png"}
	res, err := p.Preview(context.Background(), client, obj)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if res == "" {
		t.Error("Preview returned empty string")
	}

	// Verify it contains either ANSI color codes or Kitty sequence
	if !strings.Contains(res, "\x1b") {
		t.Error("Preview does not contain escape sequences")
	}
}

func TestImagePreviewer_Preview_SVG(t *testing.T) {
	svgContent := `<svg width="100" height="100"><rect width="100" height="100" style="fill:rgb(0,0,255);" /></svg>`
	client := &mockGCSClientForImage{content: []byte(svgContent)}
	p := &ImagePreviewer{}
	p.SetWidth(80)

	obj := Object{Bucket: "b", Name: "o.svg", ContentType: "image/svg+xml"}
	res, err := p.Preview(context.Background(), client, obj)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if res == "" {
		t.Error("Preview returned empty string")
	}

	// Verify it contains either ANSI color codes or Kitty sequence
	if !strings.Contains(res, "\x1b") {
		t.Error("Preview does not contain escape sequences")
	}
}

func TestImagePreviewer_Preview_GIF(t *testing.T) {
	// Create a simple 1x1 blue GIF
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.RGBA{0, 0, 255, 255}, color.RGBA{0, 0, 0, 255}})
	img.Set(0, 0, color.RGBA{0, 0, 255, 255})

	pr, pw := io.Pipe()
	go func() {
		_ = gif.Encode(pw, img, nil)
		_ = pw.Close()
	}()
	content, _ := io.ReadAll(pr)

	client := &mockGCSClientForImage{content: content}
	p := &ImagePreviewer{}
	p.SetWidth(80)

	obj := Object{Bucket: "b", Name: "o.gif", ContentType: "application/octet-stream"}
	res, err := p.Preview(context.Background(), client, obj)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if res == "" {
		t.Error("Preview returned empty string")
	}

	// Verify it contains either ANSI color codes or Kitty sequence
	if !strings.Contains(res, "\x1b") {
		t.Error("Preview does not contain escape sequences")
	}
}
