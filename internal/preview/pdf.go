package preview

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dslipak/pdf"
)

type PDFPreviewer struct{}

func (p *PDFPreviewer) Priority() int { return 60 }

func (p *PDFPreviewer) CanPreview(obj Object) bool {
	return strings.ToLower(obj.ContentType) == "application/pdf" ||
		strings.HasSuffix(strings.ToLower(obj.Name), ".pdf")
}

func (p *PDFPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	ra := client.NewReaderAt(ctx, obj.Bucket, obj.Name)
	
	reader, err := pdf.NewReader(ra, obj.Size)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	valStyle := lipgloss.NewStyle().Bold(true)

	sb.WriteString(headerStyle.Render("PDF Metadata:") + "\n")

	info := reader.Trailer().Key("Info")
	if info.Kind() == pdf.Dict {
		keys := []string{"Title", "Author", "Subject", "Keywords", "Creator", "Producer", "CreationDate", "ModDate"}
		for _, k := range keys {
			val := info.Key(k)
			if val.Kind() == pdf.String {
				fmt.Fprintf(&sb, "%s %s\n", keyStyle.Render(k+":"), valStyle.Render(val.String()))
			}
		}
	}

	fmt.Fprintf(&sb, "%s %s\n", keyStyle.Render("Pages:"), valStyle.Render(fmt.Sprint(reader.NumPage())))

	return sb.String(), nil
}

func (p *PDFPreviewer) SetWidth(width int) {}
