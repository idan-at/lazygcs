package main

import (
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fsouza/fake-gcs-server/fakestorage"
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

	tm := setupTestApp(t, objects, 8096, []string{"prod-project"}, t.TempDir())

	// Wait for buckets to load and appear on screen
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return s != "" && s != "Loading..." // Wait until initial render passes
	}, teatest.WithDuration(3*time.Second))

	// Move cursor down to 'assets'
	tm.Type("j")
	
	// Force a specific dimension for consistent snapshots
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	time.Sleep(200 * time.Millisecond) // Give Bubble Tea time to render

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Take snapshot
	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

func TestSnapshot_ObjectsAndPreview(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "images/logo.png"},
			Content:     []byte("fake-png-content"),
		},
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "README.md", ContentType: "text/markdown"},
			Content:     []byte("# Hello World\nThis is a test file."),
		},
	}

	tm := setupTestApp(t, objects, 8097, []string{"prod-project"}, t.TempDir())

	// Force terminal size
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Enter bucket 'assets'
	tm.Type("j")
	tm.Type("l")

	// Wait for README.md to be visible
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return string(bts) != ""
	}, teatest.WithDuration(3*time.Second))
	
	time.Sleep(500 * time.Millisecond) // Ensure the list is fully loaded
	
	// Move down to README.md
	tm.Type("j")

	// Wait for the preview content to load
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return string(bts) != "" // This should match the content
	}, teatest.WithDuration(3*time.Second))

	time.Sleep(200 * time.Millisecond) // Final settling before snapshot

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Take snapshot
	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

func TestSnapshot_HelpMenu(t *testing.T) {
	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "assets", Name: "init"},
			Content:     []byte("hi"),
		},
	}

	tm := setupTestApp(t, objects, 8099, []string{"prod-project"}, t.TempDir())

	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	
	// Open help menu
	tm.Type("?")
	time.Sleep(200 * time.Millisecond) // Give Bubble Tea time to render the overlay

	// Trigger quit so FinalOutput can return
	_ = tm.Quit()

	// Take snapshot
	out, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}
