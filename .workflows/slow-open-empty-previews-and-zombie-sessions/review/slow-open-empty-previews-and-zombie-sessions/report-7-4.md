TASK: 7-4 — Collapse duplicated identify/read-PID seam pairs in portal_saver.go

STATUS: Complete (exceeded scope)

SPEC CONTEXT: Architecture analysis (c1, finding #2) — two pairs of seams as same primitive under different names. Resolution: collapse each pair into single shared seam.

IMPLEMENTATION:
- Status: Implemented (over-delivered — refactor went further, collapsing every saver-side mutable seam into single `SaverSeams` struct)
- Location:
  - `internal/tmux/portal_saver.go:223-231` — `SaverSeams` struct with shared `ReadPID` + `IdentifyDaemon` at top level and `Barrier/Readiness/Version/Ops` sub-clusters
  - `internal/tmux/portal_saver.go:238-272` — single package-level `saver` var
  - `internal/tmux/portal_saver.go:425` — `escalateKillToSIGKILL` calls `saver.IdentifyDaemon`
  - `internal/tmux/portal_saver.go:366` — `killSaverAndWaitForDaemon` calls `saver.ReadPID`
  - `internal/tmux/portal_saver.go:506,510` — `isSaverDaemonReady` calls both via `saver.ReadPID` + `saver.IdentifyDaemon`
  - `internal/tmux/export_test.go:103,110` — `SaverReadPIDSeam` / `SaverIdentifyDaemonSeam`
- Production references to `killBarrierIdentifyDaemon` / `saverReadinessIdentify` / `killBarrierReadPID` / `saverReadinessReadPID` are gone
- "12 → 10" package-level seam count moot — `portal_saver.go` now exposes 3 package-level vars (`BootstrapAliveCheck`, `PortalSaverRetryDelay`, `saver`); substantive intent fully met
- Godoc on `SaverSeams` (212-222) documents shared primitives as "consumed by BOTH the kill barrier and the readiness barrier"

TESTS:
- Status: Adequate
- Kill-barrier escalation stages `saver.IdentifyDaemon` via `tmux.SaverIdentifyDaemonSeam` (`portal_saver_test.go:3499`)
- Readiness barrier stages same seam via same accessor (2676)
- Kill-barrier reads PID via `tmux.SaverReadPIDSeam` (1251)
- Readiness barrier reads PID via same accessor (2669)
- Each test stages distinct outcomes through unified seam via `t.Cleanup`-based reset

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; shared seams reflect genuine shared dependency; sub-clusters orthogonal
- Complexity: Low; unification reduces moving parts
- Modern idioms: Struct-of-seams with embedded sub-structs idiomatic
- Readability: Good; field docstrings call out shared consumption

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan's "12 → 10 package-level seams" target technically obsolete; substantive intent exceeded; plan doc could be updated
