TASK: Home the signal component — re-attribute EagerSignalHydrate's write-failure WARN and instrument the internal/state FIFO signal plumbing under signal (portal-observability-layer-5-11)

ACCEPTANCE CRITERIA:
1. EagerSignalHydrate's per-FIFO write-failure WARN renders under component signal (not hydrate), with path/error/error_class=unexpected, wrapped error passed directly.
2. Per-FIFO successful signal emits DEBUG breadcrumb under signal (silent at INFO).
3. Lower-level internal/state FIFO send/receive plumbing logs retry-ladder breadcrumb/write-failure under signal per resolved seam; no import cycle.
4. No bootstrap/hydrate-prefixed line for the signaling mechanism; grep signal: reconstructs behaviour.
5. No new attr keys; no cycle-summary INFO added.
6. Retry ladder/FIFO semantics/step-7 control flow unchanged; tests asserting old hydrate attribution updated to signal.

STATUS: Complete

SPEC CONTEXT:
Spec § Subsystem prefix taxonomy closed value space (166): signal owns the FIFO-signaling mechanism (EagerSignalHydrate, runSignalHydrate diagnostics, lower-level internal/state plumbing). Hydrate helper's own exit-path lines (incl signal timeout) stay under hydrate. Closed attrs path/error/error_class (value unexpected). No signal-sweep summary in catalog.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap/eager_signal_hydrate.go:23 (signalLogger = log.For("signal"), function-local shadow removed); :110 write-failure WARN signalLogger.Warn("eager-signal write fifo failed","path","error",err,"error_class","unexpected") (wrapped err directly); :115 success DEBUG signalLogger.Debug("fifo signalled","path"); internal/state/signal_hydrate.go:26 (signalLogger = log.For("signal")); :89 Debug("fifo signal retrying","path","error",err) per retryable transition.
- Notes: [needs-info] seam resolved option (a) (retry-ladder DEBUG in plumbing, whole-op WARN at caller), documented. Import-cycle guard holds (log imports nothing from state). DI Logger field on EagerSignalCore retained nil-tolerant unread (documented, avoids wiring churn). runSignalHydrate already homed under signalLogger (sibling task).

TESTS:
- Status: Adequate
- Location: cmd/bootstrap/eager_signal_hydrate_test.go + internal/state/signal_hydrate_test.go
- Coverage: PerFIFOWriteFailureLogsAndContinues (one WARN component=signal, path, error_class=unexpected, error KindAny errors.Is); SuccessEmitsSignalledDebugBreadcrumb (two DEBUG under signal, nothing at INFO); NoSignalingLineUnderHydrateOrBootstrap; NoCycleSummaryNorNewAttrKeys (no INFO, keys ⊆ {component,path,error,error_class}); NilLoggerTolerated; WriteFIFOSignal_EmitsRetryDebugUnderSignal (one DEBUG fifo signal retrying, error KindAny ENXIO, returns nil); RetryDebugOncePerRetryTransition (cardinality == len(SignalHydrateRetryDelays)). Existing retry-ladder suite guards no-behaviour-change.
- Notes: Behaviour-focused (exact component/level/message/attrs). No test asserts old hydrate attribution. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (Phase-5 naming signalLogger; log.For factory; no t.Parallel; snake_case; terse messages).
- SOLID: Good — seam keeps whole-op WARN at caller (path), retry detail in plumbing.
- Complexity: Low (two additive calls in loop, one in retry path).
- Modern idioms: Yes (slog attrs, errors.Is, wrapped error direct).
- Readability: Good — both signalLogger declarations document no-bare-logger shadowing rationale + import-cycle invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] signalLogger declared in two packages as var = log.For("signal") (intentional per-package factory pattern, not duplication), but the literal "signal" is repeated; a typo would silently mis-route. Consider (cross-cutting) exported const for the 15 component literals in internal/log to make the closed vocabulary compiler-checked. Pre-existing pattern across all Phase-5 loggers.
- [idea] EagerSignalCore.Logger is now declared-but-unread (documented retained for DI stability); latent dead state a future reader might re-wire; a follow-up could remove it.
