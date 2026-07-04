TASK: Collapse the triplicated @portal-id test constant in the tmux_test package (tick-70f32a / session-rename-orphans-resume-hook-analysis-1-5)

ACCEPTANCE CRITERIA:
- Exactly one @portal-id constant is declared in the tmux_test package.
- All previous usages (three consts + one inlined literal) reference the single surviving const.
- The surviving declaration keeps the import-cycle-avoidance + byte-identity comment.
- The import-cycle avoidance is unchanged (still a literal, still not session.PortalIDOption).
- go test ./internal/tmux/... compiles and passes (SkipIfNoTmux-gated tests skip cleanly where tmux is absent).

STATUS: Complete

SPEC CONTEXT: This is an analysis-phase test-scaffolding tidy (chore), not a feature from the product spec. The load-bearing invariant it protects: the "@portal-id" session user-option name is embedded byte-identically in three independent sites (session.PortalIDOption, the literal inside tmux.HookKeyFormat, and internal/state's captureFormat). The test literal must remain a spelled-out string rather than an import of session.PortalIDOption because internal/session imports internal/tmux, so the tmux (external test) package cannot import session without an import cycle. Redundant divergently-named copies of that literal within one compilation unit invite a reader to think they differ and require manual mirroring on any edit.

IMPLEMENTATION:
- Status: Implemented (converged approach — see Notes)
- Location:
  - Surviving const: internal/tmux/hookkey_test.go:17 — `const portalIDLiteral = "@portal-id"` with its explanatory comment at lines 10-16.
  - Referencing sites (all now use portalIDLiteral): hookkey_format_realtmux_test.go:66,67,119,120; list_all_pane_hookkeys_realtmux_test.go:63,64,128,129,176,177; hookkey_cross_site_realtmux_test.go:70,71,119,120; resolve_hookkey_realtmux_test.go:64,65.
- Notes: The final implementation diverges from the task's literal instruction (which named the surviving const `portalIDOption` in one of the three original files) but fully satisfies every acceptance criterion, and does so more cleanly. A pre-existing const in hookkey_test.go already declared the identical literal as `portalIDLiteral` with a superior comment (it documents all THREE embedding sites and is exercised by a tmux-less byte-identity tripwire, TestHookKeyFormatContainsPortalIDLiteral). The three duplicate consts (portalIDOption, hookKeyPortalIDOption, crossSitePortalIDOption) and the one inlined literal were all collapsed onto that surviving portalIDLiteral rather than creating yet another const. Verified by grep: zero occurrences of the three old names remain anywhere in internal/tmux; exactly one `const portalIDLiteral` decl exists; every SetSessionOption call site references it (no bare "@portal-id" is passed as an option argument). Choosing the already-tripwired const over a fourth new name is the correct, non-drifting outcome.

TESTS:
- Status: Adequate (no new test expected — this is a scaffolding tidy)
- Coverage: The consolidation is self-verified by the pre-existing tmux-less guard TestHookKeyFormatContainsPortalIDLiteral (hookkey_test.go:29), which asserts portalIDLiteral == "@portal-id" AND that tmux.HookKeyFormat contains it — so any accidental value drift on the surviving const is caught under plain `go test` without tmux. The SkipIfNoTmux-gated real-tmux guards continue to reference the shared const and skip cleanly where tmux is absent.
- Notes: Not under-tested (byte-identity is pinned by the tripwire; the option-name contract is enforced). Not over-tested (no redundant assertions introduced; the tidy adds nothing). Did not execute the suite per instructions; adequacy assessed by reading. Compilation cannot be broken by the change since all references resolve to a single in-package const and no import set changed.

CODE QUALITY:
- Project conventions: Followed. Test-only literal kept spelled-out (import-cycle avoidance) rather than importing session.PortalIDOption, matching the documented codebase constraint. Naming (portalIDLiteral) is role-descriptive and consistent with the byte-identity contract.
- SOLID principles: N/A (single test constant) — DRY improved: four divergent embeddings collapsed to one.
- Complexity: Low. Pure deletion + reference rewrite.
- Modern idioms: Yes.
- Readability: Improved — one documented const with a single byte-identity contract; no divergent names inviting a reader to assume they differ; one edit point if the option name ever changes.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
