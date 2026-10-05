# Plan: Killing All Sessions Wipes Restore State

## Phases

### Phase 1: The save path never acts on a session view it cannot confirm
status: draft

**Goal**: Every committer runs the shared commit cycle: the daemon's tick, its shutdown flush, and `portal state commit-now`. When its session listing fails, or when a read sent after the capture is refused (or answered by a server other than its own), that committer stands down instead of committing. It deletes no scrollback, empties no transcript, and leaves `sessions.json` naming only files that exist. It reports through its existing failure route with a back-off line. Separately, the daemon's dump never writes an unconfirmed empty capture over a saved transcript. Result: `tmux kill-server`, and a daemon SIGTERMed just before the server, keep the full saved state. The kill path, restore, the picker, the resolver and completion keep their current behaviour.

**Why this order**: The irreversible loss, the whole saved state wiped by the daemon's flush or a hook-run `commit-now` caught mid-capture, comes from here. It can be reproduced against real tmux, because the 10–30ms daemon-before-server window wiped the whole state in the sandbox. That gives this phase the failing-test-first shape of a bugfix. Phase 2 depends on it: a pane that outlives SIGTERM only saves its session if no committer can commit after tmux begins exiting, and this phase's confirmation is what guarantees that.

**Acceptance**:
- [ ] On a live runtime with saved sessions and scrollback, the daemon is SIGTERMed 10–30ms before the tmux server. This is repeated across many trials at the production default log level with nothing wrapping `tmux`. Every trial ends with `sessions.json` naming every session and every scrollback file present and non-empty.
- [ ] On a live runtime with saved sessions and scrollback, `tmux kill-server` is run. Afterwards `sessions.json` and every scrollback file are unchanged.
- [ ] The production client's `list-sessions` fails while the daemon's tick, its shutdown flush or `commit-now` runs its cycle. Nothing is written, and no scrollback file is deleted or emptied. Each committer fails through its existing route:
  - the tick logs the failure and re-touches `save.requested`;
  - `commit-now` logs it, touches `save.requested` and exits non-zero;
  - the flush logs it and reports `flush_completed=false`.

  A later tick then commits what the stood-down save did not, including a kill whose `commit-now` stood down.
- [ ] A capture's session listing or pane listing comes back empty (exit 0, no output) from a server that then refuses connections. When the cycle reaches its confirmation, nothing is written and the committer logs a line saying it backed off because tmux stopped answering.
- [ ] A capture's confirmation read is sent strictly after the last capture read and returns exit 0 with no output. When the cycle runs, the commit is written.
- [ ] `portal uninstall` kills `_portal-saver` on a running server. The daemon's shutdown flush then commits and reports `flush_completed=true`.
- [ ] A save's capture reads or confirmation reach a different tmux server started on the same socket. When the cycle runs, nothing is written.
- [ ] A cycle moves a waiting pane's transcript to its token-named path and then stands down. Afterwards, `sessions.json` names only scrollback files present on disk.
- [ ] A pane has a non-empty saved transcript and its capture comes back empty in a way that cannot be confirmed. When the daemon's dump runs, the saved transcript is unchanged and a line naming the pane (`pane_key`) is logged.
- [ ] The session listing fails at bootstrap. Restore rebuilds every session from the saved state, and no empty commit follows it. The picker, the resolver and shell completion still read a failed listing as "no sessions".
- [ ] Several live sessions are killed one at a time. Each kill removes that session and its scrollback, and the last kill leaves zero sessions and zero scrollback files. The next hook-staleness sweep reaps the killed sessions' resume hooks. The empty-save contract tests listed in §5.1 still pass.

### Phase 2: Portal's resume-hook panes outlast the shutdown signal, and every dropped session is logged by name
status: draft

**Goal**: Restored eager and lazy resume-hook panes catch SIGTERM and never ignore it. That covers the eager hook shell, the lazy parked chain, the panel's draw and waiter, and the hook shell beneath an answered lazy pane. Their sessions stay up until tmux exits, and a waiting pane stays waiting with its transcript. A kill's SIGHUP still ends any of them at once. Every commit that drops a session logs each dropped session by name at INFO, measured against the prior on-disk index.

**Why this order**: The second root cause is sessions dying before tmux does. Hardening those panes only protects a session once tmux's exit refuses every committer, which Phase 1 delivers. With that in place, a pane that outlives SIGTERM keeps its session until no save can reach the server. The drop logging lands here because it records exactly what this phase leaves behind: sessions that still close while tmux is answering. That record is the evidence the deferred hold waits on.

**Acceptance**:
- [ ] Restored eager and lazy resume-hook panes each receive SIGTERM on the pane's top process. Each pane and its session stay up. The hook program and the user's shell started afterwards still end on SIGTERM (default handling, no inherited ignore).
- [ ] A lazy pane has been answered on its panel and its hook program is still running. SIGTERM reaches every process in the pane. The session is kept, and the pane goes on to the user's shell when the hook program ends.
- [ ] A lazy waiting pane's waiter has caught a SIGTERM, and the user then answers the panel. The hook program and the user's shell run with default SIGTERM handling.
- [ ] A lazy pane is waiting. SIGTERM reaches its whole process tree alongside the daemon. The dump-less `commit-now` from the `_portal-saver` close lands, and the server is ended afterwards. At the next restore, the pane's token-named transcript is still referenced and present, and the pane comes back still asking.
- [ ] A lazy pane's panel is still being drawn when SIGTERM lands. The pane is left waiting, exactly as for a SIGTERM landing on the waiter.
- [ ] A waiting pane's pty closes because tmux has begun exiting. The parked chain ends on SIGHUP before its recovery tail starts, no marker clear is attempted, and the saved record keeps the pane waiting.
- [ ] A waiting or eager resume-hook pane is killed. It dies at once, and the kill removes it from the saved state as before.
- [ ] On a live runtime against real tmux, the pane programs and the daemon are signalled, then the server. Every session whose panes are interactive shells or Portal's hardened panes is preserved.
- [ ] A commit drops one or more sessions. Each dropped session is logged by name (`session`) at INFO.
- [ ] A commit drops no session: the daemon tick after a `commit-now` kill, or a commit following a session rename. No drop line is logged.
