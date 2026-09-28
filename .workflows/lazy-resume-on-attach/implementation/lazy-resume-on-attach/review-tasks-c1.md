# Review Tasks: Lazy Resume On Attach (Cycle 1)

## Task 1: Pending re-file never overwrites an existing token-named transcript
severity: high
sources: 2-6-1

**Problem**: When the saver first sees a waiting pane, it renames the pane's positional scrollback file to `scrollback/pane-<token>.bin` (`renameStoredScrollback`, `internal/state/scrollback.go`). The rename moves whatever the positional file holds at that moment. It assumes the file still holds the waiting pane's bytes, and nothing checks that. The case that breaks this is displacement. One tick re-files the waiting pane (`cmd/state_daemon.go:249`). The same tick's capture loop then writes the pane that took over its old address into the vacated positional file (`:290`) before the commit (`:303`). If anything then re-files from an index that still names that positional path, the displaced pane's capture is renamed over the waiting pane's token file. Four routes do this: the loop's `ctx.Done()` early return (`cmd/state_daemon.go:267-271`) followed by the shutdown flush (`:363`), a `Commit` error that leaves `PrevIndex` unadvanced (`:303-307`), a crash before the commit (the restart reads the on-disk `sessions.json`), and `commit-now` reading the on-disk index mid-tick (`cmd/state_commit_now.go:118`). When it happens, the waiting pane's transcript is replaced by another pane's history and no copy survives anywhere. The next reboot replays the wrong history. This is the permanent loss the re-file exists to prevent, reached through the retry path. No current test covers this sequence.

**Solution**: Make the re-file no-clobber. It moves the stored positional file onto `pane-<token>.bin` only when no file already has that name, and it does the check and the move as one atomic step, so a concurrent `commit-now` cannot slip in between. When the token file already exists, the pane adopts the token path and the positional file is left alone for whichever pane now holds that address. The existing rule for a missing source (adopt) is unchanged.

Of the three remedies the review named, this is the only one that covers all four routes using no state beyond the file itself:
- Suppressing positional writes until the re-file commits keeps its state in the daemon's memory. A crash loses that state and a concurrent `commit-now` never sees it. It also withholds the displaced pane's own scrollback from a shutdown flush.
- Advancing the index when the re-file happens covers only the in-process routes.

The no-clobber move has one residual failure: it can adopt a token file whose reclaim never happened. That requires the housekeeping pass to fail to delete the file on every writing commit between the end of one wait and the same pane's next wait. Even then, the pane gets back its own earlier transcript, not another pane's.

Conditions the task must honour:
- The real-tmux pinning test calls `portaltest.IsolateStateForTest` and `RegisterStateDirTeardownGuard` before `tmuxtest.New` (`TestTeardownGuardCoversEveryServerHostingFixture`).
- Any new `CommitNowDeps` field gets its seam case (`TestResolveCommitNowDepsMergeConvention`).
- The pinning sequence is the one the review named: stage the first waiting tick with another pane on the vacated address, cut the tick short after that pane's write (a `Commit` error or a context cancel), run a second tick or the flush, and assert the token file still holds the frozen bytes.

**Outcome**: A waiting pane's token-named file holds only that pane's bytes on every route, whether or not the tick that first re-filed it committed.

**Acceptance Criteria**:
- [ ] A waiting pane's first waiting tick finds it displaced — a live pane now sits at the address it was filed under — and is cancelled after that live pane's capture is written and before the commit. The shutdown flush that follows leaves `scrollback/pane-<token>.bin` holding the waiting pane's frozen bytes and the positional file holding the live pane's capture, and commits each pane's record naming its own file.
- [ ] The same displaced first tick instead fails its commit. The next tick leaves both files' bytes as the failed tick left them, and commits the waiting pane's record naming the token path and the live pane's naming the positional path.
- [ ] `portal state commit-now` runs while the on-disk `sessions.json` still names the waiting pane's positional path, with the token-named file already holding its frozen bytes and the positional file holding another pane's capture. It commits the waiting pane's record naming the token path and changes neither file's bytes. A daemon starting from that same on-disk index does the same on its first tick.
- [ ] A re-file that finds a file already named `pane-<token>.bin` points the record at that path, leaves that file's bytes unchanged, and leaves the positional file where it is with its bytes unchanged.
- [ ] Two re-files of one pane from indexes that both name its positional path — the daemon's and `commit-now`'s, in either order — leave the token-named file holding the bytes the first of them moved; the second overwrites nothing.
- [ ] The rest of the re-file's rules hold as today: with no token-named file present the bytes move onto it, the positional name holds nothing afterwards, and the dedup entry for that name is gone; a missing source, or a record naming no file, adopts the token path; a rename failing for any reason other than a missing source or an existing token-named file leaves the record on its stored path and its dedup entry in place, and emits one WARN carrying only `pane_key`, `path` and `error`.

**Do**:
- The re-file lives in `internal/state/scrollback.go` (`refilePendingPane`, `renameStoredScrollback`). It moves the stored positional file onto `pane-<token>.bin` only when no file already has that name, with the check and the move as one atomic step. An existing token-named file is adopted and the positional file left alone; a missing source still adopts.
- `state.CaptureAndRefile` stays the one entry point the daemon (`cmd/state_daemon.go:249`), `commit-now` (`cmd/state_commit_now.go:121`) and the lazy integration fixture take.
- Pin the Solution's sequence at tick level beside `TestCaptureAndCommit_RefilesResumePendingScrollback` (`cmd/state_daemon_resume_pending_test.go`), the commit-now route beside the existing commit-now re-file case (`cmd/state_commit_now_test.go`), and the adopt rule in `TestRefilePendingScrollback` (`internal/state/scrollback_test.go`).
- `TestRefilePendingScrollback`'s rename-failure case stages its failure by putting a directory at the token path (`internal/state/scrollback_test.go:607-649`). Under the no-clobber rule an occupied token path reads as an existing token-named file rather than a rename failure, so that case stages its failure another way and keeps the WARN rule pinned.

## Task 2: Capped command row shows only the whitespace the command has
severity: medium
sources: 3-7-1

**Problem**: On the waiting card and the discard confirmation, a command too long for the row cap is cut on its last row. `wrapCapped` (`internal/tui/text_wrap.go:17`) builds that row by joining the leftover rows with a single space, whether or not the command has a space there. `ansi.Wrap` also breaks lines straight after a hyphen, so the capped row can show a space the command does not contain. In the case the review traced, the row reads `--cwd /Users/leeovery/Code/portal/internal/tui -- o…` where the command has `--output-format`. The user sees a standalone `--` argument the registered command does not have, and resumes or discards based on that misreading. No test catches it: `assertRowsCarryCommand` (`internal/tui/resume_panel_parts_test.go:69-80`) compares text with spaces stripped, and the edge-space check only looks at row edges.

**Solution**: `wrappedLines` records, at each row boundary, the whitespace the source had there. At a hyphen break or a mid-word break that is nothing. `wrapCapped` then joins the rows past the cap with that recorded whitespace instead of a fixed space, so the capped row matches the source exactly up to the ellipsis. This is a judgment call between two technical forms with the same result. The walk is the one place that knows what whitespace it trimmed (`droppedGap` and `rowAndCarry`'s edge trims), so recording it there keeps rows and separators in a single pass. The alternative, rebuilding the remainder from source offsets, needs a second mapping between rows and source that both the re-flow and the oversize-grapheme drop would have to keep intact. The fix is in the shared helpers, so the theme panel's message (the other `wrapCapped` caller, `internal/tui/theme_panel_message.go:131`) gets the same correction. The rows `notice_band.go:122` takes from `wrappedLines` are unchanged.

Conditions the task must honour:
- Keep the single `ansi.Wrap` call in `text_wrap.go` (`TestWrapsThroughOneImplementation`). A second wrap of the remainder trips that guard.
- The traced case, `resumeCommandLines("claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 " + resumePartsLongCommand, resumeCardContentWidth)`, is pinned as a row case. It asserts the capped row's text matches the source up to the ellipsis.

**Outcome**: A row cut at its cap on the waiting card, the discard confirmation or the theme panel's message shows the source's own text up to the `…`, with no space the source does not have.

**Acceptance Criteria**:
- [ ] `resumeCommandLines("claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 " + resumePartsLongCommand, resumeCardContentWidth)` returns `claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7` twice and then `--cwd /Users/leeovery/Code/portal/internal/tui --ou…`, which reads the command's `--output-format` with no space inside it.
- [ ] For every command the suite drives, at every width from 1 to 80, the capped row with its `…` removed is the command's own text from where that row starts, whitespace included. Every row is still no wider than the width, none begins or ends with a space, and no render runs past three rows.
- [ ] Text past the cap is still marked `…` when none of it fits a row: a double-width command at width 1 still renders `{"", "", "…"}`, and the widths 1–4 tables read as they stand.
- [ ] A theme panel message that runs past its two rows, with a hyphen or mid-word break inside its capped row, shows the message's own text up to its ellipsis.
- [ ] Rows within the cap are unchanged everywhere: the notice band's rows, the shipped-copy subtests of `TestNoticeBand_RowsFitTheBand` and `TestPanelMessage_RowsFitTheInnerWidth`, and the existing row pins in `TestResumeCommandRows_Geometry` read as they stand.
- [ ] `internal/tui` still calls `ansi.Wrap` from exactly one site (`TestWrapsThroughOneImplementation`).

**Do**:
- In `internal/tui/text_wrap.go`, `wrappedLines`' walk records at each row boundary the whitespace the source had there (none at a hyphen or mid-word break), and `wrapCapped` joins the rows past the cap with that recorded whitespace instead of a fixed space. The rows `notice_band.go:122` takes from `wrappedLines` do not change.
- At width 1 the double-width command's rows past the cap are all empty rows whose graphemes were dropped as too wide (`internal/tui/resume_panel_parts_test.go:319-354`), so a join of those rows with their recorded whitespace alone is empty. The `…` that pin expects must survive the change.
- Pin the traced case as a row case in `internal/tui/resume_panel_parts_test.go`.

## Task 3: Waiter answers resizes it missed and stops swallowing keys after Alt-[ and Alt-]
severity: medium
sources: 4-6-1, 1-what-this-feature-changes-2-resume-modes-3-storing-setting-and-reading-the-mode-4-the-waiting-pane-1, 5-3-1, 5-7-1

**Problem**: Three linked defects in the waiter and the test harness it shares:

1. **Missed resizes.** The waiter compares the pane's size with the size the panel was drawn at only after a SIGWINCH starts its settle window (the settled comparison at `cmd/state_resume_wait.go:190-195` is its only one). A SIGWINCH that arrives after the draw reads the size (`cmd/state_resume_draw.go:43`) but before the waiter installs its watch (`winchSignals()`, `cmd/state_resume_wait.go:479`) is lost. That gap covers the appearance probe, the render, the exec and the waiter's start-up. The panel then stays drawn for a size that no longer exists: off-centre, or clipped with its key hints cut off, until some later unrelated resize. A client attaching while a pane is still drawing after a reboot is one way to hit this.
2. **Alt chords swallow keys.** Alt-[, Alt-Shift-O and Alt-] arrive as `ESC [`, `ESC O` and `ESC ]`, and the waiter treats them as the start of an escape sequence (`consumeResumeSequence`, `cmd/state_resume_wait.go:225-246`), which reads until the sequence ends through a blocking `reader.next()`. The next 14 bytes are swallowed (62 after Alt-]), Enter included. On the confirmation, pressing Escape repeatedly cannot back out, and resizes are not handled while this read blocks. The earlier task promised that an Alt chord costs nothing beyond itself.
3. **Flaky harness.** The `swallowed` and `answered` screen helpers (`cmd/state_resume_screens_test.go:35`, `:75`) race a real 50ms follow window against bytes that are already queued, because `newResumeWaitConfig`'s default `Settle` is `time.After` (`cmd/state_resume_wait_test.go:82`). So the ESC-led cases flake on a loaded machine.

**Solution**:

- **Startup size check.** Once its SIGWINCH watch is installed, the waiter reads the pane size once. If the read succeeds, the drawn size is positive and the two differ, it starts the same settle window a SIGWINCH starts. The missed resize then goes through the loop's existing settle-then-compare path, and a drag still in progress collapses into one redraw. The review framed this two ways: hand off at once, or start the settle. Both end in `resumeRedraw`, and the settle is the loop's existing rule for a resize. If the read fails or the drawn size is not positive, the waiter stays put, so a size read that keeps failing cannot bounce the pane between draws.
- **Bounded sequence reads.** Every byte after a CSI, SS3 or OSC introducer is read under the follow window. If the window runs out, the sequence ends and its outstanding read goes back to the screen's keys. A sequence that arrives in one write is still swallowed to its final byte, terminator or cap. The alternative, handling SIGWINCH while a sequence is read, would still swallow the 14 or 62 keys, so it does not keep the promise. This form also limits the resize gap: the loop is back within one window, and the buffered SIGWINCH channel holds a resize in the meantime. This deliberately changes the earlier task's rule that such sequences are "swallowed exactly as before" for a sequence whose next byte does not arrive within the window.
- **Harness.** `swallowed` and `answered` get a settle that never fires. The between-byte sequence cases get a harness that fires the follow window.

Conditions the task must honour:
- Redraw only through `resumeRedraw` (`TestExecSeamsAreCalledOnlyByTheHandOffHelpers`).
- Use `signal.Notify` on SIGWINCH only, never `NotifyContext` (`TestRunResumeWait_InstallsNoHangupTerminateOrInterruptHandler`).
- No `internal/theme`, `internal/tui` or `internal/prefs` import in `state_resume_wait.go`.
- Consume sequence bytes through the reader or the follow window. Never call `flushTTYInput` (`TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace`), and never set `DropInput` outside `resumeOpenDiscardConfirm` (`TestResumeDropInput_HandOffs`).
- `newResumeWaitConfig` gains a `Size` default that answers the payload's own drawn size, not a fixed 80x24 (`reportedPayload` is 100x30). Without it, every `swallowed`/`answered` case nil-panics.
- `resizedSize` and the per-case sizes at `state_resume_wait_resize_test.go:171` and `:268` answer the drawn size until the resize is delivered.
- Pin two cases: a size mismatch at startup with no SIGWINCH (exactly one hand-off to `resume-draw`), and a failed startup read (no hand-off).
- Once the startup check exists, re-check the premise that no resize settle can start in the nil-settle helpers.

**Outcome**: A resize is answered whenever it lands, and an Alt chord costs the user nothing beyond the chord: the next key acts on whichever screen is up.

**Acceptance Criteria**:
- [ ] A waiter starts over a pane whose size already differs from the size it was drawn at, and no SIGWINCH is delivered. Once the settle window elapses it hands off exactly once, to a `resume-draw` argv carrying its payload unchanged.
- [ ] A size change arriving while that startup window is open restarts it, so a drag still in progress costs one redraw; a key pressed inside it acts at once, as it does inside any settle window.
- [ ] A waiter whose startup size read fails, or which was drawn at a non-positive size, starts no settle window and hands nothing off; it goes on reading and a key the screen offers still acts. A size read that fails when a SIGWINCH's window settles still redraws, as today.
- [ ] A waiter started at the size it was drawn at arms nothing and hands nothing off.
- [ ] On the waiting panel, `ESC [`, `ESC O` and `ESC ]`, each followed by no byte within the follow window and then Enter, resume the pane on that Enter; the chord itself acts on nothing.
- [ ] On the discard confirmation, `ESC ]` followed by no byte within the follow window and then a lone Escape backs out to the waiting panel with the store and the marker untouched; `ESC [` followed after the window by `y` confirms the discard.
- [ ] A resize delivered after `ESC [`, with no further key pressed, hands the pane to a fresh draw once the resize settles.
- [ ] A CSI, SS3 or OSC sequence that arrives in one write is still swallowed to its final byte, terminator or cap on both screens: the arrow and delete keys, the OSC 11 reply in BEL and ST forms, cap-length sequences and sequences whose final byte is an acting key act on nothing, and the key after them acts.
- [ ] At most one read is outstanding at any moment, including across a sequence the window cut short, so input the loop did not dispatch stays queued for the next process image.
- [ ] Every `swallowed` and `answered` case reaches its verdict with a follow window that never fires, so no queued byte is raced against a real clock.

**Do**:
- In `cmd/state_resume_wait.go`, once the SIGWINCH watch is installed, read the pane size once. On a successful read that differs from a positive drawn size, start the settle window a SIGWINCH starts (`resumeResizeSettle` through `cfg.Settle`). The settled comparison and `resumeRedraw` are the loop's existing path from there.
- Read every byte after a CSI, SS3 or OSC introducer (`consumeResumeSequence`) under the follow window (`resumeEscapeFollow`). A window that runs out ends the sequence and hands its outstanding read back to the loop, the way `resolveResumeEscape` already does for the byte after the ESC.
- Tests live in `cmd/state_resume_wait_resize_test.go` (the two startup cases the Solution names, and the size stubs answering the drawn size until the resize is delivered), `cmd/state_resume_screens_test.go` (the `swallowed`/`answered` settle, and the between-byte cases, through a harness that hands the follow window to the test as the existing `pipedKeysHarness` does), and `cmd/state_resume_wait_test.go` (`newResumeWaitConfig`'s `Size` default).

## Task 4: Pane appearance probe reads the terminal's reply through a bounded read macOS honours
severity: high
sources: 5-the-resume-panel-6-answering-the-panel-1, test-surface-1, 5-the-resume-panel-6-answering-the-panel-2, 3-5-1

**Problem**: The resume panel asks the terminal for its background colour (OSC 11) so it can paint the light or dark theme. It reads the reply from a fresh open of the literal `/dev/tty` (`paneTTYPath`, `internal/tui/pane_appearance.go:18`, opened at `:48`). On macOS that descriptor refuses a read deadline; this was reproduced ("file type does not support deadline"). So `detect()` resolves dark at its deadline check (`:108-109`) on every production probe. By then it has already written the query (`:103`), so the terminal's reply is echoed as literal `^[]11;rgb:…` text across the painted panel; this was measured on the panel's eighth row. The result: on macOS, with the shipped light/dark default, every waiting panel and discard confirmation paints dark whatever the terminal answers. That breaks the rule that a redraw with a client present follows the terminal in front of it, and every redraw with an answering terminal prints the raw reply over the card. The tests currently prove the wrong path:
- The real-terminal test (`internal/tui/pane_appearance_realtty_test.go:21`) opens the pty's own device node, which does accept a deadline, so it passes.
- The `paneTTYPath == "/dev/tty"` pin (`internal/tui/pane_appearance_test.go:597-601`) and `TestPaneAppearance_OpensNothingButThePanesTerminal` (`internal/tui/pane_appearance_guard_test.go:113-143`) both lock in the broken path.
- The deadline-bound assertion for the no-reply case (`pane_appearance_test.go:441`) can never fail, because it runs after the deadline has already passed.

**Solution**: The probe stops opening a second descriptor. It reads the reply from the descriptor it already checks is a terminal and switches to raw mode, the pane's stdin, using a select-bounded read that waits no longer than `appearanceDetectTimeout`.

This is a judgment call between three technical forms that give the same result:
- **Opening the pane's own device by name.** Measured to accept a deadline, but it needs a separate way to find the device name on each OS (`F_GETPATH` on macOS). The Linux branch could not be exercised on this machine, and there is no CI.
- **Carrying `#{pane_tty}` on the chain's command line.** This adds a tmux read and a new flag threaded through the draw, wait and recover commands.
- **A select-bounded read on stdin (chosen).** It opens nothing and is one code path on both release platforms through `golang.org/x/sys/unix`, already a dependency. It is also the route `muesli/cancelreader` (already in `go.mod`) takes for this exact descriptor on macOS, where kqueue cannot poll `/dev/tty`.

The query is written only once the bounded read is ready, so a path that cannot read writes nothing and leaves no reply to echo. The test fake records the bound at the moment the read is armed, and the test asserts that bound is no more than the probe timeout. This replaces the assertion that could never fail.

Conditions the task must honour:
- Amend `TestPaneAppearance_OpensNothingButThePanesTerminal` and the `TestConstruction_ReadsNoThemesDirectory` `OpenFile` exemption deliberately, in the same change, keeping their intent: `internal/tui` opens nothing but the pane's own terminal (now nothing at all) and makes no `os.Stat` or `Getenv` call.
- `newPaneAppearanceProbe` keeps one caller.
- The timeout still comes from `appearanceDetectTimeout`.
- The real-terminal test drives the production read from a `Setsid`/`Setctty` child whose controlling terminal is the pty, as `cmd/tty_signals_pty_test.go:44` does. It builds no portal binary in the unit lane (`TestBuildHelpersStayInTheIntegrationLane`), and it is what proves this form on macOS.

**Outcome**: On macOS, a panel drawn with a client attached paints the member the terminal's background calls for, and no reply bytes ever land on the canvas.

**Acceptance Criteria**:
- [ ] From a process whose controlling terminal is a pty, the production probe under an adaptive pair writes the background-colour query once. Answered with a light background it resolves the light member; answered with a dark background, the dark member.
- [ ] A reply that arrives within the timeout is consumed by the probe: nothing of it is echoed back to the terminal.
- [ ] A terminal that never answers resolves dark no sooner than the probe's timeout, and the bound the read was armed with, recorded when it was armed, is no more than that timeout: a probe arming a bound ten times the timeout fails.
- [ ] A path that cannot arm the bounded read writes nothing to the terminal and resolves dark without blocking.
- [ ] The existing outcomes hold. A constant or zero nomination and a `NO_COLOR` draw write and read nothing. A stdin that is not a terminal or refuses raw mode resolves dark with nothing written. A read failure, a truncated or unparseable reply, and a reply after the deadline all resolve dark. Raw mode is restored on every path that entered it, and the input drop runs once, after the question has resolved.
- [ ] `internal/tui` opens no file: no `os.OpenFile` call remains, `TestConstruction_ReadsNoThemesDirectory` holds with no exemption, and the package makes no `os.Stat` or `os.Getenv` call.
- [ ] `newPaneAppearanceProbe` has exactly one caller, and the probe's timeout is `appearanceDetectTimeout` with no duration of its own (`TestPaneAppearance_ReachesTheProbeFromOnePlace`, `TestPaneAppearance_TakesThePickerTimeout`).
- [ ] The real-terminal test runs in the unit lane and builds no portal binary.

**Do**:
- In `internal/tui/pane_appearance.go`, the probe reads the reply from the pane's stdin, the descriptor it already checks is a terminal and puts into raw mode. It uses a select-bounded read through `golang.org/x/sys/unix` that waits no longer than `appearanceDetectTimeout`, and opens no descriptor, so the open path (`paneTTYPath`, `openTTYPath`) goes.
- The query is written only once the bounded read is ready.
- The test fake records the bound at the moment the read is armed, replacing the assertion at `internal/tui/pane_appearance_test.go:441`.
- In the same change, amend `TestPaneAppearance_OpensNothingButThePanesTerminal` (`internal/tui/pane_appearance_guard_test.go`), the `opensThePanesTerminal` exemption in `TestConstruction_ReadsNoThemesDirectory` (`internal/tui/nomination_test.go:229-231`), and the `paneTTYPath` subtest of `TestProductionPaneAppearanceProbe` (`internal/tui/pane_appearance_test.go:597-601`), keeping the intent the Solution names.
- Replace `TestProductionReader_BoundsAReadAgainstARealTerminal` (`internal/tui/pane_appearance_realtty_test.go`) with a test that drives the production read from a `Setsid`/`Setctty` child whose controlling terminal is the pty, as `foregroundOn` in `cmd/tty_signals_pty_test.go:39-60` starts its child.

## Task 5: Waiting panes preview their saved transcript
severity: medium
sources: review-report-c1.md S1 (2-6-2, 7-protecting-the-waiting-pane-s-saved-transcript-8-seeing-what-is-waiting-1)

**Problem**: After a reboot under the shipped lazy default, pressing Space in the picker on a session that holds a waiting pane shows `(no saved content)` for that pane for the whole wait, although its transcript is intact on disk. The preview resolves a pane's saved transcript from its live position (`state.ScrollbackFile(stateDir, paneKey)`, `internal/tui/preview_adapter.go:23`, key composed at `internal/tui/pagepreview.go:257`), while the re-file rule the 2026-09-21 corrigendum added moves every tokened waiting pane's bytes to `scrollback/pane-<token>.bin` at the first frozen tick, moved or not (`refilePendingPane`, `internal/state/scrollback.go`). Before this change-set the same unmoved pane previewed its transcript. The specification accepts a blank preview only "for a waiting pane that has been rearranged" (§7.2, `specification.md:312`; §10 at `:423` repeats that scope), and argued the acceptance from the artifact being narrow and lasting about a tick for any pane that moves. On the default path it is neither: it reaches every waiting pane — roughly the forty-one the specification counts after a reboot — for as long as each waits.

**Solution**: The preview reads a waiting pane's saved transcript from its token-named file, so Space on a session holding a waiting pane shows the transcript the panel is sitting over; a pane not yet re-filed shows its positional file as today. Settled by the user at the review's decision on this proposal, over accepting a blank preview for every waiting pane. Conditions from the review: carry the token and pending columns on the preview's single existing `list-panes` enumeration; fall back to the positional file through `TailScrollback`'s `(nil, nil)` not-found shape for a pane not yet re-filed; no `os.Stat` and no per-focus tmux read in `internal/tui` (`TestConstruction_ReadsNoThemesDirectory`, `TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads`); expect `tmux.ListWindowsAndPanesInSession`'s format and return shape (`internal/tmux/tmux.go:586-590`, `WindowGroup.PaneIndices []int`) and the `ScrollbackReader.Tail` seam every preview fake implements to change; pin an unmoved waiting pane previewing its saved bytes. Once the change lands, §7.2's accepted-consequence paragraph and §10's echo of it no longer hold and take a corrigendum: the preview resolves a waiting pane through its token, and no preview artifact is accepted for it.

**Outcome**: Space on a session holding a waiting pane shows that pane's saved transcript, whether or not the pane has moved.

**Acceptance Criteria**:
- [ ] A waiting pane that never moved has been re-filed under its token. Space on its session shows the pane's saved transcript, the bytes in its token-named file, not `(no saved content)`.
- [ ] A waiting pane that has moved to a different address since it was filed shows the same saved transcript.
- [ ] A waiting pane not yet re-filed, with no token-named file on disk, previews its positional file as today.
- [ ] A waiting pane carrying no token, and every pane that is not waiting, previews its positional file as today.
- [ ] Opening the preview still costs its one enumeration and each focus no tmux read: `TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads` holds, and `internal/tui` makes no `os.Stat` call (`TestConstruction_ReadsNoThemesDirectory`).
- [ ] The enumeration still returns every window and pane of the session in today's order, and now also carries each pane's token and whether it waits, from the same single `list-panes` call.

**Do**:
- Carry `#{@portal-pane-id}` and `#{@portal-resume-pending}` on `ListWindowsAndPanesInSession`'s existing `list-panes` (`internal/tmux/tmux.go:586-590`), composed from their single declarations (`state.PortalPaneIDOption` through `tmux.HookKeyFormat`, and `state.ResumePendingOption` with presence read through `state.ResumePendingSet`). Its return shape (`WindowGroup.PaneIndices []int`) changes to carry them.
- The preview resolves a waiting pane that carries a token through `state.PendingScrollbackFile(token)`, and falls back to the positional file (`state.ScrollbackFile`) on `TailScrollback`'s `(nil, nil)`. The `ScrollbackReader.Tail` seam (`internal/tui/preview_seams.go:16-18`) changes to carry what that needs.
- The set that moves with the two seams, measured: `rg -n '\) Tail\(' --type go` gives 15, the production adapter `internal/tui/preview_adapter.go:22` plus 14 fakes (13 in `internal/tui`, 1 in `internal/capture/fakes.go`). `rg -n '\) ListWindowsAndPanesInSession\(' --type go` gives 8, `internal/tmux/tmux.go:586` plus 7 fakes (6 in `internal/tui`, 1 in `internal/capture/fakes.go`). The `internal/tmux` tests calling the enumeration (`TestListWindowsAndPanesInSession`, the exact-target suites) move with its format.
- This task changes code and tests only. The §7.2/§10 corrigendum the Solution names is taken outside it.
