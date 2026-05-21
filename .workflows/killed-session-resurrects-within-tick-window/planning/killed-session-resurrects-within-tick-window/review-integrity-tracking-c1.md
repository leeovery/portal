---
status: complete
created: 2026-05-21
cycle: 1
phase: Plan Integrity Review
topic: killed-session-resurrects-within-tick-window
---

# Review Tracking: Killed Session Resurrects Within Tick Window - Integrity

## Findings

### 1. Task 1-2 ambiguous primitive selection for `@portal-restoring` query

**Severity**: Minor
**Plan Reference**: Task 1-2 (`killed-session-resurrects-within-tick-window-1-2`), Do section, first bullet
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The first Do bullet says: "query the `@portal-restoring` server option via the existing primitive used by `state.IsRestoringSet` (or call `IsRestoringSet` directly)". The "or" forces the implementer to choose between two paths without guidance. Per CLAUDE.md, `state.IsRestoringSet` is already exposed by the `state` package and is the canonical accessor used by the daemon. The task should commit to a single approach so an implementer doesn't have to make an architectural decision mid-task.

**Current**:
```markdown
- In `cmd/state_commit_now.go`, before the `ReadIndex` / `CaptureStructure` / `Commit` sequence, query the `@portal-restoring` server option via the existing primitive used by `state.IsRestoringSet` (or call `IsRestoringSet` directly).
```

**Proposed**:
```markdown
- In `cmd/state_commit_now.go`, before the `ReadIndex` / `CaptureStructure` / `Commit` sequence, call `state.IsRestoringSet(client)` (the canonical accessor used by the daemon's `tick()` body) to query the `@portal-restoring` server option. Wire it through `commitNowDeps` so tests can inject the `(false, nil)` / `(true, nil)` returns required by the acceptance criteria.
```

**Resolution**: Fixed
**Notes**: Auto-applied — pinned to state.IsRestoringSet(client) and DI through commitNowDeps.

---

### 2. Task 1-5 includes an untestable "deadlock-diagnostic" assertion

**Severity**: Minor
**Plan Reference**: Task 1-5 (`killed-session-resurrects-within-tick-window-1-5`), Tests section, fourth bullet; Acceptance Criteria, fifth bullet
**Category**: Acceptance Criteria Quality / Tests
**Change Type**: update-task

**Details**:
Task 1-5 lists a test case `"it fails with a deadlock-diagnostic message rather than a silent timeout"` and an acceptance criterion `"On hang/deadlock, the test fails with a diagnostic that distinguishes deadlock from slow progress (captures pane state and state-dir contents in the failure message)."` The Do section explains this is "forced by introducing a known-failing scenario in a sub-test or by code inspection of the timeout branch" — but code inspection isn't a test, and a "known-failing scenario" sub-test that artificially hangs would either be excluded from the suite (`t.Skip`) or fail the suite (`t.Fatal`).

The intent is clear (the timeout branch must capture diagnostic context), but the criterion as written can't be verified by `go test`. Restate as a code-level requirement on the timeout branch rather than as a runtime test case. The runtime gate stays the bounded-timeout assertion.

**Current**:
```markdown
**Acceptance Criteria**:
- [ ] Test file exists under the project's integration build tag conventions and is wired into the integration test lane.
- [ ] Test uses real tmux (via `tmuxtest` socket fixture) and a real `portal` binary (via `portalbintest`).
- [ ] Test exercises the production `RegisterPortalHooks` path to install `commitNowCommand` on `session-closed`.
- [ ] After `tmux kill-session -t B`, the test observes `sessions.json` reflecting the kill (B absent) within 1.5s, without hanging.
- [ ] On hang/deadlock, the test fails with a diagnostic that distinguishes deadlock from slow progress (captures pane state and state-dir contents in the failure message).
- [ ] Test does not use `t.Parallel()`.
- [ ] Test failure is treated as a spec-level pivot signal — the task description and PR description note that hangs here block the phase and return work to specification.

**Tests** (the test itself is the deliverable; the named cases are the assertions inside it):
- `"it does not hang when commit-now is invoked from inside the session-closed hook"` — the primary assertion (1.5s timeout)
- `"it writes a sessions.json omitting the killed session after the hook completes"`
- `"it preserves sessions other than the killed one in sessions.json"`
- `"it fails with a deadlock-diagnostic message rather than a silent timeout"` — the failure-mode assertion (forced by introducing a known-failing scenario in a sub-test or by code inspection of the timeout branch)
```

**Proposed**:
```markdown
**Acceptance Criteria**:
- [ ] Test file exists under the project's integration build tag conventions and is wired into the integration test lane.
- [ ] Test uses real tmux (via `tmuxtest` socket fixture) and a real `portal` binary (via `portalbintest`).
- [ ] Test exercises the production `RegisterPortalHooks` path to install `commitNowCommand` on `session-closed`.
- [ ] After `tmux kill-session -t B`, the test observes `sessions.json` reflecting the kill (B absent) within 1.5s, without hanging.
- [ ] The timeout branch of the test (the `context.WithTimeout` / poll-deadline path) calls `t.Fatalf` with a diagnostic string that includes (a) the current contents of the state directory, (b) the live tmux session/pane list captured via the test's tmux client, and (c) the elapsed wall time — verifiable by code inspection of the timeout branch.
- [ ] Test does not use `t.Parallel()`.
- [ ] Test failure is treated as a spec-level pivot signal — the task description and PR description note that hangs here block the phase and return work to specification.

**Tests** (the test itself is the deliverable; the named cases are the assertions inside it):
- `"it does not hang when commit-now is invoked from inside the session-closed hook"` — the primary assertion (1.5s timeout)
- `"it writes a sessions.json omitting the killed session after the hook completes"`
- `"it preserves sessions other than the killed one in sessions.json"`
```

**Resolution**: Fixed
**Notes**: The runtime-untestable "diagnostic vs silent timeout" requirement is restated as a code-inspection criterion on the timeout branch's `t.Fatalf` message. The corresponding aspirational test case is removed since it cannot be expressed as a Go test without an artificial hang injection.

---
