AGENT: standards
FINDINGS:
- FINDING: A resize that lands during a screen hand-off is lost, and the panel stays drawn at the old size
  SEVERITY: low
  FAILURE: Each draw reads the pane size at its start, then resolves the theme. For an adaptive pair this includes the appearance probe, which can wait up to its 50ms timeout. The draw then paints and execs the waiter, and the waiter arms its SIGWINCH notify only once its RunE has built the config. A size change that lands anywhere in that window reaches a process that is not watching for it, and it is dropped. The new waiter starts with the drawn width and height and never compares them with the pane's real size. It redraws only when a later SIGWINCH arrives. If the stream of size changes ends inside the window, no redraw ever happens. That can be the last step of a drag that paused long enough (150ms) to trigger a redraw, a layout change made while `d`, Escape or a report redraw is mid-hand-off, or an attach that resizes a pane still being drawn. What the user sees:
    - When the pane has grown: the card sits off-centre in a region of the old size, and the canvas does not fill the pane.
    - When the pane has shrunk: the card was centred for a larger pane, so part of it, possibly the key-hint footer, falls outside the visible area. The pane still swallows every other key, so the user is left without the hints that say which keys answer it.
    Either state lasts until the next resize.
  FILES: cmd/state_resume_wait.go:151-198, cmd/state_resume_wait.go:462-481, cmd/state_resume_draw.go:43-72
  DESCRIPTION: The spec says a resize is "the same handover run backwards: the waiter replaces itself with a fresh draw, which draws at the new width". It also says the redraw is taken once the size has settled — "one draw at the end of the stream is the whole of what it owes". Every screen change goes through a hand-off (draw, then exec of the waiter), and so does every resize redraw. So each waiting pane passes through this unwatched window many times over its life.
    The loop's settle state starts empty (`var settled <-chan time.Time`). The drawn size is consulted only in the settle arm, which only a SIGWINCH can reach. A change that lands before the notify is registered therefore leaves no trace. The resize tests drive `Winch` into an already-running loop, so this startup gap is not exercised. The gap is also why the spec's rule that a pane too small for the card must not hide its key hints can fail after a shrink.
  RECOMMENDATION: When the waiter starts, after the SIGWINCH notify is armed, read the pane size once. If a successful read differs from the payload's Width/Height, arm the settle window exactly as a SIGWINCH would, so the redraw goes through the existing path with its single-redraw-per-settle property intact. Treat a failed read at startup as no evidence and arm nothing. The settle-time comparison already reads a failure as "changed", so a pane whose size read always fails would otherwise redraw every 150ms forever.
COMMENT_CORRECTIONS:
- cmd/state_resume_recover.go:25-28 — the waiter was never the pane's process. The parked shell is the pane's process and the waiter is its child, which is exactly the premise the spec's 2026-09-22 and 2026-09-28 corrigenda corrected.
  OLD: // runResumeRecover is the tail of the chain a waiting pane parks: it runs when
// the waiter above it has stopped being the pane's process, and gives the pane a
// shell so tmux does not close it — and with it the session and the whole
// transcript of a pane that is the only one in it.
  NEW: // runResumeRecover is the tail of the chain a waiting pane parks: it runs once
// the waiter above it has ended, and gives the pane a shell so tmux does not
// close it — and with it the session and the whole transcript of a pane that is
// the only one in it.
- cmd/state_resume_chain.go:112-113 — the hand-off replaces the draw's or the waiter's own image, not the pane's process. The parked shell stays the pane's process through every hand-off.
  OLD: // resumeHandOff replaces the pane's process image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the
  NEW: // resumeHandOff replaces this process's image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the
SUMMARY: This is a full pass over all 231 files against the whole specification and its corrigenda, including the phase-8 hand-off consolidation. The implementation conforms on all of these:
- both stored registration shapes and byte preservation of entries a write did not name;
- `hook set --resume-mode` with its refusals, and the fifth column of `hook list`;
- the two informational doctor lines;
- the tolerant `prefs.json` default;
- mode resolution and marking the pane pending on all three hydrate tails;
- the draw/wait/recover chain, with its drop-after-probe, its kill-key handling and its backstop;
- the answer ordering and holding an answer when the marker will not clear;
- the capture freeze and the token-named re-file;
- both panel screens;
- the picker dots and the help legend.
One low-severity divergence clears the floor: a resize that lands in a screen hand-off is never redrawn. There are also two comment corrections, where comments still carry the "waiter is the pane's process" premise the spec corrected.
