TASK: 4-4 — Wire SweepOrphanDaemons as orchestrator step 4 between Set @portal-restoring and EnsureSaver

STATUS: Complete

SPEC CONTEXT: Component B — `SweepOrphanDaemons` after `Set @portal-restoring` (step 3) and before `EnsureSaver` (step 5). Best-effort; WARN-and-swallow.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - `cmd/bootstrap/bootstrap.go:212` — `OrphanSweeper` field
  - `cmd/bootstrap/bootstrap.go:1-34` — package docstring lists 11 steps
  - `cmd/bootstrap/bootstrap.go:279-292` — step 4 invocation, debug-entry log, Warn-and-continue
  - `cmd/bootstrap/bootstrap.go:261-372` — all step-entry debug labels use new numbering
  - `internal/bootstrapadapter/orphan_sweep.go:38-46` — `NewOrphanSweeper(client, logger)` wires `state.PgrepPortalDaemons` + `tmux.SaverPanePIDOrAbsent`
  - `cmd/bootstrap_production.go:130` — production wiring
  - `CLAUDE.md:75` — eleven-step orchestrator with new step 4
  - `cmd/bootstrap/noop.go:34-37` — `NoOpOrphanSweeper`
  - `cmd/bootstrap/defaults.go:98-101, 181-183` — `WithOrphanSweeper` option + NoOp default
- Adapter uses `tmux.SaverPanePIDOrAbsent` preserving tri-state contract from Task 4-3

TESTS:
- Status: Adequate
- Coverage:
  - `bootstrap_test.go:179-200` — full 11-step ordering pinned via `equalCalls`
  - `bootstrap_test.go:1009-1023` — `TestOrchestratorRun_invokesSweepOrphanDaemonsExactlyOnce`
  - `bootstrap_test.go:1025-1048` — positional invariant via indices
  - `bootstrap_test.go:1059-1106` — continues past failure (no propagated err, no FatalError, Warn embeds "step 4 (SweepOrphanDaemons) failed" + cause)
  - `bootstrap_test.go:1113-1127` — happy path emits no WARN
  - `bootstrap_test.go:502-544` — idempotence
  - `defaults_test.go:69-70, 121-144` — NoOp default and override
  - Multiple `composition_*_integration_test.go` exercise production adapter
  - Task 4-5 E2E at `cmd/bootstrap/orphan_sweep_integration_test.go`
- "Nil OrphanSweeper panics" test not present but matches convention

CODE QUALITY:
- Project conventions: Followed; adapter sits sibling to `adapters.go`
- SOLID: Good; single-method interface; function-field seams
- Complexity: Low; step 4 is 4 lines in `Run`
- Modern idioms: Yes; tri-state at type level
- Readability: Good; inline comment explains why-before-EnsureSaver

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Nil-field handling negative test not pinned (matches convention); optional add: `TestOrchestratorRun_panicsOnNilOrphanSweeper`
- [idea] `*state.Logger` implicitly satisfies `bootstrap.Logger`; compile-time assertion `var _ bootstrap.Logger = (*state.Logger)(nil)` would lock contract
