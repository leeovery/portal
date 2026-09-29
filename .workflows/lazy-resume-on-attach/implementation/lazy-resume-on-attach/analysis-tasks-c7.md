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

**Outcome**: Code outside `internal/state` reaches the committing cycle only through `RunCommitCycle`. A direct `CaptureAndRefile` or `RefilePendingScrollback` call does not compile there. A production file calling `state.Commit` fails the unit lane before it can land.
