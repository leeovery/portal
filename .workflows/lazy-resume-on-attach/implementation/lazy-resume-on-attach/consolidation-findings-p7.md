# Consolidation Findings: lazy-resume-on-attach (Phase 7)

## Findings

### F1: The recovery tail turns only signal generation back on, so a waiter killed in raw mode hands the user's shell a raw terminal — while the backstop route runs `stty sane`
- **Class**: behaviour
- **Failure**: The phase gives a dead waiter's pane two routes to the user's shell, and they leave the terminal in different states:
  - the Go recovery tail (7-1) turns signal generation (ISIG) back on and nothing else;
  - the shell backstop (7-2) runs `stty sane`.

  **Why the tail's restore is not enough.** A waiter holds the pane's tty raw for the whole wait. `MakeRaw` clears ICANON, ECHO, IEXTEN, ICRNL, IXON and OPOST as well as ISIG, and only the waiter's deferred restore puts them back. A waiter ended by a signal never runs that restore:
  - `pkill portal` (SIGTERM) — a route the spec itself names, and it puts every waiting pane on the install through this at once;
  - an OOM or jetsam SIGKILL;
  - a SIGINT or SIGQUIT sent to the pane's process group, which 7-1's trap deliberately routes into the tail.

  A kill key cannot reach a raw waiter as a signal, because raw mode clears ISIG. So the exposure is external signals and crashes. The draw's appearance probe is a second raw window with the same exposure.

  **What the user gets.** In every one of these cases the tail clears the marker and execs the user's shell on a terminal with ISIG back on but still no echo, no canonical input, no CR→NL input mapping and no output post-processing. A line editor sets its own modes around the prompt, but the commands the user runs inherit the raw tty: output staircases and nothing they type into a program echoes. The spec says the tail "drops the pane to a usable prompt". The user instead sees a garbled pane that only `reset` fixes. The reviewer reproduced this in a disposable tmux server.

  **Why no test catches it.** The tail's tests assert only that the ISIG seam was called.
- **Evidence**:
  - `cmd/state_resume_recover.go:56` — the tail's only terminal step is `enableTTYSignalsOrLog`; `:97` wires it to `setStdinSignals`.
  - `cmd/state_resume_chain.go:165-169` — `enableTTYSignalsOrLog`.
  - `cmd/tty_signals.go:18-20` — `setTTYSignals` sets `ISIG` alone.
  - `cmd/tty_signals.go:22-29` — `updateLocalModes` already receives the whole `Termios`.
  - `cmd/tty_signals.go:35-37` — `setStdinSignals`.
  - `cmd/state_resume_wait.go:114-119` — `MakeRaw`, restored only by `defer cfg.restore()`.
  - `cmd/state_resume_wait.go:430-436` — only SIGWINCH is notified, so SIGTERM/SIGINT/SIGQUIT end the waiter without running defers.
  - `cmd/state_resume_wait.go:442-449` — `makeStdinRaw`.
  - `github.com/charmbracelet/x/term@v0.2.2/term_unix.go:29-31` — what `MakeRaw` clears.
  - `internal/tui/pane_appearance.go:97-101` and `:167-171` — the probe's raw window.
  - `cmd/state_hydrate.go:299-302` — the backstop's `stty sane`.
  - `cmd/state_resume_signals_test.go:302-341` — the tail's tests count seam calls only.
- **Proposed shape**:
  - Add a sibling of `setTTYSignals` in `cmd/tty_signals.go` that sets back what `MakeRaw` clears and a shell needs: Lflag `ICANON|ECHO|ECHOE|ECHOK|ISIG|IEXTEN`, Iflag `ICRNL|IXON`, Oflag `OPOST|ONLCR`. It goes through the same termios updater.
  - Wire it as the recovery tail's seam (`resumeRecoverConfig`, wired at `cmd/state_resume_recover.go:97`) in place of `setStdinSignals`.
  - Leave the Enter and confirmed-discard answers on `setStdinSignals`: they restore the tty the waiter found first, so ISIG is all they owe.
  - Keep the backstop's `stty sane`, which is a superset. Both routes then land a cooked tty.
  - Pin the fix in `cmd/tty_signals_pty_test.go`: make the pty raw with `term.MakeRaw`, skip the restore, apply the tail's restore, and assert that the cooked modes are back.
- **Bank**:
  - (7-1, reviewer) the recovery tail turns only ISIG back on, so a waiter killed in raw mode hands the user's shell a raw terminal.
  - (7-2, reviewer) the backstop runs `stty sane` while the Go tail only turns ISIG back on.

### F2: The backstop hands the pane a shell that is still on the panel's alternate screen, with the cursor hidden and the pending marker set
- **Class**: behaviour
- **Failure**: 7-2's backstop runs when the tail cannot start because the binary is gone from the path the chain baked. It goes straight to `stty sane; exec "${SHELL:-/bin/sh}"`. The Go tail takes two steps before its shell, and the backstop skips both.

  **Step 1 skipped: leaving the panel's screen.** Every draw enters the alternate screen and hides the cursor (`\x1b[?1049h\x1b[?25l`), and nothing on this route leaves it. The user's shell therefore starts on the alternate screen, over the stale card, with no visible cursor. The pane's transcript stays hidden in the primary buffer.

  **Step 2 skipped: clearing `@portal-resume-pending`.** The marker stays set on a live pane that is now an ordinary shell. As a result:
  - The saver skips this pane's scrollback for the rest of the pane's life. Everything the user does in it afterwards is lost at the next reboot, which restores the transcript from the moment the pane paused.
  - The picker's pending dot and doctor's pending count keep claiming a decision is waiting.
  - Nothing records any of this. A killed waiter leaves no line at all. A waiter that died on a failed redraw exec leaves only `exec handoff failed`, which says nothing about the marker.

  The spec says a marker wrongly left set "is reachable one way, and that way is recorded". This is a second way, and it is unrecorded.

  **How it is reached.** Any waiter that ends without an answer while the baked binary is missing:
  - a killed waiter;
  - a resize, `d`, Escape or report redraw. The redraw's exec of the missing path fails in `defaultExecShell`, which exits 1; the tail then exits 127 and the backstop runs.

  One trigger is a Linuxbrew upgrade: `os.Executable` resolves `/proc/self/exe` to the versioned Cellar path, which the upgrade's cleanup removes.

  **Why no test catches it.** The route's test pins only that `stty sane` ran and a shell started.
- **Evidence**:
  - `cmd/state_hydrate.go:299-302` — the backstop body.
  - `cmd/state_hydrate.go:304-309` — `parkedResumeChain`.
  - `cmd/state_hydrate.go:29` — the `hydrateAltScreenEnter` bytes.
  - `cmd/state_resume_draw.go:60` — every draw writes them.
  - `cmd/state_resume_recover.go:43-54` — the Go tail's reset preamble, then the marker clear with its WARN on failure. The backstop has neither.
  - The failed-redraw path: `cmd/state_resume_wait.go:404-407` (`resumeRedraw`) → `cmd/state_resume_chain.go:117-127` (`resumeHandOff`) → `cmd/state_hydrate.go:393-398` (`defaultExecShell` exits 1 on a failed exec).
  - `cmd/state_daemon.go:273` and `:322-327` — the saver skips a pending pane's scrollback.
  - `internal/state/markers.go:31`, `:111-113` and `internal/tmux/tmux.go:329-335` — the marker name and the `set-option -pu -t <pane> <name>` unset.
  - `cmd/state_resume_backstop_test.go:82-120` — asserts `stty sane` and a shell, nothing else.
  - `CLAUDE.md:190` — describes the parked chain as `/bin/sh -c '<draw argv>; <recover argv>'`, with neither the trap nor the backstop.
- **Proposed shape**: Give the backstop the Go tail's two steps, in the tail's order (reset, then clear), before `stty sane` and the shell. Compose them from the same Go values so the two routes cannot drift:
  - `parkedChainBackstop` takes the payload.
  - Reset: write `hydrateResetPreamble` with `printf '%s' ` + `shellquote.Single(hydrateResetPreamble)`.
  - Clear: `shellquote.Join([]string{"tmux", "set-option", "-pu", "-t", string(tmux.PaneIDTarget(p.Pane)), state.ResumePendingOption})` followed by ` 2>/dev/null`. The pane's environment carries the `TMUX` and `PATH` the helper's own tmux calls used.
  - Extend `TestParkedResumeChain_Backstop`'s unstartable rows with a stub `tmux`, and assert both the reset bytes and the recorded `set-option -pu -t %7 @portal-resume-pending`.
  - One gap remains: there is no WARN on this route, because no Portal binary is left to emit it.
  - Update CLAUDE.md's Resume hooks description of the parked chain in the same change.

## Comment Corrections

- internal/resumekeys/leaf_guard_test.go:11-14 — the closing clause restates the `AssertDepsWithin(…, nil, ForbiddingThirdParty(), …)` call beneath it
  OLD: // The act keys are read by the renderers and by the waiter, which must not
// import the rendering path, so the package can only ever depend on the
// standard library: an empty allowlist, taken across other modules as well as
// this one.
  NEW: // The act keys are read by the renderers and by the waiter, which must not
// import the rendering path, so the package can only ever depend on the
// standard library.

## Spec Defects

### S1: The keys a waiting pane swallows "that would ordinarily kill a foreground process" are named as Ctrl-C, Ctrl-D and Ctrl-Z
- **Claim**: §4.3, "Enter and `d` act; everything else is swallowed" (specification line 131): "the waiter swallows every byte but Enter and `d`, including the three that would ordinarily kill a foreground process: Ctrl-C, Ctrl-D, Ctrl-Z."
- **Observed**:
  - The keys a terminal turns into signals for its foreground process group are Ctrl-C (SIGINT), Ctrl-\ (SIGQUIT) and Ctrl-Z (SIGTSTP, which stops rather than kills).
  - Ctrl-D is VEOF: end-of-input to a canonical reader. It signals nothing.
  - The phase's kill-key work is built on the first set:
    - `clearTTYSignals` names Ctrl-C, Ctrl-\ and Ctrl-Z (`cmd/tty_signals.go:11-13`);
    - the parked chain traps INT and QUIT (`cmd/state_hydrate.go:290`);
    - the real-pty test drives exactly those three keys (`cmd/tty_signals_pty_test.go:21-25`, `:81-85`).
  - Ctrl-\, which kills with a core dump, is missing from the spec's list.
- **Read**: spec stale. The rule itself — every byte but Enter and `d` is swallowed — is right and covers all four keys. Only the enumeration is wrong: it should name Ctrl-C, Ctrl-\ and Ctrl-Z as the keys that would signal a foreground process. If Ctrl-D is kept, it should be described as the end-of-input key, which a raw reader receives as a byte.
