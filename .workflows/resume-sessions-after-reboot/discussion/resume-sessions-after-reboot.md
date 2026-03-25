# Discussion: Resume Sessions After Reboot

## Context

Portal manages tmux sessions. After a system reboot, tmux-resurrect restores session layout, CWDs, and pane structure — but processes are dead. The idea is a generic restart command registry so Portal can resume processes (Claude Code, dev servers, etc.) in their original panes.

The core problem: tmux-resurrect's `@resurrect-processes` re-runs original launch commands, which doesn't work for tools like Claude Code where the resume command differs from the launch command (`claude --resume <uuid>` vs `claude`).

### Key Research Findings

- Claude Code's `SessionStart` hook provides `session_id` (the UUID needed for resume) and fires on every session start/resume
- `$TMUX_PANE` is available in every tmux process, so Portal can discover pane association without explicit passing
- Tmux pane IDs (e.g. `%0`, `%1`) persist across resurrect
- A hook script stays tool-specific — it only knows about Claude's session ID; Portal handles pane mapping internally

### References

- [Research: Resume Sessions After Reboot](./../research/resume-sessions-after-reboot.md)

## Questions

- [ ] What scoping model should the registry use — per-pane, per-session, or per-window?
- [ ] When should restart commands execute — eagerly on bootstrap, lazily on session select, or hybrid?
- [ ] Should Portal detect dead processes or just execute whatever is registered?
- [ ] Should Portal confirm before sending commands to panes, or auto-execute?
- [ ] What should the subcommand be called and what's the CLI surface?
- [ ] What storage format and location for the registry?
- [ ] How should stale registrations be handled (pane closed before reboot, session deleted)?
- [ ] Should Portal warn about or prevent conflicts with `@resurrect-processes`?

---

*Each question above gets its own section below. Check off as completed.*

---
