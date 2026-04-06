---
status: complete
created: 2026-04-06
cycle: 1
phase: Plan Integrity Review
topic: resume-hooks-not-firing-after-server-kill
---

# Review Tracking: resume-hooks-not-firing-after-server-kill - Integrity

## Findings

### 1. Task 1-2 mock return value for list-sessions lacks format detail

**Severity**: Important
**Plan Reference**: Phase 1 / Task 1-2 (tick-a0dabc)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-2 says the mock should "return session '0'" for the `list-sessions` command, but `ListSessions()` calls `c.cmd.Run("list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}")` and parses pipe-delimited output. The mock's `RunFunc` must return the formatted string `"0|1|0"` (name|windows|attached), not just `"0"`. Without this detail, the implementer must read `ListSessions()` source to determine the correct mock return format, breaking task self-containment.

**Current**:
```
Solution: Add TestEnsureServerThenListSessions that exercises the full flow. Mock handles 4 commands: info (fails) -> new-session -d (succeeds) -> list-sessions (returns session "0") -> info (succeeds).
```

```
Do:
1. Add TestEnsureServerThenListSessions in internal/tmux/tmux_test.go with mock RunFunc handling info (fail), new-session -d (succeed), list-sessions (return session "0"), and info (succeed)
2. Assert EnsureServer() returns (true, nil), ListSessions() returns 1 session named "0", ServerRunning() returns true
3. Verify exactly 4 mock calls in correct order
4. Run go test ./...
```

```
Acceptance Criteria:
- [ ] New test function exists exercising EnsureServer() followed by ListSessions() and ServerRunning()
- [ ] Mock verifies command sequence: info (fail) -> new-session -d -> list-sessions -> info (succeed)
- [ ] Mock returns session "0" from list-sessions
- [ ] Test asserts EnsureServer() returns (true, nil)
- [ ] Test asserts ListSessions() returns 1 session named "0"
- [ ] Test asserts ServerRunning() returns true after bootstrap
- [ ] Test asserts exactly 4 mock calls in correct order
- [ ] go test ./... passes
```

**Proposed**:
```
Solution: Add TestEnsureServerThenListSessions that exercises the full flow. Mock RunFunc handles 4 commands in sequence: info (fails) -> new-session -d (succeeds) -> list-sessions (returns "0|1|0" -- pipe-delimited format matching ListSessions' format string "#{session_name}|#{session_windows}|#{session_attached}") -> info (succeeds).
```

```
Do:
1. Add TestEnsureServerThenListSessions in internal/tmux/tmux_test.go with mock RunFunc that switches on args[0]: "info" (first call returns error, subsequent calls return nil), "new-session" (returns "", nil), "list-sessions" (returns "0|1|0" -- matching the pipe-delimited format #{session_name}|#{session_windows}|#{session_attached})
2. Assert EnsureServer() returns (true, nil), ListSessions() returns 1 session with Name "0", Windows 1, Attached false, ServerRunning() returns true
3. Verify exactly 4 mock calls in correct order: ["info"], ["new-session", "-d"], ["list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}"], ["info"]
4. Run go test ./...
```

```
Acceptance Criteria:
- [ ] New test function exists exercising EnsureServer() followed by ListSessions() and ServerRunning()
- [ ] Mock verifies command sequence: info (fail) -> new-session -d -> list-sessions -> info (succeed)
- [ ] Mock returns "0|1|0" (pipe-delimited format) from list-sessions, producing Session{Name: "0", Windows: 1, Attached: false}
- [ ] Test asserts EnsureServer() returns (true, nil)
- [ ] Test asserts ListSessions() returns 1 session with Name "0"
- [ ] Test asserts ServerRunning() returns true after bootstrap
- [ ] Test asserts exactly 4 mock calls in correct order
- [ ] go test ./... passes
```

**Resolution**: Fixed
**Notes**:

