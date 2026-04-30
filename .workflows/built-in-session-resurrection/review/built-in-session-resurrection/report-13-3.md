# Review Report: built-in-session-resurrection-13-3

**TASK**: Add the two missing Phase 5 task 5-10 acceptance assertions to `cmd/reattach_integration_test.go`

**ACCEPTANCE CRITERIA**:
- Add `saved_at` invariance assertion during steady-state reattach.
- Add `portal open <path>` resolution against a saved-only session via the alias/zoxide chain.
- Edge cases: alias vs zoxide pre-seed; saved-only resolution must reach the connector; intentional duplication of `saved_at` assertion.

**STATUS**: Complete

**SPEC CONTEXT**:
- Phase 5 acceptance bullets (planning.md L165-L166).
- Phase 5 task 5-10 bullets (phase-5-tasks.md L941-L947): includes #2 "portal open PATH resolves a saved-only name" and #7 "saved_at not advanced during steady-state reattach".
- Spec "Save-Side Architecture → Triggers & Serialization → Properties → Restoration guard": `@portal-restoring` window must suppress saves; `saved_at` must not advance.
- Cycle 6 finding: prior Phase 12-2 implementation only covered task 5-10 acceptance bullets (1)(3)(4)(5)(6); bullets (2)-path-arg and (7)-saved_at were not actually asserted.

**IMPLEMENTATION**:
- Status: Implemented (both missing assertions present)
- Location:
  - `cmd/reattach_integration_test.go:194-225` — new helper `seedSessionsJSONWithSavedAt` and reworked `seedSessionsJSON` delegating to it.
  - `cmd/reattach_integration_test.go:328-411` — `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` extended with the `saved_at` invariance assertion (L394-L404). Pre-Run timestamp seeded at L345-L346. Uses `state.ReadIndex` and `time.Time.Equal`.
  - `cmd/reattach_integration_test.go:750-848` — new `TestReattachIntegration_OpenPathResolvesSavedOnlySession`. Wires `openDeps` with a `testAliasLookup` mapping query `mysaved` to a real `projectDir` and a `testZoxideQuerier` that returns `resolver.ErrNoMatch` (alias-vs-zoxide pre-seed satisfied). Asserts: openPathFunc called with the alias-resolved path, openTUIFunc NOT called (guards against silent FallbackResult), and `has-session -t open-ghost` succeeds post-bootstrap.

**TESTS**:
- Status: Adequate (this task IS the test additions)
- Coverage:
  - `saved_at` invariance: pre-Run timestamp seeded → post-Run `ReadIndex` → `Equal` comparison. Sibling test in `phase5_marker_suppression_integration_test.go:206-209` covers the fresh-restore path — duplication is intentional and correctly justified in godoc.
  - `portal open PATH`: alias chain (alias hit, zoxide miss), openPathFunc captured, openTUIFunc guarded against, has-session cross-check on the live socket.

**CODE QUALITY**:
- Project conventions: Followed. No `t.Parallel()` (CLAUDE.md mandate); package-level seam mutation paired with `t.Cleanup` resets.
- SOLID: Good. `seedSessionsJSON` delegates to `seedSessionsJSONWithSavedAt` (single source of truth).
- Complexity: Low.
- Modern idioms: Yes. Uses `time.Date` with explicit UTC, `time.Time.Equal`.
- Readability: Good.
- Issues: None blocking.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] `TestReattachIntegration_OpenPathResolvesSavedOnlySession` mocks `openPathFunc` rather than letting the real `openPath` reach a `mockSessionConnector`. A future hardening could split openPath into a connector-injectable shape.
- [quickfix] Comment at L207 says "phase5_marker_suppression_integration_test.go:207" — line numbers in cross-file comments are fragile. Consider using a symbol/test name reference instead. Same pattern at L327.
- [idea] `seedSessionsJSON` requires reading two layers (delegation to `seedSessionsJSONWithSavedAt`) to understand. Could be inlined via a variadic option pattern if test surface grows.
