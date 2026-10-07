TASK: The Scrollback Writer Refuses A Pane The Cycle Skips (killing-all-sessions-wipes-restore-state-5-1, tick-064cc9)

ACCEPTANCE CRITERIA:
- A committing cycle's capture holds a pane that `SkipsScrollback` answers true for: one named by a skeleton marker, one carrying the resume pending marker, or one in a session carried forward from the previous index. The pane has a saved transcript. A dump hands the cycle's writer a capture for that pane. `Write` answers not-written with no error, no confirmation read is sent, the saved transcript keeps its bytes, and the cycle's dedup map holds the same entry for that key as before.
- The same holds when the capture handed over is empty and the committer's own server would confirm it.
- After such a cycle commits, `sessions.json` names for the skipped pane the same file as a cycle whose dump never handed it over, and that file is still on disk with its bytes.
- A pane the cycle does not skip is written exactly as before. The writer suites in `internal/state/empty_capture_test.go` and `internal/state/commit_cycle_answered_test.go` pass unchanged.
- The daemon's tick still sends no `capture-pane` for a skeleton-marked or waiting pane. The daemon suites pinning that skip (`TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` in `cmd/state_daemon_run_test.go`, `TestCaptureAndCommit_SkipsResumePendingPanes` in `cmd/state_daemon_resume_pending_test.go`) pass unchanged.

STATUS: complete

SPEC CONTEXT: Section 2.4 makes the commit cycle's `ScrollbackWriter` the only production route to a saved scrollback file (the commit guard refuses `WriteScrollbackIfChanged` outside `internal/state`), and that writer already enforces the empty-capture confirmation rule. The cycle has a second rule: no dump writes a skeleton-marked, waiting or carried pane (`CaptureCycle.SkipsScrollback`). Until this task, only each dumper enforced that rule. The task moves it into the writer, so a dumper that leaves the check out cannot overwrite a transcript that nothing else holds.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit_cycle.go:55 — `ScrollbackWriter` carries a `capture CaptureCycle` field.
  - internal/state/commit_cycle.go:75-77 — `Write` returns `(false, nil)` when `w.capture.SkipsScrollback(paneKey)` answers true. This is its first step, ahead of the held-transcript lookup and its `delete(w.hm, …)` (:79-86), `confirmEmptyCapture` (:87) and `WriteScrollbackIfChanged` (:90).
  - internal/state/commit_cycle.go:136-144 — `RunCommitCycle` builds the writer from the capture after `keepAnsweredTranscripts` has run. That function does not modify the skip sets, so the writer sees the same Skeleton, Pending and Carried sets that `SkipsScrollback` reads at internal/state/scrollback.go:335-342.
  - internal/state/commit_cycle.go:44-48 — `CommitCycle.Dump`'s doc now says the writer refuses a skipped pane, instead of asking the dump to skip it.
  - cmd/state_daemon.go:315 — the daemon's own check is unchanged, as the task requires.
- Notes:
  - The task commit (3e0ba4a41) touches only `internal/state/commit_cycle.go` and the new test file.
  - The two other places that construct a writer are unaffected. `heldTranscripts` (commit_cycle.go:243) already excludes skipped keys, so the new early return cannot leave a held pane half-processed. The zero-value `state.ScrollbackWriter{}` in cmd/state_commit_now_test.go:103 has nil sets, so `SkipsScrollback` answers false.
  - The lazy-panel fixture (internal/restore/lazy_resume_panel_integration_test.go:510) still checks `SkipsScrollback` before calling `writer.Write`, so the refusal never reaches it. The task text says this fixture writes through `WriteScrollbackIfChanged`; in today's tree it uses `writer.Write`. Either way the outcome is the same, so this is not a finding.
  - The refusal returns `(false, nil)` as planned, so the daemon's anomalous tally and the `write scrollback failed` line are not touched.

TESTS:
- Status: Adequate
- Coverage: `TestCycleScrollbackWriterRefusesAPaneTheCycleSkips` (internal/state/commit_cycle_skip_test.go:136) covers three worlds × two captures:
  - Worlds: skeleton-marked, waiting, and carried (a waiting pane whose session's environment read fails).
  - Captures: a live capture, and an empty capture that the committer's own server would confirm.
  - Each world puts X into exactly one of the three sets: the carried world's session is not reached by the capture, so Pending does not hold X. A fatal precondition (:150-152) checks that X is in the intended set and that `SkipsScrollback` answers true, so no subtest can pass vacuously.
  - Each subtest asserts:
    - `Write` returns `(false, nil)`;
    - zero confirmation reads, measured as a delta around the `Write` call alone;
    - the dedup entry is unchanged before and after `Write`;
    - against a control cycle whose dump hands nothing over, `sessions.json` names the same file for X;
    - the named file still holds the saved transcript.
- If the early return were removed, every subtest would fail:
  - live captures: the writer would write over X's positional file and update the hash map;
  - empty captures: `confirmEmptyCapture` would send a read, so confirms would be 1.
- The empty-capture case passes `[]byte{}` rather than nil, so it really is handed over. The `runSkipCycle` doc ("A nil data dumps nothing") makes this distinction correctly.
- Notes:
  - The named regression suites were not modified by the task commit. Reading them shows no world that marks the written pane as skipped:
    - `emptyWorldClient` holds no sessions;
    - `answeredClient` has no skeleton or pending marker;
    - `movedClient` and the shifted suite use no markers and so carry nothing.
  - So the new early return cannot change what any of their assertions observe.
  - No over-testing: six subtests, each covering a distinct combination of skip set and capture, with no redundant assertions.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`. Helpers are reused from sibling suites (`waitingIndex`, `seedScrollback`, `recordFor`, `onDiskIndex`, `paneLineWithPending`, `otherRow`). No process-artifact references in comments.
- SOLID principles: Good. The write-admission rules now live in the one production route to a scrollback file.
- Complexity: Low. The change is a single guard clause.
- Modern idioms: Yes.
- Readability: Good. The `Write` and `Dump` doc comments hold true against the code.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A pane the cycle does not skip is written exactly as before. The writer suites in `internal/state/empty_capture_test.go` and `internal/state/commit_cycle_answered_test.go` pass unchanged." — Reading settles that both suites are unchanged by the task commit and that none of their worlds hands the writer a skipped key. That they pass needs a run: `go test ./internal/state -run 'TestCycleScrollbackWriter|TestRunCommitCycle'`.
- "The daemon's tick still sends no `capture-pane` for a skeleton-marked or waiting pane. The daemon suites pinning that skip (...) pass unchanged." — Reading settles that the daemon's check at cmd/state_daemon.go:315 is unchanged and that neither suite was touched by the task commit. That they pass needs a run: `go test ./cmd -run 'TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback|TestCaptureAndCommit_SkipsResumePendingPanes'`.
