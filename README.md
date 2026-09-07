# IXI Flower TUI

A Go-based Terminal User Interface application featuring a split-pane layout with script management capabilities.

## Requirements
- Go 1.18+

## How to Run

1.  Navigate to the directory:
    ```bash
    cd ixi-flower-tui
    ```

2.  Run the application:
    ```bash
    go run main.go
    ```

    Or build and run the binary:
    ```bash
    go build -o app
    ./app
    ```

## Controls

-   **Left Menu:** Press `1`, `2`, `3`, or `4` to trigger menu actions.
    -   `1` **Add Scripts:** Shows two inputs (**Name** and **Path**). Use `Tab` to switch fields, `Enter` to save, `Esc` to cancel.
    -   `2` **Remove Scripts:** Select a script in the list and press `Enter` to remove it.
    -   `3` **Edit Scripts:** Select a script to edit its name, path, sudo/proxy settings, and assign key shortcuts.
    -   `4` **Projects:** Opens the projects section for quick access to your projects.
-   **Edit Mode Navigation:** While in edit mode, use these shortcuts to navigate between fields:
    - `n` or `N` to focus on **Name** field
    - `y` or `Y` to focus on **Path** field
    - `k` or `K` to focus on **Key Shortcut** field
    - `Tab` to cycle through all fields
-   **Right List:** Use `Up` / `Down` arrows to navigate the script list.
    - `Enter` run selected script in current terminal.
    - `Shift+Enter` run selected script in a new terminal window (may not work in all terminals).
    - `o` (or `Ctrl+Enter`) run selected script in a new terminal window (reliable fallback).
-   **Key Shortcuts:** Assign custom key combinations to scripts in edit mode. Press the assigned key combination from the main screen to run the script directly.
-   **Projects:** Press `p` or `P` to open the projects section, or select `4` from the left menu.
    - `a` to add a new project
    - `d` to delete selected project
    - `x` to auto-discover projects in `/home/ixi_flower/Documents` (scans for folders with .git, go.mod, package.json, etc.)
    - `n` to open in nvim
    - `t` to open in terminal
    - `f` to open in file manager
    - `c` to open in VS Code
    - `s` to search projects
    - `Esc` to close projects view
-   **Search:** Search box is visible by default. Press `s` to focus it, type to filter, press `Esc` to clear.
-   **Toggle Menu:** Press `m` to toggle the left menu visibility. Press `Shift+M` (capital M) to hide the menu and center the scripts.
-   **Hints Bar:** Press `k` to hide/show the bottom hints.
-   **Notes:** Press `n` to show/hide the notes panel. When notes are open:
    -   `Ctrl+S` to save current notes (an unsaved indicator "● UNSAVED" appears in the header when changes are made)
    -   `Ctrl+F` to toggle fullscreen
    -   `Ctrl+L` to show list of note categories
    -   `Ctrl+G` to create a new note category
    -   `Ctrl+N` to navigate to next category
    -   `Ctrl+P` to navigate to previous category
    -   `Ctrl+E` to open category menu with options:
        -   **Rename:** Edit category name and color
        -   **Remove:** Delete the current category
        -   **Save to Folder:** Export all categories to a folder
        -   Use `↑`/`↓` or `j`/`k` to navigate menu
        -   `Enter` to select, `Esc` to cancel
-   **Typing game:** Press `t` to launch `smassh` (exit `smassh` to return).
-   **Quit:** Press `q` or `Ctrl+C`.
