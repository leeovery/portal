# Review Tracking: Killing All Sessions Wipes Restore State - Traceability

## Findings

### 1. Phase 2's sign-off check requires the user's shell to die on SIGTERM

**Type**: Hallucinated content
**Spec Reference**: §1.2 (interactive shells ignore SIGTERM), §3.1, §5.2 (the fish residue), §6.2 first bullet
**Plan Reference**: Phase 2, Acceptance, first criterion (planning.md)
**Move**: settled
**Change Type**: update-task

**Problem**:
Phase 2's sign-off criterion asks that the user's shell, once a hardened pane hands over to it, "still end on SIGTERM". The user's shell is an interactive zsh, and zsh ignores SIGTERM on its own. It has to: that is what keeps the pane and its session up after the hook program ends. The spec's fish residue exists only because fish's shell does exit on SIGTERM. So the criterion as written fails on every correct build when the phase is signed off. The other way to pass it is to test with a non-interactive shell standing in for the user's, which proves something else. The spec actually requires that the hook program and the user's shell start with SIGTERM at its default disposition, with the trap never inherited as an ignore. Tasks 2.1 and 2.3 already state that correctly in their criteria.

**Proposal**:
Restate the criterion in the spec's terms (§6.2, first bullet): the hook program and the user's shell receive SIGTERM with default handling, meaning each starts with SIGTERM at its default disposition and never inherits it as ignored. Three spec passages decide this. §6.2 requires default handling, not death. §1.2 states that interactive shells ignore SIGTERM. §5.2 relies on the user's zsh outliving SIGTERM to keep the pane up once the hook ends.

**Current**:
- [ ] Restored eager and lazy resume-hook panes each receive SIGTERM on the pane's top process. Each pane and its session stay up. The hook program and the user's shell started afterwards still end on SIGTERM (default handling, no inherited ignore).

**Proposed Text**:
- [ ] Restored eager and lazy resume-hook panes each receive SIGTERM on the pane's top process. Each pane and its session stay up. The hook program and the user's shell started afterwards receive SIGTERM with default handling: each starts with SIGTERM at its default disposition, not inherited as ignored.

**Resolution**: Fixed
**Notes**:

---

### 2. Task 2.1's Outcome requires the user's shell to die on SIGTERM

**Type**: Hallucinated content
**Spec Reference**: §1.2 (interactive shells ignore SIGTERM), §3.1, §5.2 (the fish residue), §6.2 first bullet
**Plan Reference**: Phase 2, Task killing-all-sessions-wipes-restore-state-2-1 (tick-23370c), Outcome
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 2.1's Outcome, the end state its implementer and reviewer check against, says the hook program and the user's shell "still end on SIGTERM". When an eager pane's hook ends, the shell it hands over to is the user's interactive zsh. zsh ignores SIGTERM on its own, and that is what keeps the session up afterwards. An implementer checking this Outcome finds that a correct build fails it. The other way to pass it is to stand in a non-interactive `$SHELL`, which proves nothing about the pane the user gets. A reviewer reading the Outcome as written could also reject a correct build. The task's own criteria are already right: AC3 says the user's shell starts with SIGTERM at its default disposition, not inherited as ignored. So the Outcome contradicts both its own criteria and the spec.

**Proposal**:
Restate the Outcome's last sentence in the spec's terms (§3.1, §6.2): the hook program and the user's shell start with SIGTERM at its default disposition, never inherited as ignored. This matches §6.2's "receive SIGTERM with default handling" and the task's AC3. It also matches §1.2 and §5.2, under which the user's interactive shell outlives SIGTERM.

**Current**:
**Outcome**: A SIGTERM to the shell running a resume hook no longer ends the pane or its session. The session stays up until tmux itself exits, and by then no committer can reach the server. The hook program and the user's shell still end on SIGTERM.

**Proposed Text**:
**Outcome**: A SIGTERM to the shell running a resume hook no longer ends the pane or its session. The session stays up until tmux itself exits, and by then no committer can reach the server. The hook program and the user's shell start with SIGTERM at its default disposition, never inherited as ignored.

**Resolution**: Fixed
**Notes**:

---

## Observations

- Task 1.8's third criterion (`tmux kill-server` run across trials "so that the exit lands at different points in the save") is a test the spec's §6.5 does not list. The property it checks (§2.2) is already proven deterministically by Task 1.2's fourth criterion. Where the exit landed also cannot be observed at the default log level with nothing wrapping tmux, which is the condition the same task imposes.
- Task 1.7's fourth criterion has the empty-save contract tests pass "with their assertions unchanged", which is looser than §5.1's "stay as they are". The task's Do already carries the spec's wording ("Leave these tests as they are").
