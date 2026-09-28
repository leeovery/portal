TASK: Enter Resumes the Pane (lazy-resume-on-attach-4-3, tick-9078b4)

ACCEPTANCE CRITERIA:
- The leave sequence is written to stdout before `ClearMarker` is called, and `LookupResume` is not called until `ClearMarker` has returned nil — asserted on a shared order recorder, not on three independent call counts.
- A `ClearMarker` that returns an error produces exactly one `resume-draw` exec carrying the same command, hook key, pane and pane key plus `--report` holding the error's text; `LookupResume` is never called and no shell or hook exec happens.
- After a failed clear the redrawn panel is the waiting panel with both key hints live — the redraw is a fresh draw of the same screen, not a decision about whether to draw.
- A found registration execs `/bin/sh -c "<command>; exec <$SHELL>"`, the same shape the helper already uses for a hook; the `exec` INFO carries `hook_present` true.
- A miss, an empty command and a lookup error each exec `$SHELL` alone with `hook_present` false, and the marker is cleared in every one of those cases.
- The command executed is the one `LookupResume` returned at the moment of the answer, not the `--command` the process was launched with — proved with a seam returning a different command from the one in the payload.
- A `ClearMarker` that returns nil for an already-absent marker is indistinguishable from one that cleared a set marker: the answer proceeds either way.
- The terminal is restored before every exec on this path, including the redraw after a failed clear.
- An exec that returns (the exec failed) terminates non-zero through the existing `defaultExecShell` shape — one WARN, `log.Close(1)`, `osExit(1)` — rather than returning to the read loop.
- `$SHELL` unset still resolves `/bin/sh` through the existing `resolveShell`, and the hydrate helper's own exec suites pass unchanged after the composition is extracted.

STATUS: complete

SPEC CONTEXT: Section 6.1 — Enter hands the pane over in place: leaving the alternate screen reveals the replayed transcript and the resume command starts over it; the command is re-read from the store at the moment of the answer (an entry gone, or an unreadable store, drops the pane to a plain shell, with the marker cleared either way). Section 7.2 — the pending marker is cleared only after the pane has left the panel's screen, and a clear that fails holds the answer: the panel is redrawn with the reason on its report row, neither hook nor shell runs, and the key can be pressed again. Section 5.3 — the report row sits between the command and the key hints and stays until the next key. Section 4.3 — the hook runs in the helper's `sh -c '<HOOK>; exec $SHELL'` shape so the chain's tail finds no marker and adds no second shell.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_wait.go:315-326 — `resumeAnswerEnter`: `resumeUnfreeze`, then `resumeRegistrationOrLog(cfg.Logger, cfg.LookupResume, cfg.HookKey)`, `cfg.restore()`, `enableTTYSignalsOrLog`, `handOffToHookOrShell`.
  - cmd/state_resume_wait.go:363-370 — `resumeUnfreeze`: writes `hydrateResetPreamble` to `cfg.Stdout`, then calls `ClearMarker`; on error returns `resumeReport(cfg, resumeScreenPanel, err)` without touching the store.
  - cmd/state_resume_wait.go:375-381 / 397-400 — `resumeReport` sets `Report = err.Error()` on a copy of the payload and `resumeRedraw` restores the terminal before handing off to `resume-draw` carrying the whole payload.
  - cmd/state_resume_wait.go:74-78 — the `ClearMarker` / `LookupResume` seams; production wiring at cmd/state_resume_wait.go:482-485 (`state.UnsetResumePendingMarker(tmux.DefaultClient(), tmux.PaneIDTarget(pane))`, `lookupResumeRegistration`).
  - cmd/state_resume_wait.go:404-410 — `lookupResumeRegistration`: `loadHookStore()`, nil store gives the zero result and no error, otherwise `LookupOnResume(hookKey, hooks.ViaHydrate)`.
  - cmd/state_resume_chain.go:171-173 — `hookExecArgs` returns `"/bin/sh", {"sh", "-c", command + "; exec " + shell}`; cmd/state_resume_chain.go:140-148 — `handOffToHookOrShell` is its caller, shared by the hydrate helper (cmd/state_hydrate.go:239-252 `execShellOrHookAndExit`), the Enter answer and the recover tail (cmd/state_resume_recover.go:58).
  - cmd/state_resume_chain.go:152-165 — `resumeRegistrationOrLog`, the one lookup-degradation rule (error → `hook lookup` DEBUG result=error + `lookup on-resume hook failed` WARN; miss/empty → `hook lookup` DEBUG result=miss), shared with the helper's `lookupOnResumeOrLog` (cmd/state_hydrate.go:228-235).
  - cmd/state_hydrate.go:388-393 — `defaultExecShell` (the production `ExecSelf`), the WARN + `log.Close(1)` + `osExit(1)` shape on a returned exec.
- Notes: The body matches the prescribed order exactly (leave → clear → lookup → restore → exec INFO → exec). The one addition over the task's text, `enableTTYSignalsOrLog` between restore and exec, belongs to a later signal-handling task and sits after the restore, so it disturbs none of this task's ordering. The draw consults no store (cmd/state_resume_draw.go:37-73), so the post-failure redraw is a fresh draw of the payload rather than a draw-or-not decision. Production tmux behaviour for criterion 7 — `set-option -pu` on a pane never carrying the option exits 0 — is pinned by internal/tmux/pane_option_realtmux_test.go:52-66. No drift.

TESTS:
- Status: Adequate
- Coverage:
  - Order (cmd/state_resume_enter_test.go:67-94): the shared `probe.order` recorder is fed by the stdout writer (`orderedWriter`, cmd/state_resume_wait_test.go:63-70), the clear seam and the lookup seam (cmd/state_resume_wait_test.go:87-99); asserts `[stdout, clear, …]` and clear-before-lookup.
  - Failed clear (cmd/state_resume_enter_test.go:96-161): exact `resume-draw` argv equality with `Report` = the error's text and a single exec; zero store reads and no hook/shell exec; the redraw of the reported payload paints the command, the reason, and both footer labels (`resume` lowercase appears only in the footer hint, as the title is `Resume session` and the label `ON RESUME` — internal/tui/resume_panel.go:9-15).
  - Store at the answer (cmd/state_resume_enter_test.go:163-241): a seam returning `make deploy-rewritten` against a payload of `make deploy`; miss, empty command and unreadable store each give a bare shell with `hook_present` false and exactly one clear; the unreadable case asserts the WARN and the DEBUG result=error; `$SHELL` empty resolves `/bin/sh`.
  - Shape parity (cmd/state_resume_enter_test.go:246-277): argv equality against `execShellOrHookAndExit` driven over a staged store for a command with embedded quotes.
  - Terminal/exec failure (cmd/state_resume_enter_test.go:279-338): `restoredAtExec == 1` over the four outcomes including the failed-clear redraw; a real `syscall.Exec` failure through `defaultExecShell` gives one `osExit(1)` and one `exec handoff failed` WARN.
  - Production lookup (cmd/state_resume_enter_test.go:340-370): real store read, and an unresolvable store reading as the zero registration with no error.
- Notes: "it treats an already-absent marker as cleared" (cmd/state_resume_enter_test.go:231-240) exercises the same path as the found-registration case, since the waiter cannot tell the two apart; the property it names lives in tmux and is covered by the real-tmux unset test cited above, so nothing is left unguarded. The hydrate suites keep their literal argv expectations (e.g. cmd/state_hydrate_test.go:1334, 1403, 1460, 1625, 1666-1669), which `hookExecArgs` reproduces for the same inputs.

CODE QUALITY:
- Project conventions: Followed — seams on the config struct, the `sync.OnceFunc` restore, the exec marker immediately before the exec, the closed log vocabulary reused rather than extended.
- SOLID principles: Good — `resumeUnfreeze` holds the leave/clear/hold rule once for both answer paths; `handOffToHookOrShell` / `hookExecArgs` are the single declaration of the hand-off shape.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — comments on the changed code hold true against it.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`$SHELL` unset still resolves `/bin/sh` through the existing `resolveShell`, and the hydrate helper's own exec suites pass unchanged after the composition is extracted." — the `/bin/sh` half is settled by reading (cmd/state_hydrate.go:185-191, asserted at cmd/state_resume_enter_test.go:223-229), and reading finds `hookExecArgs` producing exactly the argv the hydrate suites expect; that those suites pass needs a run of `go test ./cmd -run 'TestHydrate'`.
