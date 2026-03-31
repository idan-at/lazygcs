package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"gotest.tools/v3/assert"
)

func TestAutoRename_PermissionError(t *testing.T) {
	tempDir := t.TempDir()
	restrictedDir := filepath.Join(tempDir, "restricted")

	err := os.Mkdir(restrictedDir, 0750)
	assert.NilError(t, err)

	filePath := filepath.Join(restrictedDir, "test.txt")

	err = os.Chmod(restrictedDir, 0000)
	assert.NilError(t, err)

	defer func() {
		_ = os.Chmod(restrictedDir, 0600)
	}()

	done := make(chan error)

	go func() {
		_, err := autoRename(filePath, nil)
		done <- err
	}()

	select {
	case err := <-done:
		// We expect it to fallback and not loop infinitely, and return an error.
		assert.Assert(t, err != nil, "Expected autoRename to return an error when os.Stat fails due to permissions")
	case <-time.After(2 * time.Second):
		t.Fatal("autoRename hit an infinite loop due to an unhandled os.Stat error")
	}
}

func TestAutoRename_Limit(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	// Create 100 files to reach the limit
	_ = os.WriteFile(filePath, []byte("original"), 0600)
	for i := 1; i <= 100; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("test_%d.txt", i))
		_ = os.WriteFile(p, []byte("exists"), 0600)
	}

	_, err := autoRename(filePath, nil)
	assert.Assert(t, err != nil, "Expected autoRename to return an error after 100 attempts")
	assert.Assert(t, strings.Contains(err.Error(), "after 100 attempts"), "Expected error message to mention 100 attempts")
}

func TestAutoRename_ActiveDestinations(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	// 1. Base file exists on disk
	_ = os.WriteFile(filePath, []byte("exists"), 0600)

	// 2. test_1.txt doesn't exist on disk, BUT it's in activeDestinations (simulating an active download)
	active := map[string]bool{
		filepath.Join(tempDir, "test_1.txt"): true,
	}

	// 3. autoRename should skip test_1.txt and pick test_2.txt
	got, err := autoRename(filePath, active)
	assert.NilError(t, err)

	expected := filepath.Join(tempDir, "test_2.txt")
	assert.Equal(t, got, expected, "Should have skipped active destination test_1.txt and picked test_2.txt")
}

func TestGetDisplayName(t *testing.T) {
	tests := []struct {
		name          string
		objectName    string
		currentPrefix string
		want          string
	}{
		{
			name:          "File in folder",
			objectName:    "folder/subfolder/file.txt",
			currentPrefix: "folder/subfolder/",
			want:          "file.txt",
		},
		{
			name:          "Folder in folder",
			objectName:    "folder/subfolder/nested/",
			currentPrefix: "folder/subfolder/",
			want:          "nested/",
		},
		{
			name:          "Root file",
			objectName:    "file.txt",
			currentPrefix: "",
			want:          "file.txt",
		},
		{
			name:          "Root folder",
			objectName:    "folder/",
			currentPrefix: "",
			want:          "folder/",
		},
		{
			name:          "No matching prefix",
			objectName:    "other/file.txt",
			currentPrefix: "folder/",
			want:          "other/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDisplayName(tt.objectName, tt.currentPrefix)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestHighlightMatch(t *testing.T) {
	// Force lipgloss to render colors for tests
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#1E1E2E")).Background(lipgloss.Color("#F9E2AF")).Bold(true)

	tests := []struct {
		name    string
		str     string
		query   string
		isFuzzy bool
		want    string
	}{
		{
			name:    "Empty query",
			str:     "hello.txt",
			query:   "",
			isFuzzy: false,
			want:    "hello.txt",
		},
		{
			name:    "No match (exact)",
			str:     "hello.txt",
			query:   "world",
			isFuzzy: false,
			want:    "hello.txt",
		},
		{
			name:    "Exact match (mid)",
			str:     "hello.txt",
			query:   "lo.",
			isFuzzy: false,
			want:    "hel" + style.Render("lo.") + "txt",
		},
		{
			name:    "Exact match (case insensitive)",
			str:     "HeLLo.txt",
			query:   "ell",
			isFuzzy: false,
			want:    "H" + style.Render("eLL") + "o.txt",
		},
		{
			name:    "Fuzzy match (all chars present)",
			str:     "hello.txt",
			query:   "hlt",
			isFuzzy: true,
			want:    style.Render("h") + "e" + style.Render("l") + "lo." + style.Render("t") + "xt",
		},
		{
			name:    "Fuzzy match (case insensitive)",
			str:     "HeLLo.txt",
			query:   "hlt",
			isFuzzy: true,
			want:    style.Render("H") + "e" + style.Render("L") + "Lo." + style.Render("t") + "xt",
		},
		{
			name:    "Fuzzy match (no match)",
			str:     "hello.txt",
			query:   "xyz",
			isFuzzy: true,
			want:    "hello.txt",
		},
		{
			name:    "Fuzzy match (partial match - fails as whole query not matched)",
			str:     "hello.txt",
			query:   "hlx",
			isFuzzy: true,
			// Since our highlightMatch doesn't validate if fuzzyMatch passed,
			// it just highlights the matching characters it finds.
			// The caller is responsible for only passing matching strings.
			want: style.Render("h") + "e" + style.Render("l") + "lo.t" + style.Render("x") + "t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightMatch(tt.str, tt.query, tt.isFuzzy)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestModel_RenderScrollbar(t *testing.T) {
	m := NewModel([]string{"p1"}, nil, "/tmp", false, false)
	m.height = 14 // maxItemsVisible = 14 - 10 = 4

	// 1. No scrollbar needed
	s := m.renderScrollbar(3, 0)
	assert.Equal(t, s, "")

	// 2. Scrollbar at top
	// total=10, visible=4, cursor=0 -> start=0
	// thumbHeight = 4/10 * 4 = 1.6 -> 1
	// thumbStart = 0/10 * 4 = 0
	s = m.renderScrollbar(10, 0)
	lines := strings.Split(s, "\n")
	assert.Equal(t, len(lines), 4)
	assert.Assert(t, strings.Contains(lines[0], "┃"))
	assert.Assert(t, strings.Contains(lines[1], "│"))

	// 3. Scrollbar at bottom
	// total=10, visible=4, cursor=9 -> start=6
	// thumbHeight = 4/10 * 4 = 1.6 -> 1
	// thumbStart = 6/10 * 4 = 2.4 -> 2
	s = m.renderScrollbar(10, 9)
	lines = strings.Split(s, "\n")
	assert.Assert(t, strings.Contains(lines[2], "┃"))
	assert.Assert(t, strings.Contains(lines[3], "│"))
}

func TestSafeJoin(t *testing.T) {
	tmpDir := t.TempDir()
	bucketName := "test-bucket"
	base := filepath.Join(tmpDir, "lazygcs", bucketName)

	tests := []struct {
		name       string
		objectName string
		wantErr    bool
	}{
		{"Normal file", "foo.txt", false},
		{"Subdirectory", "path/to/obj.txt", false},
		{"Absolute-like path", "/abs/path", false},
		{"One level up", "../pwned.txt", true},
		{"Two levels up", "../../pwned.txt", true},
		{"Deeply nested traversal", "sub/../../../pwned.txt", true},
		{"Self", ".", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeJoin(base, tt.objectName)
			if (err != nil) != tt.wantErr {
				t.Errorf("safeJoin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				absBase := filepath.Clean(base)
				if !strings.HasPrefix(got, absBase) {
					t.Errorf("safeJoin() = %q, not under %q", got, absBase)
				}
			}
		})
	}
}
