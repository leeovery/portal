# Portal: Session restore race condition on reboot

## Problem

After a system reboot, typing `x` (Portal) shows no sessions because the tmux
server hasn't started yet. tmux-continuum auto-restores sessions on server
start, but Portal queries *before* the server exists.

## Current workaround

A LaunchAgent (`com.leeovery.tmux-boot`) in dotfiles starts the tmux server on
login, triggering continuum auto-restore before the user opens a terminal.

This works but leaves a `_boot` session that's unused.

## Potential Portal-side improvements

1. **Detect missing server + existing resurrect data**: If `tmux list-sessions`
   fails but `~/.local/share/tmux/resurrect/` has save files, Portal could
   start the server and wait briefly for continuum to restore, then show
   sessions.

2. **Start server proactively**: When Portal detects no tmux server, run
   `tmux new-session -d` to boot the server (triggers plugin loading +
   continuum restore), wait 2-3 seconds, then query sessions.

3. **Show a "restoring sessions..." state**: If Portal detects the server just
   started, show a brief loading indicator while continuum runs, then refresh
   the session list.

Option 2 is simplest and self-contained within Portal.
