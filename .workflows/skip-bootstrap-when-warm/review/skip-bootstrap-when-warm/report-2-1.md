TASK: skip-bootstrap-when-warm-2-1 — Liveness-only EnsureSaver helper in package cmd (tick-66413d)

ACCEPTANCE CRITERIA:
- ensureSaverLiveness(client, stateDir) exists in package cmd and calls tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName) exactly once.
- Saver present + alive (present==true, err==nil) -> returns without calling BootstrapPortalSaver and without adding any warning.
- Saver absent (present==false, err==nil) -> calls tmux.BootstrapPortalSaver(client, stateDir).
- SaverPanePIDOrAbsent transient error (err != nil) -> treated as absent -> calls tmux.BootstrapPortalSaver.
- BootstrapPortalSaver returns an error -> exactly one bootstrap.SaverDownWarning() added to bootstrapWarnings; helper returns non-fatally (no error, no panic).
- Helper NEVER calls tmux.EnsurePortalSaverVersion and never runs a kill-barrier of its own.
- go build passes; go test ./cmd/... passes; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT: specification.md §"Abridged EnsureSaver — Liveness-Only" (lines 181-207) and §"Edge Cases … Abridged EnsureSaver hard-failure" (line 276). Class 2 "protective liveness": the warm/abridged path skips the full orchestrator but must keep _portal-saver alive across a weeks-long server lifetime because the daemon can self-eject (os.Exit(0)) mid-lifetime. A *satisfied* latch already proves the running daemon is the current binary, so the version-gate (EnsurePortalSaverVersion + kill-barrier) is redundant AND a concurrency hazard under a reopen burst — it must live solely in the full-bootstrap orchestrator step. The abridged helper is a new liveness-only function in package cmd composing SaverPanePIDOrAbsent (probe) -> BootstrapPortalSaver (idempotent re-ensure), with revive failure funneling a soft SaverDownWarning into the shared bootstrapWarnings sink and the command proceeding (line 177, 276). Spec line 125 establishes the precedent that related bootstrap failures also emit a WARN under the bootstrap component.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/abridged_saver.go:42-59 (func ensureSaverLiveness). Grounding primitives confirmed: tmux.SaverPanePIDOrAbsent (internal/tmux/saver_pane_pid.go:117), tmux.BootstrapPortalSaver (internal/tmux/portal_saver.go:591), bootstrap.SaverDownWarning (cmd/bootstrap/errors.go:64), bootstrapWarnings (cmd/bootstrap_warnings.go:64), bootstrapLogger (cmd/state_common.go:22).
- Notes: Faithfully matches every acceptance criterion.
  * SaverPanePIDOrAbsent(client, tmux.PortalSaverName) called exactly once (line 46); the single "alive" shape `present && err == nil` returns immediately.
  * Every other shape (!present OR non-nil err) folds into the revive branch — correctly mirrors the Component D "treat any error as absent" collapse for the liveness case.
  * Revive via BootstrapPortalSaver(client, stateDir) (line 55); on error adds exactly one SaverDownWarning and returns non-fatally. No error return type — matches the "command proceeds" posture.
  * Never references EnsurePortalSaverVersion in a call position (only named in the contrast docstring). Verified BootstrapPortalSaver's own internal lingering-dead barrier (portal_saver.go:594) cannot fire on the abridged absent path (HasSession returns false -> sessionPresent=false -> no KillAndWait), so the helper truly runs no kill-barrier of its own.
  * stateDir is a parameter (caller owns state.Dir() resolution) — keeps the helper unit-testable, as the task pins.
  * Docstring is thorough and load-bearing: documents Class 2, contrasts saverAdapter.EnsureSaver, and explicitly states the MUST-NOT-call-version-gate contract.
  * ADDITION beyond the literal DO text: line 56 emits one bootstrap-component WARN carrying the underlying cause before funneling the causeless SaverDownWarning. This is a justified diagnosability-parity addition (mirrors the full-bootstrap step-5 "step failed" breadcrumb; consistent with spec line 125's WARN-under-bootstrap-component precedent) and is covered by a dedicated test. The task's "do not append anything else" reads as scoped to the warnings sink (no extra sink entries) — the WARN log is a separate channel and does not violate it. No action required.

TESTS:
- Status: Adequate
- Location: cmd/abridged_saver_test.go (7 tests) + shared helper saverAbsentReviveFailsCommander (cmd/abridged_route_test.go:55).
- Coverage: All five acceptance criteria plus the added WARN emission are covered, driven end-to-end through a real *tmux.Client over the recordingCommander fake:
  * NoOpWhenSaverPresentAndAlive — asserts exactly 1 list-panes probe, zero has-session/new-session/respawn-pane/kill-session, empty sink. (criteria 1 + 2)
  * RevivesViaBootstrapPortalSaverWhenAbsent — absent via no-such-session; asserts 1 new-session (create path ran). (criterion 3)
  * TreatsProbeTransientErrorAsAbsentAndRevives — non-pid output -> ErrPanePIDParse -> transient; asserts revive attempted. (criterion 4)
  * FunnelsSaverDownWarningWhenReviveFails — asserts exactly one warning reflect.DeepEqual to bootstrap.SaverDownWarning(), non-fatal return. (criterion 5)
  * NeverInvokesVersionGate — asserts no kill-session on the alive path AND a source-level guard that abridged_saver.go contains no `EnsurePortalSaverVersion(` call (matched with the `(` so the docstring mention is not a false positive). (criterion 6)
  * LogsWarnWithUnderlyingErrorWhenReviveFails / LogsNoWarnWhenSaverPresent — cover the added WARN breadcrumb on failure and its absence on the happy path.
- Notes:
  * Tests correctly avoid t.Parallel and drain the package-global sink in resetBootstrapWarnings (before + t.Cleanup), preventing cross-test bleed. Package-level seams (BootstrapAliveCheck, PortalSaverRetryDelay) are stubbed/restored under t.Cleanup.
  * Tests would fail on regression: reversing the alive predicate breaks test 1; dropping the revive breaks test 2; treating transient-as-alive breaks test 3; dropping the funnel breaks test 4; calling the version-gate breaks test 7.
  * Not over-tested: tests 2 (absent-sentinel) and 3 (transient-error) exercise genuinely distinct SaverPanePIDOrAbsent classification branches that both fold into revive — not redundant. Tests 4 and 5 have a minor overlap (both re-assert the single SaverDownWarning) but own different concerns (warning funnel vs WARN breadcrumb), which is acceptable separation.
  * Tests 2/3 script respawn-pane to fail so BootstrapPortalSaver returns before its readiness barrier (no exported seam reachable from cmd) — an incidental warning lands in the sink but is drained on cleanup; the behavioral assertion (new-session recorded = revive attempted) is the right one and is clearly documented in-test.

CODE QUALITY:
- Project conventions: Followed. Composes existing exported primitives (no reimplementation of presence/liveness/kill logic, as the task mandates); DI via *tmux.Client over the Commander seam; error swallowed through the warning-sink posture consistent with the codebase; component logger bound once at package init. No t.Parallel in the mutating tests.
- SOLID principles: Good. Single responsibility (liveness probe + idempotent re-ensure); depends on abstractions (Commander-backed client, package seams).
- Complexity: Low. Two guarded branches, one early return.
- Modern idioms: Yes. `if _, present, err := ...; present && err == nil` scoped-init guard is idiomatic Go.
- Readability: Good. Function is 5 lines of logic; the extensive docstring is load-bearing (the MUST-NOT-call-version-gate contract and the failure posture) and matches the codebase's heavily-documented style.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
