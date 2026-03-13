package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"cloud.google.com/go/storage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main application logic.
// client is an optional dependency injection for testing. If nil, it initializes the real GCS client.
func run(args []string, client tui.GCSClient) error {
	fs := flag.NewFlagSet("lazygcs", flag.ContinueOnError)
	versionFlag := fs.Bool("version", false, "Print version and exit")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if *versionFlag {
		fmt.Printf("lazygcs %s\n", version)
		return nil
	}

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Projects) == 0 {
		return fmt.Errorf("no project IDs found in config file. Please configure them in ~/.config/lazygcs/config.toml")
	}

	if client == nil {
		ctx := context.Background()
		storageClient, err := storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create GCS client: %w", err)
		}
		defer func() { _ = storageClient.Close() }()
		client = gcs.NewClient(storageClient)
	}

	m := tui.NewModel(cfg.Projects, client, cfg.DownloadDir, cfg.FuzzySearch, cfg.Icons)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("alas, it seems we've encountered an error: %w", err)
	}

	return nil
}
