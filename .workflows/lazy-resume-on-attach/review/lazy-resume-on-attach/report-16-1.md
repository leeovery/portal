TASK: An Answered Pane Closes on Its First Exit Even After Its Binary Has Gone (tick-8ae66e, lazy-resume-on-attach-16-1)

ACCEPTANCE CRITERIA:
- An answered pane (Enter or confirmed discard, marker clear) whose baked binary has gone from the baked path by its shell's exit: the parked chain reads the marker and exits 127 — no reset bytes, no marker clear, no #{alternate_on} read, no pin unset, no stty sane, no shell.
- The same answered pane with its binary still at the path but no longer executable ends the same way with status 126.
- An unanswered pane (marker carries any non-empty value) whose tail cannot start: after the marker read the backstop takes its existing steps in their existing order — reset bytes, marker clear, pin lifted once #{alternate_on} reads 0 within the bound (kept if it never does), stty sane, the user's shell — and exits with that shell's status.
- tmux refuses the marker read: counts as still pending, same steps, reaches the shell, and the refused read writes nothing to the pane's stderr.
- No tmux on PATH: counts as still pending, reaches the shell, backstop writes nothing to stderr.
- A tail that started gets nothing from the backstop, not even the marker read, and the chain exits with the tail's status (answered pane whose tail did nothing; recovered pane whose shell exited with any status, 126 and 127 included).

STATUS: complete

SPEC CONTEXT: Section 4.2 rules that "a pane that has been answered is handed no second shell": the chain's recovery step reads the pending marker, does nothing for a pane no longer marked, and treats a failed read as still pending (an extra shell is the lesser failure against a pane that closes under the user). Section 4.3 describes the recovery tail's order (leave the panel's screen, clear the marker, lift the pin once the leave is confirmed, exec the user's shell). The Go tail (runResumeRecover) already applied the gate; the shell backstop for an unstartable tail did not, so a binary removed by an upgrade while an answered pane's shell ran produced the two-exits regression.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_hydrate.go:295-298 — the gate: a plain `tmux display-message -p -t <PaneIDTarget> -F '#{@portal-resume-pending}' 2>/dev/null` read, composed from state.ResumePendingOption and a `target` local bound to string(tmux.PaneIDTarget(payload.Pane)); `if m=$(...); then case $m in '') exit $s;; esac; fi`
  - cmd/state_hydrate.go:303-305 — the gate is the first step of the could-not-run arm, after the `126|127` match and the `[ ! -x <exe> ]` check, before the reset bytes; the clear, backstopReleasePin, `stty sane`, `exec "${SHELL:-/bin/sh}"` and trailing `exit $s` are unchanged
  - cmd/state_hydrate.go:282-292 — doc comment names the gate, the failed-read-means-pending rule, and why a plain format read suffices
  - CLAUDE.md:190 — the "Resume hooks" backstop sentence names the gate
- Notes: Shell semantics check out: the exit status of `m=$(cmd)` is the substitution's, so a refused read (tmux exits non-zero) or a missing tmux (127) skips the gate and falls through to the existing steps; a successful empty read ends with `exit $s`, carrying 126/127 through; `s` is not overwritten by the gate; `2>/dev/null` sits inside the substitution, so both tmux's own refusal and the shell's command-not-found report are discarded. Presence semantics match state.ResumePendingSet (any non-empty value pending, empty absent). parkedChainTrap and the unanswered path's order are untouched. The `target` local is bound straight from PaneIDTarget, so the target-composition guard (internal/tmux/target_composition_guard_test.go) accepts the new `-t` site. Matches the Go tail's rule in cmd/state_resume_recover.go:40-43.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_backstop_test.go:256-280 — the new answered case: answeredThenGoneExe's draw removes itself (or drops its execute bit under KEEP_UNEXECUTABLE) then exits; RESUME_PENDING= makes the stub read the marker as clear; asserts the transcript is the marker read alone (which rules out reset bytes, clear, alternate_on read, unpin, stty and shell, since every one logs to the transcript), empty backstop stderr, and status 127 / 126.
  - :174/:178/:194/:203 — the existing unstartable-tail transcripts gain the marker read ahead of the reset bytes, covering the default-pending, flipping-poll and never-confirmed-leave cases; :215-224 covers an arbitrary non-empty marker value.
  - :226-253 — failingTmux (refuses the read, stderr text asserted absent from the pane's stderr) and no tmux on PATH ("tmux:" asserted absent), both reaching the shell with status 0.
  - :282-306 — started tails (including recovered shells exiting 126/127) assert an empty transcript, so not even the marker read runs, and the status passes through.
- Notes: The tests would fail on each plausible regression: gate removed (answered case gains reset/shell), gate inverted (unanswered cases end early), gate placed before the executable check (started cases gain a marker read), gate after the reset (answered transcript gains reset bytes), stderr redirect dropped (no-tmux and refused-read leak checks). Not over-tested: the answered cases are a two-row table varying only the unstartable mode and expected status.

CODE QUALITY:
- Project conventions: Followed — the target is composed through tmux.PaneIDTarget and the option name through state.ResumePendingOption; shell words go through shellquote.Join; the read takes the same display-message -p … -F form backstopReleasePin uses; the comments make no reference to process artifacts.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
