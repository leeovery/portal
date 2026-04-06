---
phase: 1
phase_name: Fix Server Bootstrap and Add Regression Tests
total: 2
---

# Phase 1: Fix Server Bootstrap and Add Regression Tests

## resume-hooks-not-firing-after-server-kill-1-1 | approved

### Task 1: Fix StartServer and update existing tests

**Problem**: `StartServer()` in `internal/tmux/tmux.go` (lines 123-131) runs `tmux start-server`, which creates a sessionless server. tmux's default `exit-empty on` causes the server to self-terminate before tmux-continuum's delayed session restore can run (continuum sleeps 1 second before calling resurrect). This means sessions are never restored and resume hooks never fire.

**Solution**: Replace the `tmux start-server` command with `tmux new-session -d` in `StartServer()`. This creates a detached bootstrap session that keeps the server alive during plugin initialization and continuum's delayed restore. Use bare `new-session -d` with no explicit session name -- tmux defaults to session name "0", which resurrect's "restoring from scratch" logic recognizes and cleans up if it wasn't in the save file. Then update all existing tests in `TestStartServer` and `TestEnsureServer` to expect the new command.

**Outcome**: `StartServer()` issues `new-session -d` instead of `start-server`. All existing tests pass with updated expectations. The `EnsureServer()` return contract is unchanged: `(false, nil)` when already running, `(true, nil)` when just started, `(true, err)` on failure.

**Do**:
1. In `internal/tmux/tmux.go`, `StartServer()` function (lines 123-131):
   - Change `c.cmd.Run("start-server")` to `c.cmd.Run("new-session", "-d")`
   - Update the comment on line 123 from "starts the tmux server without creating any sessions" to something like: "starts the tmux server by creating a detached bootstrap session. The bootstrap session keeps the server alive during plugin initialization and session restoration. Uses bare new-session with no name so tmux defaults to session '0', which tmux-resurrect recognizes and cleans up during restore."
   - Keep the error wrapping message descriptive. Change `"failed to start tmux server: %w"` to `"failed to start tmux server (bootstrap session): %w"` to reflect that the mechanism is now a bootstrap session.
2. In `internal/tmux/tmux_test.go`, update `TestStartServer`:
   - Subtest "starts tmux server successfully" (line 392): change `wantArgs` from `"start-server"` to `"new-session -d"`
   - Subtest "returns error when start-server fails" (line 412): no args change needed (uses simple `Err` field on mock, not `RunFunc`), but verify the error message check on line 422 still matches. The `wantMsg` is `"failed to start tmux server"` -- update to match the new wrapping message (e.g., `"failed to start tmux server (bootstrap session)"`)
   - Subtest "does not retry on failure" (line 434): no change needed -- it already just checks `len(mock.Calls) == 1`
3. In `internal/tmux/tmux_test.go`, update `TestEnsureServer`:
   - Subtest "starts server and returns true when server is not running" (line 469): change the `RunFunc` branch from `args[0] == "start-server"` to checking `args[0] == "new-session"` (the full args will be `["new-session", "-d"]`)
   - Subtest "returns true and error when start-server fails" (line 494): same change -- update `args[0] == "start-server"` to `args[0] == "new-session"`
   - Subtests "returns false when server is already running" (line 447) and "does not call start-server when server is running" (line 519): no changes needed -- these never reach `StartServer()`
4. Run `go test ./internal/tmux/...` to verify all tests pass.
5. Run `go test ./...` to verify no regressions elsewhere.

**Acceptance Criteria**:
- [ ] `StartServer()` calls `c.cmd.Run("new-session", "-d")` -- no session name argument
- [ ] `StartServer()` comment explains the bootstrap session approach and why no name is used
- [ ] Error wrapping message remains descriptive and includes the original error via `%w`
- [ ] All three `TestStartServer` subtests pass with updated expectations
- [ ] All four `TestEnsureServer` subtests pass with updated expectations
- [ ] `EnsureServer()` return contract unchanged: `(false, nil)`, `(true, nil)`, `(true, err)`
- [ ] `go test ./...` passes with zero failures

**Tests**:
- `"starts tmux server successfully"` -- mock verifies `Run` called with `["new-session", "-d"]`, no error returned
- `"returns error when start-server fails"` -- mock returns error, `StartServer()` returns wrapped error containing both descriptive message and original error
- `"does not retry on failure"` -- mock returns error, verify exactly 1 call recorded (no retry)
- `"returns false when server is already running"` -- `info` succeeds, `EnsureServer` returns `(false, nil)`, `StartServer` never called
- `"starts server and returns true when server is not running"` -- `info` fails, `new-session -d` succeeds, returns `(true, nil)`
- `"returns true and error when start-server fails"` -- `info` fails, `new-session -d` fails, returns `(true, err)`
- `"does not call start-server when server is running"` -- only `info` called, exactly 1 call recorded

**Edge Cases**:
- Error wrapping message remains descriptive: the `fmt.Errorf` wrapping must still contain a human-readable prefix and wrap the underlying error with `%w` so callers can use `errors.Is`/`errors.As`
- No-retry behavior unchanged: `StartServer()` must call `Run` exactly once on failure, never retrying
- EnsureServer return contract preserved: the three return value combinations `(false, nil)`, `(true, nil)`, `(true, err)` must remain identical in semantics

**Context**:
> The specification requires bare `tmux new-session -d` with no explicit session name. tmux defaults to session name "0". This is deliberate: tmux-resurrect's "restoring from scratch" logic detects exactly 1 pane and cleans up session "0" if it wasn't in the save file. A custom name would not be recognized by this cleanup. The fix works regardless of the user's `exit-empty` setting because a session exists, so `exit-empty` never triggers.

**Spec Reference**: `.workflows/resume-hooks-not-firing-after-server-kill/specification/resume-hooks-not-firing-after-server-kill/specification.md` -- sections "Bug: Server Exits Before Session Restoration", "Fix", "Session naming", "EnsureServer Return Contract", "Testing Requirements" items 1-4.

## resume-hooks-not-firing-after-server-kill-1-2 | approved

### Task 2: Add bootstrap-to-query regression test

**Problem**: The existing tests verify `StartServer()` and `EnsureServer()` in isolation, but there is no test that validates the full bootstrap-to-query flow: starting the server via `EnsureServer()` and then immediately querying sessions via `ListSessions()`. This end-to-end flow is exactly the sequence Portal uses in production, and the bug manifested because the server died between these two calls.

**Solution**: Add a new test in `internal/tmux/tmux_test.go` within `TestEnsureServer` (or as a sibling test function) that exercises the full flow: `EnsureServer()` starts the server (mock verifies `new-session -d`), then `ListSessions()` returns sessions. The mock's `RunFunc` handles all three commands in sequence: `info` (fails, server not running) -> `new-session -d` (succeeds) -> `list-sessions` (returns session data). This validates that the bootstrap approach produces a queryable server state.

**Outcome**: A new test exists that proves the bootstrap-to-query flow works end-to-end through mock expectations. The test documents the critical invariant: after `EnsureServer()` starts the server, `ListSessions()` must find sessions.

**Do**:
1. In `internal/tmux/tmux_test.go`, add a new test function `TestEnsureServerThenListSessions` (or add it as a subtest within `TestEnsureServer` -- either approach works, but a standalone function makes the regression test more visible):
   ```go
   func TestEnsureServerThenListSessions(t *testing.T) {
       t.Run("bootstrap session is queryable after EnsureServer starts server", func(t *testing.T) {
           mock := &MockCommander{
               RunFunc: func(args ...string) (string, error) {
                   if args[0] == "info" {
                       return "", fmt.Errorf("no server running")
                   }
                   if args[0] == "new-session" {
                       return "", nil
                   }
                   if args[0] == "list-sessions" {
                       return "0|1|0", nil // bootstrap session "0", 1 window, not attached
                   }
                   t.Fatalf("unexpected command: %v", args)
                   return "", nil
               },
           }
           client := tmux.NewClient(mock)

           started, err := client.EnsureServer()
           if err != nil {
               t.Fatalf("EnsureServer() error: %v", err)
           }
           if !started {
               t.Fatal("EnsureServer() started = false, want true")
           }

           sessions, err := client.ListSessions()
           if err != nil {
               t.Fatalf("ListSessions() error: %v", err)
           }
           if len(sessions) != 1 {
               t.Fatalf("ListSessions() returned %d sessions, want 1", len(sessions))
           }
           if sessions[0].Name != "0" {
               t.Errorf("session name = %q, want %q", sessions[0].Name, "0")
           }

           // Verify the exact command sequence
           if len(mock.Calls) != 3 {
               t.Fatalf("expected 3 calls, got %d: %v", len(mock.Calls), mock.Calls)
           }
           wantSequence := []string{"info", "new-session", "list-sessions"}
           for i, want := range wantSequence {
               if mock.Calls[i][0] != want {
                   t.Errorf("call %d: got %q, want %q", i, mock.Calls[i][0], want)
               }
           }
       })
   }
   ```
2. Run `go test ./internal/tmux/...` to verify the new test passes.
3. Run `go test ./...` to verify no regressions.

**Acceptance Criteria**:
- [ ] New test function exists in `internal/tmux/tmux_test.go` that exercises `EnsureServer()` followed by `ListSessions()`
- [ ] Mock verifies the command sequence: `info` -> `new-session -d` -> `list-sessions`
- [ ] Mock returns session "0" with 1 window and not attached from `list-sessions`
- [ ] Test asserts `EnsureServer()` returns `(true, nil)`
- [ ] Test asserts `ListSessions()` returns exactly 1 session named "0"
- [ ] Test asserts exactly 3 mock calls in the correct order
- [ ] `go test ./...` passes with zero failures

**Tests**:
- `"bootstrap session is queryable after EnsureServer starts server"` -- full flow: EnsureServer starts server via `new-session -d`, then ListSessions returns the bootstrap session "0". Verifies the exact command sequence through mock call recording.

**Edge Cases**: None specified for this task.

**Context**:
> The specification (Testing Requirements item 5) explicitly calls for: "End-to-end unit test in `internal/tmux`: `EnsureServer()` starts server (mock verifies `new-session -d`) -> `ListSessions()` returns sessions -- validates the full bootstrap->query flow through mock expectations." The session name "0" is significant: it is what tmux defaults to when no `-s` flag is given, and it is what tmux-resurrect expects to find during its "restoring from scratch" cleanup.

**Spec Reference**: `.workflows/resume-hooks-not-firing-after-server-kill/specification/resume-hooks-not-firing-after-server-kill/specification.md` -- section "Testing Requirements" item 5, and "Session naming" for the "0" session name rationale.
