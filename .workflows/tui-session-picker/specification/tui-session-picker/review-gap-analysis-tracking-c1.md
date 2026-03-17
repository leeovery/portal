---
status: complete
created: 2026-02-27
cycle: 1
phase: Gap Analysis
topic: tui-session-picker
---

# Review Tracking: TUI Session Picker - Gap Analysis

## Findings

### 1. Project edit: no "On confirm" behavior specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Projects Page

**Details**:
Kill, rename, and delete all specify what happens on confirm (e.g., "On confirm: kill session via tmux, fetch fresh session list, call `SetItems()` on the list"). Project edit doesn't specify on-confirm behavior. For consistency and implementation clarity, it should.

**Proposed Addition**:
Add to the Edit section under Projects Page:
> - On confirm: save changes to project config, refresh list

**Resolution**: Approved
**Notes**: Added on-confirm behavior for consistency with other modal actions.

---

### 2. `Esc` quit behavior in normal mode undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Sessions Page, Projects Page

**Details**:
In command-pending mode, `Esc` quits (after clearing filter if active). In normal mode, only `q` is listed as quit — the spec doesn't say whether `Esc` also quits. An implementer would need to decide: should `Esc` quit in normal mode too (consistent with command-pending), or should `Esc` only be used for filter clearing and modal dismissal in normal mode? This needs an explicit decision.

**Proposed Addition**:
New section: `Esc` Key — Progressive Back/Dismiss. Esc unwinds one layer: modal → filter → browser → exit TUI. Consistent across normal and command-pending modes.

**Resolution**: Approved
**Notes**: User clarified Esc should progressively move backwards through layers of context.
