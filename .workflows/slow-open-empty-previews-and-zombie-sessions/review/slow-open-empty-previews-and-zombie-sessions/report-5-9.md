TASK: 5-9 — Unit test selfSupervisionHysteresisTicks >= 1

STATUS: Complete

SPEC CONTEXT: Component D acceptance "Measurement artefact for N" mandates unit test asserting `>= 1` to prevent accidental zeroing. Value-vs-measurement justification by code review (5-1), not by this test. Guard intentionally weak so re-measurement doesn't force test churn.

IMPLEMENTATION:
- Status: Implemented (cosmetic naming drift)
- Location:
  - Test: `cmd/state_daemon_self_supervision_test.go:703-715` `TestSelfSupervisionHysteresisTicks_LowerBound`
  - Constant: `cmd/state_daemon.go:149` `const selfSupervisionHysteresisTicks = 3`
- Test name uses underscore vs task body's no-underscore; semantically equivalent
- Co-located with other self-supervision tests
- Single `if … < 1` guard with `t.Fatalf`
- Doc comment distinguishes from supplementary `TestSelfSupervisionHysteresisTicks_ClampInvariant` (3 ≤ N ≤ 9) in `cmd/state_daemon_test.go:726-739`

TESTS:
- Status: Adequate
- Lower-bound at N < 1 covered; constant value 3 passes; no seam coupling
- Value-based comparison survives `const → var` refactor

CODE QUALITY:
- All good for single-statement guard

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] Test name underscore drift (`_LowerBound` vs `LowerBound`); cosmetic
