---
topic: tui-session-picker
status: in-progress
type: feature
work_type: feature
date: 2026-02-27
review_cycle: 0
finding_gate_mode: gated
sources:
  - name: tui-session-picker
    status: pending
---

# Specification: TUI Session Picker

## Specification

### Architecture & Component Choices

The TUI is rebuilt as a **two-page architecture** using `charmbracelet/bubbles/list`. The two pages — Sessions and Projects — are equal peers (not parent-child). Each page is a full `bubbles/list.Model` instance with built-in filtering, pagination, help bar, status bar, custom item delegates, and keybinding management.

**Component adoption:**
- **`bubbles/list`** — adopted for both pages. Brings `help`, `key`, and `paginator` as transitive dependencies.
- **`bubbles/textinput`** — retained for rename and project edit input fields.
- **`bubbles/filepicker`** — not adopted. It shows files and directories (Portal needs directories only), lacks fuzzy filtering, and doesn't support alias saving or current-directory selection. Too many gaps.
- **Custom file browser** (`internal/ui/browser.go`) — retained as-is. Purpose-built for directory-only navigation with fuzzy filtering and alias support.

**Structural changes:**
- `ProjectPickerModel` (`internal/ui/projectpicker.go`) is **deleted** along with its associated tests. All project listing functionality moves into a `bubbles/list` page within the main TUI model.
- The `viewState` enum (`viewSessionList`, `viewProjectPicker`, `viewFileBrowser`) is replaced by a page-based model with the file browser as a sub-view.
- Hand-rolled `strings.Builder` rendering is replaced by `bubbles/list` delegates and lipgloss styling.
- Any code, tests, or message types that exist solely to support the old `ProjectPickerModel` should be removed rather than left as dead code.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
