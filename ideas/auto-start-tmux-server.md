# Auto-start tmux server on Portal launch

## Problem

After a system reboot, running `x` (Portal) shows no sessions — just a `_boot` session. The tmux server hasn't started yet, so tmux-continuum hasn't had a chance to restore sessions. A LaunchAgent (`com.leeovery.tmux-boot`) currently works around this by starting the server on login, but it leaves a leftover `_boot` session.

## Decision

Portal should self-bootstrap the tmux server when it detects no server is running. This removes the dependency on the LaunchAgent entirely.

## Proposed flow

1. Portal detects no server running (`tmux list-sessions` fails)
2. Check for resurrect data at `~/.local/share/tmux/resurrect/last`
3. Start server with `tmux new-session -d -s _boot` (triggers plugin loading + continuum auto-restore)
4. Poll briefly (up to ~3s) until `list-sessions` returns more than just `_boot`
5. Kill the `_boot` session once real sessions appear
6. Proceed to TUI with restored sessions

## Implementation notes

- Add an `EnsureServer` method (or similar) to the tmux client
- Call it in `PersistentPreRunE` for commands that need tmux (before the existing `CheckTmuxAvailable`)
- If no resurrect data exists (fresh install), just start the server and proceed — TUI will show the Projects page as normal
- The resurrect file path is `~/.local/share/tmux/resurrect/last` (symlink to latest save)
- Once this is working, the `com.leeovery.tmux-boot` LaunchAgent can be removed from dotfiles

## Edge cases

- No resurrect data: start server, proceed immediately (no waiting)
- Resurrect data exists but continuum doesn't restore within timeout: proceed anyway with whatever sessions exist
- Server already running: no-op, proceed as normal
- tmux not installed: existing `CheckTmuxAvailable` error handles this
