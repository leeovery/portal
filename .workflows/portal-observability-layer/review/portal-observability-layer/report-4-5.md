TASK: Close the ShowGlobalHooks failure-log asymmetry with the missing WARN branch (portal-observability-layer-4-5)

ACCEPTANCE CRITERIA:
- RegisterHookIfAbsent emits one WARN ("show-hooks failed") with wrapped err + error_class=unexpected before returning show-hooks failed: %w.
- migrateHydrationHooks emits the same WARN before returning (0, err).
- migrateSessionClosedHook's pre-existing WARN remains, consistent in component/shape.
- error attr passes wrapped error directly (not .Error()) so the line includes tmux stderr from *CommandError.
- Each show-hooks failure logged exactly once (no double-log via errors.Join aggregate).
- All three WARNs render under bootstrap component.
- Return values, abort-before-append, errors.Join folding byte-identical.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation → gap-closure sites: ShowGlobalHooks asymmetry (one branch logs, sibling doesn't) → add missing WARN per level table. Level table: log-and-return on unexpected error dropping a unit of work → WARN error_class=unexpected. error attr carries wrapped error directly.

IMPLEMENTATION:
- Status: Implemented (exceeds plan — DRY refactor instead of two parallel edits)
- Location: hooks_register.go:126-133 (shared showGlobalHooksOrWarn(c, logger): calls ShowGlobalHooks, on error log.OrDiscard(logger).Warn("show-hooks failed","error",err,"error_class","unexpected") + returns ("", fmt.Errorf("show-hooks failed: %w", err))); :13-19 (bootstrapLogger = log.For("bootstrap")); :153 RegisterHookIfAbsent delegates; :255 migrateHydrationHooks delegates returns (0,err); :327 migrateSessionClosedHook delegates (pre-existing WARN normalised).
- Notes: Only remaining c.ShowGlobalHooks() call site is inside the helper → asymmetry closed by construction. Wrapped err passed directly (not .Error()), preserving *CommandError chain. All three → component=bootstrap. RegisterPortalHooks return/abort/errors.Join untouched; no aggregate log line.

TESTS:
- Status: Adequate
- Location: hooks_register_warn_test.go + export_test.go:35
- Coverage: RegisterHookIfAbsent_ShowHooksFailureEmitsWarn (1 WARN, shape, sentinel reachable, set-hook not called); MigrateHydrationHooks_ShowHooksFailureEmitsWarn; MigrateSessionClosedHook_ShowHooksFailureWarnIsNormalized; ShowHooksWarn_ErrorAttrCarriesCommandErrorChain (real *CommandError, errors.As recovers Stderr); RegisterPortalHooks_ShowHooksFailureLoggedExactlyOnce (10 WARNs derived from event counts, no aggregate); ShowGlobalHooksOrWarn helper-direct (failure wraps+1 WARN / success raw no WARN / nil-logger tolerated). assertShowHooksWarnShape (component=bootstrap, error_class=unexpected, error non-nil error value, errors.Is sentinel).
- Notes: recordingSlogHandler merges bound component attr (meaningful). Would fail if WARN removed/mis-leveled/double-logged/.Error(). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (internal/log component logger; log.OrDiscard; error attr direct; no t.Parallel; export_test seam).
- SOLID/DRY: Improved — helper owns the WARN+wrap shape; eliminated byte-identical triple-duplication (root cause of the asymmetry).
- Complexity: Low (helper 7 lines).
- Modern idioms: Yes (slog variadic, %w, *CommandError Unwrap chain).
- Readability: Good — helper doc explains level rationale + nil-logger fallthrough.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] In production a show-hooks failure yields one WARN at the site AND one terminal ERROR at the abort boundary (bootstrap.go:289→fatal). This is correct layered logging (per-site WARN + terminal-line-before-abort ERROR, two distinct level-table rows), NOT the prohibited errors.Join aggregate double-log. Noting so a reader greppping two lines for one failure understands they're intentional and distinct.
