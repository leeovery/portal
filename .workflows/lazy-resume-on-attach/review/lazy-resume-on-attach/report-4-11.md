TASK: lazy-resume-on-attach-4-11 (tick-15fa10) — The frozen-pane capture cycle cannot be entered halfway

ACCEPTANCE CRITERIA:
- One function runs the structure read and the re-file; the daemon, commit-now and the integration fixture all enter through it and none of the three calls `RefilePendingScrollback` itself
- A failed structure read returns before anything is re-filed, and its caller still refuses to commit
- The daemon's behaviour is unchanged: same skip set, same dump skip for a frozen pane, same `tick complete` counts, same index committed
- commit-now still passes a nil skipSet and still commits with `anyScrollbackChanged=false`, and its re-file test drives production's re-file rather than a fake's
- A pending pane's scrollback is still re-filed onto its token before the commit's housekeeping pass runs, on all three routes
- `go test ./internal/state/ ./cmd/ -count=1`, `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: complete

SPEC CONTEXT: The specification (the frozen-pane section and its 2026-09-21 corrigendum) requires that, for as long as a pane waits, its scrollback is re-filed out of the positional namespace onto `scrollback/pane-<PortalPaneID>.bin`, with the merged record carrying that path. Otherwise the next pane to take the vacated address overwrites the transcript, or the housekeeping pass (`ComputeReferencedSet` / `gcOrphanScrollback`) reclaims the file. That protection holds only if the re-file runs between the structure read and the commit. This task makes that ordering structural: callers now enter the capture cycle through one entry point instead of restating it by convention.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:160-173 — `CaptureAndRefile(c, dir, skipSet, prev, hm, logger) (Index, map[string]struct{}, error)`. It calls `CaptureStructure`, returns its index, pending set and error untouched on failure, and otherwise runs `RefilePendingScrollback` before returning the mutated index and the pending set.
  - cmd/state_daemon.go:249 — the daemon now makes one call where it had `CaptureStructure` + `RefilePendingScrollback`. `ListSkeletonMarkers` (:244), the `paneSkipsScrollback` gate (:273), `Commit` (:303) and the `tick complete` counts (:309-315) are untouched, and the arguments are the same ones the old pair received (`deps.Dir`, `skipSet`, `deps.PrevIndex`, `deps.HashMap`, `deps.Logger`).
  - cmd/state_commit_now.go:32-36, :63-65, :121 — the seam is renamed to `CaptureAndRefile` with the composite's signature and defaults to `state.CaptureAndRefile`. The standalone re-file is gone. The call still passes a nil skipSet and a nil HashMap, and `Commit` still receives `false` (:126). A capture failure still routes to `failCommitNow` before `Commit` (:122-124).
  - internal/restore/lazy_resume_panel_integration_test.go:370-420 — `captureRound` enters through the composite (:383). Its doc comment names the composite, and its dump loop with the `skipSet ∪ pending` skip stays in place (:394-399).
  - cmd/deps_merge_convention_test.go:121-123, :292-305 — the rename is carried through.
  - CLAUDE.md, `state` row — names `CaptureAndRefile` as the entry point the capture cycle is taken through, lists its three callers, and says `CaptureStructure` stays exported for structure-only callers.
- Notes:
  - A repo-wide grep finds `RefilePendingScrollback` only in its own definition, inside the composite (internal/state/scrollback.go:171), and in `internal/state/scrollback_test.go`.
  - Every remaining `CaptureStructure` caller outside `internal/state` is a structure-only read (for example `lazy_resume_panel_integration_test.go:467`, `lazy_resume_discard_integration_test.go:156`) or `cmd/bootstrap`'s `runDaemonTick` helper. That helper passes `prev=nil`, and none of its callers stage a pending pane, so it has no frozen-pane cycle to protect. This matches the task's own scope: "only the three cycle sites move".
  - Behaviour of the moved code is otherwise unchanged:
    - commit-now's nil HashMap reaches `delete(hm, …)` in `refilePendingPane`, which is a no-op on a nil map, as before.
    - Every error return in `CaptureStructure` (internal/state/capture.go:56, :65, :69, :98) yields an empty index and an empty pending set, so the early return re-files nothing, matching the composite's doc comment.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/capture_refile_test.go:14 "it re-files every frozen pane before it returns":
    - Drives the real composite over a `captureMock` whose row carries a token-stamped pending pane.
    - Asserts four things: the pending set, the token path on the record, the bytes moved to the token-named file with the positional file gone, and the dedup entry dropped.
  - internal/state/capture_refile_test.go:47 "it returns the capture's error with nothing re-filed":
    - Uses a failing `failFastCaptureClient`.
    - Asserts four things: the error passes through (`errors.Is`), the index is empty, the pending set is non-nil and empty, and the positional file is untouched with no token file created.
    - It would catch a composite that swallowed the error.
  - cmd/state_commit_now_test.go:1187-1246 `TestStateCommitNow_RefilesResumePendingScrollback`:
    - Now runs the real `state.CaptureAndRefile` against a `fakeCaptureClient`. The client emits the waiting pane's token and pending marker in a 12-column row and has a seeded prior `sessions.json` to merge from.
    - Asserts four things: the committed record, the persisted record, that the token file survives `Commit`'s housekeeping pass with its bytes, and that the positional file is gone.
    - The earlier version asserted a re-file performed by a faked capture. That silent pass is gone.
  - Requirements checked by the existing suites:
    - Nil skipSet and `anyScrollbackChanged=false`: `TestStateCommitNow_DiscardsThePendingSet` (:1142).
    - Commit refused on capture failure, commit-now: `TestStateCommitNow_ExitsNonZeroWhenCaptureAndRefileFails` (:738).
    - Commit refused on capture failure, daemon: `TestDaemonTick_LogsAndSkipsOnCaptureStructureError` (cmd/state_daemon_run_test.go:515).
    - The daemon frozen-pane cycle: `TestCaptureAndCommit_RefilesResumePendingScrollback` (cmd/state_daemon_resume_pending_test.go:323). It runs `captureAndCommit`, and therefore the composite.
  - Mutation argument (by reading): dropping the `RefilePendingScrollback` call at internal/state/scrollback.go:171 would leave the record on `scrollback/work__0.1.bin`. That fails the first composite subtest (:33-37), the daemon test's recorded-path/file assertions (state_daemon_resume_pending_test.go:342-350), and the commit-now test's committed-path assertion (state_commit_now_test.go:1229).
- Notes: The task commit (2231619601) changed no assertion in the daemon suites, the cycle-summary suite or `TestRefilePendingScrollback`. The commit-now diff is renames plus the re-pointed re-file test, which keeps all its previous assertions and adds a `Commit`-call-count check. No over-testing: the two new subtests each pin one side of the composite's contract.

CODE QUALITY:
- Project conventions: Followed. The doc comment leads with the identifier. The seam rename goes through `resolveCommitNowDeps`, the merge-convention seam table and `withCommitNowDeps` staging, and no seam is assigned directly. The commit-now tests inject `IsRestoring` so nothing reaches a live server.
- SOLID principles: Good. The composite owns the ordering invariant, and each caller keeps only what differs between them: whether it dumps and how it skips.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./internal/state/ ./cmd/ -count=1`, `go test ./...` and `go test -tags integration -p 1 ./...` pass" — requires running the unit lane and the integration lane (`-p 1`). The integration lane includes `internal/restore/lazy_resume_panel_integration_test.go`, whose `captureRound` now enters through the composite.
