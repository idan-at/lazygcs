# lazygcs ☁️🦥

A fast, keyboard-driven Terminal User Interface (TUI) for exploring and managing Google Cloud Storage (GCS).

Tired of clicking through the slow Cloud Console? Want to navigate your buckets like you navigate your local file system? `lazygcs` brings the speed and efficiency of tools like `ranger` and `yazi` straight to your GCS workflow using an intuitive Miller Column layout.

![lazygcs preview](demo/demo.gif)

## ✨ Features

*   **Miller Column Navigation:** Instantly see your buckets (grouped by project), the current directory's contents, and a preview of the selected file all at once.
*   **Lightning Fast:** Keyboard-centric workflow means your hands never have to leave the home row.
*   **Rich File Previews:** Instantly peek at file contents and metadata. Supports styled Markdown rendering and even lets you peek inside archives (ZIP, TAR, JAR) without downloading them! For ZIPs, `lazygcs` reads only the central directory, listing files instantly even for multi-GB archives.
*   **Inline Search:** Instantly filter your buckets or objects. Supports exact matching or configurable **fuzzy search**.
*   **Multi-Select & Batch Downloads:** Select multiple files or entire directories and download them all concurrently. Directories are automatically packaged into `.zip` archives!
*   **Vim-like Keybindings:** `j`/`k` for vertical movement, `h`/`l` for entering and exiting directories, `Ctrl+u`/`Ctrl+d` for page scrolling.
*   **Cross-Platform:** Works seamlessly on macOS, Linux, and Windows.

## 🚀 Installation

Ensure you have [Go](https://golang.org/doc/install) (v1.24+) installed, then run:

```bash
go install github.com/idan-at/lazygcs@latest
```

Alternatively, clone the repository and build it manually with optimization flags (to reduce binary size):

```bash
git clone https://github.com/idan-at/lazygcs.git
cd lazygcs
go build -ldflags="-s -w" -o lazygcs main.go
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
# Defaults to true. To use exact substring matching, set this to false.
fuzzy_search = false

# Optional: Display Nerd Font icons next to buckets, folders, and files.
# Requires a Nerd Font to be installed and active in your terminal.
# Defaults to false.
icons = true
```

### 🔐 Authentication
`lazygcs` relies on the standard Google Cloud Go SDK authentication. Make sure you are authenticated with your Google Cloud account before running the tool:

```bash
gcloud auth application-default login
```

**Required IAM Permissions:**
To use `lazygcs`, the authenticated user or service account must have at least the following roles:
*   `roles/storage.objectViewer` (to list and read objects)
*   `roles/storage.buckets.list` (if you need to list buckets across a project)

## ⌨️ Keybindings

### Basic Navigation
*   `j`/`k` or `↓`/`↑`: Move cursor down/up
*   `l` or `Enter` or `→`: Enter a bucket or directory
*   `h` or `←`: Go back to the parent directory or bucket list

### Basic Actions
*   `space`: Toggle selection of the highlighted item (Multi-select)
*   `d`: Download the currently highlighted item (or all selected items)
*   `/`: Start filtering the current column
*   `q` or `Ctrl+c`: Quit the application

**For a complete and detailed list of all keybindings, please see [docs/KEYBINDINGS.md](docs/KEYBINDINGS.md).**

## 🤝 Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the [issues page](https://github.com/idan-at/lazygcs/issues).

## 📝 License

[MIT License](https://github.com/idan-at/lazygcs/blob/main/LICENSE)