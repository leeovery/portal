---
status: in-progress
created: 2026-02-10
phase: Gap Analysis
topic: mux
---

# Review Tracking: mux - Gap Analysis

## Findings

### 1. tmux session name character restrictions

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Session Naming
**Priority**: Important

**Details**:
tmux session names cannot contain periods (.) or colons (:). Session names are auto-generated as `{project-name}-{nanoid}`. If a project name contains restricted characters (e.g., project "my.app" → session "my.app-x7k2m9"), tmux will reject the name. The spec doesn't address sanitization of project names when used in session names.

**Proposed Addition**: Note that project names are sanitized for tmux session naming — restricted characters replaced or stripped.

**Resolution**: Pending
**Notes**:

---

### 2. tmux server not running — empty state handling

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Design > Empty States, tmux Integration > Session Discovery
**Priority**: Important

**Details**:
When no tmux server is running (no sessions exist at all), `tmux list-sessions` exits with status 1 and outputs an error. The spec shows a "No active sessions" empty state but doesn't specify how mux handles the tmux command failing vs returning an empty list. An implementer needs to know: treat server-not-running as "no sessions" and show the empty state.

**Proposed Addition**: Note in Session Discovery that a failed `list-sessions` (no server) is treated as zero sessions.

**Resolution**: Pending
**Notes**:

---

### 3. Project picker filter — what fields are matched

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Project Memory > Project Picker Interaction
**Priority**: Minor

**Details**:
The project picker has filter mode described as "same behavior as main session list." The main list filters against session names. The project picker should filter against project names and aliases, but this isn't explicit. An implementer could reasonably match only names or only aliases.

**Proposed Addition**: Specify that project picker filter matches against project name and aliases.

**Resolution**: Pending
**Notes**:
