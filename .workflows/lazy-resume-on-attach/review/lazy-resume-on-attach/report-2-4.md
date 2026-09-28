TASK: lazy-resume-on-attach-2-4 (tick-b3514f) — The Saver's Skip Gains Its Second Condition

ACCEPTANCE CRITERIA:
- A pane carrying neither marker is captured exactly as today: one `capture-pane` for it, its scrollback file written, its `pane captured` DEBUG emitted and the tick summary's `panes` count including it.
- A pane carrying the pending marker gets no `capture-pane` call and no scrollback write, while its live sibling in the same window is captured normally in the same tick.
- A pane carrying both the skeleton and the pending marker is skipped once — one `continue`, no `capture-pane`, no duplicate or double-counted anything.
- A skipped pane is absent from the tick summary's `panes` count and emits no per-pane capture breadcrumb.
- With the marker cleared, the next tick captures that pane again and writes its scrollback under its current address.
- The shutdown flush inherits the same skip, because it runs `captureAndCommit`: a pending pane is not captured by the final flush either.
- `portal state commit-now` is unchanged and still needs no skip of its own.
- The set of log messages and attr keys emitted across a tick holding a pending pane is exactly the set emitted by a tick without one, minus that pane's per-pane breadcrumb.

STATUS: complete

SPEC CONTEXT: The spec (sections 7.1–7.3, 9.1) explains why a waiting pane must never be captured. `capture-pane -e -p -S -` over a pane whose alternate screen holds the resume card returns the scrolled-out history plus the card and drops the most recent screenful, so an unfrozen waiting pane loses that screenful permanently and gains a dead card image. Today the skip reads only the positional mid-restore skeleton marker, and the hydrate helper clears it just as the panel goes up. The skip therefore gains a second condition, the pane-scoped `@portal-resume-pending` marker, read from the capture's own twelfth column at no extra tmux cost. The freeze suppresses only the scrollback write; the structural capture still enumerates the pane into `sessions.json`. The spec calls this daemon-side skip the one exception to "nothing in the restore pipeline changes".

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:249 (pending set bound from the capture), cmd/state_daemon.go:273-275 (single `continue` ahead of `panes++` at :276, the `pane captured` DEBUG at :277, `CaptureAndHashPane` at :279 and `WriteScrollbackIfChanged` at :290), cmd/state_daemon.go:319-328 (`paneSkipsScrollback`, skeleton OR pending), cmd/state_daemon.go:309-315 (tick summary untouched), cmd/state_daemon.go:363 (shutdown flush reaches the same path through `captureAndCommit`), CLAUDE.md `state` row (the required sentence is present).
- Notes:
  - The task says to bind the set returned by `state.CaptureStructure`. The code now binds it from `state.CaptureAndRefile` (internal/state/scrollback.go:166-173), which a later task introduced. That function returns `CaptureStructure`'s pending set unchanged, so this is a sound divergence, not a loss.
  - Both halves of the check key on the pane's live address. `addPendingPanes` builds its keys from the live enumeration (internal/state/capture.go:126-136). `mergeFrozenPanes` keeps the live `Index` on the merged record (internal/state/capture.go:178-203). So the `SanitizePaneKey` recomputed in the loop matches the set's keys.
  - The read order is safe against the helper's "set pending before clearing skeleton" rule. The daemon reads the skeleton markers (:244) before the pane enumeration that carries the pending column, so no interleaving leaves both reads showing a pane as unmarked while the card is up.
  - `cmd/state_commit_now.go` writes no capture-pane scrollback (Commit is called with `anyScrollbackChanged=false`, line 126) and has no skip of its own. Commit 5b9ebf62b did not touch it.
  - The commit changed only CLAUDE.md, cmd/state_daemon.go and the new test file, so the existing skeleton-marker suites are unmodified.

TESTS:
- Status: Adequate
- Coverage: cmd/state_daemon_resume_pending_test.go:93-288 (`TestCaptureAndCommit_SkipsResumePendingPanes`) has all seven planned subtests. They stage pending panes through the twelfth `panesOut` column (`daemonPaneRow`, :19-26) and assert through `callsContaining("capture-pane")`, the scrollback directory listing and a `logtest.Sink`.
  - neither marker: one capture target, `work__0.0.bin` written, breadcrumb present, `panes == 1`.
  - pending plus sibling: only `=work:0.1`'s sibling is captured, and only its file is written.
  - both markers: the skeleton marker is staged via `markersOut`; one capture, one breadcrumb, `panes == 1`.
  - skipped pane: absent from the `panes` count and emits no breadcrumb.
  - marker cleared: the next tick captures the pane and writes its file.
  - shutdown flush: driven through `defaultShutdownFlush`, with no capture of the pending pane and `flush_completed=true`.
  - no new event or attr key: the message and key sets of a pending tick and an unmarked tick are compared, and the breadcrumb delta is asserted separately.
  - Each fixture maps the pending pane to a "must-not-be-captured" body, so a regression in the skip changes both the capture-target list and the directory listing.
  - The "marker clears" subtest moves the pane's address and carries no token, so it does not show that same-address pane returning. The later `TestCaptureAndCommit_RefilesResumePendingScrollback` subtest at :451-486 covers that case, including reclaim of the frozen file.
- Notes: The pending-plus-sibling and pane-count subtests share an identical fixture. The plan names both, so this is not reported.

CODE QUALITY:
- Project conventions: Followed. No new log component, message or attr key. No `t.Parallel()`. The shared `daemonFakeCommander` and `logtest.Sink` query chain are used as the codebase prescribes.
- SOLID principles: Good. The skip predicate is a single-purpose pure function.
- Complexity: Low
- Modern idioms: Yes (the test file uses `slices.Sorted(maps.Keys(...))` and `for tick := range 3`)
- Readability: Good. The predicate's comment states why a pending pane must not be captured, not what the code does.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
