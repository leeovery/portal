---
status: in-progress
created: 2026-01-23
phase: Gap Analysis
topic: ZW (Zellij Workspaces)
---

# Review Tracking: ZW - Gap Analysis

## Findings

### 1. New Session Creation - Directory Behavior (Critical)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Core Model section

**Details**:
The spec says "No Directory Change Before Attach" for existing sessions, but didn't specify what happens for new sessions. When starting a new session in a project directory, the implementation needs to know whether to cd first.

**Proposed Addition**:
Added "Directory Change for New Sessions" subsection to Core Model explaining that ZW cds to the project directory before running `zellij attach -c`.

**Resolution**: Approved
**Notes**: Logged to specification.

---

### 2. Layout Discovery and Application (Critical)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Session Naming section, Zellij Integration section

**Details**:
The spec says ZW presents existing Zellij layouts but doesn't explain:
- Where ZW finds layouts (typically `~/.config/zellij/layouts/`)
- How ZW specifies a layout when creating a session
- The Zellij Integration commands table is missing the layout flag

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 3. Exited Session Discovery (Important)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Design section, Zellij Integration section

**Details**:
The spec mentions "EXITED" sessions in the TUI but doesn't explain how ZW discovers them:
- Does `zellij list-sessions` include exited sessions?
- Or does ZW scan `~/.cache/zellij/*/session_info/` directly?

Source material mentioned: `~/.cache/zellij/<version>/session_info/<session-name>/session-layout.kdl`

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 4. projects.json Structure (Important)

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Project Memory section, Configuration & Storage section

**Details**:
The spec doesn't define the JSON format for projects.json:
- Just a list of paths?
- Or objects with path, display name, last_used timestamp?

This affects sorting (by recency?), display, and whether projects have custom display names.

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 5. Session Renaming Mechanism (Important)

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Session Naming section, Zellij Integration section

**Details**:
Spec says "Session renaming is supported" but doesn't specify:
- What Zellij command renames a session?
- What's the UI flow? In-place edit? Modal prompt?

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 6. Kill Confirmation (Medium)

**Source**: Specification analysis - comparing to source discussions
**Category**: Gap/Ambiguity
**Affects**: TUI Design section

**Details**:
Original cx-design discussion mentioned "with confirmation" for killing sessions. The spec's keyboard shortcuts say K kills but doesn't mention confirmation. Should there be one?

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 7. Current Directory Quick-Start (Medium)

**Source**: cx-design.md - comparing to current TUI mockup
**Category**: Gap/Ambiguity
**Affects**: TUI Design section

**Details**:
The original cx-design TUI had `[.] current directory` as an option to quickly start a session in pwd without going through the project picker. This is missing from the new TUI mockup. Is this functionality still wanted?

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:

---

### 8. Attached Status Detection (Minor)

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: TUI Design section, Zellij Integration section

**Details**:
The spec shows "● attached" indicator but doesn't specify how ZW determines if another client is attached. Presumably from `zellij list-sessions` output format?

**Proposed Addition**:
(To be discussed)

**Resolution**: Pending
**Notes**:
