# Plan: Session Rename Orphans Resume Hook

## Phases

### Phase 1: Stable Identity Foundation — @portal-id stamping + hook-key primitives
status: approved
approved_at: 2026-07-01

**Goal**: Introduce the immutable `@portal-id` session identity (`session.PortalIDOption`), stamp it at both first-party creation paths (`CreateFromDir`, `QuickStart`), and add the shared `HookKey` / `HookKeyFormat` derivation primitives in `internal/tmux` — retiring the four now-false `hooks.json`-ownership doc-comments and transferring the "format is stable across releases" invariant to the new primitives. This phase establishes the identity and the single derivation rule only; no consumer switches to the new key yet.

**Why this order**: Every downstream stage — registration, stale cleanup, and restore baking — derives its key from `HookKey` / `HookKeyFormat` and reads the option via `PortalIDOption`. These primitives plus the constant are the load-bearing foundation the spec calls out as needing to land before the four hook-key stages and persistence stages that consume them. Nothing can consume them until they exist, so they land first with zero forward references. Stamping ships here (not later) because it produces no behaviour change on its own — the id is inert until a consumer reads it — making this a self-contained, independently green increment.

**Acceptance**:
- [ ] `session.PortalIDOption = "@portal-id"` constant exists, parallel to `PortalDirOption`, importable by all stamp/re-stamp sites
- [ ] `CreateFromDir` stamps a fresh `crypto/rand` token (via `sc.gen`) with `SetSessionOption(name, PortalIDOption, token)` immediately after `NewSession`, alongside the existing `@portal-dir` stamp; both a token-generation error and a stamp error are swallowed and never abort creation
- [ ] `QuickStart.Run` generates the token in Go before assembling `ExecArgs` and interpolates it as a literal into an added `; set-option -t <name> @portal-id <token>` step (stamped while detached, before `attach-session`); a generation failure omits the step, leaving the session un-stamped
- [ ] `tmux.HookKey(portalID, name string, window, pane int) string` returns `<portalID>:w.p` when `portalID != ""` and `<name>:w.p` when empty (unit-covered including multi-pane distinct `w.p` suffixes under one id)
- [ ] `tmux.HookKeyFormat` = `#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}` and resolves to `<id>:w.p` against a stamped real-tmux session and `<name>:w.p` against an un-stamped one
- [ ] The four doc-comments (`PaneTarget`, `PaneTargetExact`, `StructuralKeyFormat`, `ListAllPanes`) no longer assert `hooks.json` key ownership; the stability invariant is documented on `HookKey` / `HookKeyFormat` instead
- [ ] Full test suite green; `PaneTarget` / `StructuralKeyFormat` / `ListAllPanes` behaviour unchanged for name-based tmux targeting and non-hook structural use

#### Tasks
status: approved
approved_at: 2026-07-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-rename-orphans-resume-hook-1-1 | Add `tmux.HookKey` pure Go hook-key formatter | empty portalID falls back to name, empty name with empty portalID yields `:w.p`, multi-pane distinct w.p suffixes under one id, base-index-1 indices, zero indices |
| session-rename-orphans-resume-hook-1-2 | Add `tmux.HookKeyFormat` tmux format string and verify against real tmux | stamped session yields `<id>:w.p`, un-stamped session yields `<name>:w.p`, multi-window/multi-pane resolves distinct suffixes |
| session-rename-orphans-resume-hook-1-3 | Add `session.PortalIDOption` constant and stamp `@portal-id` in `CreateFromDir` | token-generation error swallowed (session created un-stamped), `SetSessionOption` error swallowed (creation not aborted), stamp best-effort and non-fatal |
| session-rename-orphans-resume-hook-1-4 | Stamp `@portal-id` in `QuickStart.Run` ExecArgs chain | token-generation error omits the set-option step (session un-stamped), stamp step ordered before attach-session, token interpolated as literal argv element |
| session-rename-orphans-resume-hook-1-5 | Retire the four stale `hooks.json`-ownership doc-comments; transfer stability invariant to `HookKey`/`HookKeyFormat` | none |

### Phase 2: Live-Key Sites Adopt the Hook Key — registration + stale cleanup
status: approved
approved_at: 2026-07-01

**Goal**: Switch the two live-tmux key sites to the hook-key derivation — hook registration (`cmd/hooks.go`, via a new `ResolveHookKey` read using `HookKeyFormat`) and the stale-cleanup live-key enumeration (`cmd/clean.go` / `cmd/run_hook_stale_cleanup.go`, enumerating live panes' hook keys) — so a stamped session both registers and is swept under its immutable id rather than its mutable name.

**Why this order**: Both sites consume `HookKeyFormat` from Phase 1 and operate purely on live tmux state (no persistence), so they are fully testable before the cross-reboot chain exists. Registration and cleanup are paired because they form the live half of the central "every key-producing site derives the same key" invariant — split apart, cleanup would mass-orphan every stamped session's freshly-registered hook. This phase builds on the working, unchanged foundation from Phase 1 and adds no forward dependency on the persistence work in Phase 3.

**Acceptance**:
- [ ] `resolveCurrentPaneKey()` resolves the hook key via a new client read (`ResolveHookKey(paneID)` → `display-message -p -t <pane> <HookKeyFormat>`) instead of `ResolveStructuralKey`; `portal hooks set` / `rm` store and remove under the stable key
- [ ] `ResolveHookKey` aborts registration with the error on a read (transport/exec) failure — it never synthesizes a name-based key on failure
- [ ] The `rm --pane-key` literal pass-through remains a verbatim key with no re-derivation
- [ ] The live-key enumeration feeding `CleanStale` (bootstrap step 11 and `portal clean`) enumerates live panes' hook keys via `HookKeyFormat`; the name-based `ListAllPanes` / `StructuralKeyFormat` stay available and unchanged for non-hook structural use (skeleton-marker cleanup, daemon)
- [ ] For a given stamped session, the registration read and the cleanup live-key enumeration produce byte-identical keys (cross-site consistency, live half)
- [ ] A pre-fix, name-keyed `hooks.json` entry for an un-stamped, never-renamed session still resolves and is not mass-orphaned by stale-cleanup after upgrade (name fallback coincides with the on-disk key)
- [ ] Full test suite green

#### Tasks
status: approved
approved_at: 2026-07-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-rename-orphans-resume-hook-2-1 | Add `tmux.ResolveHookKey` client read using `HookKeyFormat` | display-message read failure aborts with wrapped error (no name-based synthesis), stamped pane yields `<id>:w.p`, un-stamped pane yields `<name>:w.p` |
| session-rename-orphans-resume-hook-2-2 | Switch `resolveCurrentPaneKey` (`cmd/hooks.go`) to resolve the hook key | ResolveHookKey read failure aborts both `hooks set` and `hooks rm`, `rm --pane-key` verbatim key bypasses re-derivation, missing `TMUX_PANE` still errors |
| session-rename-orphans-resume-hook-2-3 | Add `tmux.ListAllPaneHookKeys` enumeration (`list-panes -a` with `HookKeyFormat`) | stamped sessions enumerate `<id>:w.p`, un-stamped enumerate `<name>:w.p`, list-panes error propagates `(nil, err)`, empty output yields empty slice |
| session-rename-orphans-resume-hook-2-4 | Repoint the stale-cleanup live-key enumeration (`AllPaneLister`) to hook keys | freshly-registered stamped-session hook survives cleanup, list-panes error still preserves hooks (swallow policy intact), empty live set still triggers the mass-deletion hazard guard (no deletion), name-based `ListAllPanes`/`StructuralKeyFormat` unchanged for skeleton-marker/daemon use |
| session-rename-orphans-resume-hook-2-5 | Cross-site consistency test: registration read == cleanup enumeration (live half) | multi-pane session (distinct `w.p` suffixes under one id agree across sites), un-stamped session agrees on the name-based key across both sites |
| session-rename-orphans-resume-hook-2-6 | No-regression test: un-stamped, never-renamed `hooks.json` entry survives upgrade | name fallback coincides with on-disk key (entry preserved), a truly-stale name-keyed entry is still swept |

### Phase 3: Cross-Reboot Persistence — schema, capture, restore re-stamp + baking
status: approved
approved_at: 2026-07-01

**Goal**: Persist `@portal-id` across reboots (additive `Session.PortalID` schema field + `#{@portal-id}` capture column) and consume it at restore — re-stamping the recreated live session from the saved value and baking the stable `--hook-key` via `tmux.HookKey(sess.PortalID, ...)` in `collectArmInfos`. This closes the headline rename-then-reboot gap durably across repeated reboots and delivers the integration coverage that proves the whole invariant.

**Why this order**: Restore-lookup baking (Stage 3) consumes both the `HookKey` primitive from Phase 1 and the persisted `PortalID` field introduced here, so persistence must be in place before baking can read it. This is the only phase that can exercise the reboot gap end-to-end (rename → capture → restore → fire), so it depends on the working live-key path from Phase 2 and lands last. Its integration tests are the definitive proof of "every key-producing site agrees" across the persistence boundary.

**Acceptance**:
- [ ] `state.Session` gains a `PortalID` string field tagged `json:"portal_id"`; an old `sessions.json` with no `portal_id` decodes to `""` (tolerant decode) with no schema `Version` bump and no migration; an older binary ignores the field (forward-compatible)
- [ ] `captureFormat` appends `#{@portal-id}` as the last column, `captureFieldCount` bumps `10 → 11`, the row parser reads the new trailing index, and `Session.PortalID` is lifted from the first row of `grouped[name]`; an un-stamped session captures `""`; a zero-row session yields `PortalID == ""` and is already rejected by `Restore`
- [ ] `createSkeleton` re-stamps `@portal-id` via `SetSessionOption(sess.Name, PortalIDOption, sess.PortalID)` (best-effort) immediately after `NewSessionWithCommand` when `sess.PortalID != ""`, and skips the stamp when empty
- [ ] `collectArmInfos` bakes `hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` using saved indices; the firing path is not changed to read the live `@portal-id` (ordering-trap guard)
- [ ] Rename-then-restore fires the registered hook (not bare `$SHELL`) for both triggers: raw `tmux rename-session` and the in-TUI `renameAndRefresh` path
- [ ] Durable across repeated reboots: after rename+restore, a simulated next capture re-persists the id and a second restore still fires the hook
- [ ] Post-restore stale-cleanup (bootstrap step 11 / `portal clean`) does not delete the just-restored hook; per-pane hooks under one session fire on the correct pane after rename+restore; un-stamped sessions degrade to the name-based key everywhere with no panic

#### Tasks
status: approved
approved_at: 2026-07-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-rename-orphans-resume-hook-3-1 | Add `PortalID` field to `state.Session` schema (`json:"portal_id"`, tolerant decode) | sessions.json without `portal_id` decodes to `""`, older binary ignores the field (forward-compatible), no schema `Version` bump, encode/decode round-trip preserves the id |
| session-rename-orphans-resume-hook-3-2 | Extend capture to read `#{@portal-id}` into `Session.PortalID` (append column, bump `captureFieldCount` 10→11, parser lockstep) | un-stamped session captures `""`, zero-row session yields `PortalID==""` (benign, already rejected by `Restore`), every existing field index unchanged (append-only), row parser rejects wrong-arity rows, all panes of a session carry the same session-scoped value |
| session-rename-orphans-resume-hook-3-3 | Re-stamp `@portal-id` in `createSkeleton` from the saved value (best-effort after `NewSessionWithCommand`) | empty saved `PortalID` skips the stamp, `SetSessionOption` error swallowed (restore not aborted), re-stamp precedes `armPanes` (order preserved) |
| session-rename-orphans-resume-hook-3-4 | Bake the stable hook key in `collectArmInfos` via `tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` | empty saved `PortalID` falls back to name-based key, saved-index (base-index drift) preservation unchanged, ordering-trap guard — firing path never reads live `@portal-id`, multi-pane distinct `w.p` suffixes under one id |
| session-rename-orphans-resume-hook-3-5 | Integration: rename-then-restore fires the registered hook for both triggers (raw `tmux rename-session` and in-TUI `renameAndRefresh`) | hook fires (not bare `$SHELL`) after external rename, hook fires after in-TUI `renameAndRefresh` rename, pane process kept running across rename (no self-heal restart) |
| session-rename-orphans-resume-hook-3-6 | Integration: durable across repeated reboots + post-restore cleanup keeps the restored hook | id re-persisted by simulated next capture after restore, hook still fires on the second reboot cycle, freshly-restored hook survives the stale-cleanup pass (bootstrap step 11 / `portal clean`), live-key set from re-stamped `@portal-id` matches the id-keyed `hooks.json` entry |
| session-rename-orphans-resume-hook-3-7 | Integration: multi-pane fires on the correct pane + graceful legacy degradation | per-pane hooks fire on the correct pane after rename+restore, un-stamped session degrades to name-based key end-to-end, no panic on empty/absent `PortalID` anywhere in the chain |

### Phase 4: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-rename-orphans-resume-hook-analysis-1-1 | Copy `PortalID` in the `findOrAppendSession` append branch | append branch currently unreachable (no live bug), latent re-orphan trap if `sessionLive` gate relaxed, `Windows` stays empty `[]Window{}`, no change to merge/gate logic |
| session-rename-orphans-resume-hook-analysis-1-2 | Fix the stale `ListAllPanes` doc-comment on `cleanStaleAdapter` | comment-only edit, sibling `stale_marker_cleanup.go:43` `ListAllPanes` reference must NOT change (name-based skeleton-marker cleanup) |
| session-rename-orphans-resume-hook-analysis-1-3 | Update the `ListAllPanes` prose in the shared stale-cleanup helper | eight comment references across two files, comment-only (no call site/logic change), preserve name-based-vs-hook-key distinction |
| session-rename-orphans-resume-hook-analysis-1-4 | Add a fast static byte-identity guard for the three `@portal-id` literals | import-cycle avoidance dictates guard placement, no-tmux (does not depend on `SkipIfNoTmux`), pins canonical literal value, guard must fail on a mutated literal |
| session-rename-orphans-resume-hook-analysis-1-5 | Collapse the triplicated `@portal-id` test constant in the `tmux_test` package | three consts + one inlined literal collapse to one, import-cycle avoidance preserved (literal not `session.PortalIDOption`), surviving decl keeps byte-identity comment |

### Phase 5: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-rename-orphans-resume-hook-analysis-2-1 | Add a fast tmux-less guard binding `session.PortalIDOption` to the hook-key format strings | source-of-truth constant change caught in tmux-less path, not `//go:build integration`-gated / no `SkipIfNoTmux`, ties constant to `tmux.HookKeyFormat` embedding, `captureFormat` unreachable from `cmd` (transitive chain via shared literal), adds coverage only (no prod/cycle-1-guard edits) |
| session-rename-orphans-resume-hook-analysis-2-2 | Delete redundant `verifyRenameHookFiredOnce`; reuse the shared `assertHookFireCount` helper | single call site repointed to `assertHookFireCount(t, file, 1)`, function + doc-comment deleted, only genuinely-orphaned imports removed, `assertHookFireCount` unchanged, 3-6/3-7 files untouched, no behaviour change to 3-5 test |
