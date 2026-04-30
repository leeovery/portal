# Review Report: built-in-session-resurrection-13-2

**TASK**: Exercise the production `client-attached` / `signal-hydrate` pathway end-to-end in reboot round-trip tests

**ACCEPTANCE CRITERIA**:
- Add `DriveSignalHydrateBinary` that exec's the production `portal state signal-hydrate <session>` argv.
- Make binary-driven coverage the default in reboot round-trip tests.
- Retain direct-FIFO fallback (`DriveSignalHydrate`) only for the base-index drift variant for CI flake tolerance.
- Address `client-session-changed` coverage gap.
- Ensure production-binary path is the default coverage.

**STATUS**: Complete

**SPEC CONTEXT**:
Spec § "Hook Registration" registers both `client-attached` and `client-session-changed` to invoke `portal state signal-hydrate <session>` via tmux `run-shell`. Cycle 6 finding: pre-remediation tests bypassed the CLI argv → cobra dispatch → runSignalHydrate body pipeline by writing FIFO bytes directly. Task 13-2 mandates the binary-driven path become the primary coverage.

**IMPLEMENTATION**:
- Status: Implemented
- Location:
  - `internal/restoretest/restoretest.go:218-250` — `DriveSignalHydrateBinary` (new helper, exec's `portal state signal-hydrate <session>`, propagates `TMUX`, `PORTAL_STATE_DIR`, `PORTAL_HOOKS_FILE`, prepended PATH; per-session error reporting via `t.Errorf`)
  - `internal/restoretest/restoretest.go:136-192` — `DriveSignalHydrate` retained with godoc updated to spell out its fallback role.
  - `cmd/bootstrap/reboot_roundtrip_test.go:89-106` — `roundTripCfg.useBinary` selector field with thorough rationale comment.
  - `cmd/bootstrap/reboot_roundtrip_test.go:121-132` — `TestPhase5RebootRoundTripEndToEnd` defaults to `useBinary: true`.
  - `cmd/bootstrap/reboot_roundtrip_test.go:153-164` — `TestPhase5RebootRoundTripBaseIndexDrift` explicitly retains `useBinary: false` (drift variant).
  - `cmd/bootstrap/reboot_roundtrip_test.go:378-384` — driver dispatch in `runRebootRoundTrip`.
  - `cmd/bootstrap/reboot_roundtrip_test.go:719-896` — `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` (new sub-test that simulates attach-then-switch by exec'ing the binary twice in succession).
  - `cmd/bootstrap/reboot_roundtrip_test.go:898-937` — `waitForSessionMarkerCleared` (session-scoped barrier with prefix-anchored matching).
- Notes: Argv is identical to what the registered tmux hook fires via `run-shell`; `TMUX=<socket>,1,0` env propagation correctly targets the isolated test socket.

**TESTS**:
- Status: Adequate
- Coverage:
  - `client-attached` end-to-end via the binary-driven primary round-trip.
  - `client-session-changed` covered structurally via two sequential binary invocations, plus a mid-sequence marker-set assertion that confirms only the named session hydrates per call.
  - Base-index drift retains direct-FIFO driver — drift variant's contribution is structural-key resolution.
  - Pre-condition assertion (`markersBefore == 2`) prevents silent no-op regression.
- Notes: Tests gated `//go:build integration` and call `testing.Short()`. The new sub-test asserts both sequencing AND end-state.

**CODE QUALITY**:
- Project conventions: Followed. No `t.Parallel()`; new helpers consolidated into `internal/restoretest`.
- SOLID: Good. `roundTripCfg.useBinary` is a clean parameter switch.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Godoc unusually detailed.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] `DriveSignalHydrateBinary` reports per-session failures via `t.Errorf` (non-fatal) while the rest of the round-trip body uses `t.Fatalf` for setup failures. Defensible and consistent with other helpers but consider whether downstream assertions yield useful diagnostics on partial failure.
- [idea] The `client-session-changed` coverage is structural-only (sequential binary invocations). If portal ever takes a PTY dependency, revisit with a real `switch-client` driver.
