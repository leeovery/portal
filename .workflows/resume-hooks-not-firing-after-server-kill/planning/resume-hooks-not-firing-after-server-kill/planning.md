# Plan: Resume Hooks Not Firing After Server Kill

## Phase 1: Fix Server Bootstrap and Add Regression Tests
<!-- status: approved | approved_at: 2026-04-06 -->

**Goal**: Replace `tmux start-server` with `tmux new-session -d` in `StartServer()` so the server stays alive during plugin initialization and session restoration, and cover the fix with regression tests.

**Rationale**: This is a single-phase fix. The root cause is one line in one function (`StartServer()` in `internal/tmux/tmux.go`). The spec scopes the change to this function only — no other components need modification. The existing tests verify exact command arguments passed to the mock, so they must be updated in lockstep with the fix. A single phase covers: failing test demonstrating the old behavior, the one-line fix, updating existing tests, and adding the end-to-end unit test the spec requires (EnsureServer -> ListSessions flow).

**Acceptance Criteria**:
- [ ] `StartServer()` calls `new-session -d` instead of `start-server` (verified by mock expectations in unit tests)
- [ ] `StartServer()` comment updated to reflect it now creates a detached bootstrap session
- [ ] Error wrapping message in `StartServer()` remains descriptive and includes the underlying error
- [ ] All existing `TestStartServer` subtests pass with updated command expectations (`new-session -d`)
- [ ] All existing `TestEnsureServer` subtests pass with updated command expectations (`new-session -d` instead of `start-server`)
- [ ] `EnsureServer()` return contract unchanged: `(false, nil)` when running, `(true, nil)` on successful start, `(true, err)` on failed start
- [ ] New end-to-end unit test: EnsureServer starts server then ListSessions returns sessions, validating the bootstrap-to-query flow
- [ ] `go test ./...` passes with zero failures
