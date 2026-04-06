---
status: complete
created: 2026-04-06
cycle: 1
phase: Traceability Review
topic: Resume Hooks Not Firing After Server Kill
---

# Review Tracking: Resume Hooks Not Firing After Server Kill - Traceability

## Findings

### 1. Missing spec Testing Requirement 2: ServerRunning() returns true after bootstrap

**Type**: Incomplete coverage
**Spec Reference**: Testing Requirements item 2 -- "ServerRunning() returns true after the new bootstrap"
**Plan Reference**: Task 1-2 (tick-a0dabc) -- Add Bootstrap-to-Query Regression Test
**Change Type**: add-to-task

**Details**:
The specification explicitly requires testing that `ServerRunning()` returns true after the new bootstrap (Testing Requirements item 2). Task 1-2 tests EnsureServer -> ListSessions but never calls ServerRunning() after the bootstrap to verify the server is alive. The mock would need to handle a 4th command (info succeeds) after the bootstrap sequence.

**Current**:
```
Do:
1. Add TestEnsureServerThenListSessions in internal/tmux/tmux_test.go with mock RunFunc handling info, new-session, and list-sessions
2. Assert EnsureServer() returns (true, nil), ListSessions() returns 1 session named "0"
3. Verify exactly 3 mock calls in correct order
4. Run go test ./...

Acceptance Criteria:
- [ ] New test function exists exercising EnsureServer() followed by ListSessions()
- [ ] Mock verifies command sequence: info -> new-session -d -> list-sessions
- [ ] Mock returns session "0" from list-sessions
- [ ] Test asserts EnsureServer() returns (true, nil)
- [ ] Test asserts ListSessions() returns 1 session named "0"
- [ ] Test asserts exactly 3 mock calls in correct order
- [ ] go test ./... passes

Tests:
- "bootstrap session is queryable after EnsureServer starts server" -- full flow: EnsureServer starts server via new-session -d, then ListSessions returns the bootstrap session "0". Verifies the exact command sequence through mock call recording.
```

**Proposed**:
```
Do:
1. Add TestEnsureServerThenListSessions in internal/tmux/tmux_test.go with mock RunFunc handling info (fail), new-session -d (succeed), list-sessions (return session "0"), and info (succeed)
2. Assert EnsureServer() returns (true, nil), ListSessions() returns 1 session named "0", ServerRunning() returns true
3. Verify exactly 4 mock calls in correct order
4. Run go test ./...

Acceptance Criteria:
- [ ] New test function exists exercising EnsureServer() followed by ListSessions() and ServerRunning()
- [ ] Mock verifies command sequence: info (fail) -> new-session -d -> list-sessions -> info (succeed)
- [ ] Mock returns session "0" from list-sessions
- [ ] Test asserts EnsureServer() returns (true, nil)
- [ ] Test asserts ListSessions() returns 1 session named "0"
- [ ] Test asserts ServerRunning() returns true after bootstrap
- [ ] Test asserts exactly 4 mock calls in correct order
- [ ] go test ./... passes

Tests:
- "bootstrap session is queryable and server is running after EnsureServer starts server" -- full flow: EnsureServer starts server via new-session -d, then ListSessions returns the bootstrap session "0", then ServerRunning() returns true. Verifies the exact command sequence through mock call recording.
```

**Resolution**: Fixed
**Notes**:

---
