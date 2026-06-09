---
status: complete
created: 2026-06-09
cycle: 2
phase: Traceability Review
topic: Kill-Rename Prefix Collision
---

# Review Tracking: Kill-Rename Prefix Collision - Traceability

## Result

**CLEAN** — no findings. The plan remains a faithful, complete translation of the
specification in both directions. This cycle 2 re-check confirms the cycle 1
integrity fixes (2 minor wording corrections to Task 1's `exactTarget` internal
test-file strategy) did not introduce, drop, or untether any spec-traced content.

## Cycle 1 → Cycle 2 delta verification

Cycle 1 traceability was clean. Cycle 1 integrity applied two Minor wording fixes,
both scoped to Task 1 (kill-rename-prefix-collision-1-1):

1. The `exactTarget` unit-test **Do** bullet — replaced the investigative "OR ...
   OR ... Prefer" hedge with a committed instruction to create
   `internal/tmux/exact_target_internal_test.go` (`package tmux`, `TestExactTarget`).
2. The matching **Edge Cases** bullet — dropped the resolved "or asserted
   indirectly" alternative so it agrees with the Do step.

Both corrections are confirmed applied verbatim in `phase-1-tasks.md` (Do bullet
line 34; Edge Cases bullet line 54) and in the authored tick task `tick-6570c5`.
Neither edit changed *what* is built — both still implement the spec's "**Add** a
focused unit test: `exactTarget("foo") == "=foo"`" (§ Testing Requirements). They
only commit the Go-mechanical test-file placement (an unexported symbol cannot be
reached from the existing `package tmux_test` external file), which is a faithful
consequence of the spec, not added scope. No traceability impact.

## Direction 1: Specification → Plan (completeness)

Every specification element retains plan coverage with sufficient implementer-level detail:

| Spec element | Plan coverage |
|--------------|---------------|
| `exactTarget(session string) string` primitive (§ Chosen approach > 1) | Task 1-1 (Do, AC, Context — signature + godoc reproduced verbatim) |
| Fix `KillSession` → `exactTarget(name)` (§ 2. Fix the two destructive callers) | Task 1-1 |
| Fix `RenameSession` → `exactTarget(oldName)`, `newName` bare (§ 2) | Task 1-2 |
| RenameSession implementer trap — prefix on target only (§ 2 edge case) | Task 1-2 (Edge Cases + dedicated "keeps new name bare" argv-slot test) |
| Chokepoint fix, no caller-side change; `_portal-saver` callers gain prefix harmlessly (§ 2) | Task 1-1 (Do + Edge Cases) |
| Rationale godoc blocks on both destructive methods (§ AC) | Task 1-1 and Task 1-2 (Do + AC) |
| Migrate 5 inline `"="+name` sites: HasSession, HasSessionProbe, SwitchClient, saverPanePID, SaverPaneID (§ Sites to migrate) | Task 1-3 (Do enumerates all five with file:line) |
| Pane/window sites left as-is: SelectPane, ResizePaneZoom, SelectWindow (§ Pane/window-level sites) | Task 1-3 (explicit "Do NOT touch") |
| Explicitly out-of-scope sites: PaneTarget, bare reads/sets, pane/window writers, `display-message -t <paneID>`, quickstart bare targets (§ Explicitly out of scope) | Task 1-3 (Do + Context enumerate all) |
| Update `TestKillSession` → `kill-session -t =my-session` (§ Testing) | Task 1-1 |
| Update `TestRenameSession` → `rename-session -t =old-name new-name` (§ Testing) | Task 1-2 |
| Add prefix-collision regression tests for both, mirroring `TestHasSessionUsesExactMatchPrefix` (§ Testing) | Tasks 1-1 and 1-2 |
| Add focused `exactTarget("foo") == "=foo"` unit test (§ Testing) | Task 1-1 (incl. the now-committed internal-package test-file placement) |
| Migrated sites keep existing tests green = proof of behaviour-neutrality (§ Testing) | Task 1-3 |
| Acceptance: exact argv; colliding session never killed/renamed; no inline `"="+name` session targets remain; godoc blocks; all tests + build green (§ Acceptance criteria) | Phase 1 Acceptance + per-task AC |

**Note (not a finding):** The spec's § "Exposed user-facing callers" names
`portal kill <name>` (`cmd/kill.go`), the TUI kill key, and the TUI rename key
(`internal/tui/model.go`) as surfaces "to manually verify the wrong-session
kill/rename no longer occurs." This is explicitly a *no-caller-side-change*
informational note — the spec states the chokepoint fix "covers all of them with
no caller-side change." Nothing is buildable at those surfaces, and the acceptance
behaviour is fully captured by the Client-method regression tests in Tasks 1-1 and
1-2. No actionable plan content is missing; recorded here only for review-history
continuity (carried over from cycle 1).

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every piece of plan content still traces back to a specific specification section.
No invented requirements, approaches, edge cases, or acceptance criteria were found
in this cycle.

- Task 1-1 Problem/Solution/Do/AC/Tests/Edge Cases/Context → § Problem & Root Cause,
  § Required Behaviour & The Fix (1 & 2), § Testing Requirements & Acceptance
  Criteria. The reproduced `exactTarget` godoc and signature are verbatim from spec
  § 1. The cycle-1-revised internal-test-file detail
  (`internal/tmux/exact_target_internal_test.go`, `package tmux`) is a faithful Go
  mechanical consequence of the spec's "focused unit test" requirement against an
  unexported symbol — it does not introduce any behaviour, requirement, or scope
  absent from the spec.
- Task 1-2 → § 2 (incl. the verbatim implementer-trap edge case) and § Testing. The
  "keeps new name bare" test and argv-slot inspection
  (`["rename-session", "-t", "=foo", "bar"]`) trace directly to the spec's "prefix
  on target only; `newName` stays bare" requirement.
- Task 1-3 → § Migration Scope & Out of Scope (both the migrate list and the
  leave-as-is / out-of-scope lists) and § Testing. The "Verified inline-string
  inventory" (tmux.go:136/166/378/936, saver_pane_pid.go:49/84) and the
  SelectWindow window-level `"=" + bareTarget` exclusion match the spec's
  "leave as-is / implementer discretion."

No technical approach, behaviour, or test in the plan lacks a corresponding spec
section.

## Findings

None.

---
