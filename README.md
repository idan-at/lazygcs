# lazygcs ☁️🦥

A fast, keyboard-driven Terminal User Interface (TUI) for exploring and managing Google Cloud Storage (GCS).

Tired of clicking through the slow Cloud Console? Want to navigate your buckets like you navigate your local file system? `lazygcs` brings the speed and efficiency of tools like `ranger` and `yazi` straight to your GCS workflow using an intuitive Miller Column layout.

![lazygcs preview placeholder](https://via.placeholder.com/800x400.png?text=lazygcs+Terminal+UI) <!-- Feel free to replace this with an actual screenshot! -->

## ✨ Features

*   **Miller Column Navigation:** Instantly see your buckets (grouped by project), the current directory's contents, and a preview of the selected file all at once.
*   **Lightning Fast:** Keyboard-centric workflow means your hands never have to leave the home row.
*   **File Previews:** Peek at file contents and metadata (size, type, created/updated dates) without downloading them. Safely ignores binary files to prevent terminal corruption!
*   **Inline Search:** Instantly filter your buckets or objects. Supports exact matching or configurable **fuzzy search**.
*   **Error History Overlay:** Quickly diagnose permission or network issues with a dedicated error history view (`Ctrl+e`).
*   **Multi-Select & Batch Downloads:** Select multiple files or entire directories and download them all concurrently. Directories are automatically packaged into `.zip` archives!
*   **Vim-like Keybindings:** `j`/`k` for vertical movement, `h`/`l` for entering and exiting directories.

## 🚀 Installation

Ensure you have [Go](https://golang.org/doc/install) (v1.24+) installed, then run:

```bash
go install github.com/idan-at/lazygcs@latest
```

Alternatively, clone the repository and build it manually:

```bash
git clone https://github.com/idan-at/lazygcs.git
cd lazygcs
go build -o lazygcs main.go
sudo mv lazygcs /usr/local/bin/
```

### Usage
Run `lazygcs` to start the application.
You can use the `--version` flag to print the current version:
```bash
lazygcs --version
```

## ⚙️ Configuration

`lazygcs` is configured entirely via a TOML file. 

Create a file at `~/.config/lazygcs/config.toml` (or define the `LAZYGCS_CONFIG` environment variable to point to a custom path).

### Example `config.toml`

```toml
# Required: A list of Google Cloud Project IDs you want to explore.
projects = ["my-production-project", "my-staging-project"]

# Optional: The directory where files will be downloaded.
# Defaults to ~/Downloads if not specified.
download_dir = "/Users/me/Desktop/gcs_downloads"

# Optional: Enable fuzzy searching when using the '/' filter.
# Defaults to false (exact substring match).
fuzzy_search = true

# Optional: Display Nerd Font icons next to buckets, folders, and files.
# Requires a Nerd Font to be installed and active in your terminal.
# Defaults to false.
icons = true
```

### Authentication
`lazygcs` relies on the standard Google Cloud Go SDK authentication. Make sure you are authenticated with your Google Cloud account before running the tool:

```bash
gcloud auth application-default login
```

## ⌨️ Keybindings

### Navigation
*   `j` or `↓`: Move cursor down
*   `k` or `↑`: Move cursor up
*   `l` or `Enter` or `→`: Enter a bucket, directory, or expand/collapse a project header.
*   `h` or `←`: Go back to the parent directory, bucket list, or collapse a project header.

### Actions
*   `space`: Toggle selection of the highlighted item (Multi-select)
*   `d`: Download the currently highlighted item (or all selected items). Directories are downloaded as `.zip` files.
*   `Ctrl+e`: Toggle the Error History modal (visible when errors occur)
*   `/`: Start searching/filtering the current column
*   `?`: Toggle the Help Menu overlay
*   `esc` or `Enter`: Exit search mode
*   `q` or `Ctrl+c`: Quit the application

## 🗺️ Roadmap / Upcoming Features

*   [ ] Implement Delete (`x`) action for single and multi-selected objects.
*   [ ] Add Object Versions view (`v`) to inspect historical versions of a file.

## 🤝 Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the [issues page](https://github.com/idan-at/lazygcs/issues).

## 📝 License

[MIT License](https://github.com/idan-at/lazygcs/blob/main/LICENSE)
