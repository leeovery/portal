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

---

## Working Notes
