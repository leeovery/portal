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
- [ ] A cycle moves a waiting pane's transcript to its token-named path and then ends without committing: it stands down, the daemon's tick is cancelled mid-dump and the shutdown flush that follows stands down, or its `sessions.json` write fails. Afterwards, `sessions.json` names only scrollback files present on disk.
- [ ] A pane has a non-empty saved transcript and its capture comes back empty in a way that cannot be confirmed. When the daemon's dump runs, the saved transcript is unchanged and a line naming the pane (`pane_key`) is logged.
- [ ] The session listing fails at bootstrap. Restore rebuilds every session from the saved state, and no empty commit follows it. The picker, the resolver and shell completion still read a failed listing as "no sessions".
- [ ] Several live sessions are killed one at a time. Each kill removes that session and its scrollback, and the last kill leaves zero sessions and zero scrollback files. The next hook-staleness sweep reaps the killed sessions' resume hooks. The empty-save contract tests listed in §5.1 still pass.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-1-1 | Failed session listing stands the commit cycle down | restore with a failed listing still rebuilds every session from the saved state and no empty commit follows it (§2.1, §6.1), the picker, the resolver and shell completion still read a failed listing as no sessions (§2.1, §5.1) |
| killing-all-sessions-wipes-restore-state-1-2 | A refused confirmation read stands the commit cycle down | a confirmation answered with exit 0 and no output still counts (§2.2, §6.1), an empty `list-sessions` (exit 0, no output) from a server that then refuses connections writes nothing (§2.2, §6.1), an empty `list-panes` beside an environment read that still succeeds writes no windowless session and deletes no scrollback (§2.2, §6.1), a confirmed empty listing still commits the empty index (§5.1) |
| killing-all-sessions-wipes-restore-state-1-3 | The confirmation counts only from the committer's own server | `portal uninstall` kills `_portal-saver` on a running server and the shutdown flush still commits and reports `flush_completed=true` (§2.2, §6.1), a new server on the same socket that answers capture reads or the confirmation before its restore has run gets nothing written (§2.2, §6.1), commit-now's own server is the one whose `session-closed` hook ran it (§2.2) |
| killing-all-sessions-wipes-restore-state-1-4 | Each committer reports a stand-down through its existing failure route with a back-off line | a kill whose `commit-now` stood down is committed by the daemon's next tick (§5.1, §5.2), a transient listing failure on a healthy server delays the save by one tick (§2.5), the existing WARN lines for a failed restore-marker read stay as they are (§4.2), a marker that reads as set logs nothing new (§4.2) |
| killing-all-sessions-wipes-restore-state-1-5 | The daemon's dump never writes an unconfirmed empty capture over a saved transcript | a confirmed empty capture still replaces the saved transcript (§2.4), the confirming read must be answered by the server that answered the capture (§2.4) |
| killing-all-sessions-wipes-restore-state-1-6 | A cycle that ends uncommitted never leaves sessions.json naming a missing scrollback file | a stand-down injected after the capture cycle's renames (§2.3, §6.1), a daemon tick cancelled mid-dump after the renames and followed by a shutdown flush that stands down (§2.3, §6.1), a `sessions.json` write that fails after the renames (§2.3, §6.1), a stand-down at shutdown where no later cycle runs to repair the record (§2.3) |
| killing-all-sessions-wipes-restore-state-1-7 | Kill path stays final under the hardened save path | the last user session's kill commits the empty state while tmux keeps running for `_portal-saver` and `_portal-bootstrap` (§5.1), the next hook-staleness sweep reaps the killed sessions' resume hooks (§1.1, §6.4), the three empty-save contract tests listed in §5.1 stay green and unchanged (§5.1) |
| killing-all-sessions-wipes-restore-state-1-8 | Shutdown orderings against real tmux preserve the full saved state | the trials run at the production default log level with nothing wrapping `tmux` (§6.5), survival under `kill-server` no longer depends on which read fails first (§2.2, §6.5) |
| killing-all-sessions-wipes-restore-state-1-9 | Every capture read refused by an exiting tmux logs as a back-off | a pane listing that is answered but fails to parse, a carry collision, and an all-sessions-anomalous capture still log `tick failed` (§4.2), a failed restore-in-progress marker read keeps its existing line (§4.2) |

### Phase 2: Portal's resume-hook panes outlast the shutdown signal, and every dropped session is logged by name
status: draft

**Goal**: Restored eager and lazy resume-hook panes catch SIGTERM and never ignore it. That covers the eager hook shell, the lazy parked chain, the panel's draw and waiter, and the hook shell beneath an answered lazy pane. Their sessions stay up until tmux exits, and a waiting pane stays waiting with its transcript. A kill's SIGHUP still ends any of them at once. Every commit that drops a session logs each dropped session by name at INFO, measured against the prior on-disk index.

**Why this order**: The second root cause is sessions dying before tmux does. Hardening those panes only protects a session once tmux's exit refuses every committer, which Phase 1 delivers. With that in place, a pane that outlives SIGTERM keeps its session until no save can reach the server. The drop logging lands here because it records exactly what this phase leaves behind: sessions that still close while tmux is answering. That record is the evidence the deferred hold waits on.

**Acceptance**:
- [ ] Restored eager and lazy resume-hook panes each receive SIGTERM on the pane's top process. Each pane and its session stay up. The hook program and the user's shell started afterwards receive SIGTERM with default handling: each starts with SIGTERM at its default disposition, not inherited as ignored.
- [ ] A lazy pane has been answered on its panel and its hook program is still running. SIGTERM reaches every process in the pane. The session is kept, and the pane goes on to the user's shell when the hook program ends.
- [ ] A lazy waiting pane's waiter has caught a SIGTERM, and the user then answers the panel. The hook program and the user's shell run with default SIGTERM handling.
- [ ] A lazy pane is waiting. SIGTERM reaches its whole process tree alongside the daemon. The dump-less `commit-now` from the `_portal-saver` close lands, and the server is ended afterwards. At the next restore, the pane's token-named transcript is still referenced and present, and the pane comes back still asking.
- [ ] A lazy pane's panel is still being drawn when SIGTERM lands. The pane is left waiting, exactly as for a SIGTERM landing on the waiter.
- [ ] SIGTERM lands as the draw or the waiter starts, before it has installed its catch: at restore, as the draw execs into the waiter, or at a redraw. The pane is left waiting, with its marker set and no recovery tail run.
- [ ] A waiting pane's pty closes because tmux has begun exiting. The parked chain ends on SIGHUP before its recovery tail starts, no marker clear is attempted, and the saved record keeps the pane waiting.
- [ ] A waiting or eager resume-hook pane is killed. It dies at once, and the kill removes it from the saved state as before.
- [ ] On a live runtime against real tmux, the pane programs and the daemon are signalled, then the server. Every session whose panes are interactive shells or Portal's hardened panes is preserved.
- [ ] A commit drops one or more sessions. Each dropped session is logged by name (`session`) at INFO.
- [ ] A commit drops no session: the daemon tick after a `commit-now` kill, or a commit following a session rename. No drop line is logged.

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-2-1 | The eager resume shell catches SIGTERM | the hook program and the user's shell keep default SIGTERM handling and never inherit an ignore (§3.1, §6.2), the same shell beneath an answered lazy pane survives SIGTERM too (§3.1) |
| killing-all-sessions-wipes-restore-state-2-2 | The panel's draw and its waiter catch SIGTERM and keep the pane waiting | a SIGTERM landing while the panel is still being drawn leaves the pane waiting (§3.2, §6.2), a waiter that caught a SIGTERM and is then answered gives its hook program and the user's shell default SIGTERM handling (§3.2, §6.2) |
| killing-all-sessions-wipes-restore-state-2-3 | The parked chain catches SIGTERM, so a waiting or answered lazy pane keeps its session | an answered lazy pane whose hook program is still running keeps its session when SIGTERM reaches every process in the pane, and goes on to the user's shell (§3.1, §6.2), a SIGTERM to the waiting pane's whole process tree leaves the panel up, the marker set and the recovery tail unrun (§3.2, §6.2), the trap stays caught so the hook program and the user's shell keep default handling (§3.1) |
| killing-all-sessions-wipes-restore-state-2-4 | A SIGTERM landing as the draw or the waiter starts still leaves the pane waiting | at restore, as the parked shell starts the draw (§3.2, §6.2), as the draw execs into the waiter (§3.2, §6.2), at a redraw, as the waiter execs back into the draw (§3.2, §6.2), the window is closed by a catch, never an ignore, so an answered pane's hook program and the user's shell keep default SIGTERM handling (§3.2) |
| killing-all-sessions-wipes-restore-state-2-5 | A kill or tmux's exit still ends a hardened pane at once | a killed waiting pane and a killed eager pane each die at once and the kill removes them from the saved state as before (§3.3, §6.2), when tmux's exit closes a waiting pane's pty the parked chain ends on SIGHUP before its recovery tail starts, no marker clear is attempted, and the saved record keeps the pane waiting (§3.2, §6.2) |
| killing-all-sessions-wipes-restore-state-2-6 | Every commit that drops a session logs it by name | the daemon tick after a `commit-now` kill logs no second drop line (§4.1, §6.3), drops are measured against the prior on-disk index, not the daemon's in-memory previous index (§4.1), a commit that drops nothing logs nothing new (§4.1, §6.3), the line carries the name in `session` at INFO and adds no log component or attribute key (§4.2) |
| killing-all-sessions-wipes-restore-state-2-7 | A renamed session is not logged as dropped | a session dropped at shutdown, where no new session appears beside it, is still logged as dropped whichever identity tells renames apart (§4.1, §4.3) |
| killing-all-sessions-wipes-restore-state-2-8 | A waiting pane outlasts a shutdown SIGTERM and comes back still asking | the dump-less `commit-now` fired by the `_portal-saver` close lands between the pane's SIGTERM and the server's end (§3.2, §6.2), the server's exit then ends the parked chain on SIGHUP before its recovery tail (§3.2) |
| killing-all-sessions-wipes-restore-state-2-9 | Signalling pane programs and the daemon before the server preserves hardened sessions against real tmux | sessions whose panes are interactive shells, eager resume panes, waiting lazy panes or answered lazy panes are all preserved (§3.1, §6.5), a session whose pane runs a program started directly rather than inside a shell is accepted residue and is not asserted preserved (§5.2) |

### Phase 3: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-3-1 | The commit cycle owns the empty-capture confirmation of every scrollback write | a confirmation refused, answered by another server or naming no server keeps the saved transcript (§2.4, §4.2), a confirmed empty capture is still written (§2.4), a non-empty capture, an empty capture with no saved transcript and an empty capture over an empty saved transcript send no confirmation read (§2.4), a dedup match writes nothing (§2.4), the guard catches a value use under an import alias and ignores test files, `internal/state` and a same-named selector on another package (§2.4) |
| killing-all-sessions-wipes-restore-state-3-2 | A capture that fails because tmux stopped answering logs as a back-off whichever read failed | every environment read refused then a refused confirmation logs the back-off line from the tick, the flush and `commit-now` (§2.5, §4.2), the own server answering keeps `tick failed` for a parse failure, a carry collision and an all-anomalous capture (§4.2), an unknown own server sends no confirmation and returns the capture's error unchanged (§2.2), a refused session or pane listing ends the cycle with no confirmation after it (§4.2) |
| killing-all-sessions-wipes-restore-state-3-3 | CLAUDE.md describes the save path and resume shells as they were before this fix | every touched sentence holds whether or not the other analysis tasks land, no source or test file changes (§2.3, §3.1) |
| killing-all-sessions-wipes-restore-state-3-4 | Corrections | the `commit-now` subtest stands down on the failed listing itself, not on an unknown own server (§6.1), no production code or other test changes (§6.1) |
| killing-all-sessions-wipes-restore-state-3-5 | An Answered Lazy Pane Keeps Its Transcript Until A Confirmed Save Replaces It | an empty capture whose confirmation is refused, answered by another server or naming no server keeps the token-named transcript (§2.4, §4.2, §1.1), a refused capture or failed write keeps every later commit naming the token-named file (§1.1), a dump-less `commit-now` before any dump names the token-named transcript (§1.1, §3.2), an empty capture is judged against the token-named transcript so its confirmation read is sent (§2.4), a cycle ending uncommitted never leaves `sessions.json` naming a missing positional file (§2.3, §1.1), a pane that never waited and a pane still waiting are saved as today (§2.3, §2.4) |

### Phase 4: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-4-1 | The Answered-Pane Hold Is Decided From The Last Committed Index | the daemon's previous index predates the `commit-now` that filed the pane under its token and the pane's new positional path holds another pane's old transcript (§2.4), a confirmed dump still moves the pane to its positional file and reclaims the token-named transcript (§2.4), a fresh and a stale previous index commit the same record whether the positional path is absent or occupied (§2.4), `sessions.json` absent or not decodable falls back to the caller's previous index (§2.4), the skeleton, waiting-pane and carry merges and the drop and no-change measures are unchanged (§4.1) |
| killing-all-sessions-wipes-restore-state-4-2 | A Session Listing Tmux Answered But Portal Could Not Parse Is Classified By The Confirmation | a session named `work\|notes` with the own server answering logs `… failed` from the tick, the flush and `commit-now` (§2.5, §4.2), the failure routes still re-touch `save.requested`, exit non-zero and report `flush_completed=false` (§2.5), an unparseable listing whose confirmation is refused, names no server or is answered by another server logs the back-off line, and an unknown own server returns the parse error unchanged (§2.2, §4.2), a refused `list-sessions` still backs off with no confirmation after it (§2.1, §4.2), a parse failure matches the new `tmuxerr` sentinel and a failed read does not (§4.2) |

### Phase 5: Analysis (Cycle 3)

**Goal**: Address findings from Analysis (Cycle 3).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-5-1 | The Scrollback Writer Refuses A Pane The Cycle Skips | a skeleton-marked, waiting or carried pane handed to the writer writes nothing, sends no confirmation and leaves its dedup entry and saved transcript as they were, an empty capture the own server would confirm is refused the same way, the committed `sessions.json` names the same file for the skipped pane as a dump that never handed it over, a pane the cycle does not skip is written exactly as before, the daemon's tick still sends no `capture-pane` for a skeleton-marked or waiting pane |
| killing-all-sessions-wipes-restore-state-5-2 | A Confirmation Naming No Server Is Refused In Its Own Right | a cycle and `commit-now` that do not know their own server stand down against a confirmation naming no server (§2.2), either check alone keeps that stand-down and only removing both fails the test (§2.2), a cycle that knows its own server refuses an answer naming no server after the capture, after a failed capture and before an empty capture replaces a saved transcript (§2.2), an unknown own server with a failed capture sends no confirmation and returns the capture's error unchanged, `ConfirmAnswering` still answers pid 0 with no error and a silent answer stays classified apart from a refused read |

### Phase 6: Analysis (Cycle 4)

**Goal**: Address findings from Analysis (Cycle 4).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-6-1 | The Lazy-Resume Fixture's Capture Rounds Write Through The Cycle's Scrollback Writer | an answered pane's next round commits its record naming its positional file, and that file holds the capture (§2.4), the same subtest fails against the former `WriteScrollbackIfChanged` route (§2.4), a waiting or skeleton-marked pane is still reported unwritten while a live pane beside it is written (§2.4), the five lazy-resume suites keep every existing assertion (§2.4), no non-test file changes and `WriteScrollbackIfChanged` stays exported (§2.4) |
| killing-all-sessions-wipes-restore-state-6-2 | Each Commit Cycle Merges And Carries From The Index It Read Under The Lock | an answered pane in a carried session keeps naming its token-named transcript, which stays on disk, from the tick and the shutdown flush (§2.4, §1.1), a stale and a fresh in-memory index commit the same index and files for skeleton-marked, waiting and carried panes (§2.4), a readable `sessions.json` never calls `LoadPrev` (§2.4), an absent or undecodable `sessions.json` calls `LoadPrev` once under the lock and `commit-now` keeps its WARN lines (§2.4), the lag repairs and the uncommitted-end tests stand (§2.3) |
| killing-all-sessions-wipes-restore-state-6-3 | Commit-Now's Previous-Index Loader Is Reduced To The Fallback It Now Is | an absent and an undecodable `sessions.json` each commit from a zero-value previous index with their WARN text unchanged, a readable `sessions.json` runs the real cycle with no `LoadPrev` call and neither WARN, the stand-in cycle calls `LoadPrev` only over an absent or undecodable file, the byte-identical-on-failed-commit test's undecodable seed still sends the stand-in through `LoadPrev`, no test is added and no surviving test's assertions change |
| killing-all-sessions-wipes-restore-state-6-4 | Corrections | each old sentence in the `state` row is replaced in place between its neighbouring sentences, neither old phrase is found in CLAUDE.md afterwards, no other CLAUDE.md text and no Go source or test file changes |

### Phase 7: Analysis (Cycle 5)

**Goal**: Address findings from Analysis (Cycle 5).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-7-1 | A Pane Restored Away From Its Saved Address Keeps Its Transcript Until A Confirmed Save Replaces It | an empty capture whose confirmation is refused, answered by another server or naming no server keeps the saved file and writes nothing at the live positional path, a refused `capture-pane`, a dump-less `commit-now` or a dump-less flush keeps every later commit naming the saved file, a confirmed write files the pane at its positional path and reclaims the old file, a saved file another record names leaves the tokened pane judged at its own positional file with no file on two records, a tokenless pane restored one window lower is judged at its own positional file, an answered lazy pane on its token-named transcript is held exactly as today |
| killing-all-sessions-wipes-restore-state-7-2 | Every Moved Tokened Pane In A Shifted Run Keeps Its Transcript | the pane moved 2→1 in a shifted run keeps its saved bytes on its token-named transcript whether or not the pane moved 3→2 is written to its old file in the same dump, a refused `capture-pane`, a dump-less `commit-now` and a dump-less flush in sequence delete no saved transcript, a swap never leaves the refused pane's record naming the other pane's bytes, a new tokenless pane at the vacated address in the same or a later cycle leaves the moved pane's bytes intact with no file on two records, a confirmed write files the pane at its positional path and reclaims the token-named transcript, a linked cycle ending uncommitted leaves `sessions.json` naming only files on disk, a failed link keeps today's judgement and logs one WARN with `pane_key`, `path` and `error`, two panes claiming one file, a token the pane-token rule refuses and an answered lazy pane are unchanged |
| killing-all-sessions-wipes-restore-state-7-3 | A Pane With No Resume Hook Restored Away From Its Saved Address | every pane restored without a saved token carries a token and its first committed record names its own saved bytes whether that commit lands skeleton-marked or after hydration, an unconfirmed empty capture, a refused `capture-pane`, a dump-less `commit-now` and a dump-less flush keep the saved transcript named and on disk, a confirmed write files the pane at its positional path and reclaims the old transcript, a pane at its own saved address keeps today's judgement, `hook set` in such a pane registers under the restore-given token and stamps nothing, a saved token is re-stamped as today and a tokenless saved pane fires no resume hook |

### Phase 8: Analysis (Cycle 6)

**Goal**: Address findings from Analysis (Cycle 6).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-8-1 | Every Committer Logs An Unreadable sessions.json From The Cycle That Read It | an undecodable `sessions.json` under a running daemon logs `read sessions.json failed; committing without the saved index` once at WARN under `daemon` with the cause in `error`, then commits from the in-memory index (§4.2), an absent file on a fresh install logs the absent line once and commits, a present but unreadable file gets the failure line and never the absent one, `commit-now` warns in exactly today's cycles in the shared words and commits from an empty index with no second read, a cycle that stands down or fails before its commit has still logged its line, a readable file logs neither line and calls no `LoadPrev`, `state.Commit` logs neither line and writes as today, `CommitNowDeps.ReadIndex` and `loadPrevIndex` are gone and the seam-convention coverage guard passes |
| killing-all-sessions-wipes-restore-state-8-2 | The Daemon Reports An Unreadable sessions.json Once, From The Cycle | starting the daemon over an undecodable `sessions.json` logs nothing before the tick loop and seeds no previous index, its first tick logs `read sessions.json failed; committing without the saved index` once at WARN under `daemon` with `error` wrapping `state.ErrCorruptIndex` and commits as today, a first tick that stands down still holds that one WARN and no `ReadIndex failed` under `daemon`, a missing file still logs nothing at startup and the absent line once from the tick, a readable file still seeds the previous index, `commit-now` keeps the same one line and restore keeps its `ReadIndex failed` under `restore` |
| killing-all-sessions-wipes-restore-state-8-3 | Corrections | the replacement lands verbatim where the old sentence stood between its neighbouring sentences in the `state` row, `zero-value fallback with a WARN` is found nowhere in CLAUDE.md afterwards, no other CLAUDE.md text and no Go source or test file changes |

### Phase 9: Review Remediation (Cycle 1)

**Goal**: Address findings from Review Remediation (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| killing-all-sessions-wipes-restore-state-9-1 | The Dump Never Replaces A Scrollback Name Another Pane's Saved Record Still Names | a moved pane and a second pane at its old address whose cycle commits, is cancelled mid-dump before a stood-down flush, or fails its `sessions.json` write each leave every tokened pane's record naming its own transcript (§2.3, §2.4, §6.1), the commit after a token-named write files the pane back at its positional name and reclaims the token-named file (§2.4), two tokened panes swapping addresses each write to their own token-named transcript (§2.3, §2.4), a new tokenless pane or a pane whose token the rule refuses at a contested name is deferred with its dedup entry untouched (§2.3, §5.2), an absent or unreadable `sessions.json`, a tokenless committed record or the pane's own token contests nothing (§2.4), an unconfirmed empty capture at a contested name writes at neither name (§2.4) |
