# Analysis Tasks: lazy-resume-on-attach (Cycle 11)

## Task 1: A Discard Removes Only a Registration the User Confirmed on Screen
severity: medium
sources: standards

**Problem**: There are two routes by which a confirmed discard deletes a user-authored `on-resume` command that the user never agreed to remove on screen. Either way the user finds out at the next reboot, when the pane comes back with no hook, and the command survives only in portal.log's `op=discard` line.

1. **A large paste carries `d`…`y` through both screens.** The confirmation's input drop is a single flush. `ResolvePaneTheme` runs it once, after the appearance query (`internal/tui/pane_appearance.go:58-67`). The drop is `dropStdinInputQueue` (`cmd/state_resume_draw.go:120-125`), which calls `flushTTYInput` (`cmd/tty_flush_darwin.go:11-13`). One flush empties only what the kernel input queue holds at that instant, about 2 KB on this machine (measured at 2,046 bytes). tmux holds the rest of a larger paste and writes it as soon as the flush frees the queue, before the waiter reads anything. The waiter then reads it as keystrokes on the confirmation, and the paste's first `y` confirms the discard.
   - The standards pass reproduced this on the real chain, in a disposable `-L ptl-*` socket with an isolated HOME, XDG and hooks file. A 2,565-byte multi-line paste, with a `d` first and `yes` at the end, emptied hooks.json, cleared `@portal-resume-pending` and exec'd the shell about 20 ms after the confirmation's draw started. At 4,005 bytes, zsh then ran `es`, the leftover of `yes`.
   - A 4,004-byte single-line burst left the confirmation standing, because the canonical line limit discards an unbroken line while the draw runs. That is why the existing single-line coverage passes.
   - The specification says "Input already in flight when the confirmation opened is dropped rather than read as agreement". It names the failure this prevents: "destroying the only copy of a user-authored command with nobody seeing either screen".
   - The comment on `resumeOpenDiscardConfirm` (`cmd/state_resume_wait.go:344-349`) states the same wrong assumption: that the rest of the burst "is still queued on the tty".
2. **A replacement registration is deleted in place of the one shown.** `resumeAnswerDiscard` (`cmd/state_resume_wait.go:359-362`) goes through `discardResumeRegistration` (`:435-443`) to `Store.Discard` and `removeEntry` (`internal/hooks/store.go:227-266`). `removeEntry` deletes whatever `on-resume` value the key holds and never compares it with `cfg.Command`, the command both screens showed.
   - The entry can be rewritten while the pane waits, by a hand edit or by `hook set` run with `$TMUX_PANE` aimed at the pane. A `y` given to command X then deletes command Y.
   - The specification lists an entry "replaced by a re-registration" among those that have "already gone", which makes this a discard that finds nothing to remove.

**Solution**: Two parts. Each is derived from the specification.

1. **The drop covers the whole burst, not one snapshot of the queue.** This is derived from the rule that only a keystroke arriving after the confirmation is on screen can confirm it.
   - **What stays.** Phase 3's ordering stands: `ResolvePaneTheme` still runs the drop once, after the appearance query. `dropStdinInputQueue` stays the only caller of `flushTTYInput`.
   - **What changes.** The drop flushes, then waits for the pane's input to stay quiet for a settle window, flushing again whenever input arrives. It stops when the input is quiet or when a bound elapses.
   - **The settle window is 50 ms**, the value of `resumeEscapeFollow`, which is the chain's existing measure of "no next byte is coming". tmux refills the queue within its own event loop once a flush frees it, which is far inside 50 ms.
   - **The bound is one second.** This is a judgment call: pty throughput clears any paste tmux holds well inside it, and a stream that outlasts it is not a burst that ends.
   - **Canonical processing is off while the drain runs.** The drain puts back the tty state it found, with signal generation still off, before it returns. It must, because in canonical mode a readiness check cannot see a partial line. Measured on this machine: on a canonical pty, select, poll and FIONREAD all report nothing for an unterminated 23-byte line, and report the whole line once its newline lands. A drain run in canonical mode would therefore read a stream that is mid-line as quiet.
   - **A bound that elapses while input is still arriving is a drop that could not be completed.** It goes through the draw's existing refused-drop path (`cmd/state_resume_draw.go:52-59`): the waiting panel comes up instead of the confirmation, with the report `can't clear pending input: input kept arriving` and the `clear pending input failed` WARN. That row is 46 cells, so it renders whole on the 52-cell card. This follows the draw's rule that the confirmation never goes up over input the drop could not discard.
   - **The comment on `resumeOpenDiscardConfirm` is reworded** to say the rest of the burst may still be arriving.
2. **The discard removes only the registration the confirmation named.** This is derived from the rule that a replaced entry is a discard that finds nothing to remove.
   - **The command is compared inside the store.** `Store.Discard` takes the command the confirmation showed. Under its existing lock and load, it removes the stored `on-resume` entry only when that entry's command equals the shown command exactly. The comparison happens inside the store's locked load-mutate-save, never in the waiter, so no write can land between the check and the removal.
   - **A differing command is nothing to remove.** It answers `false, nil` with no write and no breadcrumb, as a missing entry does. The marker clears, the pane drops to a shell, and the replacement stays in the store.
   - **Only the command is compared**, because it is what both screens named. A rewrite that changes only the registration's resume mode is still the registration the user agreed to discard.
   - **Everything else keeps its current outcome.** A missing entry is still nothing to remove, and a refused lock or read is still reported on the confirmation. `Remove` is unchanged.
   - **`Discard`'s doc comment is corrected.** At `internal/hooks/store.go:227-230` it says the method removes exactly what `Remove` would. It will state the condition instead.

**Outcome**: A burst of any size that carries a `d` cannot also confirm the discard: either the confirmation goes up after the burst has been dropped, or the waiting panel comes back saying why it could not be. A confirmed discard removes the registration both screens named, or nothing.

**Acceptance Criteria**:
- [ ] A pane waiting on a real tmux server is sent a multi-line paste larger than its input queue, with a `d` first and `yes` last, at the measured 2,565- and 4,005-byte sizes. Nothing in the paste answers the confirmation: hooks.json still holds the registration, `@portal-resume-pending` is still set, the pane shows the confirmation or the waiting panel carrying the drop's report, and no shell runs any part of the paste
- [ ] The same holds for a 4,004-byte single-line burst carrying a `d` and a `y`
- [ ] A burst whose tail is an unterminated line still arriving when the drain starts is drained, not read as quiet: none of its bytes reach the waiter behind the confirmation
- [ ] When nothing arrives after the `d`, the confirmation goes up once the pane's input has been quiet for 50 ms, and a `y` pressed once it is on screen discards
- [ ] A terminal background reply that lands after the appearance probe's deadline is still dropped with the rest of the input, and it neither opens nor cancels a screen
- [ ] When input is still arriving after one second, the waiting panel comes up instead of the confirmation. Its report row reads `can't clear pending input: input kept arriving` and renders whole on the card, and one `clear pending input failed` WARN is logged. The registration and the marker both stand, and `d` can be pressed again
- [ ] However the drain ends, the pane's tty is left as the drain found it: canonical processing on, signal generation off
- [ ] A pane waiting on command X whose entry still holds X: `y` removes the entry, logs `op=discard` with `value` X, clears the marker and drops the pane to a shell, as it does today
- [ ] A pane waiting on X whose entry was rewritten to Y while it waited, by `hook set` aimed at the pane or by a hand edit: `y` leaves hooks.json byte-for-byte unchanged with Y in place and logs no `op=discard` line. The marker clears and the pane drops to a shell
- [ ] A pane waiting on X whose entry was rewritten to X under a different resume mode: `y` removes the entry and logs `op=discard` with `value` X
- [ ] Every other outcome is unchanged. An entry that is already gone is still nothing to remove: the marker clears, the pane gets a shell, and nothing is written. An unavailable lock or an unreadable store is still reported on the confirmation, with the registration and the marker standing. A flush the terminal refuses is still reported on the waiting panel in the operating system's words. `portal hook rm` still removes whatever command the key holds

**Do**:
- The drain is what `dropStdinInputQueue` (`cmd/state_resume_draw.go:120-125`) does, and `dropStdinInputQueue` stays the only caller of `flushTTYInput` (`cmd/tty_flush_darwin.go`, `cmd/tty_flush_linux.go`). `ResolvePaneTheme` (`internal/tui/pane_appearance.go:58-68`) still runs the drop once, after the appearance query
- Flush, then wait for the pane's input to stay quiet for 50 ms, which is the value of `resumeEscapeFollow` (`cmd/state_resume_wait.go:53`). Flush again whenever input arrives. Stop when the input is quiet or once one second has elapsed
- Canonical processing is off for the whole drain. Before it returns, the drain puts back the tty state it found, with signal generation still off
- A bound that elapses while input is still arriving returns an error that goes through the draw's existing refused-drop path (`cmd/state_resume_draw.go:52-59`). The error's cause reads `input kept arriving`, so `resumeDropRefusal` renders the row above and the existing `clear pending input failed` WARN fires
- Reword the comment on `resumeOpenDiscardConfirm` (`cmd/state_resume_wait.go:344-345`) to say the rest of the burst may still be arriving
- `Store.Discard` (`internal/hooks/store.go:231`) takes the command the confirmation showed. That is the waiter's `cfg.Command`, carried through `discardResumeRegistration` (`cmd/state_resume_wait.go:435-443`). The store removes the stored `on-resume` entry only when its command equals that command exactly
- The comparison runs inside the store's existing locked load-mutate-save, never in the waiter. A differing command answers `false, nil` with no write and no breadcrumb. Only the command is compared, never the resume mode. `Remove` is unchanged
- Correct `Discard`'s doc comment (`internal/hooks/store.go:227-230`) to state the condition, replacing its claim that it removes exactly what `Remove` would
- This is a behaviour change, so the executor writes the tests that pin it. The multi-line paste case delivers the paste through `paste-buffer`, as the standards pass reproduced it, and runs the real chain: a built `portal` binary on a disposable tmux socket. It therefore carries the integration tag and sits beside the real-chain panel suite in `internal/restore/lazy_resume_panel_integration_test.go`
