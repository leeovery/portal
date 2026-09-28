# Review Tasks: Lazy Resume On Attach (Cycle 1)

## Task 1: Pending re-file never overwrites an existing token-named transcript
severity: high
sources: 2-6-1

**Problem**: When the saver first sees a waiting pane, it renames the pane's positional scrollback file to `scrollback/pane-<token>.bin` (`renameStoredScrollback`, `internal/state/scrollback.go`). The rename moves whatever the positional file holds at that moment. It assumes the file still holds the waiting pane's bytes, and nothing checks that. The case that breaks this is displacement. One tick re-files the waiting pane (`cmd/state_daemon.go:249`). The same tick's capture loop then writes the pane that took over its old address into the vacated positional file (`:290`) before the commit (`:303`). If anything then re-files from an index that still names that positional path, the displaced pane's capture is renamed over the waiting pane's token file. Four routes do this: the loop's `ctx.Done()` early return followed by the shutdown flush, a `Commit` error that leaves `PrevIndex` unadvanced, a crash before the commit (the restart reads the on-disk `sessions.json`), and `commit-now` reading the on-disk index mid-tick. When it happens, the waiting pane's transcript is replaced by another pane's history and no copy survives anywhere. The next reboot replays the wrong history. This is the permanent loss the re-file exists to prevent, reached through the retry path. No current test covers this sequence.

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

## Task 2: Capped command row shows only the whitespace the command has
severity: medium
sources: 3-7-1

**Problem**: On the waiting card and the discard confirmation, a command too long for the row cap is cut on its last row. `wrapCapped` builds that row by joining the leftover rows with a single space, whether or not the command has a space there. `ansi.Wrap` also breaks lines straight after a hyphen, so the capped row can show a space the command does not contain. In the case the review traced, the row reads `--cwd /Users/leeovery/Code/portal/internal/tui -- o…` where the command has `--output-format`. The user sees a standalone `--` argument the registered command does not have, and resumes or discards based on that misreading. No test catches it: `assertRowsCarryCommand` compares text with spaces stripped, and the edge-space check only looks at row edges.

**Solution**: `wrappedLines` records, at each row boundary, the whitespace the source had there. At a hyphen break or a mid-word break that is nothing. `wrapCapped` then joins the rows past the cap with that recorded whitespace instead of a fixed space, so the capped row matches the source exactly up to the ellipsis. This is a judgment call between two technical forms with the same result. The walk is the one place that knows what whitespace it trimmed (`droppedGap` and `rowAndCarry`'s edge trims), so recording it there keeps rows and separators in a single pass. The alternative, rebuilding the remainder from source offsets, needs a second mapping between rows and source that both the re-flow and the oversize-grapheme drop would have to keep intact. The fix is in the shared helpers, so the theme panel's message (the other `wrapCapped` caller, `internal/tui/theme_panel_message.go:131`) gets the same correction. The rows `notice_band.go:122` takes from `wrappedLines` are unchanged.

Conditions the task must honour:
- Keep the single `ansi.Wrap` call in `text_wrap.go` (`TestWrapsThroughOneImplementation`). A second wrap of the remainder trips that guard.
- The traced case, `resumeCommandLines("claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 " + resumePartsLongCommand, resumeCardContentWidth)`, is pinned as a row case. It asserts the capped row's text matches the source up to the ellipsis.

## Task 3: Waiter answers resizes it missed and stops swallowing keys after Alt-[ and Alt-]
severity: medium
sources: 4-6-1, 1-what-this-feature-changes-2-resume-modes-3-storing-setting-and-reading-the-mode-4-the-waiting-pane-1, 5-3-1, 5-7-1

**Problem**: Three linked defects in the waiter and the test harness it shares:

1. **Missed resizes.** The waiter compares the pane's size with the size the panel was drawn at only after a SIGWINCH starts its settle window. A SIGWINCH that arrives after the draw reads the size but before the waiter installs its watch is lost. That gap covers the appearance probe, the render, the exec and the waiter's start-up. The panel then stays drawn for a size that no longer exists: off-centre, or clipped with its key hints cut off, until some later unrelated resize. A client attaching while a pane is still drawing after a reboot is one way to hit this.
2. **Alt chords swallow keys.** Alt-[, Alt-Shift-O and Alt-] arrive as `ESC [`, `ESC O` and `ESC ]`, and the waiter treats them as the start of an escape sequence (`consumeResumeSequence`), which reads until the sequence ends. The next 14 bytes are swallowed (62 after Alt-]), Enter included. On the confirmation, pressing Escape repeatedly cannot back out, and resizes are not handled while this read blocks. The earlier task promised that an Alt chord costs nothing beyond itself.
3. **Flaky harness.** The `swallowed` and `answered` screen helpers race a real 50ms follow window against bytes that are already queued, so the ESC-led cases flake on a loaded machine.

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

## Task 4: Pane appearance probe reads the terminal's reply through a bounded read macOS honours
severity: high
sources: 5-the-resume-panel-6-answering-the-panel-1, test-surface-1, 5-the-resume-panel-6-answering-the-panel-2, 3-5-1

**Problem**: The resume panel asks the terminal for its background colour (OSC 11) so it can paint the light or dark theme. It reads the reply from a fresh open of the literal `/dev/tty` (`paneTTYPath`). On macOS that descriptor refuses a read deadline; this was reproduced ("file type does not support deadline"). So `detect()` resolves dark on every production probe. By then it has already written the query, so the terminal's reply is echoed as literal `^[]11;rgb:…` text across the painted panel; this was measured on the panel's eighth row. The result: on macOS, with the shipped light/dark default, every waiting panel and discard confirmation paints dark whatever the terminal answers. That breaks the rule that a redraw with a client present follows the terminal in front of it, and every redraw with an answering terminal prints the raw reply over the card. The tests currently prove the wrong path:
- The real-terminal test opens the pty's own device node, which does accept a deadline, so it passes.
- The `paneTTYPath == "/dev/tty"` pin and `TestPaneAppearance_OpensNothingButThePanesTerminal` both lock in the broken path.
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
