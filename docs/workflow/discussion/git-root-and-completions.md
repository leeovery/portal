---
topic: git-root-and-completions
status: in-progress
date: 2026-01-26
---

# Discussion: Git Root Detection & Shell Completions

## Context

Two small enhancements identified from the [tmux session managers analysis](../research/tmux-session-managers-analysis.md). Both originated from sesh's feature set. Neither is complex, but both benefit from nailing down the specifics before implementation.

**Git root detection**: When a user selects a directory for a new session (via file browser, `zw .`, or `zw <path>`), resolve to the git repository root rather than using the literal directory. Prevents registering subdirectories as projects.

**Shell completions**: Add a `zw completion` subcommand that outputs shell completion scripts. Cobra provides the generation; we just need to wire it up.

### References

- [tmux-session-managers-analysis.md](../research/tmux-session-managers-analysis.md) — topics 15, 16
- [ZW specification](../specification/zw.md) — current spec (concluded)
- sesh's `root` command and `completion` command as prior art

## Questions

- [ ] Should git root resolution be automatic or a suggestion?
- [ ] What happens for directories that aren't git repos?
- [ ] Where in ZW's flow does git root detection apply?
- [ ] How should `zw completion` be structured?
- [ ] Do these features require spec changes?

---
