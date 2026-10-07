TASK: Every Commit That Drops A Session Logs It By Name (killing-all-sessions-wipes-restore-state-2-6, tick-971fca)

ACCEPTANCE CRITERIA:
- `sessions.json` holds several sessions, and a commit's index holds all but two of them. The log holds one INFO line for each of the two, naming it in `session`, and no drop line for any other session. This holds whether the commit is written by the daemon's tick, its shutdown flush or `commit-now`.
- A live session is killed, and the `commit-now` its `session-closed` hook runs removes it. That commit logs one drop line naming it. The daemon's next tick, whose in-memory previous index still holds the killed session, commits and logs no drop line.
- A commit's index holds every session `sessions.json` holds, with or without new sessions beside them. No drop line is logged.
- The last user session is killed, and the commit writes an index holding no sessions. One drop line names that session.
- The drop line uses an existing log component and the existing `session` attribute key. No new log component or attribute key is introduced.

STATUS: complete

SPEC CONTEXT: §4 closes the gap where a wipe was silent at the default level. §4.1: every commit that drops a session logs each by name at INFO, measured against the prior on-disk index `Commit` already reads for its change check, never the daemon's in-memory previous index (which does not see commit-now's writes and would double-log a kill). A commit that drops nothing logs nothing new; a rename is not a drop (identity is the implementer's; Task 2.7). §4.2: the name goes in the existing `session` key under an existing component. §2.5: a stood-down cycle writes no commit, so drops nothing. §4.3/§5.3: these lines are the evidence the deferred hold waits on.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit.go:35-61 — `commitOver` takes the on-disk `prior`; drop logging runs at :52-54 only after the atomic write at :48 succeeds and only when a prior index was read.
  - internal/state/commit.go:85-108 — `logDroppedSessions`: each prior session absent from idx by name (and not matched as a rename) logs `logger.Info("session dropped", "session", s.Name)` at :106.
  - internal/state/commit_cycle.go:121 — `committed` is read from sessions.json under the commit lock; :150 hands `committed` (not the LoadPrev fallback `prev`) to `commitOver`, so the measure is always the on-disk index.
  - Every production committer reaches the commit only through `RunCommitCycle` with `daemonLogger` (`log.For("daemon")`, cmd/state_common.go:8): daemon tick and shutdown flush via cmd/state_daemon.go:267-275 with `deps.Logger` set from `logger := daemonLogger` (:415, :450); commit-now via cmd/state_commit_now.go:91, :109-116. The only other sessions.json writer, `RecordRestoredPaneTokens` (internal/state/restored_pane_tokens.go:28-57), only adds tokens and drops nothing.
- Notes: Matches the spec exactly. Drops are measured against the on-disk index read under the commit lock, so a session commit-now removed is absent from the next tick's measure and is never reported twice. A failed write logs no drop. When sessions.json is absent there is nothing to drop; when it cannot be read or decoded, no drop is measured and the cycle's existing WARN names why (commit_cycle.go:156-163). That is consistent with "never measure against the in-memory index". No new component and no new attribute key: `daemon` and `session` both already exist in the closed vocabulary.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: internal/state/commit_drop_log_test.go:75-87 (four saved, two kept, exactly bravo+delta logged at INFO); cmd/state_commit_drop_log_test.go:86-138 runs the same scenario through each of the three committers (tick, `defaultShutdownFlush`, real `state commit-now` with `state.RunCommitCycle` wired in).
  - Criterion 2: cmd/state_commit_drop_log_test.go:140-168. commit-now drops `killed` once. Then a tick whose `PrevIndex` still holds `killed` completes its cycle (`PrevIndex` replaced, :162) and logs no drop. This test would catch the wrong design of measuring against the in-memory index.
  - Criterion 3: internal/state/commit_drop_log_test.go:89-110 forces a write (`anyScrollbackChanged=true`) so the no-drop assertion is not vacuous, covering same, superset and reordered; cmd/state_commit_drop_log_test.go:170-194 covers the same through the tick.
  - Criterion 4: internal/state/commit_drop_log_test.go:112-127 and cmd/state_commit_drop_log_test.go:196-213 (commit-now with zero live sessions writes an empty index and logs `last`).
  - Criterion 5: `droppedSessionsLogged` (cmd/state_commit_drop_log_test.go:70-84) asserts component `daemon`, level INFO and the `session` attr. For commit-now it goes through `logtest.Install`, so it checks the real production binding.
  - Extra edges: no drop on the first write (:129-140) and no drop when the write fails (:142-155).
- Notes: The tick and flush subtests supply their own `daemon`-labelled logger, so their component check proves nothing on its own. The commit-now subtest does exercise the real binding, and production wires the same `daemonLogger` into all three, so nothing is left unchecked. The rename tests in the same file belong to Task 2.7 and were not judged here.

CODE QUALITY:
- Project conventions: Followed. The component logger is passed down rather than bound in `internal/state`, and the attr key and component come from the existing closed vocabulary.
- SOLID principles: Good. Drop measurement is one small function called from the single commit chokepoint.
- Complexity: Low
- Modern idioms: Yes (`slices.IndexFunc`/`slices.Delete`/`slices.EqualFunc`, map-set lookups)
- Readability: Good. The doc comments on `Commit` (commit.go:24-27) and `commitOver` (:33-34) match the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
