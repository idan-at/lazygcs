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

## Pre-Commit Checklist
Before committing any code, MUST run:
1.  `go fmt ./...` (Format code)
2.  `go vet ./...` (Static analysis)
3.  `go mod tidy` (Clean dependencies)
4.  `go test -v ./...` (Verify tests pass)

## Current Status
*   **Phase:** Navigation Implementation.
*   **Completed:**
    *   Refactored `ListObjects` to return `ObjectList` struct.
    *   Simplified configuration (CLI args, TOML file).
    *   Async TUI initialization (Loading buckets).
    *   Basic bucket list navigation (`j`/`k`).
*   **Next Steps:**
    1.  Implement entering a bucket (`Enter` or `l`) to list objects.
    2.  Implement prefix-based navigation (drill down into "folders").
    3.  Implement backward navigation (`h` or `backspace`).
    4.  Add Miller Columns layout (view parent/current/preview).

## Key Files
*   `DESIGN.md`: The architectural blueprint.
*   `go.mod`: Dependencies.
