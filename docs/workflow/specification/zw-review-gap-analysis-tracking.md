---
status: in-progress
created: 2026-01-26
phase: Gap Analysis
topic: ZW
---

# Review Tracking: ZW - Gap Analysis

## Findings

### 1. Exited session action undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Design > Sections, Keyboard Shortcuts
**Priority**: Important

**Details**:
The TUI shows exited sessions with a `(resurrect)` indicator. For running sessions, `Enter` = attach. But what happens when the user presses `Enter` on an exited session? The spec doesn't define this. An implementer would have to guess: does it resurrect (reattach)? Show info? Prompt?

**Proposed Addition**:
(pending discussion)

**Resolution**: Pending
**Notes**:

---

### 2. New session creation should be blocked inside Zellij

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Running Inside Zellij > Utility Mode, CLI Interface
**Priority**: Important

**Details**:
Utility mode blocks attaching and hides `[n] new in project...` in the TUI. But `zw .`, `zw <path>`, and `zw <alias>` are CLI shortcuts that create new sessions. If run from inside a Zellij session, these would create a nested session. The spec doesn't state whether these should be blocked inside Zellij.

**Proposed Addition**:
(pending discussion)

**Resolution**: Pending
**Notes**:

---

### 3. "How Directories are Added" is incomplete

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Project Memory > How Directories are Added
**Priority**: Minor

**Details**:
The section says directories are added "via the file browser." But `zw .`, `zw <path>`, and `zw <alias>` also add directories to the remembered list (line 422: "The selected directory is added to remembered projects if not already present"). The subsection should reflect all entry points.

**Proposed Addition**:
(pending discussion)

**Resolution**: Pending
**Notes**:

---

### 4. Zero-prompt flow for saved projects with no layouts

**Source**: Specification analysis
**Category**: Ambiguity
**Affects**: Session Naming > New Session Flow
**Priority**: Minor

**Details**:
For saved projects, the New Session Flow shows a layout picker. But the Layout selection note says "If no custom layouts exist, ZW skips the layout picker." Combined: saved project + no layouts = select project → session created instantly with zero prompts. This is probably intended but worth making explicit so an implementer knows it's deliberate.

**Proposed Addition**:
(pending discussion)

**Resolution**: Pending
**Notes**:
