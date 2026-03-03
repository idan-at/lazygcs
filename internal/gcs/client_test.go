package gcs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"google.golang.org/api/iterator"
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
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt", Size: 100}},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/"}}, // Directory stub
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
		assert.Assert(t, containsObject(list.Objects, "file1.txt"))
		assert.Assert(t, list.Objects[0].Size == 100)

		// Should have folder1/ and folder2/ as prefixes
		assert.Assert(t, containsPrefix(list.Prefixes, "folder1/"))
		assert.Assert(t, containsPrefix(list.Prefixes, "folder2/"))

		// Should NOT have objects from subfolders
		assert.Assert(t, !containsObject(list.Objects, "file2.txt"))
	})

	t.Run("Inside folder1", func(t *testing.T) {
		list, err := client.ListObjects(context.Background(), "b1", "folder1/")
		assert.NilError(t, err)

		// Should have folder1/file2.txt
		assert.Assert(t, containsObject(list.Objects, "folder1/file2.txt"))
		assert.Assert(t, containsPrefix(list.Prefixes, "folder1/subfolder/"))

		// Should NOT contain the current prefix "folder1/" itself as a prefix or an object
		assert.Assert(t, !containsPrefix(list.Prefixes, "folder1/"))
		assert.Assert(t, !containsObject(list.Objects, "folder1/"))
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

func containsPrefix(slice []gcs.PrefixMetadata, val string) bool {
	for _, s := range slice {
		if s.Name == val {
			return true
		}
	}
	return false
}

func containsObject(slice []gcs.ObjectMetadata, name string) bool {
	for _, o := range slice {
		if o.Name == name {
			return true
		}
	}
	return false
}

func TestClient_DownloadObject(t *testing.T) {
	content := []byte("hello gcs")
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"},
				Content:     content,
			},
		},
		Host:   "127.0.0.1",
		Port:   8086,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	client := gcs.NewClient(server.Client())
	dest := filepath.Join(t.TempDir(), "downloaded.txt")

	err = client.DownloadObject(context.Background(), "b1", "file1.txt", dest)
	assert.NilError(t, err)

	got, err := os.ReadFile(dest)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, content)
}

func TestClient_GetObjectContent(t *testing.T) {
	longContent := make([]byte, 2048)
	for i := range longContent {
		longContent[i] = 'a'
	}
	shortContent := []byte("hello")

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "long.txt"},
				Content:     longContent,
			},
			{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "short.txt"},
				Content:     shortContent,
			},
		},
		Host:   "127.0.0.1",
		Port:   8087,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	client := gcs.NewClient(server.Client())

	// Test case 1: Content is longer than 1KB, should be truncated
	content, err := client.GetObjectContent(context.Background(), "b1", "long.txt")
	assert.NilError(t, err)
	assert.Equal(t, len(content), 1024, "Content should be truncated to 1024 bytes")
	assert.Equal(t, content, string(longContent[:1024]))

	// Test case 2: Content is shorter than 1KB, should be returned as is
	content, err = client.GetObjectContent(context.Background(), "b1", "short.txt")
	assert.NilError(t, err)
	assert.Equal(t, len(content), len(shortContent))
	assert.Equal(t, content, string(shortContent))
}

func TestFakestorage_Behavior(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/"}},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}},
		},
		Host:   "127.0.0.1",
		Port:   8085,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	ctx := context.Background()
	client := server.Client()

	t.Run("With delimiter", func(t *testing.T) {
		it := client.Bucket("b1").Objects(ctx, &storage.Query{Delimiter: "/"})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			assert.NilError(t, err)
			if attrs.Prefix != "" {
				t.Logf("Prefix: %q", attrs.Prefix)
			} else {
				t.Logf("Object: %q", attrs.Name)
			}
		}
	})

	t.Run("Without delimiter", func(t *testing.T) {
		it := client.Bucket("b1").Objects(ctx, &storage.Query{})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			assert.NilError(t, err)
			t.Logf("Object: %q", attrs.Name)
		}
	})
}
