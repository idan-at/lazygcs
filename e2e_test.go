package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/poll"
)

func TestListBuckets(t *testing.T) {
	// 1. Build Binary
	bin := filepath.Join(t.TempDir(), "lazygcs")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	assert.NilError(t, err, "Build failed: %s", out)

	// 2. Setup Fake GCS
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "test-bucket-1",
					Name:       "init",
				},
				Content: []byte("hi"),
			},
		},
		Host:   "127.0.0.1",
		Port:   8081,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	// 3. Run App
	cmd := exec.Command(bin, "test-project-1")
	// STORAGE_EMULATOR_HOST must be host:port without scheme
	emulatorHost := strings.TrimPrefix(server.URL(), "http://")
	cmd.Env = append(os.Environ(), fmt.Sprintf("STORAGE_EMULATOR_HOST=%s", emulatorHost))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	assert.NilError(t, cmd.Start())
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// 4. Assert with Awaitability
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		if strings.Contains(stdout.String(), "test-bucket-1") {
			return poll.Success()
		}
		return poll.Continue("waiting for bucket name in output")
	}, poll.WithTimeout(3*time.Second))
}

func TestDownloadObject_E2E(t *testing.T) {
	// 1. Build Binary
	bin := filepath.Join(t.TempDir(), "lazygcs")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	assert.NilError(t, err, "Build failed: %s", out)

	// 2. Setup Fake GCS
	content := []byte("download test content")
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "test-bucket-1",
					Name:       "file_to_dl.txt",
				},
				Content: content,
			},
		},
		Host:   "127.0.0.1",
		Port:   8088,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	// Create a temp download dir and a config file
	downloadDir := t.TempDir()
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	configContent := fmt.Sprintf("download_dir = %q\nprojects = [\"test-project\"]", downloadDir)
	assert.NilError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	// Temporarily override HOME so lazygcs reads our config
	// lazygcs looks at ~/.config/lazygcs/config.toml
	homeDir := t.TempDir()
	appConfigDir := filepath.Join(homeDir, ".config", "lazygcs")
	assert.NilError(t, os.MkdirAll(appConfigDir, 0755))
	assert.NilError(t, os.WriteFile(filepath.Join(appConfigDir, "config.toml"), []byte(configContent), 0644))


	// 3. Run App
	cmd := exec.Command(bin)
	emulatorHost := strings.TrimPrefix(server.URL(), "http://")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("STORAGE_EMULATOR_HOST=%s", emulatorHost),
		fmt.Sprintf("HOME=%s", homeDir),
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	stdin, err := cmd.StdinPipe()
	assert.NilError(t, err)

	assert.NilError(t, cmd.Start())
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Wait for bucket list
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		if strings.Contains(stdout.String(), "test-bucket-1") {
			return poll.Success()
		}
		return poll.Continue("waiting for bucket")
	}, poll.WithTimeout(3*time.Second))

	// Press 'l' to enter the bucket
	_, err = stdin.Write([]byte("l\n"))
	assert.NilError(t, err)

	// Wait for object list
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		if strings.Contains(stdout.String(), "file_to_dl.txt") {
			return poll.Success()
		}
		return poll.Continue("waiting for object\nStdout so far:\n%s", stdout.String())
	}, poll.WithTimeout(3*time.Second))

	// Press 'd' to download
	_, err = stdin.Write([]byte("d\n"))
	assert.NilError(t, err)

	// Verify the file was downloaded
	expectedPath := filepath.Join(downloadDir, "file_to_dl.txt")
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		_, err := os.Stat(expectedPath)
		if err == nil {
			return poll.Success()
		}
		return poll.Continue("waiting for file to be downloaded")
	}, poll.WithTimeout(3*time.Second))

	b, err := os.ReadFile(expectedPath)
	assert.NilError(t, err)
	assert.Equal(t, string(b), string(content))
}

