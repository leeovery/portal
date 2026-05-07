---
status: complete
created: 2026-05-07
cycle: 6
phase: Plan Integrity Review
topic: Session Scrollback Preview
---

# Review Tracking: Session Scrollback Preview - Integrity

## Findings

### 1. Phase 4 audit task hardcodes `internal/tmux/client.go`, but the file is `internal/tmux/tmux.go`

**Severity**: Minor
**Plan Reference**: `phase-4-tasks.md` — Task 4-9 ("No-new-surface audit and regression guard"), in the **Do** section, **Acceptance Criteria**, and **Tests**
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 4-9 instructs the implementer to "Diff `internal/tmux/client.go`" and pins acceptance/test wording to that exact filename. The actual production file under `internal/tmux/` that defines `Client`, `Commander`, `RealCommander`, `CapturePane`, `ListPanesInSession`, etc. is `internal/tmux/tmux.go` — there is no `client.go` (`ls internal/tmux/` lists `tmux.go`, `tmux_test.go`, `portal_saver.go`, etc.). An implementer following the task literally would either fail to find the file (false-negative on the audit — substring searches against a non-existent file always pass) or have to silently re-target to `tmux.go`. Either outcome forces a guess and weakens the regression guard the task is meant to be. Fix is to change the three `internal/tmux/client.go` references to `internal/tmux/tmux.go`.

**Current**:
````
- Diff `internal/tmux/client.go` (and any other files in `internal/tmux/`): the only new method is the Phase 1 window-grouped pane enumeration. No new capture wrappers (no `CapturePaneTail`, no `CapturePaneN`, etc.). The existing `CapturePane` signature is unchanged.
````

```
  - Assert `internal/tmux/client.go` does not contain new symbols matching `CapturePaneTail`, `CapturePaneN`, or other capture-wrapper-shaped names — substring or regex on the file contents.
```

```
- [ ] `internal/tmux/client.go` has exactly one new method beyond pre-feature baseline (the Phase 1 enumeration method); the existing `CapturePane` signature is unchanged.
```

```
- `"audit: tmux.Client has no new capture wrapper"` — assert source of `internal/tmux/client.go` does not contain `CapturePaneTail` / `CapturePaneN` / similar.
```

**Proposed**:
````
- Diff `internal/tmux/tmux.go` (and any other files in `internal/tmux/`): the only new method is the Phase 1 window-grouped pane enumeration. No new capture wrappers (no `CapturePaneTail`, no `CapturePaneN`, etc.). The existing `CapturePane` signature is unchanged.
````

```
  - Assert `internal/tmux/tmux.go` does not contain new symbols matching `CapturePaneTail`, `CapturePaneN`, or other capture-wrapper-shaped names — substring or regex on the file contents.
```

```
- [ ] `internal/tmux/tmux.go` has exactly one new method beyond pre-feature baseline (the Phase 1 enumeration method); the existing `CapturePane` signature is unchanged.
```

```
- `"audit: tmux.Client has no new capture wrapper"` — assert source of `internal/tmux/tmux.go` does not contain `CapturePaneTail` / `CapturePaneN` / similar.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Phase 1 tasks 1-5 / 1-6 instruct `c.Commander.Run(...)`; the actual field on `*tmux.Client` is unexported `cmd`

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` — Task 1-5 (Do step 3 / Acceptance Criteria 7) and Task 1-6 (Do steps 1–2 / Acceptance Criteria 1–3 / 6 / Tests 1–3)
**Category**: Task Template Compliance / Task Self-Containment
**Change Type**: update-task

**Details**:
The plan's Do/Acceptance text refers to `c.Commander.Run(...)`, implying a `Commander` field on `*Client`. The actual struct in `internal/tmux/tmux.go` is:

```go
type Client struct {
    cmd Commander
}
```

— the field is unexported `cmd` and the `Commander` interface is the field's *type*, not its name. Production call sites all use `c.cmd.Run(...)` (e.g. `ListPanesInSession`, `ListSessions`, `ListPanes`). Following the plan literally produces non-compiling Go. The plan's secondary direction "Match the pattern used by `ListPanesInSession`" already points the implementer to the right spelling, but the literal `c.Commander.Run` references are the wrong symbol. Fix is to swap the four `c.Commander.Run` references for `c.cmd.Run` and to leave the bare `Commander.Run` references (where they refer to the interface method, not a field access on a Client receiver) untouched.

**Current** (Task 1-5, **Do** step 3):
```
- Use `c.Commander.Run(...)` (the trim variant) — pipe-delimited output does not need verbatim preservation, and trimming the trailing newline is desirable. Match the pattern used by `ListPanesInSession`.
```

**Current** (Task 1-5, **Acceptance Criteria** item 7):
```
- [ ] The method uses `c.Commander.Run` (or equivalent existing trim wrapper) — no direct `os/exec.Command` call.
```

**Current** (Task 1-6, **Do** steps 1 and 2):
```
- In the method from task 1-5, when `c.Commander.Run` returns a non-nil error, wrap it with the session name for traceability: `return nil, fmt.Errorf("list windows and panes for session %s: %w", session, err)`. Use `%w` so callers can `errors.Is` against any sentinel errors `Commander.Run` returns.
- When `c.Commander.Run` returns `(nil, nil)` (i.e. successful but empty stdout — e.g. zero panes), return `([]WindowGroup{}, nil)` explicitly, not `(nil, nil)`. The empty-but-non-nil slice signals "session exists but has no panes/windows" cleanly to callers.
```

**Proposed** (Task 1-5, **Do** step 3):
```
- Use `c.cmd.Run(...)` (the trim variant) — pipe-delimited output does not need verbatim preservation, and trimming the trailing newline is desirable. Match the pattern used by `ListPanesInSession`. (`*tmux.Client` holds the `Commander` as an unexported field named `cmd`.)
```

**Proposed** (Task 1-5, **Acceptance Criteria** item 7):
```
- [ ] The method uses `c.cmd.Run` (or equivalent existing trim wrapper on the `Commander` field) — no direct `os/exec.Command` call.
```

**Proposed** (Task 1-6, **Do** steps 1 and 2):
```
- In the method from task 1-5, when `c.cmd.Run` returns a non-nil error, wrap it with the session name for traceability: `return nil, fmt.Errorf("list windows and panes for session %s: %w", session, err)`. Use `%w` so callers can `errors.Is` against any sentinel errors the `Commander` returns.
- When `c.cmd.Run` returns `(nil, nil)` (i.e. successful but empty stdout — e.g. zero panes), return `([]WindowGroup{}, nil)` explicitly, not `(nil, nil)`. The empty-but-non-nil slice signals "session exists but has no panes/windows" cleanly to callers.
```

**Resolution**: Fixed
**Notes**: The bare `Commander.Run` mentions (in test names "it uses the Commander.Run interface", and "any sentinel errors Commander.Run exposes") refer to the interface method symbolically, not a field access, and are correct as written — leave them alone.

---
