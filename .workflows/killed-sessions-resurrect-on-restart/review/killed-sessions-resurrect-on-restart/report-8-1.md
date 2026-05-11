TASK: killed-sessions-resurrect-on-restart-8-1 — Delete shellQuoteSingle and emit bare hydrate invocation

ACCEPTANCE CRITERIA:
- shellQuoteSingle no longer exists in internal/restore/session.go.
- buildHydrateCommand emits raw value-arg interpolation with no shellQuoteSingle wrap.
- White-box test no longer contains the single-quote round-trip sub-case.
- Negative-assertion sub-test ("no sh -c envelope") remains as wrapper-reintroduction guard.
- grep -rn "shellQuoteSingle" --include='*.go' returns zero hits.
- Docstring acknowledges apostrophe-bearing inputs would break bare-form but are not produced by Portal's sanitization.
- strings import removed from session.go if unused.

STATUS: Complete

SPEC CONTEXT: Phase 3 (task 3-1) dropped the outer sh -c envelope. With envelope gone, shellQuoteSingle is a no-op: there is no outer single-quoted shell envelope anchoring the escape, and Portal's sanitization (sanitizeSessionName) filters `/`, `\`, `\0`. Phase 8 cycle 5 flagged surviving helper + half-defensive test as dead defensive code.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/restore/session.go:431-436 (buildHydrateCommand body); session.go:408-430 (refreshed docstring)
- Notes:
  - shellQuoteSingle removed (grep returns zero hits).
  - buildHydrateCommand body is a direct fmt.Sprintf with raw %s interpolation.
  - `strings` import absent from session.go imports block.
  - Docstring at lines 423-430 explicitly calls out bare-form apostrophe limitation and sanitization invariant.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/internal/restore/session_build_hydrate_test.go retains three sub-tests: typical inputs; empty hookKey; negative-assertion (no sh -c envelope, no exec $SHELL trailer).
  - "single-quote round-trip" sub-case is gone.
  - session_test.go:568-594 TestSessionRestorer_HydrateCommandFormat snapshot continues to assert bare-form via production code path.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single responsibility; no longer carries half-broken shell-quoting concern.
- Complexity: Low — function body is one statement.
- Modern idioms: Yes.
- Readability: Good. Docstring cross-references sanitizeSessionName.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] If a future change to sanitizeSessionName ever relaxes the filter, this docstring becomes the only signpost. Consider whether a test assertion in panekey_test.go pinning the filtered character set would make the invariant more enforceable.
- [idea] TestBuildHydrateCommand_BareForm (white-box) and TestSessionRestorer_HydrateCommandFormat (black-box) both pin bare-form shape. Intentional duplication.
