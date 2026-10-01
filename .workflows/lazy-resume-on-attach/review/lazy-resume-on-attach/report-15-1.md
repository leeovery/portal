TASK: Hold the Resume Panel on an Alternate Screen Whatever the Pane's `alternate-screen` Setting (tick-283867, lazy-resume-on-attach-15-1)

ACCEPTANCE CRITERIA:
1. Lazy registration, each of the three tails: pane-scoped `set-option -p` pin of `alternate-screen on`, then `@portal-resume-pending`, then the mid-restore clear, then the chain parks; no read of `alternate-screen` before the pin.
2. No registration, or one resolving eager: every tail restores as today; neither pin nor marker written.
3. `$TMUX_PANE` absent or executable unresolvable: eager hook, one `set resume pending marker failed` WARN naming pane and error; neither pin nor marker written.
4. Pin refused: eager hook, one refusal WARN naming the pin's error; no marker written.
5. Pin lands, marker refused: eager hook, refusal WARN naming the marker's error, pin lifted (`set-option -pu … alternate-screen`) before the hook; a refused lift gets a second, differently worded WARN and the hook still runs.
6. Enter: leave bytes, marker clear, bounded `#{alternate_on}` poll until 0, unset, then run the stored command. A confirmed discard takes the same four steps after its removal and before the plain shell.
7. Enter or discard whose clear is refused: redraw with the reason, pin left in place.
8. Recover tail on a marked pane: leave, clear, confirm, unset, shell. On an unmarked pane: nothing at all.
9. Unconfirmed leave keeps the pin with one WARN naming the pane; a refused unset gets one WARN naming pane and error; neither stops the hand-over; neither reads `unset resume pending marker failed`.
10. Backstop does the same in shell form after its marker clear (bounded poll, unset only on 0), writes nothing to stderr, reaches the shell when tmux refuses or is absent; inert when the tail started.
11. Every new `-t` goes through `tmux.PaneIDTarget`, including the backstop argv; wait/recover files keep their import and call guards.
12. Real tmux, global `alternate-screen off`: restored lazy pane shows the panel while `capture-pane -a -p` returns the pre-reboot line.
13. Real tmux: answered by Enter, confirmed discard and recovered waiter: screen shows the transcript, no card line on screen or in `capture-pane -S -`, no pane-level `alternate-screen`.

STATUS: issues_found

SPEC CONTEXT: The panel is painted into the pane's alternate screen so it never reaches the scrollback, and the replayed transcript waits underneath it. A user's `set -g alternate-screen off` makes tmux ignore the enter and leave sequences, so the card lands on the primary grid and the saver captures it. The spec now pins `alternate-screen on` at pane level before the pending marker is written, unconditionally and with no read first. A pin that cannot be written makes the pane fall back to eager, under the existing refusal WARN. A refused marker lifts the pin. Every route off the panel lifts the pin last, and only once `#{alternate_on}` reads 0 on a bounded poll. An unconfirmed leave or a refused unset keeps the pin and gets a WARN. A refused clear keeps the pin for the redraw. The backstop takes the same steps in shell, with nothing recorded.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_hydrate.go:393-419 `markResumePending` resolves the pane, then the exe, then writes the token (from a sibling task), then the pin at :409, then the marker at :412. A refused marker lifts the pin at :413-415, with its own WARN `lift alternate-screen pin failed`.
  - cmd/state_hydrate.go:367-379: the refusal WARN `set resume pending marker failed` names the pane and whichever error refused.
  - cmd/state_resume_altscreen.go:39-67: `releaseAltScreenPin`, `awaitPrimaryScreen` (20 reads, 50ms apart) and `paneAltScreenPin`, wired through `ReadPaneOption(pane, "alternate_on")` and `UnsetPaneOption(pane, "alternate-screen")`. WARNs: `alternate screen leave unconfirmed` and `unset alternate-screen pin failed`.
  - cmd/state_resume_wait.go:351-360: `resumeUnfreeze` writes the leave, clears, and on a refused clear returns to the redraw with the pin untouched. Otherwise it releases the pin before Enter's lookup at :307 and the discard's shell at :341. Command wiring at :491.
  - cmd/state_resume_recover.go:48-62: leave, clear, release, cook, shell. An answered pane returns early at :41-43. Wiring at :109.
  - cmd/state_hydrate.go:293-322: `parkedChainBackstop` plus `backstopReleasePin`. Every tmux argv goes through `shellquote.Join` with `tmux.PaneIDTarget`, has stderr discarded, and the unset is gated on `n > 0`. The loop makes exactly `altScreenLeaveAttempts` reads before giving up.
- Notes: The code matches the specification and the task's settled shape. The token write before the pin comes from a sibling task and is consistent with the specification. The wait and recover files add no forbidden imports or identifiers. Their tmux access goes through the client alone, via the new file.

TESTS:
- Status: Adequate
- Coverage:
  - Mark step, cmd/state_hydrate_lazy_test.go:
    - Order of token, pin, marker and skeleton clear on all three tails (:419-446, :755-782), with no `alternate` read before the pin.
    - Eager and unregistered panes get no pin and no marker (:122-172).
    - Refusal table (:282-346) covers TMUX_PANE absent, exe unresolvable, pin refused, marker refused and token refused, with `wantNoWrite`, `wantNoPin` and `wantNoToken`.
    - The pin's error is named (:348-362). A refused marker lifts the pin in order, and a refused lift gets its own WARN (:364-417).
    - `assertNoPendingMarker` is narrowed to `state.ResumePendingOption` (:679-684).
  - Answer routes:
    - `TestReleaseAltScreenPin` (cmd/state_resume_altscreen_test.go:83-166) drives Enter, the confirmed discard and the recover tail through polling, an unconfirmed leave, a read that fails and a refused unpin. It checks the pause cadence, the WARN wording and pane attrs, and that the hand-over still runs. It counts the WARN sites for the new wordings.
    - The exact sequences are pinned in cmd/state_resume_enter_test.go:96-119, cmd/state_resume_discard_test.go:75,122,146,195,230,316 and cmd/state_resume_recover_test.go:110-112,160.
  - Backstop: cmd/state_resume_backstop_test.go:173-254 checks the exact transcripts for the confirm-then-unpin path, a poll that flips after 3 reads, a leave never confirmed (20 reads, no unpin), failing tmux, absent tmux, and that stderr stays empty.
  - Real tmux: internal/restore/lazy_resume_panel_integration_test.go:275-364 sets global `alternate-screen off` on every fresh server before restore (:542-544). For each of Enter, the recovered waiter and the confirmed discard it asserts:
    - the pin is present while the panel waits;
    - `capture-pane -a -p` holds the pre-reboot line and no card copy;
    - the marker clears and the pin is lifted;
    - the screen shows the transcript;
    - no card copy is on the screen or in `capture-pane -p -S -`.
- Notes: Each test would fail if its behaviour broke. The hydrate ordering tests overlap somewhat (:188, :419, :755), but each adds a distinct assertion, so this is not bloat.

CODE QUALITY:
- Project conventions: Followed. Function seams are injected through config. `-t` targets go through the exactness vocabulary. The new WARNs use distinct wordings. The integration case is in the integration lane with `IsolateStateForTest` and `RegisterStateDirTeardownGuard` called before `tmuxtest.New`.
- SOLID principles: Good. `altScreenPin` is a small three-func seam shared by the waiter and the recover tail.
- Complexity: Low
- Modern idioms: Yes (`for attempt := range altScreenLeaveAttempts`, `strings.Lines`).
- Readability: Good. Comments state the ordering rationale and hold true against the code.
- Issues: one stale subtest name, below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_discard_test.go:217 — The subtest is named "it makes no tmux call but the marker unset", but this change made it assert `"discard", "stdout", "clear", "confirm", "unpin", "exec"` at :230. The discard path now also issues the `#{alternate_on}` reads and the `alternate-screen` unset. Fix: rename it to state the actual contract, e.g. "it makes no tmux call but the marker unset and the pin's release". — FAILS: the name states a one-call tmux contract that the assertion beneath it, and the code, contradict. A reader or a later change is judged against the wrong contract.

UNSETTLED:
- "On a real tmux server with `alternate-screen off` set globally before restore, a restored lazy pane shows the panel on screen while `capture-pane -a -p` returns its pre-reboot line intact underneath." — Settled only by running the integration lane: `go test -tags integration -p 1 ./internal/restore -run TestLazyResumePanel_HoldsThePanelOnAnAlternateScreenTheInstallTurnedOff`.
- "On that server the pane is answered three ways: Enter, a confirmed discard, and a waiter that ended without answering and is recovered by the chain's tail. After each, the pane's screen shows its pre-reboot transcript, neither the screen nor `capture-pane -S -` holds any line of the card, and the pane carries no pane-level `alternate-screen`." — Settled by the same integration run. This includes whether the pre-reboot line stays within the replayed visible screenful across the second and third reboot rounds, which reading cannot establish.
