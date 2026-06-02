TASK: Implement Init/Close public API with stateDir/version/processRole wiring and startTime capture (portal-observability-layer-1-4)

ACCEPTANCE CRITERIA:
- After Init("/dir","0.5.0","tui"), a logger from For("daemon") (obtained before Init) writes through the configured handler with pid/version=0.5.0/process_role=tui baselines at the resolved level.
- A second Init re-points the handler (different processRole) without panicking; subsequent records carry new baselines.
- Close(0) returns normally and does NOT terminate the test process.
- Close called before any Init does not panic.
- Init captures startTime; Close computes a non-negative took.

STATUS: Complete

SPEC CONTEXT:
Spec § The internal/log package (46-85): Init configures process-wide logger, builds configured handler (per-record baselines), atomically swaps behind root via indirection; idempotent/re-entrant, no panic. Close emits "process: exit" computing took from package-private startTime, does NOT call os.Exit. Baseline injection per-record by handler. Import-cycle guard: internal/log must not import internal/state.

IMPLEMENTATION:
- Status: Implemented (Phase-1 contract intact under since-filled Phase-2 seams)
- Location:
  - internal/log/init.go:48-60 — Init resolves level, captures pid + package-private startTime, constructs handler via newTextHandler, swaps via setHandler, returns advisory openErr
  - init.go:26 — package-private startTime (reset each Init; zero harmless to Close)
  - init.go:151-153 — Close emits one "process: exit" INFO via For(processComponent) with code + computeTook(); no os.Exit
  - init.go:158-160 — computeTook() = time.Since(startTime)
  - init.go:121-128 — openLogWriter constructs rotating sink + eager probe; on failure returns os.Stderr + open error
  - log.go:94-143 — swapHandler indirection routes pre-Init cached loggers post-Init
- Notes: Idempotent (second Init re-resolves, re-opens, re-captures startTime, single atomic store). Prior *os.File intentionally left to OS rather than closed (documented) to avoid racing a concurrent Handle. Import-cycle guard enforced by AST test (init_test.go:235-256). Close-before-Init: zero startTime yields finite took; swap always holds valid handler → no panic.

TESTS:
- Status: Adequate
- Coverage: pre-Init cached logger routes after Init (asserts baselines incl os.Getpid()); second Init re-points without panic; startTime capture + non-negative took (+ reset); Close returns without os.Exit; Close safe before Init (two tests). Phase-2-era tests strengthen (resolved-level application, exit code/took/INFO level, exactly-one-exit-per-call, level-bypass visibility, stderr-fallback advisory error).
- Notes: Behaviour-focused (rendered lines / captured Record attrs). recordingHandler + snapshotInitState prevent cross-test leakage. Minor redundancy: two Close-before-Init tests overlap.

CODE QUALITY:
- Project conventions: Followed (stdlib slog, no t.Parallel, documented exports, best-effort contracts documented).
- SOLID: Good — Init delegates to emitLifecycleMarkers/openLogWriter/computeTook; swapHandler clean DI seam.
- Complexity: Low.
- Modern idioms: Yes — atomic.Pointer[slog.Handler], 0o600 literal, re-apply WithAttrs/WithGroup against live inner.
- Readability: Good — comments explain file-handle-leak-vs-close-race, per-record baselines, coarse INFO-floor Enabled, no-control-flow Close.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Close hand-builds the took pair (`"took", computeTook()`) while the package exposes log.Took(start) as the declared single source of truth for "took". Output byte-identical, but Close's took is the one summary-style attr not routed through Took; reconciling would restore the invariant.
- [idea] Two Close-before-Init tests (init_test.go:164, close_exit_test.go:163) overlap; consider consolidating.
