# Discussion: Auto-Start Tmux Server

## Context

After a system reboot, running Portal (`x`) shows no sessions because the tmux server hasn't started yet. tmux-continuum can't restore sessions until a server exists. A LaunchAgent (`com.leeovery.tmux-boot`) currently works around this by starting the server on login, but it leaves a leftover `_boot` session.

Portal should self-bootstrap the tmux server when it detects none is running, removing the LaunchAgent dependency entirely.

### References

- [ideas/auto-start-tmux-server.md](../../../ideas/auto-start-tmux-server.md) — Original idea doc with proposed flow and edge cases

## Questions

- [ ] Where should EnsureServer live — tmux.Client method or standalone function?
- [ ] Where in the call chain should it be invoked — PersistentPreRunE, openTUI, or elsewhere?
- [ ] Should non-TUI commands (list, attach, kill) also trigger server bootstrap?
- [ ] How should the TUI handle the bootstrap wait — block before TUI, or show a loading state?
- [ ] How to handle _boot session cleanup — timing and responsibility?
- [ ] Should the resurrect file path be configurable or hardcoded?

---
