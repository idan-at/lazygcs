package gcs

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// Client provides methods to interact with Google Cloud Storage.
// It wraps the official storage.Client to provide a simplified API for the TUI.
type Client struct {
	storageClient *storage.Client
}

// ObjectMetadata holds metadata for a GCS object.
type ObjectMetadata struct {
	Name        string
	Size        int64
	ContentType string
	Updated     time.Time
}

// ObjectList holds the list of objects and prefixes (folders) returned by ListObjects.
type ObjectList struct {
	Objects  []ObjectMetadata
	Prefixes []string
}

// NewClient initializes a new GCS Client with the provided storage client.
func NewClient(sc *storage.Client) *Client {
	return &Client{storageClient: sc}
}

// ListBuckets retrieves the names of all buckets accessible within the given projects.
//
// Arguments:
//   - ctx: The context for the API calls.
//   - projectIDs: A list of Google Cloud Project IDs to scan for buckets.
//
// Returns:
//   - []string: A combined list of bucket names from all projects.
//   - error: If any underlying API call fails.
func (c *Client) ListBuckets(ctx context.Context, projectIDs []string) ([]string, error) {
	var allBuckets []string
	for _, pID := range projectIDs {
		it := c.storageClient.Buckets(ctx, pID)
		for {
			bucketAttrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to list buckets for project %q: %w", pID, err)
			}
			allBuckets = append(allBuckets, bucketAttrs.Name)
		}
	}
	return allBuckets, nil
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
			list.Prefixes = append(list.Prefixes, attrs.Prefix)
		} else {
			if attrs.Name != prefix {
				list.Objects = append(list.Objects, ObjectMetadata{
					Name:        attrs.Name,
					Size:        attrs.Size,
					ContentType: attrs.ContentType,
					Updated:     attrs.Updated,
				})
			}
		}
	}

	return list, nil
}
