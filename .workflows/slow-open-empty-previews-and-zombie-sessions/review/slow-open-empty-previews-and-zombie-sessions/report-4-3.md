TASK: 4-3 — Implement SweepOrphanDaemons core (pgrep + legitimate-set + identity + kill)

STATUS: Complete

SPEC CONTEXT: Component B — Bootstrap-Time Orphan Sweep. Canonical pgrep enumeration; legitimate set from `_portal-saver` pane PID; best-effort, never aborts; SIGKILL only (no SIGTERM). Together with Component A converges N-orphan install to singleton in one bootstrap.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/orphan_sweep.go`
- `OrphanSweeper` interface (29-31), `*OrphanSweepCore` struct (50-92)
- All four seams (Pgrep, SaverPanePID, Identify, Kill) as function fields plus optional Logger
- SaverPanePID seam widened to tri-state `(pid, present, err)` — T10-2/T10-3 refinement; `SaverPanePIDOrAbsent` is sole production entry; encoded at type level (pid=0 cannot silently flip "absent" to "legitimate empty PID")
- Algorithm (132-196): no-op logger default; identify/kill production defaults; pgrep WARN-and-nil; saver-pane switch (err/!present/present); candidate loop with own-PID skip → legitimate-set skip → identify gating → kill with INFO-on-success / WARN-on-failure
- INFO string matches canonical `"sweep: killed orphan daemon pid=%d"`
- All log calls route through `state.ComponentBootstrap`
- Compile-time interface guard (94)
- `defaultKill` (14-16) uses `syscall.Kill(pid, SIGKILL)` — spec compliant

TESTS:
- Status: Adequate
- Location: `cmd/bootstrap/orphan_sweep_test.go`
- All 11 plan-mandated tests present + extras: `killsTwoOrphansLeavesLegitimate`, `saverAbsentKillsAllIdentifying`, `pgrepErrorLogsWarn`, `listPanesErrorTreatsLegitimateEmpty`, `identifyDead/NotPortalDaemon/TransientError`, `killErrorLogsWarnContinues`, `cleanStateZeroInfo`, `neverSIGTERM`, `defensiveOwnPIDSkip`, `pgrepEmptyListNoOp`, `emitsKilledOrphanInfo`, `nilLoggerSafe`, `presentVsAbsentTriState`, `neverReturnsError`
- Lightweight recording fakes consistent with `MarkerCleanupCore` precedent
- `neverSIGTERM` necessarily indirect (Kill seam takes only int); architectural soundness via `defaultKill` being only call site

CODE QUALITY:
- Project conventions: Followed; mirrors `stale_marker_cleanup.go`/`eager_signal_hydrate.go`; no `t.Parallel`; cmd/bootstrap pure-orchestration
- SOLID: Single responsibility; seams enable substitution; 1-method interface
- Complexity: Low; one switch + one for-loop with three guards
- Modern idioms: Yes; `map[int]struct{}` set; function-field seams; switch on saver state
- Readability: Good; extensive doc comments explain rationale (no-Pgrep-default, tri-state, SIGKILL, own-PID skip)

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] SaverPanePID seam widened post-original-spec; original plan still shows two-return shape; brief plan-history note for traceability
- [idea] `neverSIGTERM` is indirect-by-design; explanatory comment present
