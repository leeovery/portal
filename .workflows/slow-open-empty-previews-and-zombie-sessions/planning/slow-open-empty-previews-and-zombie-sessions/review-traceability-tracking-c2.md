---
status: in-progress
created: 2026-05-22
cycle: 2
phase: Traceability Review
topic: Slow Open Empty Previews And Zombie Sessions
---

# Review Tracking: Slow Open Empty Previews And Zombie Sessions - Traceability

## Summary

Cycle 2 follow-up review after cycle 1 integrity findings (7 items) were applied. Verified both directions:

**Direction 1 (Spec → Plan)**: every specification element has plan coverage.
- Shared Primitive — `state.IdentifyDaemon`: Phase 1 Task 1-1
- Component A — Kill-Barrier Escalation: Phase 4 Tasks 4-1, 4-2
- Component B — Bootstrap-Time Orphan Sweep: Phase 4 Tasks 4-3, 4-4, 4-5
- Component C — `daemon.lock` stabilisation: Phase 4 Tasks 4-6, 4-7, 4-8, 4-9, 4-10
- Component D — Daemon Self-Supervision: Phase 5 Tasks 5-1 through 5-9
- Component E — `CaptureStructure` per-session log-and-continue: Phase 2 Tasks 2-1 through 2-5
- Component F — Saver creation ordering: Phase 3 Tasks 3-1 through 3-6
- Component G — Test Isolation Contract: Phase 1 Tasks 1-2, 1-3, 1-4, 1-5
- Composite End-to-End Verification: Phase 6 Tasks 6-1 through 6-8
- End-State Verification observables: covered in Phase 6 (pgrep convergence, scrollback stability, pre-check live, F observables) and Phase 3 (no "no such session" log entries)
- Transitional Recovery: spec-marked as "not part of the shipped fix" — correctly out of scope
- Risk Summary mitigation (D empirical measurement): Phase 5 Task 5-1

**Direction 2 (Plan → Spec)**: every plan element traces back to the spec.
- All task Spec References cite concrete spec sections. Spot-checked: Tasks 1-1, 4-1, 4-3, 4-6, 5-3, 6-3, 6-6, 6-8 — each cites a specific spec section that justifies the task content.
- Cycle 1 integrity fixes (applied) were re-validated:
  - **Task 5-3** (applied): pseudo-code and ordering rationale match spec § Component D self-check sequence steps 1–5. ✓
  - **Task 6-6** (applied): the planning-time resolution explicitly acknowledges that the composite live-context cannot exercise D's `os.Exit(0)` path against the legitimate saver-pane daemon, and reframes the assertion to observable end-state (daemon dead, scrollback bytes-identical via SIGKILL bypassing defers). The proposed text documents this divergence inline in the Do section and references Phase 5 Tasks 5-5/5-6 for the in-isolation D path. This is a deliberate planning-time scoping decision documented in cycle 1 integrity Notes; it is not a new traceability gap introduced by the cycle 1 fix.
  - **Task 5-1** (applied): the floor-of-3 clarification correctly distinguishes spec's "starting estimate" (Component D Hysteresis rationale) from the hard minimum (N >= 1 from Task 5-9). Spec-grounded.
  - **Task 4-4** (applied): the new nil-handling acceptance criterion is an implementation-consistency check (mirror prevailing convention), not a spec assertion. Does not introduce hallucinated content.
  - **Task 6-5** (applied): the EWOULDBLOCK fallback edge case correctly acknowledges that both pre-check and EWOULDBLOCK paths return `ErrDaemonLockHeld` and satisfy spec § Composite step 7's primary assertion. The "pre-check path was exercised" sub-assertion is downgraded to best-effort — a minor scoping clarification, not a divergence from the spec's actual requirement (which is the `ErrDaemonLockHeld` return).
  - **Task 2-2** (applied): refactor-cycle annotation is task-design metadata; does not affect spec coverage.

## Findings

No new traceability gaps introduced by the cycle 1 integrity fixes. All applied content remains traceable to the specification, and no spec element identified in cycle 1 as covered has lost coverage.

A minor observation worth recording for completeness (not raised as a finding because it predates cycle 1 fixes and was explicitly resolved by the integrity review's intentional planning-time decision):

- **Task 6-6 scoping divergence from spec § Composite step 8 literal wording.** Spec step 8 reads "the daemon self-ejects within (N+1) tick intervals (Component D in the live context)". The applied Task 6-6 text explicitly acknowledges this is structurally infeasible against the legitimate saver-pane daemon and rescopes to "observable end-state (daemon dead, scrollback bytes-identical) which is the user-visible consequence under either eject path." This is a documented planning-time deviation from spec wording, resolved deliberately by cycle 1 integrity review. The decision is sound (the spec's literal assertion cannot be realised in the composite context without staging that contradicts the "live state" framing) but reviewers should be aware the composite test no longer literally exercises D's `os.Exit(0)` eject — that path is verified in isolation by Phase 5 Tasks 5-5/5-6. Not flagged as a traceability finding because the cycle 1 integrity fix made this divergence explicit and intentional; flagging it again would re-litigate a resolved decision.

## Conclusion

Plan remains a faithful, complete translation of the specification. Cycle 1 integrity fixes did not introduce new traceability gaps.
