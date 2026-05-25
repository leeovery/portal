TASK: 8-3 — Unify the pgrep-portal-daemon regex pattern and enumeration helper

STATUS: Complete

SPEC CONTEXT: Analysis-cycle refactor (c2#2). Spec § Component B canonicalises `pgrep -fx '^portal state daemon( |$)'`. Before T8-3 regex declared in three places.

IMPLEMENTATION:
- Status: Implemented (planned follow-up in T9-1 completed)
- Locations:
  - `internal/state/daemon_identity.go:34-47` — exported `PortalDaemonArgvPattern` const; `daemonArgvPattern` regex compiled from constant
  - `internal/portaltest/pgrep.go:22-24` — `PgrepPortalDaemons() ([]int, error)` thin forwarder
  - `internal/state/pgrep.go:49-83` — canonical `state.PgrepPortalDaemons` body (added by T9-1)
  - `internal/bootstrapadapter/orphan_sweep.go:40` — adapter wires `Pgrep: state.PgrepPortalDaemons`
- End state (T8-3 + T9-1): single regex declaration, single canonical pgrep implementation in `internal/state` with two thin forwarders
- Repo-wide grep confirms literal regex only in `internal/state/daemon_identity.go`

TESTS:
- Status: Adequate
- ~14 call sites consume `portaltest.PgrepPortalDaemons()` across integration test suite
- Exit-1+empty-stdout shape exercised whenever fresh isolated state dir bootstrapped
- Multi-PID parse loop exercised by 3-daemon harness tests

CODE QUALITY:
- Project conventions: Followed; leaf-package primitive in `internal/state`; no `t.Parallel`; no production import of `internal/portaltest`
- SOLID: Single source of truth; interface segregation
- Complexity: Low; exec → exit-classify → parse loop
- Modern idioms: `errors.As` for `*exec.ExitError`; `strings.Split`; `strconv.Atoi`
- Readability: Doc explicitly documents three-shape contract; explains `-fx` (not `-fxc`) for BSD pgrep portability

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] No direct unit test for `state.PgrepPortalDaemons`'s exit-1+empty-stdout vs other-exit branching; integration coverage adequate but small table-driven unit test would lock three-shape contract independently
