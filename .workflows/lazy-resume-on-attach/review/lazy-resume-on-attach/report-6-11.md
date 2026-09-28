TASK: lazy-resume-on-attach-6-11 — The Picker Opened Straight After a Reboot Shows Every Waiting Pane

ACCEPTANCE CRITERIA:
- Cold route: the post-restore refetch reads session `S` with no pending pane while `S`'s pane still carries its `@portal-skeleton-*` marker; a moment later the pane is marked `@portal-resume-pending` and its skeleton marker cleared. With no keypress, `S`'s row gains the pending dot — and panes marked at different moments each gain theirs as they are marked
- Cold route: when the last pane is marked and the last skeleton marker cleared between two re-reads — or between the refetch and the first re-read — the picker still ends showing that session's dot, so the set it settles on holds every session with a pane marked before the last skeleton marker cleared
- Cold route: once the picker has found no skeleton marker remaining, it takes no further skeleton-marker read and no further pending read beyond the existing kill, rename and preview-dismissal re-reads
- Cold route: a skeleton marker that never clears stops the re-reading once `hydrateTimeout` plus the margin has elapsed; the picker keeps the last pending set it read and stays fully usable
- A re-read landing while the user has moved the cursor or typed a filter changes only which rows carry the pending dot — the rows, their order, the cursor's row and the filter text are as the user left them
- Warm route (server already running when the picker opened): no skeleton-marker read is taken, and the pending set is read only alongside the session list, as today

STATUS: issues_found

SPEC CONTEXT: The picker's session row carries an `accent.attention` pending dot when any pane in the session is waiting on a lazy resume. The pending state is the pane option `@portal-resume-pending`, read through one whole-server enumeration. On the cold reboot route the only pending read that happened was the post-restore refetch, which can land before the hydrate helpers mark their panes, so the first picker after a reboot, which is the moment the dot exists for, showed too few dots. The helper marks pending before it unsets the pane's skeleton marker (`cmd/state_hydrate.go:348-356`, reached from all three tails at :177, :320, :340). That ordering is what makes a pending read final once no skeleton marker remains.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/pending_settle.go:9-82 — the interval (100 ms), the `SkeletonMarkerReader` seam, the `WithPendingSettle` option, start/tick/read/apply. The marker read is taken ahead of the pending read (:67-70). A failed marker read counts as markers remaining (:70). The loop ends on no markers or on reaching the deadline, and it applies the final reading before stopping (:75-82)
  - internal/tui/model.go:118-119 (`afterRestore` on `SessionsMsg`), :1502-1510 (only the cold-route refetch sets it), :1655-1662 (settle starts only on an `afterRestore` SessionsMsg; tick and read arms)
  - internal/tui/model.go:1169-1172 — `applyPendingSessions`, the pending-only variant. `applySessions` now routes through it (:1162), and it is the one assignment site of `m.pendingSessions`
  - internal/tui/build.go:33-36, :141-143 — `SkeletonMarkers` and `PendingSettleBound` added to `tui.Deps` beside `PendingReader`. Nil-tolerant
  - cmd/open.go:488-499 — `pendingSettleBound = hydrateTimeout + 2*time.Second` and the `skeletonMarkerReader` adapter over `state.ListSkeletonMarkers`. Wired at :515, :590-591 and :724 over the same `*tmux.Client`
- Notes: The implementation matches the plan's Do list. Bootstrap, the eager signal pass and the hydrate helper are untouched. The marker-then-pending order inside the read command is correct: the pending read is evaluated after the marker read returns. The bound's comment holds, because every helper starts at restore (step 6) before the settle's deadline is set, so a never-signalled helper's 3 s timeout expires well inside the 5 s window. A kill, rename or preview-dismissal refresh carries no `afterRestore`, so none of them restarts the settle.

TESTS:
- Status: Adequate
- Coverage: internal/tui/pending_settle_test.go covers each criterion:
  - The race pin, where the dot appears with no keypress: :93
  - Staggered marking: :109
  - The last marker clearing before the first re-read and between two re-reads, with the per-step read order logged as markers-then-pending: :134
  - No further reads once settled, including a stray tick and a kill: :183
  - The bound, with the last set kept and the picker usable: :221
  - Cursor and filter left intact: :244
  - Warm route takes no marker read: :297
  - Unwired seam: :321
  - cmd/open_pending_settle_test.go covers the adapter's pass-through and its error propagation.
- Notes:
  - The error branch of the marker read has no test. Nothing exercises `err != nil` at internal/tui/pending_settle.go:70, because `markerReaderStub` (internal/tui/pending_settle_test.go:14-31) can only return `nil` errors. See FINDINGS.
  - No test observes the production wiring in `openTUI` (cmd/open.go:724). This matches the existing untested `pendingReader` wiring beside it, so it is not raised.

CODE QUALITY:
- Project conventions: Followed. It uses small seam interfaces and an optional nil-tolerant Deps field, it has no `t.Parallel`, and it leaves the log vocabulary alone
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (builtin `min`, range-over-int)
- Readability: Good. The comments hold against the code
- Issues: None beyond the test gap below

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/pending_settle.go:70 — The rule that a failed skeleton-marker read counts as markers remaining has no test. `markerReaderStub` (internal/tui/pending_settle_test.go:14-31) has no error field, so no case drives the `err != nil` arm. Fix: give the stub an error for a chosen step. Then add a cold-route case where the first marker read fails and a later one succeeds with none remaining. Assert that the failed read scheduled a further re-read, and that the picker ends with the dot the later pending read carries. — FAILS: dropping `err != nil ||` breaks no test. A transient `show-options` failure would then read as "no markers" and end the settle on its first tick, so panes whose helpers mark afterwards never gain their dot. That is the exact symptom the task exists to fix, and no test would catch it.

UNSETTLED:
- None
