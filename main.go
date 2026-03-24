// Package main provides functionality for main.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"cloud.google.com/go/storage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/config"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
)

var version = "dev"

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	if err := run(os.Args[1:], nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main application logic.
// client is an optional dependency injection for testing. If nil, it initializes the real GCS client.
func run(args []string, client tui.GCSClient) error {
	if len(args) > 0 && args[0] == "init" {
		initCmd := flag.NewFlagSet("init", flag.ContinueOnError)

		// We use a custom string slice flag since the standard library doesn't have it built-in.
		var projects stringSlice
		initCmd.Var(&projects, "project", "GCP Project ID to add to config (can be specified multiple times)")

		downloadDir := initCmd.String("download-dir", "", "Directory where files will be downloaded (default is ~/Downloads)")
		nerdIcons := initCmd.Bool("nerd-icons", false, "Enable Nerd Font icons")

		if err := initCmd.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}

		if len(projects) == 0 {
			return fmt.Errorf("at least one --project is required")
		}

		cfg := config.Config{
			Projects:    projects,
			DownloadDir: *downloadDir,
			NerdIcons:   *nerdIcons,
		}

		return config.InitConfig("", cfg)
	}

	fs := flag.NewFlagSet("lazygcs", flag.ContinueOnError)
	versionFlag := fs.Bool("version", false, "Print version and exit")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `lazygcs - A fast, keyboard-driven TUI for Google Cloud Storage.

Usage:
  lazygcs [flags]
  lazygcs init --project <project_id> [--project <project_id> ...] [--download-dir <dir>] [--nerd-icons]

Commands:
  init        Initialize configuration file with provided project IDs

Flags:
  -version    Print version and exit
  -help       Print this help message

Configuration:
  lazygcs requires a configuration file at ~/.config/lazygcs/config.toml
  containing your Google Cloud project IDs.

  Example:
  projects = ["my-gcp-project-1", "my-gcp-project-2"]
  download_dir = "~/Downloads"
  fuzzy_search = true
  nerd_icons = true

Controls:
  Use arrow keys or h/j/k/l to navigate.
  Enter    - Drill down into folders/objects
  Space    - Select multiple objects
  d        - Download selected objects
  /        - Search current view
  ?        - Show full help menu
  q/Ctrl+C - Quit

For more details, see the ? help menu inside the application.
`)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *versionFlag {
		// If version is still "dev" (e.g., when using 'go install'), try to read the module version from build info.
		if version == "dev" {
			if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
				version = info.Main.Version
			}
		}
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

	m := tui.NewModel(cfg.Projects, client, cfg.DownloadDir, cfg.FuzzySearch, cfg.NerdIcons)

	p := tea.NewProgram(&m, tea.WithAltScreen())
	m.SetSendMsg(p.Send)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("alas, it seems we've encountered an error: %w", err)
	}

	return nil
}
