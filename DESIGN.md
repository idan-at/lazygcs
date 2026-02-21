# lazygcs - Design Document

## 1. Overview
`lazygcs` is a terminal user interface (TUI) for Google Cloud Storage (GCS). It aims to provide a fast, intuitive, and "lazy" way to navigate, manage, and interact with GCS buckets and objects.

## 2. Tech Stack
*   **Language:** Go (v1.21+)
*   **UI Framework:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Model-View-Update architecture)
*   **Styling:** [Lip Gloss](https://github.com/charmbracelet/lipgloss)
*   **GCS SDK:** [Google Cloud Storage Go Client](https://pkg.go.dev/cloud.google.com/go/storage)
*   **Testing:** [fsouza/fake-gcs-server/fakestorage](https://github.com/fsouza/fake-gcs-server)
    *   *Strategy:* Use the real `storage.Client` pointed at a local in-memory fake server. No manual interface mocking of the GCS client.

## 3. Architecture
The application follows the **Elm Architecture** (Model-View-Update) provided by Bubble Tea.

### Core Components
1.  **Model:** Represents the state of the application.
    *   Current View (List, Help, Input, etc.)
    *   Navigation Stack (Current Bucket, Prefix/Folder path)
    *   Selection State (Cursor position, Selected items)
    *   Data Cache (Buffered list of objects/buckets to minimize API calls)
2.  **Update:** Handles messages (KeyPasses, WindowSize, API responses) and returns a new Model + Commands.
    *   **Navigation Logic:** Handle `j`/`k` (cursor), `h`/`l` (directory traversal).
    *   **Data Fetching:** Async commands (Cmd) to list buckets/objects.
3.  **View:** Renders the TUI as a string based on the Model.
    *   **Layout:** Miller Columns (Parent | Current | Preview).

## 4. Layout (Miller Columns)
Inspired by `ranger` and `yazi`.

| Left Pane (Parent/Context) | Middle Pane (Current List) | Right Pane (Preview/Info) |
| :------------------------ | :------------------------- | :------------------------ |
| - List of Buckets (Root)  | - **folder/**              | *If Folder Selected:*     |
| - or Parent Folder items  | - file.txt                 | - Item Count              |
|                           | - image.png                | - Total Size              |
|                           |                            | *If File Selected:*       |
|                           |                            | - Metadata (Type, Size)   |
|                           |                            | - Content Preview (Text)  |
|                           |                            | - **Object Versions**     |

## 5. Key Features
1.  **Navigation:**
    *   `j`/`k`: Move cursor down/up.
    *   `h`: Go to Parent Directory / Back to Bucket List.
    *   `l`: Enter Directory / Select Bucket.
    *   `Enter`: Same as `l`.
2.  **Operations:**
    *   `space`: Toggle selection (multi-select support planned).
    *   `d`: Download selected item(s).
    *   `x`: Delete selected item(s) (with confirmation).
    *   `v`: Toggle Object Versions view in Right Pane.
3.  **Search:**
    *   `/`: Filter current list.
    *   `Ctrl+p`: Global Bucket Search (scan configured projects).
4.  **System:**
    *   `?`: Toggle Help.
    *   `q`: Quit.

## 6. Configuration Strategy
Project IDs are resolved in the following precedence order:
1.  **CLI Arguments:** `lazygcs project-a project-b` (Takes precedence).
2.  **Environment Variable:** `LAZYGCS_PROJECTS` (Comma-separated list).
3.  **Environment Variable:** `GOOGLE_CLOUD_PROJECT` (Standard fallback).
4.  **Config File:** `~/.config/lazygcs/config.toml`
    ```toml
    projects = ["project-a", "project-b"]
    ```
5.  **Fallback:** Active `gcloud` project (`gcloud config get-value project`).

## 7. Testing Strategy
*   **E2E Tests (`e2e_test.go`):** Verify the entire binary lifecycle.
    *   Build the binary.
    *   Run against a `fakestorage` server via env vars (`STORAGE_EMULATOR_HOST`).
    *   Assert output contains expected data.
*   **Integration Tests (`integration_test.go`):** Verify core logic functions (e.g., `Run`).
    *   Inject `storage.Client` (connected to `fakestorage`) directly into functions.
    *   Verify logic correctness without process overhead.
*   **Unit Tests:** Verify pure logic and UI state.
    *   Test `Update` function transitions.
    *   Test View rendering logic.
    *   No GCS calls involved.

## 8. Development Philosophy
*   **TDD First:** All features must be implemented test-first.
    *   **Fail First:** Create a reproduction test case or feature test that fails *for the right reason*.
    *   **Fake it:** Use `fakestorage` to simulate GCS state (buckets, objects, versions).
*   **Clean Code:** Refactor often. Keep the `Update` loop clean by delegating complex logic to helper functions or sub-models.
