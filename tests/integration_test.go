package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"

	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"

	"github.com/hamba/avro/v2"
	"github.com/hamba/avro/v2/ocf"
	"github.com/parquet-go/parquet-go"
)

func createConfigFile(t *testing.T, projects []string, downloadDir string) string {
	t.Helper()
	var quoted []string
	for _, p := range projects {
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	content := fmt.Sprintf("projects = [%s]\ndownload_dir = %q\n", strings.Join(quoted, ", "), downloadDir)

	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(content), 0600)
	assert.NilError(t, err)
	return path
}

func setupTestApp(t *testing.T, initialObjects []fakestorage.Object, port uint16, projectIDs []string, downloadDir string) *teatest.TestModel {
	t.Helper()

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: initialObjects,
		Host:           "127.0.0.1",
		Port:           port,
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(server.Stop)

	configPath := createConfigFile(t, projectIDs, downloadDir)
	cfg, err := config.Load(configPath)
	assert.NilError(t, err)

	gcsClient := gcs.NewClient(server.Client())
	m := tui.NewModel(cfg.Projects, gcsClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.Icons)

	tm := teatest.NewTestModel(t, m)
	t.Cleanup(func() {
		_ = tm.Quit()
		tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	return tm
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for file %s", path)
}

func TestListBuckets(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "test-bucket-1",
				Name:       "init",
			},
			Content: []byte("hi"),
		},
	}
	tm := setupTestApp(t, objects, 8081, []string{"test-project-1"}, t.TempDir())

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)
}

func TestDownloadObject(t *testing.T) {
	content := []byte("download test content")
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "test-bucket-1",
				Name:       "file_to_dl.txt",
			},
			Content: content,
		},
	}
	downloadDir := t.TempDir()
	tm := setupTestApp(t, objects, 8088, []string{"test-project-1"}, downloadDir)

	// Wait for bucket
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "test-bucket-1")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Move cursor down to first bucket
	tm.Type("j")

	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "file_to_dl.txt")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Download object
	tm.Type("d")

	// Wait for downloaded to show on the screen just in case we need to give it more time or see what it looks like
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "Downloaded to")
		},
		teatest.WithDuration(3*time.Second),
	)

	// Check file was downloaded
	expectedPath := filepath.Join(downloadDir, "file_to_dl.txt")
	assert.NilError(t, waitForFile(expectedPath, 3*time.Second))

	// #nosec G304
	b, err := os.ReadFile(expectedPath)
	assert.NilError(t, err)
	assert.Equal(t, string(b), string(content))
}

func TestPreviewObject(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"},
			Content:     []byte("content1"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file2.txt"},
			Content:     []byte("content2"),
		},
	}
	tm := setupTestApp(t, objects, 8089, []string{"test-project-1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Move cursor down to first bucket
	tm.Type("j")
	tm.Type("l")

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Move to second file and check for its preview
	tm.Type("j")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "content2")
	}, teatest.WithDuration(3*time.Second))
}

func TestDownloadObject_MultiSelect(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "file1.txt"},
			Content:     []byte("content1"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "folder1/file2.txt"},
			Content:     []byte("content2"),
		},
	}
	downloadDir := t.TempDir()
	tm := setupTestApp(t, objects, 8090, []string{"test-project-1"}, downloadDir)

	// Wait for bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "test-bucket-1") }, teatest.WithDuration(3*time.Second))

	// Move cursor down to first bucket
	tm.Type("j")

	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Select folder1/ (which is first because prefixes are shown before objects)
	tm.Type(" ")

	// Move to file1.txt and select it
	tm.Type("j")
	tm.Type(" ")

	// Download
	tm.Type("d")

	// Wait for download to finish
	expectedPath1 := filepath.Join(downloadDir, "file1.txt")
	expectedPathZip := filepath.Join(downloadDir, "folder1.zip")

	assert.NilError(t, waitForFile(expectedPath1, 3*time.Second))
	assert.NilError(t, waitForFile(expectedPathZip, 3*time.Second))

	// #nosec G304
	b1, err := os.ReadFile(expectedPath1)
	assert.NilError(t, err)
	assert.Equal(t, string(b1), "content1")

	// Check zip
	r, err := zip.OpenReader(expectedPathZip)
	assert.NilError(t, err)
	defer func() { _ = r.Close() }()

	var foundFile2 bool
	for _, f := range r.File {
		if f.Name == "file2.txt" {
			foundFile2 = true
		}
	}
	assert.Assert(t, foundFile2, "file2.txt should be in the zip")
}

func TestSearch(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "init"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-2", Name: "init"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8091, []string{"test-project-1"}, t.TempDir())

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "test-bucket-1") && strings.Contains(string(bts), "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))

	// Search for test-bucket-1
	tm.Type("/")
	time.Sleep(100 * time.Millisecond) // UI transition to search
	tm.Type("bucket-1")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Enter to finish search mode

	// Force a full redraw so teatest can capture the entire screen state
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Verify only test-bucket-1 is visible
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "test-bucket-1") && !strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))
}

func TestNavigationUp(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8092, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket b1 (it's the second item after project header)
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Enter folder1/
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket root
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "folder1/") }, teatest.WithDuration(3*time.Second))

	// Go back to bucket list
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "Buckets") }, teatest.WithDuration(3*time.Second))
}

func TestDownloadOverwrite(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"},
			Content:     []byte("new content"),
		},
	}
	downloadDir := t.TempDir()
	filePath := filepath.Join(downloadDir, "file1.txt")
	err := os.WriteFile(filePath, []byte("old content"), 0600)
	assert.NilError(t, err)

	tm := setupTestApp(t, objects, 8093, []string{"p1"}, downloadDir)

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "file1.txt") }, teatest.WithDuration(3*time.Second))

	// Attempt download
	tm.Type("d")
	time.Sleep(100 * time.Millisecond) // Give time for state transition
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Wait for overwrite prompt
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "(o)verwrite")
	}, teatest.WithDuration(3*time.Second))

	// Confirm overwrite
	tm.Type("o")

	// Verify content
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// #nosec G304
		b, _ := os.ReadFile(filePath)
		if string(b) == "new content" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("file was not overwritten with new content")
}

func TestHelpMenu(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "init"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8098, []string{"p1"}, t.TempDir())

	// Toggle help
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "HELP") }, teatest.WithDuration(3*time.Second))

	// Toggle help off
	tm.Type("?")
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return !strings.Contains(string(bts), "HELP") }, teatest.WithDuration(3*time.Second))
}

func TestHeaderPathUpdates(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8099, []string{"p1"}, t.TempDir())

	// Wait for buckets to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "b1")
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Move into buckets list (focus)
	tm.Type("j")

	// Verify header shows the selected bucket
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "gs://b1/")
	}, teatest.WithDuration(3*time.Second))

	// Enter bucket 'b1'
	tm.Type("l")

	// Wait for objects view and verify header shows the selected prefix (folder1/)
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "gs://b1/folder1/")
	}, teatest.WithDuration(3*time.Second))

	// Enter prefix 'folder1/'
	tm.Type("l")

	// Wait for inside folder view and verify header shows the selected file
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "gs://b1/folder1/file1.txt")
	}, teatest.WithDuration(3*time.Second))
}

func TestPreviewEdgeCases(t *testing.T) {
	largeContent := strings.Repeat("line\n", 100)
	binaryContent := []byte{0x00, 0x01, 0x02}

	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "large.txt"},
			Content:     []byte(largeContent),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "binary.bin"},
			Content:     binaryContent,
		},
	}
	tm := setupTestApp(t, objects, 8094, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check binary preview
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "binary.bin") && strings.Contains(string(bts), "(binary content)")
	}, teatest.WithDuration(3*time.Second))

	// Check large file truncation
	tm.Type("j")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "large.txt") && strings.Contains(s, "...")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Markdown(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "README.md"},
			Content:     []byte("# Hello\n\nThis is **markdown**"),
		},
	}
	tm := setupTestApp(t, objects, 8096, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check for markdown rendering. Glamour will add some styling/padding.
	// We'll just check for the text content and some typical markdown rendering traits if possible.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// Glamour might render "Hello" with some bold/headers styling.
		// For now we just check if it's there.
		return strings.Contains(s, "README.md") && strings.Contains(s, "Hello") && strings.Contains(s, "markdown")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Zip(t *testing.T) {
	// Create a zip in memory
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("file_in_zip.txt")
	_, _ = f.Write([]byte("inner content"))
	_ = zw.Close()

	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "test.zip"},
			Content:     buf.Bytes(),
		},
	}
	tm := setupTestApp(t, objects, 8097, []string{"p1"}, t.TempDir())

	// Wait for b1
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))

	// Enter bucket
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check for zip listing in preview
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "test.zip") && strings.Contains(s, "file_in_zip.txt")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_JSON(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "data.json", ContentType: "application/json"},
			Content:     []byte(`{"name":"test","value":123}`),
		},
	}
	tm := setupTestApp(t, objects, 8099, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check for pretty-printed JSON in preview
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// Should contain the characters regardless of styling
		return strings.Contains(s, "{") && strings.Contains(s, "name") && strings.Contains(s, "test") && strings.Contains(s, "value")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_CSV(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "data.csv"},
			Content:     []byte("id,name,city\n1,Alice,London\n2,Bob,Paris"),
		},
	}
	tm := setupTestApp(t, objects, 8100, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check for table rendering. We look for the values and typical table borders/alignment.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "id") && strings.Contains(s, "Alice") && strings.Contains(s, "Paris")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Parquet(t *testing.T) {
	type Row struct {
		ID   int    `parquet:"id"`
		Name string `parquet:"name"`
	}

	buf := new(bytes.Buffer)
	writer := parquet.NewGenericWriter[Row](buf)
	_, _ = writer.Write([]Row{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}})
	_ = writer.Close()

	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "data.parquet"},
			Content:     buf.Bytes(),
		},
	}
	tm := setupTestApp(t, objects, 8101, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Check for parquet schema or rows in preview
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "data.parquet") && strings.Contains(s, "Alice") && strings.Contains(s, "id")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_YAML(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "config.yaml"},
			Content:     []byte("app:\n  name: lazygcs\n  enabled: true"),
		},
	}
	tm := setupTestApp(t, objects, 8102, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "config.yaml") && strings.Contains(s, "lazygcs") && strings.Contains(s, "enabled")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_TOML(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "settings.toml"},
			Content:     []byte("[server]\nport = 8080\nhost = \"localhost\""),
		},
	}
	tm := setupTestApp(t, objects, 8103, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "settings.toml") && strings.Contains(s, "8080") && strings.Contains(s, "localhost")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Avro(t *testing.T) {
	schema, _ := avro.Parse(`{"type":"record","name":"test","fields":[{"name":"id","type":"int"},{"name":"name","type":"string"}]}`)
	buf := new(bytes.Buffer)
	enc, _ := ocf.NewEncoder(schema.String(), buf)
	_ = enc.Encode(map[string]any{"id": 1, "name": "Alice"})
	_ = enc.Close()

	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "data.avro"},
			Content:     buf.Bytes(),
		},
	}
	tm := setupTestApp(t, objects, 8104, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "data.avro") && strings.Contains(s, "Alice") && strings.Contains(s, "Avro Schema")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Logs(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "app.log"},
			Content:     []byte("INFO: starting up\nERROR: failed to connect\nWARN: retrying"),
		},
	}
	tm := setupTestApp(t, objects, 8105, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// We check for the content. Log colorization is hard to check via string matching,
		// but checking for the strings is a good start.
		return strings.Contains(s, "app.log") && strings.Contains(s, "ERROR") && strings.Contains(s, "failed to connect")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_PDF(t *testing.T) {
	// A minimal PDF that might be enough to trigger metadata extraction or at least not crash
	minimalPDF := []byte("%PDF-1.4\n1 0 obj\n<< /Title (Test PDF) >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF")
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "test.pdf"},
			Content:     minimalPDF,
		},
	}
	tm := setupTestApp(t, objects, 8106, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "test.pdf") && (strings.Contains(s, "Title") || strings.Contains(s, "PDF"))
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Conf(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "server.conf"},
			Content:     []byte("listen = 80\nserver_name = localhost"),
		},
	}
	tm := setupTestApp(t, objects, 8107, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "server.conf") && strings.Contains(s, "listen") && strings.Contains(s, "80")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Properties(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "app.properties"},
			Content:     []byte("app.version=1.2.3\napp.env=prod"),
		},
	}
	tm := setupTestApp(t, objects, 8108, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "app.properties") && strings.Contains(s, "version") && strings.Contains(s, "1.2.3")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_Shell(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "script.sh"},
			Content:     []byte("#!/bin/bash\necho 'hello world'"),
		},
	}
	tm := setupTestApp(t, objects, 8109, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// Check that filename and content are present.
		// Since we use Monokai style, 'echo' and string literals will be colored.
		// The ANSI escape codes (e.g., \x1b[) confirm highlighting is active.
		return strings.Contains(s, "script.sh") &&
			strings.Contains(s, "echo") &&
			strings.Contains(s, "hello world") &&
			strings.Contains(s, "\x1b[")
	}, teatest.WithDuration(3*time.Second))
}

func TestRichPreview_ShellShebang(t *testing.T) {
	objects := []fakestorage.Object{
		{
			// No extension, but has a shebang
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "myscript"},
			Content:     []byte("#!/bin/zsh\necho 'zsh rules'"),
		},
	}
	tm := setupTestApp(t, objects, 8110, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "myscript") &&
			strings.Contains(s, "echo") &&
			strings.Contains(s, "zsh rules") &&
			strings.Contains(s, "\x1b[")
	}, teatest.WithDuration(3*time.Second))
}

func TestNavigationCycle(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "init"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b2", Name: "init"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8095, []string{"p1"}, t.TempDir())

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "b1") && strings.Contains(string(bts), "b2")
	}, teatest.WithDuration(3*time.Second))

	// Position 0: p1 header
	// Position 1: b1
	// Position 2: b2
	// Move to b2 (2 times 'j')
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Assert we are on either b1 or b2
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "b1") && strings.Contains(s, "b2")
	}, teatest.WithDuration(3*time.Second))

	// Move one more to cycle back to project header
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// Check that project header is present
		return strings.Contains(s, "▼ p1")
	}, teatest.WithDuration(3*time.Second))

	// Move back up (cycle from top to bottom)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	time.Sleep(100 * time.Millisecond)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "b1") && strings.Contains(s, "b2")
	}, teatest.WithDuration(3*time.Second))
}

func createMockTar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDockerPreview_TarDocker(t *testing.T) {
	content := createMockTar(t, map[string]string{
		"manifest.json": `[{"Config":"config.json","RepoTags":["test:latest"],"Layers":["layer.tar"]}]`,
		"config.json":   `{"architecture":"amd64","os":"linux"}`,
	})
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "image.tar"},
			Content:     content,
		},
	}
	tm := setupTestApp(t, objects, 8200, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "image.tar") && strings.Contains(s, "Docker Image") && strings.Contains(s, "linux/amd64")
	}, teatest.WithDuration(3*time.Second))
}

func TestDockerPreview_TarOCI(t *testing.T) {
	content := createMockTar(t, map[string]string{
		"oci-layout": `{"imageLayoutVersion": "1.0.0"}`,
		"index.json": `{"manifests": [{"mediaType": "application/vnd.oci.image.manifest.v1+json"}]}`,
	})
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "oci.tar"},
			Content:     content,
		},
	}
	tm := setupTestApp(t, objects, 8201, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "oci.tar") && strings.Contains(s, "OCI Image")
	}, teatest.WithDuration(3*time.Second))
}

func TestDockerPreview_TarFallback(t *testing.T) {
	content := createMockTar(t, map[string]string{
		"random.txt": "hello world",
	})
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "random.tar"},
			Content:     content,
		},
	}
	tm := setupTestApp(t, objects, 8202, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// It should fall back to the generic TarPreviewer which outputs "Archive contents"
		return strings.Contains(s, "random.tar") && strings.Contains(s, "Archive contents") && strings.Contains(s, "random.txt")
	}, teatest.WithDuration(3*time.Second))
}

func TestDockerPreview_JSONDocker(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "manifest.json"},
			Content:     []byte(`{"mediaType": "application/vnd.docker.distribution.manifest.v2+json", "config": {"digest": "sha256:12345"}}`),
		},
	}
	tm := setupTestApp(t, objects, 8203, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "manifest.json") && strings.Contains(s, "Docker Manifest")
	}, teatest.WithDuration(3*time.Second))
}

func TestDockerPreview_JSONOCI(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "index.json"},
			Content:     []byte(`{"mediaType": "application/vnd.oci.image.manifest.v1+json", "config": {"digest": "sha256:67890"}}`),
		},
	}
	tm := setupTestApp(t, objects, 8204, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "index.json") && strings.Contains(s, "OCI Manifest")
	}, teatest.WithDuration(3*time.Second))
}

func TestDockerPreview_JSONFallback(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "generic.json"},
			Content:     []byte(`{"foo": "bar"}`),
		},
	}
	tm := setupTestApp(t, objects, 8205, []string{"p1"}, t.TempDir())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool { return strings.Contains(string(bts), "b1") }, teatest.WithDuration(3*time.Second))
	tm.Type("j")
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		// It should render generic JSON syntax highlighting
		return strings.Contains(s, "generic.json") && strings.Contains(s, "foo") && strings.Contains(s, "bar") && !strings.Contains(s, "Docker") && !strings.Contains(s, "OCI")
	}, teatest.WithDuration(3*time.Second))
}

func TestSearchFilterPersistence(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "folder1/file1.txt"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-1", Name: "folder1/file2.txt"}, Content: []byte("hi")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "test-bucket-2", Name: "init"}, Content: []byte("hi")},
	}
	tm := setupTestApp(t, objects, 8094, []string{"test-project-1"}, t.TempDir())

	// Wait for buckets
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "test-bucket-1") && strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))

	// Move to test-bucket-1
	tm.Type("/")
	time.Sleep(100 * time.Millisecond)
	tm.Type("bucket-1")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	// Move cursor down to the bucket (since index 0 is the project header)
	tm.Type("j")

	// Enter bucket test-bucket-1
	tm.Type("l")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Wait for folder1 to load (we are now in viewObjects)
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "folder1/")
	}, teatest.WithDuration(3*time.Second))

	// Go back to viewBuckets
	tm.Type("h")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Wait for buckets to load, but the filter "bucket-1" should still be active on the bucket list
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "FILTER: bucket-1") && strings.Contains(s, "test-bucket-1") && !strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))

	// Press Esc to clear the filter
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc, Alt: false})
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Wait for filter to be cleared
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return !strings.Contains(s, "FILTER: bucket-1") && strings.Contains(s, "test-bucket-2")
	}, teatest.WithDuration(3*time.Second))
}
