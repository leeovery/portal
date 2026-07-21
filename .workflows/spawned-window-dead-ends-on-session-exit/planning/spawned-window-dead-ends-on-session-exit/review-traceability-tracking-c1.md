---
status: complete
created: 2026-07-21
cycle: 1
phase: Traceability Review
topic: Spawned Window Dead-Ends On Session Exit
---

# Review Tracking: Spawned Window Dead-Ends On Session Exit - Traceability

## Result

**CLEAN — no findings.** The plan is a faithful, complete, bidirectional translation of the specification.

## Findings

_None._

## Analysis Summary

### Direction 1: Specification → Plan (completeness)

Every specification element is represented in the single phase (`spawned-window-dead-ends-on-session-exit-1`) and task (`spawned-window-dead-ends-on-session-exit-1-1`):

| Spec element | Plan location |
|--------------|---------------|
| Wrap command as `bash -lc '<composed open argv>; exec "$SHELL" -il'` (The Fix; AC2) | Task Do 1-2, Acceptance 1, Tests |
| Drop `wait after command` from osascript (The Fix; AC2) | Task Do 3, Acceptance 2, Tests |
| Build wrapper as real `["bash","-lc",<payload>]` argv rendered through shared shell-quote helper; `'\''` close-escape-reopen nesting (Quoting constraint) | Task Do 1-2, Acceptance 1/4, Edge Cases |
| PATH must still be carried (constraint; AC3) | Task Acceptance 3, Context, Tests |
| `--ack` marker ordering unchanged; burst confirms; logs `spawn: opened N/N` (constraint; AC4) | Task Acceptance 5; Phase Acceptance |
| `syscall.Exec` attach path / AttachConnector / connector selection untouched (constraint; AC6) | Task Acceptance 5 |
| Scoped to `internal/spawn/ghostty.go`; both burst entry points benefit; shared `composeOpenArgv`/`renderCommandString` unchanged (Where change lives; AC5/6) | Phase Goal, Task Solution, Acceptance 5 |
| argv-agnostic mint surfaces (`--path`, `-- <command…>` passthrough) (The Fix; AC3) | Task Do, Tests, Edge Cases |
| Degenerate `exec "$SHELL"` failure → clean close, no `$SHELL` fallback (Resulting shell) | Task Edge Cases |
| No shared-composition wrap; custom-terminal residual accepted (Non-Goals) | Task Context |
| Rejected Option A (Portal-owned lifecycle) / Option C (close-on-exit) not pursued (Non-Goals) | Task Context (why explicit wrapper; why not shared) |
| Close-confirm accepted residual; rejected mitigations not shipped (Trade-off; AC8) | Task Context |
| Unit coverage: wrapper shape vs escaped string, no `wait after command`, PATH prefix, quote-sensitive fixture, round-trip uncorrupted (Testing; AC7) | Phase Acceptance 4-5, Task Tests (all 5 bullets) |
| Manual validation already performed; no manual deliverable gated (Testing) | Task Context |
| AC1 behavioural landing (explicitly non-gated per spec note — "only criteria 2, 3, 7 remain to be built") | Substance captured in Task Problem/Solution as context |

Depth of coverage is sufficient — an implementer would not need to return to the spec: the task carries the exact wrapper form, the mechanism (3-element argv through `renderCommandString`), the AppleScript-escape ordering, the PATH/`-u TMUX` preservation, the argv-agnostic mint handling, the quote-nesting rationale, and the full unit-test list.

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every plan element traces to a specific spec section:

- `wrapWithShellFallback` helper + `renderCommandString(wrapWithShellFallback(command))` → spec "Recommended mechanism" (build wrapper as a real argv and render through the existing helper). The helper name is new but the mechanism is spec-prescribed.
- AppleScript-escape line ordering, `ghosttyScriptTemplate` record shape (`{command:"%s"}`), `ghosttyEmbed`/`ghosttyOpenScript` names → codebase-grounded, preserving "as today" behaviour per the spec's osascript-layer constraint.
- "percent-inert property," `//go:build ghosttycompile` template-guard, `//go:build manual` comment housekeeping → regression-preservation grounded in the spec's "unchanged in behaviour / full suite green" (AC6) and "osascript boundary stays `//go:build manual`" (Non-Goals). Not scope expansion.
- All acceptance criteria, tests, edge cases, and Context blocks map to identified spec sections (The Fix; Constraints; Scope & Non-Goals; Accepted Trade-off; Testing Requirements; Acceptance Criteria 2/3/6/7).

No content was found that lacks a specification anchor. No invented requirements, behaviours, or edge cases.

### Structural note (non-finding)

The single task omits an explicit `Outcome` field from the canonical template, but the success state (spec AC1 — the window lands at the user's normal interactive login shell prompt) is fully captured in the task's Problem ("dead-ends…") and Solution ("execs the user's interactive login shell into the window (visible AND usable)"). No spec content is lost; this is a template-shape observation for the broader structural review lens, not a traceability gap, and is recorded here only for completeness.
