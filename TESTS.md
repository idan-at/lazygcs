# Test Refactoring Plan

## Goals
1. Increase true integration and unit test coverage.
2. Reorganize test files into a coherent, maintainable structure.
3. Improve test reliability by moving away from brittle UI text assertions where appropriate.
4. Ensure cross-package coverage is tracked accurately in CI.

---

## Phase 1: Test Utilities & Preview Unit Tests (Decoupling) [COMPLETED]
**Objective:** Decouple preview parsing logic from the TUI rendering to drastically speed up tests and improve reliability.

1. **Create Shared Utilities:** [DONE]
   - Created `internal/testutil/testutil.go`.
   - Moved `fake-gcs-server` setup and mocking helpers into the shared package.
2. **Extract Preview Tests:** [DONE]
   - Removed all `TestRichPreview_*` and `TestDockerPreview_*` from `tests/integration_test.go`.
   - Created `internal/preview/previewers_test.go` with comprehensive table-driven tests for all previewers.
3. **Registry Verification:** [DONE]
   - Implemented `preview.NewDefaultRegistry()` to centralize registration.
   - Added verification in `internal/preview/registry_test.go` to ensure correct routing.
4. **Cleanup & Integration:** [DONE]
   - Refactored `internal/tui/model.go` to use the centralized registry.
   - Cleaned up `tests/integration_test.go` and `tests/snapshot_test.go` to use `internal/testutil`.
   - Removed redundant local test helpers and extracted test cases.

---

## Phase 2: E2E and Snapshot Tests Refinement
**Objective:** Streamline top-level black-box and visual regression tests.

1. **Minimal `tests/e2e_test.go`:**
   - Strip down to test only the compiled binary execution from the outside.
   - **Test Cases:**
     - Success with a valid `LAZYGCS_CONFIG`.
     - Failure (non-zero exit code & error message) without a config.
     - Ensure `--version` flag prints the correct format.
     - Ensure `--help` flag prints usage instructions.
2. **Enhance `tests/snapshot_test.go`:**
   - Keep existing snapshots: `InitialBucketsView`, `ObjectsAndPreview` (for standard text), `HelpMenu`, `ErrorsModal`.
   - **Add `SearchView`:** Take a snapshot while the user is actively typing in the `/` search bar.
   - **Add `MultiSelectionView`:** Take a snapshot showing multiple items selected to ensure the distinct visual styling (colors/prefix) renders correctly.

---

## Phase 3: TUI Integration Tests (Flows)
**Objective:** Replace the monolithic integration test file with focused, end-to-end user journeys using `teatest`.

1. **Rename & Refactor:**
   - Rename `tests/integration_test.go` to `tests/tui_test.go`.
   - Remove all the exhaustive file-type checks (handled in Phase 1).
2. **Implement Core User Journeys:**
   - **Flow 1: Deep Navigation & Paging:**
     - Go down into a bucket and nested prefixes (`l`).
     - Test paging: half-page (`ctrl+d`/`ctrl+u`) and full-page (`ctrl+f`/`ctrl+b`).
     - Test jumping to the top (`g`) and bottom (`G`) of a large list.
     - Step back one level (`h`).
     - Fast-escape all the way back to the bucket list from a deep prefix (`H`).
   - **Flow 2: Search & Filter Lifecycle:**
     - Open search (`/`), type a query, apply (`Enter`), and verify the filtered list.
     - Clear the filter (`Esc`) and verify the full list is restored.
   - **Flow 3: The Download Journey:**
     - Select multiple files (`space`).
     - Trigger download (`d`) where a local file already exists.
     - Test **Abort**: Press `a` (or `esc`) at the prompt; verify no file changes.
     - Test **Rename**: Press `r` at the prompt, type a new name, press `Enter`; verify the file is saved correctly under the new name.
     - Test **Overwrite**: Trigger download again, press `o`; verify the file is overwritten.
   - **Flow 4: External Integrations:**
     - **Edit (`e`)**: Mock `$EDITOR` to a dummy script that modifies the file. Trigger `e`, wait for editor exit, and verify the file is uploaded to GCS and the preview refreshes.
     - **Open (`O` or configured open key)**: Mock the system open command (e.g., `xdg-open` or `open` in the test path). Trigger open and verify the command executed with the downloaded file path.
   - **Flow 5: App State & Data Refresh:**
     - Toggle Help menu (`?`) on and off.
     - **Refresh (`ctrl+r`)**: Manually inject a new file directly into the `fake-gcs-server` backend, press Refresh, and verify the new file appears in the UI.

---

## Phase 4: View & State Unit Tests (TUI package)
**Objective:** Break down the massive TUI test file and test logic without relying on fragile terminal output parsing.

1. **Split `internal/tui/model_test.go`:**
   - Divide the 55KB file into logical domain files: `buckets_view_test.go`, `objects_view_test.go`, `search_view_test.go`, and keep only core initialization/update logic in `model_test.go`.
2. **State Transition Testing:**
   - Instead of parsing `teatest` terminal output strings, test state transitions by passing specific `tea.Msg` to the model's `Update()` function and asserting directly on the resulting struct state (e.g., `assert.Equal(t, m.ActiveBucket, "expected")`).

---

## Phase 5: CI & Pre-commit Configuration
**Objective:** Ensure true coverage is tracked and maintained.

1. **CI Pipeline:**
   - Update `.github/workflows/ci.yml` (and any other CI files) to run tests with cross-package coverage:
     `go test -coverpkg=./... -coverprofile=coverage.out ./...`
2. **Documentation / Pre-commit:**
   - Update `GEMINI.md` and `README.md` pre-commit checklists to instruct developers to run tests with cross-package coverage.
