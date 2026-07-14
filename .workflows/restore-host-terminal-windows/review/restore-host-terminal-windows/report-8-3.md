TASK: restore-host-terminal-windows-8-3 — Run pre-flight before the unsupported gate on the picker burst path

ACCEPTANCE CRITERIA:
- On an unsupported/undriven terminal with N≥2 marked and one marked session externally killed, the picker surfaces the gone-session message (not the unsupported banner) and prunes the gone session from the selection — matching cmd/spawn.go.
- When no session is gone, the unsupported atomic no-op still fires unchanged on an unsupported terminal.
- The supported-terminal path (pre-flight then sequential spawn) is behaviourally unchanged.
- Pre-flight is evaluated before the unsupported gate on both the CLI and picker paths.

STATUS: Complete

SPEC CONTEXT:
The spec frames spawn as "one service, two callers" (CLI runSpawn + picker burst) running the identical pre-flight → sequential spawn → per-window ack → self-attach-last flow. Pre-flight is the primary Enter gate ("Before opening a single window, verify every selected session still exists"; "All-or-nothing applies at the pre-flight gate"). The architecture-sourced defect was that the picker's decideBurst short-circuited to the unsupported atomic no-op on DetectUnsupported() BEFORE any pre-flight ran, so a session killed between marking and Enter on an unsupported terminal was masked by the unsupported banner and never pruned — diverging from cmd/spawn.go (which pre-flights FIRST, cmd/spawn.go:136, ahead of the N≥2 unsupported gate at :156) and from the picker's own handlePreflightAbort prune-keeping-survivors contract.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/burst_progress.go:416-437 (decideBurst) — the fix inserts spawn.PreflightMissing(ordered, m.sessionExists) at :427-431 BEFORE emitUnsupportedNoop/setFlash (:434-435); routes a non-empty result to handlePreflightAbort (:429). handlePreflightAbort at internal/tui/burst_preflight_abort.go:38-60.
- Notes:
  - Ordering now mirrors the CLI exactly: pre-flight (:428) before the unsupported no-op (:434), matching cmd/spawn.go's :136-before-:156 sequence. Acceptance criterion 4 met.
  - Pre-flight probes `ordered` — the FULL list-ordered marked set (external + trigger) — identical to the CLI's `sessions` arg and to burstRunner.run's `all := external + trigger`. This correctly catches a gone trigger (self-attach target), not just gone externals. The synchronous probe uses the same m.sessionExists (production: client.HasSession) that burstRunner.run uses, so probe-fault-folds-to-gone semantics match the CLI.
  - Both entry points into decideBurst are covered by construction because the fix lives inside decideBurst: the direct already-resolved N≥2 Enter (handleMultiSelectEnter → beginBurst → decideBurst) and the deferred-detection resolution (terminalDetectedMsg arm → model.go:2482 → decideBurst). Acceptance criterion "ordering holds for both entry points" met.
  - Nil-guarded (if m.sessionExists != nil) to honour WithSessionExists nil-tolerance for the capture harness; falls through to the unsupported no-op when unwired. Consistent with the option's documented contract.
  - Supported path untouched: decideBurst's supported branch still returns m.dispatchBurst(ordered), which keeps its async goroutine pre-flight inside burstRunner.run. No double pre-flight (unsupported branch returns before dispatch). Acceptance criterion 3 met.
  - No drift from the tick's Do steps 1-4.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/burst_preflight_before_unsupported_test.go: TestBurstUnsupported_PreflightAbortBeforeNoop (already-resolved entry) and TestBurstUnsupported_DeferredPreflightAbortBeforeNoop (deferred entry). Both drive Apple Terminal (resolved unsupported) with bravo killed between marking and Enter; the shared assertUnsupportedPreflightAbort helper verifies the abort banner == spawn.GoneMessage(["bravo"]), NO unsupported flash (flashText==""), bravo goneFlagged + pruned, alpha survivor kept, count==1, zero adapter OpenWindow calls, not burst-pending, still multi-select, no tea.Quit. This directly verifies acceptance criterion 1 on BOTH entry points.
  - The killed session (bravo) is the list-order-last = the trigger/self-attach target, so the test also proves pre-flight over the FULL ordered set catches a gone trigger, not just externals.
  - Regression (acceptance criterion 2): pre-existing burst_unsupported_noop_test.go TestBurstUnsupported_NonNullAtomicNoOp / _NullFlash / _DeferredThenUnsupported wire allPresent, so the newly-inserted pre-flight finds nothing gone and falls through to the unchanged atomic no-op (asserts intact selection, no prune, named/NULL flash) — covering "no session gone → no-op unchanged" on both entry points.
  - Regression (acceptance criterion 3): burst_dispatch_test.go supported-terminal (ResolutionNative, allPresent) dispatch tests exercise the untouched supported path.
- Notes:
  - Not under-tested: both entry points, gone and not-gone cases, and supported regression all covered. A test failure would occur if the ordering regressed (the abort assertions would flip to the no-op flash).
  - Not over-tested: the two new tests share one assert helper and differ only by entry point — the minimal delta that pins the deferred vs direct distinction. Only trigger-gone is tested (not external-gone), which is adequate since spawn.PreflightMissing treats every element identically; a second external-gone test would be redundant.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md and the file's own header note). White-box package tui tests via existing seams. Reuses shared spawn.PreflightMissing / spawn.GoneMessage / handlePreflightAbort — no duplicated copy or logic. DI seam (m.sessionExists) used, not a fresh DefaultClient.
- SOLID principles: Good. Single shared pre-flight primitive; the fix is additive and open/closed (unsupported branch extended, supported branch untouched).
- Complexity: Low. One guarded branch inserted ahead of an existing branch; both entry points converge on decideBurst so the fix lands once.
- Modern idioms: Yes.
- Readability: Good. The §8-3 rationale is documented inline at both the doc comment (:397-400) and the branch (:417-426), and cross-references cmd/spawn.go's ordering.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_progress.go:428 — The unsupported-branch pre-flight runs N sequential m.sessionExists (tmux has-session) probes synchronously on the Bubble Tea Update thread, whereas the supported path defers pre-flight into a goroutine (burstRunner.run). The branch is a rare, deliberate, one-shot no-op and N is the small marked set, and it matches the CLI's fully-synchronous behaviour, so this is acceptable as-is — but the codebase already tracks a sensitivity to N-sequential-tmux-reads-on-the-UI-thread (project_grouped_switch_perf_followup). If ever a concern, batch the probes into a single list-sessions read or defer them; decide whether it is worth the added machinery for a no-op path.
