TASK: skip-bootstrap-when-warm-4-2 — Decouple daemon capture startup from best-effort hooks-cleanup store resolution

ACCEPTANCE CRITERIA:
- loadHookStore() failure no longer returns an error from daemon RunE; the daemon proceeds to its tick loop with cleanup disabled.
- Exactly one WARN is emitted on the disabled-cleanup path, using only the closed vocabulary (`error` attr under the `daemon` component) — no new attr or event invented.
- maybeRunHookCleanup is a no-op when the store is nil; capture/commit and the self-supervision probe are unaffected.
- Existing state_daemon tests (run, hook-cleanup) remain green.

STATUS: Complete

SPEC CONTEXT:
Spec §Daemon-Owned Hooks Cleanup (lines 253, 337-339) establishes that hooks stale-cleanup is re-homed onto the _portal-saver daemon as an explicitly best-effort responsibility ("never crash the daemon"). The store is built ONCE at daemon startup via loadHookStore() and carried on daemonDeps. The runtime path (maybeRunHookCleanup / runHookStaleCleanup) already logs-and-swallows every cleanup error. This task closes the posture inconsistency where the *wiring* (loadHookStore failure in RunE) was still fatal — coupling scrollback capture (the daemon's primary job) to a secondary feature's dependency resolution.

Task 3-1 reconciliation confirmed: 3-1's "loadHookStore error surfacing" criterion was deliberately superseded here. This task correctly inverts that: a startup hook-store resolution failure now degrades cleanup (WARN + nil store) instead of aborting the daemon, so the capture/commit tick loop starts regardless.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_daemon.go:715-719 — RunE call site: `hookStore, err := loadHookStore()`; on err, `logger.Warn("load hook store failed; hooks stale-cleanup disabled", "error", err)` and `hookStore = nil`, then proceeds (no error return). Preceded by an explanatory comment block (704-714) tying the change to the "never crash the daemon" posture.
  - cmd/state_daemon.go:422-433 — `maybeRunHookCleanup` guarded with `if deps.HookStore == nil { return }` as the FIRST statement, before the throttle check, so lastCleanup is never touched on the nil path.
  - cmd/state_daemon.go:415-419 — doc comment documents the nil-store semantics (task 4-2 referenced).
- Notes:
  - daemonLogger is bound `log.For("daemon")` (cmd/state_common.go:18) so the WARN is correctly under the `daemon` component.
  - loadHookStore() (cmd/hooks.go:123-130) only errors on hooksFilePath()→configFilePath() path resolution — confirmed the sole failure surface, matching the task's "latent structural" framing.
  - runHookStaleCleanup argument set on the store-present path is unchanged (lister=deps.Client, store=deps.HookStore, logger=deps.Logger, onRemoved=nil).
  - Nil-check placed BEFORE the throttle check is the correct ordering: it satisfies "no-op when store is nil ... without disturbing lastCleanup semantics" exactly.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_daemon_hook_cleanup_test.go:200-229 (TestMaybeRunHookCleanup_NilStoreNoOps) — nil store with an ELAPSED throttle (the branch that would otherwise run cleanup): asserts no panic, no `list-panes` tmux call (capture path untouched), lastCleanup unchanged (throttle not mutated), and no gate WARN. Would fail if the nil guard were removed (nil store.Load panic + list-panes call + lastCleanup advance).
  - cmd/state_daemon_test.go:1008-1044 (TestStateDaemon_HooksCleanupWiring / "it disables cleanup with a WARN rather than aborting...") — forces loadHookStore failure deterministically (PORTAL_HOOKS_FILE="" + HOME="" → configFilePath's os.UserHomeDir() errors at config.go:101, BEFORE XDG is consulted, so the failure is induced regardless of the developer's XDG_CONFIG_HOME). Asserts: RunE returns no error (reaches tick loop), deps.HookStore is nil, the disabled-cleanup WARN is present, it carries `component=daemon`, and it appears EXACTLY once. Directly covers all three behavioural criteria.
  - Regression safety: makeDeps (state_daemon_run_test.go:191-207) and hookCleanupDeps default HookStore to a non-nil empty store, so existing tick/run/hook-cleanup suites are unperturbed by the new nil branch.
- Notes: The two new tests are focused and non-overlapping (unit no-op vs startup-degradation). No redundancy, no over-mocking, no testing of implementation details. Error induction is deterministic and documented in-test. Not over-tested; not under-tested.

CODE QUALITY:
- Project conventions: Followed. WARN uses the established free-text-message + `error`-attr shape identical to sibling daemon WARNs ("acquire daemon lock failed", "ReadIndex failed", "capture pane failed", "hooks stale-cleanup failed") — no new attr key, no new closed-catalog event invented, satisfying the closed-vocabulary constraint. Component binding via log.For("daemon") is the standard pattern.
- SOLID principles: Good. The change reinforces single-responsibility separation — capture availability no longer depends on the secondary cleanup subsystem's wiring. The nil-store guard is a clean null-object-style degrade.
- Complexity: Low. One added guard clause + one changed error branch; no new branches in the hot capture path.
- Modern idioms: Yes. Idiomatic Go error handling (log-and-continue with a sentinel nil).
- Readability: Good. Both the RunE call site and the maybeRunHookCleanup guard carry comments explaining the posture rationale and cross-referencing task 4-2; intent is self-evident.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
