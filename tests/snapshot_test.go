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

	tm := setupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

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
	io.Copy(&buf, tm.FinalOutput(t))
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

	tm := setupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

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
	io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_HelpMenu(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := setupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

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
	io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}

func TestSnapshot_ErrorsModal(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := setupTestApp(t, objects, 0, []string{"prod-project"}, t.TempDir())

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
	io.Copy(&buf, tm.FinalOutput(t))
	teatest.RequireEqualOutput(t, buf.Bytes())
}
