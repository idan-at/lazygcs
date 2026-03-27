# lazygcs Keybindings

This document provides a comprehensive list of all keybindings available in `lazygcs`.

## Navigation
*   `j` or `↓`: Move cursor down
*   `k` or `↑`: Move cursor up
*   `g`: Go to the top of the list
*   `G`: Go to the bottom of the list
*   `Ctrl+f`: Move cursor one page down
*   `Ctrl+b`: Move cursor one page up
*   `Ctrl+d`: Move cursor half-page down
*   `Ctrl+u`: Move cursor half-page up
*   `l` or `Enter` or `→`: Enter a bucket, directory, or expand/collapse a project header.
*   `h` or `←`: Go back to the parent directory, bucket list, or collapse a project header.
*   `H`: Jump all the way back to the bucket list (home).

## Actions
*   `space`: Toggle selection of the highlighted item (Multi-select)
*   `y`: Copy the `gs://` URI of the highlighted item (or all selected items) to the system clipboard.
*   `R`: Refresh the current view/prefix.
*   `o`: Open the highlighted file using the system's default application. *Note: Does not support multi-selection.*
*   `e`: Edit the highlighted file using `$EDITOR`. Changes are re-uploaded automatically. *Note: Does not support multi-selection.*
*   `n`: Create a new item. If in the bucket list, it creates a new bucket. In the object list, it creates a new file (or a directory if the name ends with `/`).
*   `d`: Download the currently highlighted item (or all selected items). Directories are downloaded as `.zip` files.
*   `i`: Toggle Metadata view in the Preview Pane.
*   `v`: Toggle Object Versions view in the Preview Pane.

## Filtering & Search
*   `/`: Start filtering the current column (buckets or objects).
*   `Esc`: Clear the active filter, or close any open dialogs. Filters are specific to their column and persist while you navigate.
*   `Enter`: Accept the current filter and exit search mode.

## System
*   `m`: Toggle the Messages/Log view overlay.
*   `?`: Toggle the Help Menu overlay.
*   `q` or `Ctrl+c`: Quit the application.
