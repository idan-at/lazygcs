package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/idan-at/lazygcs/internal/testutil"
	"github.com/idan-at/lazygcs/internal/tui"
)

func TestSnapshot_InitialBucketsView(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "logs", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load and appear on screen
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "assets")
	}, teatest.WithDuration(3*time.Second))

	// Move cursor down to 'assets'
	tm.Type("j")

	// Force a specific dimension for consistent snapshots
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Wait for the terminal to resize and render
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "assets")
	}, teatest.WithDuration(2*time.Second))

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Read any remaining output
	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_ObjectsAndPreview(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "images/logo.png"},
			Content:     []byte("fake-png-content"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName:  "assets",
				Name:        "README.md",
				ContentType: "text/markdown",
				Created:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Updated:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			Content: []byte("# Hello World\nThis is a test file."),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..."
	}, teatest.WithDuration(3*time.Second))

	// Force terminal size
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Enter bucket 'assets'
	tm.Type("j")
	tm.Type("l")

	// Wait for README.md to be visible
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "README.md")
	}, teatest.WithDuration(3*time.Second))

	// Move down to README.md
	tm.Type("j")

	// Wait for the preview content to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "Hello World")
	}, teatest.WithDuration(10*time.Second))

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Read any remaining output
	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_HelpMenu(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..."
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Open help menu
	tm.Type("?")

	// Wait for help menu to appear
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		out := string(bts)
		return strings.Contains(out, "HELP") || strings.Contains(out, "quit")
	}, teatest.WithDuration(2*time.Second))

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Read any remaining output
	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_SearchView(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..."
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Open search
	tm.Type("/")
	// Type query
	tm.Type("ass")

	// Wait for search bar to be visible and have text
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "SEARCH") && strings.Contains(string(bts), "ass")
	}, teatest.WithDuration(2*time.Second))

	// Trigger quit
	_ = tm.Quit()

	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_MultiSelectionView(t *testing.T) {
	fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "assets",
				Name:       "file1.txt",
				Created:    fixedTime,
				Updated:    fixedTime,
			},
			Content: []byte("content1"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "assets",
				Name:       "file2.txt",
				Created:    fixedTime,
				Updated:    fixedTime,
			},
			Content: []byte("content2"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "assets",
				Name:       "file3.txt",
				Created:    fixedTime,
				Updated:    fixedTime,
			},
			Content: []byte("content3"),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..."
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Wait for buckets to load and appear
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "assets")
	}, teatest.WithDuration(3*time.Second))

	// Move down to bucket 'assets'
	tm.Type("j")
	// Enter bucket
	tm.Type("l")

	// Wait for objects to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "file1.txt")
	}, teatest.WithDuration(3*time.Second))

	// Select file1.txt
	tm.Type(" ")
	// Move to file2.txt
	tm.Type("j")
	// Select file2.txt
	tm.Type(" ")

	// Wait for the second selection to be processed and rendered.
	// We wait for "content2" (the preview of file2.txt) to appear to ensure a stable, deterministic state before quitting.
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		return strings.Contains(string(bts), "content2")
	}, teatest.WithDuration(3*time.Second))

	// Trigger quit
	_ = tm.Quit()

	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_ErrorsModal(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := testutil.SetupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

	var buf bytes.Buffer
	tee := io.TeeReader(tm.Output(), &buf)

	// Wait for buckets to load
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..."
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Inject errors
	tm.Send(tui.BucketsPageMsg{Err: errors.New("simulated permission denied error")})
	tm.Send(tui.BucketsPageMsg{Err: errors.New("simulated network timeout")})

	// Open errors modal
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlE})

	// Wait for errors modal to appear
	teatest.WaitFor(t, tee, func(bts []byte) bool {
		out := string(bts)
		return strings.Contains(out, "ERRORS") || strings.Contains(out, "simulated permission denied")
	}, teatest.WithDuration(2*time.Second))

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Read any remaining output
	_, _ = io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}
