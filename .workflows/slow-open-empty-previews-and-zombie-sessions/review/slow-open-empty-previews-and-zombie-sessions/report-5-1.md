TASK: 5-1 — Measure legitimate transient durations and lock in selfSupervisionHysteresisTicks with in-source provenance

STATUS: Complete

SPEC CONTEXT: Component D sole tuning knob. Required mitigation per Risk Summary. In-source provenance + single-digit ceiling + "max×2 > 5 → flag upstream defect" rule.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Constant + provenance: `cmd/state_daemon.go:124-149`
  - Harness: `cmd/state_daemon_hysteresis_measurement_test.go` (`//go:build integration`)
  - Memo: `.workflows/.../specification/.../component-d-hysteresis-measurement.md`
- `const selfSupervisionHysteresisTicks = 3` (compile-time literal); comment block names all four scenarios with measured values (0/0/0/0), 2× safety factor (→ 0), clamp to [3, 9] (→ 3), upstream-defect flag (false), measurement date (2026-05-23), binary version ("dev"), memo path

TESTS:
- Status: Adequate
- Harness is re-runnable verification artefact spec requires
- Asserts safety-factor invariant (197-202) and clamp invariant (203-206)
- Per-scenario functions cover all four spec scenarios
- No `t.Parallel`; integration build tag

CODE QUALITY:
- Project conventions: Followed; `portaltest.IsolateStateForTest`; integration tag
- SOLID: Good; `scenarioResult` separates observations; per-scenario `measure*` single-purpose
- Complexity: Low; linear flows
- Modern idioms: channels for cancellation, `math.Ceil`, `sort.Ints`
- Readability: Good; extensive inline rationale incl. PTY-absence substitution notes

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] Memo path drift: plan specifies `planning/...`; committed memo at `specification/...`. In-source comment matches committed location. Either relocate or amend plan (recommend latter — spec-adjacent more durable)
- [idea] Binary version `"dev"` rather than tagged release; for re-measurement on tagged release ldflag would propagate; document expectation in memo
- [idea] `measureAttachDetach`/`measureClientAttached` substitute `refresh-client`/`run-shell -b true` for true PTY attach; regression specifically in real `client-attached` hook fire path would silently under-measure
- [idea] Constant chose planning-time floor of 3 because all measured worst-cases were 0; could call out "max-observed × 2 = 0, raised to floor of 3" at constant site
