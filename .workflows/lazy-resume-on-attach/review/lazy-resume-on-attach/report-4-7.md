TASK: lazy-resume-on-attach-4-7 (tick-104e82) — A Restored Pane Comes Back Holding the Panel. An integration-tagged real-tmux suite that reboots a two-session fixture (a lazy subject with an unregistered sibling pane, and an eager control) through the real restore orchestrator with a built binary, then asserts the pane through Portal's own reads and `capture-pane`, and drives Enter and `exit` from tmux.

ACCEPTANCE CRITERIA:
- The lazy subject's pane key is in `CaptureStructure`'s pending set and the eager control's is not.
- `capture-pane -p` on the subject returns the panel (title, registered command, both key hints) and `capture-pane -a -p` returns the pre-reboot line intact underneath it.
- The command the panel shows is the stored command as stored, its space and embedded single quote included.
- The eager control fires its hook within the existing pane-reaction budget in the same run, shows no panel, and carries no pending marker.
- The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`; no `portal state resume-draw` survives the hand-off.
- The subject's resting tree carries a combined resident size below 22 MB, reported whether the test passes or fails.
- While the subject holds the panel, a command sent to its sibling with `send-keys` runs in that pane and its output is readable from `capture-pane -p` on the sibling.
- The sibling carries no pending marker, is absent from the pending set, shows no panel, and its scrollback file is written while the subject's is not.
- A second capture taken while the subject waits leaves its scrollback file byte-unchanged and carries its previous record forward.
- Restored a second time from that capture, the subject comes back holding the panel with the pre-reboot line intact underneath, its pending marker set, and its key in the fresh capture's pending set.
- `send-keys Enter` clears `@portal-resume-pending` within a bounded poll, produces the subject's sentinel, and leaves the pre-reboot line visible with the resumed command's output over it.
- `send-keys exit` closes the subject's pane on the first press, within the existing restored-pane suite's budget.
- Portal's conclusions are read through Portal's own reads; pane content through `capture-pane`; raw tmux only stages and sends keys.
- `//go:build integration`, `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket, a `restoretest`-built binary, no `portal state daemon`, nothing left behind.
- The suite signals no process and enumerates only the pid tmux reports for its own pane.

STATUS: issues_found

SPEC CONTEXT: The spec's §4.1 and §5.1 require a lazy pane to hold a live Portal process that paints the panel into the pane's alternate screen, with the replayed transcript kept in the primary buffer underneath. Panes beside it stay fully live. §4.2 requires the draw to hand off before waiting, so the resting pane is one parked `sh -c` carrying the recovery tail over one `resume-wait`. The 2026-09-22 corrigendum records the resting tree at 9920 KB, measured by this suite, and asserts the daemon's 22 MB as the per-pane ceiling. §6.1 says Enter hands over in place, with "nothing wiped". Leaving the alternate screen reveals the transcript and the command starts over it. The corrigendum on the chain's recovery step says an answered pane gets no second shell, so the first `exit` closes it. Under §7.2/§7.3 and the 2026-09-21 corrigendum, a frozen pane's bytes are re-filed under its durable token while it waits. §9.1: a frozen pane is still restored on the next boot with the transcript it had when it paused.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/lazy_resume_panel_integration_test.go:1 (build tag), :67-262 (the 13 ordered subtests, names matching the plan's list verbatim), :279-326 (fixture setup), :375-420 (captureRound), :425-434 (reboot/restore/hydrate), :523-535 and :580-723 (process-tree read)
- Notes:
  - All the scaffolding the plan asked for is present: `-short` skip, `tmuxtest.SkipIfNoTmux`, `restoretest.BuildPortalBinaryDir`, `IsolateStateForTest` plus `PORTAL_STATE_DIR` and `state.EnsureDir`, `PORTAL_HOOKS_FILE`/`PORTAL_PREFS_FILE` pointed into a temp dir with prefs never written, `RegisterStateDirTeardownGuard`, and `tmuxtest.New(t, "ptl-lazy-")`. There is no `t.Parallel` and no daemon is spawned.
  - The subject's registration goes through `hooks.Registration{Command: …}` with no mode, so the string form inherits the shipped lazy default (:333-336). The control carries `resumemode.Eager` (:337-340). The subject's command `echo 'resumed ok' | tee subject-fired` has spaces and embedded single quotes. It is 37 cells, inside the card's 52-cell content width (`destructiveBodyWidth`, internal/tui/destructive_confirm.go:15). It crosses the parked `sh -c` as the draw's `--command` argv (cmd/state_resume_chain.go:66), so the pane-level check does prove the quoting.
  - The panel strings match the renderer: "Resume session" (internal/tui/resume_panel.go:9) and the "⏎ resume" / "d discard" footer (resume_panel.go:13-15, :78).
  - Sound divergences from the plan's wording:
    - captureRound uses `state.CaptureAndRefile` + per-pane dump + `state.Commit` instead of `CaptureStructure` + `EncodeIndex`. That is the entry point the codebase now mandates, so an index never names a waiting pane's vacated positional path. It mirrors cmd/state_daemon.go:249-294.
    - `restoretest.RestoreFromState` is exactly `NewRestoreOrchestrator` + `RestoreWithMarker` (internal/restoretest/reboot.go:55-58).
    - Subtest (e) asserts the record names `state.PendingScrollbackFile(token)` and that the bytes there equal the pre-pause bytes. That matches the re-file design the 2026-09-21 corrigendum introduced.
    - `split-window` without `-h` stacks the panes vertically rather than left/right. This does not change what is being proven.
  - The process-tree read resolves `#{pane_pid}` from the fixture socket and walks descendants with `pgrep -P <pid>`, one level at a time (:639-650). It describes them with `ps -o pid=,ppid=,rss=,command= -p <list>` (:674). Nothing is signalled.

TESTS:
- Status: Adequate (two assertions weaker than their criteria; see FINDINGS)
- Coverage:
  - All thirteen planned tests exist and map one-to-one onto the acceptance criteria.
  - The pending set is read through `state.CaptureStructure`/`CaptureAndRefile` and the marker through `tmux.ReadPaneOption`. The panel is checked on `capture-pane -p` and the underlying transcript on `capture-pane -a -p`. `-a` errors when the pane is not on its alternate screen, so (b) and the second-reboot check also prove the alternate-screen mechanism.
  - The byte-identity check across the unanswered capture is present, and so is the second reboot's return of the panel.
  - A sentinel-absent check before Enter proves the lazy registration never fired across two reboots.
  - First-`exit` closure is asserted within `exitClosesPaneBudget`.
- Notes:
  - Not over-tested: each subtest carries one criterion and nothing is mocked.
  - The subject-not-written assertion (:177) follows from the test's own dump loop skipping pending keys (:397), so it re-states pending-set membership. The plan prescribed that route, and the byte-identity subtest carries the substantive check.
  - The sibling-live assertion (:152-153) is satisfied by the shell echoing the typed command, so it does not prove the command ran. See FINDINGS.
  - The reveal assertion (:238) reads the whole history, not the visible screen. See FINDINGS.

CODE QUALITY:
- Project conventions: Followed. Integration tag, isolation helpers, `restoretest`-built binary, exact-match pane targets for addressed reads, and no seam assignment.
- SOLID principles: Good
- Complexity: Acceptable
- Modern idioms: Yes (`strings.FieldsSeq`/`SplitSeq`, `slices.Contains`; go 1.26)
- Readability: Good
- Issues: `restingTree`'s settle condition folds in the shape check its own comment says belongs to the caller (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/restore/lazy_resume_panel_integration_test.go:152 — The sibling-live check sends `echo sibling-still-live` and waits for the sibling's screen to contain `sibling-still-live` (:153). The shell's echo of the typed command line (`<prompt> echo sibling-still-live`) already contains that string, so the wait is satisfied whether or not the command ran. Fix: send a command whose output does not appear in its own text, e.g. `printf 'sibling-%s\n' still-live`, and keep awaiting `sibling-still-live`. Alternatively, assert a screen line equal to `siblingLiveLine` after `strings.TrimSpace`. — FAILS: a sibling whose pane accepts keystrokes but never runs them (input echoed by the tty line discipline or the line editor, no command executed) passes "it leaves the pane beside it live". The criterion that the command "runs in that pane and its output is readable" is left unproven.
- [in-scope] [contained] internal/restore/lazy_resume_panel_integration_test.go:238 — "it reveals the transcript that was underneath the panel" looks for the pre-reboot line in `paneTranscript` (`capture-pane -p -S -`, :452). That read includes the pane's scrollback history. The criterion and the plan's step (f) require the line visible on `capture-pane -p`. Fix: read `fx.paneScreen(t, subjectPaneTarget())` for that assertion (:238-242). Subtest (b) and the second-reboot check already show the line on the primary screen via `-a`, and the resume adds only the command's output and a prompt. — FAILS: if the hand-over cleared the primary screen after leaving the alternate screen, tmux's default `scroll-on-clear` would move the transcript into history. The user would see a blank pane under `resumed ok`, which violates the spec's "nothing is wiped". The test still passes, because `-S -` finds the line in history.
- [in-scope] [contained] internal/restore/lazy_resume_panel_integration_test.go:529 — `restingTree` only settles when `len(tree) == 2`. Otherwise it fails with "never settled after the draw handed off" (:532). Its comment (:520-522) says "the assertions about the tree's shape are the caller's", which the code contradicts, and `assertRestingTreeShape`'s count branch (:582-585) can never fire. Fix: settle on `len(tree) >= 2 && !treeRuns(tree, resumeDrawArgv)` and leave the count to `assertRestingTreeShape`. — FAILS: a pane that stacks a second shell, the exact regression "it carries one shell parent and one waiter" exists to catch, waits out the full 10s `HydrateBudget`. It is then reported as a hand-off that never settled rather than as a tree with the wrong process count. The RSS subtest fails the same way before it computes or logs the combined figure.

UNSETTLED:
- "The subject's resting tree — the parked shell plus the waiter, measured once the pane is waiting and the draw is gone — carries a combined resident size below 22 MB, the ceiling the daemon was measured at, and the figure is reported by the test whether it passes or fails." — Settled by running `go test -tags integration -p 1 -v ./internal/restore -run TestLazyResumePanel_RestoredPaneHoldsThePanel` and reading the logged combined figure. The `t.Logf` line is visible on a passing run only under `-v`.
- "The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`, and no `portal state resume-draw` survives the hand-off." — Needs the same run. Whether the pane's own process is the parked `/bin/sh -c` depends on the host `sh` exec-optimising tmux's `respawn-pane` command, which reading cannot settle.
- "`send-keys exit` closes the subject's pane on the first press, within the same budget the existing restored-pane suite uses." — Needs the same run: a real parked shell's tail has to find no marker and add no second shell.
- "Restored a second time from that capture, the subject comes back holding the panel with the pre-reboot line intact underneath it, its pending marker set, and its key in the fresh capture's pending set — an unanswered offer returns rather than being spent, and nothing of the transcript is lost across the second reboot." — Needs the same run.
- "The suite carries `//go:build integration`, uses `portaltest.IsolateStateForTest`, a disposable `tmuxtest` socket and a `restoretest`-built binary, spawns no `portal state daemon`, and leaves no server or subprocess behind." — The "leaves nothing behind" half needs a post-run check that no `ptl-lazy-*` tmux server and no `portal state resume-wait` process outlived the run. The rest is settled by reading.
