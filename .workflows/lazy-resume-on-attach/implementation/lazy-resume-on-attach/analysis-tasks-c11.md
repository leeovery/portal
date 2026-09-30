# Analysis Tasks: lazy-resume-on-attach (Cycle 11)

## Task 1: A Discard Removes Only a Registration the User Confirmed on Screen
severity: medium
sources: standards

**Problem**: There are two routes by which a confirmed discard deletes a user-authored `on-resume` command that the user never agreed to remove on screen. Either way the user finds out at the next reboot, when the pane comes back with no hook, and the command survives only in portal.log's `op=discard` line.

1. **A large paste carries `d`…`y` through both screens.** The confirmation's input drop is a single flush. `ResolvePaneTheme` runs it once, after the appearance query (`internal/tui/pane_appearance.go:58-67`). The drop is `dropStdinInputQueue` (`cmd/state_resume_draw.go:120-125`), which calls `flushTTYInput` (`cmd/tty_flush_darwin.go:11-13`). One flush empties only what the kernel input queue holds at that instant, about 2 KB on this machine. tmux holds the rest of a larger paste and writes it as soon as the flush frees the queue, before the waiter reads anything. The waiter then reads it as keystrokes on the confirmation, and the paste's first `y` confirms the discard.
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
