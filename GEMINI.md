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
    *   Follow `docs/DESIGN.md`.
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
3.  `go test -v -coverpkg=./... -coverprofile=coverage.out ./...` (Verify tests pass with coverage)

## Key Files
*   `docs/DESIGN.md`: The architectural blueprint.
*   `go.mod`: Dependencies.
