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
