package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
	tea "github.com/charmbracelet/bubbletea"
	"lazygcs/internal/config"
	"lazygcs/internal/gcs"
	"lazygcs/internal/tui"
)

func main() {
	ctx := context.Background()

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create GCS client: %v", err)
	}
	defer storageClient.Close()

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

	gcsClient := gcs.NewClient(storageClient)
	m := tui.NewModel(cfg.Projects, gcsClient)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Alas, it seems we've encountered an error: %v", err)
	}
}
