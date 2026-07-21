---
status: complete
created: 2026-07-21
cycle: 2
phase: Plan Integrity Review
topic: Spawned Window Dead-Ends On Session Exit
---

# Review Tracking: Spawned Window Dead-Ends On Session Exit - Integrity

## Result

**CLEAN** — full fresh structural pass over the entire plan (Phase 1 + task
`spawned-window-dead-ends-on-session-exit-1-1`). No findings. The plan meets
structural quality standards and is implementation-ready.

## Coverage of review dimensions

- **Task Template Compliance** — All required fields present and substantive
  (Problem/Solution/Outcome/Do/Acceptance Criteria/Tests), plus Edge Cases,
  Context, and Spec Reference. Problem states WHY (spawned window dead-ends when
  its one-shot exec chain returns with no parent shell); Solution states WHAT
  (`bash -lc '<argv>; exec "$SHELL" -il'` wrapper + drop `wait after command`);
  Outcome defines the verifiable end state.
- **Code-reference accuracy** — Every referenced symbol exists and is used
  correctly: `ghosttyEmbed`, `ghosttyScriptTemplate`, `ghosttyOpenScript`
  (`internal/spawn/ghostty.go`), `renderCommandString` / `shellQuote`
  (`internal/spawn/recipe.go`), `composeOpenArgv` (`internal/spawn/command.go`),
  target test file `internal/spawn/ghostty_command_test.go`, and the
  template-guard / manual-test files named in Context.
- **Vertical Slicing** — Single complete, independently-testable increment (fix
  plus regression coverage at the existing command-composition seam).
- **Phase Structure** — Single phase justified in "Why this order"; the wrapper
  and the `wait after command` drop are two halves of one conceptual change.
  Clear phase-level acceptance criteria.
- **Dependencies / Ordering** — Single task; no explicit dependencies required.
- **Task Self-Containment** — Context pulls forward all spec decisions
  (explicit-wrapper vs implicit-append rationale, adapter-scoped vs shared
  composition, PATH-carry requirement, accepted residuals) so the task is
  executable standalone.
- **Scope / Granularity** — One TDD cycle; Do = 4 required steps + 1 optional
  housekeeping step, within bounds.
- **Acceptance Criteria Quality** — All criteria pass/fail and concrete
  (no `wait after command` substring; byte-identical composed argv for both
  surface kinds; quote-sensitive round-trip via mint `-- <command...>`
  passthrough with the doubled-backslash escape signature; PATH/`-u TMUX`
  prefix preserved; build + `go test ./...` green). Tests include the primary
  behaviour plus quote-nesting and argv-agnostic mint-parity edge cases.

## Findings

None.
