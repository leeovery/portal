# Implementation Review: Killing All Sessions Wipes Restore State

**Plan**: killing-all-sessions-wipes-restore-state
**Verdict**: Fail

## Summary

All 37 tasks across the eight phases are delivered, and no task carries a blocking issue. The three parts of the fix hold against the specification, by reading and by measurement on real tmux:
- the save path stands down on a failed listing or an unconfirmed view;
- Portal's resume-hook panes outlast the shutdown SIGTERM;
- every dropped session and backed-off save is logged.

The shutdown orderings that wiped the saved state before the fix now preserve it. All 33 daemon-before-server trials passed. In the 11 trials whose logs were observed, the server's exit landed inside the final flush. 48 kill-server-during-save trials and the kill-path regression suite also pass, with each kill committed by the real `session-closed` hook. Linters are clean in both lanes, and the unit lane is green over the corrected tree.

The review fails on one replan item. In the first daemon cycle after a restore that moved a tokened pane off its saved address (a session restored with a window-index gap), the dump can overwrite the positional scrollback name that pane's committed record still names with the capture of the pane now at that address. If that cycle then ends uncommitted (a cancelled tick followed by a stood-down flush, or a failed `sessions.json` write), the moved pane's record names a file holding another pane's transcript. Closing it is a design decision with three defensible forms.

Two contained corrections were applied and committed in this session:
- a false field comment;
- a unit-lane real-tmux test that wrote to the developer's real `~/.zsh_history` on every run.

One pre-existing order-dependent integration test failure is held as out of scope.

## QA Verification

### Specification Compliance

#### 1. Problem and Governing Rule + 2. The Save Path Stops Trusting an Unconfirmed Session List

- The commit cycle reads the session list through a listing that returns a failed `list-sessions` as an error, and the cycle stands down on it rather than reading zero sessions — internal/state/capture.go:95-103, internal/tmux/tmux.go:164-170 and :230-232 — read; run: `go test -count=1 -v ./internal/state -run TestRunCommitCycleStandsDownOnAFailedSessionListing` and `go test -count=1 -v ./cmd -run TestCommittersStandDownOnAFailedSessionListing` → PASS
- The shared listing still swallows a failed read for the picker, resolver, completion and restore. `ListSessions` is unchanged, and restore's `snapshotLiveSessions` still reads the swallowing `ListSessionNames` — internal/tmux/tmux.go:151-158, internal/restore/restore.go:114-124 — measured: `rg -n 'Swallowed deliberately' internal/tmux/tmux.go` → 1 hit (:154); `rg -n 'ListSessionNamesProbe|ListSessionsProbe' --type go -g '!*_test.go' .` → production callers are internal/state/capture.go and cmd/open_search.go only; run: TestOrchestrator_RebuildsEverySavedSessionWhenListSessionsFails → PASS (in the `go test -tags integration -p 1 -count=1 -v ./internal/restore` run)
- A failed `list-panes` stands the cycle down too, and a listing tmux answered but Portal could not parse is classified by the confirmation rather than read as refused — internal/state/capture.go:108-117, internal/tmux/tmux.go:172-200, internal/tmuxerr/errors.go:22 — read; run: `go test -count=1 -v ./internal/state -run 'TestRunCommitCycle'` → TestRunCommitCycleClassifiesAnUnparseableSessionListingByItsConfirmation PASS
- The confirmation is sent after the last capture read (the per-session `show-environment`), before any link, re-file, dump or commit, and every committer reaches it, because every production commit goes through `RunCommitCycle` — internal/state/scrollback.go:356-375, internal/state/commit_cycle.go:114-154 — read; measured: `rg -n 'RunCommitCycle\(|state\.Commit\(|WriteScrollbackIfChanged\(' --type go -g '!*_test.go' .` → callers only cmd/state_daemon.go:267 and cmd/state_commit_now.go:109; run: TestCommittersConfirmAfterTheLastCaptureRead → PASS
- The confirmation counts only when it is answered at exit 0 with a pid equal to the committer's own server; an empty answer (pid 0), another server's pid, or an unknown own server stands the cycle down — internal/tmux/tmux.go:92-105, internal/state/scrollback.go:267-282 — read; run: TestRunCommitCycleStandsDownOnAConfirmationNamingNoServer, …StandsDownWithNoOwnServer, …StandsDownOnANewServerOnTheSameSocket → PASS
- Each committer knows its own server from the `TMUX` its process runs under, which tmux sets for both the saver pane and a run-shell job; the pid is read second-from-end because a socket path may carry a comma — internal/tmux/detect.go:17-27, cmd/state_commit_now.go:111 and :128-131, cmd/state_daemon.go:452 — read; measured: `rg -n 'Unsetenv\("TMUX|Setenv\("TMUX' --type go -g '!*_test.go' .` → no matches (nothing in production rewrites it); run: integration TestKillPathStaysFinalUnderHardenedSavePath ×4 → PASS
- A new server on the same socket confirms nothing, and the shutdown flush after `_portal-saver` is killed on a running server still commits with `flush_completed=true` — cmd/state_commit_own_server_realtmux_test.go:94-170 — run: `go test -count=1 -v ./cmd -run 'TestShutdownFlushCommitsAfterItsSaverSessionIsKilledOnARunningServer_RealTmux|TestCommittersCommitOnTheirOwnServer_RealTmux|TestCommittersWriteNothingFromANewServerOnTheSameSocket_RealTmux'` → PASS
- Under `tmux kill-server` no committer commits, whichever read the exit lands after — measured by the integration runs under MEASURED [1-8] → 25/25 trials leave sessions.json and every scrollback file unchanged
- A daemon SIGTERMed 10-30ms before the server preserves the full state — measured: TestShutdown_DaemonSIGTERMedBeforeServerPreservesFullState → 11/11 PASS
- A waiting pane's re-file is a hard link that never overwrites an existing token-named file, and the positional name stays until a committing cycle's housekeeping removes it, so an uncommitted end (stand-down, cancelled dump, failed sessions.json write) never names a missing file — internal/state/scrollback.go:106-157, internal/state/commit.go:35-60 (housekeeping only after a successful write) — read; measured: `ls internal/state/rename_noreplace_*` → no match (the rename path is gone); run: [6-2] runs → PASS
- A daemon tick cancelled mid-dump returns before the commit, and the flush that follows can stand down without stranding a record — cmd/state_daemon.go:255-277 and :301-325 — read; run: TestDaemonTickCancelledAfterTheRefileThenAStoodDownFlushLeavesEverySavedScrollbackOnDisk → PASS
- The daemon's dump writes only through the cycle's ScrollbackWriter. An empty capture over a saved file that may hold bytes (an uninspectable file counts as holding bytes) is written only after a confirmation sent after that capture is answered by the own server. Otherwise it is refused, logged naming the pane, and tallied anomalous — internal/state/commit_cycle.go:74-95, internal/state/scrollback.go:295-312, cmd/state_daemon.go:342-352 — read; run: `go test -count=1 ./cmd` → ok (covers cmd/state_daemon_empty_capture_test.go's TestDaemonDump* tests) and `go test -count=1 -v ./internal/state -run TestCycleScrollbackWriter` → PASS; guard internal/state/commit_guard_test.go:81-88 and :177 refuses `WriteScrollbackIfChanged` in production code outside internal/state → PASS
- A pane's saved transcript is the file its last committed record names. A tokened pane matched by token whose record names a file other than its live positional file is held on its token-named transcript (linked, not moved) until a dump writes its capture, whether the capture is empty-unconfirmed, failed, or no dump runs (`commit-now`). Once written, its record moves to its positional file — internal/state/commit_cycle.go:131-149 and :175-269 — read; run: the TestRunCommitCycle answered/moved/restored/shifted suites (TestRunCommitCycleCommitNowAfterAnAnswerKeepsTheTranscriptForTheNextRestore, …KeepsEachSwappedPanesOwnTranscript, …FirstCommitAfterRestoreKeepsEveryRestoredPanesSavedBytes) → PASS
- Every pane restore recreates carries an identity: a saved token is re-stamped, and a pane saved with none gets a minted token recorded on its saved record under the commit lock before any commit can run — internal/restore/session.go:172-200, internal/restore/restore.go:71 and :85-89, internal/state/restored_pane_tokens.go:28-57 — read; run: TestOrchestrator_RecordsEachMintedTokenOnTheSavedRecordItsPaneWasBuiltFrom, …RecordsNoTokenForAPaneWhoseStampFailed → PASS (same ./internal/restore run)
- A stand-down writes nothing and ends through each committer's existing failure route. The tick logs `tick backed off: tmux stopped answering` and re-touches save.requested, `commit-now` logs through `failCommitNow`, touches save.requested and exits non-zero, and the flush logs and reports `flush_completed=false` — cmd/state_daemon.go:202-209, :244-249, :388-393; cmd/state_commit_now.go:117-119 and :143-149 — read; measured: `rg -n 'TouchSaveRequested' cmd/state_daemon.go` → 1 hit (:205) after the tick-failure line; run: `go test -count=1 ./cmd` → ok (covers cmd/state_commit_backoff_test.go's TestDaemonTickReportsAStandDownAsABackOff, TestCommitNowReportsAStandDownAsABackOff, TestShutdownFlushReportsAStandDownAsABackOff and TestDaemonTickCommitsOnTheTickAfterAStandDown), and TestDaemonTickCommitsAKillWhoseCommitNowStoodDown → PASS under -v
- A kill stays final: each kill's `commit-now` removes the session and its scrollback, the last kill leaves an empty restore state, and the sweep reaps the killed sessions' hooks — measured: integration TestKillPathStaysFinalUnderHardenedSavePath ×4 → PASS
- The section's measured "today" spans read against HEAD. The swallow stays in the shared listing (1 hit). Capture no longer calls `ListSessionNames()` (`rg -n 'ListSessionNames\(\)|ListSessionNamesProbe\(\)' internal/state/capture.go internal/restore/restore.go` → capture.go:95 probe, restore.go:115 shared). `rg -n 'list-sessions failed' internal/restore/restore.go` → 1 hit (:117)
- Lint and build are clean over the change-set — run: `golangci-lint run` → 0 issues; `go vet ./...` and `go vet -tags integration ./...` → clean; `gofmt -l` over every changed .go file → no output; `go build -o <scratch>/portal .` → ok

#### 3. Portal's Own Panes Outlast the Shutdown Signal + 4. Dropped Sessions and Backed-Off Saves Are Logged

- The eager resume shell catches SIGTERM as a caught trap, not an ignored one — cmd/state_resume_chain.go:213,219-221 — read: `hookExecArgs` returns `sh -c 'trap : TERM; <command>; exec <shell>'`. A caught trap is reset to default in the forked hook program and across the trailing exec. SIGHUP is not trapped
- Every hook hand-off goes through that one shell: eager hydrate, answered panel, timeout and file-missing tails, and the lazy-to-eager downgrade — cmd/state_resume_chain.go:172-180; cmd/state_hydrate.go:251; cmd/state_resume_wait.go:319 — read: `rg -n handOffToHookOrShell` shows 4 call sites (hydrate:251, wait:319 and :350 with an empty command, recover:62 with an empty command), and only the command-bearing ones compose `hookExecArgs`
- The parked chain traps TERM beside INT and QUIT, still as a caught trap, and leaves HUP uncaught — cmd/state_hydrate.go:281 — read: `trap : INT QUIT TERM; `
- A draw or waiter killed by SIGTERM before its catch is redrawn while the pane still waits, and a failed marker read counts as pending — cmd/state_hydrate.go:325-344 — read: `while <draw>; [ $? -eq 143 ] && { ! m=$(tmux display-message …) || [ -n "$m" ]; }; do :; done`. POSIX keeps `$?` across the trap action. The command-substitution subshell takes a default SIGTERM, so a signalled read fails and counts as pending, the safe direction
- An answered pane is never put back on its panel by the redraw loop — cmd/state_resume_wait.go:309-320,360-368 — read: Enter execs the hook only after `ClearMarker` succeeds, and a refused clear redraws instead, so a 143 exit after an answer meets a cleared marker and leaves the loop
- The backstop's answered gate means the same as before the change — cmd/state_hydrate.go:295 — read: `if ! { ! m=$(…) || [ -n "$m" ]; }` exits exactly when the read succeeds and comes back empty, the same as the old `if m=$(…); then case $m in '') exit $s;; esac; fi`
- The draw and the waiter catch SIGTERM before any other work, through signal.Notify (a catch, not Ignore) — cmd/state_resume_chain.go:204-206; cmd/state_resume_draw.go:44,185; cmd/state_resume_wait.go:103,502 — read: `rg -n 'signal\.(Ignore|Notify|Reset)|SIG_IGN' cmd internal -g '!*_test.go'` → 3 hits, none an Ignore. Both production config constructions (draw.go:166, wait.go:469) set CatchSIGTERM, so no nil-func call is reachable
- The waiter's SIGHUP keeps its default disposition — cmd/state_resume_wait.go:427-431 — read: Notify for SIGWINCH only
- §3.1's claim that exactly two non-interactive-shell panes exist — cmd/state_resume_chain.go:220, cmd/state_hydrate.go:272 — measured: `rg -n '"sh", "-c"' --type go -g '!*_test.go' cmd internal` → 2 hits
- The §3 behaviours are pinned by tests that would fail without the fix — cmd/state_parked_chain_term_test.go:29, cmd/state_resume_term_test.go:464, cmd/state_resume_handoff_term_test.go:67, cmd/state_resume_hook_shell_test.go:208,231,252,264 — read: the harness runs the real `parkedResumeChain` with the real Go draw and waiter, re-executed from the test binary with real `catchSIGTERM`. `awaitEnded` proves a held pre-catch process really died in its window. `assertEndsOnSIGTERM` proves the hook program and the user's shell keep default handling. The SIGHUP subtests prove a kill still ends the pane
- The kill path's SIGHUP still ends eager and waiting panes at once — internal/restore/resume_pane_hangup_integration_test.go:41 — measured: `go test -tags integration -p 1 ./internal/restore -run TestResumePanes_EndWhenTheirTerminalCloses -count=10` → 10/10 PASS
- Every written commit logs each dropped session by name at INFO with attr `session` — internal/state/commit.go:35-60,85-107 — read: `logDroppedSessions` runs only after the AtomicWrite succeeds and only when a prior index was read. A no-change commit returns at :44 before any log
- "Dropped" is measured against the on-disk index read under the commit lock, never against the daemon's in-memory index — internal/state/commit_cycle.go:121,150 — read: `committed` comes from `readPriorIndex` under the lock and is passed unchanged to `commitOver`. `LoadPrev`'s fallback never reaches the drop measure, so a commit-now kill is not logged a second time by the next tick
- No other sessions.json writer drops a session without a drop line — internal/state/restored_pane_tokens.go:28-56 — read: the only other AtomicWrite of sessions.json rewrites tokens onto existing records and drops none
- A rename gets no drop line — internal/state/commit.go:85-118 — read: an unmatched prior name is paired one-to-one with a newly appeared session whose window layouts (carrying tmux's server-unique pane ids) are identical. The spec leaves the choice of identity to the implementer
- The drop-log behaviour is pinned through every committer — internal/state/commit_drop_log_test.go:75-267, cmd/state_commit_drop_log_test.go — measured: the [4-1] runs above, 45/45 and 4/4 PASS
- All three committers log a stand-down as `<stage> backed off: tmux stopped answering` through their existing failure route, with the cause in `error` — cmd/state_daemon.go:204,244-249,391; cmd/state_commit_now.go:144 — read: the tick and flush log WARN, commit-now logs ERROR, and the tick re-touch and `flush_completed=false` are unchanged
- Every stand-down cause is classed ErrTmuxStoppedAnswering, so it takes the back-off wording — internal/state/scrollback.go:359,367,380-389; internal/state/capture.go:102,111 — read: a failed marker list, a failed session or pane listing and a refused confirmation are wrapped. Any other capture failure (including an unparseable listing) is classified by the confirmation sent after it
- The restore-marker read-failure lines stay, and a set marker logs nothing new — cmd/state_daemon.go:180,380; cmd/state_commit_now.go:101 — measured: `rg -n 'read @portal-restoring|isRestoring query failed' cmd/state_daemon.go cmd/state_commit_now.go` → 3 hits; read: the set-marker paths log at DEBUG, or with the pre-existing commit-now INFO skip line
- An unconfirmed empty capture over a saved transcript is refused and logged naming the pane — internal/state/commit_cycle.go:87; internal/state/scrollback.go:295-312; cmd/state_daemon.go:343-347 — read: WARN `empty capture not confirmed; saved transcript kept` with `pane_key` and `error`, judged against the held file for an answered or moved pane
- Every §4 line uses existing components and closed-vocabulary keys at INFO or above — measured: `rg -n '^\| \`(session|pane_key|error)\` \|' .workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` → 3 hits; read: the drop line is INFO under the committer's `daemon` logger, and the back-off and refusal lines are WARN/ERROR
- CLAUDE.md's resume-hook and save-path text matches the code (hookShellTrap, parkedChainTrap, parkedChainDraw, catchSIGTERM, SIGHUP uncaught, back-off wording, refused empty write) — CLAUDE.md "Resume hooks" and the `state` row — read against cmd/state_hydrate.go:281-350 and cmd/state_resume_chain.go:204-221
- Lint and format are clean across the section's packages — cmd, internal/state, internal/restore — run: `gofmt -l` over every changed .go file → none; `go vet ./cmd ./internal/state ./internal/restore` → exit 0; `go vet -tags integration` on the same → exit 0; `golangci-lint run ./cmd/... ./internal/state/... ./internal/restore/...` → 0 issues

#### 5. Unchanged Behaviour, Accepted Residue and Deferred Work + 6. Testing

- Linters clean over the change-set — repo — run: `gofmt -l <change-set .go files>` → no output; `go vet` and `go vet -tags integration` over ./cmd/... ./internal/{state,tmux,restore,resolver,tui,tmuxerr} → clean; `golangci-lint run` → "0 issues."
- §5.1: the shared listing still swallows a failed list-sessions — internal/tmux/tmux.go:151-157 — read. Only the committing path takes the probe: internal/state/capture.go:95 calls `ListSessionNamesProbe`, while internal/restore/restore.go:115, internal/resolver/query.go:90 and cmd/completion.go:16 still call the shared listing — read: `rg 'ListSessionNames\(\)|ListSessionNamesProbe\(\)|ListSessionsProbe\(\)' -g '!*_test.go'`
- §5.1: picker, resolver, completion and restore keep the "no server, no sessions" reading — read. The picker test (internal/tui/session_listing_failure_test.go:11) and the resolver test (internal/resolver/query_listing_failure_test.go:12) would both go red if either switched to the probe, because each asserts Err nil or a miss. The restore test (internal/restore/restore_test.go:800) would go red on the probe's skip-every-session branch, since it asserts both saved sessions are created. The completion test (cmd/completion_test.go:65) yields no names under either reading, which is the same user-visible behaviour.
- §5.1 and §6.1: restore after a failed listing rebuilds every saved session, and no empty commit follows — internal/restore/restore_test.go:800-848 — read. The test runs `RunCommitCycle` on the same failing client, asserts the error wraps the list-sessions failure, and asserts sessions.json is byte-identical.
- §5.1: commit-now still runs synchronously from session-closed and is resolved on PATH, so it "moves at once" after an upgrade — internal/tmux/hooks_register.go:84 (`run-shell "command -v portal … && portal state commit-now"`, no `-b`); the file is untouched by the range — read
- §5.1: a stood-down commit-now's kill is committed by the next daemon tick — cmd/state_commit_backoff_test.go:242-263 — read. With LastSaveAt set to now, only the save.requested that failCommitNow touched can make the tick fire, so the test pins the whole chain.
- §5.1: the last user session's kill commits an empty state while tmux runs on for `_portal-saver`/`_portal-bootstrap` — cmd/state_commit_drop_log_test.go:196 (real cycle, zero live sessions → 0 committed) and internal/state/commit_cycle_confirm_test.go:267 (a listing of only the two Portal sessions commits empty and removes every transcript) — read. Also measured end-to-end under [1-7].
- §5.1: the shutdown flush still runs on SIGHUP and SIGTERM — cmd/state_daemon.go:469 (`signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)`) and :376-395 (`defaultShutdownFlush` → `captureAndCommit`) — read. Also measured: 33 trials logged `daemon: shutdown reason=signal`.
- §5.1: the three empty-save contract tests have unchanged assertions — internal/state/capture_test.go:1677-1700, :761-786 and cmd/state_commit_now_test.go:151-175 — read (`git diff`) and measured (PASS, see [1-7]). The only edits nearby are fixture plumbing the new interface requires: `ListSessionNamesProbe`, `ConfirmAnswering`, and the commander's `display-message` answer.
- §5.2: measured claims hold — cmd — measured: `rg -n 'rootCmd\.AddCommand\(' cmd -g '!*_test.go' | wc -l` → 11; `rg -n 'CompletionOptions|DisableDefaultCmd' cmd` → 0 matches; cmd/root.go:23-34 `skipTmuxCheck` holds 10 entries, with open/list/kill/completion absent.
- §5.2: the residue windows exist as recorded, and HEAD adds no others in the hardened shells — cmd/state_resume_chain.go:213-221 (`trap : TERM; <hook>; exec <shell>`) and cmd/state_hydrate.go:281, :341-351 (`trap : INT QUIT TERM;` with the draw loop re-run only on status 143 while still pending) — read. Draw and waiter catch SIGTERM from their first statement: cmd/state_resume_chain.go:204-206, wired as the first statement of runResumeDraw (cmd/state_resume_draw.go:43-44) and runResumeWait (cmd/state_resume_wait.go:102-103).
- §5.3 and §5.4: deferred work is not built — the change-set holds no delayed-removal hold, and none of the 99 files carries reboot instrumentation — read
- §6.1 failed listing (tick, flush, commit-now through the production client) — cmd/state_commit_listing_failure_test.go:99-141 — read. It uses `tmux.NewClient` over the fake commander (makeDeps, cmd/state_daemon_run_test.go:200) and asserts sessions.json and scrollback are unchanged.
- §6.1 refused confirmation, empty shutdown answers, and a confirmation naming no server — internal/state/commit_cycle_confirm_test.go:187, :201, :237, :306 — read. Each would go red without the confirmation: an empty listing would commit an empty index.
- §6.1 a different server on the same socket, and the flush after a killed `_portal-saver` (`flush_completed=true`) — cmd/state_commit_own_server_realtmux_test.go:94-110, :135-175 and internal/state/commit_cycle_confirm_test.go:339 — read
- §6.1 uncommitted ends (stand-down, cancelled tick plus stood-down flush, failed write) — internal/state/commit_cycle_uncommitted_test.go:110-152 and cmd/state_daemon_cancelled_refile_test.go:63 — read, and run under [6-2]
- §6.1 an unconfirmed empty capture never overwrites a saved transcript (refused, another server, no-server answer, for tick and flush) — cmd/state_daemon_empty_capture_test.go:75-100 — read
- §6.2 eager and answered hook shells survive SIGTERM, then hand on with default SIGTERM and leave SIGHUP uncaught — cmd/state_resume_hook_shell_test.go:208, :231, :252, :264 — read. Real processes are used, and the user's-shell stub ends on SIGTERM only at the default disposition (`exec sleep`).
- §6.2 parked chain: waiting, and answered with the hook running, under a SIGTERM to the whole group; default SIGTERM after a caught one; SIGHUP still kills — cmd/state_parked_chain_term_test.go:29-108 — read
- §6.2 SIGTERM mid-draw, on the waiter, at a redraw, and before the catch at each hand-off — cmd/state_resume_term_test.go:464-543 and cmd/state_resume_handoff_term_test.go:67-125 — read
- §6.2 pty hangup and killed panes — cmd/state_resume_hangup_pty_test.go:50 and internal/restore/resume_pane_hangup_integration_test.go:41-115 — read, and measured under [2-5]
- §6.3 drops logged by name at INFO; no duplicate on the tick after a commit-now kill; no line for a rename — cmd/state_commit_drop_log_test.go:86-168 and internal/state/commit_drop_log_test.go:75, :185, :211, :250 — read. The tick's sink provably captures drop lines (first subtest), so the no-duplicate assertion is not vacuous.
- §6.3 back-off lines and the refused-empty-write line — cmd/state_commit_backoff_test.go:143, :164, :191 and cmd/state_daemon_empty_capture_test.go:64-73 — read
- §6.5 trials run at the production default log level with tmux unwrapped — cmd/state_shutdown_orderings_integration_test.go:147-169 (`requireUnwrappedTmux` refuses a `#!` tmux) and :215-234 (`assertDefaultLogLevel` reads the daemon's own `resolved=info source=default` line) — read. Also measured: every run passed that gate.
- §6.6 the capture subtest now drives the production client's failed list-sessions — internal/state/capture_test.go:1618-1633 — read. It uses `tmux.NewClient(mock.commander())` with a `*tmux.CommandError` and `errors.Is` on that error.
- §6.6 `TestListSessions` keeps the case for the picker and no longer describes the save path — internal/tmux/tmux_test.go:41 — read

#### 7. Prior Specifications This Fix Touches

- killed-session-resurrects-within-tick-window — session-closed still commits synchronously: the registration is a blocking `run-shell` (no `-b`) of `portal state commit-now`, and nothing in the range changed it — internal/tmux/hooks_register.go:24, :84 — read (`git log b218a434e..HEAD -- internal/tmux/hooks_register.go` → no commits)
- killed-session-resurrects-within-tick-window — commit-now confirms against the server whose hook ran it, taken from TMUX's second-from-last field (comma-safe, session idx -1 tolerated) — cmd/state_commit_now.go:111, :128-131; internal/tmux/detect.go:17-27 — read, and measured end to end: `go test -tags integration -p 1 ./cmd -run 'TestKillPathStaysFinalUnderHardenedSavePath$' -count=8` → ok
- killed-session-resurrects-within-tick-window — a stood-down commit-now is recovered by the daemon's next tick: failCommitNow touches save.requested, and the tick consumes the flag before its cycle and re-touches it on failure — cmd/state_commit_now.go:143-149; cmd/state_daemon.go:186-210 — read
- killed-session-resurrects-within-tick-window — the empty-save contract still holds: an empty `keep` and an all-natural-churn capture both commit an empty index — internal/state/capture_test.go (the two §5.1 subtests); cmd/state_commit_now_test.go — run: `go test ./internal/state -run 'TestCaptureStructurePreLoopFailFatal|TestCaptureStructurePerSessionLogAndContinue' -v -count=1` → PASS; `go test ./cmd -run TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions -count=1` → PASS
- killed-session-resurrects-within-tick-window — no new path resurrects a killed session: the missed-session carry only carries a previous session whose waiting token is on a still-live pane, and RecordRestoredPaneTokens only adds tokens to records already in sessions.json, under the commit lock — internal/state/capture.go:186-224; internal/state/restored_pane_tokens.go:28-58 — read
- built-in-session-resurrection — housekeeping runs only after a confirmed capture: gcOrphanScrollback is reached only from commitOver; commitOver's only production caller is RunCommitCycle, which runs it after captureAndRefile has passed confirmOwnServer; the exported Commit has no production caller — internal/state/commit.go:35-60, :131; internal/state/commit_cycle.go:127-152; internal/state/scrollback.go:356-376 — read (`grep -rn '\bCommit(\|commitOver(' cmd internal` excluding tests → commit.go:30 and commit_cycle.go:150 only)
- built-in-session-resurrection — every capture read comes before the confirmation: skeleton markers, the session listing, the pane listing and the per-session environment reads all happen inside captureStructure, before confirmOwnServer runs. An answer naming no server or another server is refused, and so is an unknown own server — internal/state/scrollback.go:267-283, :356-376; internal/state/capture.go:95-138 — read
- built-in-session-resurrection — the SIGHUP/SIGTERM final flush goes through the same cycle with the daemon's own server, logs "final flush backed off: tmux stopped answering" and reports flush_completed=false on a stand-down — cmd/state_daemon.go:244-249, :268-275, :389-393, :452 — read
- built-in-session-resurrection — the daemon's own server is the pid in the TMUX its `_portal-saver` pane gives it (launched by `respawn-pane` with `portal state daemon`, no env rewrite) — internal/tmux/portal_saver.go:35, :360; cmd/state_daemon.go:452 — read
- resume-hooks-silently-lost — the hook-staleness sweep's stand-downs on an empty or failed pane read are untouched — internal/hooksweep/reason.go:22-23; internal/hooksweep/sweep.go:98 — read (`git diff --stat b218a434e..HEAD -- internal/hooksweep internal/hooks` → empty)
- resume-hooks-silently-lost — the session commit path has the matching posture: a failed list-sessions and a failed list-panes are each wrapped in ErrTmuxStoppedAnswering and stand down, and an empty or partial answer from an exiting server is refused by the confirmation sent after it — internal/state/capture.go:95-112; internal/state/scrollback.go:369-371, :380-388 — read
- lazy-resume-on-attach — the parked chain's trap now catches TERM (caught, not ignored) and leaves HUP uncaught — cmd/state_hydrate.go:281 — read
- lazy-resume-on-attach — the draw and the waiter catch SIGTERM first thing through signal.Notify (a catch, so exec resets it to default). The only production Notify sites are this one, the waiter's SIGWINCH and the daemon's HUP/TERM, so neither resume process catches SIGHUP — cmd/state_resume_chain.go:204-206; cmd/state_resume_draw.go:44, :185; cmd/state_resume_wait.go:103, :502 — read (`grep -rn 'signal\.\(Notify\|Ignore\|Reset\)'` excluding tests → 3 sites)
- lazy-resume-on-attach — a draw or waiter ended by SIGTERM (status 143) in a hand-off window is redrawn only while the marker still reads pending, so an answered pane's own shell is never put back on the panel — cmd/state_hydrate.go:325-350 — read
- lazy-resume-on-attach — the answered pane's hook shell catches TERM with `trap : TERM;` ahead of the spliced command, and the hook program and `$SHELL` keep default handling — cmd/state_resume_chain.go:213, :220 — read
- v1 — the shared listing still swallows a failed list-sessions as no sessions, and ListSessionNames behaves byte-for-byte as before (refactored through sessionNames). Wrapping parse failures in ErrSessionListUnparseable leaves every caller's control flow unchanged — internal/tmux/tmux.go:150-157, :175-218, :224-243 — read against `git show b218a434e:internal/tmux/tmux.go`
- v1 — only the committing path reads ListSessionNamesProbe; the other probe caller (cmd/open_search.go) is from before the range — internal/state/capture.go:95 — read (`grep -rn 'ListSessionNamesProbe\|ListSessionsProbe'` excluding tests; `git show b218a434e:cmd/open_search.go | grep -c ListSessionsProbe` → 2)
- v1 — the picker (internal/tui/model.go:41, internal/tui/pending_resume.go:19), resolver (internal/resolver/query.go:90, :143, :159), completion (cmd/completion.go:15-21) and restore (internal/restore/restore.go:114-125) still read the swallowing listing — read
- v1 — delivered tests that would go red if the shared listing began returning its failure: the TUI fetch asserts Err nil and no sessions (internal/tui/session_listing_failure_test.go); the resolver asserts a miss with no error for the bare chain and a glob (internal/resolver/query_listing_failure_test.go); restore asserts both saved sessions are rebuilt and the following commit cycle errors with sessions.json byte-identical (internal/restore/restore_test.go:800-848); completion asserts no names against a dead socket (cmd/completion_test.go, "offers no names when the production listing fails") — read
- the change-set lints, vets and builds clean in both lanes — repo — measured: `golangci-lint run` → 0 issues; `gofmt -l .` → no files; `go vet ./...` → ok; `go vet -tags integration ./...` → ok; `go build -o <scratch>/portal .` → ok (artefact removed)

#### Test surface

- §6.1 failed listing through the production client: every committer stands down with nothing written — cmd/state_commit_listing_failure_test.go:99-141 (tick, shutdown flush and commit-now, all over `tmux.NewClient(daemonFakeCommander)`) — read: the fake's confirmation answers as the own server by default (cmd/state_daemon_run_test.go:121-131), so a listing failure read as zero sessions would commit an empty index and fail `assertUnchanged`
- §6.6 the capture subtest now drives the production client — internal/state/capture_test.go:1618 — read: `errors.Is(err, listErr)` against `tmux.NewClient(mock.commander())` plus zero list-panes/show-environment calls; the empty-save subtests at :761 and :1677 are unchanged; run: see MEASURED [1-7]
- §6.6 TestListSessions no longer describes the save path — internal/tmux/tmux_test.go:41 — read: the case is renamed to the picker's reading; ListSessionNamesProbe's failure path is pinned in internal/tmux/list_sessions_probe_test.go:24,150
- §2.1/§5.1 the picker, resolver, completion and restore keep "failed listing = no sessions" — internal/tui/session_listing_failure_test.go:11, internal/resolver/query_listing_failure_test.go:12, cmd/completion_test.go:65 (TMUX pointed at a dead temp socket, never the real server), internal/restore/restore_test.go:800 — read: restore rebuilds both saved sessions, then RunCommitCycle returns an error wrapping the listing failure and sessions.json is byte-identical
- §6.1 a refused confirmation writes nothing; empty listings from an exiting server write nothing; the server exiting after any capture read writes nothing — internal/state/commit_cycle_confirm_test.go:187,201,237 — read: the exitingServer model refuses every read after the chosen one, and each case asserts sessions.json bytes and scrollback contents unchanged
- §6.1 a confirmation answered with exit 0 and no output names no server and stands down — internal/tmux/own_server_test.go:48; internal/state/commit_cycle_confirm_test.go:306 — read: ErrNotOwnServer, with saved state unchanged
- §6.1 reads or confirmation reaching a new server on the same socket write nothing — internal/state/commit_cycle_confirm_test.go:339-382 — read: five split points, each asserting the successor answered at least one read (`len(server.successor.calls) == 0` is fatal), so the split cannot pass vacuously
- §6.1 the flush after `_portal-saver` is killed on a running server commits with flush_completed=true; a new server writes nothing — cmd/state_commit_own_server_realtmux_test.go:94,135 — read: a real tmuxtest `-S` socket, `defaultShutdownFlush` asserting `flush_completed`, `replaceWithANewServer` asserting a different pid
- §2.2 confirmation sent after the last capture read, for all three committers — cmd/state_commit_confirmation_test.go:99; internal/state/commit_cycle_confirm_test.go:170-185 — read: fails if any capture read follows the first display-message
- §2.5 back-off line, save.requested re-touch, non-zero commit-now, flush_completed=false — cmd/state_commit_backoff_test.go:143,164,191,217 — read: five stand-down shapes per committer; the line must carry exactly `component` and `error`, with tmux's stderr reachable; the next tick commits
- §4.2 restore-marker read failure lines kept, and a set marker logs nothing new — cmd/state_commit_backoff_test.go:332,359 — read
- §6.1 uncommitted ends (stand-down after the re-file, failed sessions.json write) leave only on-disk paths named — internal/state/commit_cycle_uncommitted_test.go:110 (both committer shapes) — read: the injected dump first asserts the token file holds the transcript, so the re-file happened; run: MEASURED [6-2]
- §6.1 a tick cancelled after the re-file, then a stood-down flush — cmd/state_daemon_cancelled_refile_test.go:63 — read: asserts capture-pane ran (cancelled mid-dump) and flush_completed=false, and every named path is on disk
- §2.4/§6.1 the dump never zeroes a saved transcript on an unconfirmed empty capture, and the refusal is logged with `pane_key` — cmd/state_daemon_empty_capture_test.go:75,102,124,209,270 — read: tick and flush × refused, other-server and silent confirmations. The positive case (:102) asserts the confirmed empty write lands and exactly two display-message reads were made, so the refusal path cannot pass on a writer that never confirms
- §2.4 corrigenda (answered, moved, shifted and restored panes keep their last committed bytes) — internal/state/commit_cycle_answered_test.go:119,154; commit_cycle_moved_test.go:233,259; commit_cycle_shifted_test.go:66,113; commit_cycle_restored_test.go:85,109,137 — read: each asserts the record names a file on disk holding the saved bytes, and no file sits on two records
- §6.2 eager and answered hook shells survive SIGTERM to the top process; the hook program and user's shell keep default SIGTERM; SIGHUP still ends them — cmd/state_resume_hook_shell_test.go:208,231,252,264 — read: real `/bin/sh` processes run the production argv (from runHydrate and from the waiter), and default disposition is proved by the stub ending on SIGTERM (`ws.Signal() == SIGTERM`)
- §6.2 a lazy parked chain survives SIGTERM to its top process and to its whole group; an answered pane goes on to the user's shell; SIGHUP still ends it — cmd/state_parked_chain_term_test.go:29-110 — read: runs the real `parkedResumeChain` string under a stub tmux on PATH (no real server is reachable)
- §6.2 a waiter or a mid-draw that catches SIGTERM leaves the pane waiting; after an answer the hook and shell get default SIGTERM — cmd/state_resume_term_test.go:464 — read: draw and waiter run as the re-exec'd test binary, and assertStillWaiting checks the process is alive, no `cleared` event and no `recovered` event
- §3.2 corrigendum / §6.2 a SIGTERM before the catch (at restore, at draw→waiter, at a redraw) still leaves the pane waiting — cmd/state_resume_handoff_term_test.go:67 — read: `awaitEnded` proves the held process actually died in the pre-catch window before the redraw is credited
- §3.1 the parked chain opens with a caught `trap : INT QUIT TERM` — cmd/state_resume_signals_test.go:93-101 — read
- §6.2 pty hangup ends the parked chain on SIGHUP before the tail — cmd/state_resume_hangup_pty_test.go:50 — measured: `go test ./cmd -run '^TestParkedResumeChain_PaneTerminalHangup$' -count=50` → ok, 50/50
- §6.2 tmux exit and kill end waiting and eager panes with no recovery tail; the saved record stays waiting — internal/restore/resume_pane_hangup_integration_test.go:41 — read: it refuses to pass when portal.log carries no chain start line (:72-74), so the absence check cannot be vacuous
- §6.2/§6.5 waiting pane SIGTERM with daemon, saver commit-now between, server later, next restore still asking — cmd/state_shutdown_waiting_pane_integration_test.go:52 — read: signals only pids walked down from the fixture pane's own `#{pane_pid}`; `awaitCommitNowCommitted` requires a commit-now that exited 0 while the fixture server still answered
- §6.5 pane programs and daemon, then server, preserve interactive-shell and hardened sessions — cmd/state_shutdown_hardened_panes_integration_test.go:66 — read: $SHELL pinned to /bin/zsh or /bin/bash under IsolateStateForTest (HISTFILE and ZDOTDIR re-pointed)
- §6.5 the 10–30ms lead trials, kill-server, and kill-server during a save — cmd/state_shutdown_orderings_integration_test.go:59,76,86 — read: the default log level is asserted from the daemon's own `log-level resolved` line (:215), a script-wrapped tmux is refused (:147), and the during-save test requires at least one in-flight landing (:111)
- §6.4 kill path: each kill is committed by the hook's commit-now alone (daemon retired), the last kill leaves zero sessions and zero scrollback files, and the sweep reaps every killed key — cmd/state_kill_path_integration_test.go:57 — read
- §6.3 a commit logs each dropped session by name at INFO; no line for a kept, reordered or renamed session or a failed write; a drop beside an unrelated new session is logged — internal/state/commit_drop_log_test.go:75,89,142,185,211,250; internal/state/commit_rename_realtmux_test.go:17 — read
- §6.3 each committer logs a drop once, and the tick after a commit-now kill logs none — cmd/state_commit_drop_log_test.go:86,140 — read: :162 fails if the tick did not commit, so "no drop line" cannot pass on a tick that wrote nothing; run: MEASURED [4-1]
- §5.1 the empty-save contract tests stay green and unchanged — measured: see MEASURED [1-7]
- the source guard against committing or writing scrollback around the cycle cannot pass on an empty scan — internal/state/commit_guard_test.go:144 — read
- lane and isolation discipline across all 80 test files — measured: a per-file scan (`grep` for `t.Parallel`, `//go:build`, `IsolateStateForTest`, the portalbintest/SpawnIsolatedDaemon helpers and `tmuxtest.`) → 0 uses of t.Parallel. Every file that builds or execs a portal binary is `//go:build integration` and reaches IsolateStateForTest directly or through newKillPathFixture/newSymptomFixture/setupLazyResumePanel. No change-set test calls `tmux.DefaultClient()`, `exec.Command("tmux"…)` bare, `pkill`, `pgrep -f` or a broad kill
- lint and build — run: `gofmt -l <change-set .go files>` → none; `go vet ./...` → exit 0; `go vet -tags integration ./...` → exit 0; `golangci-lint run` → 0 issues

### Plan Completion
- [ ] Phase 1 acceptance criteria met, except any named below as not measured. Not met in one case: task 1-6's fourth criterion ("the path the waiting pane's record names holds that pane's transcript") fails for a moved tokened pane whose old positional name is overwritten by the pane now at that address before the cycle ends uncommitted. See Needs planning, A3.
- [x] Phase 2 acceptance criteria met, except any named below as not measured
- [x] Phase 3 acceptance criteria met, except any named below as not measured
- [x] Phase 4 acceptance criteria met, except any named below as not measured
- [x] Phase 5 acceptance criteria met, except any named below as not measured
- [ ] Phase 6 acceptance criteria met, except any named below as not measured. Not met in one case: task 6-1's "the panel, discard, burst, renumbered-restore and hangup suites pass" was measured, and the burst file's `TestLazyResumePaste_NeverAnswersAWaitingPane` fails in every package run of `./internal/restore`. It passes when run alone and also fails at the base commit b218a434e, so the cause predates this work. The panel, discard, renumbered-restore and hangup suites pass. See Out of scope, A4.
- [x] Phase 7 acceptance criteria met, except any named below as not measured
- [x] Phase 8 acceptance criteria met, except any named below as not measured
- [x] All tasks completed or deliberately discarded. All 37 tasks are done; none was skipped or cancelled.
- [x] No scope creep

**Criteria not measured**
- [3-3] "`go test ./...` stays green." — needs a unit-lane run of `go test ./...` over the current tree. Reading settles that this task cannot affect the result (CLAUDE.md-only commit; no Go file reads CLAUDE.md), but whether the whole tree passes is only known by running it.
  - Neither verification layer ran the whole suite. The do-now fix verifier ran `go test ./...` twice over the corrected tree after this reconciliation, and both runs were green. On the second run, `TestTailScrollback_PerformanceBudget` (internal/state) missed its 5ms budget at 11.2ms under load average ~20–44 on 10 cores; it passed on the first run, on a package rerun and at `-count=5`, and neither correction touches TailScrollback.

### Code Quality

- `internal/state/commit_cycle.go:34-36`: the `CommitCycle.OwnServer` doc claimed a zero own-server "stands every cycle down". A zero-own-server cycle whose capture fails returns a plain failure. Corrected in this session (A1).
- `internal/state/commit_cycle.go:60-61`: the `ScrollbackWriter.held` doc says held is the file the pane's last committed record named. That is false in the cycle that links a moved pane to its token-named transcript. It is folded into the replan item (A3), so the corrected doc describes whichever writer contract the fix settles on.

Otherwise no issues found: conventions, narrow interfaces, closed log vocabulary and the source guards all hold across the change-set.

### Test Quality

Tests adequately verify requirements, with these exceptions:
- `internal/state/commit_rename_realtmux_test.go` (unit lane) ran its panes under the developer's real `HOME`. It wrote a `cd … && exec sleep 30` line to `~/.zsh_history` on every run (114 so far), breaking the rule that a test never touches anything outside its temp dirs. Corrected in this session (A2). The history count did not move across three targeted runs and two full unit-lane runs.
- No test covers a moved waiting pane with a second pane at its old address and an uncommitted cycle end. Part of A3.
- `TestLazyResumePaste_NeverAnswersAWaitingPane` depends on test order. Out of scope (A4).
- Minor overlaps that verifiers noted and judged harmless:
  - the pane-listing row of `TestRunCommitCycleSendsNoConfirmationAfterARefusedListing`;
  - the commit-now row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON`.

### Blocking Issues

None.

## Findings

### Needs planning

**A3: The dump can overwrite a name that an uncommitted record still holds.**
- **Source ids:** 1-6-1, 7-1-1, 7-2-1.
- **Files:** `internal/state/commit_cycle.go`, `internal/state/scrollback.go`, `cmd/state_daemon.go`.

**What is wrong.** Restore brings a session back one window lower because its window indices had a gap:
- Lazy pane X, saved at work:2.0 with record `work__2.0.bin`, comes back waiting at work:1.0.
- Pane Y, saved at work:3.0, now sits at work:2.0.

On the first tick:
1. X's record is merged on its token and keeps `work__2.0.bin`.
2. The re-file links `work__2.0.bin` to `pane-<X>.bin`.
3. The dump writes Y's capture through `ScrollbackWriter.Write`, which always writes the live key's positional name. Y's bytes replace the name `work__2.0.bin`.

The cycle can then end uncommitted in either of two ways:
- the tick is cancelled at the per-pane context check (`cmd/state_daemon.go:310-312`) and the shutdown flush that follows stands down;
- the `sessions.json` write fails.

Either way, `sessions.json` still records X at 2.0 naming `work__2.0.bin`, which now holds Y's bytes.

The same replacement reaches any pane, held or not and tokened or not, whose live positional name the committed index names on a different pane's record.

The `held` field doc at `commit_cycle.go:60-61` is folded in, since it is false in the linking cycle. If held's semantics stay unchanged, the rewrite is: "held maps a pane key to the file keepAnsweredTranscripts held it on — its token-named transcript, or the file its last committed record named — which its record names in this cycle and its empty capture is judged against." If the fix adds an alternate-name write, the struct and Write docs describe that path too.

**Failure it causes.**
- At the next restore, X replays Y's transcript.
- The re-file adopts the still-present `pane-<X>.bin` on EEXIST, so X's own bytes survive only until X's next capture is written, and that capture carries the replayed Y history.
- §2.4 and task 1-6's fourth criterion fail for this case. It is reachable when shutdown lands within roughly the first tick after restoring such a session.

**How far the fix reaches.**
- It changes the contract of `ScrollbackWriter.Write`, the one writer every daemon dump goes through, and possibly the commit-time filing (`fileAtPositional` / `commitOver`).
- It needs a decision between three forms:
  - (a) write such a capture to another name and file it at commit;
  - (b) defer writing a positional name the committed index names on another pane's record until a commit lands;
  - (c) record it as accepted residue in §5.2, a spec amendment.
- Within (a) or (b), the protected set is also a decision: only held panes, or every pane whose positional name another committed record names.
- No suite case covers this today.

Guard conditions:
- any alternate-name write stays inside `internal/state`'s `ScrollbackWriter` (`TestNoProductionCommitOutsideState`);
- a real-tmux covering test calls `IsolateStateForTest` and `RegisterStateDirTeardownGuard` before `tmuxtest.New` (`TestTeardownGuardCoversEveryServerHostingFixture`).

### Corrected in this session

2 of 2 applied, 0 skipped, 0 reverted. Commit `4379aabb9` (`review(killing-all-sessions-wipes-restore-state): apply do-now findings`), touching `internal/state/commit_cycle.go` and `internal/state/commit_rename_realtmux_test.go`.

- **A1: The OwnServer doc said zero stands every cycle down.**
  - **Source id:** 1-3-1.
  - **File:** `internal/state/commit_cycle.go`.
  - **Failure it fixed:** a maintainer would expect every zero-own-server cycle to log "… backed off". A zero-own-server cycle whose capture fails logs "… failed".
  - **Change:** the doc's second sentence now reads "The cycle commits only on a confirmation that server answered; zero commits nothing, and sends no confirmation."
- **A2: The real-tmux rename test wrote to the developer's zsh history.**
  - **Source id:** test-surface-1.
  - **File:** `internal/state/commit_rename_realtmux_test.go`.
  - **Failure it fixed:** every `go test ./...` appended `cd <tempdir> && exec sleep 30` to the developer's live shell history. Recalling that line turns their shell into `sleep 30` and closes the pane.
  - **Change:**
    - `TestCommitRealTmuxTellsARenameFromADropBesideANewSession` now calls `portaltest.IsolateStateForTest` and `portaltest.RegisterStateDirTeardownGuard` before `tmuxtest.New`.
    - Its `stateDir := t.TempDir()` is replaced by the isolated state dir.
    - `movePaneOn` is unchanged.
  - **Checks:**
    - `~/.zsh_history` held 114 matching lines before and after every run.
    - `TestTeardownGuardCoversEveryServerHostingFixture` passes.

Suite final state:
- **Unit lane:** `go test ./...` green over the corrected tree. One unrelated `TestTailScrollback_PerformanceBudget` timing miss on a second run under load passed on rerun.
- **Linters:** `gofmt` and `golangci-lint` are clean. `go vet` compiles both lanes.

### Out of scope

**A4 (bug, worth investigating): the lazy-resume paste test fails after the discard burst test.**
- **Source id:** 1-problem-and-governing-rule-2-the-save-path-stops-trusting-an-unconfirmed-session-list-1.
- **File:** `internal/restore/lazy_resume_burst_integration_test.go`.

**What happens.** `TestLazyResumePaste_NeverAnswersAWaitingPane` depends on test order. Run right after `TestLazyResumeDiscard_BurstNeverConfirms`, as every package run of `./internal/restore` does:
- its "the rest of a paste the drain gave up on answers nothing" subtest never shows the drain-gave-up report;
- the Enter subtest (:183) and the sentinel cleanup (:201) then fail in cascade.

**Evidence it predates this work.**
- It reproduced 4/4 at HEAD and 1/3 at b218a434e, before this work.
- It passed 3/3 when run alone.
- No commit in the change-set touched the test or `cmd/tty_drain.go`.

**Failure it causes.** The integration lane goes red on every package run of `internal/restore` whether or not the save path is correct, so the lane's verdict cannot be read without rerunning that test alone.

**Guard conditions for whoever takes it.**
- `flushTTYInput` stays the sole flush site (`TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace`).
- A per-test fixture change calls `IsolateStateForTest` and `RegisterStateDirTeardownGuard` before `tmuxtest.New`.
