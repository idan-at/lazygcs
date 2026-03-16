# Plan: Expand Vim Keybindings (Phased)

## Objective
Expand Vim keybindings in `lazygcs` to provide a comprehensive, keyboard-driven navigation experience that aligns with standard Vim and file-manager conventions.

## Phase 1: Navigation Expansion [DONE]
**Objective**: Implement jumping to the top/bottom of lists, full-page navigation, and an instant "jump to root" command.

### New Keybindings
- **Top/Bottom (`g` / `G`)**: Jump to the start or end of the current list.
- **Page Up/Down (`ctrl+b` / `ctrl+f`)**: Full screen page jumps, complementing the existing half-page jumps (`ctrl+u` / `ctrl+d`).
- **Go to Root/Buckets (`H`)**: Instantly jump all the way back up the directory tree to the initial Buckets view.

### Implementation Steps
1. **Define Keybindings (`internal/tui/keys.go`)**:
   - Add `Top`, `Bottom`, `PageUp`, `PageDown`, and `Root` fields to the `keyMap` struct.
   - Initialize them with `key.NewBinding()`.
   - Update `OrderedHelp` and `FullHelp` methods.
2. **Handle Input Updates (`internal/tui/update.go`)**:
   - Implement `g` (Top): Set cursor to `0`.
   - Implement `G` (Bottom): Set cursor to last element.
   - Implement `ctrl+b`/`ctrl+f`: Full page jumps.
   - Implement `H` (Root): Return to buckets view.
3. **Documentation**: Update `docs/KEYBINDINGS.md`.
4. **Verification**:
   - Write failing tests in `internal/tui/navigation_test.go`.
   - Update snapshot tests for the help menu.

## Phase 2: Action Expansion (Copy URI & Refresh)
**Objective**: Add utility actions for managing GCS items.

### New Keybindings
- **Copy URI (`y`)**: Copy the `gs://` URI of the currently hovered or selected item(s) to the system clipboard.
- **Refresh (`r`)**: Reload the current view/prefix.

### Implementation Steps
1. **Add Dependencies**: `go get github.com/atotto/clipboard`.
2. **Define Keybindings**: Add `Copy` and `Refresh` to `keyMap`.
3. **Handle Input Updates**:
   - Implement `y`: Construct URI and write to clipboard.
   - Implement `r`: Dispatch reload command.
4. **Documentation**: Update `docs/KEYBINDINGS.md`.
5. **Verification**: Add unit tests for URI construction and refresh command.

## Phase 3: External Integration (Open File)
**Objective**: Allow opening files in the system's default application.

### New Keybindings
- **Open (`o`)**: Open the selected file using the default system application.

### Implementation Steps
1. **Handle Input Updates**:
   - Implement `o`: Trigger background download to temp dir and launch system `open` command.
2. **Documentation**: Update `docs/KEYBINDINGS.md`.
3. **Verification**: Add integration tests for the workflow.
