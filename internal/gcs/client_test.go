package gcs_test

import (
	"context"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"
	"lazygcs/internal/gcs"
)

func TestClient_ListBuckets(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "o1"}},
		},
		Host:   "127.0.0.1",
		Port:   8082,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	client := gcs.NewClient(server.Client())
	buckets, err := client.ListBuckets(context.Background(), []string{"test-project"})
	assert.NilError(t, err)
	assert.Assert(t, contains(buckets, "b1"))
}

func TestClient_ListObjects(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"}},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file2.txt"}},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/subfolder/file3.txt"}},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder2/file4.txt"}},
		},
		Host:   "127.0.0.1",
		Port:   8083,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	client := gcs.NewClient(server.Client())

	t.Run("Root level", func(t *testing.T) {
		list, err := client.ListObjects(context.Background(), "b1", "")
		assert.NilError(t, err)

		// Should have file1.txt as object
		assert.Assert(t, contains(list.Objects, "file1.txt"))
		// Should have folder1/ and folder2/ as prefixes
		assert.Assert(t, contains(list.Prefixes, "folder1/"))
		assert.Assert(t, contains(list.Prefixes, "folder2/"))
		// Should NOT have objects from subfolders
		assert.Assert(t, !contains(list.Objects, "file2.txt"))
	})

	t.Run("Inside folder1", func(t *testing.T) {
		list, err := client.ListObjects(context.Background(), "b1", "folder1/")
		assert.NilError(t, err)

		// Should have folder1/file2.txt
		assert.Assert(t, contains(list.Objects, "folder1/file2.txt"))
		assert.Assert(t, contains(list.Prefixes, "folder1/subfolder/"))

		// Should NOT contain the current prefix "folder1/" itself
		assert.Assert(t, !contains(list.Prefixes, "folder1/"))
	})
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
