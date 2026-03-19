# Specification: Auto Start Tmux Server

## Specification

### Overview

After a system reboot, the tmux server isn't running. Portal (`x`) currently shows no sessions because tmux-continuum can't restore until a server exists. A LaunchAgent (`com.leeovery.tmux-boot`) works around this but leaves a leftover `_boot` session and lives outside Portal's codebase.

Portal should self-bootstrap the tmux server when it detects none is running, removing the LaunchAgent dependency entirely.

### Design Philosophy

**Portal is plugin-agnostic.** Portal has zero knowledge of tmux-continuum, tmux-resurrect, or any other tmux plugin. Whether the user has resurrect/continuum, some other plugin, or nothing — that's tmux's business. Portal's only job is to ensure a server is running.

This means:
- No checking for resurrect data (`~/.local/share/tmux/resurrect/last`)
- No conditional logic based on plugin presence
- No awareness of how sessions get restored — they just appear (or don't)

---

## Working Notes

[Optional - capture in-progress discussion if needed]
