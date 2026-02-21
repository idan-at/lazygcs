package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"lazygcs/internal/config"
)

func main() {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCS client: %v", err)
	}
	defer client.Close()

	// Determine config path: ~/.config/lazygcs/config.toml
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".config", "lazygcs", "config.toml")

	cfg, err := config.Load(os.Args[1:], configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Projects) == 0 {
		log.Fatal("No project IDs found. Please provide them as arguments or configure them in ~/.config/lazygcs/config.toml")
	}

	if err := Run(ctx, cfg.Projects, client, os.Stdout); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// Run executes the core application logic, listing buckets from the provided client for each project.
func Run(ctx context.Context, projectIDs []string, client *storage.Client, w io.Writer) error {
	for _, pID := range projectIDs {
		it := client.Buckets(ctx, pID)
		for {
			bucketAttrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to list buckets for project %q: %w", pID, err)
			}
			fmt.Fprintln(w, bucketAttrs.Name)
		}
	}
	return nil
}
