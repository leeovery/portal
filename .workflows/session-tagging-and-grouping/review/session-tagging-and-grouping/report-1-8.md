TASK: session-tagging-and-grouping-1-8 — Best-effort lazy re-stamp of derived `@portal-dir`

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: SetSessionOption failure swallowed and re-attempted next render; git-root derivation failure yields no stamp; derived value used for current render regardless of write outcome.

SPEC CONTEXT: spec § Failure & ordering semantics — derived value used for THIS render, stamp is side-effect for subsequent renders; write best-effort, re-attempted next render, never drops session; derivation failure → no stamp → Unknown/Untagged, re-attempted each render.

IMPLEMENTATION: Implemented.
- dirstamp.go:48-72 ResolveAndStampDir; PaneStamper seam (dirstamp.go:14-17, embeds PaneCurrentPathReader); consumed at tui/model.go:1020-1033 resolveSessionDirs, invoked only from grouped arms (model.go:1052,1054). Fast path short-circuits when stamp present. On err||!ok returns ("",false), stamps nothing. Derived dir computed → stamp swallowed side-effect (`_ =`) → dir returned regardless. Re-attempt is structural (failed write leaves option empty). Chokepoint copies Session value, never mutates m.sessions.

TESTS: Adequate. dirstamp_test.go (7 subtests) + rebuild_dir_resolution_test.go (6) + gate test. All 3 edge cases directly asserted; re-attempt verified across two render passes (2 reads + 2 stamp attempts); fast-path guard; non-mutation; Unknown routing. Compile-time seam assertion. Not over-tested.

CODE QUALITY: Conventions followed (narrow DI seam, documented swallow, closed-log-vocabulary); SOLID good (ISP — one method over reader); low complexity; value-copy non-mutation idiom. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
