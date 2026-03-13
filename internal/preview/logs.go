package preview

import (
	"bufio"
	"context"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type LogPreviewer struct{}

func (p *LogPreviewer) Priority() int { return 70 }

func (p *LogPreviewer) CanPreview(obj Object) bool {
	return strings.HasSuffix(strings.ToLower(obj.Name), ".log") ||
		strings.Contains(strings.ToLower(obj.Name), "error") ||
		obj.ContentType == "text/plain" // fallback handled by text previewer if not .log
}

func (p *LogPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	var sb strings.Builder
	scanner := bufio.NewScanner(io.LimitReader(rc, 10*1024)) // 10KB limit

	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	debugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	for scanner.Scan() {
		line := scanner.Text()
		upper := strings.ToUpper(line)
		
		switch {
		case strings.Contains(upper, "ERROR") || strings.Contains(upper, "CRITICAL") || strings.Contains(upper, "FATAL"):
			sb.WriteString(errorStyle.Render(line))
		case strings.Contains(upper, "WARN") || strings.Contains(upper, "WARNING"):
			sb.WriteString(warnStyle.Render(line))
		case strings.Contains(upper, "INFO"):
			sb.WriteString(infoStyle.Render(line))
		case strings.Contains(upper, "DEBUG"):
			sb.WriteString(debugStyle.Render(line))
		default:
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	return sb.String(), scanner.Err()
}

func (p *LogPreviewer) SetWidth(width int) {}
