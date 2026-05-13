---
status: complete
created: 2026-05-13
cycle: 1
phase: Traceability Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Traceability

## Findings

No findings. The plan is a faithful translation of the specification.

### Direction 1 (Spec → Plan): complete coverage

Every spec element traces to a task or phase acceptance criterion:

- **Problem / Goal** — Task 1-3 closes the conflation in `GetServerOption`; Tasks 1-1/1-2 introduce the prerequisite typed error and wiring; daemon-consumer goal is reflected in Task 1-3 outcome and Task 1-6 tests.
- **Design: `CommandError` Type** — Task 1-1 (`Stderr`/`Err` fields, three `Error()` cases, `Unwrap()`, exported struct-literal-constructable, no factory, verbatim `Stderr` storage).
- **Wiring at `RealCommander`** — Task 1-2 (`Run` + `RunRaw` identical wraps, `*exec.ExitError` `Stderr` extraction, non-ExitError empty-`Stderr` wrap, `cmd.Stderr == nil` invariant preserved, original error preserved via `Unwrap()`).
- **Mock surface** — Tasks 1-1 / 1-4 (struct-literal returns from `Commander` mocks).
- **Discrimination in `GetServerOption`** — Task 1-3 (`errors.As` extraction, three return shapes, fallthrough when `errors.As` returns false).
- **Option-absent pattern family** — Task 1-3 (unexported `optionAbsentStderrPatterns` slice, three substrings, `for`/`strings.Contains` iteration, case-sensitive, verbatim-tolerant). Compatibility floor is covered behaviourally by Task 1-4's discriminator-set test (locked to the slice).
- **Failure modes that do NOT match** — Task 1-3 (propagation) + Task 1-4 (`TestGetServerOption_TransportError` parametrised over socket-connect and `lost server`; `TestGetServerOption_NonExitErrorPropagates`).
- **`TryGetServerOption` body unchanged** — Task 1-3 (`Do` explicitly states "Do not modify `TryGetServerOption`").
- **Consumer surface unchanged** — Task 1-3 (`Do` explicitly states "Do not modify any daemon consumer code").
- **`ErrOptionNotFound` unchanged** — Task 1-3 (preserved as sentinel `var`).
- **Documentation & Test-Comment Updates items 1-4** — Task 1-7.
- **Documentation & Test-Comment Updates item 5** — Task 1-6 (remove comment block + add replacement test).
- **Testing → internal/tmux/tmux_test.go** (5 spec items) — Task 1-4 (reshape, `TestGetServerOption_TransportError`, `TestGetServerOption_NonExitErrorPropagates`, `TestTryGetServerOption_PropagatesTransportError`, discriminator-set).
- **Testing → cmd/state_daemon_run_test.go** — Task 1-6 (`defaultShutdownFlush` err-branch test + `tick()` audit-or-add).
- **Testing → internal/state/markers_test.go** (continues to pass unchanged) — Phase Acceptance bullet 9 (penultimate).
- **Testing → internal/tmux Commander layer** — Task 1-5 (`TestRealCommander_RunWrapsExitError`, `TestRealCommander_RunWrapsNonExitError`).
- **Test policy reminders** (no `t.Parallel()`) — Tasks 1-4, 1-5, 1-6 each restate the constraint.
- **Pre-implementation sweep** (production + test) — Phase Acceptance bullet 10 (last); Task 1-6 also reinvokes the sweep for the `tick()` coverage decision.
- **Acceptance Criteria 1-8** — all mapped into Phase Acceptance bullets 3-9.
- **Risk & Rollout — Platform applicability (Darwin + Linux)** — Task 1-5 (defensive `t.Skip` if `sh` not on `PATH`).
- **Implementation Ordering** (units 1-5, "must land together", single PR/commit recommendation) — Phase "Why this order" captures the load-bearing rationale.
- **Alternatives Considered** — correctly omitted from the plan (non-actionable historical record).

### Direction 2 (Plan → Spec): no hallucinated content

Every plan element traces to a spec section:

- All seven tasks' Problem / Solution / Outcome paragraphs paraphrase or quote the spec's Problem, Design, and Testing sections.
- Task 1-1's `Error()` formatting cases (three branches, `<no error>` sentinel) and `Unwrap()` requirements are verbatim from spec "Type".
- Task 1-2's `cmd.Stderr == nil` invariant + inline-comment guidance is grounded in the spec's explicit warning ("load-bearing invariant", "wiring is responsible for preserving"); the inline comment is a reasonable mechanism for preserving the invariant, not new scope.
- Task 1-3's pseudocode (slice contents, iteration form, `errors.As` extraction, propagation fallthrough) matches the spec's "Iteration form" and "Fallthrough when `errors.As` returns false" wording.
- Task 1-4's six test functions enumerate the five tmux-test-file additions in the spec's Testing section plus the discriminator-set test (which the spec calls for in the same section).
- Task 1-5's `runner` helper or test-only constructor is explicitly authorised in the spec: "factor out a small `runner` helper that accepts the binary name and have the test target it".
- Task 1-6's "audit existing tick coverage; update-or-add" decision tree is verbatim from the spec's `tick()` test guidance.
- Task 1-7's site-by-site docstring edits map 1:1 to the spec's "Documentation & Test-Comment Updates" items 1-4. The `GetServerOption` docstring replacement (currently vestigial) is consistent with the spec's "Add or update a docstring describing the new contract."
- Edge-case lists on every task are drawn from the spec's enumerated edge cases (`errors.As` fallthrough, empty `Stderr` propagation, verbatim storage, ambiguous-option trailing space, non-`ExitError` underlying type, `cmd.Stderr` assignment invariant, no `t.Parallel()`, platform applicability).
- No task introduces scope outside the spec's "In scope" list. No task touches surfaces in the spec's "Out of scope" list (e.g., `Commander` signature, `ShowAllServerOptions`, `SetServerOption`/`UnsetServerOption`, daemon consumer logic, other callers, `ErrOptionNotFound` shape).
- Phase Acceptance bullets 1-10 each map to a spec acceptance criterion (1-8) or a spec section (pre-implementation sweep, markers test).

### Notes

The plan is a single-phase, seven-task decomposition that respects the spec's Implementation Ordering constraint ("(1)+(2)+(3) must land together") via single-phase grouping with explicit per-task notes that error-shape changes only become externally visible after Task 1-3 lands. Test tasks (1-4, 1-5, 1-6) cover the spec's full Testing section; docstring task (1-7) covers items 1-4 of the Documentation section; the comment-block removal (Documentation item 5) is owned by Task 1-6 alongside its replacement test, matching the spec's structural assignment.

No fixes proposed. Cycle closes clean.
