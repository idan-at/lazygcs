package gcs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/idan-at/lazygcs/internal/gcs"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"gotest.tools/v3/assert"
)

func TestClient_GetProjectMetadata(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.URL.Path, "/v1/projects/my-test-project")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"projectId": "my-test-project",
				"projectNumber": "1234567890",
				"name": "My Test Project",
				"createTime": "2023-10-01T12:00:00Z",
				"labels": {"env": "test"},
				"parent": {"type": "folder", "id": "98765"}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		sc, err := storage.NewClient(ctx, option.WithoutAuthentication())
		assert.NilError(t, err)
		defer func() { _ = sc.Close() }()

		c := gcs.NewClient(sc, option.WithEndpoint(server.URL), option.WithHTTPClient(server.Client()), option.WithoutAuthentication())

		meta, err := c.GetProjectMetadata(ctx, "my-test-project")
		assert.NilError(t, err)
		assert.Equal(t, meta.ProjectID, "my-test-project")
		assert.Equal(t, meta.Name, "My Test Project")
		assert.Equal(t, meta.ProjectNumber, int64(1234567890))
		assert.Equal(t, meta.ParentType, "folder")
		assert.Equal(t, meta.ParentID, "98765")
		assert.Equal(t, meta.Labels["env"], "test")
		assert.Assert(t, !meta.CreateTime.IsZero())
	})

	t.Run("Error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()

		ctx := context.Background()
		sc, err := storage.NewClient(ctx, option.WithoutAuthentication())
		assert.NilError(t, err)
		defer func() { _ = sc.Close() }()

		c := gcs.NewClient(sc, option.WithEndpoint(server.URL), option.WithHTTPClient(server.Client()), option.WithoutAuthentication())

		_, err = c.GetProjectMetadata(ctx, "my-test-project")
		assert.ErrorContains(t, err, "failed to fetch project metadata")
	})
}

func TestClient_PermissionDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Permission denied."}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	// Create a storage client that points to our forbidden server
	sc, err := storage.NewClient(ctx,
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL),
		option.WithoutAuthentication(),
	)
	assert.NilError(t, err)

	client := gcs.NewClient(sc)

	t.Run("ListBuckets Forbidden", func(t *testing.T) {
		_, _, err := client.ListBucketsPage(ctx, "project", "", 10)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "403"))
	})

	t.Run("GetObjectContent Forbidden", func(t *testing.T) {
		_, err := client.GetObjectContent(ctx, "b", "o")
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "403"))
	})
}

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
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/sub/sub2/file3.txt"}, Content: []byte("content3")},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "other.txt"}, Content: []byte("other")},
	}
	_, client := setupTestServer(t, objects)

	t.Run("Specific prefix", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "folder1.zip")
		err := client.DownloadPrefixAsZip(context.Background(), "b1", "folder1/", dest, nil)
		assert.NilError(t, err)

		r, err := zip.OpenReader(dest)
		assert.NilError(t, err)
		defer func() { _ = r.Close() }()

		expectedFiles := map[string]string{
			"file1.txt":          "content1",
			"sub/file2.txt":      "content2",
			"sub/sub2/file3.txt": "content3",
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
	})

	t.Run("Empty prefix (entire bucket)", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "all.zip")
		err := client.DownloadPrefixAsZip(context.Background(), "b1", "", dest, nil)
		assert.NilError(t, err)

		r, err := zip.OpenReader(dest)
		assert.NilError(t, err)
		defer func() { _ = r.Close() }()

		expectedFiles := map[string]string{
			"folder1/file1.txt":          "content1",
			"folder1/sub/file2.txt":      "content2",
			"folder1/sub/sub2/file3.txt": "content3",
			"other.txt":                  "other",
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
	})
}

func TestClient_GetBucketMetadata(t *testing.T) {
	bucketName := "meta-bucket"

	server, client := setupTestServer(t, []fakestorage.Object{})

	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{
		Name:              bucketName,
		VersioningEnabled: true,
	})

	t.Run("Existing bucket", func(t *testing.T) {
		metadata, err := client.GetBucketMetadata(context.Background(), bucketName)
		assert.NilError(t, err)
		assert.Equal(t, metadata.Name, bucketName)
		assert.Equal(t, metadata.VersioningEnabled, true)
	})

	t.Run("Non-existent bucket", func(t *testing.T) {
		_, err := client.GetBucketMetadata(context.Background(), "non-existent-bucket")
		assert.ErrorIs(t, err, storage.ErrBucketNotExist)
	})
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

	err := client.DownloadObject(context.Background(), "b1", "file1.txt", dest, nil)
	assert.NilError(t, err)

	// #nosec G304
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
			if errors.Is(err, iterator.Done) {
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
			if errors.Is(err, iterator.Done) {
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

func TestClient_UploadObject(t *testing.T) {
	bucketName := "upload-test-bucket"
	objectName := "new-file.txt"
	content := []byte("test upload data")

	server, client := setupTestServer(t, []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "dummy"}}, // to create the bucket
	})

	srcPath := filepath.Join(t.TempDir(), "upload_source.txt")
	err := os.WriteFile(srcPath, content, 0600)
	assert.NilError(t, err)

	err = client.UploadObject(context.Background(), bucketName, objectName, srcPath)
	assert.NilError(t, err)

	obj, err := server.GetObject(bucketName, objectName)
	assert.NilError(t, err)
	assert.Equal(t, string(obj.Content), string(content))
}

func TestClient_DownloadObject_Errors(t *testing.T) {
	bucketName := "err-bucket"
	_, client := setupTestServer(t, []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "exists.txt"}, Content: []byte("hi")},
	})

	t.Run("Object Not Found (404)", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "notfound.txt")
		err := client.DownloadObject(context.Background(), bucketName, "non-existent.txt", dest, nil)
		assert.ErrorIs(t, err, storage.ErrObjectNotExist)
	})

	t.Run("Bucket Not Found", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "nobucket.txt")
		err := client.DownloadObject(context.Background(), "no-such-bucket", "file.txt", dest, nil)
		assert.Assert(t, err != nil)
		// fakestorage might return ErrObjectNotExist even if bucket is missing
		assert.Assert(t, strings.Contains(err.Error(), "storage:"))
	})
}

func TestClient_UploadObject_Errors(t *testing.T) {
	bucketName := "upload-err-bucket"
	_, client := setupTestServer(t, []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "dummy"}},
	})

	t.Run("Source File Not Found", func(t *testing.T) {
		err := client.UploadObject(context.Background(), bucketName, "remote.txt", "/non/existent/path/to/file")
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "failed to open source file"))
	})
}

func TestClient_GetObjectMetadata(t *testing.T) {
	bucketName := "test-bucket"
	objectName := "test-obj.txt"
	updatedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	createdTime := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

	objects := []fakestorage.Object{
		{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName:      bucketName,
				Name:            objectName,
				ContentType:     "application/json",
				ContentEncoding: "gzip",
				CacheControl:    "public, max-age=3600",
				StorageClass:    "STANDARD",
				Generation:      12345,
				Etag:            "etag-123",
				Crc32c:          "9876",
				Size:            123,
				Updated:         updatedTime,
				Created:         createdTime,
				Metadata:        map[string]string{"custom": "value"},
			},
		},
	}
	_, client := setupTestServer(t, objects)

	t.Run("Existing object", func(t *testing.T) {
		metadata, err := client.GetObjectMetadata(context.Background(), bucketName, objectName)
		assert.NilError(t, err)
		assert.Equal(t, metadata.Name, objectName)
		assert.Equal(t, metadata.Bucket, bucketName)
		assert.Equal(t, metadata.ContentType, "application/json")
		assert.Equal(t, metadata.ContentEncoding, "gzip")
		assert.Equal(t, metadata.CacheControl, "public, max-age=3600")
		assert.Equal(t, metadata.StorageClass, "STANDARD")
		assert.Equal(t, metadata.Generation, int64(12345))
		assert.Equal(t, metadata.ETag, "etag-123")
		assert.Equal(t, metadata.Size, int64(123))
		assert.Equal(t, metadata.Updated.Unix(), updatedTime.Unix())
		assert.Equal(t, metadata.Created.Unix(), createdTime.Unix())
		assert.DeepEqual(t, metadata.Metadata, map[string]string{"custom": "value"})
	})

	t.Run("Non-existent object", func(t *testing.T) {
		_, err := client.GetObjectMetadata(context.Background(), bucketName, "non-existent.txt")
		assert.ErrorIs(t, err, storage.ErrObjectNotExist)
	})
}

func TestClient_ListObjectsPage(t *testing.T) {
	bucketName := "page-bucket"
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "obj1"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "obj2"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "obj3"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "obj4"}},
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: bucketName, Name: "obj5"}},
	}
	_, client := setupTestServer(t, objects)

	// Page 1
	list1, token1, err := client.ListObjectsPage(context.Background(), bucketName, "", "", 2)
	assert.NilError(t, err)
	assert.Equal(t, len(list1.Objects), 2)
	assert.Assert(t, token1 != "")

	// Page 2
	list2, token2, err := client.ListObjectsPage(context.Background(), bucketName, "", token1, 2)
	assert.NilError(t, err)
	assert.Equal(t, len(list2.Objects), 2)
	assert.Assert(t, token2 != "")

	// Page 3
	list3, token3, err := client.ListObjectsPage(context.Background(), bucketName, "", token2, 2)
	assert.NilError(t, err)
	assert.Equal(t, len(list3.Objects), 1)
	assert.Equal(t, token3, "")
}

func TestClient_NetworkInterruption(t *testing.T) {
	t.Run("Download Interruption", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If it's a metadata request (JSON API style)
			if strings.Contains(r.URL.Path, "/o/") && !strings.Contains(r.URL.RawQuery, "alt=media") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"o","size":"1000"}`))
				return
			}

			// For media download (either JSON alt=media or XML API GET)
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 100))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
		}))
		defer server.Close()

		ctx := context.Background()
		sc, err := storage.NewClient(ctx,
			option.WithHTTPClient(server.Client()),
			option.WithEndpoint(server.URL),
			option.WithoutAuthentication(),
		)
		assert.NilError(t, err)
		client := gcs.NewClient(sc)

		dest := filepath.Join(t.TempDir(), "interrupted.txt")
		err = client.DownloadObject(ctx, "b", "o", dest, nil)
		assert.Assert(t, err != nil, "expected error due to network interruption")
		// The error message might vary depending on where exactly it fails
		assert.Assert(t, strings.Contains(err.Error(), "failed to copy content") ||
			strings.Contains(err.Error(), "unexpected EOF") ||
			strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "EOF"), "unexpected error message: %v", err)
	})

	t.Run("Upload Interruption", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// For uploads, we can hijack during the POST/PUT request
			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
		}))
		defer server.Close()

		ctx := context.Background()
		sc, err := storage.NewClient(ctx,
			option.WithHTTPClient(server.Client()),
			option.WithEndpoint(server.URL),
			option.WithoutAuthentication(),
		)
		assert.NilError(t, err)
		client := gcs.NewClient(sc)

		src := filepath.Join(t.TempDir(), "upload.txt")
		_ = os.WriteFile(src, make([]byte, 1000), 0600)

		err = client.UploadObject(ctx, "b", "o", src)
		assert.Assert(t, err != nil, "expected error due to network interruption")
	})
}

func TestClient_NewReader(t *testing.T) {
	content := []byte("sequential read content")
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "file.txt"}, Content: content},
	}
	_, client := setupTestServer(t, objects)

	t.Run("Success", func(t *testing.T) {
		rc, err := client.NewReader(context.Background(), "b1", "file.txt")
		assert.NilError(t, err)
		defer func() { _ = rc.Close() }()

		data, err := io.ReadAll(rc)
		assert.NilError(t, err)
		assert.DeepEqual(t, data, content)
	})
}

func TestClient_DownloadPrefixAsZip_ErrorsAndProgress(t *testing.T) {
	objects := []fakestorage.Object{
		{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "b1", Name: "folder1/file1.txt", Size: 8}, Content: []byte("content1")},
	}
	_, client := setupTestServer(t, objects)

	t.Run("Destination Dir Error", func(t *testing.T) {
		conflictFile := filepath.Join(t.TempDir(), "conflict")
		err := os.WriteFile(conflictFile, []byte("data"), 0600)
		assert.NilError(t, err)

		badDest := filepath.Join(conflictFile, "file.zip")
		err = client.DownloadPrefixAsZip(context.Background(), "b1", "folder1/", badDest, nil)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "failed to create destination directory"))
	})

	t.Run("Destination File Error", func(t *testing.T) {
		destDir := t.TempDir()
		// Create a directory where the file should be, causing os.Create to fail
		err := os.Mkdir(filepath.Join(destDir, "folder1.zip"), 0750)
		assert.NilError(t, err)

		err = client.DownloadPrefixAsZip(context.Background(), "b1", "folder1/", filepath.Join(destDir, "folder1.zip"), nil)
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "failed to create destination file"))
	})

	t.Run("With Progress Tracker", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "folder1.zip")
		var totalProgress int64
		var currentProgress int64

		onProg := func(current, total int64) {
			currentProgress = current
			totalProgress = total
		}

		err := client.DownloadPrefixAsZip(context.Background(), "b1", "folder1/", dest, onProg)
		assert.NilError(t, err)

		assert.Equal(t, totalProgress, int64(8)) // size of content1
		assert.Equal(t, currentProgress, totalProgress)
	})
}
