# Analysis Tasks: lazy-resume-on-attach (Cycle 8)

## Task 1: A Hydrate Tail Parks Only the Pane Its Mark Step Returned
severity: medium
sources: architecture

**Problem**: The helper resolves the pane's resume decision once, at the top of `runHydrate` (`cmd/state_hydrate.go:112`), and stores it in `hydrateConfig.Decision`, a `*resumeDecision` (`:61-64`). Waiting is made safe by a step that runs later, `markPendingThenUnsetSkeletonMarker` (`:348-356`). That step writes the pending marker, fills the decision's `Pane` and `Exe` (`markResumePending`, `:374`), sets `Wait` back to false on a refusal, and then clears the skeleton marker. It runs in three places:
- inline on the replay tail (`:177`);
- inside `handleHydrateTimeout` (`:320`);
- inside `handleHydrateFileMissing` (`:340`).

The two handlers are injectable seams that take `hydrateConfig` by value. The pointer exists only so that a change made inside a handler's copy reaches the exec running on `runHydrate`'s own copy. So `Wait` means "resolved lazy" until the mark step runs and "marked" after it. `execResumeChainAndExit` (`:262-274`) cannot tell the two apart. It parks on `Decision.Exe` and `Decision.Pane`, and only the mark step fills those.

A tail that reaches the exec without the mark step therefore parks the pane on an empty executable (`'' state resume-draw …`). That exits 127, and the backstop drops the pane to a bare shell. The user gets no panel, no hook, no pending marker and no `set resume pending marker failed` WARN. The only log record is an exec breadcrumb reading `hook_present=true`. The registration is skipped for that boot, and `portal.log` does not say why.

Every tail marks today, so this cannot happen on the current tree. But the specification's rule that "the mode is resolved and the marker written once, ahead of whichever clear runs" is held only because each tail remembers one call, and two of those calls sit inside replaceable seams. This shape has gone wrong before: the 2026-09-19 corrigendum records that both degraded tails used to restore eagerly whatever mode was resolved.

There is also a second route to the exec. `execShellOrHookAndExit`'s nil-Decision arm (`:243-244`) does its own lookup and fires a lazy registration eagerly. Production never reaches it, because `runHydrate` always sets the decision, but `TestHydrateLazy_NilDecisionFallsBackToItsOwnLookup` (`cmd/state_hydrate_lazy_test.go:454`) keeps it alive. It is also the route nine other test call sites take into the exec, since none of them sets a decision (listed under **Do**).

**Solution**: Make the marked pane a value the exec needs, rather than a field one step fills in on a shared pointer. The direction follows from the specification's rule above.
- `handleHydrateTimeout` and `handleHydrateFileMissing` keep only their tail-specific reporting. The timeout handler keeps the reset preamble, the FIFO removal, and its WARN and INFO lines. The file-missing handler keeps its three-way WARN and its INFO line. Neither calls the mark step, and both seams keep their signatures and their nil-handler behaviour.
- `runHydrate` runs mark-then-unset itself on each of its three tails, in today's order:
  - replay: settle sleep, then mark;
  - signal timeout: handler, then mark, then settle sleep, then exec;
  - file missing: handler, then mark, then exec.
- The mark step returns the parked pane (pane id and executable) as a value. It returns nil when the decision is not to wait, and nil when the mark is refused; a refusal still emits its `set resume pending marker failed` WARN.
- The exec takes the resolved registration and that value. A non-nil parked pane parks on the chain (`execHandOff`, `hook_present=true`). A nil one goes through `handOffToHookOrShell` with the registration's command, as cycle 2 settled.
- `hydrateConfig` loses its `Decision` field. `resumeDecision` keeps only what is resolved at the top (whether the pane waits, and the registration). It never carries `Exe` or `Pane`, and nothing mutates it after it is resolved.
- The nil-Decision arm of `execShellOrHookAndExit` is removed, along with the test that pins it. Tests that drive the exec directly build the decision they need.

Nothing else changes. That covers the settled hand-off helpers (`execHandOff`, `resumeHandOff`, `handOffToHookOrShell`) and the exec-seam guard's permitted set. It also covers the parked chain's trap and backstop, the order in which markers are set and cleared, and every tail's log lines, reset bytes and argv. The lazy suite runs all three tails through the production handlers (`lazyTails`, `cmd/state_hydrate_lazy_test.go:37-66`), and that suite, the hydrate suites and the exec-log suites form the regression net.

**Outcome**: A pane can park on its resume chain only through the pane id and executable that the mark step returned, after it wrote the pending marker. A tail that did not mark has no such value, so parking on an empty executable becomes impossible to write. No route to the exec ignores the resolved mode. On the tree as it stands, every tail marks, logs, clears and hands off exactly as it does today.

**Acceptance Criteria**:
- [ ] With a lazy registration, on each of the three tails (replay, signal timeout, scrollback missing), the helper writes `@portal-resume-pending` before it clears the pane's `@portal-skeleton-*` marker. It then execs `/bin/sh` on the parked chain composed from the pane id and executable the mark step resolved, byte-identical to today's argv.
- [ ] With a lazy registration and the signal-timeout or scrollback-missing handler replaced by one that only returns nil, the pane is still marked pending before the skeleton clear and still parks on the chain. Whether a pane waits no longer depends on what a handler seam does.
- [ ] Either handler run on its own does its tail's reporting and issues no tmux command, so neither marker is written or cleared. The timeout handler writes the reset preamble, removes the FIFO and logs `timeout waiting for hydrate signal` (WARN) and `signal timeout` (INFO). The file-missing handler logs its per-cause WARN and `scrollback missing` (INFO).
- [ ] With a lazy registration whose mark is refused (the marker write fails, `$TMUX_PANE` is absent, or the executable cannot be resolved), on any of the three tails the pane gets `sh -c '<command>; exec $SHELL'` and carries no pending marker. Exactly one `set resume pending marker failed` WARN names the pane and the error.
- [ ] Handed a lazy decision and no parked pane, the exec runs the registration's command through the hook chain with an exec line reading `hook_present=true`. It never execs an argv naming an empty executable, and it performs no hook lookup of its own.
- [ ] With no registration, or a registration resolving eager, every tail restores exactly as today: a bare `$SHELL`, or `sh -c '<command>; exec $SHELL'`. No pending marker is written, and the hook store and `prefs.json` are each read once per pane.
- [ ] Nothing else moves. These all stay as today: each tail's log lines and their order ahead of the exec INFO, and its reset bytes. The settle sleep on the replay and timeout tails, and its absence on the file-missing tail. A nil handler returning its error with no exec and no pending marker.

**Do**:
- The work lives in `cmd/state_hydrate.go` and the tests named below.
- `handleHydrateTimeout` (`:308`) and `handleHydrateFileMissing` (`:327`) drop their `markPendingThenUnsetSkeletonMarker` call (`:320`, `:340`) and keep everything else. `HandleTimeout` and `HandleFileMissing` keep their signatures, and a nil handler still returns its error without an exec.
- `runHydrate` runs mark-then-unset itself on every tail, in today's order:
  - replay: postamble, settle sleep, mark, `scrollback replayed` INFO, exec;
  - signal timeout: handler, mark, settle sleep, exec;
  - scrollback missing: handler, mark, exec, at both of its sites (the scrollback open failing, `:147`, and the copy failing mid-stream, `:162`).
- The mark step (`markPendingThenUnsetSkeletonMarker` / `markResumePending`, `:348-376`) returns the parked pane (pane id and executable) as a value. It returns nil when the decision is not to wait, and nil when the mark is refused, still emitting the refusal WARN. The skeleton clear follows either way.
- The exec (`execShellOrHookAndExit` / `execResumeChainAndExit`, `:239-274`) takes the resolved registration and that value. A non-nil value parks on the chain through `execHandOff` with `hook_present=true`. A nil one goes through `handOffToHookOrShell` with the registration's command. The nil-Decision arm (`:243-244`) goes.
- `hydrateConfig` loses `Decision` (`:61-64`). `resumeDecision` (`:70-75`) keeps only whether the pane waits and the registration. It never carries `Exe` or `Pane`, and nothing mutates it after `resolveResumeDecision` returns.
- Left as they are: `execHandOff`, `resumeHandOff`, `handOffToHookOrShell`, the permitted set in `cmd/exec_handoff_guard_test.go`, `parkedChainTrap`, `parkedChainBackstop`, and the order in which markers are set and cleared.
- `TestHydrateLazy_NilDecisionFallsBackToItsOwnLookup` (`cmd/state_hydrate_lazy_test.go:454-478`) is removed with the arm.
- Direct exec callers, measured with `rg -n 'execShellOrHookAndExit\(' --type go`: 15 occurrences across 5 files.
  - Production: the definition plus 4 call sites in `cmd/state_hydrate.go` (`:121`, `:151`, `:166`, `:181`).
  - Tests: 10 call sites. Seven are in `cmd/state_hydrate_exec_log_test.go` (`:41`, `:87`, `:133`, `:171`, `:213`, `:240`, `:299`). The others are `cmd/state_resume_enter_test.go:257`, `cmd/hooks_read_lock_test.go:123`, and `cmd/state_hydrate_lazy_test.go:469`, which goes with its test.
  - The nine surviving test sites all reach the exec today with no decision, through the arm being removed. Each builds the decision it needs and keeps every assertion it makes today: the `hook lookup` DEBUG and `lookup on-resume hook failed` WARN lines, the degraded-read record, and the exec line and argv.
- Direct handler callers, measured with `rg -n 'handleHydrateTimeout\(cfg\)|handleHydrateFileMissing\(cfg,' --type go`: 8, all in tests.
  - Two assert that the skeleton clear happens inside the handler. They are `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` (`cmd/state_hydrate_test.go:1264`, clear assertion at `:1291-1302`) and `TestHydrateFileMissingLog_PreservesPerCauseWARNsAndNoSettleSleep` (`cmd/state_hydrate_file_missing_log_test.go:193`, at `:231-241`). Those assertions describe the old placement and come out; the rest of each test stands.
  - The clear on those tails stays pinned through `runHydrate` by `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` (`cmd/state_hydrate_test.go:1161`), `TestHydrate_FileMissing_UnsetsSkeletonMarkerWithSetOptionSU` (`:795`) and `TestHydrateTimeoutLog_PreservesWarnUnlinkAndMarkerUnset` (`cmd/state_hydrate_timeout_log_test.go:78`).
