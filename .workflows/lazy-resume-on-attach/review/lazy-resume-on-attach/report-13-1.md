TASK: Only the Locked Cycle Can Commit From Outside `internal/state` (lazy-resume-on-attach-13-1, tick-e4633a)

ACCEPTANCE CRITERIA:
- On the tree as it stands, the guard reports nothing and `go test ./...` passes. All 33 existing `state.Commit(` call sites compile and run as written; every one is in a `_test.go` file, nine in `cmd` and `cmd/bootstrap`, including the commit-now fixture's stand-in cycle at `cmd/state_commit_now_test.go:107`.
- Run over a staged tree that holds a non-test file outside `internal/state` calling `state.Commit`, the guard fails and reports that file by its repository path.
- Run over a staged tree whose only `state.Commit` calls are in `_test.go` files outside `internal/state`, or in non-test files inside `internal/state`, the guard reports nothing.
- A file outside `internal/state` that calls `state.CaptureAndRefile` or `state.RefilePendingScrollback` fails to compile.
- The black-box suites in `internal/state/capture_refile_test.go` and `internal/state/scrollback_test.go` pass with their call sites unchanged. The daemon's tick, its shutdown flush and `portal state commit-now` capture, re-file, dump and commit exactly as before. Both lanes stay green, and no test's semantics change.
- CLAUDE.md's `state` row no longer names an exported `CaptureAndRefile`; the sentence "Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving." is replaced by one naming the fence (the unexported capture-and-re-file pieces, and the guard refusing `state.Commit` outside `internal/state`).

STATUS: complete

SPEC CONTEXT: Section 7.2 of the spec re-files a frozen (resume-pending or mid-restore) pane's scrollback under a token-derived name (`scrollback/pane-<PortalPaneID>.bin`). The housekeeping pass builds its reachable set from stored record paths. A committer that interleaves with another can therefore collect a token-named transcript the other has just filed, and a frozen pane is never re-captured, so the loss stays hidden until the next reboot. Phase 12 put a lock inside `RunCommitCycle`. This task makes that locked cycle the only committing route open to code outside the package.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:99: `refilePendingScrollback` is unexported, and its doc comment at :88 follows the rename.
  - internal/state/scrollback.go:275: `captureAndRefile` is unexported, with its doc comment at :267. Its internal call to `refilePendingScrollback` is at :286.
  - internal/state/commit_cycle.go:58: `RunCommitCycle` calls `captureAndRefile`. `Commit` stays exported at internal/state/commit.go:22, and its one production caller is commit_cycle.go:68.
  - internal/state/export_test.go:14-17: aliases `CaptureAndRefile` and `RefilePendingScrollback`, next to `StubRenameNoReplaceUnsupported`.
  - internal/state/commit_guard_test.go:15-19: the repo guard `TestNoProductionCommitOutsideState`. The scan is `scanForDirectCommit` at :129-145, the selector match is `namesStateCommit` at :147-163, and import-name resolution is `stateImportName` at :165-177.
  - CLAUDE.md:61: the state row now says "The unexported `captureAndRefile`…" and "…through `captureAndRefile`, the caller's `Dump` and `Commit`…". The closing warning is replaced by "Code outside `internal/state` cannot commit around the lock: `captureAndRefile` and `refilePendingScrollback` are unexported, and a unit-lane guard (`internal/state/commit_guard_test.go`) fails any non-test file outside `internal/state` naming `state.Commit`…".
- Notes:
  - Reading the tree: no non-test Go file outside `internal/state` names `.Commit` through any import of the package. No file anywhere aliases or dot-imports `internal/state`. There are 33 `state.Commit(` call sites across 10 `_test.go` files, as the task measured. The only other two matches are fixture strings inside the guard file itself.
  - `state.(CaptureAndRefile|RefilePendingScrollback)` occurs 21 times, all in the two `package state_test` black-box suites (7 and 14), and all resolve through the `export_test.go` aliases unchanged. The aliases compile only into `internal/state`'s own test build, so any other package gets an undefined-identifier compile error.
  - The in-package skip is an exact `filepath.Dir(source.Path) == internal/state` comparison, not a prefix match. So `internal/statetest`, which has a non-test file importing `internal/state`, is still scanned. `RepoSources` parses regardless of build tags, so integration-tagged and `!integration` production files are both covered.
  - The daemon, commit-now and shutdown paths are untouched except for the one rename inside `RunCommitCycle`. None of the four files changed after the task's commit.

TESTS:
- Status: Adequate
- Coverage:
  - `TestCommitGuard_Rule` (commit_guard_test.go:21-99) runs the same scan over trees staged through `sourceguardtest.Rooted`. It covers five cases:
    - A production file in `cmd` calling `state.Commit`: one finding, reported by the repo-relative path `cmd/save.go`.
    - An aliased import taking `saved.Commit` as a value: one finding.
    - `_test.go` callers in `cmd` and `cmd/bootstrap`, beside a clean production file so the scan is non-empty: nothing reported.
    - A production file inside `internal/state`: nothing reported.
    - A `Commit` selector on an unrelated package also named `state`: nothing reported.
  - `TestCommitGuard_FatalsWhenItScansNothing` (:102-111) shows the guard cannot pass vacuously over an empty tree.
  - The repo-wide `TestNoProductionCommitOutsideState` pins the current tree clean.
  - Each rule case would fail if the corresponding rule broke: a missing skip, a wrong path form, a missed alias, or an over-broad package match.
- Notes: Focused, not bloated. The aliased-value case goes beyond the criteria but closes a real bypass of a call-only match.

CODE QUALITY:
- Project conventions: Followed. The guard is untagged and unit-lane, built on `sourceguardtest.RepoSources`/`Rooted`/`NonTestSources` and reporting through `harnesstest.TestingT`, as the tree's other source guards do. It does not use `t.Parallel()`.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`slices.Equal`, `strconv.Unquote` on the import path)
- Readability: Good. Comments hold true against the code and cite no process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "On the tree as it stands, the guard reports nothing and `go test ./...` passes." — requires running `go test ./...`. Reading settles the guard half: no non-test file outside `internal/state` names `state.Commit`.
- "The unit lane and the integration lane (`go test -tags integration -p 1 ./...`) both stay green" — requires running both lanes.
