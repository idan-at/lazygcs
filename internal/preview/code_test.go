package preview_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/idan-at/lazygcs/internal/preview"
	"gotest.tools/v3/assert"
)

func TestHighlight_Go(t *testing.T) {
	content := "package main\n\nfunc main() {}"
	out, err := preview.Highlight("main.go", content)
	assert.NilError(t, err)

	// We check for the presence of the keywords.
	assert.Assert(t, strings.Contains(out, "package"))
	assert.Assert(t, strings.Contains(out, "main"))
}

func TestHighlight_ConfAndProperties(t *testing.T) {
	content := "foo=bar\n[sec]\nval=1"
	outConf, err := preview.Highlight("app.conf", content)
	assert.NilError(t, err)

	outProps, err := preview.Highlight("app.properties", content)
	assert.NilError(t, err)

	// Should contain content and ANSI escape codes for formatting
	assert.Assert(t, strings.Contains(outConf, "foo"))
	assert.Assert(t, strings.Contains(outConf, "\x1b["))
	assert.Assert(t, strings.Contains(outProps, "foo"))
	assert.Assert(t, strings.Contains(outProps, "\x1b["))
}

func TestHighlight_DDL(t *testing.T) {
	content := "CREATE TABLE users (id INT PRIMARY KEY);"
	out, err := preview.Highlight("schema.ddl", content)
	assert.NilError(t, err)

	// Compare with explicit .sql highlighting to ensure consistency
	outSQL, err := preview.Highlight("schema.sql", content)
	assert.NilError(t, err)

	assert.Equal(t, out, outSQL, "DDL should be highlighted the same as SQL")
}

func TestHighlight_Shell(t *testing.T) {
	content := "echo 'hello world'"
	outSH, err := preview.Highlight("script.sh", content)
	assert.NilError(t, err)

	outZSH, err := preview.Highlight("script.zsh", content)
	assert.NilError(t, err)

	outBASH, err := preview.Highlight("script.bash", content)
	assert.NilError(t, err)

	// Compare with plain text to ensure it's actually highlighted
	outPlain, err := preview.Highlight("script.txt", content)
	assert.NilError(t, err)
	assert.Assert(t, outSH != outPlain, ".sh should be highlighted, not plain text")

	// Verify all shell extensions yield identical highlighting
	assert.Equal(t, outSH, outBASH, ".sh should be highlighted the same as .bash")
	assert.Equal(t, outZSH, outBASH, ".zsh should be highlighted the same as .bash")
}

func TestHighlight_ShellAnalyse(t *testing.T) {
	// A file with no extension but a shebang should be highlighted as shell
	content := "#!/bin/bash\necho 'hello world'"
	out, err := preview.Highlight("script-no-ext", content)
	assert.NilError(t, err)

	// Compare with plain text (which wouldn't have the ANSI codes for 'echo')
	outPlain, err := preview.Highlight("script.txt", content)
	assert.NilError(t, err)

	assert.Assert(t, out != outPlain, "file with shebang should be highlighted via Analyse fallback")
}

func TestCodePreviewer_CanPreview_Shell(t *testing.T) {
	p := &preview.CodePreviewer{}

	extensions := []string{"script.sh", "script.bash", "script.zsh"}
	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			obj := preview.Object{
				Bucket: "b1",
				Name:   ext,
			}
			assert.Assert(t, p.CanPreview(obj), "CodePreviewer should be able to preview %s files", ext)
		})
	}
}

func TestCodePreviewer_CanPreview_DDL(t *testing.T) {
	p := &preview.CodePreviewer{}
	obj := preview.Object{
		Bucket: "b1",
		Name:   "schema.ddl",
	}
	assert.Assert(t, p.CanPreview(obj), "CodePreviewer should be able to preview .ddl files")
}

type mockGCSClientForCode struct {
	content []byte
}

func (m *mockGCSClientForCode) NewReader(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.content)), nil
}

func (m *mockGCSClientForCode) GetObjectContent(_ context.Context, _, _ string) (string, error) {
	return string(m.content), nil
}

func (m *mockGCSClientForCode) DownloadObject(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockGCSClientForCode) DownloadPrefixAsZip(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockGCSClientForCode) NewReaderAt(_ context.Context, _, _ string) io.ReaderAt {
	return bytes.NewReader(m.content)
}

func TestCodePreviewer_ZeroSizeBug(t *testing.T) {
	client := &mockGCSClientForCode{
		content: []byte("package main\n\nfunc main() {}"),
	}

	p := &preview.CodePreviewer{}

	// Intentionally setting size to 0 to simulate the bug scenario
	obj := preview.Object{
		Bucket: "b1",
		Name:   "main.go",
		Size:   0,
	}

	out, err := p.Preview(context.Background(), client, obj)
	assert.NilError(t, err)

	assert.Assert(t, strings.Contains(out, "package"))
	assert.Assert(t, strings.Contains(out, "main"))
}

func TestCodePreviewer_LargeFileLimit(t *testing.T) {
	// Create content larger than 10KB
	content := bytes.Repeat([]byte("a"), 15*1024)
	client := &mockGCSClientForCode{
		content: content,
	}

	p := &preview.CodePreviewer{}

	obj := preview.Object{
		Bucket: "b1",
		Name:   "large.txt",
		Size:   int64(len(content)),
	}

	out, err := p.Preview(context.Background(), client, obj)
	assert.NilError(t, err)

	assert.Assert(t, out != "")
	// Should be truncated at 10KB, with some highlighting output wrapper
	assert.Assert(t, len(out) >= 10*1024)
}
