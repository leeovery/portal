---
status: complete
created: 2026-02-28
cycle: 1
phase: Traceability Review
topic: tui-session-picker
---

# Review Tracking: TUI Session Picker - Traceability

## Findings

### 1. Task 3-6 Esc on Projects page in normal mode contradicts spec

**Type**: Hallucinated content
**Spec Reference**: Esc Key -- Progressive Back/Dismiss, step 4: "Sessions or Projects page (nothing active) -> exit TUI"
**Plan Reference**: Phase 3 / tui-session-picker-3-6 (tick-5c4639) -- Command-Pending Esc and Quit Behavior
**Change Type**: update-task

**Details**:
Task 3-6 has an acceptance criterion "In normal mode, Esc on Projects page still goes back to Sessions" and a corresponding test. The spec explicitly states that Esc on any page with nothing active exits the TUI -- there is no "go back to previous page" behavior for Esc. Step 4 of the progressive back says "Sessions or Projects page (nothing active) -> exit TUI." Navigation between pages is via p/s/x keys only.

This also contradicts Task 1-8 (Esc Progressive Back Behavior) which correctly states: "If on sessions or projects page with nothing active, exit the TUI."

**Current**:
```
**Acceptance Criteria**:
- [ ] Esc with nothing active in command-pending mode exits TUI
- [ ] Esc with filter active clears filter first (does not exit)
- [ ] Second Esc after clearing filter exits TUI
- [ ] Esc with modal active dismisses modal first
- [ ] Esc in file browser during command-pending returns to Projects page
- [ ] q exits from any state in command-pending mode
- [ ] In normal mode, Esc on Projects page still goes back to Sessions

**Tests**:
- "Esc with nothing active in command-pending mode exits TUI"
- "Esc with filter active clears filter first in command-pending mode"
- "two Esc presses: clear filter then exit in command-pending mode"
- "Esc with modal active dismisses modal in command-pending mode"
- "Esc in file browser returns to Projects page in command-pending mode"
- "q exits from any state in command-pending mode"
- "Esc on Projects page in normal mode goes back to Sessions"
```

**Proposed**:
```
**Acceptance Criteria**:
- [ ] Esc with nothing active in command-pending mode exits TUI
- [ ] Esc with filter active clears filter first (does not exit)
- [ ] Second Esc after clearing filter exits TUI
- [ ] Esc with modal active dismisses modal first
- [ ] Esc in file browser during command-pending returns to Projects page
- [ ] q exits from any state in command-pending mode
- [ ] In normal mode, Esc on Projects page with nothing active exits TUI (same as command-pending)

**Tests**:
- "Esc with nothing active in command-pending mode exits TUI"
- "Esc with filter active clears filter first in command-pending mode"
- "two Esc presses: clear filter then exit in command-pending mode"
- "Esc with modal active dismisses modal in command-pending mode"
- "Esc in file browser returns to Projects page in command-pending mode"
- "q exits from any state in command-pending mode"
- "Esc on Projects page in normal mode with nothing active exits TUI"
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 3-2 missing e/d keybinding disabling in command-pending mode

**Type**: Incomplete coverage
**Spec Reference**: Command-Pending Mode help bar: `[enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit` (notably excludes [e] edit and [d] delete from the normal Projects help bar)
**Plan Reference**: Phase 3 / tui-session-picker-3-2 (tick-310db8) -- Command-Pending Mode Core
**Change Type**: update-task

**Details**:
The spec's command-pending help bar excludes `e` (edit) and `d` (delete) compared to the normal Projects help bar. Task 3-2 correctly shows the right help bar content, but the Do section and acceptance criteria only mention disabling `s`, `x`, and `p`. The `e` and `d` keybindings are not mentioned as needing to be disabled. An implementer following the task would disable s/x/p but leave e/d active, which contradicts the spec's restricted help bar.

**Current**:
```
**Do**:
- Add commandPending bool and command []string fields
- Implement WithCommand(command []string) Option
- When commandPending true, do not register s, x, or p keybindings
- Help bar shows: [enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit

**Acceptance Criteria**:
- [ ] WithCommand sets commandPending = true and starts on Projects page
- [ ] Pressing s in command-pending mode does nothing
- [ ] Pressing x in command-pending mode does nothing
- [ ] Help bar does not show s or x keybindings in command-pending mode
- [ ] Normal mode still has s and x keybindings working
- [ ] enter label shows "run here" in command-pending mode

**Tests**:
- "WithCommand sets command-pending mode and starts on Projects page"
- "pressing s in command-pending mode does nothing"
- "pressing x in command-pending mode does nothing"
- "help bar omits s and x in command-pending mode"
- "help bar shows run here for enter in command-pending mode"
- "normal mode retains s and x keybindings"
```

**Proposed**:
```
**Do**:
- Add commandPending bool and command []string fields
- Implement WithCommand(command []string) Option
- When commandPending true, do not register s, x, p, e, or d keybindings (only enter, n, b, /, q remain)
- Help bar shows: [enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit

**Acceptance Criteria**:
- [ ] WithCommand sets commandPending = true and starts on Projects page
- [ ] Pressing s in command-pending mode does nothing
- [ ] Pressing x in command-pending mode does nothing
- [ ] Pressing e in command-pending mode does nothing
- [ ] Pressing d in command-pending mode does nothing
- [ ] Help bar does not show s, x, e, or d keybindings in command-pending mode
- [ ] Normal mode still has s, x, e, and d keybindings working
- [ ] enter label shows "run here" in command-pending mode

**Tests**:
- "WithCommand sets command-pending mode and starts on Projects page"
- "pressing s in command-pending mode does nothing"
- "pressing x in command-pending mode does nothing"
- "pressing e in command-pending mode does nothing"
- "pressing d in command-pending mode does nothing"
- "help bar omits s, x, e, and d in command-pending mode"
- "help bar shows run here for enter in command-pending mode"
- "normal mode retains s, x, e, and d keybindings"
```

**Resolution**: Fixed
**Notes**:

---

### 3. Task 3-2 disabling p keybinding not in spec

**Type**: Hallucinated content
**Spec Reference**: Command-Pending Mode: "Locked to the Projects page -- s and x keybindings are not registered (pressing them does nothing, they don't appear in the help bar)"
**Plan Reference**: Phase 3 / tui-session-picker-3-2 (tick-310db8) -- Command-Pending Mode Core
**Change Type**: update-task

**Details**:
Task 3-2 disables `p` alongside `s` and `x` in command-pending mode. The spec only says `s` and `x` are not registered. While `p` (go to Projects) would be a no-op since the user is already on Projects, the spec does not call for disabling it. In normal mode, `p` is shown on the Sessions help bar (not on the Projects help bar), so it would not appear anyway. Including `p` in the disabled list is an invention.

This finding overlaps with Finding 2 (same task, same field). The proposed content in Finding 2 already includes removing `p` from the disabled list. If Finding 2 is approved, this finding is resolved. If Finding 2 is rejected, this finding should be addressed independently.

**Current**:
```
- When commandPending true, do not register s, x, or p keybindings
```

**Proposed**:
```
- When commandPending true, do not register s, x, e, or d keybindings (only enter, n, b, /, q remain; p is already absent from Projects help bar)
```

**Resolution**: Fixed
**Notes**: Resolved by Finding 2's applied fix — p is no longer in the disabled list, noted as already absent from Projects help bar.
