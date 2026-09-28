TASK: 5-5 (tick-7f0ea9) — The Confirm Key Discards the Registration and Drops the Pane to a Shell

ACCEPTANCE CRITERIA:
- `y` calls `DiscardRegistration` exactly once with the waiter's `--hook-key`, and nothing is written to stdout before it returns — the confirmation is still on screen when the store answers.
- A `DiscardRegistration` error produces exactly one `resume-draw` exec carrying `--screen discard` and `--report` holding the error's text; `ClearMarker` is never called, no reset preamble is written, and no shell is exec'd.
- `y` pressed on the redrawn confirmation retries the removal — the same path runs again, so a refusal is recoverable without leaving the screen.
- A `DiscardRegistration` reporting `(false, nil)` — nothing to remove — proceeds exactly as a removal does: reset preamble, marker clear, shell.
- On the success path the order is reset preamble → `ClearMarker` → shell exec, asserted on a shared order recorder rather than on three independent call counts.
- A `ClearMarker` error produces exactly one `resume-draw` exec carrying no `--screen`, the same `--command` the waiter was launched with, and `--report` holding the error's text; no shell is exec'd and nothing else runs.
- That redraw is a fresh draw of the same screen: nothing on the path re-reads the store to decide whether to paint, so a pane whose registration is gone is never left blank and frozen.
- The shell exec'd on the success path is `resolveShell()` alone with no wrapper, so the pane closes on the first `exit`.
- The only tmux call on the whole path is the marker unset: no pane option is set, no token is read or unstamped, and no session command is issued.
- The terminal is restored before every exec on this path, including both redraws.
- An exec that returns (the exec failed) terminates non-zero through the existing `defaultExecShell` shape — one WARN, `log.Close(1)`, `osExit(1)` — rather than returning to the read loop.
- The waiting program emits no log line of its own for the removal: the breadcrumb is the store's.

STATUS: issues_found

SPEC CONTEXT: §6.2 — a confirmed discard removes the pane's registration permanently and nothing else; a discard finding nothing is still a discard; a discard that cannot be written leaves the pane waiting and reports in place, with registration and marker both standing and `y` retryable; the tmux session is untouched and no token is unstamped. §5.4 — the confirmation carries its own report row and `y` retries from it, because a confirmation that closed on a failed write would read as a cancel. §7.2 — on both answer paths the pane leaves the panel's screen before the marker clears; a clear that fails brings the waiting card back (removed command, both hints live, reason on the report row) as a fresh draw that never re-reads the store. §6.4 — the removal is logged by the store under `op=discard`, `via=panel`, carrying the removed command as `value`; the waiter adds no line of its own.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_wait.go:80-82 — `DiscardRegistration func(hookKey string) (bool, error)` seam on `resumeWaitConfig`
  - cmd/state_resume_wait.go:343-356 — `resumeAnswerDiscard`: removal first with nothing yet written; on error `resumeReport(cfg, resumeScreenDiscard, err)` (no marker touch, no preamble); else `resumeUnfreeze` (preamble → `ClearMarker`, with a refused clear redrawing `resumeScreenPanel` carrying the unchanged `Command` and the error text); else restore, re-enable tty signals, and `handOffToHookOrShell(..., "")` — the bare `resolveShell()` shape with `hook_present=false`
  - cmd/state_resume_wait.go:363-381 — shared `resumeUnfreeze` / `resumeReport` (report set to `err.Error()`, `DropInput` cleared), reused with Enter
  - cmd/state_resume_wait.go:397-400 — `resumeRedraw` restores the terminal before every redraw hand-off
  - cmd/state_resume_wait.go:415-421 — production binding `discardResumeRegistration`: `loadHookStore()` error returned as-is, then `store.Discard(hookKey, hooks.EventOnResume, hooks.ViaPanel)`
  - cmd/state_resume_wait.go:486 — wired into `stateResumeWaitCmd`
  - cmd/state_resume_chain.go:83-85 — `resumeScreenPanel` (`""`) emits no `--screen`, so the failed-clear redraw carries none
  - cmd/state_resume_draw.go:37-73 — the draw paints from the payload alone; no store read anywhere on the redraw path
- Notes: Matches the prescribed order exactly. The only tmux-reaching seam on the path is `ClearMarker` (`state.UnsetResumePendingMarker`); `DiscardRegistration` touches only the hooks store, and `EnableTTYSignals` (added to the success path by later work) is a termios call, not a tmux one. The store's `Discard` (internal/hooks/store.go:203-205, 207-252) leaves the file unwritten on every failure and emits the single `op=discard` INFO carrying `value`. Production `ExecSelf` is `defaultExecShell` (cmd/state_hydrate.go:388-393), so an exec that returns ends with WARN, `log.Close(1)`, `osExit(1)`.

TESTS:
- Status: Adequate
- Coverage: All 15 named tests are present in cmd/state_resume_discard_test.go and assert on the shared `orderedWriter` / seam recorder (`probe.order`) rather than independent counts. Removal-before-write (order[0] == "discard"); refused write → `resume-draw` argv equal to `resumeChainArgv` of the discard screen plus the report, no drop-input, order `[discard, exec]`, zero clears, empty stdout, no shell; retry from a waiter launched with a non-empty report; `(false, nil)` proceeds to preamble → clear → shell; failed clear → panel redraw with no `--screen`, the same `--command`, no store read, exec target = the portal binary, and a real `runResumeDraw` of that payload painting the command, the reason and both hints; bare-shell hand-off with `hook_present=false`; seam call counts; restore-before-exec over all three outcomes; exec failure → exactly one `osExit(1)` and one WARN; breadcrumb test over a real staged store proving the only discard record is the store's and the waiter emits nothing but `exec`. `TestDiscardResumeRegistration` pins removal, miss, and the unresolvable-store error for the production binding. The integration suite (internal/restore/lazy_resume_discard_integration_test.go) drives the wired binary end to end.
- Notes: The `via` the production binding passes is not asserted anywhere (see FINDINGS). Several tests assert the same full order sequence; each carries a distinct named subject, so this is acceptable overlap, not bloat.

CODE QUALITY:
- Project conventions: Followed — seams are function fields injected into the config struct; the exec marker is the statement right before the exec; no `t.Parallel()`; logging goes through the store's chokepoint.
- SOLID principles: Good — the unfreeze, report and redraw steps are shared with the Enter path instead of being restated.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the comments on `resumeAnswerDiscard`, `discardResumeRegistration`, `resumeUnfreeze` and `resumeReport` all hold against the code.
- Issues: None beyond the finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_discard_test.go:291 — The breadcrumb test swaps the production binding for its own closure (`store.Discard(hookKey, hooks.EventOnResume, hooks.ViaPanel)`), and `TestDiscardResumeRegistration` (cmd/state_resume_discard_test.go:313-353), which does call `discardResumeRegistration`, installs no log sink. So nothing asserts the `via` the real binding at cmd/state_resume_wait.go:420 passes. Fix: in the "it removes the registration the store holds" subtest, install `logtest.Install(t)` before calling `discardResumeRegistration`, then assert `sink.Records().Matching("hooks", "discard").Only(t, …).AttrOrEmpty("via") == hooks.ViaPanel.String()`. Alternatively, have the breadcrumb test set `cfg.DiscardRegistration = discardResumeRegistration` over `hooksFileInTempDir` and assert `via` there. — FAILS: the binding could file the panel's removal under any other `via` (for example `ViaCLI`) and every unit and integration test would still pass. `grep via=panel` over portal.log would then silently find nothing, even though §6.4 added `panel` to the closed vocabulary to make this route greppable.

UNSETTLED:
- None
