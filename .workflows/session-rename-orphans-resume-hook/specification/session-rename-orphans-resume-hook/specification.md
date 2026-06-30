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

## Cross-Reboot Persistence of `@portal-id`

tmux user-options are in-memory server state; they do not survive a reboot. Portal owns save/restore, so it must persist `@portal-id` itself. This is **required** for the headline case: a session renamed *then* rebooted. At capture the session's saved `Name` is the post-rename name, but its hook was registered under the immutable id; restore must recover that id to bake the matching `--hook-key`. The id is unrecoverable after rename unless persisted — so it rides `sessions.json`.

(Note: `@portal-dir` is deliberately *not* persisted/re-stamped today — it lazy-re-derives. `@portal-id` cannot lazy-re-derive an opaque token, so it must be saved. This is the one place the new option diverges from the `@portal-dir` precedent.)

**1. Schema (`internal/state/schema.go`).** Add one field to `state.Session`:
```go
PortalID string `json:"portal_id"`
```
Additive and optional: an old `sessions.json` with no `portal_id` decodes to `""` (tolerant decode, same as other optional fields); a new binary reading it falls back to the session name. No schema `Version` bump and no `sessions.json` migration. Forward-compatible too — an older binary ignores the unknown field.

**2. Capture (`internal/state/capture.go`).** Extend `captureFormat` with a session-scoped `#{@portal-id}` field and populate `Session.PortalID` from it. `#{@portal-id}` resolves per-pane to the owning session's option value, so it is present on every pane row for that session; the parser takes it when assembling the session. A legacy/un-stamped session captures `PortalID == ""`. (The opaque token is alphanumeric, so it cannot contain the `|||` field delimiter.)

**3. Restore re-stamp (`internal/restore/session.go`).** In `createSkeleton`, immediately after `NewSessionWithCommand(sess.Name, …)` recreates the session, re-stamp the saved id when present — best-effort, mirroring creation-time stamping:
```go
if sess.PortalID != "" {
    _ = r.Client.SetSessionOption(sess.Name, PortalIDOption, sess.PortalID)
}
```
`sessions.json` is a snapshot the daemon **continuously regenerates from live tmux state**, not a store of record — so re-seeding the live session with its id is what keeps the id alive past the single restore read. Without the re-stamp the id resolves correctly for the *first* resume (baked from the saved snapshot) but is then lost, because:
- **(a) Re-persistence.** The first post-restore capture (~1s later) rewrites `sessions.json` from live state; a session with no live `@portal-id` is captured as `PortalID == ""`, erasing the id from the snapshot → the *next* reboot resurrects a bare shell.
- **(b) Survives cleanup.** Post-restore stale-cleanup (bootstrap step 11) builds its live-key set from the live `@portal-id`; with the id absent, the restored session's live key falls back to the name, which no longer matches the id-keyed `hooks.json` entry, so cleanup deletes the hook that just fired — in the same bootstrap.
- **(c) Future rename.** A subsequent rename of the restored session stays stable only while the live id is present.

A legacy session with no saved id is left un-stamped and falls through to the name-based key, exactly as before.

**Constant.** The option name is a single shared constant `PortalIDOption = "@portal-id"`, referenced by every set-option site (creation, restore re-stamp) and kept in sync with the literal `@portal-id` embedded in `tmux.HookKeyFormat`. Its package placement is an implementation detail (it must be importable by the stamp and re-stamp sites).

**Firing does not depend on the re-stamp (ordering).** Restore's order is `collectArmInfos` → `createSkeleton` → `armPanes` (`session.go:86`). The hook key is computed from **saved** `sess.PortalID` in `collectArmInfos` and baked into the helper's `--hook-key`; the helper resolves `hooks.json` by that baked key and **never reads the live `@portal-id`**. Hook firing is therefore correct independent of the re-stamp. The re-stamp (in `createSkeleton`) nonetheless precedes the helper launch (in `armPanes`), and serves only the post-restore concerns (a)–(c) above. **Implementation constraint:** the firing path must not be changed to read the live `@portal-id` — doing so would make hook firing depend on re-stamp ordering and reintroduce a rename-window race.

---

## Working Notes
