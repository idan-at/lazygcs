package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCS client: %v", err)
	}
	defer client.Close()

	if err := Run(ctx, client, os.Stdout); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// Run executes the core application logic, listing buckets from the provided client.
func Run(ctx context.Context, client *storage.Client, w io.Writer) error {
	// For now, we list buckets without a specific project filter.
	// In the emulator, this returns all buckets.
	it := client.Buckets(ctx, "")
	for {
		bucketAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list buckets: %w", err)
		}
		fmt.Fprintln(w, bucketAttrs.Name)
	}
	return nil
}
