TASK: A Hydrate Tail Parks Only the Pane Its Mark Step Returned (lazy-resume-on-attach-14-1, tick-b281c8)

ACCEPTANCE CRITERIA:
1. With a lazy registration, on each of the three tails (replay, signal timeout, scrollback missing), the helper writes `@portal-resume-pending` before it clears the pane's `@portal-skeleton-*` marker, then execs `/bin/sh` on the parked chain composed from the pane id and executable the mark step resolved, byte-identical to today's argv.
2. With a lazy registration and the signal-timeout or scrollback-missing handler replaced by one that only returns nil, the pane is still marked pending before the skeleton clear and still parks on the chain.
3. Either handler run on its own does its tail's reporting and issues no tmux command, so neither marker is written or cleared. Timeout handler: reset preamble, FIFO removal, `timeout waiting for hydrate signal` (WARN), `signal timeout` (INFO). File-missing handler: per-cause WARN and `scrollback missing` (INFO).
4. With a lazy registration whose mark is refused (marker write fails, `$TMUX_PANE` absent, executable unresolvable), on any of the three tails the pane gets `sh -c '<command>; exec $SHELL'` and carries no pending marker, with exactly one `set resume pending marker failed` WARN naming the pane and the error.
5. Handed a lazy decision and no parked pane, the exec runs the registration's command through the hook chain with `hook_present=true`, never execs an empty executable, and performs no hook lookup of its own.
6. With no registration, or one resolving eager, every tail restores exactly as today (bare `$SHELL` or `sh -c '<command>; exec $SHELL'`), no pending marker is written, and the hook store and `prefs.json` are each read once per pane.
7. Nothing else moves: each tail's log lines and their order ahead of the exec INFO, reset bytes, the settle sleep on replay and timeout and its absence on file-missing, and a nil handler returning its error with no exec and no pending marker.

STATUS: issues_found

SPEC CONTEXT: The specification's pending-marker rule requires the pending marker to be set before the mid-restore marker is cleared on every path the helper can end on, with "the mode resolved and the marker written once, ahead of whichever clear runs", so whether a pane waits never depends on whether its replay happened. A pane that cannot be marked does not wait and emits one `set resume pending marker failed` WARN; a marked pane whose chain could not be composed would be frozen for life. The 2026-09-29 corrigendum (from this implementation) records that the timeout and missing-scrollback handlers only report and touch neither marker, each of the three tails marks and clears itself, and a pane parks only on the pane id and executable the mark step returned.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_hydrate.go:62-67 — `resumeDecision` reduced to `Wait` + `Lookup`; `hydrateConfig.Decision` removed (struct at :38-60).
  - cmd/state_hydrate.go:69-74 — new `parkedPane{Pane, Exe}` value.
  - cmd/state_hydrate.go:111 — decision resolved once into a local value, passed by value thereafter.
  - cmd/state_hydrate.go:115-126 — timeout tail: handler → mark (:122) → settle sleep (:123) → exec (:124).
  - cmd/state_hydrate.go:148-165, 180-188 — both file-missing sites (open failure :151, mid-stream copy failure :162) route through `runFileMissingTail`: handler → mark → exec, no sleep.
  - cmd/state_hydrate.go:167-177 — replay: postamble → settle sleep → mark → `scrollback replayed` INFO → exec.
  - cmd/state_hydrate.go:244-251 — `execShellOrHookAndExit(cfg, registration, parked)`: non-nil parked → `execResumeChainAndExit` (`execHandOff`, `hook_present=true`); nil → `handOffToHookOrShell(registration.Command)`. The nil-Decision own-lookup arm is gone.
  - cmd/state_hydrate.go:261-273 — chain composed from `parked.Pane` / `parked.Exe`.
  - cmd/state_hydrate.go:305-334 — both handlers keep only their reporting; no mark call.
  - cmd/state_hydrate.go:341-375 — mark step returns `*parkedPane` (nil when not waiting, nil on refusal after the WARN); skeleton clear follows either way; pane and executable resolved before the marker write.
- Notes: Every criterion holds on reading. The order of set/clear, log lines, reset bytes and sleeps on each tail is unchanged from the pre-task tree (compared against the parent of the task commit). A nil or erroring handler still returns before any mark or clear. `execHandOff`, `resumeHandOff`, `handOffToHookOrShell`, `parkedChainTrap`, `parkedChainBackstop` and the exec-seam guard (cmd/exec_handoff_guard_test.go) are untouched, and `runFileMissingTail` reaches the exec seam only through `execShellOrHookAndExit`, so the guard's permitted set still holds. Only `markResumePending` builds a non-zero `parkedPane` in production, and `resumeChainExe` refuses an empty path, so there is no production route to an empty-executable park. `rg 'execShellOrHookAndExit\('` now finds the definition, 4 production call sites (cmd/state_hydrate.go:124, :176, :186 — one of which serves both file-missing sites), the test helper `execLookedUpRegistration` (cmd/state_hydrate_exec_log_test.go:33) and one direct test call (cmd/state_hydrate_lazy_test.go:533). The nine former nil-decision test sites route through `execLookedUpRegistration`, which performs the lookup explicitly, so their `hook lookup` / `lookup on-resume hook failed` / degraded-read assertions still see the same records.

TESTS:
- Status: Adequate (one gap under AC4, see FINDINGS)
- Coverage:
  - AC1: `TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker` (all three tails, ordering); parked argv via `lazyChainArgs` on replay (`TestHydrateLazy_ParksTheDrawAndTheTailInOneShell`) and on both degraded tails (`TestHydrateLazy_MarksAndParksWhateverTheHandlerSeamDoes`). The argv is composed in `runHydrate`/exec, not in the handler, so the inert-handler case observes the same composition the production handler reaches.
  - AC2: `TestHydrateLazy_MarksAndParksWhateverTheHandlerSeamDoes` (cmd/state_hydrate_lazy_test.go:454) — inert handlers, marks before clear, parks.
  - AC3: `TestHydrateHandlers_ReportWithoutTouchingEitherMarker` (:566) — zero tmux calls, exactly one WARN and INFO each. Preamble and FIFO removal on the timeout tail are observed through `runHydrate` (`TestHydrate_TimeoutWritesResetPreambleToStdout`, `TestHydrate_TimeoutRemovesFIFO`), where the handler is now the only writer/remover.
  - AC4: all three causes on replay (`TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked`, :278); marker-write failure on every tail, with the skeleton clear still issued (`TestHydrateLazy_FiresTheHookOnEveryTailWhenThePaneCannotBeMarked`, :495). One WARN is enforced by `execLogLine`'s exactly-one check.
  - AC5: `TestHydrateLazy_ExecWithNoParkedPaneRunsTheHookAndLooksNothingUp` (:520) — hook argv, `hook_present=true`, zero `hook lookup` records against a store that would have answered.
  - AC6: `TestHydrateLazy_NoRegistrationRestoresAsToday`, `TestHydrateLazy_EagerRegistrationRestoresAsToday`, `TestHydrateLazy_ReadsTheStoreOnceAndThePrefsOncePerPane` — all three tails.
  - AC7: settle sleeps (`TestHydrate_Sleeps100msBeforeUnsettingMarker`, `TestHydrate_Timeout_PreservesSettleSleepBeforeExec`, `TestHydrate_FileMissing_SkipsSettleSleep`), log order ahead of exec INFO (`TestHydrateTimeoutLog_SignalTimeoutPrecedesExecINFO`, `TestHydrateFileMissingLog_PathAttrIsFileAndPrecedesExecINFO`), skeleton clear through `runHydrate` on the degraded tails (`TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU`, `TestHydrate_FileMissing_UnsetsSkeletonMarkerWithSetOptionSU`, `TestHydrateTimeoutLog_PreservesWarnUnlinkAndMarkerUnset`), nil handler (`TestHydrateTimeoutLog_NilHandleTimeout_NoSignalTimeoutNoExec`, cmd/state_hydrate_file_missing_fallthrough_test.go).
- Notes: The two handler-level clear assertions the task named were removed as planned. The rest of each test still stands. Nothing is over-tested.

CODE QUALITY:
- Project conventions: Followed (no `t.Parallel`, seams staged through `hydrateCfg`, exec only through the hand-off helpers)
- SOLID principles: Good — handlers are single-purpose reporters; the mark step owns marking and returns what the exec needs
- Complexity: Low — the old three-arm switch on a shared pointer is now one nil check
- Modern idioms: Yes
- Readability: Good — `runFileMissingTail` removes the duplicated file-missing tail
- Issues: None in production code. Comments in the changed code hold against it.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_hydrate_lazy_test.go:289-296 — Two refusal cases in `TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked` never check that the pane carries no pending marker: "TMUX_PANE is absent" (:289) and "the executable cannot be resolved" (:293). AC4 requires that check. The assertions at :308-312 check only the hook argv and the single refusal WARN. The ordering that keeps the marker off is the one cmd/state_hydrate.go:355-357 calls load-bearing: `markResumePending` resolves the pane (:359) and the executable (:367) before `state.SetResumePendingMarker` (:371). Fix: declare `cmder := commandertest.Quiet()` and set `opts.Commander = cmder` before `refusal.prepare`. Add a per-case `wantNoWrite bool`, true for those two cases. When it is set, call `assertNoPendingMarker(t, cmder)` after `lazyRun`. The marker-write-fails case replaces the commander in its own `prepare` and attempts the write by design, so it skips the check. — FAILS: if `markResumePending` were reordered to write the marker before resolving the executable, the "executable cannot be resolved" case would still pass: the hook argv and the one WARN come out the same. The pane would then keep `@portal-resume-pending` while its hook fired, so the saver skips it permanently, which is the frozen-for-life state the mark step exists to prevent. No test in the tree would catch that.

UNSETTLED:
- None
