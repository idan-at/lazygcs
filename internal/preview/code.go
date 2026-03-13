package preview

import (
	"bytes"
	"context"
	"io"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

type CodePreviewer struct{}

func (p *CodePreviewer) Priority() int { return 90 }

func (p *CodePreviewer) CanPreview(obj Object) bool {
	lexer := lexers.Get(obj.Name)
	if lexer == nil {
		lexer = lexers.Match(obj.Name)
	}
	return lexer != nil
}

func (p *CodePreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	limit := int64(10 * 1024)
	if obj.Size < limit {
		limit = obj.Size
	}
	buf := make([]byte, limit)
	n, err := io.ReadFull(rc, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	content := string(buf[:n])

	if IsBinary(content) {
		return "(binary content)", nil
	}

	return Highlight(obj.Name, content)
}

func (p *CodePreviewer) SetWidth(width int) {}

func Highlight(filename, content string) (string, error) {
	lexer := lexers.Get(filename)
	if lexer == nil {
		lexer = lexers.Match(filename)
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
		return content, nil
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return content, nil
	}

	return buf.String(), nil
}
