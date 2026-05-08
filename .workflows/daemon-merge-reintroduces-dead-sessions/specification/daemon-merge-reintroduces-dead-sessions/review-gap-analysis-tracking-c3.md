---
status: complete
created: 2026-05-08
cycle: 3
phase: Gap Analysis
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Gap Analysis

## Findings

### 1. `ListAllPanes` swallows tmux errors — cleanup can mass-unset every marker on tmux failure

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B → Adapter Wiring (live pane enumeration); Fix Component B → Soft-Warning Posture

**Details**:
`ListAllPanes` returns `([]string{}, nil)` on tmux failure (`internal/tmux/tmux.go:551-557`), which would silently treat every marker as stale and mass-unset them — destabilising a still-live tmux server.

**Resolution**: Approved
**Notes**: Approved via auto mode. Switched live-pane enumeration recommendation from `ListAllPanes` to `ListAllPanesWithFormat` (error-propagating). Soft-Warning Posture extended with explicit mass-unset hazard guard. Added belt-and-braces "if zero panes, skip cleanup" guard recommendation.

---

### 2. Parse step from `session:window.pane` to `SanitizePaneKey` arguments is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component B → Adapter Wiring (live pane enumeration paragraph)

**Details**:
`SanitizePaneKey(session, window int, pane int)` requires three typed arguments; `ListAllPanesWithFormat` returns a string `session:window.pane`. Parse contract was unstated (rightmost `:` separator, integer parse failure handling, helper extraction).

**Resolution**: Approved
**Notes**: Approved via auto mode. Added explicit "Parse contract for `session:window.pane`" subsection: split on rightmost `:`, split right half on `.`, integer-parse via `strconv.Atoi`, skip-on-failure with optional soft warning. Helper extraction left as implementation choice.

---

### 3. No acceptance criterion verifies scrollback-save resumption after marker cleanup

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria; Fix Component B → Why This Step Is Needed

**Details**:
The "Why This Step Is Needed" section justified Fix Component B by closing a scrollback-save skip; the acceptance criteria didn't verify that resolution.

**Resolution**: Approved
**Notes**: Approved via auto mode. Added acceptance criterion 8: after cleanup unsets a stale marker whose pane is still live, the next daemon tick saves scrollback for that pane (skip-save guard at `cmd/state_daemon.go:131-133` no longer applies).

---
