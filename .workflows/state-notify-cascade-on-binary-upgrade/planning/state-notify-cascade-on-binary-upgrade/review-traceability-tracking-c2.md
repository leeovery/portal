---
status: complete
created: 2026-06-03
cycle: 2
phase: Traceability Review
topic: State Notify Cascade on Binary Upgrade
---

# Review Tracking: State Notify Cascade on Binary Upgrade - Traceability

## Summary

Cycle-2 bidirectional traceability analysis of the plan against its
specification, performed with fresh eyes after the cycle-1 AC 8 fix. The full
specification, the planning file, all seven tick tasks (1-1 through 1-7), the
phase record, and the cycle-1 tracking file were re-read in full. Live-codebase
verification of every task's implementation pointer was repeated.

**Result: clean. No findings.**

### Direction 1 (Spec → Plan, completeness)

Every specification element traces to at least one task with adequate
implementer-level depth:

- **Solution Strategy / read-per-event shift** → Tasks 1-1 (seam), 1-2
  (registration), 1-4 (teardown), 1-5 (no-arg deletion).
- **`ShowGlobalHooksForEvent(event)` seam** (byte-identical output, trimming
  `Run` path, `failed to show global hooks: %w` wrap) → Task 1-1, verbatim.
- **Per-event convergence algorithm, all four steps** (read, collect
  Portal-authored by union of fingerprints, idempotent fast path on
  exactly-one-equals-desired, else descending-order unset then single append)
  → Task 1-2.
- **Per-event parameter table** (six notify events → `notifyCommand`;
  `session-closed` → union `portal state notify` + `portal state commit-now` →
  `commitNowCommand`; two hydration events → `portal state signal-hydrate` →
  `--`-separated `signalHydrateCommand`) → reproduced in Task 1-2.
- **`session-closed` union fingerprint fast-path nuance** → Task 1-2 acceptance
  + NOTE.
- **Hook body shapes** (guard-prefixed `run-shell` constants, unexpanded
  `#{session_name}`, byte-for-byte fast-path equality) → Task 1-2 NOTE; the
  three constants verified unchanged in `internal/tmux/hooks_register.go`.
- **User-hook coexistence guarantee** → Tasks 1-2, 1-4, 1-7.
- **Logging / ordering / failure semantics** (single `reaped` INFO across all
  events only on eviction; absence-as-signal on fast path; per-index unset
  WARN-and-continue; per-event read failure folded into `errors.Join` with the
  canonical `show-hooks failed` WARN at `error_class=unexpected`; order no
  longer load-bearing) → Tasks 1-2 and 1-4; phase-acceptance bullets.
- **Migration-Helper Consolidation** (delete `migrateHydrationHooks` +
  `migrateSessionClosedHook`, fold into unified path) → Task 1-3.
- **One behavioral change** (substring vs exact-string match) → Task 1-3
  (explicit re-targeting of the `--debug` preservation sub-test + a new
  documenting test).
- **What is intentionally NOT consolidated** (`portal state migrate-rename`
  stays in teardown predicate only; registration/teardown predicate sets
  intentionally divergent) → Task 1-3 NOTE, Task 1-4.
- **Teardown Rewrite** (`UnregisterPortalHooks` per-event read,
  fold-and-continue, unchanged predicate/events/reverse-index removal) → Task
  1-4.
- **All eight Acceptance Criteria** → ACs 1–7 enumerated across the phase
  acceptance list and Tasks 1-2/1-4/1-6/1-7; **AC 8 ("Cascade eliminated") is
  now enumerated as an explicit acceptance bullet on Task 1-6** (the cycle-1
  finding), tied to the no-growth structural guard. Confirmed present; not
  re-raised.
- **All five Testing Requirements** → Tasks 1-6 (TR 1 no-growth, TR 2
  blind-spot) and 1-7 (TR 3 self-heal, TR 4 teardown-at-depth, TR 5
  idempotency/no-churn), all mandating `internal/tmuxtest` real-tmux fixtures.
- **Out of Scope / Non-Goals** (no change to hook behavior; managed-event set
  unchanged; migrate-rename v2 untouched; CPU/vanish leads not committed) →
  honoured; no task introduces out-of-scope work.

### Direction 2 (Plan → Spec, fidelity)

No hallucinated content. Every task's Problem / Solution / acceptance criteria /
tests / edge cases trace to a specific spec section. Implementation pointers
(file paths, line numbers, identifier and fixture names, constant bodies) were
re-verified against the live codebase this cycle and are accurate — they serve
ambiguity resolution, not invention:

- `internal/tmux/hooks_unregister.go:65` is the lone production caller of the
  no-arg `c.ShowGlobalHooks()` (Task 1-4 migrates it); `showGlobalHooksOrWarn`
  in `hooks_register.go` and the `ShowGlobalHooksOrWarn` re-export in
  `export_test.go:35` exist (Tasks 1-3/1-5 handle them); the migration-test
  readers (`hooks_migration_test.go` L107/L170/L336), the warn-test suite
  (`hooks_register_warn_test.go`), `TestShowGlobalHooks` (`hooks_test.go:11`),
  and `cmd/bootstrap/reboot_roundtrip_test.go:1285` all exist exactly where the
  tasks cite them.
- The three desired-body constants (`notifyCommand`, `commitNowCommand`,
  `signalHydrateCommand`) match the spec's hook-body shapes byte-for-byte,
  including the `command -v portal` guard and the ` -- #{session_name}` form.
- The canonical `show-hooks failed` WARN (`error`, `error_class=unexpected`)
  and the `show-hooks failed: %w` wrap shape exist in source exactly as Tasks
  1-2 and 1-4 describe.
- The `window-pane-changed` / `window-renamed` / `window-resized` events from
  the spec's blind-spot description are correctly *absent* from Portal's
  managed set; the blind-spot guard (Task 1-6) tests the two Portal-managed
  blind events (`pane-focus-out`, `window-layout-changed`) against the
  enumerated control (`session-created`) — faithful to TR 2.

## Findings

None. The plan is a faithful, complete translation of the specification. The
single cycle-1 finding (AC 8 enumeration on Task 1-6) is confirmed fixed and is
not re-raised.
