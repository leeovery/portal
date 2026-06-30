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

---

## Working Notes
