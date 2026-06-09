# Plan: Kill-Rename Prefix Collision

## Phases

### Phase 1: Enforce exact-match session targeting at the tmux chokepoint
status: approved
approved_at: 2026-06-09

**Goal**: Eliminate the silent wrong-session kill/rename by introducing the `exactTarget` session-level primitive in `internal/tmux`, fixing the two destructive callers (`KillSession`, `RenameSession`) to build their `-t` target with the `=` exact-match prefix, migrating the five existing inline `"="+name` session-target sites onto the helper, and locking the behaviour with regression and unit tests.

**Why this order**: This is a single-root-cause bugfix contained entirely within one subsystem (`internal/tmux`) at the Client-method argv-construction chokepoint. The specification defines one fix with no independently-valuable intermediate state — the helper, the two argv corrections, the behaviour-neutral migration, and the test changes are highly cohesive and share the same blast radius, so splitting would create thin phases with no real checkpoint between them. The fix lives at the chokepoint with no caller-side changes anywhere, so the whole correction is one verifiable increment.

**Acceptance**:
- [ ] `exactTarget(session string) string` exists in `internal/tmux` as the canonical session-level exact-match target builder (sibling to `PaneTargetExact`); a focused unit test asserts `exactTarget("foo") == "=foo"`.
- [ ] `KillSession(name)` issues `kill-session -t =<name>`; `RenameSession(oldName, newName)` issues `rename-session -t =<oldName> <newName>` with `newName` kept **bare** (prefix on target only).
- [ ] New prefix-collision regression tests for both `KillSession` and `RenameSession` (mirroring `TestHasSessionUsesExactMatchPrefix`, simulating tmux's exact-match semantics via `MockCommander.RunFunc`) prove a live prefix-colliding session (e.g. `foo-2`) is never killed or renamed when the target (`foo`) is not a live exact match — a dropped-`=` regression fails loudly.
- [ ] The five migration sites (`HasSession`, `HasSessionProbe`, `SwitchClient` in `tmux.go`; `saverPanePID`, `SaverPaneID` in `saver_pane_pid.go`) are routed through `exactTarget` with **identical argv**, and their existing tests stay green — no inline `"="+name` session-target strings remain anywhere in the `internal/tmux` package.
- [ ] Both `KillSession` and `RenameSession` carry rationale godoc blocks mirroring the already-fixed exact-match sites.
- [ ] `TestKillSession` is updated to expect `kill-session -t =my-session` and `TestRenameSession` to expect `rename-session -t =old-name new-name` (replacing the assertions that pinned the buggy bare-`-t` form).
- [ ] `go build` and `go test ./...` are green; all existing tmux package tests pass.
