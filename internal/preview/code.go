// Package preview provides functionality for preview.
package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/idan-at/lazygcs/internal/util"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// CodePreviewer ...
type CodePreviewer struct{}

// Priority ...
func (p *CodePreviewer) Priority() int { return 90 }

// getLexer attempts to find the appropriate chroma.Lexer for a given filename.
func getLexer(filename string) chroma.Lexer {
	lexer := lexers.Get(filename)
	if lexer == nil {
		lexer = lexers.Match(filename)
	}
	if lexer == nil {
		lowerName := strings.ToLower(filename)
		if strings.HasSuffix(lowerName, ".conf") || strings.HasSuffix(lowerName, ".properties") {
			lexer = lexers.Get("ini")
		} else if strings.HasSuffix(lowerName, ".ddl") {
			lexer = lexers.Match("f.sql")
		} else if strings.HasSuffix(lowerName, ".sh") ||
			strings.HasSuffix(lowerName, ".zsh") ||
			strings.HasSuffix(lowerName, ".bash") ||
			strings.HasSuffix(lowerName, ".bashrc") ||
			strings.HasSuffix(lowerName, ".zshrc") ||
			strings.HasSuffix(lowerName, ".profile") ||
			strings.HasSuffix(lowerName, "bashrc") ||
			strings.HasSuffix(lowerName, "zshrc") {
			lexer = lexers.Get("bash")
		}
	}
	return lexer
}

// CanPreview ...
func (p *CodePreviewer) CanPreview(obj Object) bool {
	return getLexer(obj.Name) != nil
}

// Preview ...
func (p *CodePreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	lr := io.LimitReader(rc, 10*1024)
	buf, err := io.ReadAll(lr)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	content := string(buf)

	if util.IsBinary(content) {
		return "(binary content)", nil
	}

	return Highlight(obj.Name, content)
}

// SetWidth ...
func (p *CodePreviewer) SetWidth(_ int) {}

// Highlight ...
func Highlight(filename, content string) (string, error) {
	lexer := getLexer(filename)
	if lexer == nil {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("catppuccin-mocha")
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content, nil //nolint:nilerr
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return content, nil //nolint:nilerr
	}

	// Split chroma output and add styled line numbers
	lines := strings.Split(buf.String(), "\n")

	// If the last line is completely empty (often due to trailing newline), drop it from numbering
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Dynamic padding for line numbers
	maxLen := len(fmt.Sprintf("%d", len(lines)))
	if maxLen < 3 {
		maxLen = 3
	}

	numberStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A6ADC8")). // Dimmed text
		Faint(true)

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#414559")) // Dimmed border

	var result strings.Builder
	for i, line := range lines {
		lineNum := fmt.Sprintf("%*d", maxLen, i+1)
		result.WriteString(numberStyle.Render(lineNum) + " " + separatorStyle.Render("│") + " " + line + "\n")
	}

	return result.String(), nil
}
