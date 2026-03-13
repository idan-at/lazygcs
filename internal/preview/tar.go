package preview

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TarPreviewer struct{}

func (p *TarPreviewer) Priority() int { return 41 }

func (p *TarPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	if strings.HasSuffix(obj.Name, ".tar.gz") || strings.HasSuffix(obj.Name, ".tgz") {
		return true
	}
	return ext == ".tar" || obj.ContentType == "application/x-tar" || obj.ContentType == "application/gzip"
}

func (p *TarPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var r io.Reader = rc
	if strings.HasSuffix(obj.Name, ".tar.gz") || strings.HasSuffix(obj.Name, ".tgz") || obj.ContentType == "application/gzip" {
		gr, err := gzip.NewReader(rc)
		if err != nil {
			return "", fmt.Errorf("failed to open gzip: %w", err)
		}
		defer gr.Close()
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
		if err == io.EOF {
			break
		}
		if err != nil {
			return sb.String(), fmt.Errorf("error reading tar: %w", err)
		}

		if count >= maxFiles {
			fmt.Fprintf(&sb, "%s\n", headerStyle.Render("... listing truncated"))
			break
		}

		name := header.Name
		if header.Typeflag == tar.TypeDir {
			sb.WriteString(dirStyle.Render("  " + name) + "\n")
		} else {
			sb.WriteString(fileStyle.Render("  " + name) + "\n")
		}
		count++
	}

	return sb.String(), nil
}

func (p *TarPreviewer) SetWidth(width int) {}
