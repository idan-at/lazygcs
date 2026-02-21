package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"
)

func TestRun_ListsBuckets(t *testing.T) {
	// 1. Setup Fake GCS
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "bucket-alpha",
					Name:       "obj1",
				},
				Content: []byte("content"),
			},
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "bucket-beta",
					Name:       "obj2",
				},
				Content: []byte("content"),
			},
		},
		Host: "127.0.0.1",
		Port: 8081,
	})
	assert.NilError(t, err)
	defer server.Stop()

	// 2. Create Client connected to Fake Server
	client := server.Client()
	defer client.Close()

	// 3. Capture Output
	var stdout bytes.Buffer

	// 4. Run Application Logic
	err = Run(context.Background(), []string{"test-project-1"}, client, &stdout)
	assert.NilError(t, err)

	// 5. Assertions
	output := stdout.String()
	assert.Assert(t, strings.Contains(output, "bucket-alpha"))
	assert.Assert(t, strings.Contains(output, "bucket-beta"))
}
