# Review Tracking: killing-all-sessions-wipes-restore-state - Gap Analysis

## Findings

### 1. The governing rule promises more than the accepted residue allows

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §1.1 The rule (colliding with §5.2 Accepted residue)

**Problem**:
The rule at the top of the specification is absolute in both directions. A session the user kills never comes back. Detaching, `tmux kill-server` and a reboot leave every session restorable, and the user never has to shut Portal down before rebooting. The accepted-residue list says the fix does not deliver all of that:
- A kill can come back. Its synchronous save backs off when its session listing fails or tmux begins exiting. If tmux then exits before the daemon's next tick commits the kill, the killed session returns at the next restore.
- Sessions that stop some other way can still be lost at a reboot. A session whose panes run a program directly, or a fish shell, is removed. So is every session when macOS hard-kills pane programs before tmux.
- On the first reboot after upgrading, the user needs `portal uninstall` first to be covered. That contradicts "never has to shut Portal down beforehand".

A builder or tester who takes the rule as written has to make the kill half true somehow. The easy way is to exempt the kill's save from the new back-off rules. Then a kill save that runs as tmux exits, with its listing failed, commits an empty saved state and deletes every scrollback file. That is the wipe the fix exists to stop. A tester holding the rule would also write a "kill, then kill-server at once, and the session stays gone" expectation, which a correct build fails. The unchanged-by-design list already states the kill with this exception. But the rule is the first thing a builder reads, and it still states the absolute.

**Proposal**:
Leave the rule as stated and point it at the residue it is subject to. The residue decisions the specification already records settle this. Each one is a case the fix deliberately leaves short of the rule, and the unchanged-by-design list already words the kill rule with the matching exception. Dropping or narrowing the residue would reopen decisions already made, so only the wording is open. A one-sentence pointer adds the qualification without restating the residue list in a second place.

**Current**:
A session the user kills never comes back. A session that stops any other way does come back.

**Proposed Text**:
A session the user kills never comes back. A session that stops any other way does come back. The cases this fix still leaves short of the rule are accepted residue, recorded in §5.2.

**Resolution**: Approved
**Notes**: Determined by the residue decisions §5.2 already records. Applied to §1.1 as staged.

---

## Observations

- If a user's on-resume command itself starts with `exec`, the hook program replaces the hardened shell and becomes the pane's top process. At SIGTERM that pane is as exposed as a directly started program. The residue bullet for direct programs arguably covers it, but it does not name it.
- "Backs off" and "stands down" are used side by side. The logging section counts a failed restore-marker read as a back-off, while the stand-down rule names only a failed listing and a refused confirmation. Each passage says which case it means.
- Drop lines from the first tick after a boot whose restore skipped a session would sit in the same log as a reboot's shutdown lines. Their timestamps and process role tell the two apart.
