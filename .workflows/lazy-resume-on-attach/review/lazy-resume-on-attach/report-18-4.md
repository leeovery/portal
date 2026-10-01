TASK: Corrections (tick-b31eb9, lazy-resume-on-attach-18-4). Two CLAUDE.md edits: the `state` row is brought up to date on the capture cycle (`CaptureCycle.SkipsScrollback`, `Carried`, the missed-session carry), and the Resume hooks lazy paragraph is brought up to date on the token write and the eager-downgrade causes.

ACCEPTANCE CRITERIA:
- The `state` row states that `captureAndRefile` returns `CaptureCycle{Index, Pending, Skeleton, Carried}` and that `Carried` holds the pane keys of a session carried forward from the previous index
- A contributor writing a new `Dump` from the `state` row is pointed at `CaptureCycle.SkipsScrollback` as the skip it takes; the row no longer states the saver's skip as mid-restore or pending, as `CaptureCycle.Skeleton` or `CaptureCycle.Pending`, or as "two sets"
- The `state` row carries one sentence on the carry: a live waiting pane whose session missed the capture has its previous session carried whole under its previous name for that cycle, with no token put on a second record, and a carry onto a reached session's name fails the capture so the callers' existing failure routes retry
- The `state` row's commit-now sentence says it discards the sets, not both sets
- The lazy paragraph orders the lazy pane's writes as `markResumePending` does: the baked key written as the pane's `@portal-pane-id` with no read first, then the alternate-screen pin, then `@portal-resume-pending`, all before the mid-restore marker is cleared
- The lazy paragraph's causes that downgrade a pane to eager include a refused token write, under the same `set resume pending marker failed` WARN, so the list names every refusal `markResumePending` can return
- Every identifier the edited text names exists in the tree, and nothing in CLAUDE.md outside the edited sentences changes

STATUS: complete

SPEC CONTEXT: §7.2 says a waiting pane whose session a capture misses keeps its place: the previous index's session is carried whole under its previous name for that cycle, no token lands on a second record, and a carry that would land on a reached session's name refuses the commit, which the caller then retries through its existing failure route. §7.3 says the saver's skip covers mid-restore panes, pending panes, and (from the carry) every pane of a carried session. §7.2 (token match) and §8.2 say restore's re-stamp is best-effort, so the helper writes the pane's saved token itself, with no read first, before it pins and marks the pane. CLAUDE.md is the contributor-facing description that new committers and hydrate changes are written from.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:61 (`state` row), CLAUDE.md:190 (Resume hooks lazy paragraph)
- Notes:
  - Criterion 1: CLAUDE.md:61 says `captureAndRefile` returns "a `CaptureCycle{Index, Pending, Skeleton, Carried}` — `Carried` holding the pane keys of a session carried forward from the previous index rather than captured". This matches the struct at internal/state/scrollback.go:258-269 and the constructor at internal/state/scrollback.go:299.
  - Criterion 2: the skip sentence now reads "The saver's per-pane scrollback skip is whatever `CaptureCycle.SkipsScrollback` answers — a key in `Skeleton` …, `Pending` … or `Carried`". The return sentence adds "whose skip the caller's own scrollback dump takes through `CaptureCycle.SkipsScrollback`, never by reading the sets itself". This matches `SkipsScrollback` (internal/state/scrollback.go:276-283) and the daemon's dump, which calls it (cmd/state_daemon.go:304). The "mid-restore or pending", "`CaptureCycle.Skeleton` or `CaptureCycle.Pending`" and "two sets" wording is gone.
  - Criterion 3: the carry sentence matches internal/state/capture.go:44-49 (the carry), :70-72 (`errCarryNameTaken`) and :195-199 (the refusal). The refusal reaches both named failure routes: `RunCommitCycle` wraps the capture error (internal/state/commit_cycle.go:59-62), the daemon tick's `tick failed` WARN re-touches `save.requested` (cmd/state_daemon.go:200-205), and commit-now routes the error to `failCommitNow` (cmd/state_commit_now.go:123-124, :141-147).
  - Criterion 4: the commit-now sentence now reads "discards the sets and dumps nothing". This matches `_, err = deps.RunCommitCycle(...)` with no `Dump` (cmd/state_commit_now.go:114-122).
  - Criterion 5: CLAUDE.md:190 orders the writes as token ("the baked key written as the pane's `@portal-pane-id`, with no read first"), then pin, then marker, "all **before** the mid-restore marker is cleared". This matches `markResumePending` (cmd/state_hydrate.go:406, :409, :412) and `markPendingThenUnsetSkeletonMarker`, which calls `unsetSkeletonMarkerOrLog` after the mark (cmd/state_hydrate.go:367-379).
  - Criterion 6: the downgrade list reads "an absent `$TMUX_PANE`, an unresolvable executable, a refused token write, a refused alternate-screen pin or a failed marker write". These are all the returns of `markResumePending`: `requireTmuxPane` fails only on an empty `TMUX_PANE` (cmd/hooks.go:49-55), then resolveExe, the token write, the pin and the marker (cmd/state_hydrate.go:394-417). All of them reach the single WARN at cmd/state_hydrate.go:372.
  - Criterion 7, identifiers: every identifier in the edited text exists. `CaptureCycle.SkipsScrollback` (internal/state/scrollback.go:276). `captureAndRefile` (:293). `errCarryNameTaken` (internal/state/capture.go:72). The `tick failed` WARN (cmd/state_daemon.go:201). `failCommitNow` (cmd/state_commit_now.go:141). `@portal-pane-id` (`PortalPaneIDOption`, internal/state/markers.go:26). `@portal-resume-pending` (`ResumePendingOption`, internal/state/markers.go:31). `alternate-screen` (`altScreenOption`, cmd/state_resume_altscreen.go:14). The `set resume pending marker failed` WARN (cmd/state_hydrate.go:372).
  - Criterion 7, scope: the second half (nothing outside the edited sentences changed) needs a diff and is recorded under UNSETTLED.

TESTS:
- Status: Adequate
- Coverage: documentation-only task; no test is called for and none would observe CLAUDE.md prose.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The edited text names the real identifiers and keeps the row's existing voice.
- SOLID principles: Good (N/A — prose)
- Complexity: Low
- Modern idioms: Yes (N/A — prose)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "nothing in CLAUDE.md outside the edited sentences changes" — reading the current file cannot show what changed. Settling it needs a diff of CLAUDE.md across the commit(s) that landed this task, confirming that the only hunks are the CLAUDE.md:61 sentences (skip, `captureAndRefile` return, carry, commit-now) and the CLAUDE.md:190 sentences (ordered writes, downgrade list).
