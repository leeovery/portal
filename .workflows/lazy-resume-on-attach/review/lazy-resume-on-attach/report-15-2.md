TASK: Corrections (lazy-resume-on-attach-15-2, tick-c30df9) — amend CLAUDE.md's Resume hooks paragraph (line 190) so the lazy branch, recovery tail, backstop step list and eager-downgrade refusal list describe the alternate-screen pin, its confirmed-leave release and the refused-pin downgrade as the code runs them

ACCEPTANCE CRITERIA:
- Lazy-branch sentence: pane first pinned to a pane-level `alternate-screen on`, then marked `@portal-resume-pending`, both before the mid-restore marker is cleared, with the reason the pin goes first (a pin left behind overrides one display setting on one pane; a marker left behind freezes that pane's scrollback).
- Recovery-tail sentence: pin lifted after the marker clear and before the cooked terminal and the shell, only once tmux reports `#{alternate_on}` `0` within a bounded poll; an unconfirmed leave keeps the pin (an unset processed before the leave bytes makes tmux ignore the leave); Enter and a confirmed discard take the same release after a clear that landed; a refused clear keeps the pin for the redraw.
- Backstop sentence: reset bytes, `tmux set-option -pu -t <pane> @portal-resume-pending` (errors discarded), the bounded `#{alternate_on}` poll followed by `tmux set-option -pu -t <pane> alternate-screen` only once it reads `0` (errors discarded), `stty sane`, `${SHELL:-/bin/sh}` — and "same steps in the same order" holds against the amended tail sentence.
- Refusal sentence: a refused alternate-screen pin is among the causes downgrading to eager under the one `set resume pending marker failed` WARN; a refused marker lifts the pin it follows; a refused lift logs its own `lift alternate-screen pin failed` WARN.
- Every other sentence of line 190 and every other line of CLAUDE.md byte-unchanged; no Go source or test file touched; `go test ./...` stays green.

STATUS: complete

SPEC CONTEXT: The specification's corrigendum of 2026-09-30 corrected §5.1 (the waiting pane is pinned to the alternate screen because an install can set `alternate-screen off`; every route off the panel's screen lifts the pin last and only once tmux confirms the leave via `#{alternate_on}` reading `0` on a short bounded poll), §4.3 (the tail's order: leave, clear, confirmed release, shell), §7.3 and §8.2 (a pane whose token, pin or marker cannot be written does not wait; a refused marker lifts the pin written ahead of it). CLAUDE.md is loaded into every agent session as the account of the resume chain, so a stale step list invites an agent to strip or reorder the pin/poll/unset.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:190 (commit 7ed45cd36 touched CLAUDE.md alone, one line). Verified against the code as it stands:
  - Lazy branch: cmd/state_hydrate.go:367-379 (`markPendingThenUnsetSkeletonMarker` — mark step, then `unsetSkeletonMarkerOrLog`); cmd/state_hydrate.go:409 (pin `alternate-screen on`) precedes :412 (`state.SetResumePendingMarker`); the stated reason matches the in-source comment at :389-392.
  - Recovery tail: cmd/state_resume_recover.go:48 (leave bytes) -> :55 (marker clear) -> :58 (`releaseAltScreenPin`) -> :60 (cook) -> :62 (shell). cmd/state_resume_altscreen.go:39-59 — unset only after `awaitPrimaryScreen` sees `0` within `altScreenLeaveAttempts` x `altScreenLeavePoll`; an unconfirmed leave returns without unpinning.
  - Enter / confirmed discard: cmd/state_resume_wait.go:351-360 (`resumeUnfreeze` — release only after a clear that landed; a refused clear returns to the redraw with the pin standing), reached from :303 (Enter) and :335 (discard).
  - Backstop: cmd/state_hydrate.go:303-305 (gate -> reset -> marker clear with `2>/dev/null` -> `backstopReleasePin` -> `stty sane` -> `${SHELL:-/bin/sh}`); :310-322 (`backstopReleasePin` polls `#{alternate_on}` on the same attempts/poll pair and unsets `alternate-screen` only when it read `0`, both commands with stderr discarded). Order matches the tail, so "same steps in the same order" holds.
  - Refusals: cmd/state_hydrate.go:409-411 (refused pin returns before the marker), :412-416 (refused marker lifts the pin; refused lift WARNs `lift alternate-screen pin failed`), :372 (the single `set resume pending marker failed` WARN every refusal returns into).
- Notes: A sentence-level diff of the commit shows only the four targeted sentences changed (the recovery-tail amendment adding one sentence for the Enter/discard release); every other line of CLAUDE.md is byte-identical across the commit. Later tasks (16-1, 17-3, 18-4) have since layered further edits onto line 190 (the backstop's answered-pane gate, the refused token write); the 15-2 content survives intact in the current text and still matches the code.

TESTS:
- Status: Adequate
- Coverage: Documentation-only task; no tests are expected. The commit touches no Go file, and no Go source or test in the repository references CLAUDE.md, so the edit cannot change any test outcome — the "`go test ./...` stays green" criterion is settled by reading.
- Notes: None

CODE QUALITY:
- Project conventions: Followed (CLAUDE.md prose names no task ids, phases or spec section numbers)
- SOLID principles: Good (N/A — prose only)
- Complexity: Low
- Modern idioms: Yes (N/A)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
