TASK: 9-1 — Promote pgrep enumeration to state.PgrepPortalDaemons

STATUS: Complete

SPEC CONTEXT: T8-3 introduced `portaltest.PgrepPortalDaemons` + unified `state.PortalDaemonArgvPattern` const but left byte-equivalent `pgrepPortalDaemons` private in `internal/bootstrapadapter/orphan_sweep.go`. Drift risk between prod and test enumerators.

IMPLEMENTATION:
- Status: Implemented (exceeds spec — bootstrapadapter eliminates local function entirely)
- Locations:
  - `internal/state/pgrep.go:49` — canonical `PgrepPortalDaemons()`, 83-LOC file with package-level rationale doc
  - `internal/bootstrapadapter/orphan_sweep.go:40` — wires `Pgrep: state.PgrepPortalDaemons` directly; no local function remains
  - `internal/portaltest/pgrep.go:22-24` — one-line forwarder
- Three-shape contract (`([]int, nil)` / `(nil, nil)` for exit-1-empty / `(nil, err)`) documented at `internal/state/pgrep.go:33-45`; defensive exit-0-empty-output branch at 61-67
- Uses `PortalDaemonArgvPattern` (no string-literal duplication)
- BSD-pgrep `-c`-unsupported rationale comment centralized
- File header cross-references canonical site
- 24 test call sites continue using `portaltest.PgrepPortalDaemons` unchanged

TESTS:
- Status: Adequate (for code-move refactor)
- No new dedicated unit test for `state.PgrepPortalDaemons`; exercised by existing 24-site integration surface
- Body lifted verbatim; behavioural equivalence preservation-by-construction; forwarder shape means drift would fail compilation

CODE QUALITY:
- Project conventions: Followed; package-level doc cross-links both consumers
- SOLID: Good; bootstrapadapter carries only DI wiring now
- Complexity: Low; one function, one error branch, one parse loop
- Modern idioms: `errors.As`, `fmt.Errorf %w`, `exec.Command(...).Output()`
- Readability: Good; three-shape contract enumerated explicitly

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Focused unit test (`internal/state/pgrep_test.go`) with PATH-injected pgrep stub would lock three-shape contract independent of host process state
- [idea] `PortalDaemonArgvPattern` (string const) and `daemonArgvPattern` (compiled regex) sibling forms — one-line comment on compiled var would make intent explicit
