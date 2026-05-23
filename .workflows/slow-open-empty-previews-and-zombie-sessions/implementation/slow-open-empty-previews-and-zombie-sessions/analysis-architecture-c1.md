# Architecture Analysis — Cycle 1

STATUS: findings
FINDINGS_COUNT: 4

SUMMARY: Architecture is broadly sound — clean leaf packages, consistent step renumbering, well-documented seams. Four improvement opportunities found, mostly composition/duplication.

## Finding 1: applyHostNoiseMitigation duplicated verbatim across three test packages

SEVERITY: medium

FILES:
- `cmd/bootstrap/orphan_sweep_integration_test.go:423`
- `cmd/state_daemon_self_supervision_integration_test.go:668`
- `internal/tmux/portal_saver_endstate_integration_test.go:116`

DESCRIPTION: Bit-for-bit identical 3-line body in all three sites, each carrying an inline comment apologising for the duplication. Root cause: canonical copies live in `_test`-only files which Go can't share. Helper is a pure pre-condition for `portaltest.NewIsolatedStateEnv` to work safely on a developer machine with a live host daemon. Leaving three copies guarantees divergence the next time the helper acquires a new responsibility.

RECOMMENDATION: Promote into `internal/portaltest` (e.g. `portaltest.NeutralizeHostEnv(t)`) alongside `NewIsolatedStateEnv`. Either fold into `NewIsolatedStateEnv`'s entry (preferred — callers can't forget the ordering invariant) or expose as a separate function. Delete the three private copies.

## Finding 2: Duplicated identity-check / read-PID seam pairs in portal_saver.go

SEVERITY: medium

FILES:
- `internal/tmux/portal_saver.go:178-279`

DESCRIPTION: Two pairs are the same primitive duplicated under different names: (a) `killBarrierIdentifyDaemon` and `saverReadinessIdentify` both wrap `state.IdentifyDaemon`; (b) `killBarrierReadPID` and `saverReadinessReadPID` both wrap `state.ReadPIDFile`. Split is purely so tests can stage different canned outcomes — but no test actually composes both paths through a single stub. Package surface as a whole (12 package-level vars) is over-segmented.

RECOMMENDATION: Collapse the duplicated pairs to a single seam per primitive (`saverIdentifyDaemon`, `saverReadPID`) shared between kill-barrier escalation and readiness barrier. Net: 12 → 10 package-level seams.

## Finding 3: defaultDaemonRun post-T4-8 refactor left WriteVersionFile asymmetrically in RunE

SEVERITY: low

FILES:
- `cmd/state_daemon.go:189-209` (defaultDaemonRun: lock + WritePIDFile)
- `cmd/state_daemon.go:446` (RunE: WriteVersionFile)

DESCRIPTION: T4-8 moved `acquireDaemonLock` + `WritePIDFile` into `defaultDaemonRun`, but `WriteVersionFile` deliberately stayed at RunE. Startup writes are split across two functions. AST adjacency invariant only forbids work between acquire and pidfile write; doesn't require version-file colocation. Maintainability nit — AST test guards the live invariant.

RECOMMENDATION: Either (a) move `WriteVersionFile` into `defaultDaemonRun` immediately after `WritePIDFile`, OR (b) add a top-of-function comment listing the full startup write sequence with line refs. (a) preferred.

## Finding 4: bootstrap.Logger interface widened to 4 mandatory methods

SEVERITY: low

FILES:
- `cmd/bootstrap/bootstrap.go:171-195`

DESCRIPTION: Component B's INFO entry forced `bootstrap.Logger` to gain `Info` (T4-3), bringing the surface to Debug/Info/Warn/Error — all mandatory. Not a defect — flagged so future contributors don't bolt on Trace/Fatal without thinking.

RECOMMENDATION: No code change needed. Consider a one-line docstring addition to `bootstrap.Logger` noting "the four methods correspond to Run's emission levels; do not add a fifth without a corresponding emission site".
