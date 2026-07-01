# Implementation Review: Session Rename Orphans Resume Hook

**Plan**: session-rename-orphans-resume-hook
**QA Verdict**: Approve

## Summary

The fix is implemented faithfully and completely. All 25 tasks (18 plan tasks across 3 phases + 7 analysis-cycle hardening tasks) were independently verified against their acceptance criteria and the specification, and every one returned **Complete with zero blocking issues**. The core mechanism — an immutable `@portal-id` session user-option stamped at both creation paths, persisted through `sessions.json`, re-stamped at restore, and used as the single hook-key anchor at every key-producing site — is derived by one shared rule (`HookKey` / `HookKeyFormat`) across registration, stale-cleanup enumeration, and restore baking, with the load-bearing ordering trap (firing must never read the live `@portal-id`) correctly respected. The central "every key-producing site agrees" invariant is guarded three ways: a real-tmux cross-site byte-identity test, and two fast tmux-less static guards binding the three independent `@portal-id` literals plus the source-of-truth constant. Test coverage is thorough and well-targeted — headline rename→reboot firing for both triggers, durability across repeated reboots, post-restore cleanup safety, multi-pane per-pane routing, and graceful legacy degradation are all exercised end-to-end, while unit/component tests pin the primitives, schema, capture arity contract, and best-effort stamping. Non-blocking notes are limited to post-rename comment/label staleness and optional test-scaffolding consolidation; none affects behaviour.

## QA Verification

### Specification Compliance

Implementation aligns with the specification on every substantive point:

- **Stable identity** (`@portal-id`): fresh opaque token, fire-and-forget, immutable, stamped best-effort at `CreateFromDir` and inside the `QuickStart` argv chain (ordered after `@portal-dir`, before `attach-session`); a stamp failure never aborts creation.
- **Hook-key derivation**: single rule `prefer @portal-id, else session_name` implemented as both the tmux format string `HookKeyFormat` and the pure-Go `HookKey`, with the byte-identical `@portal-id` literal confirmed across `session.PortalIDOption`, `HookKeyFormat`, and `captureFormat`. All four stale hooks.json-ownership doc-comments retired and the stability invariant transferred to the new primitives.
- **Four stages agree**: registration (`ResolveHookKey`), stale-cleanup enumeration (`ListAllPaneHookKeys`, added separately so the name-based `ListAllPanes`/`StructuralKeyFormat` stay available to their out-of-scope callers), restore baking (`HookKey` from saved state), and the unchanged hydrate consumer.
- **Cross-reboot persistence**: additive `PortalID string json:"portal_id"` field (tolerant decode, no `Version` bump, no migration); capture appends `#{@portal-id}` as the 11th fixed-arity column with `captureFieldCount` bumped in lockstep; restore re-stamps best-effort after `NewSessionWithCommand`, skipped on empty; firing correct independent of the re-stamp.
- **Scope & non-goals honoured**: no rename interception, `renameAndRefresh` unchanged, no external/UI change, no legacy backfill.

No deviations from the specification were found.

### Plan Completion

- [x] Phase 1 (Stable Identity Foundation) acceptance criteria met — tasks 1-1 … 1-5
- [x] Phase 2 (Live-Key Sites Adopt the Hook Key) acceptance criteria met — tasks 2-1 … 2-6
- [x] Phase 3 (Cross-Reboot Persistence) acceptance criteria met — tasks 3-1 … 3-7
- [x] Analysis cycle 1 hardening complete — tasks analysis-1-1 … analysis-1-5
- [x] Analysis cycle 2 hardening complete — tasks analysis-2-1, analysis-2-2
- [x] All 25 tasks completed and independently verified
- [x] No scope creep — every changed file maps to a plan task; consequential test updates (e.g. daemon capture-field fixtures affected by the `captureFieldCount` bump) are expected, not out-of-scope

### Code Quality

No issues found. The implementation mirrors established codebase patterns (`@portal-dir` stamping precedent, best-effort swallowed errors documented in-line, interface-method renames that make a name-based regression a compile error). The `_ =` discarded error on the re-stamp is deliberate, spec-mandated best-effort handling consistent with the creation paths, not a lapse. The one optional production suggestion (an explicit `var _ HookKeyResolver = (*tmux.Client)(nil)` compile-time assertion) is a hardening nicety, not a defect.

### Test Quality

Tests adequately verify requirements, with good balance:

- **Not under-tested**: primitives, schema round-trip/tolerant-decode, the fixed-arity capture contract (including wrong-arity rejection), best-effort stamping failure paths, cross-site byte-identity, and all headline integration scenarios are covered. The tmux-less static guards close the gap where real-tmux tests silently skip.
- **Not over-tested**: assertions are focused; the analysis cycles actively removed redundancy (deleted `verifyRenameHookFiredOnce` in favour of the shared `assertHookFireCount`, collapsed triplicated test constants).
- Real-tmux and integration tests are correctly gated (`SkipIfNoTmux`, `//go:build integration`, `testing.Short()`), use `portaltest.IsolateStateForTest`, and avoid `t.Parallel()` per project conventions.

### Required Changes (if any)

None. No blocking issues were identified.

## Recommendations

### Do now

1. Refresh post-rename stale vocabulary in test comments, labels, and messages (the code is correct; only the prose lags the `ListAllPanes → ListAllPaneHookKeys` / `structural key → hook key` renames):
   - `cmd/hooks_test.go:167,192,195,305,310,313,407,424,427,431,453,456,565,599` — reword "structural key" → "hook key" in comments, `t.Error` messages, and the subtest name "reads pane ID from TMUX_PANE and resolves structural key" (Report 2-2)
   - `cmd/clean_test.go:422,448,453` — subtest label "ListAllPanes error preserves hooks (safety net)" and its assertion messages still say "ListAllPanes"; rename to `ListAllPaneHookKeys` (Report 2-4)
   - `cmd/bootstrap_production_test.go:180,186` — godoc and subtest name reference "a ListAllPanes failure"; update to `ListAllPaneHookKeys` (Report 2-4)
   - `cmd/cleanstale_transient_listpanes_clean_integration_test.go:4,16,151,290` — file/godoc comments still describe "ListAllPanes" as the intercepted enumeration; refresh to `ListAllPaneHookKeys` (Report 2-4)
2. `cmd/hooks.go:58` — optionally add an explicit compile-time assertion `var _ HookKeyResolver = (*tmux.Client)(nil)` near the interface so the production client's contract fails fast if the method signature drifts (Report 2-2)
3. `internal/restore/rename_reboot_hook_integration_test.go:40-41` — tighten the header's example run command from `-run RenameReboot` to `-run TestRenameRebootHook` for exactness (prefix match already works; doc-only) (Report 3-5)
4. `internal/state/portal_id_literal_guard_test.go:11` — reword the import-cycle rationale: the `internal/state → internal/session` dependency is transitive, not direct ("internal/session transitively depends on internal/state"); the conclusion is correct (Report analysis-1-4)

### Ideas

5. **APPLIED** — Consolidated shared test setup into named helpers:
   - `internal/tmux/hookkey_realtmux_shared_test.go` (new) now owns `seedThreePaneStampedSession` + `sessionPaneIDs`; the cross-site and format real-tmux guards both call it instead of duplicating the create→stamp→split→new-window setup (Report 2-5)
   - `internal/restore/rename_reboot_shared_test.go` (new) now owns the rename→reboot fixture consts (`renamePortalID`/`renameOldName`/`renameNewName`) and the shared leaves (`findCapturedSession`, `verifyHookKeyed`, `persistIndex`, `seedScrollback`, `assertHookFireCount`), moved out of the hook/durability files so ownership is explicit (Report 3-5)
6. **APPLIED** — `internal/restore/rename_reboot_durability_integration_test.go` — the durability leg now captures once into an outer `nextIdx` and reuses it for cycle 2 (nothing mutates the live server between the assertion and cycle 2); the parent-level non-vacuous guard is retained with a note that a subtest `Fatalf` does not halt the parent (Report 3-6)
7. **DROPPED** (user decision) — base-index-drift variant for `internal/restore/multipane_legacy_integration_test.go`: net-new test coverage not required by the acceptance criteria; the existing no-drift coverage plus the documented note is deemed sufficient (Report 3-7)
