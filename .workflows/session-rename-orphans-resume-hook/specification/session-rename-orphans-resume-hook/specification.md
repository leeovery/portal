# Specification: Session Rename Orphans Resume Hook

## Specification

## Problem Statement

Portal lets a user register a per-pane **resume hook** (`portal hooks set --on-resume "<cmd>"`, persisted in `hooks.json`) so that after a reboot the pane resumes what it was running instead of starting a bare shell. Renaming a tmux session silently orphans that hook: the session keeps running fine, but after the next restart it comes back as a bare shell.

The failure is **silent** (no error or warning at rename time) and **delayed** (invisible until the next reboot). It affects both rename triggers:
- external `tmux rename-session`, and
- Portal's own in-TUI rename modal (`r` key → `renameAndRefresh` → `tmux.RenameSession`), which today does a bare rename + list refresh with **zero** hook re-keying.

It bites **only when the inner pane process does not restart** across the rename. If the process restarts (e.g. the external tool's own start-hook re-runs `portal hooks set` under the new name), the hook self-heals — which is why the bug hid in everyday use.

## Root Cause

Resume hooks are keyed by the **structural key** `session_name:window.pane` (`tmux.PaneTarget` / `StructuralKeyFormat`), which embeds the **mutable** tmux session name. Four stages independently derive this key from whatever the session is *currently* called, and they run at different times, so a rename desynchronizes them:

1. **Registration** (`cmd/hooks.go`) — resolves the live pane's structural key and stores the hook under it.
2. **Capture** (`internal/state/capture.go`) — writes `sessions.json` with the session's current name.
3. **Restore lookup** (`internal/restore/session.go` → `cmd/state_hydrate.go`) — recomputes the key from the *saved* (post-rename) name and looks it up; a miss degrades to bare `$SHELL`.
4. **Stale cleanup** (`internal/hooks/store.go` `CleanStale`, called from bootstrap step 11 and `portal clean`) — deletes any hook key not present among live panes' structural keys; the old-name key is absent → deleted.

After a rename the hook stays registered under the **old** name while every later stage uses the **new** name. The proximate cause of the bare shell is the restore-time lookup miss (stage 3); the stale-cleanup deletion (stage 4) makes the loss permanent. There is no rename-aware re-keying path and no stable, rename-immune pane identity — the chosen anchor for hook identity is the one tmux attribute the user is free to change at any time.

## Fix Overview: Stable Session Identity (`@portal-id`)

The fix introduces an **immutable, rename-immune Portal session identity** and keys resume hooks off it instead of the mutable session name. It mirrors the existing `@portal-dir` pattern: a tmux **session user-option** stamped at creation, carried on the session object (not its name), so it is unaffected by `rename-session`.

**The option.** A new session user-option `@portal-id` (constant `session.PortalIDOption = "@portal-id"`), parallel to `session.PortalDirOption`.

**Its value: a fresh opaque id.** `@portal-id` is stamped with a freshly-generated random token (a `crypto/rand` nanoid, **independent of the session name**), frozen at creation. An opaque token keeps the identity conceptually distinct from the name and can never collide with a session name. Token width is chosen so birthday-collision across the whole session population is negligible (implementation detail — the existing `NewNanoIDGenerator` scheme, widened if warranted).

**It is immutable.** Once stamped, `@portal-id` is never rewritten. A `rename-session` (external or in-TUI) changes `#{session_name}` but leaves `@portal-id` untouched, so the hook key derived from it is stable across any number of renames.

**Where it is stamped.** Both first-party creation paths, mirroring `@portal-dir` exactly:
- `SessionCreator.CreateFromDir` (`internal/session/create.go`) — a best-effort `SetSessionOption(name, PortalIDOption, <token>)` immediately after `NewSession`, alongside the existing `@portal-dir` stamp. Failure is swallowed (no log component), as with `@portal-dir`.
- `QuickStart.Run` (`internal/session/quickstart.go`) — an additional `; set-option -t <name> @portal-id <token>` step in the chained detached-create → stamp → attach `ExecArgs`, alongside the existing `@portal-dir` step (stamped while detached, before `attach-session` blocks the chain).

**Hook key = prefer `@portal-id`, else session name.** The hook key becomes `<@portal-id or session_name>:window.pane`, derived via a single tmux conditional. Stamped sessions key by their immutable id (rename-immune); un-stamped sessions (legacy, manually-created tmux sessions, or a best-effort stamp that failed) fall back to the session name — which equals the key already on disk, so existing `hooks.json` entries keep matching with **no migration**. This is the same un-stamped-session fallback role `@portal-dir`'s lazy resolver already plays.

**Coverage / natural migration.** The fix protects every session **created after it ships**. Pre-existing (legacy) sessions keep working via the name fallback but are **not** retrofitted: renaming a legacy session still orphans its hook (the original bug) until that session is next recreated from scratch, at which point it gains an `@portal-id` and becomes rename-immune. There is **no backfill and no `hooks.json` re-key migration** — legacy sessions migrate naturally as they are recreated.

## Hook-Key Derivation (the four stages)

The fix's central invariant: **every site that produces or consumes a hook key derives it by the identical rule** — `prefer @portal-id, else session_name`, suffixed `:window.pane`. If any site disagrees, hooks orphan (the bug). There are three key-*producing* sites (registration, stale-cleanup, restore) plus one key-*consuming* site (hydrate); all must agree.

**Decoupling from `tmux.PaneTarget`.** `PaneTarget` stays exactly as-is — it remains the canonical, name-based `-t` *target* formatter, still used to address live panes (e.g. `respawn-pane`, `select-pane`). The hook key becomes a **separate concern** with its own formatter, so the change touches only hook identity, not tmux targeting.

Two new derivation primitives in `internal/tmux`:
- **`HookKeyFormat`** — a tmux format string for live reads: `#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}`. tmux resolves the conditional per-session: a stamped session yields `<id>:w.p`, an un-stamped one yields `<name>:w.p`.
- **`HookKey(portalID, name string, window, pane int) string`** — a pure formatter for the saved path: returns `<portalID>:w.p` when `portalID != ""`, else `<name>:w.p`. The in-Go mirror of the tmux conditional, for use where the values come from saved state rather than a live tmux read.

### Stage 1 — Registration (`cmd/hooks.go`)
`resolveCurrentPaneKey()` today resolves `ResolveStructuralKey($TMUX_PANE)`, which reads the name-based `StructuralKeyFormat`. It is changed to resolve the **hook key** via a new client read using `HookKeyFormat` (e.g. `ResolveHookKey(paneID)` → `display-message -p -t <pane> <HookKeyFormat>`). `portal hooks set`/`rm` then store/remove under the stable key. The `--pane-key` literal pass-through on `rm` is unchanged (still a verbatim key).

### Stage 2 — Stale cleanup live keys (`cmd/run_hook_stale_cleanup.go`, `cmd/clean.go`)
`CleanStale(liveKeys)` deletes any `hooks.json` key not in `liveKeys`. The live-key enumeration that feeds it (today `ListAllPanes()` → `ListAllPanesWithFormat(StructuralKeyFormat)`) is changed to enumerate live panes' **hook keys** via `HookKeyFormat`. This is the load-bearing consistency point: liveKeys must be produced by the same rule as registration, or cleanup mass-orphans every stamped session's hook. The name-based `StructuralKeyFormat` / `ListAllPanes` remain available for any non-hook structural use; only the hook-cleanup enumeration switches to the hook-key format.

### Stage 3 — Restore lookup baking (`internal/restore/session.go`)
`collectArmInfos` today sets `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` (saved name). It is changed to `hookKey: tmux.HookKey(sess.PortalID, sess.Name, w.Index, p.Index)` — preferring the **saved** `@portal-id` (from the persisted schema field; see persistence topic), else the saved name. The baked `--hook-key` therefore matches what registration stored. Base-index drift preservation is unchanged (the key still uses saved indices, FIFOs still use live indices).

### Stage 4 — Hydrate lookup (`cmd/state_hydrate.go`)
**No change.** The helper looks up `hooks.LookupOnResume(store, cfg.HookKey)` using the baked `--hook-key`, then execs `sh -c '<hook>; exec $SHELL'` on a hit or bare `$SHELL` on a miss. Because stage 3 now bakes the stable key and stage 1 stored under it, the lookup hits for any renamed-but-stamped session.

**Post-restore consistency.** After restore re-stamps `@portal-id` on the recreated live session (persistence topic), a subsequent stage-2 cleanup read of the *live* session yields the same key stage 3 baked — so cleanup never treats a freshly-restored hook as stale.

---

## Working Notes
