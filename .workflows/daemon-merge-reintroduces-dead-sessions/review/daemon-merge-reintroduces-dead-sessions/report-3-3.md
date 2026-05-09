TASK: Extract shared stateDir + logger preamble for cmd/bootstrap integration tests (3-3)

ACCEPTANCE CRITERIA:
- Nine sites collapse to one helper.
- `t.TempDir()`-rooted state dir, `t.Setenv("PORTAL_STATE_DIR", ...)`, `state.EnsureDir()`, non-rotating logger with `t.Cleanup` for close.
- Helper gated `//go:build integration`.

STATUS: Issues Found

SPEC CONTEXT: Cycle 1 analysis flagged ~9 lines of stateDir + logger boilerplate duplicated across 9 cmd/bootstrap integration test sites. Severity: low.

IMPLEMENTATION:
- Status: Implemented (with deviation from spec)
- Location: `cmd/bootstrap/orchestrator_builder_test.go:83-116`
- Notes:
  - Implementer split the spec's single helper into two: `newIntegrationStateDir(t) string` (lines 108-116) and `openTestLogger(t, stateDir) *state.Logger` (lines 87-95). Justification (lines 102-107) explains the orchestrator end-to-end smoke test wires no logger.
  - Helpers placed in `orchestrator_builder_test.go` rather than dedicated `integration_helpers_test.go`. Acceptable colocation.
  - All 9 former call sites consume helpers cleanly:
    - reboot_roundtrip_test.go:180, 325 / 888, 928 / 1112, 1170
    - phase5_marker_suppression_integration_test.go:77, 137
    - scrollback_resumption_test.go:77, 119 / 188, 209 / 269, 303
    - phase5_integration_test.go:146, 186 / 242, 289

TESTS:
- Status: N/A — test-helper refactor.
- Coverage: All 9 call-site tests use helpers. Behavior preservation rests on existing integration suites.

CODE QUALITY:
- Project conventions: Followed. `t.Helper()` set in both helpers; `t.Cleanup` for logger close.
- SOLID: Good — two single-purpose helpers, properly composed.
- Complexity: Low.
- Modern idioms: Yes — `t.TempDir()`, `t.Setenv`, `t.Cleanup`, `t.Helper()`.
- Readability: Good — comprehensive docstrings explain helper intent and deviation rationale.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Plan acceptance criteria explicitly required `//go:build integration` gating but `orchestrator_builder_test.go` has no build tag. Implementer cannot apply because `phase5_integration_test.go` (one of 9 call sites) is also untagged. Either update plan to acknowledge helper must be untagged, or add `//go:build integration` to phase5_integration_test.go.
- [idea] Plan listed single combined helper `setupIntegrationStateAndLogger`; implementer split into two with documented rationale. Worth confirming planning doc updated.
- [idea] Helpers live in `orchestrator_builder_test.go` rather than dedicated `integration_helpers_test.go` as spec named. Functional equivalent.
