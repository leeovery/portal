---
status: in-progress
created: 2026-04-06
cycle: 1
phase: Input Review
topic: resume-hooks-not-firing-after-server-kill
---

# Review Tracking: resume-hooks-not-firing-after-server-kill - Input Review

## Findings

### 1. CLI bootstrap path (bootstrap_wait) not covered in behavior table

**Source**: Investigation, Analysis > Blast Radius (lines 130-132): "`cmd/bootstrap_wait.go` -- CLI bootstrap wait also polls against a dead server. Any command depending on `PersistentPreRunE` starting the server (attach, open, kill, list)"
**Category**: Enhancement to existing topic
**Affects**: Behavior After Fix (scenario table)

**Details**:
The spec's "Behavior After Fix" table only describes TUI scenarios (portal open with no args, landing on sessions/projects page). But the investigation identifies that CLI commands with path arguments (e.g., `portal open ~/project`, `portal attach session-name`) also go through `PersistentPreRunE` and hit `bootstrap_wait.go` when the server was just started. This CLI path polls for sessions via `bootstrapWait()` (prints to stderr, polls for 1-6s). The same dead-server bug affects this path. The scenario table should document expected CLI bootstrap behavior after the fix.

**Current**:
```markdown
### Behavior After Fix

| Scenario | Expected Behavior |
|----------|-------------------|
| Server killed, resurrect installed + has save | Bootstrap session created -> continuum restores saved sessions -> resurrect replaces bootstrap session -> Portal shows restored sessions -> hooks fire |
| Server killed, resurrect installed, no save | Bootstrap session "0" persists -> Portal shows sessions page with one session |
| Server killed, resurrect NOT installed | Bootstrap session "0" persists -> Portal shows sessions page with one session |
| Server already running | No change -- `EnsureServer()` returns `(false, nil)`, `StartServer()` not called |
```

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added CLI bootstrap path row to behavior table.

---

### 2. Integration test recommended in investigation but absent from spec

**Source**: Investigation, Fix Direction > Testing Recommendations (line 163): "Integration test: bootstrap -> poll for sessions -> verify server persists"
**Category**: Enhancement to existing topic
**Affects**: Testing Requirements

**Details**:
The investigation explicitly recommends an integration-level test: "bootstrap -> poll for sessions -> verify server persists." The spec's testing requirements are all unit-level (StartServer creates session, ServerRunning returns true, etc.). The integration test validates the end-to-end flow -- that after the new bootstrap, the server remains alive long enough for polling to find sessions. This is the exact sequence that was failing.

**Current**:
```markdown
### Testing Requirements

1. `StartServer()` creates a detached session (server stays alive after call returns)
2. `ServerRunning()` returns true after the new bootstrap
3. When resurrect is not installed, bootstrap session "0" persists harmlessly
4. Existing `EnsureServer()` tests pass -- return contract unchanged
```

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Added integration test requirement to testing section.

---

### 3. Session name/parameters for `new-session -d` not specified

**Source**: Investigation, Fix Direction > Chosen Approach (lines 140-143) and Notes (line 183): "Resurrect has built-in 'restoring from scratch' handling: when it detects exactly 1 pane, it replaces the bootstrap session with saved state and cleans up the default session '0' if it wasn't in the save file."
**Category**: Enhancement to existing topic
**Affects**: Fix

**Details**:
The spec says "replace `tmux start-server` with `tmux new-session -d`" but doesn't specify what session name or parameters to use. The investigation notes that resurrect detects "exactly 1 pane" and cleans up "the default session '0'." This implies the command should be a bare `tmux new-session -d` with no explicit session name (tmux defaults to session name "0"). If a custom name were used, resurrect's cleanup logic might not recognize it. This detail matters for the resurrect integration to work correctly.

**Current**:
```markdown
### Fix

Replace `tmux start-server` with `tmux new-session -d` in `StartServer()`. This creates a detached bootstrap session that keeps the server alive during plugin initialization and continuum's delayed restore.

**Scope:** `internal/tmux/tmux.go` -- `StartServer()` function only. No changes to hooks, TUI, polling, or any other component.
```

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

