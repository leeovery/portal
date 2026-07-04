AGENT: standards
FINDINGS:
- FINDING: Stale doc-comment names retired ListAllPanes as the AllPaneLister satisfier
  SEVERITY: medium
  FILES: cmd/bootstrap_production.go:71
  DESCRIPTION: The cleanStaleAdapter doc-comment states "production wiring passes a *tmux.Client which satisfies the interface via ListAllPanes." After this fix the AllPaneLister interface method is ListAllPaneHookKeys (cmd/clean.go:23) and *tmux.Client satisfies it via ListAllPaneHookKeys, not ListAllPanes. The comment now points a future reader at the retired name-based enumeration — the exact class of drift the spec's "retire the stale doc-comments" deliverable exists to prevent (spec §Hook-Key Derivation; Risks → Missed key-producing site). The production wiring itself is correct (Stage 2 enumeration switched to ListAllPaneHookKeys); only the prose lies.
  RECOMMENDATION: Change "satisfies the interface via ListAllPanes" to "satisfies the interface via ListAllPaneHookKeys" so the comment names the hook-key enumeration the interface actually requires.

- FINDING: Algorithm doc-comments in the shared stale-cleanup helper still say "ListAllPanes"
  SEVERITY: low
  FILES: cmd/run_hook_stale_cleanup.go:16, cmd/run_hook_stale_cleanup.go:31, cmd/run_hook_stale_cleanup.go:46, cmd/run_hook_stale_cleanup.go:66, cmd/clean.go:118, cmd/clean.go:119, cmd/clean.go:144, cmd/clean.go:150
  DESCRIPTION: The algorithm/policy prose in runHookStaleCleanup and clean.go describes the list step as "ListAllPanes" (e.g. "swallowListError (bool): how to surface a non-nil err from ListAllPanes", "1. ListAllPanes. On error emit Warn"). The live call is now lister.ListAllPaneHookKeys() (run_hook_stale_cleanup.go:89). These are name-based structural-enumeration references left over from before the Stage 2 switch; they describe hook-key cleanup as if it still enumerated name-based structural keys. Cosmetic (the code is correct) but they blur the load-bearing distinction the spec draws between the name-based enumeration (ListAllPanes / StructuralKeyFormat) and the hook-key enumeration (ListAllPaneHookKeys / HookKeyFormat).
  RECOMMENDATION: Update the prose references to ListAllPaneHookKeys (or a neutral phrasing like "the live-key enumeration") so the internal docs match the switched call and preserve the name-based-vs-hook-key distinction.

- FINDING: findOrAppendSession append branch drops PortalID (currently inert)
  SEVERITY: low
  FILES: internal/state/capture.go:260-265
  DESCRIPTION: When mergeSkippedPanes reintroduces a prev session, findOrAppendSession's append branch copies Name/Environment/Windows but not the new PortalID field. If ever reached, a merged-in session would silently lose its @portal-id, defeating the Cross-Reboot Persistence chain the fix relies on (spec §Cross-Reboot Persistence (a)/(b)). It is currently UNREACHABLE: mergeSkippedPanes skips any prev session whose name is absent from the fresh/live structure (capture.go:187), so findOrAppendSession always hits the "found" branch and the fresh session already carries its live-captured PortalID (re-stamped in createSkeleton). So no live loss today — but the omitted field is a latent trap for any future caller that reuses findOrAppendSession, and the struct-copy is now silently partial.
  RECOMMENDATION: Add PortalID: ps.PortalID to the appended Session literal for struct-copy completeness and defence-in-depth; keeps the merge path correct if the sessionLive guard is ever relaxed.

SUMMARY: The core fix conforms to spec across all four hook-key stages, cross-reboot persistence, the four retired tmux.go doc-comments, and Acceptance Criteria 1-8 (each has targeted test coverage; no t.Parallel(); integration tests use IsolateStateForTest + tmuxtest; both rename triggers covered). Findings are two doc-drift items pointing readers back at the retired name-based ListAllPanes enumeration (one confirmed at bootstrap_production.go:71, one set of internal-prose references in the stale-cleanup helper) plus a currently-inert missing PortalID copy in a merge helper.
