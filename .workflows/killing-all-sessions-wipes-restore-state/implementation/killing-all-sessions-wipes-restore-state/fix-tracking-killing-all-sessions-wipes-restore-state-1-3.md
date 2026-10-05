## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/cmd/state_commit_confirmation_test.go:75 — `TestCommitNowFailsOnARefusedConfirmation` no longer sends a confirmation.
  - Why: it sets no `TMUX`, so commit-now inherits the package-wide poison `TMUX=/nonexistent/portal-test-must-set-tmux-socket,0,0` (/Users/leeovery/Code/portal/cmd/testmain_isolation_test.go:78). `ownTmuxServer()` returns 0, so `confirmOwnServer` (/Users/leeovery/Code/portal/internal/state/scrollback.go:279) returns `ErrNotOwnServer: own server unknown` before `ConfirmAnswering` is ever called.
  - Effect: the test's only assertion, `errors.Is(err, errCommitNowFailed)`, is satisfied by that stand-down. The refused `display-message` the test is named for is never sent.
  - Nothing else covers it: its sibling `TestCommittersStandDownOnARefusedConfirmation/commit-now` (line 59) discards commit-now's error. So no test now checks that commit-now exits non-zero on a refused confirmation. A regression that let commit-now exit 0 on a refused confirmation (no `save.requested` touch, so the retry is lost) would pass the whole suite.
  - Cause: this task's change. The executor added `withOwnTmuxServer` to the other commit-now fixtures but missed this one.
  FIX:
  - At the top of the test, add `withOwnTmuxServer(t, fakeOwnServerPID)`.
  - Hold the commander in a variable: `fc := workOnlyCommander(refusedConfirmation())`, then `client := tmux.NewClient(fc)`.
  - Beside the existing `errCommitNowFailed` check, assert `len(fc.callsContaining("display-message")) > 0`. That is the shape `TestCommittersStandDownOnARefusedConfirmation` uses at lines 62-69.
  - `failCommitNow` puts the cause into the error text rather than wrapping it. So `strings.Contains(err.Error(), "confirm tmux answering")` also pins that the refusal, not an unknown own server, ended the run.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:25 — the new const was inserted between `daemonFakeCommander`'s doc comment and its type. The whole block now documents `fakeOwnServerPID`, and `daemonFakeCommander` has no doc. Separately, "Unset commands return ("", nil)" is no longer true of the confirmation read, which now answers with the own-server pid.
  OLD: // daemonFakeCommander is kept bespoke rather than retired onto
// commandertest.Scripted: it is not an argv-pattern script but a model of the
// daemon's whole tmux surface, and it carries one behaviour a static script
// cannot express — dispatchHook, a callback that fires after every dispatch,
// matched or not, so a caller can cancel the context while a tmux subcall is
// in flight. The mutex covers the concurrent drive from the goroutines
// running defaultDaemonRun. Its Run/RunRaw still route through
// commandertest.Trim/Verbatim, so the trim-versus-verbatim contract keeps its
// single implementation.
//
// Unset commands return ("", nil), so unrelated tmux calls do not fail a test.
// fakeOwnServerPID is the pid of the tmux server a faked committer belongs to.
const fakeOwnServerPID = 4242

type daemonFakeCommander struct {
  NEW: // fakeOwnServerPID is the pid of the tmux server a faked committer belongs to.
const fakeOwnServerPID = 4242

// daemonFakeCommander is kept bespoke rather than retired onto
// commandertest.Scripted: it is not an argv-pattern script but a model of the
// daemon's whole tmux surface, and it carries one behaviour a static script
// cannot express — dispatchHook, a callback that fires after every dispatch,
// matched or not, so a caller can cancel the context while a tmux subcall is
// in flight. The mutex covers the concurrent drive from the goroutines
// running defaultDaemonRun. Its Run/RunRaw still route through
// commandertest.Trim/Verbatim, so the trim-versus-verbatim contract keeps its
// single implementation.
//
// Unset commands other than the confirmation read return ("", nil), so
// unrelated tmux calls do not fail a test.
type daemonFakeCommander struct {
- /Users/leeovery/Code/portal/internal/restore/lazy_resume_panel_integration_test.go:366 (correction 1 of 2) — the new `serverPID` method was inserted under `lazyPanelFixture`'s doc comment, which now documents the method. This correction removes the type's doc from above the method.
  OLD: // lazyPanelFixture is the two-session install the suite reboots: a lazy subject
// beside the plain pane a user works in, and an eager control proving both modes
// ship live together.
// serverPID is the pid of the server the fixture's socket answers on now: the
  NEW: // serverPID is the pid of the server the fixture's socket answers on now: the
- /Users/leeovery/Code/portal/internal/restore/lazy_resume_panel_integration_test.go:380 (correction 2 of 2) — puts the type's doc back above the type.
  OLD: type lazyPanelFixture struct {
  NEW: // lazyPanelFixture is the two-session install the suite reboots: a lazy subject
// beside the plain pane a user works in, and an eager control proving both modes
// ship live together.
type lazyPanelFixture struct {
- /Users/leeovery/Code/portal/internal/state/scrollback.go:268 — says a committer that does not know its own server "sent" a confirmation. `confirmOwnServer` refuses that case before sending anything.
  OLD: // ErrNotOwnServer is a confirmation that does not prove the committer's own
// tmux server answered it: one answered by another server, one naming no
// server, or one sent by a committer that does not know its own server.
  NEW: // ErrNotOwnServer is a confirmation that does not prove the committer's own
// tmux server answered it: one answered by another server or naming none. A
// committer that does not know its own server sends none and is refused with it.
- /Users/leeovery/Code/portal/internal/state/scrollback.go:273 — "sends the confirmation" is false when `ownServer` is unknown.
  OLD: // confirmOwnServer sends the confirmation and refuses it unless ownServer
// answered it. A tmux server that has begun exiting refuses every new
  NEW: // confirmOwnServer refuses the cycle unless ownServer answers the
// confirmation. A tmux server that has begun exiting refuses every new

NOTES:
- **Needs your call: spec and code now disagree on an empty answer.** The spec says an exit-0 answer with no output counts as answered (§2.2) and is "safe" (§6.1). The code stands the cycle down on it. No reply-based check can tell which server sent an empty answer, so I accept the change: the save is lost only at the instant the own server starts exiting, and the previous save stays. Since the spec never made this decision, a corrigendum to §2.2/§6.1 would keep the two consistent. Task 1.2's test that asserted a commit was flipped to `TestRunCommitCycleStandsDownOnAConfirmationNamingNoServer`.
- **I checked the mechanism on disposable `-L ptl-review-*` sockets.** A `session-closed` hook's `run-shell` job got `TMUX=<socket>,<server pid>,0`, and a pane process in `_portal-saver` got `TMUX=<socket>,<server pid>,1`. In both, the pid matched `display-message -p '#{pid}'`. Both sockets were killed afterwards and no `ptl-` servers were left.
- **Checking only the confirmation is sound.** By tmux's two properties, an answer from the own server proves every earlier read reached it. Only exotic cases fall outside: the socket file deleted and recreated mid-capture, or a new server reusing the exited server's pid.
- **What I ran:** `go build ./...` clean; `go vet` on both lanes clean; `golangci-lint` on cmd, internal/state, internal/tmux and internal/restore: 0 issues; `go test` on cmd, internal/state and internal/tmux: pass. Repeats: the new real-tmux tests passed 15 times in a row, and `TestConfirmAnswering_RealTmux` 30 times, with load around 10. Kill-server followed straight by new-session did not flake.
- **Not reproduced: the four `cmd/bootstrap` composite integration failures.** The executor confirmed one fails on an untouched HEAD export. All four fail at the same harness setup step ("saver daemon did not come up"), and nothing in this diff touches saver readiness.
- **The two self-eject integration tests still pinned to `TMUX=<sock>,1,0`** (/Users/leeovery/Code/portal/cmd/state_daemon_self_supervision_integration_test.go:73 and :430) start no tmux server. Their daemons never committed either way, so no check in them is weakened.
- **Task 1.5 will need the own-server check from `cmd`.** The dump lives in `cmd/state_daemon.go`, but `confirmOwnServer` is unexported in `internal/state`, so that task will need to export it or add an entry point in `state`.
- **Unknown own server is detected only after the capture.** A committer with no `TMUX` takes the lock and runs a full capture before standing down. That only happens off the production paths, so it is harmless.
- **Confusing error text for an empty answer.** The stand-down reads "answered by server pid 0, own server pid N". Task 1.4's reporting may want wording such as "named no server".
- **Nothing extra arrived with the dispatch** beyond the listed inputs.
