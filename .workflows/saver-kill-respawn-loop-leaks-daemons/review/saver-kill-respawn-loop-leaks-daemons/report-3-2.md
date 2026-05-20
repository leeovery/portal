TASK: Thread A Real *state.Logger To The Bootstrap-Side Defensive WriteVersionFile Call (3-2)

ACCEPTANCE CRITERIA:
- Bootstrap-survived-path repair emits one DEBUG breadcrumb (version + pid + dest path) tagged ComponentDaemon
- Wrapper site no longer flags follow-up gap (i.e. no longer passes nil logger)
- Production wiring threads the real *state.Logger through

STATUS: Complete

SPEC CONTEXT: Change 3 of the spec mandates a single greppable DEBUG breadcrumb (prefix `daemon.version write:`) at every state.WriteVersionFile call, including the bootstrap-survived-path defensive write introduced by Task 1-4. The single grep anchor must hold regardless of caller. Tagged ComponentDaemon. Acceptance Criterion #9 of the spec pins the breadcrumb requirement.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/portal_saver.go:58-60 — wrapper forwards versionWriterLogger to state.WriteVersionFile
  - internal/tmux/portal_saver.go:62-71 — versionWriterLogger package-level sink (nil-safe default)
  - internal/tmux/portal_saver.go:86-91 — SetVersionWriterLogger installer (nil-tolerant)
  - internal/bootstrapadapter/adapters.go:83-87 — production wiring installs the logger inside RegisterPortalHooks alongside SetBarrierLogger
  - internal/bootstrapadapter/adapters.go:60-82 — HookRegistrar doc comment documents the new side-effect symmetrically with SetBarrierLogger
  - cmd/bootstrap_production.go:127 — production constructs HookRegistrar with the real *state.Logger
- Notes: Wrapper composition is correct — breadcrumb fires inside WriteVersionFile (internal/state/daemon_state.go:105) BEFORE the AtomicWrite. SetVersionWriterLogger mirrors SetBarrierLogger's nil-tolerance and idempotency.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tmux/portal_saver_test.go:2244-2298 — TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb pins all four tokens: count==1, level==DEBUG, component==ComponentDaemon, version=, pid=<getpid>, path=<dir>/daemon.version
  - internal/tmux/portal_saver_test.go:2300-2323 — TestSetVersionWriterLogger_IgnoresNilLogger pins the nil-tolerance contract
  - internal/tmux/export_test.go:56-67 — VersionWriterLoggerSeam and PortalSaverWriteVersionFileSeam test exports support save/restore via t.Cleanup
- Notes: Test exercises the production wrapper through the seam (no rewiring), so it guards against future regression where the wrapper might revert to passing nil.

CODE QUALITY:
- Project conventions: Followed — pattern matches SetBarrierLogger exactly.
- SOLID principles: Good — single seam, single setter, single sink.
- Complexity: Low — wrapper is a one-line lambda closing over a package-level *state.Logger.
- Modern idioms: Yes — uses Go's nil-receiver no-op contract on *state.Logger.Debug.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Wrapper indirection (portalSaverWriteVersionFile var) is a seam-as-implementation idiom — defensible because Task 1-4 needed it for stub injection, but worth revisiting if the wrapper grows.
