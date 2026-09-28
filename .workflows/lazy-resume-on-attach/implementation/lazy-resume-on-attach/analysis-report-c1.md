# Analysis Report: lazy-resume-on-attach (Cycle 1)

## Stats

- Total findings: 9
- Deduplicated findings: 9
- Proposed tasks: 3

## Summary

Nine findings across the three agents (duplication 3, standards 1, architecture 5), none overlapping. Three become proposals: a kill key pressed during a screen hand-off reaching the parked shell and closing the pane, the recovery tail's argv failing under a binary upgraded mid-wait (its proposed shell backstop re-keyed so a recovered pane does not take two exits to close), and the panel's `d`/`y` keys declared separately for the footer and the waiter. The six low-severity findings were discarded, none clustering into a pattern, and five comment corrections were collected for direct application.

## Comment Corrections

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
- cmd/state_hydrate.go:311 — the timeout path clears the skeleton marker too, and for a pane marked pending by the call below, capture does not resume on the next tick
  OLD: // Unlike the timeout path: with no scrollback to dump, the save loop should
	// resume capturing this pane on the next tick rather than skip it forever.
  NEW:
- internal/state/capture.go:119 — livePane carries the pane's active flag, not its liveness
  OLD: // reads its token and liveness from the read rather than from the fresh index,
  NEW: // reads its token and active flag from the read rather than from the fresh index,

## Discarded Findings

- The OSC string terminator is parsed twice, and the two parsers disagree on C1 ST (duplication, low) — low severity, no cluster, and the failure it names cannot arise: every reply a waiting pane reads is composed by tmux's pane emulator, which terminates its OSC colour replies with BEL or ESC-backslash (mirroring the query) and never with the 8-bit ST byte, so the waiter's consumer never meets 0x9C.
- The plain-shell fall-through is composed in four places, and the hook-or-shell choice in two (duplication, low) — low severity, no cluster; the divergence needs a deliberate change to one site's shell launch, and the ordering rule those sites share already has its one declaration in the settled hand-off shape (`execHandOff`, the sibling one-liner phase 4 chose for the bare-shell and hook hand-offs).
- The resume decision is valid only after the mark step, which three sites must remember to run; a nil decision fires lazy hooks eagerly (architecture, low) — low severity, no cluster; every current tail runs the mark step and the nil-decision branch has no production caller, so the failure needs a future tail that drops the call. Its pane-closing half (an empty executable in the parked chain) would also be caught by task 2's could-not-run backstop, since sh answers an empty command name with status 127.
- Registration's JSON encoding ignores its exported fields whenever it was decoded (architecture, low) — low severity, no cluster; no current caller edits a decoded registration in place, and the recommendation's removal of `Set`'s re-projection reverses the settled phase 1 direction (the store re-projects what it stores, the constructor alternative having been rejected as the weaker shape) with no measured defect as its ground.
- The scrollback re-file renames on disk before the index that names the new path is committed (architecture, low) — low severity, no cluster; the window is one tick per pane per boot, between the first frozen tick's rename and that tick's commit, and loss needs the daemon to die inside it and the tmux server to be gone before any later tick commits — any later commit against a live server repairs it through the missing-source tolerance.
- Callers must apply the waiting-pane scrollback skip themselves, unlike the re-file (architecture, low) — low severity, no cluster; the daemon is the only production scrollback writer and applies the skip, the other copies are test fixtures, and the finding names no current test the dropped frozen set makes pass wrongly — only a future writer or test that forgets.
