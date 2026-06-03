---
status: complete
created: 2026-06-03
cycle: 2
phase: Plan Integrity Review
topic: State Notify Cascade on Binary Upgrade
---

# Review Tracking: State Notify Cascade on Binary Upgrade - Integrity

## Summary

Cycle-2 standalone integrity review of the plan (one phase, seven tasks) for
structural quality and implementation readiness. Fresh-eyes pass over the
corrected plan; the single cycle-1 finding (Task 1-5's grep acceptance
criterion) is verified fixed and not re-raised.

**Result: clean.** No Critical, Important, or Minor findings.

### What I checked

Read the plan end-to-end plus all seven tick task bodies (1-1 tick-98fc91
through 1-7 tick-6a7cec), the phase (tick-0fb6d5), and the cycle-1 tracking
file. Every load-bearing codebase reference in the tasks was verified against
the live tree:

- **Production callers of the defective seam are exactly the two the tasks
  claim.** `grep '\.ShowGlobalHooks()'` (non-test) returns precisely
  `internal/tmux/hooks_register.go:127` (via `showGlobalHooksOrWarn`) and
  `internal/tmux/hooks_unregister.go:65` — the targets of Tasks 1-2/1-3 and
  1-4. Test readers cited by Task 1-5 (`hooks_migration_test.go:107,170,336`
  and `cmd/bootstrap/reboot_roundtrip_test.go:1285`) all exist at the stated
  lines.
- **The method, re-export, and primitives exist as described.**
  `ShowGlobalHooks` at `tmux.go:769`; `ShowGlobalHooksOrWarn` re-export at
  `export_test.go:35`; `AppendGlobalHook` (`tmux.go:780`),
  `UnsetGlobalHookAt` (`tmux.go:950`), `ParseShowHooks`
  (`hooks_parse.go:42`), `HookEntry` (`hooks_parse.go:15`) all present with
  the assumed shapes.
- **The migration helpers and their support symbols slated for deletion in
  1-3 all exist:** `migrateHydrationHooks`, `migrateSessionClosedHook`,
  `isStaleSignalHydrateEntry`, `staleSignalHydratePrefix/Marker`,
  `RegisterHookIfAbsent`, `hookCategory`, `portalHookCategories`,
  `notifySubstring`, `signalHydrateSubstring`, `sessionClosedEvent`.
- **Teardown invariants 1-4 relies on are real:** `portalCommandSubstrings`
  (incl. the `portal state migrate-rename` legacy substring), `portalEvents`,
  `portalEntriesFor`, `containsAny` are all present and unchanged-as-assumed.
- **Test fixtures named for migration exist:** `dispatchUnregisterHooks`
  (currently returns the whole table for every `show-hooks -g`, exactly as
  1-4 says it must be made per-event-filtered), `dispatchPortalHooks` /
  `dispatchShowHooks`, the `recordingLogger` / `recordingMigrationLogger`
  capture seams, `TestRegisterPortalHooks_SessionClosedMigration` with its
  `--debug` user-hook sub-test (`hooks_register_test.go:922`), and the three
  `TestShowGlobalHooksOrWarn_*` warn tests.
- **Integration-test API is accurate:** `tmuxtest.New(t, prefix) *Socket`,
  `SkipIfNoTmux(t)`, `Socket.Run(t, args...)`, `Socket.Client() *tmux.Client`
  all exist with the signatures Tasks 1-6/1-7 invoke.
- **Spec ↔ plan parameter table matches byte-for-byte.** The spec's per-event
  fingerprint/body table (specification §§ Per-event parameters, lines 73-75)
  is carried into Task 1-2's managed-event table exactly: six notify events →
  `portal state notify` → `notifyCommand`; `session-closed` → union of
  `portal state notify` + `portal state commit-now` → `commitNowCommand`; two
  hydration events → `portal state signal-hydrate` → `signalHydrateCommand`.

### Criterion-by-criterion

- **Task Template Compliance** — every task carries Problem, Solution,
  Outcome (folded into opening + acceptance), Do, Acceptance Criteria, Tests,
  Edge Cases, and a Spec Reference. Acceptance criteria are concrete and
  pass/fail; tests enumerate edge cases, not just happy paths.
- **Vertical Slicing / Scope** — seam-first (1-1) → convergence engine (1-2)
  → helper deletion (1-3) → teardown migration (1-4) → method deletion (1-5)
  → two real-tmux guard tasks (1-6, 1-7). 1-1 and 1-5 are the seam/delete
  endpoints of a refactor rather than standalone feature slices, but each is
  independently verifiable (1-1 by its own unit test, 1-5 by full-suite
  green) — the correct decomposition for this architectural shift, as already
  reasoned in cycle 1. 1-2 is the largest task but is one cohesive behaviour
  (the convergence engine) inside a single file boundary; splitting it would
  manufacture a non-independently-testable horizontal slice. Not flagged.
- **Phase Structure** — single phase with a well-argued "Why this order"
  (single-root-cause bug, shared seam couples register + teardown, no-arg
  read deletable only after both migrate). Phase acceptance is comprehensive.
- **Dependencies and Ordering** — verified in tick: 1-5 ← {1-2,1-3,1-4}
  (three migrations must precede deletion of the method), 1-6 ← {1-2}
  (no-growth needs the rebuilt register), 1-7 ← {1-2,1-4} (self-heal needs
  register, teardown-at-depth needs unregister). Convergence points carry
  explicit edges. The capability needs 1-2 ← 1-1 and 1-4 ← 1-1 (both call the
  new seam) are satisfied by natural creation order, so per the review
  criteria no explicit edge is required. No circular dependencies; no missing
  cross-phase edges (single phase). All priorities default medium — graph
  position is encoded by the blocked_by edges, which is sufficient.
- **Task Self-Containment** — each task carries the context, file/line
  anchors, and algorithm detail needed to execute without reading siblings;
  cross-task contracts (e.g. "1-2 may inline or reuse `showGlobalHooksOrWarn`;
  1-3 handles whichever it chose") are stated explicitly with a decision rule,
  not left implicit.
- **Acceptance Criteria Quality** — pass/fail and behaviour-covering. The
  session-closed union-fingerprint fast-path edge (notify+commit-now → count 1
  → body must equal commitNowCommand) is correctly specified: a lone stale
  `notifyCommand` gives union count 1 but body ≠ desired, so it converges
  rather than short-circuiting. The cycle-1 grep contradiction is fixed (the
  bare-form word-boundary grep now correctly expects an empty result, with a
  separate grep confirming the per-event seam survives).
- **External Dependencies** — N/A (bugfix work type).

### Edge of scope, deliberately not flagged

- The spec notes the tmux 3.6b blind class is broader than Portal's managed
  set (`pane-*` plus `window-pane-changed` / `window-renamed` /
  `window-resized` alongside `window-layout-changed`). The plan correctly
  scopes its managed-event table to only the events Portal actually
  registers (`pane-focus-out`, `window-layout-changed` from that class) and
  does not over-reach into events Portal does not own. This is correct, not a
  gap.

## Findings

None. The plan meets structural quality and implementation-readiness
standards.
