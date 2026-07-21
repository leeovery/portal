---
status: complete
created: 2026-07-21
cycle: 1
phase: Plan Integrity Review
topic: Spawned Window Dead-Ends On Session Exit
---

# Review Tracking: Spawned Window Dead-Ends On Session Exit - Integrity

## Findings

### 1. Task 1-1 is missing the required `Outcome` field

**Severity**: Minor
**Plan Reference**: Phase 1 → Task `spawned-window-dead-ends-on-session-exit-1-1` (tick-5f6bf3)
**Category**: Task Template Compliance
**Change Type**: add-to-task

**Details**:
The canonical task template (task-design.md, Field Requirements) lists **Outcome** as a required field — "one sentence minimum — what success looks like". Task 1-1 supplies Problem, Solution, Do, Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference, but has no dedicated `Outcome` field. The verifiable end state is implied by the Solution clause ("...so that when the session command finishes the wrapper execs the user's interactive login shell into the window (visible AND usable)...") and by the Acceptance Criteria, so implementation is not blocked — this is a template-completeness gap rather than an ambiguity. Adding an explicit Outcome makes the success state a first-class, standalone statement per the template. The Outcome below is drawn from content already present in the task (Problem/Solution/Context); it introduces no new scope.

**Current**:

**Solution**: Scoped entirely to the native Ghostty adapter (`internal/spawn/ghostty.go`), wrap the composed open argv as: `bash -lc '<composed open argv>; exec "$SHELL" -il'` — so that when the session command finishes the wrapper execs the user's interactive login shell into the window (visible AND usable) — and drop `wait after command` from the osascript since the exec'd shell, not the flag, now keeps the window alive. Build the wrapper as a real 3-element argv `[bash, -lc, PAYLOAD]` and render it through the existing shared shell-quote helper `renderCommandString` so the inner per-element single-quotes nest correctly via the POSIX close-escape-reopen idiom (`'\''`) rather than naive string concatenation. Both burst entry points (picker multi-select and `portal open` multi-target) benefit automatically via the shared adapter.

**Do**:
1. In `internal/spawn/ghostty.go` add a pure helper `wrapWithShellFallback(command []string) []string`...

**Proposed**:

**Solution**: Scoped entirely to the native Ghostty adapter (`internal/spawn/ghostty.go`), wrap the composed open argv as: `bash -lc '<composed open argv>; exec "$SHELL" -il'` — so that when the session command finishes the wrapper execs the user's interactive login shell into the window (visible AND usable) — and drop `wait after command` from the osascript since the exec'd shell, not the flag, now keeps the window alive. Build the wrapper as a real 3-element argv `[bash, -lc, PAYLOAD]` and render it through the existing shared shell-quote helper `renderCommandString` so the inner per-element single-quotes nest correctly via the POSIX close-escape-reopen idiom (`'\''`) rather than naive string concatenation. Both burst entry points (picker multi-select and `portal open` multi-target) benefit automatically via the shared adapter.

**Outcome**: A burst-spawned (N−1 external) native-Ghostty window, after its session exits or detaches, lands at the user's normal interactive login-shell prompt — visible and usable — instead of the "Process exited. Press any key to close the terminal." dead-end; the change is confined to `internal/spawn/ghostty.go`, both burst entry points inherit the fix through the shared adapter, and the composed open argv and all other spawn/attach behaviour are unchanged.

**Do**:
1. In `internal/spawn/ghostty.go` add a pure helper `wrapWithShellFallback(command []string) []string`...

**Resolution**: Fixed
**Notes**: Applied verbatim — `Outcome` field inserted into the tick task (tick-5f6bf3) between Solution and Do. Approved via auto mode.

---
