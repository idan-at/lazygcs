package gcs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"google.golang.org/api/iterator"
	"gotest.tools/v3/assert"
	"github.com/idan-at/lazygcs/internal/gcs"
)

func setupTestServer(t *testing.T, objects []fakestorage.Object) (*fakestorage.Server, *gcs.Client) {
	t.Helper()
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: objects,
		Host:           "127.0.0.1",
		Port:           0,
		Scheme:         "http",
	})
	assert.NilError(t, err)
	t.Cleanup(func() { server.Stop() })
	return server, gcs.NewClient(server.Client())
}

func TestClient_DownloadPrefixAsZip(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}, Content: []byte("content1")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/sub/file2.txt"}, Content: []byte("content2")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "other.txt"}, Content: []byte("other")},
	}
	_, client := setupTestServer(t, objects)

	dest := filepath.Join(t.TempDir(), "folder1.zip")
	err := client.DownloadPrefixAsZip(context.Background(), "b1", "folder1/", dest)
	assert.NilError(t, err)

	// Verify zip contents
	r, err := zip.OpenReader(dest)
	assert.NilError(t, err)
	defer func() { _ = r.Close() }()

	expectedFiles := map[string]string{
		"file1.txt":     "content1",
		"sub/file2.txt": "content2",
	}

	assert.Equal(t, len(r.File), len(expectedFiles))

	for _, f := range r.File {
		expectedContent, ok := expectedFiles[f.Name]
		assert.Assert(t, ok, "Unexpected file in zip: %s", f.Name)

		rc, err := f.Open()
		assert.NilError(t, err)
		content, err := io.ReadAll(rc)
		assert.NilError(t, err)
		_ = rc.Close()

		assert.Equal(t, string(content), expectedContent)
	}
}

func TestClient_ListBucketsPage(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "o1"}},
	}
	_, client := setupTestServer(t, objects)

	buckets, nextToken, err := client.ListBucketsPage(context.Background(), "test-project", "", 500)
	assert.NilError(t, err)
	assert.Equal(t, len(buckets), 1)

	assert.Assert(t, buckets[0] == "b1", "b1 not found in buckets")
	assert.Equal(t, nextToken, "")
}

func TestClient_ListObjects(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt", Size: 100}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/"}}, // Directory stub
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file2.txt"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/subfolder/file3.txt"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder2/file4.txt"}},
	}
	_, client := setupTestServer(t, objects)

	t.Run("Root level", func(t *testing.T) {
		t.Parallel()
		list, err := client.ListObjects(context.Background(), "b1", "")
		assert.NilError(t, err)

		// Should have file1.txt as object
		assert.Assert(t, slices.ContainsFunc(list.Objects, func(o gcs.ObjectMetadata) bool { return o.Name == "file1.txt" }))
		assert.Assert(t, list.Objects[0].Size == 100)

		// Should have folder1/ and folder2/ as prefixes
		assert.Assert(t, slices.ContainsFunc(list.Prefixes, func(p gcs.PrefixMetadata) bool { return p.Name == "folder1/" }))
		assert.Assert(t, slices.ContainsFunc(list.Prefixes, func(p gcs.PrefixMetadata) bool { return p.Name == "folder2/" }))

		// Should NOT have objects from subfolders
		assert.Assert(t, !slices.ContainsFunc(list.Objects, func(o gcs.ObjectMetadata) bool { return o.Name == "file2.txt" }))
	})

	t.Run("Inside folder1", func(t *testing.T) {
		t.Parallel()
		list, err := client.ListObjects(context.Background(), "b1", "folder1/")
		assert.NilError(t, err)

		// Should have folder1/file2.txt
		assert.Assert(t, slices.ContainsFunc(list.Objects, func(o gcs.ObjectMetadata) bool { return o.Name == "folder1/file2.txt" }))
		assert.Assert(t, slices.ContainsFunc(list.Prefixes, func(p gcs.PrefixMetadata) bool { return p.Name == "folder1/subfolder/" }))

		// Should NOT contain the current prefix "folder1/" itself as a prefix or an object
		assert.Assert(t, !slices.ContainsFunc(list.Prefixes, func(p gcs.PrefixMetadata) bool { return p.Name == "folder1/" }))
		assert.Assert(t, !slices.ContainsFunc(list.Objects, func(o gcs.ObjectMetadata) bool { return o.Name == "folder1/" }))
	})
}

func TestClient_DownloadObject(t *testing.T) {
	content := []byte("hello gcs")
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file1.txt"}, Content: content},
	}
	_, client := setupTestServer(t, objects)

	dest := filepath.Join(t.TempDir(), "downloaded.txt")

	err := client.DownloadObject(context.Background(), "b1", "file1.txt", dest)
	assert.NilError(t, err)

	got, err := os.ReadFile(dest)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, content)
}

func TestClient_GetObjectContent(t *testing.T) {
	longContent := bytes.Repeat([]byte("a"), 2048)
	shortContent := []byte("hello")

	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "long.txt"}, Content: longContent},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "short.txt"}, Content: shortContent},
	}
	_, client := setupTestServer(t, objects)

	tests := []struct {
		name        string
		objectName  string
		expectedLen int
		expectedStr string
	}{
		{
			name:        "Content > 1KB (truncated)",
			objectName:  "long.txt",
			expectedLen: 1024,
			expectedStr: string(longContent[:1024]),
		},
		{
			name:        "Content < 1KB (full)",
			objectName:  "short.txt",
			expectedLen: len(shortContent),
			expectedStr: string(shortContent),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content, err := client.GetObjectContent(context.Background(), "b1", tt.objectName)
			assert.NilError(t, err)
			assert.Equal(t, len(content), tt.expectedLen)
			assert.Equal(t, content, tt.expectedStr)
		})
	}
}

func TestClient_NewReaderAt(t *testing.T) {
	content := []byte("0123456789abcdef")
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file.txt"}, Content: content},
	}
	_, client := setupTestServer(t, objects)

	readerAt := client.NewReaderAt(context.Background(), "b1", "file.txt")

	t.Run("Read from middle", func(t *testing.T) {
		p := make([]byte, 4)
		n, err := readerAt.ReadAt(p, 4)
		assert.NilError(t, err)
		assert.Equal(t, n, 4)
		assert.Equal(t, string(p), "4567")
	})

	t.Run("Read till EOF", func(t *testing.T) {
		p := make([]byte, 8)
		n, err := readerAt.ReadAt(p, 12)
		assert.Equal(t, err, io.EOF)
		assert.Equal(t, n, 4)
		assert.Equal(t, string(p[:n]), "cdef")
	})
}

func TestFakestorage_Behavior(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt"}},
	}
	server, _ := setupTestServer(t, objects)

	ctx := context.Background()
	sc := server.Client()

	t.Run("With delimiter", func(t *testing.T) {
		t.Parallel()
		it := sc.Bucket("b1").Objects(ctx, &storage.Query{Delimiter: "/"})
		foundPrefix := false
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			assert.NilError(t, err)
			if attrs.Prefix != "" {
				foundPrefix = true
			}
		}
		assert.Assert(t, foundPrefix)
	})

	t.Run("Without delimiter", func(t *testing.T) {
		t.Parallel()
		it := sc.Bucket("b1").Objects(ctx, &storage.Query{})
		foundObject := false
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			assert.NilError(t, err)
			if attrs.Name != "" {
				foundObject = true
			}
		}
		assert.Assert(t, foundObject)
	})
}
