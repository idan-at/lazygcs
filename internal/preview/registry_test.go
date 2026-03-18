package preview_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/idan-at/lazygcs/internal/preview"
	"gotest.tools/v3/assert"
)

// mockPreviewer is a simple previewer for testing registry logic.
type mockPreviewer struct {
	name     string
	priority int
	match    func(preview.Object) bool
}

func (m *mockPreviewer) Priority() int                      { return m.priority }
func (m *mockPreviewer) CanPreview(obj preview.Object) bool { return m.match(obj) }
func (m *mockPreviewer) Preview(_ context.Context, _ preview.GCSClient, _ preview.Object) (string, error) {
	return m.name, nil
}
func (m *mockPreviewer) SetWidth(_ int) {}

func TestRegistry_Logic(t *testing.T) {
	reg := preview.NewRegistry()

	// Register out of order to test sorting
	reg.Register(&mockPreviewer{name: "low", priority: 100, match: func(_ preview.Object) bool { return true }})
	reg.Register(&mockPreviewer{name: "high", priority: 10, match: func(o preview.Object) bool { return strings.HasSuffix(o.Name, ".high") }})
	reg.Register(&mockPreviewer{name: "mid", priority: 50, match: func(o preview.Object) bool { return strings.HasSuffix(o.Name, ".mid") }})

	testCases := []struct {
		filename string
		expected string
	}{
		{"test.high", "high"},
		{"test.mid", "mid"},
		{"test.other", "low"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			out, err := reg.GetPreview(context.Background(), nil, preview.Object{Name: tc.filename})
			assert.NilError(t, err)
			assert.Equal(t, out, tc.expected)
		})
	}
}

func TestRegistry_Routing(t *testing.T) {
	reg := preview.NewDefaultRegistry()

	// Using a more realistic mock client that doesn't return empty for NewReader
	client := &mockPreviewGCSClient{content: []byte("dummy content")}

	testCases := []struct {
		name string
		obj  preview.Object
	}{
		{name: "Markdown", obj: preview.Object{Name: "README.md", Size: 100}},
		{name: "JSON", obj: preview.Object{Name: "data.json", ContentType: "application/json", Size: 100}},
		{name: "YAML", obj: preview.Object{Name: "config.yaml", Size: 100}},
		{name: "CSV", obj: preview.Object{Name: "data.csv", Size: 100}},
		{name: "Image", obj: preview.Object{Name: "test.png", ContentType: "image/png", Size: 100}},
		{name: "PDF", obj: preview.Object{Name: "doc.pdf", Size: 100}},
		{name: "Go Code", obj: preview.Object{Name: "main.go", Size: 100}},
		{name: "Python Code", obj: preview.Object{Name: "app.py", Size: 100}},
		{name: "Log", obj: preview.Object{Name: "sys.log", Size: 100}},
		{name: "Zip", obj: preview.Object{Name: "archive.zip", Size: 100}},
		{name: "Tar", obj: preview.Object{Name: "archive.tar", Size: 100}},
		{name: "Docker Archive", obj: preview.Object{Name: "docker.tar", Size: 100}},
		{name: "Text", obj: preview.Object{Name: "notes.txt", Size: 100}},
		{name: "Unknown (Fallback)", obj: preview.Object{Name: "something.unknown", Size: 100}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := reg.GetPreview(context.Background(), client, tc.obj)
			assert.NilError(t, err, "failed to get preview for %s", tc.name)
			assert.Assert(t, out != "", "expected non-empty preview for %s", tc.name)
			assert.Assert(t, out != "(no preview available)", "expected a valid previewer to catch %s", tc.name)
		})
	}
}

func TestRegistry_NoMatch(t *testing.T) {
	reg := preview.NewRegistry() // Empty registry
	out, err := reg.GetPreview(context.Background(), nil, preview.Object{Name: "test.txt"})
	assert.NilError(t, err)
	assert.Equal(t, out, "(no preview available)")
}

func TestRegistry_PreviewErrorFallback(t *testing.T) {
	reg := preview.NewRegistry()

	// High priority that fails
	reg.Register(&mockErrorPreviewer{priority: 10})
	// Low priority that succeeds
	reg.Register(&mockPreviewer{name: "fallback", priority: 100, match: func(_ preview.Object) bool { return true }})

	out, err := reg.GetPreview(context.Background(), nil, preview.Object{Name: "test.txt"})
	assert.NilError(t, err)
	assert.Equal(t, out, "fallback")
}

type mockErrorPreviewer struct {
	priority int
}

func (m *mockErrorPreviewer) Priority() int                    { return m.priority }
func (m *mockErrorPreviewer) CanPreview(_ preview.Object) bool { return true }
func (m *mockErrorPreviewer) Preview(_ context.Context, _ preview.GCSClient, _ preview.Object) (string, error) {
	return "", fmt.Errorf("boom")
}
func (m *mockErrorPreviewer) SetWidth(_ int) {}
