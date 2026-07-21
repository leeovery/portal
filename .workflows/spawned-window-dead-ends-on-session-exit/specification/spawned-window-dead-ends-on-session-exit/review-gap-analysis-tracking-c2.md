---
status: complete
created: 2026-07-21
cycle: 2
phase: Gap Analysis
topic: spawned-window-dead-ends-on-session-exit
---

# Review Tracking: spawned-window-dead-ends-on-session-exit - Gap Analysis

## Findings

### 1. Manual-validation deliverable: commands not reproduced and their home/form unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Scope & Non-Goals ("In scope" bullet 4), Testing Requirements → Manual validation, Acceptance Criteria #8

**Details**:
Three sections make "ship the documented manual-validation commands" an in-scope, acceptance-gated deliverable:
- In scope: "Shipping the validated sandboxed manual-test commands as the documented manual validation for this fix."
- Testing Requirements: "Ship the validated sandboxed Ghostty test commands as the documented manual validation for this fix."
- Acceptance Criteria #8: "The documented sandboxed manual-validation commands reproduce the clean shell landing on a throwaway `-L` tmux socket."

All three reference "the validated sandboxed manual-test commands" as a concrete, pre-existing artifact, but the spec neither reproduces the commands nor points to where they live. Two things are left for the implementer to guess:
1. **The commands themselves.** The Manual-validation prose does describe the *steps* well (open a Ghostty window via the adapter's command shape → kill/detach the session → confirm it lands at `$SHELL` login+interactive, not the "Press any key" dead-end; note the `[detached …]` line is expected) and the Sandbox rule pins the `-L <socket>` requirement — so the commands are reconstructable. But "the validated sandboxed commands" reads as a specific artifact the standalone reader cannot access.
2. **Where/what form they ship as.** The spec says the osascript boundary "stays `//go:build manual`" (an existing test) but never says whether the documented commands belong in that manual test, a new manual test, a comment block, or a markdown doc. A planner turning "ship the documented manual-validation commands" into a task cannot pin the deliverable's location or form without deciding it themselves.

This does not affect the core code fix (fully specified) or the automated unit coverage; it only leaves the in-scope documentation deliverable (and its acceptance criterion #8) under-plannable.

**Proposed Addition**:
Resolved by removing the deliverable, not pinning it. Per user clarification, the fix mechanism was **already validated live** during the investigation (sandbox validation), so there is no outstanding manual-test artifact to ship. Reframed the Manual validation subsection to "already performed (during investigation)"; removed the In-scope "ship manual-test commands" bullet; removed AC #8 (documented commands reproduce landing) and added a note that no manual-validation deliverable is gated — only the code change + unit coverage (AC 2/3/7) remain. Sandbox rule kept, reframed as applying to any future re-validation.

**Resolution**: Adjusted
**Notes**: User clarified the functionality is already tested (investigation §Sandbox Validation). The finding correctly caught an under-specified "deliverable" — the right fix was to drop the deliverable framing, since it does not exist, rather than pin its home/form.

---
