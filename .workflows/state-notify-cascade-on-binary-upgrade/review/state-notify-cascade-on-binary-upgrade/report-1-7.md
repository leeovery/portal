TASK: Real-tmux self-heal, teardown-at-depth, and idempotency/no-churn guards (state-notify-cascade-on-binary-upgrade-1-7)

STATUS: Complete

ACCEPTANCE CRITERIA: self-heal K-deep on pane-focus-out collapses to one (body == desired) with user hook untouched; teardown-at-depth reaps both blind events to zero with user hooks intact; second registration leaves indices unchanged + no eviction INFO + no WARN; all three real-tmux + SkipIfNoTmux + no t.Parallel; pass on tmux.

SPEC CONTEXT: §§ Testing Requirements (3,4,5), Acceptance Criteria (2,4,5,7).

IMPLEMENTATION:
- Status: Implemented
- internal/tmux/hooks_register_realtmux_test.go: SelfHeals... (298-358), ReapsAtDepthOnBlindEvents... (396-476), SecondRegistrationIsChurnFree (491-546). Helpers: portalEntryCommandsForEvent (76), countPortalEntriesForEvent (96), snapshotEventIndices (552), stackDepth=5. All read via the per-event oracle (never no-arg). No t.Parallel; SkipIfNoTmux on every test.

TESTS:
- Status: Adequate
- Self-heal: seeds K=5 expectedNotifyCommand on pane-focus-out + user hook; NON-VACUOUS pre-condition asserts pre-seed count == stackDepth; after one register exactly one Portal entry (body byte-equal) + user hook count==1 (AC#2 + AC#4).
- Teardown-at-depth: both blind events K-deep + user hook, each non-vacuous; one UnregisterPortalHooks; per blind event ZERO across all four teardown fingerprints + user hook survives (AC#5 + AC#4). session-closed commit-now stack (427-440, 469-475) is the 4-1 extension (bonus, doesn't detract from 1-7 scope).
- Churn-free: converge once, snapshot indices, second register (fresh logger), re-snapshot; (a) indices byte-identical, (b) no INFO with reaped>0, (c) zero WARN (AC#7). Index-stability proof is meaningful (run 2 hits fast path on every event).
- Four-fingerprint teardown list mirrors production portalCommandSubstrings. Real tmux is oracle throughout per spec mandate.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, SkipIfNoTmux gate, DRY read primitive, fingerprint constants mirror production with "kept in sync" comments). SOLID: Good. Complexity: Low. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Self-heal exercises only pane-focus-out (window-layout-changed depth-collapse covered indirectly by 1-6 no-growth at depth-1 across N). Optional: parameterise self-heal over both blind events. Not required by AC.
- [idea] Churn-free "no eviction INFO" check fails only when a recorded INFO carries reaped>0 (not "zero INFO lines"). Faithful to spec wording (signal is absence of the reaped line); a future unrelated INFO without reaped attr would pass silently here. Mock-level NoReapedInfoOnZeroEviction asserts the stronger form.
