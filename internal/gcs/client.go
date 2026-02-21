package gcs

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// Client wraps the GCS storage client.
type Client struct {
	storageClient *storage.Client
}

// NewClient creates a new GCS client.
func NewClient(sc *storage.Client) *Client {
	return &Client{storageClient: sc}
}

// ListBuckets fetches buckets for the given project IDs.
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

// ListObjects fetches objects and prefixes (folders) for a given bucket and prefix.
func (c *Client) ListObjects(ctx context.Context, bucketName, prefix string) ([]string, []string, error) {
	var objects []string
	var prefixes []string

	it := c.storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list objects for bucket %q: %w", bucketName, err)
		}

		if attrs.Prefix != "" {
			prefixes = append(prefixes, attrs.Prefix)
		} else {
			objects = append(objects, attrs.Name)
		}
	}

	return objects, prefixes, nil
}
