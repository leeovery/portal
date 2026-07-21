---
status: complete
created: 2026-07-21
cycle: 2
phase: Traceability Review
topic: Spawned Window Dead-Ends On Session Exit
---

# Review Tracking: Spawned Window Dead-Ends On Session Exit - Traceability

## Result

**CLEAN — no findings.** The plan is a faithful and complete translation of the specification in both directions. A full, fresh bidirectional pass was performed over the entire plan (Phase 1 + task `spawned-window-dead-ends-on-session-exit-1-1`), not narrowed to prior fixes.

## Direction 1: Specification → Plan (completeness)

Every specification element is represented in the plan with implementer-sufficient depth:

- **In-scope item 1 — wrap the native Ghostty adapter command** (`bash -lc '<composed open argv>; exec "$SHELL" -il'`): phase Goal + acceptance checkbox 1; task Do 1–2, AC 1.
- **In-scope item 2 — drop `wait after command`**: phase acceptance checkbox 2; task Do 3, AC 2.
- **In-scope item 3 — unit coverage at the command-composition seam**: phase acceptance checkboxes 4–5; task Tests + AC 1–4.
- **Constraint — PATH still carried**: task AC 3, Context, Tests.
- **Constraint — quoting nests correctly via the shared shell-quote helper (real 3-element argv, `'\''` idiom, no naive concatenation)**: task Do 1–2, AC 1 & 4, Edge Cases, Tests.
- **Constraint — `--ack` marker ordering unchanged**: task AC 5; phase acceptance checkbox 6.
- **Constraint — `syscall.Exec` attach path untouched**: task AC 5; phase acceptance checkbox 6.
- **Where the change lives (`internal/spawn/ghostty.go`)**: phase Goal, task Solution/Do.
- **Argv-agnostic (mint `--path` + optional `-- <command…>` passthrough wrapped identically to attach)**: task AC 3–4, Edge Cases, Tests.
- **Resulting shell / no `$SHELL` fallback specified / degenerate exec closes cleanly**: task Edge Cases + Context.
- **Why explicit wrapper (not implicit-append)**: task Context.
- **Why not shared composition (would inject shell metacharacters into `{command}`)**: task AC 5 + Context.
- **Accepted residual — custom `terminals.json` terminals unchanged**: task Context.
- **Accepted close-confirm trade-off + rejected mitigations (do not ship)**: task Context.
- **No change to trigger path / single-session `portal open`/attach**: task AC 5; phase acceptance checkbox 6.
- **Manual validation already performed, no deliverable gated**: task Context.
- **Acceptance Criteria 1–8**: all mapped — AC1 (Outcome), AC2 (Do 2–3, AC 1–2), AC3 (AC 3), AC4 (AC 5 + phase checkbox 6), AC5 (Solution/Outcome), AC6 (AC 5), AC7 (Tests + AC 1–4), AC8 (Context).

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every element of the plan traces to a specification section. No invented requirements, behaviours, or edge cases:

- All implementation symbols referenced by the task are codebase-grounded and verified present: `composeOpenArgv` (command.go), `ghosttyEmbed` / `ghosttyOpenScript` / `ghosttyScriptTemplate` (ghostty.go), `renderCommandString` / `shellQuote` (recipe.go), `ghostty_command_test.go`, and the current `wait after command:true` literal at ghostty.go:21. `wrapWithShellFallback` is correctly the single new symbol to add. The spec explicitly names `renderCommandString` / `shellQuote` / `composeOpenArgv`.
- The task's edge cases (quote-sensitive mint passthrough round-trip; argv-agnostic mint wrapping; degenerate `exec "$SHELL"`) are all drawn from the spec (Constraints, argv-agnostic paragraph, Resulting shell).
- Non-scope-expanding, codebase-grounded consequences only: the optional housekeeping to update stale `wait after command:true` comments in the `//go:build manual` test, the note that the `//go:build ghosttycompile` template guard still passes, and preserving existing test properties (percent-inert, single-quote-join, spaced-session-name, `TestGhosttyEmbed` sub-tests) are direct downstream effects of the specified change, not new scope.

## Findings

None.

---
