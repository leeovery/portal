---
status: in-progress
created: 2026-04-03
cycle: 1
phase: Traceability Review
topic: Resume Hooks Lost On Server Restart
---

# Review Tracking: Resume Hooks Lost On Server Restart - Traceability

## Findings

### 1. Missing test for old pane-ID entries cleaned on upgrade

**Type**: Incomplete coverage
**Spec Reference**: Specification > Breaking change to hooks.json: "Old pane-ID-keyed entries (e.g., `%0`) are automatically cleaned by `CleanStale` on the first run with live panes after upgrading -- they won't match any live structural key."
**Plan Reference**: Phase 3, Task 3-5 (resume-hooks-lost-on-server-restart-3-5)
**Change Type**: add-to-task

**Details**:
The specification explicitly describes the upgrade path behavior: old pane-ID-keyed entries (`%0`, `%3`) are cleaned by `CleanStale` on the first run with live panes after upgrading because they won't match any live structural key. No plan task tests this specific upgrade scenario. Task 3-5 is the closest match (acceptance tests) but only covers multi-pane, graceful no-op, and hook survival scenarios. Adding an upgrade-path test to Task 3-5 verifies the spec's stated breaking-change behavior and ensures old entries don't persist or cause issues.

**Current**:
**Problem**: The plan's acceptance criteria require two specific test scenarios: (1) a session with multiple panes has independent hook entries keyed by distinct structural positions, and (2) hooks with structural keys that don't match any live panes produce no errors.

**Solution**: Add three new tests to internal/hooks/executor_test.go:
1. Multi-pane independent hooks: session with 3 panes (0.0, 0.1, 1.0), each with independent on-resume hooks. Verify all three fire independently with correct targets.
2. Graceful no-op: hooks keyed by structural positions that no longer exist. Verify no errors and no send-keys calls for orphaned hooks.
3. Hook survival after restart: hooks exist but ListAllPanes returns empty (server just restarted). Verify CleanStale NOT called (Phase 1 guard) and hooks remain intact for when sessions are restored.

**Outcome**: Acceptance criteria for multi-pane and graceful no-op scenarios have dedicated tests. Tests verify the structural key system works end-to-end.

**Acceptance Criteria**:
- [ ] Multi-pane test: 3 panes with independent hooks, all fire correctly
- [ ] Graceful no-op test: orphaned structural keys produce no errors
- [ ] Hook survival test: empty pane list preserves hooks
- [ ] go test ./internal/hooks/... passes
- [ ] go test ./... passes (full suite)

**Tests**:
- "multi-pane independent hooks fire correctly with structural key targets"
- "orphaned structural keys produce no errors and no send-keys calls"
- "empty pane list preserves hooks for post-restart survival"

**Proposed**:
**Problem**: The plan's acceptance criteria require two specific test scenarios: (1) a session with multiple panes has independent hook entries keyed by distinct structural positions, (2) hooks with structural keys that don't match any live panes produce no errors, and (3) old pane-ID-keyed entries from before the upgrade are cleaned on first run.

**Solution**: Add four new tests to internal/hooks/executor_test.go:
1. Multi-pane independent hooks: session with 3 panes (0.0, 0.1, 1.0), each with independent on-resume hooks. Verify all three fire independently with correct targets.
2. Graceful no-op: hooks keyed by structural positions that no longer exist. Verify no errors and no send-keys calls for orphaned hooks.
3. Hook survival after restart: hooks exist but ListAllPanes returns empty (server just restarted). Verify CleanStale NOT called (Phase 1 guard) and hooks remain intact for when sessions are restored.
4. Old pane-ID entries cleaned on upgrade: store contains old pane-ID-keyed entries (%0, %3) alongside structural-key entries. ListAllPanes returns structural keys only. CleanStale removes the old pane-ID entries (they don't match any live structural key) while preserving the structural-key entries.

**Outcome**: Acceptance criteria for multi-pane, graceful no-op, and upgrade-path scenarios have dedicated tests. Tests verify the structural key system works end-to-end including the breaking-change upgrade path.

**Acceptance Criteria**:
- [ ] Multi-pane test: 3 panes with independent hooks, all fire correctly
- [ ] Graceful no-op test: orphaned structural keys produce no errors
- [ ] Hook survival test: empty pane list preserves hooks
- [ ] Upgrade path test: old pane-ID entries cleaned by CleanStale when live structural keys exist
- [ ] go test ./internal/hooks/... passes
- [ ] go test ./... passes (full suite)

**Tests**:
- "multi-pane independent hooks fire correctly with structural key targets"
- "orphaned structural keys produce no errors and no send-keys calls"
- "empty pane list preserves hooks for post-restart survival"
- "old pane-ID entries cleaned on first run after upgrade"

**Resolution**: Pending
**Notes**:
The spec explicitly calls out this upgrade behavior as a conscious decision ("This is acceptable since the current format produces broken behavior anyway"). A dedicated test documents this intentional breaking change and ensures CleanStale handles mixed old/new key formats correctly.

---
