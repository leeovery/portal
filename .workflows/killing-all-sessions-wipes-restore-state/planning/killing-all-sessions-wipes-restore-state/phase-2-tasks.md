# Phase 2: Portal's resume-hook panes outlast the shutdown signal, and every dropped session is logged by name — 9 tasks

## killing-all-sessions-wipes-restore-state-2-1

### Task 2.1: The eager resume shell catches SIGTERM

**Problem**: A restored eager resume-hook pane runs as a non-interactive shell, `sh -c '<hook>; exec <shell>'` (`hookExecArgs` in `cmd/state_resume_chain.go`), and SIGTERM kills it. The shutdown SIGTERM reaches the programs in panes independently of the tmux server. A session whose programs exit on it closes while the server still answers. Two saves then remove that session and its scrollback: the `commit-now` that the `session-closed` hook runs synchronously, and the daemon's SIGTERM flush. Interactive shells ignore SIGTERM, so the sessions most exposed are the ones carrying resume hooks. A lazy pane the user has answered runs its hook through this same shell, beneath its parked chain, so it carries the same exposure.

**Solution**: The shell a resume hook runs in survives SIGTERM, the way an interactive shell already does. The handling is a caught trap, never an ignored disposition, so the hook program and the user's shell the pane goes on to keep default SIGTERM handling. An answered lazy pane runs its hook through the same shell, so the same change covers it.

**Outcome**: A SIGTERM to the shell running a resume hook no longer ends the pane or its session. The session stays up until tmux itself exits, and by then no committer can reach the server. The hook program and the user's shell start with SIGTERM at its default disposition, never inherited as ignored.

**Acceptance Criteria**:
- [ ] A restored eager resume pane's hook program is still running, and the pane's top process (the shell running the hook) receives SIGTERM. The pane and its session stay up, the hook program keeps running, and when it ends the pane goes on to the user's shell. (§3.1, §6.2)
- [ ] A restored eager resume pane's hook program receives SIGTERM. It ends on it with default handling, and the pane goes on to the user's shell. (§3.1, §6.2)
- [ ] The user's shell an eager resume pane goes on to starts with SIGTERM at its default disposition, not inherited as ignored. (§3.1, §6.2)
- [ ] A lazy pane has been answered on its panel and its hook program is still running. The shell running that hook receives SIGTERM. It survives, its hook program and the user's shell keep default SIGTERM handling, and when the hook program ends the pane goes on to the user's shell. (§3.1)

**Do**:
- The eager resume shell is composed by `hookExecArgs` (`cmd/state_resume_chain.go`). It is the one shell a hook runs in, whether the hydrate helper hands an eager pane to it or the waiter hands an answered lazy pane to it (`handOffToHookOrShell`, same file) (§3.1).
- Handle SIGTERM with a caught trap, never an ignored disposition (§3.1).
- Leave SIGHUP uncaught, so a kill still ends the pane (§3.3).

**Context**:
> Portal starts two kinds of pane as a non-interactive shell: this eager resume pane, and the lazy waiting pane's parked chain (Task 2.3). They are the only non-interactive-shell panes Portal creates (`rg -n '"sh", "-c"' --type go -g '!*_test.go' cmd internal | wc -l` → 2). Session trees run `$SHELL -ic`, which is interactive (`BuildShellCommand` in `internal/session/create.go`). Every other pane process Portal starts is either not a shell (the hydrate helper, the saver's daemon) or an interactive shell (`_portal-bootstrap`, created with no command by `StartServer` in `internal/tmux/tmux.go`) (§3.1).
>
> The handling must be a caught trap because an ignored signal stays ignored across `exec`, so the hook program and the user's shell would inherit it. That is already the parked chain's rule (`parkedChainTrap`, `cmd/state_hydrate.go`) (§3.1).
>
> An answered lazy pane keeps its session when SIGTERM reaches every process in it only once its parked chain survives too. That is Task 2.3.
>
> Accepted residue (§5.2): fish installs a SIGTERM handler that exits, so once a fish user's hook ends, their pane is exposed again (the user's shell is zsh). Portal's resume-hook panes keep running the version they were started with until the next restore rebuilds them.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2.2, §3.1, §3.3, §5.2, §6.2

## killing-all-sessions-wipes-restore-state-2-2

### Task 2.2: The panel's draw and its waiter catch SIGTERM and keep the pane waiting

**Problem**: While a lazy pane waits, its parked shell runs the panel's draw, which hands off to the waiter (`portal state resume-draw` → `portal state resume-wait`). Today the waiter handles only SIGWINCH (`rg -n 'signal.Notify' cmd/state_resume_wait.go` → 1 hit, `SIGWINCH`), so a reboot SIGTERM ends it. The parked shell then runs its recovery tail, and `resume-recover` clears `@portal-resume-pending` while tmux is still answering. A capture then builds the pane a fresh record naming its positional scrollback path, a file that was renamed away when the pane first went waiting. A commit with no scrollback dump writes that record. One such commit is the `commit-now` that fires when `_portal-saver` itself closes after the daemon's flush. Its housekeeping pass then deletes the token-named transcript. The session survives, but its scrollback is silently lost.

**Solution**: The draw and the waiter both catch SIGTERM and carry on. The panel stays up and the marker stays set, so the pane is saved still waiting, with its transcript, and after the reboot it comes back still asking. The handling is a catch, never an ignore, so an answered pane's hook program and the user's shell keep default SIGTERM handling.

**Outcome**: A SIGTERM reaching a waiting pane's draw or waiter leaves the pane waiting, with its marker set and no recovery tail run. A pane answered after such a SIGTERM gives its hook program and the user's shell default SIGTERM handling.

**Acceptance Criteria**:
- [ ] A lazy pane is waiting on its panel, and its waiter receives SIGTERM. The panel stays up, `@portal-resume-pending` stays set on the pane, and no recovery tail runs. (§3.2, §6.2)
- [ ] A lazy pane's panel is still being drawn when its draw receives SIGTERM. The pane is left waiting on its panel, exactly as for a SIGTERM landing on the waiter: the marker stays set and no recovery tail runs. (§3.2, §6.2)
- [ ] A waiting pane is resized, so its panel is redrawn. A SIGTERM to the redrawn panel's waiter leaves the pane waiting, with its marker set and no recovery tail run. (§3.2)
- [ ] A waiting pane's waiter has caught a SIGTERM, and the user then answers the panel. The hook program ends on SIGTERM with default handling, and the user's shell the pane goes on to starts with SIGTERM at its default disposition, not inherited as ignored. (§3.2, §6.2)

**Do**:
- The draw is `runResumeDraw` (`cmd/state_resume_draw.go`). The waiter is `cmd/state_resume_wait.go`, whose signal registration (`winchSignals`) covers SIGWINCH alone today (§3.2).
- Catch SIGTERM, never ignore it (§3.2).
- SIGHUP keeps its default disposition, so tmux closing the pane's pty still ends the waiter (§3.3).

**Context**:
> Every process the waiting pane runs while it waits outlasts SIGTERM: the panel's draw, and the waiter it becomes. An ignored signal stays ignored across `exec`, and an answered pane goes on to run its hook program and then the user's shell (`handOffToHookOrShell`, `cmd/state_resume_chain.go`), which keep default SIGTERM handling (§3.2).
>
> A catch is not inherited across `exec`. The moment as each process starts, before it has installed its catch (at restore, as the draw execs into the waiter, and at every redraw), is Task 2.4. The parked shell's own SIGTERM trap is Task 2.3.
>
> When tmux finally exits, it closes the waiting pane's pty. The SIGHUP ends the parked shell, and the waiter with it, before the recovery tail can start. That is Task 2.5.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §3.2, §3.3, §6.2

## killing-all-sessions-wipes-restore-state-2-3

### Task 2.3: The parked chain catches SIGTERM, so a waiting or answered lazy pane keeps its session

**Problem**: A restored lazy pane runs as a parked chain, `sh -c 'trap : INT QUIT; <draw>; <recover>; <backstop>'` (`parkedChainTrap` / `execResumeChainAndExit` in `cmd/state_hydrate.go`). Its trap does not cover TERM, so a SIGTERM to the pane's top process kills it, and the session closes while tmux still answers. Lazy is the shipped default resume mode, so after a restore every unanswered pane runs as the parked chain. An answered lazy pane still has the parked shell above the shell running its hook, so it carries the same exposure.

**Solution**: The parked chain survives SIGTERM too, still through a caught trap. With the draw and the waiter catching SIGTERM (Task 2.2) and the hook's shell catching it (Task 2.1), a SIGTERM reaching every process in a waiting or answered lazy pane leaves the pane and its session up.

**Outcome**: A waiting lazy pane outlasts SIGTERM with its panel up, its marker set and its recovery tail unrun. An answered lazy pane whose hook program is still running keeps its session, and goes on to the user's shell when the hook program ends.

**Acceptance Criteria**:
- [ ] A restored lazy pane is waiting, and the pane's top process (its parked shell) receives SIGTERM. The pane and its session stay up, the panel stays up, `@portal-resume-pending` stays set, and no recovery tail runs. (§3.1, §6.2)
- [ ] A restored lazy pane is waiting, and SIGTERM reaches every process in the pane. The pane and its session stay up, the panel stays up, the marker stays set, and no recovery tail runs. (§3.2, §6.2)
- [ ] A lazy pane has been answered on its panel and its hook program is still running. SIGTERM reaches every process in the pane. The parked shell and the shell running the hook both survive, the session is kept, and when the hook program ends the pane goes on to the user's shell. (§3.1, §6.2)
- [ ] An answered lazy pane's hook program, and the user's shell it goes on to, start with SIGTERM at its default disposition, not inherited as ignored. (§3.1)

**Do**:
- The parked chain is composed by `parkedResumeChain` and launched by `execResumeChainAndExit` (`cmd/state_hydrate.go`). Its trap, `parkedChainTrap`, covers INT and QUIT alone today (§3.1).
- Keep the handling a caught trap, never an ignored disposition (§3.1).
- Leave SIGHUP uncaught, so a kill or tmux's exit still ends the pane (§3.2, §3.3).

**Context**:
> Today's trap is caught for the same reason: an ignored signal stays ignored across `exec`, and the hook program and the user's shell would inherit it (§3.1).
>
> A lazy pane the user has answered runs its hook through the eager shell, `sh -c '<hook>; exec <shell>'`, beneath its parked chain (§3.1). That shell's SIGTERM handling is Task 2.1, and the draw's and the waiter's is Task 2.2. The moment as the draw or the waiter starts, before it has installed its catch, is Task 2.4.
>
> With the pane's processes surviving SIGTERM, its session stays up until tmux itself exits, and by then no committer can reach the server (§2.2, §3.1).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §3.1, §3.2, §3.3, §6.2

## killing-all-sessions-wipes-restore-state-2-4

### Task 2.4: A SIGTERM landing as the draw or the waiter starts still leaves the pane waiting

**Problem**: A catch is not inherited. The parked shell's trap is reset to default in the draw it starts, the draw's catch is reset when it execs into the waiter, and the same happens at every redraw, when the waiter execs back into the draw. So each of those processes has a moment, from its start until it has installed its catch, in which a SIGTERM still ends it. The parked shell then runs its recovery tail and clears `@portal-resume-pending` while tmux still answers, and the pane's transcript is lost the way Task 2.2 describes. The moment recurs at restore, at each hand-off from the draw to the waiter, and at every redraw, and §5.2 does not list it as accepted residue.

**Solution**: Close that window, so a SIGTERM landing as the draw or the waiter starts leaves the pane waiting, with its marker set and no recovery tail run. It is closed by a catch, never an ignore, so an answered pane's hook program and the user's shell keep default SIGTERM handling.

**Outcome**: There is no moment while a pane waits in which a SIGTERM ends its draw or its waiter. A pane answered afterwards still gives its hook program and the user's shell default SIGTERM handling.

**Acceptance Criteria**:
- [ ] At restore, a SIGTERM reaches a waiting pane as its parked shell starts the draw, before the draw has installed its catch. The pane is left waiting, with its marker set and no recovery tail run. (§3.2, §6.2)
- [ ] A SIGTERM reaches a waiting pane as its draw execs into the waiter, before the waiter has installed its catch. The pane is left waiting, with its marker set and no recovery tail run. (§3.2, §6.2)
- [ ] A waiting pane is resized, and a SIGTERM reaches it as its waiter execs back into the draw, before the draw has installed its catch. The pane is left waiting, with its marker set and no recovery tail run. (§3.2, §6.2)
- [ ] A waiting pane is answered on its panel after a SIGTERM landed in one of those windows. Its hook program and the user's shell start with SIGTERM at its default disposition, not inherited as ignored. (§3.2)

**Do**:
- Close the window by a catch, never an ignore (§3.2).
- The hand-offs are the parked chain starting the draw (`parkedResumeChain`, `cmd/state_hydrate.go`), and the draw exec'ing into the waiter and the waiter exec'ing back into the draw at a redraw (`resumeHandOff`, `cmd/state_resume_chain.go`, called from `cmd/state_resume_draw.go` and `cmd/state_resume_wait.go`).
- SIGHUP stays uncaught throughout, so a kill or tmux's exit still ends the pane (§3.2, §3.3).

**Context**:
> How the window is closed is the implementer's, within the rule that SIGTERM is caught, never ignored (§3.2). An ignored signal stays ignored across `exec`, so the hook program and the user's shell would inherit it.
>
> The corrigendum to §3.2 records why this is the builder's to close: a SIGTERM landing in this window still leaves the pane waiting, and §5.2 does not list it as accepted residue.
>
> The steady-state catch in the draw and the waiter is Task 2.2, and the parked shell's trap is Task 2.3.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §3.2, §3.3, §5.2, §6.2, Corrigenda

## killing-all-sessions-wipes-restore-state-2-5

### Task 2.5: A kill or tmux's exit still ends a hardened pane at once

**Problem**: Hardening Portal's panes against SIGTERM must not let them outlive a kill. A kill is final (§1.1). tmux ends a killed pane by closing its pty, which delivers SIGHUP, and tmux 3.7c sends no signal to a pane's process itself. A hardened process that also caught SIGHUP would linger past its kill. When tmux itself exits, it closes a waiting pane's pty the same way. A parked chain that survived that would start its recovery tail and try to clear `@portal-resume-pending`.

**Solution**: Coverage that SIGHUP stays uncaught by every hardened process: the eager resume shell, the parked chain, the draw and the waiter. A killed pane, eager or waiting, still dies at once, and the kill path is unchanged. When tmux's exit closes a waiting pane's pty, the parked chain ends on SIGHUP before its recovery tail can start.

**Outcome**: Killing a hardened pane ends it at once and removes it from the saved state as before. tmux's exit ends a waiting pane with no marker clear attempted, so its saved record keeps it waiting.

**Acceptance Criteria**:
- [ ] A restored lazy pane is waiting on its panel, and it is killed. Its parked shell and its waiter end at once, and the kill removes the pane from the saved state as it does today. (§3.3, §6.2)
- [ ] A restored eager resume pane's hook program is still running, and the pane is killed. The pane ends at once, and the kill removes it from the saved state as it does today. (§3.3, §6.2)
- [ ] A restored lazy pane is waiting when its pty closes because tmux has begun exiting. The parked chain ends on SIGHUP before its recovery tail starts: `portal state resume-recover` never runs, and no clear of `@portal-resume-pending` is attempted. (§3.2, §6.2)
- [ ] After that exit, `sessions.json` still names the waiting pane's token-named transcript, and that file is present. (§3.2, §6.2)

**Do**:
- SIGHUP stays uncaught in the eager resume shell (`hookExecArgs`, `cmd/state_resume_chain.go`), the parked chain (`parkedChainTrap`, `cmd/state_hydrate.go`), the draw (`cmd/state_resume_draw.go`) and the waiter (`cmd/state_resume_wait.go`) (§3.3).
- The kill path is unchanged (§1.1, §3.3).

**Context**:
> Measured: `sh -c 'trap : INT QUIT TERM; sleep 3; exit 7'` run as a pty's session leader through `python3`'s `pty.fork()`, master closed after 0.5s → killed by signal 1, SIGHUP (§3.2).
>
> A kill is intentional and names the session: the picker's kill (`k`, then `y`), `tmux kill-session` (including a key the user binds to it), or the user ending the session themselves by exiting its last program. A kill removes the session's record from `sessions.json`, deletes its scrollback files, and (through the hook-staleness sweep) removes its resume hooks (§1.1). The kill path under Phase 1's hardened save path is Task 1.7.
>
> The waiter already leaves SIGHUP at its default disposition, so tmux tearing the pane down ends it (`winchSignals`, `cmd/state_resume_wait.go`).
>
> No commit follows tmux's exit, because once tmux has begun exiting no committer can pass the confirmation (§2.2). That is what keeps the waiting pane's last saved record in place.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.1, §2.2, §3.2, §3.3, §6.2

## killing-all-sessions-wipes-restore-state-2-6

### Task 2.6: Every commit that drops a session logs it by name

**Problem**: Today a wipe is silent at the default log level. The only trace is `capture: tick complete sessions=0` or bare `process: start … state commit-now` lines. `Commit` itself logs nothing below WARN, and only for failed housekeeping (`rg -n 'logger\.(Info|Warn|Debug|Error)' internal/state/commit.go` → 2 hits, both `Warn`). After this fix, some sessions can still close while tmux is answering, such as one whose pane's top program was started directly rather than inside a shell (§5.2). Nothing records which sessions a commit removed, and that record is the evidence the deferred hold (§5.3) waits on.

**Solution**: Every commit that drops a session logs each dropped session by name at INFO, with the name in `session`. "Dropped" is measured against the prior on-disk index, which `Commit` already reads to decide whether anything changed. It is not measured against the daemon's in-memory previous index, which does not see `commit-now`'s writes. A commit that drops nothing logs nothing new.

**Outcome**: At the production default level, the log names every session a commit removed, once each, whichever committer removed it.

**Acceptance Criteria**:
- [ ] `sessions.json` holds several sessions, and a commit's index holds all but two of them. The log holds one INFO line for each of the two, naming it in `session`, and no drop line for any other session. This holds whether the commit is written by the daemon's tick, its shutdown flush or `commit-now`. (§4.1, §4.2, §6.3)
- [ ] A live session is killed, and the `commit-now` its `session-closed` hook runs removes it. That commit logs one drop line naming it. The daemon's next tick, whose in-memory previous index still holds the killed session, commits and logs no drop line. (§4.1, §6.3)
- [ ] A commit's index holds every session `sessions.json` holds, with or without new sessions beside them. No drop line is logged. (§4.1, §6.3)
- [ ] The last user session is killed, and the commit writes an index holding no sessions. One drop line names that session. (§4.1, §5.1)
- [ ] The drop line uses an existing log component and the existing `session` attribute key. No new log component or attribute key is introduced. (§4.2)

**Do**:
- Measure drops against the prior on-disk index that `Commit` already reads to decide whether anything changed (`structuralChange`, `internal/state/commit.go`), never against the daemon's in-memory previous index (§4.1).
- Log each dropped session at INFO, with its name in `session` (§4.2).

**Context**:
> The daemon's in-memory previous index does not see `commit-now`'s writes. Measured against it, a session the user just killed would be logged a second time by the next tick (§4.1).
>
> `session` is an attribute key Portal's closed log vocabulary already defines (`` rg -n '^\| `(session|pane_key|error)` \|' .workflows/portal-observability-layer/specification/portal-observability-layer/specification.md `` → 3 hits), so the line needs no new log component or attribute key (§4.2).
>
> After a reboot, drop lines mean sessions closed while tmux was still answering. A teardown in which everything is hard-killed (SIGKILL) runs no committer and leaves no line at all, so a log with no drop lines does not by itself show how macOS ended things (§4.3). If the log of a reboot taken after a restore on the fixed version shows sessions dropped during shutdown, the deferred hold (§5.3) becomes the follow-up.
>
> A stood-down cycle writes no commit (§2.5), so it drops nothing. A renamed session is not a drop, and telling it apart is Task 2.7.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.5, §4, §4.1, §4.2, §4.3, §5.1, §5.2, §5.3, §6.3

## killing-all-sessions-wipes-restore-state-2-7

### Task 2.7: A renamed session is not logged as dropped

**Problem**: The saved index names a session by its name alone. When a session is renamed, the next commit's index no longer holds its old name, so the drop measurement (Task 2.6) would log it as dropped though nothing was lost. Drop lines are read as evidence that sessions closed while tmux was still answering, so a rename logged as a drop is a false trail.

**Solution**: Tell a rename apart from a drop with an identity the commit can compare, so a renamed session gets no drop line. Which identity is the implementer's.

**Outcome**: Renaming a session logs no drop line. A session that closes, with no new session appearing beside it, is still logged as dropped.

**Acceptance Criteria**:
- [ ] A live session is renamed, and the next commit's index holds it under its new name. No drop line is logged, for its old name or its new one. (§4.1, §6.3)
- [ ] A commit's index lacks one or more of the sessions `sessions.json` holds, and holds no session that `sessions.json` does not. A drop line names each missing session. (§4.1, §4.3)

**Do**:
- Tell renames apart within the drop measurement against the prior on-disk index (`Commit`, `internal/state/commit.go`) (§4.1).

**Context**:
> Which identity tells a rename from a drop is the implementer's. At shutdown no new session appears beside a dropped one, so the drop lines a reboot's log is read for come out the same whichever identity is used (§4.1, Corrigenda).
>
> After a reboot, drop lines mean sessions closed while tmux was still answering (§4.3).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §4.1, §4.3, §6.3, Corrigenda

## killing-all-sessions-wipes-restore-state-2-8

### Task 2.8: A waiting pane outlasts a shutdown SIGTERM and comes back still asking

**Problem**: At a reboot, a waiting lazy pane goes through a whole shutdown sequence, not a single signal. SIGTERM reaches the pane's whole process tree alongside the daemon. The daemon's shutdown flush runs and `_portal-saver` closes, and that close fires a `commit-now`, which passes no scrollback dump. Only later does the server end. If the pane's marker is cleared anywhere in that sequence while tmux still answers, the dump-less commit writes a record naming the pane's vacated positional scrollback path, and its housekeeping pass deletes the token-named transcript. The per-process handling (Tasks 2.2 to 2.5) has to hold across that whole sequence.

**Solution**: End-to-end coverage, against real tmux, of a waiting lazy pane through the shutdown sequence and the next restore.

**Outcome**: A waiting pane is saved still waiting, with its transcript. After the next restore it comes back still asking, which is what lazy resume already does for an unanswered pane.

**Acceptance Criteria**:
- [ ] A restored lazy pane is waiting on its panel, with its transcript at its token-named path. SIGTERM reaches the pane's whole process tree alongside the daemon. The daemon's shutdown flush runs, `_portal-saver` closes, and the `commit-now` its `session-closed` hook runs commits while the server still answers. Throughout, the panel stays up, `@portal-resume-pending` stays set and no recovery tail runs. (§3.2, §6.2)
- [ ] The server is ended after that `commit-now`. Its exit ends the parked chain on SIGHUP before its recovery tail starts. Afterwards `sessions.json` names the pane's session and the pane's token-named transcript, and that transcript is present with the content it had before the shutdown. (§3.2, §6.2)
- [ ] At the next restore, the pane's token-named transcript is still referenced and present, and the pane comes back still asking: its panel is shown and its marker is set. (§3.2, §6.2)

**Context**:
> Without this handling, three things follow once the waiting pane's marker is cleared while tmux still answers. A capture builds the pane a fresh record naming its positional scrollback path, which was renamed away when the pane first went waiting. A commit with no scrollback dump writes that record; the `commit-now` that fires when `_portal-saver` closes after the daemon's flush is one. The housekeeping pass then deletes the token-named transcript. The session survives, but its scrollback is silently lost (§3.2).
>
> No commit follows the server's end, because once tmux has begun exiting no committer can pass the confirmation (§2.2).
>
> This runs a real daemon, its `_portal-saver` close and the `session-closed` hook's `commit-now` against a real tmux server. Under the repository's lane rule (CLAUDE.md) that puts it in the integration lane, with state isolated through `portaltest.IsolateStateForTest`, and it may signal only processes it started itself. Existing real-tmux lazy-resume fixtures live in `internal/restore` (`lazy_resume_*_integration_test.go`) and `internal/restoretest`.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §3.2, §6.2

## killing-all-sessions-wipes-restore-state-2-9

### Task 2.9: Signalling pane programs and the daemon before the server preserves hardened sessions against real tmux

**Problem**: Sessions dying before tmux does is the second root cause, and it belongs to a real shutdown ordering. SIGTERM reaches the programs in panes independently of the server, and a session whose programs exit on it closes while the server still answers. Coverage of each hardened process on its own cannot show that whole sessions survive that ordering on real tmux, alongside the save path Phase 1 hardened. Phase 1 left this ordering to this phase (Task 1.8).

**Solution**: An integration-lane test on real tmux, with an isolated socket, signals the pane programs and the daemon, then the server. The live runtime it runs against holds interactive-shell sessions beside every kind of hardened Portal pane.

**Outcome**: Against real tmux, every session whose panes are interactive shells or Portal's hardened panes keeps its record and its scrollback through that ordering.

**Acceptance Criteria**:
- [ ] A live runtime holds sessions whose panes are interactive shells, restored eager resume panes with their hook programs running, waiting lazy panes, and answered lazy panes with their hook programs running, each with saved scrollback. The pane programs and the daemon are sent SIGTERM, then the server. Afterwards `sessions.json` names every one of those sessions, and every scrollback file it names is present and non-empty. (§3.1, §3.2, §6.5)
- [ ] In that run, each waiting lazy pane's record still names its token-named transcript. (§3.2, §6.5)

**Do**:
- Run in the integration lane, on real tmux with an isolated socket (§6.5).

**Context**:
> A session whose pane runs a program started directly rather than inside a shell is accepted residue. SIGTERM ends it while tmux still answers, and when no pane in its session survives, the session is removed with its scrollback (§5.2). Its survival is not part of this outcome. Also accepted: a reboot in which macOS hard-kills pane programs before tmux, and a user whose interactive shell exits on SIGTERM, as fish does (§5.2).
>
> Interactive shells ignore SIGTERM (§1.2). Portal's hardened panes are the eager resume shell, the lazy parked chain, the panel's draw and waiter, and the hook shell beneath an answered lazy pane (§3.1, §3.2).
>
> The other two orderings in §6.5, the daemon SIGTERMed 10–30ms before the server and `tmux kill-server`, are Task 1.8.
>
> Under the repository's test-isolation rules (CLAUDE.md), the test signals only processes it started itself, on its own tmux socket, with state isolated through `portaltest.IsolateStateForTest`.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §3.1, §3.2, §5.2, §6.5
