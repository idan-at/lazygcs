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

// CanPreview ...
func (p *CodePreviewer) CanPreview(obj Object) bool {
	lexer := lexers.Get(obj.Name)
	if lexer == nil {
		lexer = lexers.Match(obj.Name)
	}
	return lexer != nil
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
	lexer := lexers.Get(filename)
	if lexer == nil {
		lexer = lexers.Match(filename)
	}
	if lexer == nil {
		if strings.HasSuffix(filename, ".conf") || strings.HasSuffix(filename, ".properties") {
			lexer = lexers.Get("ini")
		}
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
