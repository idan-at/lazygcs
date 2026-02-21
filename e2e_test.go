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
	cmd := exec.Command(bin)
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
