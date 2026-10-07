TASK: Shutdown Orderings Against Real Tmux Preserve The Full Saved State (killing-all-sessions-wipes-restore-state-1-8, tick-3089e7)

ACCEPTANCE CRITERIA:
- A live runtime holds saved sessions and their scrollback. The daemon is SIGTERMed, and the tmux server is SIGTERMed 10–30ms later. Across many trials, every trial ends with `sessions.json` naming every session and every scrollback file present and non-empty.
- A live runtime holds saved sessions and their scrollback, and `tmux kill-server` is run. Afterwards `sessions.json` and every scrollback file are unchanged.
- `tmux kill-server` is run while the daemon has a save in flight, repeated across trials so that the exit lands at different points in the save. Every trial leaves `sessions.json` and every scrollback file unchanged.
- Every trial runs at the production default log level, with nothing wrapping `tmux`.

STATUS: complete

SPEC CONTEXT: Section 6.5 requires integration-lane trials on real tmux with an isolated socket, covering the orderings that wiped the state before the fix: the daemon SIGTERMed 10–30ms before the server (the sandbox wiped the whole state at the default log level and kept it under debug logging, so the trials must run at the default level with nothing wrapping tmux), and `tmux kill-server`. Section 2.2 is what makes kill-server safe: no committer passes the own-server confirmation once the exit has begun, whichever read the exit lands after. Section 1.2 explains why kill-server survived before the fix only because the first read (the restoring marker) happened to fail, which left a save already past that read exposed. The third 6.5 ordering (pane programs signalled first) belongs to Phase 2 and is out of this task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_shutdown_orderings_integration_test.go:1 — `//go:build integration`
  - cmd/state_shutdown_orderings_integration_test.go:59-74 — TestShutdown_DaemonSIGTERMedBeforeServerPreservesFullState: one fresh runtime per delay. The daemon is SIGTERMed, the test sleeps the delay, then the server pid is SIGTERMed. The delays are 10, 12, …, 30ms (daemonLeadDelays, :118-124), giving 11 trials.
  - cmd/state_shutdown_orderings_integration_test.go:76-84 — TestShutdown_KillServerLeavesSavedStateUnchanged
  - cmd/state_shutdown_orderings_integration_test.go:86-114 — TestShutdown_KillServerDuringSaveLeavesSavedStateUnchanged: 24 trials. Each one calibrates a requested save's start and `took` from the daemon's `capture: tick complete` summary (:368-385). It then requests a save ahead of the next tick (:427-438) and runs kill-server at an offset spread from -took/4 to 1.25·took (saveOffset, :128-131). Each landing is classified from the log and the save.requested file (:457-481). The test fails at :111-113 if no trial landed while the save was in flight, so it cannot pass without testing anything.
  - cmd/state_shutdown_orderings_integration_test.go:133-169 — stageShutdownBinary / requireUnwrappedTmux: fails if the `tmux` on PATH, with symlinks resolved, is a `#!` script.
  - cmd/state_shutdown_orderings_integration_test.go:183-211 — newShutdownRuntime: unsets PORTAL_LOG_LEVEL before the server starts (:188-191). It reuses the kill-path fixture (cmd/state_kill_path_integration_test.go:100-136: IsolateStateForTest, RegisterStateDirTeardownGuard and a `ptl-killpath-` tmuxtest socket, with bootstrap through `portal list`). It waits for the first dump with non-empty scrollback, then reads the daemon's own `process: log-level resolved` line by pid and requires `resolved=info` and `source=default` (:215-234). Every trial goes through this path.
  - cmd/state_shutdown_orderings_integration_test.go:270-302 — awaitShutdown: waits until both pids return ESRCH, then until the state directory has held still for 500ms, so that straggling session-closed commit-now runs land before the assertions.
  - cmd/state_shutdown_orderings_integration_test.go:315-337 — assertFullStatePreserved: requires the session set to equal the three sessions exactly. Every scrollback file named afterwards, and every file named before, must be present and non-empty.
  - cmd/state_shutdown_orderings_integration_test.go:339-359 — assertSavedStateUnchanged: compares sessions.json and every saved scrollback file byte for byte.
- Notes:
  - The byte-identity assertion is sound for trials whose save completed before the kill. Commit writes nothing when nothing changed structurally and no scrollback changed (internal/state/commit.go:44-46), and the fixture's panes are static, so a no-op save leaves sessions.json byte-identical.
  - The landing classifier is conservative. A save consumes save.requested only once its cycle starts (cmd/state_daemon.go:199). A failed cycle re-touches the file but logs a `tick …` WARN (:204-205). A misclassification can only move a trial toward "not shown", which makes the in-flight guard fail rather than pass falsely.
  - Every signal is sent to the fixture's own processes only: the saver pane pid and the server pid read from the test socket. kill-server goes to the isolated socket. The no-touching-the-real-system rule is respected.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: 11 trials across 10–30ms, with the full-state assertion matching the criterion's wording.
  - AC2: a single kill-server on an idle runtime, with byte-identity.
  - AC3: 24 trials spread across the save, with byte-identity on each and an explicit check that at least one landed in flight.
  - AC4: checked on every trial through the daemon's own log-level resolution line, plus the unwrapped-tmux check before any trial runs.
  - Would fail if the feature broke: yes. A flush or tick that committed the shutdown's empty listing would change sessions.json and delete scrollback. A dump that wrote an empty capture would zero a file. All three assertions catch both.
- Notes: Not over-tested. Every runtime is needed: each trial needs a fresh server, because a kill or SIGTERM ends it.

CODE QUALITY:
- Project conventions: Followed. Integration tag, IsolateStateForTest through the reused fixture, RegisterStateDirTeardownGuard, portalbintest staging, a tmuxtest isolated socket, no t.Parallel, and no signal sent to a process the test did not start.
- SOLID principles: Good
- Complexity: Acceptable
- Modern idioms: Yes (slices/maps iterators, strings.FieldsSeq, range-over-int)
- Readability: Good. Comments hold against the code and reference no process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Across many trials, every trial ends with `sessions.json` naming every session and every scrollback file present and non-empty." — To settle: run `go test -tags integration -p 1 ./cmd -run TestShutdown_DaemonSIGTERMedBeforeServerPreservesFullState`, repeated, and confirm that at these leads the server's exit actually lands inside the daemon's shutdown flush on this fixture. That second check would need the flush's `took` or a run against the pre-fix code that reproduces the wipe.
- "`tmux kill-server` is run. Afterwards `sessions.json` and every scrollback file are unchanged." — To settle: run `TestShutdown_KillServerLeavesSavedStateUnchanged` in the integration lane.
- "`tmux kill-server` is run while the daemon has a save in flight, repeated across trials so that the exit lands at different points in the save. Every trial leaves `sessions.json` and every scrollback file unchanged." — To settle: run `TestShutdown_KillServerDuringSaveLeavesSavedStateUnchanged` repeatedly. Check the logged landings: at least one should be in flight, and they should spread across the save rather than cluster. Also check that the at-least-one-in-flight check stays stable across runs.
