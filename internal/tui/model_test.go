package tui_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

type mockGCSClient struct {
	projects     []gcs.ProjectBuckets
	objects      *gcs.ObjectList
	contentError error // Used to force an error for GetObjectContent

	lastDownload struct {
		Bucket string
		Object string
		Dest   string
	}
	lastUpload struct {
		Bucket string
		Object string
		Src    string
	}
}

func (f *mockGCSClient) ListBucketsPage(_ context.Context, projectID, _ string, _ int) ([]string, string, error) {
	for _, p := range f.projects {
		if p.ProjectID == projectID {
			return p.Buckets, "", nil
		}
	}
	return nil, "", nil
}

func (f *mockGCSClient) ListObjects(_ context.Context, _, _ string) (*gcs.ObjectList, error) {
	return f.objects, nil
}

func (f *mockGCSClient) ListObjectsPage(_ context.Context, _, _, _ string, _ int) (*gcs.ObjectList, string, error) {
	return f.objects, "", nil
}

func (f *mockGCSClient) GetObjectMetadata(_ context.Context, _, objectName string) (*gcs.ObjectMetadata, error) {
	// Simple mock: find in prefixes or objects
	if f.objects != nil {
		for _, p := range f.objects.Prefixes {
			if p.Name == objectName {
				return &gcs.ObjectMetadata{Name: p.Name, Updated: p.Updated, Created: p.Created, Owner: p.Owner}, nil
			}
		}
		for _, o := range f.objects.Objects {
			if o.Name == objectName {
				return &o, nil
			}
		}
	}
	return nil, fmt.Errorf("not found")
}

func (f *mockGCSClient) GetObjectContent(_ context.Context, _, objectName string) (string, error) {
	if f.contentError != nil {
		return "", f.contentError
	}
	if f.objects != nil {
		for _, o := range f.objects.Objects {
			if o.Name == objectName {
				// Fake content for testing
				return fmt.Sprintf("content of %s", objectName), nil
			}
		}
	}
	return "", fmt.Errorf("not found")
}

func (f *mockGCSClient) DownloadObject(_ context.Context, bucket, object, dest string) error {
	f.lastDownload.Bucket = bucket
	f.lastDownload.Object = object
	f.lastDownload.Dest = dest
	return nil
}

func (f *mockGCSClient) UploadObject(_ context.Context, bucket, object, src string) error {
	f.lastUpload.Bucket = bucket
	f.lastUpload.Object = object
	f.lastUpload.Src = src
	return nil
}

func (f *mockGCSClient) DownloadPrefixAsZip(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *mockGCSClient) NewReader(_ context.Context, _, objectName string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(fmt.Sprintf("content of %s", objectName))), nil
}

func (f *mockGCSClient) NewReaderAt(_ context.Context, _, objectName string) io.ReaderAt {
	return strings.NewReader(fmt.Sprintf("content of %s", objectName))
}

// Helper to create simple object list from names
func simpleObjectList(names []string, prefixes []string) *gcs.ObjectList {
	var objects []gcs.ObjectMetadata
	for _, n := range names {
		objects = append(objects, gcs.ObjectMetadata{Name: n})
	}
	var prefs []gcs.PrefixMetadata
	for _, p := range prefixes {
		prefs = append(prefs, gcs.PrefixMetadata{Name: p})
	}
	return &gcs.ObjectList{Objects: objects, Prefixes: prefs}
}

func updateModel(m tui.Model, msg tea.Msg) (tui.Model, tea.Cmd) {
	if msg == nil {
		return m, nil
	}
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var finalCmds []tea.Cmd
		for _, cmd := range batchMsg {
			if cmd != nil {
				resM, resCmd := updateModel(m, cmd())
				m = resM
				if resCmd != nil {
					finalCmds = append(finalCmds, resCmd)
				}
			}
		}
		return m, tea.Batch(finalCmds...)
	}
	updatedM, cmd := m.Update(msg)
	return updatedM.(tui.Model), cmd
}

func setupTestModel(projects []gcs.ProjectBuckets, objects *gcs.ObjectList, downloadDir string) (tui.Model, *mockGCSClient) {
	client := &mockGCSClient{
		projects: projects,
		objects:  objects,
	}
	m := tui.NewModel([]string{"p1"}, client, downloadDir, false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 150, Height: 50})
	return m, client
}

func enterBucket(m tui.Model, projects []gcs.ProjectBuckets, bucket string, objects *gcs.ObjectList) tui.Model {
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: projects[0].ProjectID, Buckets: projects[0].Buckets})
	m, _ = pressKey(m, 'j')
	m, _ = pressKeyType(m, tea.KeyEnter)
	if objects != nil {
		m, _ = updateModel(m, tui.ObjectsMsg{Bucket: bucket, Prefix: "", List: objects})
	}
	return m
}

func pressKey(m tui.Model, key rune) (tui.Model, tea.Cmd) {
	return updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
}

func pressKeyType(m tui.Model, keyType tea.KeyType) (tui.Model, tea.Cmd) {
	return updateModel(m, tea.KeyMsg{Type: keyType})
}

func resolveFetchCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()

	// Helper to resolve nested messages (like from AddMessage or Debounce)
	var resolve func(tea.Msg) tea.Msg
	resolve = func(m tea.Msg) tea.Msg {
		if m == nil {
			return nil
		}
		if batchMsg, ok := m.(tea.BatchMsg); ok {
			for _, c := range batchMsg {
				if c != nil {
					res := resolve(c())
					if res != nil {
						return res
					}
				}
			}
			return nil
		}
		if dMsg, ok := m.(tui.DebouncePreviewMsg); ok {
			if dMsg.FetchCmd != nil {
				return resolve(dMsg.FetchCmd())
			}
		}
		if hMsg, ok := m.(tui.HoverPrefetchTickMsg); ok {
			if hMsg.FetchCmd != nil {
				return resolve(hMsg.FetchCmd())
			}
		}
		// Skip UI infrastructure messages
		if _, ok := m.(tui.ClearStatusMsg); ok {
			return nil
		}
		if _, ok := m.(spinner.TickMsg); ok {
			return nil
		}
		return m
	}

	return resolve(msg)
}

func resolveAllFetchCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	var msgs []tea.Msg

	var resolve func(tea.Msg)
	resolve = func(m tea.Msg) {
		if m == nil {
			return
		}
		if batchMsg, ok := m.(tea.BatchMsg); ok {
			for _, c := range batchMsg {
				if c != nil {
					resolve(c())
				}
			}
			return
		}
		// Skip UI infrastructure messages
		if _, ok := m.(tui.ClearStatusMsg); ok {
			return
		}
		if _, ok := m.(spinner.TickMsg); ok {
			return
		}
		msgs = append(msgs, m)
	}

	resolve(msg)
	return msgs
}

func TestModel_UI_WrappingBug(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	view := m.View()
	lines := strings.Split(view, "\n")

	assert.Assert(t, len(lines) <= 50, "View height %d exceeded window height 50 due to wrapping", len(lines))
}

func TestModel_ModernSelectionUI(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1"}, nil)
	m, client := setupTestModel(projects, objects, "/tmp")
	_ = client

	// Enter bucket and load objects
	m = enterBucket(m, []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}, "b1", client.objects)

	view := m.View()
	// Old indicators should be gone
	assert.Assert(t, !strings.Contains(view, ">"), "View should not contain old cursor '>'")
	assert.Assert(t, !strings.Contains(view, "[ ]"), "View should not contain old unselected indicator '[ ]'")

	// Select the item
	m, _ = pressKey(m, ' ')

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "[x]"), "View should not contain old selected indicator '[x]'")
	assert.Assert(t, strings.Contains(view, "✓"), "View should contain new selection indicator '✓'")
}

func TestModel_HelpMenu(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	// Assert help menu is not shown initially
	view := m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"))

	// Press '?' to show help
	m, _ = pressKey(m, '?')

	view = m.View()
	// In 'Which Key' style, we should see both the main content AND the help at the bottom
	assert.Assert(t, strings.Contains(view, "Buckets"), "Buckets column should still be visible")
	assert.Assert(t, strings.Contains(view, "HELP"), "View should contain 'HELP' header")
	assert.Assert(t, !strings.Contains(view, "WHICH-KEY"), "View should NOT contain 'WHICH-KEY' anymore")
	assert.Assert(t, strings.Contains(view, "toggle help"), "View should list the help keybind")

	// Press '?' again to hide help
	m, _ = pressKey(m, '?')

	view = m.View()
	assert.Assert(t, !strings.Contains(view, "HELP"), "View should no longer contain 'HELP'")
}

func TestModel_Update_Quit(t *testing.T) {
	client := &mockGCSClient{projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	m.Update(tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1"}})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.Assert(t, cmd != nil)
	assert.Assert(t, cmd() == tea.Quit())
}

func TestModel_Resize(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	m, client := setupTestModel(projects, nil, "/tmp")
	_ = client

	view := m.View()
	assert.Assert(t, len(view) > 0)

	// Very narrow
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 20, Height: 10})
	viewNarrow := m.View()

	assert.Assert(t, len(viewNarrow) > 0)
}

func TestModel_LayoutIntegrity(t *testing.T) {
	// 1. Setup model with fixed dimensions and a very wide/long preview content
	wideContent := strings.Repeat("THIS LINE IS VERY VERY WIDE AND SHOULD BE TRUNCATED BY THE UI TO PREVENT COLUMN EXPANSION ", 10)
	longContent := ""
	for i := 0; i < 100; i++ {
		longContent += fmt.Sprintf("Line %d\n", i)
	}

	client := &mockGCSClient{
		projects: []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}},
		objects:  simpleObjectList([]string{"obj1"}, nil),
	}
	m := tui.NewModel([]string{"p1"}, client, "/tmp", false, false)
	width := 100
	height := 20
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: width, Height: height})

	// Enter bucket to show preview
	m = enterBucket(m, client.projects, "b1", nil)
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: client.objects})

	// Inject the oversized content
	m, _ = updateModel(m, tui.ContentMsg{ObjectName: "obj1", Content: wideContent + "\n" + longContent})

	view := m.View()
	lines := strings.Split(view, "\n")

	// 2. Assert Height: The total view height should not exceed the requested height.
	assert.Assert(t, len(lines) <= height, "View height (%d) exceeds requested height (%d)", len(lines), height)

	// 3. Assert Width: Each line of the content should not exceed the terminal width.
	for i, line := range lines {
		w := lipgloss.Width(line)
		assert.Assert(t, w <= width, "Line %d width (%d) exceeds requested width (%d): %q", i, w, width, line)
	}
}

func TestModel_UI_Wrapping_Bug_Detected(t *testing.T) {
	// This test specifically checks if the rendered columns have the expected height.
	// If wrapping occurs, the number of lines will exceed columnHeight.
	m := tui.NewModel([]string{"p1"}, nil, "/tmp", false, false)
	m, _ = updateModel(m, tea.WindowSizeMsg{Width: 100, Height: 20})

	// maxVisible = 10. columnHeight = 12.
	// We'll add 10 buckets.
	var buckets []string
	for i := 0; i < 10; i++ {
		buckets = append(buckets, "a_fairly_long_bucket_name_to_trigger_wrapping")
	}
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: buckets}}
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: projects[0].ProjectID, Buckets: projects[0].Buckets})

	view := m.View()
	lines := strings.Split(view, "\n")

	assert.Assert(t, len(lines) <= 20, "View height %d exceeded terminal height 20. Wrapping likely occurred!", len(lines))
}
