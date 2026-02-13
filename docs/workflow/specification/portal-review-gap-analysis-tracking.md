---
status: in-progress
created: 2026-02-13
phase: Gap Analysis
topic: Portal
---

# Review Tracking: Portal - Gap Analysis

## Findings

### 1. Command exec behavior when attaching to existing session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface → x — The Launcher, tmux Integration → Command Execution in Sessions

**Details**:
The spec says command execution applies "when launching a new session." But when `x -e claude` opens the TUI, the user could select an existing session to attach to (not create a new one). The spec doesn't define what happens to the exec command in this case. An implementer must decide: silently ignore it? Show a warning? Prevent attaching when a command is pending?

**Proposed Addition**: (pending discussion)

**Resolution**: Pending
**Notes**:

---

### 2. Shell completions for custom function names

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface → Shell Functions

**Details**:
Cobra tab completions are registered for the `portal` binary name. The shell functions `x` and `xctl` (or custom names via `--cmd`) are not the binary — they're shell functions wrapping `portal`. Tab completion typically won't work for shell functions unless the init script explicitly wires it up (e.g., `compdef xctl=portal` for zsh). The spec should clarify whether completions are expected to work for the function names, or only for `portal` directly.

**Proposed Addition**: (pending discussion)

**Resolution**: Pending
**Notes**:
