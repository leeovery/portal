TASK: Emit the FIFO-timeout exit-path INFO hydrate: signal timeout took=3s before exec (portal-observability-layer-6-2)

ACCEPTANCE CRITERIA:
- FIFO timeout (signal never arrives within hydrateTimeout) emits one INFO hydrate: signal timeout took=3s.
- took is the hydrateTimeout time.Duration (renders took=3s, NOT a quoted string).
- signal timeout INFO precedes the hydrate: exec INFO on the timeout path.
- Existing timeout WARN, FIFO unlink, reset-preamble, marker-unset, 100ms settle sleep unchanged.
- nil-HandleTimeout fall-through (test-only, no exec) does NOT emit the signal timeout INFO.

STATUS: Complete

SPEC CONTEXT:
Spec § Hook-firing observability limit Rule 3 timeout row (966): Info("signal timeout","took",signalTimeout), 3s Duration constant renders took=3s (not quoted), then exec. Live constant is hydrateTimeout. § taxonomy (166): signal timeout renders under hydrate (not signal). Duration via Duration().String() (handler.go:288-296).

IMPLEMENTATION:
- Status: Implemented (no drift)
- Location: cmd/state_hydrate.go:360 cfg.Logger.Info("signal timeout","took",hydrateTimeout) inside handleHydrateTimeout (339-365), after retained WARN (:352) before unsetSkeletonMarkerOrLog (:362).
- Notes: Placement in handleHydrateTimeout is the natural single-fire site (documented :354-359); structurally guarantees INFO precedes exec INFO (runHydrate timeout branch :119-132 calls HandleTimeout then execShellOrHookAndExit). took passed as hydrateTimeout Duration value (not stringified) → renders took=3s. Fires once (only in handler); nil-HandleTimeout fall-through (:134) returns error without invoking handler → no INFO. WARN/unlink/preamble/marker-unset/settle-sleep (paid by runHydrate :127) preserved. Logger *slog.Logger via log.For("hydrate").

TESTS:
- Status: Adequate
- Location: cmd/state_hydrate_timeout_log_test.go
- Coverage: EmitsSignalTimeoutTookOnTimeoutPath (one INFO, took=3s); TookAttrIsDurationNotString (took.Kind()==KindDuration && took.Duration()==hydrateTimeout — load-bearing against quoted-string regression); SignalTimeoutPrecedesExecINFO; PreservesWarnUnlinkAndMarkerUnset (WARN once, FIFO unlinked, set-option -su marker-unset); NilHandleTimeout_NoSignalTimeoutNoExec. Settle-sleep preservation covered by pre-existing state_hydrate_test.go (timeout elapsed >= hydrateSettleSleep + handler-must-not-own-sleep guard). Negative cross-test: file_missing path does NOT collapse into signal timeout.
- Notes: Drives real production timeout branch via timeoutCfg (instant ErrHydrateTimeout, no 3s wait). Would fail if broken.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; logger via config seam; log.For factory; took=3s Duration rendering).
- SOLID: Good — INFO in the function owning timeout-recovery (single responsibility), no new state.
- Complexity: Low.
- Modern idioms: Yes (structured slog attrs, Duration value direct).
- Readability: Good — comment block documents spec ref, Duration-not-string rationale, placement-before-exec.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Spec Rule-3 table uses constant name signalTimeout; live code uses hydrateTimeout. Correctly handled per task instruction, but spec table + live constant name remain out of sync — a future spec-hygiene pass could reconcile the name.
