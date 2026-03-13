package gcs

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type Client struct {
	storageClient *storage.Client
}

// ObjectMetadata holds metadata for a GCS object.
type ObjectMetadata struct {
	Name        string
	Size        int64
	ContentType string
	Updated     time.Time
	Created     time.Time
	Owner       string
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

// DownloadObject downloads a GCS object to the local file system.
func (c *Client) DownloadObject(ctx context.Context, bucketName, objectName, dest string) error {
	rc, err := c.storageClient.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader for %q in %q: %w", objectName, bucketName, err)
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, rc); err != nil {
		return fmt.Errorf("failed to copy content to destination: %w", err)
	}

	return nil
}

// DownloadPrefixAsZip downloads all objects under a prefix and packages them into a ZIP file.
func (c *Client) DownloadPrefixAsZip(ctx context.Context, bucketName, prefix, dest string) error {
	it := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: prefix})

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
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

		if _, err := io.Copy(w, rc); err != nil {
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
	defer rc.Close()

	n, err = io.ReadFull(rc, p)
	if err == io.ErrUnexpectedEOF {
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
		Name:        attrs.Name,
		Size:        attrs.Size,
		ContentType: attrs.ContentType,
		Updated:     attrs.Updated,
		Created:     attrs.Created,
		Owner:       attrs.Owner,
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
					Name:        attr.Name,
					Size:        attr.Size,
					ContentType: attr.ContentType,
					Updated:     attr.Updated,
					Created:     attr.Created,
					Owner:       attr.Owner,
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
		if err == iterator.Done {
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
					Name:        attrs.Name,
					Size:        attrs.Size,
					ContentType: attrs.ContentType,
					Updated:     attrs.Updated,
					Created:     attrs.Created,
					Owner:       attrs.Owner,
				})
			}
		}
	}

	return list, nil
}
