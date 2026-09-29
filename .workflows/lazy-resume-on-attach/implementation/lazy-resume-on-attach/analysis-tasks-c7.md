# Analysis Tasks: Lazy Resume On Attach (Cycle 7)

## Task 1: Only the Locked Cycle Can Commit From Outside `internal/state`
severity: medium
sources: architecture

**Problem**: Phase 12's commit lock is correct inside `state.RunCommitCycle` (`internal/state/commit_cycle.go:45-72`), but the unlocked pieces it composes are still exported, and each is a complete committing step on its own:
- `Commit` (`internal/state/commit.go:22`), the only code in the tree that deletes scrollback files, through `gcOrphanScrollback`.
- `CaptureAndRefile` (`internal/state/scrollback.go:275`).
- `RefilePendingScrollback` (`internal/state/scrollback.go:99`).

No production caller outside `internal/state` uses any of the three. The daemon's tick, its shutdown flush and `commit-now` all enter through `RunCommitCycle`. The only non-test caller of `CaptureAndRefile` is `RunCommitCycle` (`commit_cycle.go:58`), and the only non-test caller of `RefilePendingScrollback` is `CaptureAndRefile` (`scrollback.go:286`).

So a future committer written against the exported API compiles, behaves correctly in isolation and leaves every test green, but it takes no `commit.lock`. Examples: a new save or flush command, a doctor repair, or a test harness promoted to production. Such a committer reopens the interleaving cycle 6 closed. When it and the daemon disagree on whether a pane is still mid-restore or already pending, the second one's housekeeping deletes the `pane-<token>.bin` the first has just filed. A frozen pane is never captured again, so nothing rewrites the file. The user finds out at the next reboot, when the panel comes up over an empty pane. Today the only protection is one sentence in CLAUDE.md: "Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving."

**Solution**: Make `RunCommitCycle` the only committing route that code outside `internal/state` can take, without breaking any fixture that commits through the real `Commit`.
- **Unexport `CaptureAndRefile` and `RefilePendingScrollback`.** Neither has a caller outside `internal/state`. The package's black-box tests (`internal/state/capture_refile_test.go`, `internal/state/scrollback_test.go`) reach them through aliases in the existing `internal/state/export_test.go`, which already bridges `StubRenameNoReplaceUnsupported`.
- **Keep `Commit` exported, and fence it with a unit-lane guard driven by `sourceguardtest`.** The guard fails any non-test file outside `internal/state` that calls `state.Commit`. It also has rule tests that run it over a staged tree (`Rooted`), so it is shown to fire rather than pass by scanning nothing.
  - Why a guard and not unexporting `Commit`: unexporting would break nine test call sites in `cmd` and `cmd/bootstrap`, and one of them has no migration target. The commit-now fixture's stand-in cycle (`cmd/state_commit_now_test.go:107`) commits through the real `Commit`, and its tests read what landed in `sessions.json`. `restoretest.WriteIndex` cannot take its place, because it writes unconditionally and collects nothing.
  - Every committer the failure names is a non-test file, and a non-test scan covers them all. A guard is also how the tree already fences a rule the compiler is not enforcing (`internal/tmux/target_composition_guard_test.go`, `cmd/seam_guard_test.go`).
- **Update CLAUDE.md's `state` row (`CLAUDE.md:61`).** Its references to `CaptureAndRefile` follow the rename. Its closing warning ("Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving.") becomes a sentence naming the fence: the unexported pieces, plus the guard refusing `state.Commit` outside the package.

**Outcome**: Code outside `internal/state` reaches the committing cycle only through `RunCommitCycle`. A direct `CaptureAndRefile` or `RefilePendingScrollback` call does not compile there. A production file calling `state.Commit` fails the unit lane before it can land. The daemon's tick, its shutdown flush and `portal state commit-now` commit exactly as before, and every existing test compiles and passes as written.

**Acceptance Criteria**:
- [ ] On the tree as it stands, the guard reports nothing and `go test ./...` passes. All 33 existing `state.Commit(` call sites compile and run as written. Every one of them is in a `_test.go` file, and nine are in `cmd` and `cmd/bootstrap`, including the commit-now fixture's stand-in cycle at `cmd/state_commit_now_test.go:107`.
- [ ] Run over a staged tree that holds a non-test file outside `internal/state` calling `state.Commit`, the guard fails and reports that file by its repository path.
- [ ] Run over a staged tree whose only `state.Commit` calls are in `_test.go` files outside `internal/state`, or in non-test files inside `internal/state`, the guard reports nothing.
- [ ] A file outside `internal/state` that calls `state.CaptureAndRefile` or `state.RefilePendingScrollback` fails to compile.
- [ ] The black-box suites in `internal/state/capture_refile_test.go` and `internal/state/scrollback_test.go` pass with their call sites unchanged. The daemon's tick, its shutdown flush and `portal state commit-now` capture, re-file, dump and commit exactly as before. The unit lane and the integration lane (`go test -tags integration -p 1 ./...`) both stay green, and no test's semantics change.
- [ ] CLAUDE.md's `state` row no longer names an exported `CaptureAndRefile`. The sentence "Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving." is replaced by one that names the fence: the unexported capture-and-re-file pieces, and the guard refusing `state.Commit` outside `internal/state`.

**Do**:
- In `internal/state/scrollback.go`, unexport `CaptureAndRefile` (line 275) and `RefilePendingScrollback` (line 99). Their only non-test callers are inside the package, at `internal/state/commit_cycle.go:58` and `internal/state/scrollback.go:286`, and both follow the rename.
- Alias both in the existing `internal/state/export_test.go`, beside `StubRenameNoReplaceUnsupported`, so the black-box call sites stay as written. Measured with `rg -c 'state\.(CaptureAndRefile|RefilePendingScrollback)\b' -g '*.go'`: 21 occurrences in 2 files, 7 in `internal/state/capture_refile_test.go` and 14 in `internal/state/scrollback_test.go`. No other Go file names either function.
- Keep `Commit` (`internal/state/commit.go:22`) exported. Add an untagged, unit-lane guard driven by `sourceguardtest`: a non-test scan through `RepoSources` that fails on any non-test file outside `internal/state` calling `state.Commit`. Give it rule tests that run it over staged trees through `sourceguardtest.Rooted`. Measured with `rg -c '\bstate\.Commit\(' -g '*.go'`: 33 call sites across 10 files, all of them `_test.go`. None of them is converted.
- In CLAUDE.md's `state` row (`CLAUDE.md:61`), the three references to `CaptureAndRefile` follow the rename. The closing warning becomes the sentence naming the fence.
