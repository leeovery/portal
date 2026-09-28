SECTION: 1. What This Feature Changes / 2. Resume Modes / 3. Storing, Setting and Reading the Mode / 4. The Waiting Pane

SCOPE: Commit range 34c342f90..df5297eb9. 34c342f90 is the parent of the first task commit, 70301898879d518fe77d1e8b64a84fde7936ccd8. The range is 194 commits, filtered to the 231 files in impl-files.txt. Spec sections 1-4 were read in full, along with every corrigendum; the ones that bear on this section are §4.2 (2026-09-19, 2026-09-22), §3.1 (2026-09-19), §7.3 (2026-09-19 tails, 2026-09-28 backstop) and §4.3 (2026-09-22, 2026-09-28).
Production files read in their final state:
- internal/resumemode/resumemode.go
- internal/prefs/store.go
- internal/hooks/registration.go, store.go and lookup.go
- cmd/hooks.go
- cmd/doctor.go (the resume-mode and pending lines, dep resolution and the exit-code arms)
- cmd/config.go (loadPrefsStoreNoMigrate and configFilePath)
- cmd/state_hydrate.go, also against its pre-feature version at 34c342f90
- cmd/state_resume_chain.go, state_resume_draw.go, state_resume_wait.go and state_resume_recover.go
- cmd/tty_signals.go, tty_flush_darwin.go and tty_termios_darwin.go
- internal/log/process_role.go
- internal/tui/pane_appearance.go (ResolvePaneTheme and the drop ordering)
Also confirmed:
- `git diff --stat` shows no production file changed under internal/restore or cmd/bootstrap.
- The README hook and prefs text was checked.
Linters and build checks run:
- `go vet ./...`: exit 0
- `go vet -tags integration ./...`: exit 0
- `gofmt -l $(git ls-files '*.go')`: no output
- `golangci-lint run`: 0 issues
Package tests run:
- `go test ./cmd -count=1 -shuffle=on`: 3 random seeds plus a fixed seed 20260928, run twice
- `go test ./internal/tmux -count=1 -v`
- `go test ./internal/hooks`, `./internal/hooksweep`, `./internal/resumemode`, `./internal/prefs`, `./cmd/bootstrap` and `./internal/shellquote`, each `-count=1`
Integration tests run, each with `-p 1` against a disposable tmuxtest socket, with ps checked afterwards:
- `go test -tags integration -p 1 -count=1 -v ./internal/restore -run TestLazyResumePanel_RestoredPaneHoldsThePanel`
- `go test -tags integration -p 1 -count=1 -v ./internal/restore -run 'TestExitClosesRestoredPane_|TestNoParkedShWrapperPostRestore'`
- `go test -tags integration -p 1 -count=1 -v ./cmd -run 'TestRenameRestoreCleanupSurvival_KeepsRestoredTokenKeyedHook|TestDoctorFix_TmuxTransient_DoesNotWipeHooks|TestDaemon_ThrottledHookCleanup_ReapsStaleRetainsLiveOnIdleServer'`
One guard mutation was run in a throwaway copy of the source under the system temp dir. The copy was removed afterwards.
Machine load during the runs was a load average of 406-463 on 10 cores.

MEASURED:
- "`doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites." [1-2] — `go test ./internal/hooks -count=1` ok; `go test ./internal/hooksweep -count=1` ok; `go test ./cmd -count=1 -shuffle=on` ok; `go test ./cmd/bootstrap -count=1` ok; `go test -tags integration -p 1 -count=1 -v ./cmd -run 'TestRenameRestoreCleanupSurvival_KeepsRestoredTokenKeyedHook|TestDoctorFix_TmuxTransient_DoesNotWipeHooks|TestDaemon_ThrottledHookCleanup_ReapsStaleRetainsLiveOnIdleServer'` → all three PASS — holds
- "Both lanes build: `go test ./...` and `go test -tags integration -p 1 ./...` each compile the whole tree, so no integration-tagged suite is left calling the old signature." [1-3] — `go vet ./...` exit 0 and `go vet -tags integration ./...` exit 0; vet type-checks every package's test files in each lane — holds
- "`$SHELL` unset still resolves `/bin/sh` through the existing `resolveShell`, and the hydrate helper's own exec suites pass unchanged after the composition is extracted." [4-3] — `go test ./cmd -count=1 -shuffle=on -v` → every TestHydrate_* and TestHydrateExecLog_* test PASS — holds
- "`tmux.ReadPaneOption` ... reads back `1` for a set marker and the empty string for an unset one on a real pane, and errors for a target no live pane answers to — on the probe's exit status ...; `internal/tmux/target_composition_guard_test.go` passes with it in place." [4-4] — `go test ./internal/tmux -count=1 -v`. All of these PASS and none skipped:
  - TestReadPaneOption_RealTmux, both subtests: "reads back a set and an unset pane option" and "fails a read against a target no live pane answers to"
  - TestReadPaneOption
  - TestTmuxTargetsAreComposedThroughTheExactnessVocabulary
  - TestBareTargetGuard_*
  — holds
- "`TestNoParkedShWrapperPostRestore`, `TestExitClosesRestoredPane_NoHook` and `TestExitClosesRestoredPane_WithHook` pass unmodified — the parked shell exists only on the lazy path." [4-5] — `go test -tags integration -p 1 -count=1 -v ./internal/restore -run 'TestExitClosesRestoredPane_|TestNoParkedShWrapperPostRestore'` → 3 PASS. The only diff to exit_closes_pane_integration_test.go is the CaptureStructure return arity and a fixture registration pinned `resumemode.Eager`; the assertions are unchanged — holds
- "The subject's resting tree — the parked shell plus the waiter, measured once the pane is waiting and the draw is gone — carries a combined resident size below 22 MB, the ceiling the daemon was measured at, and the figure is reported by the test whether it passes or fails." [4-7] — `go test -tags integration -p 1 -count=1 -v ./internal/restore -run TestLazyResumePanel_RestoredPaneHoldsThePanel` → PASS. It logged "waiting pane resting tree: 10304 KB combined resident over 2 processes": the sh -c at 2080 KB plus resume-wait at 8224 KB, against the 22528 KB ceiling — holds
- "The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`, and no `portal state resume-draw` survives the hand-off." [4-7] — same run. The logged tree is pid 35136 `sh -c trap : INT QUIT; '<exe>' 'state' 'resume-draw' …; '<exe>' 'state' 'resume-recover' …; s=$?; …`, parent of pid 35621 `portal state resume-wait …`. No resume-draw appears — holds
- "`send-keys exit` closes the subject's pane on the first press, within the same budget the existing restored-pane suite uses." [4-7] — same run, subtest "it closes the pane on the first exit after the resume" PASS — holds
- "Restored a second time from that capture, the subject comes back holding the panel with the pre-reboot line intact underneath it, its pending marker set, and its key in the fresh capture's pending set — an unanswered offer returns rather than being spent, and nothing of the transcript is lost across the second reboot." [4-7] — same run, subtests "it keeps the waiting pane's transcript through a capture it was never answered through" and "it offers the panel again on the reboot after the one that drew it" PASS — holds
- "The suite carries `//go:build integration`, uses `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket and a `restoretest`-built binary, spawns no `portal state daemon`, and leaves no server or subprocess behind." [4-7] — read: internal/restore/lazy_resume_panel_integration_test.go:1 carries the build tag, and the file uses `restoretest.BuildPortalBinaryDir` at :283, `portaltest.IsolateStateForTest` at :287 and `tmuxtest.New(t, "ptl-lazy-")` at :304, with no daemon spawn. Measured after the run: `ps -eo pid,ppid,etime,command | grep -E 'ptl-|resume-wait|resume-draw|resume-recover'` → nothing — holds
- "the guard is proved to discriminate by mutation" (the planned mutation: a bare `cfg.ExecShell(...)` in a production function outside the two helpers reddens `TestExecSeamsAreCalledOnlyByTheHandOffHelpers`) [4-8] — In a copy of cmd/, internal/, go.mod, go.sum and main.go under the system temp dir, `cfg.ExecShell("/bin/sh", []string{"sh"})` was added inside `handleHydrateTimeout`. `go test ./cmd -count=1 -run TestExecSeamsAreCalledOnlyByTheHandOffHelpers` → FAIL "state_hydrate.go:311 handleHydrateTimeout hands the process image over directly". The copy was removed — holds
- "`go test ./cmd -count=1` and `go test ./cmd -shuffle=on` pass" [4-10] — holds. Runs:
  - `go test ./cmd -count=1 -shuffle=on` with random seeds (one unlogged, 1790611590949212000, 1790612066107032000): all ok
  - `-shuffle=20260928`: failed once in TestSelfSupervisionCounter_BoundaryKEqualsNMinus1 with "probe invoked 12 times; want ≥ 15". That test runs a 1 ms ticker inside a 100 ms wall-clock window, and the machine was at a load average of 406-463 on 10 cores.
  - The same seed re-run: ok
  - `go test ./cmd -count=5 -run 'TestSelfSupervisionCounter_BoundaryKEqualsNMinus1$'`: ok
  The one red run is a load-induced timing miss in a daemon test that the flag reset does not touch, not an order dependency.
- "On a real pty: bytes written to the master before the flush are not readable from the slave after it, and bytes written after the flush are." [5-4] — `go test ./cmd -count=1 -shuffle=on -v` → TestFlushTTYInput_RealPTY PASS on darwin — holds
- "CSI (`[`), SS3 (`O`) and OSC (`]`) sequences are swallowed exactly as before, to their final byte, terminator or cap — every existing case in `cmd/state_resume_screens_test.go` passes unmodified" [5-7] — same run → TestRunResumeWait_Screens PASS; cmd/state_resume_wait.go:225-246 keeps the 16/64 caps and the CSI-final/OSC-terminator rules — holds
- "`rg -n 'LoadResumeMode\(' --type go --glob '!*_test.go'` reports the definition in `internal/prefs/store.go` and exactly one call site, the shared resolver in `cmd`" [6-9] — ran it → `internal/prefs/store.go:213` (the definition) and `cmd/state_hydrate.go:222` (inside `installResumeModeOf`), and nothing else — holds
- "`cmd/doctor_resume_mode_test.go`, `cmd/state_hydrate_lazy_test.go` and `cmd/state_hydrate_test.go` pass without modification" [6-9] — `git show --stat e5e09e3b7` shows that commit touched only cmd/doctor.go, cmd/install_resume_mode_test.go and cmd/state_hydrate.go. TestDoctorResumeMode, every TestHydrateLazy_* and every TestHydrate_* PASS in the cmd run — holds
- "The existing hydrate, Enter, discard, recover, signal-generation and exec-seam guard suites pass with what they assert unchanged" [8-1] — `go test ./cmd -count=1 -shuffle=on -v`. All of these PASS, and the only skips in the run are fish-shell completion cases:
  - TestResumeAnswerEnter_*, TestResumeAnswerDiscard_*, TestRunResumeRecover*, TestStateResumeRecoverCommand*
  - TestParkedResumeChain_Backstop and TestParkedResumeChain_GroupSignal
  - TestHydrateLazy_ClearsSignalGenerationBeforeTheFirstDraw, TestResumeWait_SignalGenerationOnHandingThePaneOn and TestResumeWait_SignalGenerationStaysOffAcrossScreens
  - the pty-backed TestTTYSignals_RealPTY, TestCookTTY_RealPTY and TestResumeHandOffs_TerminalModes_RealPTY
  - TestExecSeamsAreCalledOnlyByTheHandOffHelpers
  The package compiles, so no test still names `execShellAndExit` — holds

NOT MEASURED:
- "`go test ./...` passes and `go vet -tags integration ./...` is clean: the signature is unchanged, so no integration-tagged suite needs an edit." [1-8] — The `go test ./...` half is the whole unit lane, which this verifier may not run; the suite runs once, later, over the corrected tree. The halves measured here hold: `go vet -tags integration ./...` exit 0, `go test ./internal/hooks -count=1` ok, `go test ./cmd -count=1` ok.
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" [4-8] — Both whole lanes are reserved for the later suite pass. The planned mutation is measured above.
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" [4-9] — Both whole lanes are reserved for the later suite pass. The declaration-collision concern is settled: `go vet ./...` and `go vet -tags integration ./...` type-check the cmd test package, with `registrationHookKey`, `lookupReturning` and `assertLookupDebug` in it, without error.
- "`go test ./internal/tmux/ ./cmd/ -count=1` and `go test ./...` pass" [4-12] — The first command was run and both packages pass. The `go test ./...` half is the whole unit lane, reserved for the later suite pass.
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" [4-13] — Both whole lanes are reserved for the later suite pass. `go test ./internal/shellquote -count=1` and `go test ./cmd -count=1` pass.
- "`go test ./internal/state/ ./cmd/ -count=1`, `go test ./...` and `go test -tags integration -p 1 ./...` pass" [4-11] — subject is the frozen-pane capture composite: section 7, Protecting the Waiting Pane's Saved Transcript
- "Against a real tmux pane: a set marker reads back as `1` through `#{@portal-resume-pending}`; after the unset it reads back empty; a second unset of the now-absent option succeeds; an unset against a target naming no live pane fails." [2-1] — subject is the pending-marker primitive: section 7 (§7.3) / section 8 (§8.2)
- "The daemon's tick summary, its per-pane `pane captured` DEBUG and every existing capture test pass unchanged against twelve-field fixtures." [2-2] — subject is the saver's capture arity: section 7 (§7.3)
- "After each of `break-pane`, a `kill-window` of an earlier window under `renumber-windows on`, `move-pane` back into the original window, `move-pane` into another session, `respawn-pane -k` and `rename-session`, the subject pane is still reported in `CaptureStructure`'s pending set under its current pane key." [2-5] — subject is marker durability: section 7 (§7.3)
- "The suite runs in the unit lane (`go test ./internal/state`), skips cleanly where tmux is absent, spawns no daemon, builds no binary, and leaves no server behind." [2-5] — subject is the marker durability suite: section 7 (§7.3)
- "The production reader takes a read deadline against a real terminal and times a read out at the duration set — asserted over a pty slave rather than a pipe, because a pipe takes a deadline whatever reader shape is bound." [3-5] — subject is the pane's appearance probe: section 5 (§5.2)
- "A reader whose `SetReadDeadline` is unsupported returns dark without reading and without blocking, and that branch is reachable only through a seam a test binds — no production path takes it." [3-5] — subject is the pane's appearance probe: section 5 (§5.2)
- "`ansi.Wrap` is called in exactly one non-test file in `internal/tui`, and all three sites reach it through that one helper." [3-8] — subject is panel text wrapping: section 5, The Resume Panel
- "Today's shipped band and panel copy renders the same visible text at the same row widths as before the change, at widths 20-120 — the latent defect is fixed without moving any copy that ships." [3-8] — subject is panel and band copy rendering: section 5, The Resume Panel
- "Every existing canvas suite passes untouched: `TestCanvasCellBackground_EveryInGridCellIsCanvas`, `TestCanvasCellBackground_TitleAndFooterGaps`, `TestOuterFill_*`, `TestContentInset_GutterPaintedCanvas`, `TestColourless_FillEmitsNoCanvasBackground`." [3-9] — subject is the canvas fill: section 5, The Resume Panel
- "`capture-pane -p` on the subject returns the discard confirmation — its `▲ Discard resume?` title, the registered command and its `y discard   esc cancel` footer — while `capture-pane -a -p` still returns the pre-reboot line intact underneath it." [5-6] — subject is the discard route: section 6 (§6.2)
- "`@portal-resume-pending` reads back empty on the subject within a bounded poll, the pane is absent from `CaptureStructure`'s pending set, and a scrollback write for that pane lands where it was previously skipped." [5-6] — subject is the discard route: section 6 (§6.2)
- "`send-keys exit` closes the subject's pane on the first press, within the budget the existing restored-pane suite uses." [5-6] — subject is the discard route's integration suite: section 6 (§6.2)
- "A second reboot restored from the post-discard capture brings the subject back showing its transcript with no panel, carrying no pending marker, and with no waiter in its pane." [5-6] — subject is the discard route: section 6 (§6.2)
- "The suite carries `//go:build integration`, uses `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket and a `restoretest`-built binary, spawns no `portal state daemon`, and leaves no server or subprocess behind." [5-6] — subject is the discard integration suite: section 6 (§6.2)
- "Against a real tmux server: one marked pane among several sessions is reported once, with its own session name, and the remaining panes are reported unmarked." [6-1] — subject is the whole-server pending read: section 8, Seeing What Is Waiting
- "No raw hex appears at the call site — `internal/tui`'s colour-literal guard passes with no exemption added." [6-4] — subject is the picker row's pending indicator: section 8 (§8.3)
- "The palette-swap completeness guard enumerates the coloured fixture and passes over it" [6-8] — subject is the pending-resume capture fixture: section 8 (§8.3)

FINDINGS:
- [in-scope] [contained] cmd/state_resume_wait.go:151 — The waiter checks the pane's size against the size the panel was drawn at only after it receives a SIGWINCH (the `case <-cfg.Winch` arm at :187 and the settled comparison at :190-195). Nothing compares them when a wait starts.
  The drawn size is read by the draw (cmd/state_resume_draw.go:43) and passed to the waiter (:71), but the waiter's SIGWINCH channel is armed only in its own RunE (`Winch: winchSignals()`, cmd/state_resume_wait.go:479). A resize between those two points is lost. That window covers the draw's appearance probe (up to `appearanceDetectTimeout`, 50 ms, internal/tui/appearance_gate.go:12), its render, and the exec and start-up of the wait process. In that window the signal reaches a Go process that has not asked for SIGWINCH, or crosses an exec where it is at SIG_DFL, and both discard it. The waiter then holds a panel drawn for the old size indefinitely.
  The same window opens on every hand-off back to a draw: the first draw, `d`, Escape, a report redraw, and each settled resize.
  Fix:
  - At the top of `resumeWaitLoop`, read `cfg.Size()` once and hand off through `resumeRedraw(cfg)` when the read succeeds and differs from `cfg.Width`/`cfg.Height`. A failed read stays put, so a size read that keeps failing cannot loop redraws.
  - Add two `TestRunResumeWait_Resize` cases: one where the size already differs at start and no SIGWINCH is delivered, expecting exactly one hand-off to `resume-draw`; one where the start read fails, expecting none.
  - Change the harness's size stubs (`resizedSize` and the per-case sizes at cmd/state_resume_wait_resize_test.go:171 and :268) to answer the drawn 80x24 until the resize is delivered.
  — FAILS: When a pane's final size change lands in that window, the panel stays painted at the previous size until the pane is next resized. This can happen when a window drag pauses long enough to settle (150 ms) and then stops inside the redraw's window, or when a client attaches to a session whose restore-time draw is still running. On a larger pane the card sits off-centre and part of the pane is not painted with the canvas; on a smaller one tmux clips the alt-screen rows, which can cut off the card and its key hints. This is contrary to §4.2: "the waiter replaces itself with a fresh draw, which draws at the new width".

COVERAGE:
- The install-wide default is lazy, and resolving a registration against the install never answers Unset — internal/resumemode/resumemode.go:31, :64-72 — read; `go test ./internal/resumemode -count=1` ok
- The mode vocabulary is strict: only "eager" and "lazy" parse, with no trimming or case-folding — internal/resumemode/resumemode.go:51-60 — read
- A stored value decodes tolerantly: a string is the command; an object's command and resume are each read only when they are strings; a resume the vocabulary refuses carries no mode; any other shape decodes to nothing without failing the file — internal/hooks/registration.go:31-48, :74-80 — read; `go test ./internal/hooks -count=1` ok
- The writer picks the stored shape: the string form when the registration carries no mode, the object form when it does; an entry the call did not name is re-emitted from its decoded bytes — internal/hooks/registration.go:53-64 — read
- A registration is written whole: Set stores only the command and mode it was handed, so nothing from the replaced value or its bytes carries forward — internal/hooks/store.go:162 — read
- A pin is dropped when a re-registration omits the flag, because an Eager/Lazy predecessor differs from an Unset registration and is rewritten as a string (not treated as a no-op) — internal/hooks/store.go:174-187 with registration.go:68-70 — read; TestHooksSetResumeMode "it drops an object-form predecessor's mode when the flag is not passed" PASS
- A registration carrying no command, or an empty one, is not a registration, so the pane gets a plain shell — internal/hooks/lookup.go:37-40 and cmd/state_resume_chain.go:159-161 — read; TestHydrateLazy_EmptyStoredCommandIsNoRegistration PASS
- Staleness judgement is unaffected by the value shape: StaleKeys and narrowToSnapshot read keys only — internal/hooks/store.go:292-318 — read; the stale-hook integration trio PASS (see MEASURED [1-2])
- prefs.json `resume_mode` decodes tolerantly. A non-string value decodes without error and so does not zero the record. The key is omitted on write when unset, and every existing writer preserves a hand-set value. LoadResumeMode answers the default for a missing, empty or unrecognised key and for an unreadable file — internal/prefs/store.go:73-83, :96, :213-222 — read; `go test ./internal/prefs -count=1` ok
- There is no Portal writer for `resume_mode`, so there is no surface for changing it — internal/prefs/store.go:261-330 — read: the Save* methods cover the grouping mode and theme keys only
- `--resume-mode` needs `--on-resume`, and the mode is refused before any tmux read. `--on-resume` is required (cobra refuses before RunE), the mode flag is parsed before resolveCurrentPaneKey, and a non-vocabulary or explicitly empty value is a usage error — cmd/hooks.go:197-202, :239-256, :338 — read; TestHooksSetResumeMode PASS
- `hook list` appends the mode as a fifth tab-separated column after location; it is empty for no mode or an unrecognised one — cmd/hooks.go:154 — read; TestHooksListModeColumn PASS
- Doctor reports the install's resume mode on an informational line that never drives the exit code. The line is checkInfo from the non-migrating prefs store, and checkInfo is outside doctorUnhealthy and doctorCheckCounts — cmd/doctor.go:129-136, :363-366, :609-633 — read; TestDoctorResumeMode "it moves neither the exit code nor the summary counts" PASS
- The mode is resolved once, before the FIFO open, so the replay tail, the timeout tail and the file-missing tail decide alike — cmd/state_hydrate.go:112, :196-200; each tail calls markPendingThenUnsetSkeletonMarker at :177, :320 and :340 — read; TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker (all tails) PASS
- The eager path matches today's behaviour: a registration that does not wait reaches handOffToHookOrShell → `sh -c '<HOOK>; exec $SHELL'`, and no registration gets a bare $SHELL — cmd/state_hydrate.go:239-252, cmd/state_resume_chain.go:140-148, :171-173 — read against 34c342f90's execShellOrHookAndExit; TestHydrateLazy_EagerRegistrationRestoresAsToday PASS
- A pane that cannot be marked does not wait: a refusal from the pane id, the exe resolution or the marker write downgrades Wait to eager and emits one WARN — cmd/state_hydrate.go:348-376 — read; TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked PASS
- Signal generation is switched off before the chain is parked, and the parked chain traps INT and QUIT and composes the draw and then the recover with a could-not-run backstop — cmd/state_hydrate.go:262-304, cmd/tty_signals.go:14-16 — read; TestHydrateLazy_ClearsSignalGenerationBeforeTheFirstDraw, TestParkedResumeChain_GroupSignal and TestParkedResumeChain_Backstop PASS
- The draw hands off to a fresh wait, so no drawing process is resident while the pane waits — cmd/state_resume_draw.go:37-73 — read; measured: `go test -tags integration -p 1 -count=1 -v ./internal/restore -run TestLazyResumePanel_RestoredPaneHoldsThePanel` → tree is sh -c + resume-wait, 10304 KB combined
- The waiter swallows every byte but Enter and `d` on the panel, and every byte but `y` and a lone Escape on the confirmation. Kill keys arrive as bytes under raw mode — cmd/state_resume_wait.go:135-147, :151-198, :114 — read; TestRunResumeWait_Screens and TestTTYSignals_RealPTY PASS
- The waiter declines no hangup: only SIGWINCH is notified, so tmux tearing the pane down ends it — cmd/state_resume_wait.go:425-429 — read; TestRunResumeWait_InstallsNoHangupTerminateOrInterruptHandler PASS; measured: no resume-* process survived the integration runs
- A resize waits for the size to settle (150 ms, restarted by each change) and redraws only when the size differs — cmd/state_resume_wait.go:46, :187-195, :268-271 — read; TestRunResumeWait_Resize PASS. The start-of-wait gap is the finding above.
- Every screen change is a hand-off: `d`, Escape, a report and a resize each go through resumeRedraw → resumeHandOff to a fresh draw — cmd/state_resume_wait.go:330-400 — read
- Input already queued when the confirmation opens is dropped after the appearance probe and before the paint, and a failed drop puts the panel up rather than the confirmation — cmd/state_resume_draw.go:45-58, internal/tui/pane_appearance.go:69-75, cmd/tty_flush_darwin.go:11-13 — read; TestFlushTTYInput_RealPTY PASS
- On Enter, the pane leaves the alt screen before the marker is cleared; a clear that fails brings the panel back with a report; the hook is read from the store at the moment of the answer — cmd/state_resume_wait.go:315-326, :363-370 — read; TestResumeAnswerEnter_Order and TestResumeAnswerEnter_FailedClear PASS
- An answered pane gets no second shell: the recovery tail does nothing when the marker is gone, a failed read counts as pending, and it hands the shell over whether or not its clear lands, logging a failed clear as a WARN — cmd/state_resume_recover.go:35-60 — read; measured: "it closes the pane on the first exit after the resume" PASS in the lazy integration run
- The chain subcommands take the existing hydrate process role, so the role space does not grow — internal/log/process_role.go:27 — read
- Nothing triggers the panel: no production file under cmd/bootstrap or internal/restore changed, and the global hooks are untouched — `git diff --stat 34c342f90..df5297eb9 -- internal/restore cmd/bootstrap` → test files only
- The panel is not answered by being ignored and is offered again after a reboot — measured: "it offers the panel again on the reboot after the one that drew it" PASS
- The panes beside a waiting pane stay usable and captured — measured: "it leaves the pane beside it live" and "it goes on capturing the pane beside it" PASS
- Linters are clean over the change-set — measured: `go vet ./...` exit 0, `go vet -tags integration ./...` exit 0, `gofmt -l $(git ls-files '*.go')` no output, `golangci-lint run` 0 issues
