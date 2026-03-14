package preview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"os"
	"path/filepath"
	"strings"

	"github.com/dolmen-go/kittyimg"
	"github.com/nfnt/resize"
	"github.com/qeesung/image2ascii/convert"
	_ "golang.org/x/image/bmp"  // Register BMP decoder
	_ "golang.org/x/image/tiff" // Register TIFF decoder
	_ "golang.org/x/image/webp" // Register WebP decoder
)

// ImagePreviewer handles image files.
type ImagePreviewer struct {
	width int
}

// Priority defines the order of evaluation.
func (p *ImagePreviewer) Priority() int { return 25 }

// CanPreview determines if this previewer should handle the object.
func (p *ImagePreviewer) CanPreview(obj Object) bool {
	// Limit inline image previews to 5MB to prevent memory exhaustion/hangs
	if obj.Size > 5*1024*1024 {
		return false
	}
	if strings.HasPrefix(obj.ContentType, "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(obj.Name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff":
		return true
	}
	return false
}

// Preview renders the image to an ANSI string.
func (p *ImagePreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	img, _, err := image.Decode(rc)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	// Downscale large images to a sensible max size to prevent generating
	// massive strings (MBs) that crash the TUI's rendering engine.
	const maxWidth = 800
	const maxHeight = 800

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	if width > maxWidth || height > maxHeight {
		// Calculate proportional scaling
		if float64(width)/float64(maxWidth) > float64(height)/float64(maxHeight) {
			img = resize.Resize(maxWidth, 0, img, resize.Lanczos3)
		} else {
			img = resize.Resize(0, maxHeight, img, resize.Lanczos3)
		}
	}

	if supportsKitty() {
		return renderKitty(img, p.width)
	}
	return renderASCII(img, p.width)
}

// SetWidth updates the preferred rendering width.
func (p *ImagePreviewer) SetWidth(width int) {
	p.width = width
}

func supportsKitty() bool {
	term := os.Getenv("TERM")
	if strings.Contains(term, "kitty") || strings.Contains(term, "wezterm") || strings.Contains(term, "ghostty") {
		return true
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("WEZTERM_PANE") != "" {
		return true
	}
	return false
}

func renderKitty(img image.Image, viewportWidth int) (string, error) {
	var buf bytes.Buffer
	if err := kittyimg.Fprint(&buf, img); err != nil {
		return "", fmt.Errorf("kitty render: %w", err)
	}
	seq := buf.String()

	widthPx := img.Bounds().Dx()
	heightPx := img.Bounds().Dy()

	if widthPx <= 0 {
		widthPx = 1
	}
	if heightPx <= 0 {
		heightPx = 1
	}

	// Rough estimate: cell is 10px wide, 20px high
	cols := viewportWidth
	if widthPx/10 < cols {
		cols = widthPx / 10
	}
	if cols <= 0 {
		cols = 1
	}

	// Calculate lines to reserve
	rows := (cols * heightPx) / (widthPx * 2)
	if rows <= 0 {
		rows = 1
	}

	// Cap rows to prevent vertical overflow that bleeds past the terminal height.
	// 25 is a safe generic maximum for typical terminal heights.
	if rows > 25 {
		rows = 25
		// Adjust cols to maintain aspect ratio
		cols = (rows * widthPx * 2) / heightPx
		if cols <= 0 {
			cols = 1
		}
	}

	// Insert C=1 (do not move cursor), c=cols, r=rows into kitty sequence
	seq = strings.Replace(seq, "q=1,", fmt.Sprintf("q=1,C=1,c=%d,r=%d,", cols, rows), 1)

	// Add newlines so the viewport reserves the space
	return seq + strings.Repeat("\n", rows), nil
}

func renderASCII(img image.Image, viewportWidth int) (string, error) {
	converter := convert.NewImageConverter()
	opts := convert.DefaultOptions

	widthPx := img.Bounds().Dx()
	heightPx := img.Bounds().Dy()
	if widthPx <= 0 {
		widthPx = 1
	}

	// Terminal cells are roughly 2x as tall as they are wide.
	calculatedHeight := (viewportWidth * heightPx) / (widthPx * 2)
	if calculatedHeight <= 0 {
		calculatedHeight = 1
	}

	opts.FixedWidth = viewportWidth
	opts.FixedHeight = calculatedHeight
	opts.FitScreen = false

	res := converter.Image2ASCIIString(img, &opts)
	return res, nil
}
