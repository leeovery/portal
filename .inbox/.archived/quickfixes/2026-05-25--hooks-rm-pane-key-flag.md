# Add `--pane-key` flag to `portal hooks rm`

`portal hooks rm` currently resolves the target hook entry exclusively from `$TMUX_PANE` via `resolveCurrentPaneKey()` in `cmd/hooks.go`. That makes the command unable to remove an entry for any pane other than the one the user is currently inside. The fix is a small CLI surface extension: accept an optional `--pane-key <key>` flag and use it directly when set, falling back to the existing `resolveCurrentPaneKey()` lookup when unset.

The motivation is external. The user-level Claude `SessionStart` hook at `~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh` needs to prune stale entries from `~/.config/portal/hooks.json` — entries whose recorded UUID no longer has a corresponding `~/.claude/projects/*/<uuid>.jsonl` history file (Claude considers them unresumable). The bash hook can detect staleness cheaply (one `compgen` per entry), but without a way to address a specific paneKey through Portal's CLI it would have to edit `hooks.json` directly, racing with Portal's own atomic-write path. Adding the flag lets Portal stay the sole writer of its own file.

Scope is minimal and entirely inside `cmd/hooks.go`:

- Register `Flags().String("pane-key", "", "...")` on `hooksRmCmd` (around line 163, next to the existing `--on-resume` registration).
- Branch in the `RunE` body (around lines 143-154): if the flag value is non-empty use it as `structuralKey` directly; otherwise call `resolveCurrentPaneKey()` as today.
- One or two unit tests covering both branches.

No change to `hooks set`, no change to the `hooks.json` schema, no change to any other command or package. `store.Remove(key, "on-resume")` already takes the key as an argument, so the storage primitive is untouched. The companion bash-side prune loop in the user's dotfiles is a separate change outside the Portal repo and is not part of this quick-fix.
