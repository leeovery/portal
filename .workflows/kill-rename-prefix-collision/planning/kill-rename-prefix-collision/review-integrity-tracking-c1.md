---
status: complete
created: 2026-06-09
cycle: 1
phase: Plan Integrity Review
topic: Kill-Rename Prefix Collision
---

# Review Tracking: Kill-Rename Prefix Collision - Integrity

## Summary

The plan is well-constructed and implementation-ready. All three tasks follow the
canonical task template, are vertically sliced into independent TDD cycles, carry
explicit and correct dependencies (Task 2 and Task 3 blocked_by Task 1 for the
`exactTarget` compile-time prerequisite), and pull the right specification context
forward. Every load-bearing factual claim was verified against the codebase:

- Function locations (`KillSession` line 352, `RenameSession` line 361,
  `PaneTargetExact` line 546, `SwitchClient` line 377) — accurate.
- The inline `"="+name` session-target inventory (5 migrate sites + the excluded
  window-level `SelectWindow` at line 936) — exactly matches `grep` of the package.
- The existing test pins (`TestKillSession` line 737 `kill-session -t my-session`,
  `TestRenameSession` line 953 `rename-session -t old-name new-name`,
  `TestHasSessionUsesExactMatchPrefix` line 443, `TestHasSessionProbe` line 533,
  `saver_pane_pid_test.go` line 41 `=_portal-saver`) — accurate; the "stays green"
  neutrality claims for migrated sites hold.
- `MockCommander{RunFunc, Calls}` (tmux_test.go lines 14-20), `package tmux_test`
  external declaration, and the existence of `package tmux` internal test files
  (`option_discriminator_internal_test.go`, `export_test.go`) — all verified.
- Spec path and exposed caller sites (`cmd/kill.go`, `cmd/state_cleanup.go`, TUI)
  — exist as described.

No Critical or Important findings. Two Minor findings below tighten
implementation-readiness by removing investigative hedging the codebase already
resolves.

## Findings

### 1. Task 1 hedges the internal-test-file decision the implementer should not have to make

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1 (kill-rename-prefix-collision-1-1) — the `exactTarget` unit-test bullet in **Do**, plus the matching **Edge Cases** bullet
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The Do step for the focused `exactTarget` unit test asks the implementer to
*investigate and decide* the test-file strategy at implementation time ("Check
whether the existing `tmux_test.go` is `package tmux` or `package tmux_test`...
OR be exercised indirectly... OR assert the prefix indirectly... Prefer the
internal-package test file"). Planning is the place to settle this, and the
codebase already settles it: `tmux_test.go` is `package tmux_test` (external,
verified line 1) and the package already carries `package tmux` internal test
files (`option_discriminator_internal_test.go`, `export_test.go`), so the
internal-test-file path is unambiguously available and is the established
convention. The branching "OR ... OR ... Prefer" phrasing leaves an implementer
re-deriving a fact the plan can state outright, and the unexported-symbol
constraint (the test cannot live in the external file) makes the internal file
the only direct option — not a "prefer". Stating it directly removes a small
design decision from the implementer and matches the spec's intent of a "focused
unit test".

**Current**:
```
- In `internal/tmux/tmux_test.go`, add a focused unit test `TestExactTarget` asserting `exactTarget("foo") == "=foo"`. Note `exactTarget` is unexported, so this test must live in the internal `package tmux` test file if one exists, OR be exercised indirectly. Check whether the existing `tmux_test.go` is `package tmux` or `package tmux_test`: the file imports `tmux` as an external package (calls `tmux.NewClient`), so it is `package tmux_test` and cannot call the unexported `exactTarget` directly. Place the `exactTarget("foo") == "=foo"` assertion in a same-package internal test file (create `internal/tmux/exact_target_internal_test.go` with `package tmux`) OR assert the prefix indirectly through `KillSession`'s observed argv. Prefer the internal-package test file for a direct, focused assertion: `if got := exactTarget("foo"); got != "=foo" { t.Errorf(...) }`.
```

**Proposed**:
```
- Add the focused `exactTarget` unit test in a same-package internal test file. `exactTarget` is unexported and `tmux_test.go` is `package tmux_test` (external — it calls `tmux.NewClient`), so it cannot reach `exactTarget` directly. The package already uses `package tmux` internal test files (e.g. `option_discriminator_internal_test.go`, `export_test.go`), so follow that convention: create `internal/tmux/exact_target_internal_test.go` with `package tmux` and a `TestExactTarget` asserting `if got := exactTarget("foo"); got != "=foo" { t.Errorf("exactTarget(\"foo\") = %q, want \"=foo\"", got) }`.
```

**Resolution**: Fixed
**Notes**: Applied verbatim to the authored tick task (tick-6570c5) Do section and the phase-1-tasks.md detail file. Auto-approved.

---

### 2. Task 1 Edge Cases bullet restates the same now-resolved "or indirect" hedge

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1 (kill-rename-prefix-collision-1-1) — third **Edge Cases** bullet
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The Edge Cases section repeats the unresolved fork from finding 1 ("must be in a
`package tmux` internal test file (or asserted indirectly via observed argv)").
Once finding 1 fixes the Do step to commit to the internal test file, this bullet
should match — keeping the genuinely useful constraint (external test file cannot
reach the unexported symbol) while dropping the resolved "or asserted indirectly"
alternative so the two sections don't disagree on the chosen approach.

**Current**:
```
- `tmux_test.go` is `package tmux_test` (external) and cannot reach the unexported `exactTarget` — the focused unit assertion must be in a `package tmux` internal test file (or asserted indirectly via observed argv).
```

**Proposed**:
```
- `tmux_test.go` is `package tmux_test` (external) and cannot reach the unexported `exactTarget`, so the focused unit assertion lives in the `package tmux` internal test file `exact_target_internal_test.go` (per the Do step). The regression tests, which need `MockCommander`, stay in the external `tmux_test.go` and drive the exported `KillSession`.
```

**Resolution**: Fixed
**Notes**: Applied verbatim to the authored tick task (tick-6570c5) Edge Cases section and the phase-1-tasks.md detail file. Auto-approved.

---
