---
status: in-progress
created: 2026-06-03
cycle: 1
phase: Plan Integrity Review
topic: State Notify Cascade on Binary Upgrade
---

# Review Tracking: State Notify Cascade on Binary Upgrade - Integrity

## Summary

Standalone integrity review of the plan (one phase, seven tasks) for structural
quality and implementation readiness.

The plan is in strong shape. Every task carries Problem, Solution, Outcome
(folded into the opening paragraph + acceptance criteria), Do, Acceptance
Criteria, Tests, Edge Cases, and a Spec Reference. The "Do" sections name real
files, methods, and line numbers, all of which I verified against the live
codebase (`hooks_register.go:127` and `hooks_unregister.go:65` are the two
production `ShowGlobalHooks()` callers the tasks claim; the `reaped` attr,
`bootstrap` component, and `show-hooks failed` / `error_class=unexpected` WARN
shape all exist as referenced). Acceptance criteria are concrete and pass/fail.
Edge-case tests are enumerated, not just happy paths.

**Dependencies and ordering**: The graph is sound. Convergence points carry
explicit edges (1-5 ← {1-2,1-3,1-4} reflects the three production callers that
must migrate before `ShowGlobalHooks` can be deleted; 1-7 ← {1-2,1-4} reflects
self-heal needing register and teardown-at-depth needing unregister). The
capability dependencies 1-2 ← 1-1 and 1-4 ← 1-1 (both call the new
`ShowGlobalHooksForEvent` seam) are satisfied by natural creation order
(1-1 precedes 1-2 and 1-4), so per the review criteria no explicit edge is
required. No circular dependencies. No missing cross-phase edges (single phase).

**Vertical slicing / scope**: 1-1 (add seam) and 1-5 (delete old method) are
seam-first / delete-last steps of a refactor rather than independent feature
slices, but each is independently verifiable (1-1 by its own unit test, 1-5 by
full-suite green) and this seam→migrate→delete decomposition is the correct
shape for this kind of architectural shift — not flagged. 1-2 is the largest
task but is a single cohesive behaviour (the convergence engine) within one
file boundary; splitting it would create a non-independently-testable horizontal
slice — not flagged.

One minor finding: a verification grep in Task 1-5's acceptance criteria is
self-contradictory as written.

## Findings

### 1. Task 1-5 acceptance-criterion grep contradicts its own prose

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-5 (state-notify-cascade-on-binary-upgrade-1-5, tick-54b1dc) — Acceptance Criteria
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The acceptance criterion uses the word-boundary pattern `ShowGlobalHooks\b` and
then describes the expected result as "returns only ShowGlobalHooksForEvent
references (no bare ShowGlobalHooks)". A `\b` word boundary after
`ShowGlobalHooks` does **not** match `ShowGlobalHooksForEvent` (the next
character `F` is itself a word character, so there is no boundary). I verified
this empirically: `grep -rn 'ShowGlobalHooks\b'` returns the bare-form
occurrences and excludes every `ShowGlobalHooksForEvent` line. So the regex
returns the **opposite** of what the prose claims.

After Task 1-5 deletes the method and migrates all callers, the correct
post-condition for this exact regex is that it returns **nothing** (zero bare
`ShowGlobalHooks` references remain). An implementer running the literal command
and reading the prose would get a confusing, contradictory signal — the command
is meant to be a clean pass/fail gate but its stated expectation is wrong.

The fix restates the criterion so the command and its expected result agree:
the word-boundary grep for the bare form must return nothing; a separate check
confirms the per-event seam survives.

**Current**:
```
  - The ShowGlobalHooks method is removed from internal/tmux/tmux.go.
  - grep -rn "ShowGlobalHooks\b" internal cmd --include=*.go returns only ShowGlobalHooksForEvent references (no bare ShowGlobalHooks).
  - The ShowGlobalHooksOrWarn re-export is removed (or repointed at the per-event seam if showGlobalHooksOrWarn survives) and no test drives the deleted form.
  - cmd/bootstrap/reboot_roundtrip_test.go's verifyHydrationHookEntries reads via ShowGlobalHooksForEvent.
  - go build -o portal . passes and the full go test ./... suite is green with no regressions.
```

**Proposed**:
```
  - The ShowGlobalHooks method is removed from internal/tmux/tmux.go.
  - grep -rn "ShowGlobalHooks\b" internal cmd --include=*.go returns nothing — the word-boundary pattern matches only the bare no-arg form (it does NOT match ShowGlobalHooksForEvent, where the trailing 'F' suppresses the boundary), so a clean result proves every bare reference (production and test) is gone.
  - The per-event seam still exists and is the sole survivor: grep -rn "ShowGlobalHooksForEvent" internal cmd --include=*.go returns the expected ShowGlobalHooksForEvent call sites.
  - The ShowGlobalHooksOrWarn re-export is removed (or repointed at the per-event seam if showGlobalHooksOrWarn survives) and no test drives the deleted form.
  - cmd/bootstrap/reboot_roundtrip_test.go's verifyHydrationHookEntries reads via ShowGlobalHooksForEvent.
  - go build -o portal . passes and the full go test ./... suite is green with no regressions.
```

**Resolution**: Fixed
**Notes**: Applied verbatim to Task 1-5 (tick-54b1dc) — grep criterion now expects an empty result for the bare form, with a separate grep confirming the per-event seam survives.

---
