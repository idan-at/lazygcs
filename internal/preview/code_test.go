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
