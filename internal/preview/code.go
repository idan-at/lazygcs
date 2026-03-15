// Package preview provides functionality for preview.
package preview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
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

	if IsBinary(content) {
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

	style := styles.Get("monokai")
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

	return buf.String(), nil
}
