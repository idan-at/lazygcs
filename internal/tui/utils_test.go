package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestAutoRename_PermissionError(t *testing.T) {
	tempDir := t.TempDir()
	restrictedDir := filepath.Join(tempDir, "restricted")

	err := os.Mkdir(restrictedDir, 0755)
	assert.NilError(t, err)

	filePath := filepath.Join(restrictedDir, "test.txt")

	err = os.Chmod(restrictedDir, 0000)
	assert.NilError(t, err)

	defer os.Chmod(restrictedDir, 0755)

	done := make(chan error)

	go func() {
		_, err := autoRename(filePath)
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
