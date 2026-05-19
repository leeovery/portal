STATUS: findings
FINDINGS_COUNT: 4

AGENT: architecture

FINDINGS:

- FINDING: Bootstrap defensive `WriteVersionFile` silently bypasses Change 3's breadcrumb contract
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:51-59, /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:310, /Users/leeovery/Code/portal/internal/state/daemon_state.go:96-107, /Users/leeovery/Code/portal/cmd/state_daemon.go:304
  DESCRIPTION: The `portalSaverWriteVersionFile` seam wraps `state.WriteVersionFile` with `nil` logger so the Change-3 breadcrumb does not fire for the bootstrap-side defensive write. The spec's Change 3 explicitly anchors the breadcrumb at `ComponentDaemon` precisely because "the bootstrap-side defensive write (Change 1) also flows through the same helper; using ComponentDaemon keeps a single grep anchor regardless of caller." The implementation defeats that contract for the very caller Change 1 introduces. Defect 3 investigations grepping `portal.log` for `daemon.version write:` will see entries for the daemon's own startup write but not for the bootstrap-survived-path repair, producing a misleading paper trail.
  RECOMMENDATION: Introduce a package-level logger sink in `internal/tmux` for `portalSaverWriteVersionFile` (mirroring the existing `killBarrierLogger` + `SetBarrierLogger` pattern), or change `portalSaverWriteVersionFile`'s signature to accept a logger. Wire from `internal/bootstrapadapter` at the same site that calls `SetBarrierLogger`.

- FINDING: `shouldKillSaverOnVersionDecision` duplicates dev-short-circuit logic from `portalSaverVersionMismatch`
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:334-358, /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:390-401
  DESCRIPTION: Two predicates now encode overlapping dev-build rules: `shouldKillSaverOnVersionDecision` (new, authoritative gate) and `portalSaverVersionMismatch` (existing). The dev short-circuit and read-error-is-mismatch behaviours are reproduced inline in both, with the in-source comment explicitly noting they are "byte-equivalent in semantics." This is the parallel-computation anti-pattern. The two predicates differ only in their handling of `ErrVersionFileAbsent` (true vs false). Future changes to dev semantics must be applied in both places or the kill decision and the predicate contract drift apart silently.
  RECOMMENDATION: Compose, don't duplicate. Derive `shouldKillSaverOnVersionDecision` from `portalSaverVersionMismatch`: `if errors.Is(readErr, state.ErrVersionFileAbsent) { return false }; return portalSaverVersionMismatch(stored, currentVersion, readErr)`. Alternative: drop `portalSaverVersionMismatch` entirely and reframe its test against `shouldKillSaverOnVersionDecision` directly.

- FINDING: `portalSaverVersionMismatch` is now dead production code, retained only by its test
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:390-401, /Users/leeovery/Code/portal/internal/tmux/export_test.go:18, /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:1957-2031
  DESCRIPTION: Repo-wide grep shows `portalSaverVersionMismatch` has zero production callers after this implementation. It is referenced only by its own test (via `tmux.PortalSaverVersionMismatch` re-export) and by comments. The function exists solely to satisfy a test that exists to verify the function. This is the YAGNI / dead-abstraction case: a private helper kept alive by a test pinning behaviour that nothing else depends on.
  RECOMMENDATION: Either (a) delete `portalSaverVersionMismatch` and the dedicated predicate-matrix test, reframing those test cases against `shouldKillSaverOnVersionDecision` as a single source of truth, or (b) refactor `shouldKillSaverOnVersionDecision` to delegate to `portalSaverVersionMismatch` (with the absent-file case handled as a pre-check), giving the predicate a real production caller again.

- FINDING: `WriteVersionFile` signature mixes data and observability concerns
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/state/daemon_state.go:96, /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:57-59, /Users/leeovery/Code/portal/cmd/state_daemon.go:304
  DESCRIPTION: `WriteVersionFile(dir, version string, logger *Logger) error` adds an observability parameter to what was previously a pure persistence function. Callers without a logger pass nil and rely on the Logger nil-receiver contract. The breadcrumb's "exactly one DEBUG line per call" contract is then strictly true only at the daemon call site — bootstrap-survived-path repair emits zero. The current "logger param exists but is sometimes nil" is the worst of both worlds — caller API friction without the corresponding observability guarantee.
  RECOMMENDATION: Either remove the logger parameter from `WriteVersionFile` and lift the breadcrumb to callers (each caller owns its own observability), or ensure every production call site wires a real logger.

SUMMARY: The implementation lands all three spec changes against their behavioural contracts, but introduces two parallel encodings of dev-build version semantics and silently exempts the bootstrap-survived-path defensive write from the breadcrumb grep anchor Change 3 was meant to install across all `WriteVersionFile` callers. `portalSaverVersionMismatch` is now production-dead and exists only to satisfy its own test. None block correctness; all are structural debt.
