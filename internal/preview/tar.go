package preview

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TarPreviewer ...
type TarPreviewer struct{}

// Priority ...
func (p *TarPreviewer) Priority() int { return 41 }

// CanPreview ...
func (p *TarPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	if strings.HasSuffix(obj.Name, ".tar.gz") || strings.HasSuffix(obj.Name, ".tgz") {
		return true
	}
	return ext == ".tar" || obj.ContentType == "application/x-tar" || obj.ContentType == "application/gzip"
}

// Preview ...
func (p *TarPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	// Limit to 2MB of compressed stream to prevent massive layers from hanging the UI
	var r io.Reader
	r = io.LimitReader(rc, 2*1024*1024)
	if strings.HasSuffix(obj.Name, ".tar.gz") || strings.HasSuffix(obj.Name, ".tgz") || obj.ContentType == "application/gzip" {
		gr, err := gzip.NewReader(r)
		if err != nil {
			return "", fmt.Errorf("failed to open gzip: %w", err)
		}
		defer func() { _ = gr.Close() }()
		r = gr
	}

	tr := tar.NewReader(r)
	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))

	fmt.Fprintf(&sb, "%s\n", headerStyle.Render("Archive contents (streamed):"))

	maxFiles := 100
	count := 0
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A limited reader might cause unexpected EOF. Gracefully truncate.
			if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "unexpected EOF") {
				fmt.Fprintf(&sb, "%s\n", headerStyle.Render("... listing truncated (archive too large)"))
				return sb.String(), nil
			}
			return sb.String(), fmt.Errorf("error reading tar: %w", err)
		}

		if count >= maxFiles {
			fmt.Fprintf(&sb, "%s\n", headerStyle.Render("... listing truncated"))
			break
		}

		name := header.Name
		if header.Typeflag == tar.TypeDir {
			sb.WriteString(dirStyle.Render("  "+name) + "\n")
		} else {
			sb.WriteString(fileStyle.Render("  "+name) + "\n")
		}
		count++
	}

	return sb.String(), nil
}

// SetWidth ...
func (p *TarPreviewer) SetWidth(_ int) {}
