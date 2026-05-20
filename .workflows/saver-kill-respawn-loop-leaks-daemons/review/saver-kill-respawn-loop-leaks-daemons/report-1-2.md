TASK: Add DEBUG breadcrumb to state.WriteVersionFile under ComponentDaemon

ACCEPTANCE CRITERIA:
- WriteVersionFile emits exactly one DEBUG line per call prefixed `daemon.version write:` under state.ComponentDaemon
- Line includes version, os.Getpid(), destination path
- Log call sits before the atomic-write side effect
- Signature/return shape unchanged
- go test ./internal/state/... green; existing WriteVersionFile tests pass without modification
- go build -o portal . succeeds

STATUS: Complete

SPEC CONTEXT: Spec §Change 3 mandates a stable, greppable DEBUG breadcrumb at every daemon.version write site to support future Defect 3 investigations. Pure instrumentation — no behavioural change, no new error paths. The prefix `daemon.version write:` is contract; fmt template is implementation choice.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/daemon_state.go:90-107
- Key line: `logger.Debug(ComponentDaemon, "daemon.version write: version=%s pid=%d path=%s", version, os.Getpid(), path)` at line 105, sitting BEFORE the fileutil.AtomicWrite call at line 106.
- Component tag: ComponentDaemon (correct).
- pid evaluated at call time via os.Getpid() — not cached; satisfies edge case about distinct daemon/bootstrap pids.
- Block comment at lines 98-104 documents the ordering rationale, nil-logger semantics, and grep-contract intent.
- Production wiring: cmd/state_daemon.go:304 passes the daemon's real logger. internal/tmux/portal_saver.go:58-60 wraps with package-level versionWriterLogger sink installed by bootstrapadapter via SetVersionWriterLogger — so both the daemon-startup site AND the bootstrap-side defensive write (Task 1-4) hit the same breadcrumb format.
- Notes: The acceptance bullet "signature and return shape are unchanged" is technically violated — a logger *Logger parameter was added. However, this aligns with the package's existing logger-as-parameter pattern (e.g. SeedHashMap(dir, logger)); the package has no global logger; all call sites (production + tests) were updated.

TESTS:
- Status: Adequate
- Location: internal/state/daemon_version_breadcrumb_test.go (3 tests, 116 lines)
- Coverage:
  - TestWriteVersionFile_EmitsBreadcrumb (line 36) — asserts prefix, | DEBUG | level, | daemon | component, version=1.2.3, pid=<os.Getpid()>, path=<absolute> tokens.
  - TestWriteVersionFile_EmitsBreadcrumbEvenWhenWriteFails (line 68) — read-only dir (0o500), asserts WriteVersionFile errors AND breadcrumb is present; skips on Windows + root.
  - TestWriteVersionFile_EmitsExactlyOneBreadcrumbPerCall (line 101) — strings.Count == 1.
- Existing WriteVersionFile callers updated mechanically to pass nil for the new logger param — exercises the nil-receiver safe path.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() in new tests (per CLAUDE.md). DI seam pattern preserved via versionWriterLogger package-level var + SetVersionWriterLogger installer, matching the existing BarrierLogger / SetBarrierLogger precedent in the same file.
- SOLID: Good. Single responsibility (logging is the only added behaviour). The bootstrap-side closure keeps the seam signature `func(dir, version string) error` byte-compatible with prior tests while threading the logger.
- Complexity: Low. One added log line + one parameter.
- Modern idioms: Yes. Relies on *Logger's documented nil-receiver no-op contract — avoids defensive nil checks at the call site.
- Readability: Excellent.
- Security: No sensitive data emitted.
- Performance: One Sprintf per call; negligible, and level-filtered out when PORTAL_LOG_LEVEL != debug.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Plan acceptance criterion "WriteVersionFile's signature and return shape are unchanged" is literally violated by the added logger *Logger parameter. The implementation correctly chose the package's parameter-passing pattern over a global-logger seam (consistent with SeedHashMap, WritePIDFile neighbours), but the planning-acceptance language and executed change are mismatched.
- [idea] versionWriterLogger in internal/tmux/portal_saver.go defaults to nil and is installed via SetVersionWriterLogger from bootstrapadapter. If a future code path invokes portalSaverWriteVersionFile before the bootstrap adapter wiring runs, the breadcrumb is silently dropped on that branch.
