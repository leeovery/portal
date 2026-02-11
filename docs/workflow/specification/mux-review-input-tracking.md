---
status: in-progress
created: 2026-02-11
phase: Input Review
topic: mux
---

# Review Tracking: mux - Input Review

## Findings

### 1. File browser type-to-filter missing from spec

**Source**: cx-design.md — Directory Discovery section; zellij-multi-directory.md — "Unchanged from original design"
**Category**: Enhancement to existing topic
**Affects**: File Browser > Behavior

**Details**:
The cx-design discussion specified "Typing filters directories at current level" as part of the file browser's interaction model. The zellij-multi-directory discussion explicitly confirmed the file browser was "unchanged from original design." However, the spec's File Browser section lists navigation controls (arrow keys, Enter, Backspace, Esc, Space) but does not include type-to-filter functionality.

**Proposed Addition**:
(Pending discussion)

**Resolution**: Pending
**Notes**:

---

### 2. `mux list` behavior when no sessions exist

**Source**: Specification analysis (potential gap not addressed in sources)
**Category**: Gap/Ambiguity
**Affects**: CLI Interface > Scripting & fzf Integration

**Details**:
The spec covers the TUI empty state ("No active sessions") and notes that `tmux list-sessions` returns non-zero when no server is running. But `mux list` behavior in this case is unspecified — does it output empty stdout with exit code 0? Print a message? Exit non-zero? This matters for scripting and fzf integration (e.g., `mux attach $(mux list | fzf)` when there are no sessions).

**Proposed Addition**:
(Pending discussion)

**Resolution**: Pending
**Notes**:
