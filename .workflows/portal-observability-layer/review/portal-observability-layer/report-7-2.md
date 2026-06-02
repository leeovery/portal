TASK: Extract the thrice-repeated "show-hooks failed" WARN + wrap block in hooks_register.go (portal-observability-layer-7-2)

ACCEPTANCE CRITERIA:
- A single showGlobalHooksOrWarn helper exists; the three call sites delegate to it.
- The emitted WARN line and wrapped error unchanged.
- Logger source across the three branches reconciled to one consistent source.
- go build / go test ./internal/tmux/... pass.

STATUS: Complete

SPEC CONTEXT:
Spec level-discipline table (280-282): log-and-continue on an unexpected error dropping a unit of work → Warn error_class=unexpected. ShowGlobalHooks failure drops a unit of work → canonical Warn("show-hooks failed","error",err,"error_class","unexpected"). Pure DRY extraction must preserve the shape byte-for-byte. Test-only log.SetTestHandler seam (73).

IMPLEMENTATION:
- Status: Implemented (faithful, no drift)
- Location: hooks_register.go:126-133 (showGlobalHooksOrWarn); call sites :153 RegisterHookIfAbsent, :255 migrateHydrationHooks, :327 migrateSessionClosedHook; export_test.go:31-35 re-export.
- Notes: Helper is the single locus of c.ShowGlobalHooks() + fmt.Errorf("show-hooks failed: %w") (grep confirms one literal). WARN shape preserved: log.OrDiscard(logger).Warn("show-hooks failed","error",err,"error_class","unexpected"). Logger reconciled to single injected *slog.Logger; RegisterHookIfAbsent (no injected logger) passes bootstrapLogger explicitly. All three → component=bootstrap. Nil-logger tolerated via log.OrDiscard.

TESTS:
- Status: Adequate
- Location: hooks_register_warn_test.go
- Coverage: ShowGlobalHooksOrWarn_FailureWrapsAndEmitsSingleWarn (("",err), show-hooks failed: prefix, errors.Is, exactly one WARN uniform shape); SuccessReturnsRawNoWarn; NilLoggerTolerated; three sibling call-site tests preserve error path; RegisterPortalHooks_ShowHooksFailureLoggedExactlyOnce (no-double-log, 10 WARNs); ShowHooksWarn_ErrorAttrCarriesCommandErrorChain (error attr is wrapped value, *CommandError reachable via errors.As).
- Notes: Behaviour-focused. Each would fail if broken. Not over-tested (direct-helper vs sibling/aggregate cover distinct concerns).

CODE QUALITY:
- Project conventions: Followed (internal/log seam; MockCommander DI; external tmux_test + export_test shim; no t.Parallel).
- SOLID: Good — single-responsibility helper; callers depend on small abstraction.
- Complexity: Low (7-line helper).
- Modern idioms: Yes (%w wrapping, *slog.Logger injection, OrDiscard nil-tolerance).
- Readability: Good — call sites collapse to two lines; RegisterHookIfAbsent comment explains why it passes bootstrapLogger.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Migrate-helpers call log.OrDiscard(logger) at their top (:253,:325) then pass the normalized logger into showGlobalHooksOrWarn which calls OrDiscard again (double-normalization, harmless, defensible since the helper must self-guard for direct-call tests); a one-line comment would preempt a future reader "simplifying" one away.
