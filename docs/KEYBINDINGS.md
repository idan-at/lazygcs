# lazygcs Keybindings

This document provides a comprehensive list of all keybindings available in `lazygcs`.

## Navigation
*   `j` or `↓`: Move cursor down
*   `k` or `↑`: Move cursor up
*   `g` or `Home`: Go to the top of the list
*   `G` or `End`: Go to the bottom of the list
*   `Ctrl+f` or `PageDown`: Move cursor one page down
*   `Ctrl+b` or `PageUp`: Move cursor one page up
*   `Ctrl+d`: Move cursor half-page down
*   `Ctrl+u`: Move cursor half-page up
*   `l` or `Enter` or `→`: Enter a bucket, directory, or expand/collapse a project header.
*   `h` or `←`: Go back to the parent directory, bucket list, or collapse a project header.
*   `H`: Jump all the way back to the bucket list.

## Actions
*   `space`: Toggle selection of the highlighted item (Multi-select)
*   `y`: Copy the `gs://` URI of the highlighted item (or all selected items) to the system clipboard.
*   `r`: Refresh the current view/prefix.
*   `d`: Download the currently highlighted item (or all selected items). Directories are downloaded as `.zip` files.
*   `x`: Delete the highlighted item (or all selected items). *Note: Requires confirmation.*
*   `v`: Toggle Object Versions view in the Preview Pane.

## Filtering & Search
*   `/`: Start filtering the current column (buckets or objects).
*   `Esc`: Clear the active filter, or close any open dialogs. Filters are specific to their column and persist while you navigate.
*   `Enter`: Accept the current filter and exit search mode.

## System
*   `?`: Toggle the Help Menu overlay.
*   `q` or `Ctrl+c`: Quit the application.
