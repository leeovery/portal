---
status: in-progress
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
The spec names `(*tmux.Client).ListAllPanes()` as the live-pane enumeration source for the cleanup step's set difference. `ListAllPanes` swallows tmux errors and returns `([]string{}, nil)` on failure (`internal/tmux/tmux.go:551-557`). If tmux is unavailable or transiently fails when the cleanup step runs, the live-pane set is empty — so **every** `@portal-skeleton-*` marker is computed as stale and unset, including markers protecting genuinely live hydrate-in-progress panes.

The spec's Soft-Warning Posture section says "tmux unavailable → soft warning collected by the orchestrator", but with `ListAllPanes` swallowing the error there is no error to surface — only a silently empty result. An implementer must decide whether to:

- Switch the seam to `ListAllPanesWithFormat` (which propagates errors and lets the cleanup degrade to a soft warning instead of unsetting everything), or
- Add an empty-result guard in the cleanup step (skip cleanup when live-pane enumeration returns zero panes), or
- Accept the current behaviour as a known risk.

Without this decision pinned, the cleanup step has a non-trivial mass-unset hazard that could destabilise a still-live tmux server and erase legitimate skeleton markers mid-hydrate.

**Proposed Addition**:
{To be discussed — likely a clarification under Adapter Wiring specifying that the live-pane enumeration must surface tmux failures (e.g. via `ListAllPanesWithFormat`) so the cleanup can degrade to a soft warning rather than treating an empty result as authoritative truth.}

**Resolution**: Pending
**Notes**:

---

### 2. Parse step from `session:window.pane` to `SanitizePaneKey` arguments is unspecified

**Source**: Specification analysis
**Affects**: Fix Component B → Adapter Wiring (live pane enumeration paragraph)

**Category**: Gap/Ambiguity

**Details**:
The Adapter Wiring section says the cleanup step "must convert each entry to canonical paneKey form via `state.SanitizePaneKey(session, window, pane)`". `SanitizePaneKey`'s signature is `(session string, window, pane int) string` (`internal/state/panekey.go:27`). `ListAllPanes` returns `[]string` of form `session:window.pane` — so the conversion requires an intermediate parse from a single colon/dot-delimited string into `(string, int, int)`.

The spec does not describe this parse, leaving these decisions to the implementer:

- How to split on `:` when session names contain `:` (tmux allows this; the rightmost `:` separates session from `window.pane`).
- How to parse window/pane as integers and how to handle parse failures (skip the entry? treat as a non-match? warn?).
- Whether to extract a shared helper (parse + sanitize) or inline it in the cleanup step.

Either of: (a) document the parse contract briefly here, or (b) point to an existing helper that callers should use, would unblock implementation. Without it, the test-harness paneKey-normalisation test (already specified) cannot prescribe the exact normalisation pipeline, and two implementers could produce different parse behaviours.

**Proposed Addition**:
{To be discussed — likely a one-paragraph note under Adapter Wiring describing the parse step (rightmost `:` separator, `.` between window/pane, integer parse with skip-on-failure or similar) or pointing to a helper that should encapsulate the conversion.}

**Resolution**: Pending
**Notes**:

---

### 3. No acceptance criterion verifies scrollback-save resumption after marker cleanup

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria; Fix Component B → Why This Step Is Needed

**Details**:
The "Why This Step Is Needed" section identifies a second harm beyond resurrection: while a stale marker is live for a paneKey, the daemon skips scrollback save for that pane (`cmd/state_daemon.go:131-133`). The cleanup step is justified specifically by closing this gap.

However, the seven acceptance criteria do not include a test for this behaviour. AC#4 only asserts "no stale `@portal-skeleton-*` marker exists for a paneKey that has no corresponding live pane after a successful bootstrap" — it does not assert that scrollback save resumes for previously-stale-marker panes whose underlying pane is alive (the case where a marker leaked but the user re-created or kept the pane).

Without an explicit AC, the secondary harm the cleanup step is added to fix has no verification gate. An implementer could ship Fix Component B with all stated ACs green while the scrollback-save resumption remains broken in some edge of the marker-staleness space.

**Proposed Addition**:
{To be discussed — likely an additional acceptance criterion asserting that after cleanup unsets a stale marker whose pane is still live, the next daemon tick saves scrollback for that pane (i.e. the skip-save guard no longer applies).}

**Resolution**: Pending
**Notes**:

---
