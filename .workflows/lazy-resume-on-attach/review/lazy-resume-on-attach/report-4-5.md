TASK: lazy-resume-on-attach-4-5 (tick-71b703) — The Helper Decides, Marks and Hands the Pane to the Panel

ACCEPTANCE CRITERIA:
- A pane with no registration, and one whose mode resolves eager, produce byte-identical behaviour to today on all three tails: no `set-option -p`, no pending marker, the same skeleton-marker clear, the same `exec` argv and the same log records — covering both routes to eager (pinned `eager` under the shipped lazy install; no mode under an `eager` install).
- A registration pinned `lazy` under an install whose `resume_mode` is `eager` still waits.
- `execShellOrHookAndExit` called with a nil `Decision` is byte-identical to today; the eight existing direct callers pass with no edit.
- `TestNoParkedShWrapperPostRestore`, `TestExitClosesRestoredPane_NoHook` and `TestExitClosesRestoredPane_WithHook` pass unmodified.
- On each of the three tails, a lazy registration produces the pending `set-option -p` before the skeleton marker's unset, asserted on a shared call-order recorder.
- A lazy registration execs `/bin/sh -c "<draw argv>; <recover argv>"`, `;` not `&&`, every interpolated value single-quoted.
- The recover half carries `--pane` and `--pane-key` alone.
- A failed `SetResumePendingMarker`, an absent `$TMUX_PANE` and an unresolvable executable each fire the hook as eager does and emit exactly one WARN carrying `pane_key` and `error`.
- A failed prefs read resolves the install default to lazy and the pane waits.
- A hook store that could not be built resolves to no registration; a prefs store that could not be resolved takes the shipped default; neither ends the helper.
- An unreadable store is today's bare shell and never a wait.
- Exactly one hook-store lookup and one prefs load per helper run, on all three tails.
- The chain is composed from the executable path and pane id the mark step resolved, each read exactly once.
- A registration whose stored command is empty is not a registration.
- Scrollback replay, FIFO handling, settle sleep, reset preamble/postamble and the three tail records are unchanged.
- No bootstrap step, step ordering, eager signal pass or global hook is touched.
- No README sentence still states a registered command re-executes by itself after a reboot.

STATUS: issues_found

SPEC CONTEXT: §7.3 / §8.2 and the 2026-09-19 corrigendum: the helper resolves the pane's mode once, ahead of whichever mid-restore clear runs (replay, signal timeout, missing scrollback file), so whether a pane waits never depends on whether replay happened; only a pane that is going to wait is marked, and the pending marker lands before the skeleton marker is cleared. A pane that cannot be marked does not wait — it fires as eager and records one WARN naming the pane and the error (§8.2). §3.1: an unreadable `prefs.json` resolves to the shipped lazy default. §9: nothing in the restore pipeline changes except the helper's own tail. §2 / §2.1: a registration's own mode beats the install's in both directions.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_hydrate.go:61-75 — `Decision *resumeDecision` on `hydrateConfig`; `resumeDecision{Wait, Lookup, Exe, Pane}`
  - cmd/state_hydrate.go:51-54 — nil-tolerant `LoadPrefsStore` (and `ResolveExe`) seams
  - cmd/state_hydrate.go:112 — decision resolved once at the top of `runHydrate`, before the FIFO open
  - cmd/state_hydrate.go:196-235 — `resolveResumeDecision` / `installResumeMode` / `installResumeModeOf` / `lookupOnResumeOrLog` (absent-store guard kept; records emitted through the shared `resumeRegistrationOrLog`, cmd/state_resume_chain.go:152)
  - cmd/state_hydrate.go:239-252 — `execShellOrHookAndExit` branching on a nil-tolerant `Decision`
  - cmd/state_hydrate.go:262-304 — `execResumeChainAndExit` / `parkedResumeChain` composed from `Decision.Exe` / `Decision.Pane` through `shellquote.Join`
  - cmd/state_hydrate.go:177, :320, :340 — `markPendingThenUnsetSkeletonMarker` at all three clear sites
  - cmd/state_hydrate.go:348-376 — mark step: `$TMUX_PANE` and the executable resolved before the write, recorded on the decision, one WARN + downgrade on any refusal
  - README.md:214, :218, :232-237, :399-403, :408-410 and the Features bullet at :86 — rewritten to describe the panel under the shipped default and name `eager` as the fire-on-restore mode
  - CLAUDE.md "Resume hooks" section — describes the eager/lazy decision, the ordering and the refusal WARN
- Notes:
  - The chain has moved past the task's literal shape through later tasks (the `trap : INT QUIT;` prefix, the could-not-start backstop suffix, `DisableTTYSignals`, the `ResolveExe` seam, `shellquote.Join` in place of `shellWords`). Each is a sound extension; the draw/`;`/recover core the criteria describe is intact.
  - The prefs load is skipped entirely when no registration was found (`lookup.Found && …` short-circuits at cmd/state_hydrate.go:198). That is fewer reads than the criterion's "one prefs load", on the one path where the value cannot matter. Sound.
  - The integration fixture now pins `Resume: resumemode.Eager` (internal/restore/exit_closes_pane_integration_test.go:139). Under the shipped lazy default an unpinned `WithHook` registration is meant to wait, so "unmodified" could not hold together with the spec's default. Pinning eager keeps the test's subject (the hook fires, then the first `exit` closes the pane). This divergence is correct, not a loss.
  - The `hook lookup` DEBUG / lookup-failure WARN now come before the FIFO wait rather than just before the exec. The records are the same; only their position moved, which is what resolving at the top requires.
  - No production bootstrap, restore-orchestration or global-hook file is in the change-set. Only `cmd/bootstrap` test files appear there.

TESTS:
- Status: Adequate
- Coverage: cmd/state_hydrate_lazy_test.go covers the criteria.
  - No registration: all three tails (:122).
  - Both routes to eager: all three tails (:141).
  - Pinned lazy beats an eager install (:170).
  - Pending-before-clear on the shared commander call log: all three tails (:184).
  - Full parked-chain argv equality (:209).
  - Command quoting for spaces, single quotes, `$(…)`, backtick and newline (:223).
  - Single `;` separator with no `&&` (:246).
  - Recover half carries the pane flags alone (:261).
  - The three refusals, each asserting the eager exec and exactly one WARN naming `pane_key` and `error` (:278).
  - Prefs read failure and an unbuildable prefs store (:317, :331).
  - Absent hook store (:344) and unreadable store (:362).
  - One lookup record and one counted prefs load: all three tails (:385).
  - Exe and pane read once, with `TMUX_PANE` moved after the marker write to prove no second read (:410).
  - Empty stored command (:440).
  - Nil `Decision` fallback (:454).
  - The existing direct-caller suites (cmd/state_hydrate_exec_log_test.go, cmd/hooks_read_lock_test.go) are not in the change-set, so they are unedited.
- Notes: The eager and no-registration cases assert exec argv and no pane-option write, but not the skeleton-marker clear or the log records. Both are shared unconditional code (`unsetSkeletonMarkerOrLog` inside `markPendingThenUnsetSkeletonMarker`, `handOffToHookOrShell`), and the existing tail suites already pin the clear for the no-registration pane, so this is not a gap that matters.

CODE QUALITY:
- Project conventions: Followed. Seams are nil-tolerant func fields; there is no new log component or attr key (`pane_key` / `error` on the existing hydrate catalog); quoting goes through `internal/shellquote`; the prefs read uses the non-migrating route.
- SOLID principles: Good. Resolution, marking and hand-off are separate functions, and one decision pointer is carried between them.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. Comments hold against the code.
- Issues: Two pre-existing test names in the modified suite now state the opposite of the ordering this task established (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_hydrate_test.go:1672 — `TestHydrate_SignalArrived_LookupHappensAfterSleepAndMarkerUnset`, and its sibling `TestHydrate_FileMissing_LookupHappensAfterMarkerUnset` at cmd/state_hydrate_test.go:1732, are named for a lookup that follows the settle sleep and the skeleton-marker unset. Since this task, the lookup runs at the top of `runHydrate` (cmd/state_hydrate.go:112), before the FIFO is even opened. What the bodies actually assert is that the hand-off exec follows the unset. Rename both for what they assert, e.g. `…_HandOffHappensAfterSleepAndMarkerUnset` / `…_HandOffHappensAfterMarkerUnset`. — FAILS: the suite names as intended the very ordering this task inverted (a lookup deferred until after the mid-restore clear). A maintainer reading it is told that resolving after the clear is the rule, and that is the one ordering the specification forbids because it reopens the no-marker tick window.

UNSETTLED:
- "`TestNoParkedShWrapperPostRestore`, `TestExitClosesRestoredPane_NoHook` and `TestExitClosesRestoredPane_WithHook` pass unmodified — the parked shell exists only on the lazy path." — needs an integration-lane run: `go test -tags integration -p 1 ./internal/restore -run 'TestExitClosesRestoredPane_|TestNoParkedShWrapperPostRestore'`. Reading confirms the fixture pins `eager` (a sound adaptation to the lazy default) and that no parked shell is composed on a non-lazy decision. Whether the suite passes can only be observed by running it.
