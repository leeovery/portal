---
status: in-progress
created: 2026-02-10
phase: Input Review
topic: mux
---

# Review Tracking: mux - Input Review

## Findings

### 1. Rename shortcut missing from TUI keyboard shortcuts

**Source**: zellij-to-tmux-migration.md (utility mode section) + cx-design.md (keyboard shortcuts)
**Category**: Enhancement to existing topic
**Affects**: TUI Design > Keyboard Shortcuts

**Details**:
The spec documents rename via `tmux rename-session -t <name> <new-name>` and states it's available in all contexts (inside and outside tmux). However, the main TUI keyboard shortcuts table has no rename key. The old Zellij spec had rename only in utility mode (which no longer exists as a concept). Since tmux rename works from outside, the main TUI should include a rename shortcut.

**Proposed Addition**: Add `R` key to keyboard shortcuts table for renaming the selected session.

**Resolution**: Pending
**Notes**:

---

### 2. `mux list` behavior inside tmux — should it exclude current session?

**Source**: Specification analysis (gap not addressed in discussions)
**Category**: Gap/Ambiguity
**Affects**: CLI Interface > Scripting & fzf Integration, Running Inside tmux

**Details**:
The TUI excludes the current session when running inside tmux. The `mux list` command outputs session names for scripting/fzf. Should `mux list` also exclude the current session when piped from inside tmux? Not addressed in any discussion. Including all sessions is likely correct for scripting (callers can filter), but it's ambiguous.

**Proposed Addition**: TBD — depends on user's preference.

**Resolution**: Pending
**Notes**:
