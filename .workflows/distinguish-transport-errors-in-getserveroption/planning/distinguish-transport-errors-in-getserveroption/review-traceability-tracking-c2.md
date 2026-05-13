---
status: in-progress
created: 2026-05-13
cycle: 2
phase: Traceability Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Traceability

## Findings

No findings. The plan is a faithful translation of the specification.

### Restructure verification (cycle 1 → cycle 2)

Cycle 1 reviewed a 7-task plan and closed clean. Between cycles the plan was restructured to 5 tasks:

| Cycle-1 task                                              | Cycle-2 task                          | Disposition       |
|-----------------------------------------------------------|---------------------------------------|-------------------|
| 1-1 CommandError type                                     | 1-1 CommandError type                 | unchanged         |
| 1-2 RealCommander wiring                                  | 1-2 RealCommander wiring + tests      | absorbed 1-5      |
| 1-3 GetServerOption discriminator                         | 1-3 discriminator + tests             | absorbed 1-4      |
| 1-4 GetServerOption tests (discriminator/transport/etc.)  | (merged into new 1-3)                 | removed (absorbed)|
| 1-5 RealCommander wrap tests                              | (merged into new 1-2)                 | removed (absorbed)|
| 1-6 daemon err-branch tests                               | 1-4 daemon err-branch tests           | renumbered        |
| 1-7 docstring tightening                                  | 1-5 docstring tightening              | renumbered        |

Each merge folds the test deliverable into the task that owns the production change it verifies. No spec content was lost in the consolidation — the verification matrices below confirm bidirectional coverage holds under the new 5-task structure.

### Direction 1 (Spec → Plan): complete coverage

Every spec element traces to a task or phase acceptance criterion under the new 5-task layout:

- **Problem / Goal** — Task 1-3 closes the conflation in `GetServerOption`; Tasks 1-1/1-2 introduce the prerequisite typed error and wiring; daemon-consumer goal is reflected in Task 1-3 outcome and Task 1-4 tests.
- **Design: `CommandError` Type** — Task 1-1 (`Stderr`/`Err` fields, three `Error()` cases, `Unwrap()`, exported struct-literal-constructable, no factory, verbatim `Stderr` storage).
- **Wiring at `RealCommander`** — Task 1-2 (`Run` + `RunRaw` identical wraps, `*exec.ExitError` `Stderr` extraction, non-ExitError empty-`Stderr` wrap, `cmd.Stderr == nil` invariant preserved with inline comment, original error preserved via `Unwrap()`).
- **Mock surface** — Tasks 1-1, 1-3, 1-4 (struct-literal returns from `Commander` mocks; the daemon test in 1-4 confirms cross-package literal construction).
- **Discrimination in `GetServerOption`** — Task 1-3 (`errors.As` extraction, three return shapes, fallthrough when `errors.As` returns false).
- **Option-absent pattern family** — Task 1-3 (unexported `optionAbsentStderrPatterns` slice, three substrings, `for`/`strings.Contains` iteration, case-sensitive, verbatim-tolerant). Compatibility floor is covered behaviourally by the discriminator-set test (locked to the slice via table-driven iteration over `optionAbsentStderrPatterns` directly).
- **Failure modes that do NOT match** — Task 1-3 (propagation in `Do` + `TestGetServerOption_TransportError` parametrised over socket-connect and `lost server`; `TestGetServerOption_NonExitErrorPropagates`).
- **`TryGetServerOption` body unchanged** — Task 1-3 (`Do` explicitly states "Do not modify `TryGetServerOption`").
- **Consumer surface unchanged** — Task 1-3 (`Do` explicitly states "Do not modify any daemon consumer code").
- **`ErrOptionNotFound` unchanged** — Task 1-3 (preserved as sentinel `var`).
- **Documentation & Test-Comment Updates items 1-4** — Task 1-5 (site-by-site).
- **Documentation & Test-Comment Updates item 5** — Task 1-4 (remove comment block + add replacement test).
- **Testing → internal/tmux/tmux_test.go** (5 spec items) — Task 1-3 (reshape, `TestGetServerOption_TransportError`, `TestGetServerOption_NonExitErrorPropagates`, `TestTryGetServerOption_PropagatesTransportError`, discriminator-set).
- **Testing → cmd/state_daemon_run_test.go** — Task 1-4 (`defaultShutdownFlush` err-branch test + `tick()` err-branch test with audit-or-add fallback).
- **Testing → internal/state/markers_test.go** (continues to pass unchanged) — Phase Acceptance bullet 9 (penultimate).
- **Testing → internal/tmux Commander layer** — Task 1-2 (`TestRealCommander_RunWrapsExitError`, `TestRealCommander_RunWrapsNonExitError`, plus `runs_raw_variant` subtests for both).
- **Test policy reminders** (no `t.Parallel()`) — Tasks 1-1, 1-2, 1-3, 1-4 each restate the constraint where relevant.
- **Pre-implementation sweep** (production + test) — Task 1-3 `Do` step 1 (production-code grep) and `Tests` step 1 (test-code grep); Phase Acceptance bullet 10.
- **Acceptance Criteria 1-8** — all mapped into Phase Acceptance bullets 3-9.
- **Risk & Rollout — Platform applicability (Darwin + Linux)** — Task 1-2 Edge Cases (defensive `t.Skip` if `sh` not on `PATH`).
- **Implementation Ordering** (units 1-5, "must land together", single PR/commit recommendation) — Phase "Why this order" captures the load-bearing rationale.
- **Alternatives Considered** — correctly omitted from the plan (non-actionable historical record).

### Direction 2 (Plan → Spec): no hallucinated content

Every plan element traces to a spec section under the new 5-task layout:

- All five tasks' Problem / Solution / Outcome paragraphs paraphrase or quote the spec's Problem, Design, and Testing sections.
- Task 1-1's `Error()` formatting cases (three branches, `<no error>` sentinel) and `Unwrap()` requirements are verbatim from spec "Type".
- Task 1-2's `cmd.Stderr == nil` invariant + inline-comment guidance is grounded in the spec's explicit warning ("load-bearing invariant", "wiring is responsible for preserving"); the inline comment is a reasonable mechanism for preserving the invariant, not new scope.
- Task 1-2's `runner` helper or test-only constructor is explicitly authorised in the spec: "factor out a small `runner` helper that accepts the binary name and have the test target it".
- Task 1-2's `TestRealCommander_RunWrapsExitError/runs_raw_variant` and `TestRealCommander_RunWrapsNonExitError/runs_raw_variant` subtests cover both `Run` and `RunRaw` — the spec calls for identical wiring in both methods, so verifying both is a faithful expansion, not invention.
- Task 1-3's pseudocode (slice contents, iteration form, `errors.As` extraction, propagation fallthrough) matches the spec's "Iteration form" and "Fallthrough when `errors.As` returns false" wording.
- Task 1-3's six test functions (reshape + four explicit spec tests + discriminator-set with negative case) enumerate the five tmux-test-file items in the spec's Testing section; the discriminator-set negative case is explicitly required by the spec ("A negative case asserts an unrelated stderr does not match").
- Task 1-3's table-driven iteration over `optionAbsentStderrPatterns` directly (not hardcoded) is a faithful realisation of the spec's "discriminator-set unit tests... so the unexported optionAbsentStderrPatterns slice is directly addressable."
- Task 1-4's "audit existing tick coverage; update-or-add" decision tree is verbatim from the spec's `tick()` test guidance.
- Task 1-4's optional warn-log assertion is grounded in spec ("warn-log is an observability detail, not a correctness invariant").
- Task 1-5's site-by-site docstring edits map 1:1 to the spec's "Documentation & Test-Comment Updates" items 1-4. The `GetServerOption` docstring replacement (currently vestigial) is consistent with the spec's "Add or update a docstring describing the new contract."
- Edge-case lists on every task are drawn from the spec's enumerated edge cases (`errors.As` fallthrough, empty `Stderr` propagation, verbatim storage, ambiguous-option trailing space, non-`ExitError` underlying type, `cmd.Stderr` assignment invariant, no `t.Parallel()`, platform applicability).
- No task introduces scope outside the spec's "In scope" list. No task touches surfaces in the spec's "Out of scope" list (e.g., `Commander` signature, `ShowAllServerOptions`, `SetServerOption`/`UnsetServerOption`, daemon consumer logic, other callers, `ErrOptionNotFound` shape).
- Phase Acceptance bullets 1-10 each map to a spec acceptance criterion (1-8) or a spec section (pre-implementation sweep, markers test).

### Notes

The cycle-2 restructure consolidates test deliverables into the production-change tasks they verify, producing a tighter 5-task decomposition that still respects the spec's Implementation Ordering constraint ("(1)+(2)+(3) must land together") via single-phase grouping with explicit per-task notes that error-shape changes only become externally visible after Task 1-3 lands. Every spec element from cycle 1's coverage matrix continues to trace under the new task numbering. No hallucinated content surfaced. No fixes proposed.

Cycle closes clean.
