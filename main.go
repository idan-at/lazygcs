package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	tea "github.com/charmbracelet/bubbletea"
	"lazygcs/internal/config"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("lazygcs %s\n", version)
		os.Exit(0)
	}

	ctx := context.Background()

	// Determine config path: LAZYGCS_CONFIG env var or ~/.config/lazygcs/config.toml
	configPath := os.Getenv("LAZYGCS_CONFIG")
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "lazygcs", "config.toml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", configPath, err)
	}

	if len(cfg.Projects) == 0 {
		log.Fatal("No project IDs found in config file. Please configure them in ~/.config/lazygcs/config.toml")
	}

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCS client: %v", err)
	}
	defer func() { _ = storageClient.Close() }()

	gcsClient := gcs.NewClient(storageClient)
	m := tui.NewModel(cfg.Projects, gcsClient, cfg.DownloadDir, cfg.FuzzySearch, cfg.Icons)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Alas, it seems we've encountered an error: %v", err)
	}
}
