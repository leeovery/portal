---
status: in-progress
created: 2026-04-06
cycle: 1
phase: Gap Analysis
topic: resume-hooks-not-firing-after-server-kill
---

# Review Tracking: resume-hooks-not-firing-after-server-kill - Gap Analysis

## Findings

### 1. Testing requirement #4 contradicts the actual fix

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements (requirement #4)

**Details**:
Testing requirement #4 states "Existing `EnsureServer()` tests pass -- return contract unchanged." While the return contract is indeed unchanged, the existing tests will NOT pass as-is. The current `TestEnsureServer` and `TestStartServer` tests hardcode `"start-server"` in their mock `RunFunc` callbacks (e.g., `if args[0] == "start-server"` at tmux_test.go:475, 500). After changing the command to `new-session -d`, these mocks won't recognize the new command and tests will fail via `t.Fatalf("unexpected command")`.

The requirement as written is misleading -- it implies the implementer should only add new tests while existing ones pass unmodified. In reality, the existing `StartServer` and `EnsureServer` unit tests must be updated to expect the new command arguments. The return contract is preserved, but the mock expectations change.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Integration test (requirement #5) lacks implementation guidance

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements (requirement #5)

**Details**:
Testing requirement #5 specifies "Integration test: `EnsureServer()` bootstrap -> `ListSessions()` returns non-empty -> server still running after poll window." This requires a real tmux server -- mock-based unit tests cannot verify that a server stays alive. However, the spec doesn't clarify:

- Whether this should be a subprocess-based integration test (like the existing pattern in `cmd/root_integration_test.go` which builds the binary and tests via subprocess execution)
- Or a test that directly calls `tmux.NewClient(tmux.NewRealCommander())` against a real tmux instance
- Whether it should be skipped in CI environments where tmux may not be available
- What file/package it should live in

This matters because the project has an established integration test pattern (subprocess in `cmd/`) and an established unit test pattern (mocks in `internal/tmux/`). The implementer needs to know which pattern to follow.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
