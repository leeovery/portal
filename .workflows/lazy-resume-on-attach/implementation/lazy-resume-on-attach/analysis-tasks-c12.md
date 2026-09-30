# Analysis Tasks: Lazy Resume On Attach (Cycle 12)

## Task 1: A Waiting Pane's Transcript Survives a Capture That Misses Its Session
severity: low
sources: standards

**Problem**: `CaptureStructure` (`internal/state/capture.go:55-96`) runs its reads in sequence:
1. It reads the session names.
2. It reads the whole-server pane enumeration, filtered to those names (`parsePaneRows`, `:338-358`).
3. It calls `ShowEnvironment` once per kept session. On a 44-session install that loop takes roughly 100–200 ms.

A session renamed inside that window drops out of the capture, whether the rename comes from the picker's `r` or from `tmux rename-session`:
- If the rename lands before the pane read, the session's rows carry the new name and are filtered out.
- If it lands after, `ShowEnvironment` on the old name answers no-such-session, and the session is skipped as vanished.

The capture still succeeds because other sessions captured. `Commit`'s housekeeping pass (`internal/state/commit.go:39`, `:77-111`) then deletes every `.bin` the index does not name. That includes a waiting pane's `scrollback/pane-<token>.bin`.

On the next capture the session is back under its new name. The previous index is the one just committed: the daemon keeps it in memory (`cmd/state_daemon.go:272`), and `commit-now` reads it from `sessions.json` under the commit lock (`cmd/state_commit_now.go:117-120`). So the token merge (`mergeFrozenPanes`, `internal/state/capture.go:183-206`) finds no record. The re-file treats the missing positional source as adoption (`placeStoredScrollback`, `internal/state/scrollback.go:143-152`) and points the record at the file that was just deleted. The pane is frozen, so nothing re-captures it.

Before this feature, a skipped session cost one tick, because every unfrozen pane is re-captured from the live pane on the next tick. A waiting pane has no second source, so the loss is permanent:
- At the next reboot the pane comes back with its panel over an empty pane, and `scrollback file not found` appears in portal.log.
- If the pane is answered before any reboot, it is re-captured from the live pane and nothing is lost.

The specification holds that a waiting pane keeps its place in the saved set throughout, and is restored with its original content however many reboots it waits through. It names a session rename among the rearrangements the freeze survives. Both committers run this capture: the daemon's tick and `commit-now`.

**Solution**: The committed index keeps naming a live waiting pane's transcript even when the pane's session did not reach the capture.
- **Reading waiting panes.** The capture reads the pending marker and the token from every non-internal row of its one enumeration. That includes rows under a name the session-name read did not return, and rows of a session the per-session read skipped.
- **Carrying the record.** Take each live waiting pane whose token no fresh record carries but a previous record does. The capture carries the previous index's session that holds that record forward whole, under its previous name, into the index it returns.
- **Recovery.** The next capture finds the pane under its new name and takes its record by token, as it does today. For the daemon, the previous index is the one just committed; `commit-now` reads it under the lock. The carried session then drops out on its own once no live waiting pane needs it.
- **Limits.** A carried record never puts a token on a second record (ad-hoc task 1's one-token-one-record rule). A session is never carried over a live session that reached the capture under the same name. In that case the cycle commits nothing and retries through the caller's existing failure route, because that state resolves on the next capture.
- **A session killed mid-capture** is carried for that one cycle and dropped by the next. The session-closed `commit-now` runs that next cycle straight after.
- **Where it lives.** The rule goes in the shared capture, so the daemon's tick, its shutdown flush and `commit-now` all take it.

Why this direction:
- The housekeeping pass keeps deleting every `.bin` its own index does not name. Cycle 6 rejected having it spare token-named files, and carrying the record leaves that rule intact.
- A record is carried only for a pane the same enumeration shows live, which keeps the specification's guard against resurrecting a gone pane.
- Refusing the commit outright whenever a live waiting pane misses the index would turn the per-session skip the tree tolerates into a stalled saver for every session. That skip covers one session failing its environment read.

**Outcome**: After every commit, a waiting pane's token-named transcript is on disk and `sessions.json` names it. That holds even when a capture was taken while the pane's session was being renamed, or while its environment read failed. The pane restores with that transcript at the next reboot.

**Acceptance Criteria**:
In each scenario, session `foo` holds waiting pane X, which carries token T. The previous index names X's record in `foo` at `scrollback/pane-T.bin`, and that file is on disk.
- [ ] A committing cycle's capture misses `foo` in one of three ways:
  - (a) The session-name read returns `foo`, but the pane enumeration lists X, still marked pending, under `bar`. `foo`'s environment read answers no-such-session.
  - (b) The enumeration lists X under `foo`, and `foo`'s environment read answers no-such-session.
  - (c) Another session captures normally, and `foo`'s environment read fails with some other error.

  After the commit, `pane-T.bin` is on disk and holds the bytes it held before. `sessions.json` holds `foo` as the previous index had it, with X's record naming `pane-T.bin`. A daemon tick and `portal state commit-now` both leave this state.
- [ ] Continuing from any of those, the next committing cycle reaches X's session: as `bar` after (a) or (b), as `foo` after (c). It commits X's record at its live address naming `pane-T.bin`, and the file is still on disk holding the same bytes. After (a) or (b), `foo` is no longer in `sessions.json`.
- [ ] A daemon tick that carries `foo` after (c) captures no scrollback from X and writes no scrollback file for it. The freeze holds for X as it does for any waiting pane.
- [ ] `foo` is killed after the pane enumeration listed X. That cycle commits `foo` carried forward, as in the first scenario. The next committing cycle, whose enumeration no longer lists X, commits an index without `foo`.
- [ ] Nothing is carried for a pane that the same enumeration does not list with the pending marker. A session killed before the pane enumeration, and a session missing from the capture that holds no waiting pane, are both left out of the committed index, as they are today.
- [ ] When a carried session holds a record whose token a pane in the fresh capture also carries, the committed index holds that token on one record only.
- [ ] A carry can land on the name of a session that reached the capture. For example, within one capture `foo` is renamed to `bar` and another session is renamed to `foo`. In that case the cycle commits nothing, and `sessions.json` and the scrollback directory are unchanged:
  - The daemon's tick logs its existing `tick failed` WARN and re-touches `save.requested`.
  - `commit-now` exits non-zero through its existing failure route, touching `save.requested`.

  The next committing cycle in which X's session reaches the capture commits X's record naming `pane-T.bin`.
- [ ] Where no waiting pane's session misses the capture, every committing cycle commits what it commits today.

**Do**:
- **Where it lives.** The rule goes in the shared capture in `internal/state` that every committing cycle runs, so the daemon's tick, its shutdown flush and `commit-now` all take it. The path is `RunCommitCycle` (`internal/state/commit_cycle.go:51-72`) → `captureAndRefile` (`internal/state/scrollback.go:275-288`) → `CaptureStructure` (`internal/state/capture.go:49-118`).
- **Reading waiting panes.** The pending marker and the token come from the capture's one `ListAllPanesWithFormat` enumeration, whose `captureFormat` already carries both columns. Every non-internal row counts:
  - rows `parsePaneRows` (`:338-358`) drops today because their session name is not in the session-name read;
  - rows of a session the `ShowEnvironment` loop (`:78-96`) skips.

  There is no second tmux read.
- **Carrying.** Take each live waiting pane whose token no fresh record carries but a previous record does. The previous index's session that holds that record goes into the returned index whole, under its previous name.
- **Limits.**
  - A carried record never puts a token on a second record.
  - A carry that would land on the name of a session that reached the capture fails the capture. `RunCommitCycle` then commits nothing, and the callers' existing failure routes handle it: the tick's `tick failed` WARN and `save.requested` re-touch (`cmd/state_daemon.go:200-206`), and `failCommitNow` (`cmd/state_commit_now.go:141-147`).
  - There is no new retry path.
- **Recovery.** Recovery rides the existing token merge (`mergeFrozenPanes`, `internal/state/capture.go:183-206`) against the previous index: the daemon's in-memory one, or the one `commit-now` reads under the lock. A carried session drops out once no live waiting pane needs it.
- **Unchanged.** `gcOrphanScrollback` (`internal/state/commit.go:77-112`) keeps deleting every `.bin` its own index does not name, token-named files included.
- This is a behaviour change, so the executor writes the tests that pin it.

## Task 2: A Waiting Pane Carries the Token That Protects Its Registration
severity: low
sources: architecture

**Problem**: Restore re-stamps each pane's saved token best-effort (`restampPaneToken`, `internal/restore/session.go:156-168`). If the `set-option -p` fails, restore logs `set pane token failed` and carries on. The hydrate helper still finds the registration under its baked key, resolves it lazy and parks the pane. The step that parks it, `markResumePending` (`cmd/state_hydrate.go:381-411`), resolves the pane and executable, pins the alternate screen and writes the pending marker. It never writes the token.

The sequence that follows:
1. About ten seconds after the daemon starts, its throttled hook sweep (`cmd/state_daemon.go:106`, `:212-223`) enumerates the live tokens.
2. The baked key is token-shaped and no live pane carries it, so `StaleKeys` (`internal/hooks/store.go:336-351`) calls it stale and the sweep deletes the registration.
3. The panel keeps showing the command, because the command is carried in the chain's argv.
4. When the user presses Enter, the waiter re-reads the store (`resumeAnswerEnter`, `cmd/state_resume_wait.go:302-313`), finds nothing and hands them a plain shell.

The resume never runs. The command survives only as the sweep's `clean-stale` INFO line in portal.log.

Before this feature the eager hook had already fired from the baked key, so a failed re-stamp cost only the next reboot's hook. Under lazy, which is the shipped default, it costs the current wait. The capture's token merge, the re-file, the preview and the sweep's protection all read the same token, and nothing on the waiting path makes sure the pane carries it.

**Solution**: The step that decides a pane waits also sets the token that protects its registration.
- **The write.** `markResumePending` writes the baked hook key onto the pane as `state.PortalPaneIDOption`. It uses the same unconditional `SetPaneOption`, with no read first, that it uses for the alternate-screen pin.
- **Order.** After the pane and executable resolve, the writes run token, then pin, then marker.
- **Refusal.** A refused token write is handled like a refused pin: the pane does not wait, its hook fires eagerly, and the existing `set resume pending marker failed` WARN names the error.
- **Leftovers.** A token left behind by a later refusal is the pane's own saved identity, so it needs no lift.

Why this direction:
- Review cycle 2 settled that the mark step's protective writes are unconditional, and that a pane Portal cannot protect falls back to eager.
- The baked key is the pane's saved token. Where restore's re-stamp landed, the write sets the same value and changes nothing. Where it failed, the write sets the value restore was meant to set. Either way the pane's durable token never changes.
- The write reads nothing, so the firing path still never reads the live token.

**Outcome**: Every pane the helper parks carries its registration's token for the whole wait, whether or not restore's re-stamp landed. The stale-hook sweep therefore keeps the registration, and Enter on the panel runs the registered command. A pane whose token cannot be written does not wait: its hook fires eagerly, as it does for a pane whose pin or marker is refused.

**Acceptance Criteria**:
In each scenario, pane X was restored with a registration under its saved token T, and that registration resolves lazy.
- [ ] Restore's re-stamp of T failed, so X carries no `@portal-pane-id`. When the helper parks X, X carries T as its `@portal-pane-id`.
- [ ] Continuing, the daemon's stale-hook sweep runs while X waits. X's registration is still in `hooks.json` afterwards, so Enter on X's panel runs the registered command.
- [ ] Restore's re-stamp landed. Parking X leaves its `@portal-pane-id` reading T.
- [ ] On each of the helper's three tails (replay, signal timeout, missing scrollback file), the writes run in this order, all before the mid-restore marker is cleared:
  1. the token
  2. the alternate-screen pin
  3. the pending marker
- [ ] The helper never reads a pane's `@portal-pane-id` on any path. The token is written without a read first.
- [ ] The token write is refused. X does not wait:
  - no pin and no pending marker are written;
  - the registered command runs as an eager hook (`sh -c '<command>; exec $SHELL'`);
  - exactly one `set resume pending marker failed` WARN names the pane and the token write's error.
- [ ] The pin or the marker is refused after the token write landed. X fires eagerly, as it does today, and no write removes T from X. A refused marker still lifts the pin, as it does today.
- [ ] No token is written in these cases:
  - `$TMUX_PANE` is absent, or the executable cannot be resolved (no pin or marker is written either);
  - the pane's registration resolves eager;
  - the pane has no registration.

**Do**:
- **The write.** In `markResumePending` (`cmd/state_hydrate.go:388-411`), write `cfg.HookKey` onto the resolved pane as `state.PortalPaneIDOption` through `cfg.Client.SetPaneOption`. That is the same unconditional pane-option write the alternate-screen pin uses, with no read of the pane's token first.
- **Order.** After `requireTmuxPane` and the executable resolve, the writes run token, then pin, then marker.
- **Refusal.** `markResumePending` returns a refused token write the way it returns a refused pin. `markPendingThenUnsetSkeletonMarker`'s existing `set resume pending marker failed` WARN (`cmd/state_hydrate.go:367-379`) then names its error, and the pane takes the eager hand-off. There is no new log event.
- **No lift.** A pin or marker refused after the token landed leaves the token on the pane.
- **Unchanged.** Restore's best-effort re-stamp (`restampPaneToken`, `internal/restore/session.go:157-169`).
- This is a behaviour change, so the executor writes the tests that pin it. The refused-token case sits beside the existing mark-step refusals in `TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked` (`cmd/state_hydrate_lazy_test.go:280-334`).
