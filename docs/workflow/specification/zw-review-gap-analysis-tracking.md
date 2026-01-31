---
status: in-progress
created: 2026-01-31
phase: Gap Analysis
topic: ZW
---

# Review Tracking: ZW - Gap Analysis

## Findings

### 1. `zw clean` has conflicting scope

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface, Stale Project Cleanup
**Priority**: Critical

**Details**:
CLI table says: "Remove exited/dead sessions (non-interactive)"
Stale Project Cleanup section says: "detects missing or renamed directories and offers to remove them"
These are two different operations (Zellij session cleanup vs. project list cleanup). "Offers to remove" implies interactivity, contradicting "non-interactive."

**Proposed Addition**: Updated CLI table and Stale Project Cleanup section.
**Resolution**: Approved
**Notes**: Option C chosen - both session cleanup and stale project removal, with printed output, non-interactive.

---

### 2. Exec vs subprocess for Zellij handoff is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Core Model, Zellij Integration
**Priority**: Critical

**Details**:
The spec says ZW runs `zellij attach -c <session-name>` but doesn't say whether ZW should `exec` (replace its process) or spawn a subprocess. This is fundamental:
- exec: ZW ceases to exist, clean handoff, no post-attach actions
- subprocess: ZW waits, can do cleanup after, but must manage terminal state

**Proposed Addition**: New "Process Handoff" subsection in Zellij Integration.
**Resolution**: Approved
**Notes**: exec model chosen — standard pattern for session pickers, no post-attach actions needed.

---

### 3. Fuzzy filter / keyboard mode model undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Design
**Priority**: Critical

**Details**:
"Typing" activates fuzzy filter but there's no mode model:
- What gets filtered (sessions? exited? new option?)
- How filter input is displayed
- How to clear filter
- Conflict: `k` is both navigation (down) and a typeable character. `n` is both "jump to new" and a typeable character. How does the TUI distinguish?

**Proposed Addition**: New "Filter Mode" subsection in TUI Design, updated shortcuts table (replaced "Typing" with `/").
**Resolution**: Approved
**Notes**: Option A chosen — dedicated `/` key enters filter mode, Esc exits. Clear separation between shortcut and filter modes.

---

### 4. Utility mode "Enter shows info" is undefined

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Running Inside Zellij
**Priority**: Important

**Details**:
"Enter on a session shows info instead of attaching" but what info? How displayed? Tab names? A popup? Details pane?

**Proposed Addition**: Inline expansion with tab names, toggle on Enter, no popup.
**Resolution**: Approved

---

### 5. Project picker interaction model incomplete

**Source**: Specification analysis
**Category**: Insufficient Detail
**Affects**: Project Memory, TUI Design
**Priority**: Important

**Details**:
The project picker is a core screen but its full interaction model is sparse:
- What keyboard shortcuts exist for project management (rename, aliases, remove)?
- Can you fuzzy-filter the project list?
- How to navigate back to session list?

**Proposed Addition**: New "Project Picker Interaction" subsection under Project Memory. Updated empty state mock and file browser access to remove `/` shortcut conflict.
**Resolution**: Approved
**Notes**: `/` is consistently "filter mode" across all screens. File browser accessed via list item, not shortcut.

---

### 6. `zw attach` on non-existent session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Interface
**Priority**: Important

**Details**:
What happens when `zw attach <name>` is called with a name that doesn't match any session? Error message format?

**Proposed Addition**: (pending discussion)
**Resolution**: Pending
