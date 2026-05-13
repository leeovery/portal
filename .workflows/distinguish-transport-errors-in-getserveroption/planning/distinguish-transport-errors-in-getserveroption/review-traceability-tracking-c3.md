---
status: complete
created: 2026-05-13
cycle: 3
phase: Traceability Review
topic: Distinguish Transport Errors in GetServerOption
---

# Review Tracking: Distinguish Transport Errors in GetServerOption - Traceability

## Summary

Bidirectional traceability analysis performed against the validated specification.

**Direction 1 (Spec to Plan)** — every spec element has plan coverage:

- Problem & Goal → phase Goal and Task 1-3 Problem
- Design: CommandError type → Task 1-1 (Type fields, Error() formatting cases, Unwrap, no factory, exported)
- Design: RealCommander wiring → Task 1-2 (wrap on `*exec.ExitError`, empty Stderr on non-exit, cmd.Stderr invariant, Unwrap preservation)
- Design: GetServerOption discriminator → Task 1-3 (`errors.As` extraction, fallthrough behaviour, verbatim Stderr storage)
- Design: option-absent pattern family → Task 1-3 (unexported slice, three patterns, case-sensitive substring, ambiguous trailing-space note)
- Design: TryGetServerOption + consumer surface unchanged → Task 1-3 Solution/Outcome + Acceptance Criteria
- Documentation & Test-Comment Updates items 1-4 → Task 1-5
- Documentation & Test-Comment Updates item 5 → Task 1-4 (removal + replacement)
- Testing internal/tmux/tmux_test.go (reshape, transport, non-exit, try-wrapper, discriminator-set) → Task 1-3 Tests
- Testing cmd/state_daemon_run_test.go (flush test, tick test) → Task 1-4 Tests
- Testing internal/state/markers_test.go (continues to pass note) → phase Acceptance bullet
- Testing internal/tmux Commander layer (RealCommander wrap tests) → Task 1-2 Tests
- Pre-implementation sweep (production + test) → Task 1-3 Do/Tests bullets
- Scope in/out → reflected across tasks; non-goals respected
- Acceptance Criteria 1-8 → phase Acceptance + per-task Acceptance Criteria
- Implementation Ordering (1-5 must-land-together) → phase "Why this order"
- Platform applicability (Darwin + Linux) → Task 1-2 Edge Cases

**Direction 2 (Plan to Spec)** — every plan element traces back to the specification:

- Task 1-1 content (struct shape, three Error() cases, "<no error>" fallback, no factory) → spec "Type" section verbatim
- Task 1-2 content (runner helper or test-only constructor option, cmd.Stderr invariant comment, behavioural assertions) → spec "Wiring at RealCommander" and "Testing — internal/tmux — Commander layer"
- Task 1-3 content (slice contents, iteration form, fallthrough, verbatim Stderr) → spec "Design: Discrimination in GetServerOption" and "Option-absent pattern family"
- Task 1-4 content (flush test fault injection, zero-commit assertion via existing seam, optional warn-log, tick test) → spec "Testing — cmd/state_daemon_run_test.go"
- Task 1-5 content (four docstring sites, ErrOptionNotFound naming, errors.As wrapping note) → spec "Documentation & Test-Comment Updates" items 1-4

No hallucinated content found. No over-scoping. No invented edge cases — every edge case (whitespace-only Stderr, nil Err, both empty, non-ExitError underlying type, cmd.Stderr invariant, ambiguous trailing space, errors.As false fallthrough, verbatim storage tolerated, future tmux phrasing, negative unrelated stderr) maps to either spec text or test-design guidance in the spec.

The two prior integrity cycles (cycle 1 restructure + tautology fix + audit pre-resolution + quote fix; cycle 2 subtest split parity + edge-case merge) have left the plan a faithful, complete translation of the specification.

## Findings

No findings. The plan is a faithful translation of the specification in both directions.
