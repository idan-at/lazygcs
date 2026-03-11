# lazygcs - Developer Context

## Project Overview
`lazygcs` is a terminal user interface (TUI) for Google Cloud Storage written in Go.
*   **Goal:** A fast, keyboard-driven GCS explorer.
*   **Style:** Miller Columns (Parent | Current | Preview).

## Tech Stack
*   **Language:** Go (v1.21+)
*   **Libraries:**
    *   `github.com/charmbracelet/bubbletea` (TUI framework)
    *   `github.com/charmbracelet/lipgloss` (Styling)
    *   `cloud.google.com/go/storage` (GCS Client)
    *   `github.com/fsouza/fake-gcs-server/fakestorage` (Integration Testing)

## Development Workflow
1.  **TDD is Mandatory.**
    *   **Unit Tests:** For pure logic, state transitions, and UI rendering, write standard Go unit tests.
    *   **Integration Tests:** For GCS interactions, use `fakestorage` to simulate real API behavior.
    *   **Process:**
        1.  Create a failing test.
        2.  Confirm the failure is correct (behavioral, not just syntax).
        3.  Implement the feature.
        4.  Refactor.
2.  **Architecture:**
    *   Follow `DESIGN.md`.
    *   Use `main.go` for the entry point, but keep logic in packages (e.g., `tui`, `gcs`, `models`).

## UI Snapshot Testing
We use `teatest` to verify the exact visual output of the TUI.
*   **Running Snapshot Tests:** `go test -v ./tests -run TestSnapshot`
*   **Updating Snapshots:** If you intentionally change the UI layout, colors, or components, you must update the snapshot "golden" files by running the tests with the `-update` flag:
    ```bash
    go test -v ./tests -run TestSnapshot -update
    ```
    After updating, review the changes in `tests/testdata/` to ensure the new visual output is exactly as expected.

## Pre-Commit Checklist
Before committing any code, MUST run:
1.  `golangci-lint run` (Linting, formatting, and static analysis)
2.  `go mod tidy` (Clean dependencies)
3.  `go test -v ./...` (Verify tests pass)

## Current Status
*   **Phase:** Feature Implementation.
*   **Completed:**
    *   Multi-column Miller Layout (Buckets | Objects).
    *   Folder/Prefix navigation (Drill down/up) with `h`/`l` and `Enter`.
    *   Relative path display for nested objects.
    *   Refactored `ListObjects` to return `ObjectList` struct.
    *   Simplified configuration (TOML file).
    *   Async TUI initialization (Loading buckets).
    *   Basic bucket list navigation (`j`/`k`) with cycle support.
    *   Implemented Download (`d`) action with overwrite/rename handling.
    *   Refactored TUI View logic into smaller files (`views.go`, `types.go`).
    *   Added File Preview pane (3rd column) with binary detection and truncation.
    *   Added inline Search functionality (`/`).
    *   Implemented Help Menu overlay (`?`).
    *   Implemented multi-select (`space`).

## UI Polish Roadmap
To transition the UI from a functional text layout to a delightful, modern terminal application, we will execute the following tasks sequentially.

*   [x] **Task 1: Focus & Active State (Borders & Dimming)**
    *   Add distinct vertical borders to separate the three columns.
    *   Implement "active state" highlighting: The currently active column (Buckets or Objects) should have a bright, primary-colored border.
    *   Implement "inactive state" dimming: Inactive columns should have a muted gray border and their text color should be slightly dimmed to draw the user's eye to the active pane.
*   [x] **Task 2: Elevate Cursor & Selection**
    *   Remove the utilitarian `>` cursor character.
    *   Replace it with a full-row background highlight (inverted color or distinct background) using `lipgloss` to feel like a native application.
    *   Remove the `[x]` and `[ ]` selection brackets.
    *   Replace them by changing the text color of selected items to a distinct, bright color (e.g., Gold/Pink) and optionally prefixing them with a clean `•` or `✓` symbol.
*   [x] **Task 3: Preview Pane Formatting**
    *   Style the metadata keys (`Name:`, `Size:`, `Type:`) with a dimmed color to establish visual hierarchy.
    *   Style the metadata values with a bold or bright color.
    *   Implement a human-readable size formatter (e.g., convert `1048576 bytes` to `1.0 MB`).
    *   Add a distinct visual separator (using `lipgloss.BorderTop`) between the metadata block and the actual file content preview.
*   [x] **Task 4: Status Bar & Footer Polish**
    *   Replace the raw text footer with a structured bottom ribbon (similar to `vim` or `k9s`).
    *   Create a left-aligned context/status pill (e.g., `[ NORMAL ]`, `[ SEARCH ]`, `[ DOWNLOADING ]`) with solid background colors based on state.
    *   Integrate `github.com/charmbracelet/bubbles/help` to render the right-aligned keybind hints dynamically and cleanly.
*   [x] **Task 5: Iconography & Loading States (Optional/Configurable)**
    *   Add an `icons = true/false` flag to `config.toml`.
    *   Implement a helper to map file extensions/types to Nerd Font icons (e.g., `🪣` Buckets, `` Folders, `` Text, `` Images).
    *   Replace the static "Loading..." text with a dynamic spinner using `github.com/charmbracelet/bubbles/spinner`.

## Future Features
    1.  Implement Delete (`x`) action (with multi-select support).
    2.  Add Object Versions view (`v`) to inspect historical versions of a file.

## Key Files
*   `DESIGN.md`: The architectural blueprint.
*   `go.mod`: Dependencies.
