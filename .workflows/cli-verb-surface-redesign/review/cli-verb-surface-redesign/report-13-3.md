TASK: cli-verb-surface-redesign-13-3 — Fix Two Paths That Report Success While An Operation Silently Failed (type: bug, severity: medium)

ACCEPTANCE CRITERIA:
- A transient tmux probe fault in `killSaver` produces an error, not a silent "saver absent" + "removed" message.
- On trigger-connect failure, `portal.log` does not report an `opened` count that includes the un-attached trigger, or a corrective WARN is emitted noting the trigger did not attach.
- Existing uninstall and open-burst tests still pass.

STATUS: Complete

SPEC CONTEXT: This is a bug-fix task sourced from prior review reports (report-4-6, report-3-8), not a spec-governed feature. The specification does not mention these internal paths (grep for `trigger did not attach` / `saver probe` / `HasSessionProbe` in the spec returns nothing — expected). Governing context is CLAUDE.md: the discriminating `HasSessionProbe` (internal/tmux/tmux.go:165) already exists as the three-shape (present,nil)/(absent,err)/(present,err) discriminator, and the `spawn` log component carries the closed attr vocabulary (`session`/`detail` both present in it).

IMPLEMENTATION:
- Status: Implemented (both paths)
- Location:
  - Fix 1 — cmd/uninstall.go:144-171 (`killSaver`). Now probes via `c.HasSessionProbe(PortalSaverName)` and a three-arm switch: `err==nil` → present, fall through to KillSession; `!present` (genuine non-zero exit) → return nil (idempotent success, no false "removed"); default `(present, err)` (OS-layer/transient fault) → WARN + `return fmt.Errorf("saver probe: %w", err)`. RunE (cmd/uninstall.go:101-103) wraps that as `daemon kill: %w` and accumulates via errors.Join, so the completion message still prints but the command exits non-zero. The transient branch deliberately does NOT call KillSession and does NOT emit the removal INFO. Classification is correct against the probe contract.
  - Fix 2 — cmd/open_burst_run.go:229-240. The optimistic batch summary (triggerAttached=true) still emits before the connect (load-bearing: a successful outside-tmux attach exec-replaces the process and never returns). On the rare `connectTrigger` error path the process survives, so `spawn.LogTriggerConnectFailed(deps.Logger, trigger.Value, err.Error())` (internal/spawn/logemit.go:106) emits a WARN "trigger did not attach" with attrs `session` + `detail`, then propagates the error. This satisfies the "or a corrective WARN is emitted" branch of AC2 (the durable log is kept honest despite the optimistic count).
- Notes: The corrective WARN fires for BOTH attach and mint trigger-connect failures (single shared site after `connectTrigger` returns). Attr keys `session`/`detail` are within the closed spawn attr vocabulary; `log.OrDiscard` gives nil-logger tolerance.

TESTS:
- Status: Adequate
- Coverage:
  - Fix 1: `TestUninstall_TransientProbeFaultSurfacesErrorNotSilentRemoval` (cmd/uninstall_test.go:320) injects a non-*exec.ExitError probe fault (`exec: "tmux": ... not found`), asserts (a) a non-nil error, (b) error contains the `daemon kill` wrap, (c) `errors.Is(err, probeFault)` — the fault is truly folded in, (d) kill-session is NEVER invoked (t.Fatalf guard), (e) no "killed _portal-saver" removal claim in the log, (f) a WARN is logged, (g) the completion message still prints. Directly exercises every AC1 clause. Pre-existing sibling tests (`TestUninstall_IsIdempotentWhenSaverAbsent`, `...ToleratesKillSessionCantFindSessionError`, `...KillSessionOtherFailureContributesJoinedError`, `...LogsInfoWhenSaverKilledSuccessfully`) confirm the other two probe arms and the KillSession error arms remain correct — the fix did not regress them.
  - Fix 2: `TestRunOpenBurst_TriggerConnectFails_EmitsCorrectiveWarn` (cmd/open_burst_run_test.go:718) forces `recordingConnector.err`, asserts (a) the connect error propagates, (b) exactly one WARN "trigger did not attach", (c) WARN `session` attr == "trig", (d) WARN `detail` carries the connect error text, (e) the optimistic `opened` batch summary still emitted exactly once (the whole point — honest log despite the count). The pre-existing `TestRunOpenBurst_TriggerOwnConnectFails_PropagatesError` (line 690) asserts only error propagation and does not assert WARN-absence, so the added emission does not break it. `HasSessionProbe` itself is unit-covered by `TestHasSessionProbe` (internal/tmux/tmux_test.go:516).
- Notes: Not over-tested — each assertion targets a distinct AC clause. The mint-trigger analogue (`TestRunOpenBurst_TriggerMintOwnConnectFails_PropagatesError`, line 775) covers error propagation on the mint path but does not re-assert the corrective WARN; the WARN emission site is shared and fully asserted by the attach test, so coverage is adequate (not a gap).

CODE QUALITY:
- Project conventions: Followed. Error wrapping uses `%w` at both layers (`saver probe:` then `daemon kill:`), matching golang-error-handling. Logging binds the existing daemon/spawn component loggers with closed attr keys (golang-observability). No new log component invented. `log.OrDiscard` nil-tolerance is idiomatic for the seam.
- SOLID principles: Good. `killSaver` retains single responsibility; the switch is exhaustive over the probe's three shapes. `LogTriggerConnectFailed` isolates the emission in internal/spawn so open-burst and any future caller cannot drift.
- Complexity: Low. The killSaver switch is flat (3 arms); the open-burst change is a single guarded log+return on an existing error path.
- Modern idioms: Yes (`errors.As` discrimination inside HasSessionProbe, `errors.Join` accumulation, `%w` wrapping).
- Readability: Good. Both sites carry thorough, accurate godoc/inline comments that explain the load-bearing ordering (summary-before-connect) and the three probe outcomes.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_burst_run_test.go:775 — `TestRunOpenBurst_TriggerMintOwnConnectFails_PropagatesError` asserts only error propagation; optionally add a one-line assertion that the corrective WARN carries the mint trigger's `Value` (dir) as its `session` attr, since the mint path passes a directory rather than a session name. Low value (the emission site is shared and already asserted on the attach path) — include only if mint-path attr fidelity is worth pinning.
