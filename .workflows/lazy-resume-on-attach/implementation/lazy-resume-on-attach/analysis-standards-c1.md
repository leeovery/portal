AGENT: standards
FINDINGS:
- FINDING: A Ctrl-C or Ctrl-\ that lands during a screen hand-off closes the waiting pane instead of being swallowed
  SEVERITY: medium
  FAILURE: A waiting pane spends some tens of milliseconds moving between screens after `d`, after Escape on the confirmation, after a resize redraw, and after a report redraw. The tty is cooked for that window. A Ctrl-C (or Ctrl-\) pressed in it becomes SIGINT (or SIGQUIT) for the pane's whole foreground process group. The parked `/bin/sh` dies along with the draw, and the recovery tail never runs. Most sessions hold a single pane, and for those the session closes too. The next capture then drops the session from `sessions.json` along with its whole transcript. The user sees a session disappear from the picker after pressing a key on its panel.
  FILES: cmd/state_resume_wait.go:398-401, cmd/state_resume_wait.go:386-391, cmd/state_resume_draw.go:37-73, cmd/state_hydrate.go:264-275
  DESCRIPTION: The spec requires the waiting pane to swallow Ctrl-C, Ctrl-D and Ctrl-Z. It limits the damage from a waiter that dies by relying on the parked shell underneath, whose recovery tail "drops the pane to a usable prompt". It also states what happens without that tail: the pane closes, "and the next capture drops it from the saved set with its whole transcript".
    The swallow only holds while a `resume-wait` process has the tty in raw mode. Every screen change goes through `resumeRedraw`, which restores the cooked tty before it execs `resume-draw`. The draw then runs cooked (it is raw only inside the appearance probe), and the next waiter stays cooked until its own `MakeRaw`. Terminal signal generation (ISIG) is live for that whole window, and nothing in the window survives the resulting signal:
    - neither the draw nor the waiter handles SIGINT or SIGQUIT;
    - the parked chain is a bare `/bin/sh -c '<draw>; <recover>'`.
    Reproduced with macOS `/bin/sh -c 'sleep 3; echo RECOVER-RAN'` in its own process group. A group SIGINT or SIGQUIT kills the shell (rc -2 and -3) and the second command never runs. With `trap : INT QUIT;` in front, the same group signal leaves the tail running (rc 0, `RECOVER-RAN` printed). A caught trap resets to default in exec'd children, so the hook and the user's shell still get an ordinary Ctrl-C.
    The plan's acceptance criterion ("the restore runs before the hand-off exec, so the next process image inherits a cooked tty") delivers the spec's guarantee only while a waiter is running, not across hand-offs. No test sends a terminal signal during a hand-off: the swallow tests drive only the waiter's byte loop.
  RECOMMENDATION: Keep terminal signal generation off for the whole wait chain, not only inside each waiter:
    - Clear ISIG across screen-to-screen hand-offs. Do not clear OPOST, because the draw's newline-separated render needs it.
    - Turn signal generation back on only on the paths that hand the pane to a hook or a shell.
    That way a Ctrl-C in the window is a byte the next waiter swallows, which is what the spec states. As the backstop the spec's fallback depends on, make the parked chain survive a group signal (`trap : INT QUIT;` ahead of the draw), so a signal that still lands ends in the recovery tail rather than a closed pane.
COMMENT_CORRECTIONS:
- internal/tui/resume_discard_confirm.go:5-7 — cites the specification (a workflow artifact) and argues the reasoning instead of stating the constraint
  OLD: // Stated verbatim by the specification. It names no tool and is not
	// paraphrased here: it is the only copy standing between a keypress and the
	// loss of a user-authored command.
  NEW: // Tool-agnostic: the resume machinery runs whatever command a registration holds.
- cmd/capturetool/main.go:200-203 — makes a claim about how many callers there are ("shared by the surface branch and the fixture one", "the two routes"), which the next caller will make false
  OLD: // The one read, shared by the surface branch and the fixture one: NO_COLOR wins
// over --theme (there is no canvas to select), so a capture shows no painted
// canvas whatever palette was named, and the two routes cannot disagree about
// when that is.
  NEW: // NO_COLOR wins over --theme: there is no canvas to select, so a capture shows
// no painted canvas whatever palette was named.
- cmd/state_resume_wait.go:424-426 — "the one signal it takes off nothing" does not parse; what the comment needs to say is only that a hangup keeps its default disposition
  OLD: // The pane's own resizes reach the waiter as a signal, which is the one signal
// it takes off nothing: every other keeps its default disposition, so tmux
// tearing the pane down ends the waiter with it.
  NEW: // A hangup keeps its default disposition, so tmux tearing the pane down ends
// the waiter with it.
SUMMARY: The implementation matches the specification on hooks.json storage, the `hook set` flag and `hook list` column, the doctor lines, the capture freeze and scrollback re-file, both panel screens, and the picker row and legend. One divergence clears the floor: the spec's promise that Ctrl-C is swallowed and the parked shell survives lapses during the cooked window of every screen hand-off, where a terminal signal can close the pane and lose a single-pane session together with its transcript. There are also three comment corrections.
