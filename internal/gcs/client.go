// Package gcs provides functionality for gcs.
package gcs

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// Client ...
type Client struct {
	storageClient *storage.Client
}

// ObjectMetadata holds metadata for a GCS object.
type ObjectMetadata struct {
	Name            string
	Bucket          string
	Size            int64
	ContentType     string
	ContentEncoding string
	CacheControl    string
	StorageClass    string
	Generation      int64
	Metageneration  int64
	ETag            string
	MD5             []byte
	CRC32C          uint32
	Updated         time.Time
	Created         time.Time
	Owner           string
	Metadata        map[string]string
}

// PrefixMetadata holds metadata for a GCS common prefix (virtual folder).
type PrefixMetadata struct {
	Name    string
	Created time.Time
	Updated time.Time
	Owner   string
	Fetched bool // Indicates if a metadata fetch has been attempted
	Err     error
}

// ObjectList holds the list of objects and prefixes (folders) returned by ListObjects.
type ObjectList struct {
	Objects  []ObjectMetadata
	Prefixes []PrefixMetadata
}

// NewClient initializes a new GCS Client with the provided storage client.
func NewClient(storageClient *storage.Client) *Client {
	return &Client{
		storageClient: storageClient,
	}
}

// ProgressFunc is a callback for tracking download progress.
type ProgressFunc func(current, total int64)

// progressWriter wraps an io.Writer to track progress.
type progressWriter struct {
	w       io.Writer
	total   int64
	current int64
	onProg  ProgressFunc
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	pw.current += int64(n)
	if pw.onProg != nil {
		pw.onProg(pw.current, pw.total)
	}
	return n, err
}

// progressReader wraps an io.Reader to track progress.
type progressReader struct {
	r       io.Reader
	total   int64
	current *int64 // Pointer to share progress across multiple readers for zip
	onProg  ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	*pr.current += int64(n)
	if pr.onProg != nil {
		pr.onProg(*pr.current, pr.total)
	}
	return n, err
}

// DownloadObject downloads a GCS object to the local file system.
func (c *Client) DownloadObject(ctx context.Context, bucketName, objectName, dest string, onProg ProgressFunc) error {
	rc, err := c.storageClient.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader for %q in %q: %w", objectName, bucketName, err)
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// #nosec G304
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() { _ = f.Close() }()

	pw := &progressWriter{
		w:      f,
		total:  rc.Attrs.Size,
		onProg: onProg,
	}

	if _, err := io.Copy(pw, rc); err != nil {
		return fmt.Errorf("failed to copy content to destination: %w", err)
	}

	return nil
}

// UploadObject uploads a local file to GCS.
func (c *Client) UploadObject(ctx context.Context, bucketName, objectName, src string) error {
	// #nosec G304
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = f.Close() }()

	wc := c.storageClient.Bucket(bucketName).Object(objectName).NewWriter(ctx)
	if _, err := io.Copy(wc, f); err != nil {
		_ = wc.Close()
		return fmt.Errorf("failed to copy content to GCS: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to finalize upload to GCS: %w", err)
	}

	return nil
}

// CreateBucket creates a new GCS bucket.
func (c *Client) CreateBucket(ctx context.Context, projectID, bucketName string) error {
	if err := c.storageClient.Bucket(bucketName).Create(ctx, projectID, nil); err != nil {
		return fmt.Errorf("failed to create bucket %q in project %q: %w", bucketName, projectID, err)
	}
	return nil
}

// CreateEmptyObject creates a 0-byte object in the specified bucket.
func (c *Client) CreateEmptyObject(ctx context.Context, bucketName, objectName string) error {
	wc := c.storageClient.Bucket(bucketName).Object(objectName).NewWriter(ctx)
	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to create empty object %q in %q: %w", objectName, bucketName, err)
	}
	return nil
}

// DownloadPrefixAsZip downloads all objects under a prefix and packages them into a ZIP file.
func (c *Client) DownloadPrefixAsZip(ctx context.Context, bucketName, prefix, dest string, onProg ProgressFunc) error {
	it := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: prefix})

	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// #nosec G304
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Calculate total size for progress tracking if onProg is provided
	var totalSize int64
	var currentSize int64
	if onProg != nil {
		sizeIt := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: prefix})
		for {
			attrs, err := sizeIt.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to calculate total size for prefix %q: %w", prefix, err)
			}
			if attrs.Name != prefix && filepath.Base(attrs.Name) != "" {
				totalSize += attrs.Size
			}
		}
	}

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list objects for prefix %q: %w", prefix, err)
		}

		// Skip "directory" objects (objects ending with /)
		if attrs.Name == prefix || filepath.Base(attrs.Name) == "" {
			continue
		}

		rc, err := c.storageClient.Bucket(bucketName).Object(attrs.Name).NewReader(ctx)
		if err != nil {
			return fmt.Errorf("failed to create reader for %q: %w", attrs.Name, err)
		}

		// Create file in zip with relative path
		relPath := attrs.Name[len(prefix):]
		w, err := zw.Create(relPath)
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("failed to create zip entry for %q: %w", attrs.Name, err)
		}

		pr := &progressReader{
			r:       rc,
			total:   totalSize,
			current: &currentSize,
			onProg:  onProg,
		}

		if _, err := io.Copy(w, pr); err != nil {
			_ = rc.Close()
			return fmt.Errorf("failed to copy %q to zip: %w", attrs.Name, err)
		}
		_ = rc.Close()
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalize zip file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close zip file: %w", err)
	}

	return nil
}

// GetObjectContent retrieves the first 1KB of content for a specific object.
func (c *Client) GetObjectContent(ctx context.Context, bucketName, objectName string) (string, error) {
	rc, err := c.storageClient.Bucket(bucketName).Object(objectName).NewRangeReader(ctx, 0, 1024)
	if err != nil {
		// A 416 (InvalidRange) error typically means the object is 0 bytes.
		// Return empty content instead of failing.
		if strings.Contains(err.Error(), "416") || strings.Contains(err.Error(), "InvalidRange") || strings.Contains(err.Error(), "Requested range not satisfiable") {
			return "", nil
		}
		return "", fmt.Errorf("failed to create reader for %q in %q: %w", objectName, bucketName, err)
	}
	defer func() { _ = rc.Close() }()

	bytes, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("failed to read content from %q in %q: %w", objectName, bucketName, err)
	}

	return string(bytes), nil
}

type gcsReaderAt struct {
	ctx context.Context
	obj *storage.ObjectHandle
}

func (r *gcsReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	rc, err := r.obj.NewRangeReader(r.ctx, off, int64(len(p)))
	if err != nil {
		// handle out of range if length exceeds object size
		return 0, err
	}
	defer func() { _ = rc.Close() }()

	n, err = io.ReadFull(rc, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return n, io.EOF
	}
	return n, err
}

// NewReaderAt returns an io.ReaderAt for a specific object, allowing random access.
func (c *Client) NewReaderAt(ctx context.Context, bucketName, objectName string) io.ReaderAt {
	return &gcsReaderAt{
		ctx: ctx,
		obj: c.storageClient.Bucket(bucketName).Object(objectName),
	}
}

// NewReader returns a sequential io.ReadCloser for a specific object.
func (c *Client) NewReader(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	return c.storageClient.Bucket(bucketName).Object(objectName).NewReader(ctx)
}

// GetObjectMetadata retrieves full metadata for a specific object or directory stub.
func (c *Client) GetObjectMetadata(ctx context.Context, bucketName, objectName string) (*ObjectMetadata, error) {
	attrs, err := c.storageClient.Bucket(bucketName).Object(objectName).Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for %q in %q: %w", objectName, bucketName, err)
	}

	return &ObjectMetadata{
		Name:            attrs.Name,
		Bucket:          attrs.Bucket,
		Size:            attrs.Size,
		ContentType:     attrs.ContentType,
		ContentEncoding: attrs.ContentEncoding,
		CacheControl:    attrs.CacheControl,
		StorageClass:    attrs.StorageClass,
		Generation:      attrs.Generation,
		Metageneration:  attrs.Metageneration,
		ETag:            attrs.Etag,
		MD5:             attrs.MD5,
		CRC32C:          attrs.CRC32C,
		Updated:         attrs.Updated,
		Created:         attrs.Created,
		Owner:           attrs.Owner,
		Metadata:        attrs.Metadata,
	}, nil
}

// ProjectBuckets holds the buckets for a specific project.
type ProjectBuckets struct {
	ProjectID string
	Buckets   []string
}

// ListBucketsPage retrieves a specific page of buckets for a specific project.
//
// Arguments:
//   - ctx: The context for the API call.
//   - projectID: The Google Cloud Project ID to scan for buckets.
//   - pageToken: Token for pagination.
//   - pageSize: Maximum number of buckets to return.
//
// Returns:
//   - []string: A list of bucket names.
//   - string: The next page token.
//   - error: If the underlying API call fails.
func (c *Client) ListBucketsPage(ctx context.Context, projectID string, pageToken string, pageSize int) ([]string, string, error) {
	it := c.storageClient.Buckets(ctx, projectID)
	pager := iterator.NewPager(it, pageSize, pageToken)
	var attrs []*storage.BucketAttrs
	nextToken, err := pager.NextPage(&attrs)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch buckets page for project %q: %w", projectID, err)
	}

	var buckets []string
	for _, attr := range attrs {
		buckets = append(buckets, attr.Name)
	}
	return buckets, nextToken, nil
}

// ListObjectsPage retrieves a specific page of object names and common prefixes (folders).
func (c *Client) ListObjectsPage(ctx context.Context, bucketName, prefix, pageToken string, pageSize int) (*ObjectList, string, error) {
	it := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})

	pager := iterator.NewPager(it, pageSize, pageToken)
	var attrs []*storage.ObjectAttrs
	nextToken, err := pager.NextPage(&attrs)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch page for bucket %q: %w", bucketName, err)
	}

	list := &ObjectList{}
	for _, attr := range attrs {
		if attr.Prefix != "" {
			list.Prefixes = append(list.Prefixes, PrefixMetadata{Name: attr.Prefix})
		} else {
			if attr.Name != prefix {
				list.Objects = append(list.Objects, ObjectMetadata{
					Name:            attr.Name,
					Bucket:          attr.Bucket,
					Size:            attr.Size,
					ContentType:     attr.ContentType,
					ContentEncoding: attr.ContentEncoding,
					CacheControl:    attr.CacheControl,
					StorageClass:    attr.StorageClass,
					Generation:      attr.Generation,
					Metageneration:  attr.Metageneration,
					ETag:            attr.Etag,
					MD5:             attr.MD5,
					CRC32C:          attr.CRC32C,
					Updated:         attr.Updated,
					Created:         attr.Created,
					Owner:           attr.Owner,
					Metadata:        attr.Metadata,
				})
			}
		}
	}

	return list, nextToken, nil
}

// ListObjects retrieves object names and common prefixes (folders) for a specific bucket and prefix.
// It uses "/" as a delimiter to enable hierarchical navigation.
//
// Arguments:
//   - ctx: The context for the API call.
//   - bucketName: The name of the bucket to list.
//   - prefix: The object prefix (folder path) to list within.
//
// Returns:
//   - *ObjectList: A struct containing lists of objects and prefixes.
//   - error: If the underlying API call fails.
func (c *Client) ListObjects(ctx context.Context, bucketName, prefix string) (*ObjectList, error) {
	it := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})

	list := &ObjectList{}

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects for bucket %q: %w", bucketName, err)
		}

		if attrs.Prefix != "" {
			list.Prefixes = append(list.Prefixes, PrefixMetadata{Name: attrs.Prefix})
		} else {
			if attrs.Name != prefix {
				list.Objects = append(list.Objects, ObjectMetadata{
					Name:            attrs.Name,
					Bucket:          attrs.Bucket,
					Size:            attrs.Size,
					ContentType:     attrs.ContentType,
					ContentEncoding: attrs.ContentEncoding,
					CacheControl:    attrs.CacheControl,
					StorageClass:    attrs.StorageClass,
					Generation:      attrs.Generation,
					Metageneration:  attrs.Metageneration,
					ETag:            attrs.Etag,
					MD5:             attrs.MD5,
					CRC32C:          attrs.CRC32C,
					Updated:         attrs.Updated,
					Created:         attrs.Created,
					Owner:           attrs.Owner,
					Metadata:        attrs.Metadata,
				})
			}
		}
	}

	return list, nil
}
