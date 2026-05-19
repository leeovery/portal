STATUS: findings
FINDINGS_COUNT: 1

AGENT: standards

FINDINGS:

- FINDING: Bootstrap-side defensive WriteVersionFile suppresses spec-mandated DEBUG breadcrumb
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:57-59, /Users/leeovery/Code/portal/internal/state/daemon_state.go:96-107
  DESCRIPTION: Spec § Change 3 mandates the breadcrumb inside `state.WriteVersionFile`. Acceptance Criterion #9 restates "Each `state.WriteVersionFile` call emits one DEBUG log line containing version, caller pid, and destination path." The spec further says "the bootstrap-side defensive write (Change 1) also flows through the same helper; using `ComponentDaemon` keeps a single grep anchor regardless of caller" — meaning the Change 1 defensive write was an explicit motivating case for the breadcrumb. The implementation routes the bootstrap-side write through a wrapper that passes a nil logger (`return state.WriteVersionFile(dir, version, nil)`), which under `Logger`'s nil-receiver no-op contract means the bootstrap path produces ZERO breadcrumbs. The author acknowledged this in a comment ("does not land for this defensive call site … wiring a real logger here can be a follow-up"), but this defeats the "single grep anchor regardless of caller" property the spec called out as desirable, and weakens the audit trail precisely on the surface (alive + absent → defensive write) Defect 3 investigations were meant to use.
  RECOMMENDATION: Thread a real `*state.Logger` to the bootstrap-side defensive call so its breadcrumb lands. The existing bootstrap wiring already constructs a `*state.Logger` for `SetBarrierLogger`; either add a `SetVersionWriterLogger` seam in `internal/tmux` mirroring the `SetBarrierLogger` precedent, or extend `EnsurePortalSaverVersion`'s wiring to accept and forward a logger to `portalSaverWriteVersionFile`. Update the seam comment to drop the "follow-up" framing once wired.

SUMMARY: One medium drift from spec — the bootstrap-side defensive `WriteVersionFile` suppresses the DEBUG breadcrumb mandated by Acceptance Criterion #9 by passing nil logger, narrowing the "single grep anchor regardless of caller" guarantee in Change 3's rationale. All other Change 1, Change 2, and Change 3 contracts match the specification.
