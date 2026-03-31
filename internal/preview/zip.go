package preview

import (
	"archive/zip"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ZipPreviewer ...
type ZipPreviewer struct{}

// Priority ...
func (p *ZipPreviewer) Priority() int { return 40 }

// CanPreview ...
func (p *ZipPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	return ext == ".zip" || ext == ".jar" || ext == ".war" || obj.ContentType == "application/zip"
}

// Preview ...
func (p *ZipPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	// Limit zip preview to 100MB to prevent excessive memory usage reading central directory
	if obj.Size > 100*1024*1024 {
		return "(zip archive too large to preview)", nil
	}

	ra := client.NewReaderAt(ctx, obj.Bucket, obj.Name)

	zr, err := zip.NewReader(ra, obj.Size)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))

	fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("Archive contains %d files:", len(zr.File))))

	// Limit output to first 100 files
	maxFiles := 100
	for i, f := range zr.File {
		if i >= maxFiles {
			fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("... and %d more files", len(zr.File)-maxFiles)))
			break
		}

		name := f.Name
		if f.FileInfo().IsDir() {
			sb.WriteString(dirStyle.Render("  "+name) + "\n")
		} else {
			sb.WriteString(fileStyle.Render("  "+name) + "\n")
		}
	}

	return sb.String(), nil
}

// SetWidth ...
func (p *ZipPreviewer) SetWidth(_ int) {}
