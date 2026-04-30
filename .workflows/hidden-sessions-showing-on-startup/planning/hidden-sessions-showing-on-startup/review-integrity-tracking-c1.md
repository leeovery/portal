---
status: in-progress
created: 2026-04-30
cycle: 1
phase: Plan Integrity Review
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Integrity

## Findings

### 1. Misleading test-location guidance for `TestStartServer` [FIXED]

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md`, Task 2-1, "Do" section, fourth bullet
**Category**: File-Path / Line-Number Accuracy
**Change Type**: update-task

**Details**:
Task 2-1's Do step instructs the implementer to "Locate the existing `TestStartServer` in `internal/tmux/tmux_test.go` (alongside `TestListSessions` at line 44 onwards)". `TestStartServer` is actually at `internal/tmux/tmux_test.go:404`, not adjacent to `TestListSessions` (which is at line 44). The phrasing implies the two tests are co-located. An implementer skimming the file from line 44 will not find `TestStartServer` quickly. Minor — easily resolved by `grep` — but worth correcting because the rest of the plan's line citations are precise and this one stands out.

**Current**:
```
- Locate the existing `TestStartServer` in `internal/tmux/tmux_test.go` (alongside `TestListSessions` at line 44 onwards). Update it to assert that the recorded `Commander.Run` invocation contains the args `-s` and `tmux.PortalBootstrapName` in addition to the existing `new-session -d` assertion. Reference the value via the exported constant — do NOT compare against the literal `"_portal-bootstrap"` string.
```

**Proposed**:
```
- Locate the existing `TestStartServer` in `internal/tmux/tmux_test.go` (currently at line 404 onwards; `TestListSessions` and `MockCommander` live earlier in the same file at lines 12-100+). Update it to assert that the recorded `Commander.Run` invocation contains the args `-s` and `tmux.PortalBootstrapName` in addition to the existing `new-session -d` assertion. Reference the value via the exported constant — do NOT compare against the literal `"_portal-bootstrap"` string.
```

**Resolution**: Fixed
**Notes**: Applied verbatim to Task 2-1 "Do" section in `phase-2-tasks.md`.

---

### 2. `Client.ListSessions` line range off by 2; `ListSessionNames` off by 5 [FIXED]

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md`, Task 1-1 ("Do" first bullet, "Context" final paragraph)
**Category**: File-Path / Line-Number Accuracy
**Change Type**: update-task

**Details**:
Task 1-1 cites `Client.ListSessions` at "lines 106-150" and `ListSessionNames` at "lines 152-167". The actual locations in the current `internal/tmux/tmux.go` are `ListSessions` at lines 108-150 and `ListSessionNames` at lines 157-167. The drift is small but the plan elsewhere is precise about line numbers, so two near-misses on the same task are worth correcting in one shot.

**Current** (Task 1-1 "Do" first bullet):
```
- In `internal/tmux/tmux.go`, edit `Client.ListSessions` (currently at lines 106-150). After the existing parsing loop builds the `sessions` slice, add a final post-processing pass that filters out any `Session` whose `Name` satisfies `strings.HasPrefix(s.Name, "_")`. The filter is unconditional (no flag, no escape hatch) and runs **last** — after parsing and any future ordering/enrichment — so the contract "the returned slice never contains a `_*` name" survives further pipeline evolution.
```

**Proposed**:
```
- In `internal/tmux/tmux.go`, edit `Client.ListSessions` (currently at lines 108-150). After the existing parsing loop builds the `sessions` slice, add a final post-processing pass that filters out any `Session` whose `Name` satisfies `strings.HasPrefix(s.Name, "_")`. The filter is unconditional (no flag, no escape hatch) and runs **last** — after parsing and any future ordering/enrichment — so the contract "the returned slice never contains a `_*` name" survives further pipeline evolution.
```

**Current** (Task 1-1 "Do" third bullet):
```
- Do **not** touch `ListSessionNames` (lines 152-167). It is a thin wrapper around `ListSessions` and the spec mandates it stay that way ("`ListSessionNames` MUST remain a delegation to `ListSessions` — it MUST NOT bypass `ListSessions` to query tmux directly"). Leave the existing delegation in place; it inherits the filter for free.
```

**Proposed**:
```
- Do **not** touch `ListSessionNames` (lines 157-167). It is a thin wrapper around `ListSessions` and the spec mandates it stay that way ("`ListSessionNames` MUST remain a delegation to `ListSessions` — it MUST NOT bypass `ListSessions` to query tmux directly"). Leave the existing delegation in place; it inherits the filter for free.
```

**Current** (Task 1-1 "Context" final paragraph):
```
> Existing `Client.ListSessions` lives at `internal/tmux/tmux.go:106-150`. Existing `ListSessionNames` lives at `internal/tmux/tmux.go:152-167`. Existing `TestListSessions` and `MockCommander` live at `internal/tmux/tmux_test.go:12-100+`.
```

**Proposed**:
```
> Existing `Client.ListSessions` lives at `internal/tmux/tmux.go:108-150`. Existing `ListSessionNames` lives at `internal/tmux/tmux.go:157-167`. Existing `TestListSessions` and `MockCommander` live at `internal/tmux/tmux_test.go:12-100+`. Existing `TestStartServer` lives at `internal/tmux/tmux_test.go:404+`.
```

**Resolution**: Fixed
**Notes**: All three citations updated in `phase-1-tasks.md` Task 1-1: "Do" first bullet (106-150 → 108-150), "Do" third bullet (152-167 → 157-167), and Context final paragraph (both ranges + added `TestStartServer` line citation).

---

### 3. Inconsistent task-header style across phase task files

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` (Tasks 1-1, 1-2 headers) vs `phase-2-tasks.md` (Tasks 2-1, 2-2, 2-3 headers)
**Category**: Internal Consistency
**Change Type**: update-task

**Details**:
Phase 2's task headers follow the canonical task template ("### Task N: ..." — e.g. "### Task 2-1: Add ..."). Phase 1's headers use a non-numbered form ("### Task: Add underscore-prefix filter..."). Both styles are unambiguous but mixing them across the two phase files is a minor consistency issue. Aligning Phase 1's headers to the Phase 2 / task-design.md style improves at-a-glance scanning and matches the canonical template ("Task N: [Clear action statement]").

**Current** (`phase-1-tasks.md`):
```
## hidden-sessions-showing-on-startup-1-1 | approved

### Task: Add underscore-prefix filter to `Client.ListSessions` with unit test
```

**Proposed**:
```
## hidden-sessions-showing-on-startup-1-1 | approved

### Task 1-1: Add underscore-prefix filter to `Client.ListSessions` with unit test
```

**Current** (`phase-1-tasks.md`):
```
## hidden-sessions-showing-on-startup-1-2 | approved

### Task: Verify `cmd/list.go` empty-input contract and refresh `tmux.PortalSaverName` doc-comment
```

**Proposed**:
```
## hidden-sessions-showing-on-startup-1-2 | approved

### Task 1-2: Verify `cmd/list.go` empty-input contract and refresh `tmux.PortalSaverName` doc-comment
```

**Resolution**: Pending
**Notes**:

---
