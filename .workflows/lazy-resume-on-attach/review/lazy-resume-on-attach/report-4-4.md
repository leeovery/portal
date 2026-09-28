TASK: lazy-resume-on-attach-4-4 (tick-11a36d) — The Chain Tail: a Waiter That Dies Leaves a Usable Pane (`portal state resume-recover` + `tmux.ReadPaneOption`)

ACCEPTANCE CRITERIA:
- A marker read returning an empty value with no error produces: no write to stdout, no `ClearMarker` call, no exec, and a nil return.
- A marker read returning `1` produces, in order: the leave sequence on stdout, one `ClearMarker` call, and one `$SHELL` exec.
- A marker read that returns an error is treated as still pending — the pane is recovered exactly as a set marker is.
- A `ClearMarker` that fails still execs the shell, and emits exactly one WARN carrying `pane_key` and `error` under the `hydrate` component; a clear that succeeds emits no WARN.
- The WARN's message is distinct from the helper's failed-mark WARN, so a grep separates "came back eager because a write failed" from "is wrongly frozen".
- The leave sequence is written before `ClearMarker` is called.
- `tmux.ReadPaneOption` composes `show-options -p -t <target>` naming no option followed by `display-message -p -t <target> -F "#{<option>}"`, in that order, takes `tmux.Target`, reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to — on the probe's exit status; `internal/tmux/target_composition_guard_test.go` passes with it in place.
- The command reads no `hooks.json`, no `prefs.json` and no theme, and calls no renderer.
- The command's log records are the WARN above and the existing `exec` INFO, and nothing else — no new event and no new attr key.

STATUS: complete

SPEC CONTEXT: §4.3 specifies that a waiter exiting without handing the pane over drops the pane to a plain shell through a chain tail that leaves the panel's screen, clears the pending marker, then execs the user's shell. The shell runs whether or not the clear landed, and a failed clear is recorded as a WARN, since no waiter is left to hold the answer back (§7.2, §7.3). The Corrigendum of 2026-09-19 adds that the tail reads the pending marker and does nothing for an answered pane, so a restored pane still closes on the first `exit`. A failed read counts as still pending. Later corrigenda (2026-09-22, 2026-09-28) confirm that the parked shell, not the waiter, is the pane's process, and they add a backstop for a tail that cannot start. That backstop belongs to another task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_recover.go:35-60 — `runResumeRecover`. The do-nothing branch is at :38-41 (`err == nil && !state.ResumePendingSet(value)` returns nil before any write, clear or exec). The leave sequence is at :46, the clear and its WARN "unset resume pending marker failed" (`pane_key`, `error`) at :52-54, and the hand-off at :58 via `handOffToHookOrShell(..., "")`, which emits the existing `exec` INFO with `target`/`args`/`hook_present=false` and execs `resolveShell()` (cmd/state_resume_chain.go:140-148, :132-135).
  - cmd/state_resume_recover.go:62 — `resumeRecoverRunFunc` seam. :66-98 is the hidden `stateResumeRecoverCmd` with `--pane`/`--pane-key`. Production binds `tmux.DefaultClient().ReadPaneOption(tmux.PaneIDTarget(pane), state.ResumePendingOption)`, `state.UnsetResumePendingMarker`, and `defaultExecShell`. It is registered on `stateCmd` at :100-105.
  - internal/tmux/tmux.go:346-356 — `ReadPaneOption(target Target, name string)`. It runs a `show-options -p -t <target>` probe naming no option, whose error returns before the read, then `display-message -p -t <target> -F "#{<name>}"`, each failure wrapped.
  - cmd/state_hydrate.go:299 — `parkedResumeChain` joins draw and recover with `;`, so a draw that could not exec the waiter still falls through to the tail (edge case covered).
  - internal/log/process_role.go:27 — `resume-recover` resolves to the hydrate role.
  - CLAUDE.md `tmux` row — the options clause describes `ReadPaneOption`'s probe-then-read shape and its reason, and the list of methods handed an already-composed target includes `ReadPaneOption`. Both places the task named are updated.
- Notes: The code has moved past the task text in several places, all of them sound:
  - `EnableTTYSignals` is called between the clear and the exec, added by the kill-key task. It can emit the existing `enable terminal signals failed` WARN, so it adds no new event or attr key.
  - The command tolerates unknown flags and positionals, and falls back to `$TMUX_PANE` when `--pane` is empty. Both came with the version-skew task.
  - `ResolveHookKey` was later reduced to delegate to `ReadPaneOption` (tmux.go:248-250), making it the sole home of the probe-then-read rule instead of two copies. CLAUDE.md reflects this.
  - The argv pins live in `internal/tmux/pane_option_test.go` beside the SetPaneOption/UnsetPaneOption tests, not in `tmux_test.go`.

  Nothing the intent needs was lost. The only record the tail can emit that the criteria do not name is the later-added `enable terminal signals failed`. It is an existing catalog entry, so the "no new event, no new attr key" intent holds.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_recover_test.go:86-190 (`TestRunResumeRecover`):
    - "it does nothing for a pane that was already answered" asserts one read, zero stdout bytes, zero clears and zero execs.
    - "it recovers a pane whose marker is still set", "it treats a failed marker read as still pending" and "it execs the shell even when the clear failed" go through `assertRecovered`, which checks the exact preamble, one clear, one exec of `resolveShell()` with argv `[shell]`, and exactly one `hydrate`/`exec` INFO with target/args/`hook_present=false`.
    - "it leaves the panel's screen before it drops the protection" pins the order `read, stdout, clear, exec` through a recording writer.
    - "it reads no store and draws nothing" asserts that stdout carries only the preamble. It also runs a source assertion over `state_resume_recover.go` that forbids the hooks, prefs, theme and tui imports and calls to `loadHookStore`, `loadPrefsStore`, `loadPrefsStoreNoMigrate` and `newThemeLoader`. All four names exist, at cmd/hooks.go:277, cmd/config.go:117, cmd/config.go:91 and cmd/open.go:546.
  - cmd/state_resume_recover_test.go:242-290 (`TestRunResumeRecover_FailedClearRecord`):
    - The failed clear produces exactly one record at WARN or above, matching (`hydrate`, message), with `pane_key` and an `errors.Is`-matching `error`.
    - An AST scan over cmd's production sources confirms the message occurs at exactly one WARN site. It is distinct from the helper's `set resume pending marker failed` at cmd/state_hydrate.go:351.
    - A successful clear produces no WARN and exactly one record, the exec marker.
  - internal/tmux/pane_option_test.go:96-165: both argvs pinned in order through `commandertest.FromFunc`, which returns `*Scripted`; the probe failure returns before the read with one call recorded; a failed value read is wrapped.
  - internal/tmux/pane_option_realtmux_test.go:79-125: a real-tmux round trip of unset → set → unset reads `""`/`1`/`""`, and a read against `%99999` errors.
  - The command's parse is tested at cmd/state_resume_recover_test.go:292-323, and cmd/state_test.go registers the hidden subcommand.
- Notes: Each test would fail if its behaviour broke:
  - Reordering the preamble after the clear fails the order test.
  - Gating the exec on a successful clear fails "execs the shell even when the clear failed".
  - Ignoring the read error fails "treats a failed marker read as still pending".

  The tests are not bloated. The six foreign-argv and four environment-pane cases belong to later tasks.

CODE QUALITY:
- Project conventions: Followed. It uses the `Target` type at every `-t`, the function-var seam staged through `withFuncSeam`, the `hydrate` component logger with existing attr keys, `logtest.Sink` queried through `Records().Matching(...).Only(...)`, the sourceguardtest primitives, and a real-tmux test on a disposable socket in the unit lane. It adds no `t.Parallel()`.
- SOLID principles: Good. The tail depends only on three narrow function seams plus the TTY seam, and the tmux method is a single-purpose read.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments state the reasons for the order and for the unconditional shell, and they hold against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`tmux.ReadPaneOption` ... reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to — on the probe's exit status ...; `internal/tmux/target_composition_guard_test.go` passes with it in place." — Reading found the real-tmux tests present and correctly shaped, and the argv uses the `string(target)` over a `Target`-typed parameter shape the guard's own clean fixtures use. Settling it requires running `go test ./internal/tmux -run 'TestReadPaneOption|TestTmuxTargetsAreComposedThroughTheExactnessVocabulary|TestBareTargetGuard'` on this machine's tmux.
