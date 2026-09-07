# Key Shortcut Feature Documentation

## Overview
The ixi-flower-tui application now supports key shortcuts for scripts. Users can assign custom key combinations to their scripts and trigger them directly from the main interface.

## Features Added

### 1. Data Structure Updates
- Added `keyShortcut` field to both `item` and `storedItem` structs
- Updated serialization/deserialization to preserve key shortcuts

### 2. Edit Mode Enhancements
- Added a new input field for key shortcuts in edit mode
- Implemented tab navigation between all three input fields (name, path, key shortcut)
- Added validation to prevent duplicate key shortcuts
- Shows current key shortcut in the UI

### 3. Keyboard Handling
- Implemented global key shortcut detection
- When a key combination matches a script's assigned shortcut, the script executes
- Works for various key combinations (F1-F12, Ctrl+X, etc.)

### 4. UI Improvements
- Script titles now display their assigned key shortcuts in brackets
- Footer text updated to indicate key shortcut functionality
- Enhanced description display to show script flags

## How to Use

### Assigning Key Shortcuts
1. Press `3` to enter edit mode
2. Select a script to edit
3. Use Tab to navigate to the "Key Shortcut" field
4. Enter your desired key combination (e.g., "F1", "Ctrl+A", etc.)
5. Press Enter to save

### Using Key Shortcuts
- Once assigned, press the key combination directly from the main screen
- The corresponding script will execute immediately

### Duplicate Prevention
- The system validates that no two scripts have the same key shortcut
- If a duplicate is detected, an error message appears

## Technical Implementation Notes

### Key Detection
- Uses `msg.String()` from Bubble Tea to detect key presses
- Case-insensitive comparison using `strings.EqualFold`
- Only active when not in edit/add/remove/search modes

### Data Persistence
- Key shortcuts are saved to the JSON storage file along with other script properties
- Maintains backward compatibility with existing script configurations

## Example Key Combinations
- Function keys: "F1", "F2", ..., "F12"
- Control combinations: "Ctrl+A", "Ctrl+S", etc.
- Single keys: "A", "B", "1", "2", etc.
- Special keys: "Space", "Enter", etc.

Note: Be careful to avoid conflicts with existing application key bindings.