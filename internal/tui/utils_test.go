package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
