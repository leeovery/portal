# Pressing `m` should mark the highlighted session as the first selection

**User intent (2026-07-14):** Entering multi-select mode by pressing `m` on a highlighted session should immediately select that session as the first marked item — rather than entering the mode with nothing selected. The "enter with zero selected" behaviour was an intentional original choice that now reads as a mistake: the common case is "I'm looking at a session, I want it in the set."

**Current behaviour:** The first `m` enters mode with an empty set; a second `m` toggles the cursor row. See `handleMultiSelectToggle` (`internal/tui/model.go:3503-3512`) — the mode-entry branch sets `selectedSessions = {}` and returns without marking the cursor row.

**Proposed change:** In the mode-entry branch, mark the currently-highlighted session on entry, reusing the existing `selectedSessionItem()` lookup (which already returns `ok=false` on a non-selectable `HeaderItem` row, so entering on a group header cleanly enters with zero — no special-casing needed).

**Spec reversal (load-bearing):** This directly reverses a documented spec decision in *Multi-Select Mode → Trigger & marking*: "**`m` enters an explicit multi-select mode** … It is a real mode you can sit in with **zero selected** — not an implicit mark-on-entry." The spec (and the plan/task-5-1 "enter mode with zero selected" acceptance) must be updated in lockstep, which triggers a knowledge-base reindex.

**Edge cases to confirm before it lands:**
- Cursor on a group-header (`HeaderItem`) row → enter with zero (the `selectedSessionItem()` `ok=false` path).
- Zero-selected state stays reachable: double-`m` (enter + immediately toggle the auto-marked row off) yields an empty mode; `Esc` still clears/exits. Confirm this is the intended way to reach zero.
- Multi-tag (By-Tag Pattern B) highlighted row → marks the underlying session once (same identity rule as the toggle branch).

**Size:** Small. ~3 lines in one function; the bulk is flipping the handful of "enters with zero selected" test assertions and updating the spec/plan text (+ reindex).

Source: user request during review close-out of restore-host-terminal-windows
