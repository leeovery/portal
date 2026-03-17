# Discussion: Auto-Start Tmux Server

## Context

After a system reboot, running Portal (`x`) shows no sessions because the tmux server hasn't started yet. tmux-continuum can't restore sessions until a server exists. A LaunchAgent (`com.leeovery.tmux-boot`) currently works around this by starting the server on login, but it leaves a leftover `_boot` session.

Portal should self-bootstrap the tmux server when it detects none is running, removing the LaunchAgent dependency entirely.

### References

- [ideas/auto-start-tmux-server.md](../../../ideas/auto-start-tmux-server.md) — Original idea doc with proposed flow and edge cases

## Questions

- [ ] What's the right bootstrap flow — and does the proposed sequence from the idea hold up?
- [ ] Which Portal commands should trigger bootstrap vs skip it?
- [ ] What should the user experience be during the wait for session restore?
- [ ] How should we handle the _boot session lifecycle?
- [ ] What happens when things go wrong — timeouts, no resurrect data, partial restores?

---
