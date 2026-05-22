---
status: in-progress
created: 2026-05-22
cycle: 2
phase: Plan Integrity Review
topic: Slow Open Empty Previews And Zombie Sessions
---

# Review Tracking: Slow Open Empty Previews And Zombie Sessions - Integrity

Cycle 2 follow-up after applying the 7 findings from cycle 1. Six of the seven applications cleanly resolve their issues. One application (cycle-1 finding #2 → Task 6-6) was applied to the `Do` section but is internally inconsistent with the surrounding Acceptance Criteria and Tests, and the applied Do prose still contains stream-of-consciousness deliberation rather than a single canonical instruction. The remaining cycle-1 fixes (5-3 ordering, 5-1 N-floor, 4-4 nil handling, 6-5 EWOULDBLOCK, 2-2 refactor annotation) read cleanly with no new structural issues introduced.

## Findings

### 1. Task 6-6 Do section still contains unresolved deliberation and contradicts its own Acceptance Criteria / Tests after cycle-1 fix

**Severity**: Critical
**Plan Reference**: `phase-6-tasks.md` Task 6-6 (Assert Component D self-eject in live context after external saver-pane kill), `Do` section + `Acceptance Criteria` + `Tests`.
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Cycle 1's finding #2 was applied to the `Do` section of Task 6-6, but the replacement prose did not resolve the underlying ambiguity — it preserved it in a different form, and crucially the cycle-1 application did NOT also update the `Acceptance Criteria` and `Tests` blocks that are now structurally contradicted by the new Do text.

Concretely, after the cycle-1 fix:

1. **Do step 1 opens with "Trigger saver-pane pid mismatch WITHOUT killing the legitimate daemon process"** but the body of step 1 ends with "use `respawn-pane -k` — which DOES send SIGKILL to the saver pane (the legitimate daemon)". This contradicts the step's opening intent within five lines.

2. **Do step 2** explicitly admits the inconsistency: *"this task's name claims 'self-eject', but the mechanism `respawn-pane -k` produces SIGKILL-induced death, not `os.Exit(0)` self-eject"*. This is planning-time deliberation visible to the implementer; the spec contract was supposed to be resolved in planning. Step 2 then *changes the scope of the task* — "this task asserts that AFTER an external kill via `respawn-pane -k`, the daemon is dead AND the scrollback directory is bytes-identical" — composing A+D rather than testing D alone.

3. **Tests at lines 329–333 still assert the original (pre-cycle-1) framing**:
   - `"composite D: legitimate daemon self-ejects within (N+1) tick intervals after external saver-pane kill"`
   - `"composite D: self-eject exit code is 0 (os.Exit(0), not signal-induced)"`
   The "exit code is 0" assertion is unsatisfiable under the cycle-1-applied Do prose — a SIGKILL'd process exits with signal=KILL, NOT exit code 0. `os.Process.Wait` exposes this via `ProcessState.ExitCode() == -1` and `ProcessState.Exited() == false` on Unix. The Tests directly contradict the Do.

4. **Acceptance Criterion at line 319** still says: *"External saver-pane kill via `tmux respawn-pane` triggers the daemon's self-check to fire (consult Phase 5 Task 5-6 for the canonical respawn command form)."* The "consult Phase 5 Task 5-6" pointer was the exact issue cycle-1 finding #2 flagged as forcing the implementer to figure out the mechanism, and the applied cycle-1 fix to the Do section did NOT update this criterion.

5. **Acceptance Criterion at line 332** says: *"`composite D: self-eject exit code is 0 (os.Exit(0), not signal-induced)"`* and edge-case at line 339 instructs the test to *"Assert exit status indicates clean `os.Exit(0)` (no signal). If the test mistakenly uses the wrong respawn form, this assertion catches it."* — directly contradicting the Do's SIGKILL mechanism.

The implementer is presented with a task that says: "use a mechanism that produces SIGKILL-induced death; assert the exit code is 0 (proving it was os.Exit(0) and not signal-induced)". These cannot both be true.

The cycle-1 proposed replacement text accepted the scope change ("this composes A+D: A's SIGKILL-bypass-of-final-flush invariant in the live context") but did not propagate that scope change to the Acceptance Criteria and Tests. Either the task tests A's SIGKILL-bypass-of-final-flush in live context (requiring Acceptance + Tests rewrite to drop the "exit code 0 / os.Exit(0)" claim) OR the task tests D's `os.Exit(0)` self-eject in live context (requiring a different mechanism that does NOT SIGKILL the daemon).

The cleanest resolution is the latter: pick a mechanism that triggers D's self-check without killing the daemon process, so the existing Tests/Acceptance ("exit code 0", "self-eject", "no final flush") all hold consistently. The mechanism is `tmux respawn-pane` (without `-k`) which on modern tmux refuses to respawn a live pane process — so the test must first arrange for the legitimate daemon to NOT be the saver pane process. The way to do that in live state: `tmux move-pane` or `tmux break-pane -t _portal-saver` (re-parents the saver-pane elsewhere, leaving `_portal-saver` empty and its pane pid different), OR `tmux kill-pane -t _portal-saver` followed by `tmux new-window -t _portal-saver` (replaces the pane; the original daemon process becomes orphaned from the saver session but its process is still alive). The orphaned-but-alive daemon's next tick observes `pane_pid != os.Getpid()` and self-ejects.

This is the canonical live-context D test mechanism: structurally re-parent or replace the saver pane WITHOUT killing the daemon process, and observe the daemon self-eject within `(N+1)` tick intervals via `os.Exit(0)` with exit code 0 and bytes-identical scrollback.

**Current**:
```markdown
**Do**:
- Capture pre-eject baseline: snapshot `<stateDir>/scrollback/` immediately before the external mismatch event. Use the same fingerprint-diff helper from Task 6-4.
- Capture the legitimate daemon's PID from Task 6-3's survivor (also equals `_portal-saver`'s pane PID at this point).
- Trigger saver-pane pid mismatch WITHOUT killing the legitimate daemon process. The composite-context mechanism (which differs from Phase 5 Task 5-6 because here the daemon IS the saver-pane process, not a non-saver subprocess):
  1. `tmux kill-session -t _portal-saver` against the test's tmux socket. The legitimate daemon receives SIGHUP via tmux's normal session-teardown path. **However**, Phase 4 Task 4-1's escalation arm is NOT what we're testing here — we are testing Component D's tick-loop self-check. To prevent the SIGHUP-driven shutdown handler from running (which would invoke `defaultShutdownFlush` and contaminate the scrollback no-final-flush assertion), the test MUST arrange for the legitimate daemon to NOT be the saver pane process at the moment of the kill. Achieve this by:
     - `tmux respawn-pane -t _portal-saver 'sh -c "exec tail -f /dev/null"'` (WITHOUT `-k`). On modern tmux this refuses if the pane process is still running, so first send a no-op signal to confirm the daemon is alive, then use `respawn-pane -k` — which DOES send SIGKILL to the saver pane (the legitimate daemon). SIGKILL bypasses Go's signal handlers entirely, so `defaultShutdownFlush` does NOT run. The daemon dies via SIGKILL, not self-eject.
  2. **Revised mechanism for testing the self-eject path specifically**: this task's name claims "self-eject", but the mechanism `respawn-pane -k` produces SIGKILL-induced death, not `os.Exit(0)` self-eject. The composite test cannot exercise D's `os.Exit(0)` path against the legitimate saver-pane daemon without also surviving the SIGHUP-via-kill-session path's shutdown handler. Resolution: this task asserts that AFTER an external kill via `respawn-pane -k`, the daemon is dead AND the scrollback directory is bytes-identical (no final flush ran — SIGKILL bypasses defers). This composes A+D: A's SIGKILL-bypass-of-final-flush invariant in the live context.
  3. Document in the test preamble that Component D's `os.Exit(0)` self-eject path is verified in Phase 5 Tasks 5-5/5-6 against staged conditions; the composite test verifies the OBSERVABLE end-state (daemon dead, scrollback bytes-identical) which is the user-visible consequence under either eject path.
- Poll for the daemon's exit via `kill(legitimatePID, 0) == ESRCH` every 100 ms, bounded to `(selfSupervisionHysteresisTicks + 1) * TickerPeriod + 2 s` from the external-kill timestamp. Read `selfSupervisionHysteresisTicks` from `cmd/state_daemon.go` (the test lives in a package that can reference the unexported constant directly).
- After observing exit, snapshot `<stateDir>/scrollback/` and diff against the pre-eject baseline. Assert byte-identical (no new files, no deletions, no mtime/size/content changes).
- Verify `daemon.pid` is left in place after exit (intentional per Component D — Component C's pre-check handles the stale value on the next acquire). Do NOT delete it.

**Acceptance Criteria**:
- [ ] External saver-pane kill via `tmux respawn-pane` triggers the daemon's self-check to fire (consult Phase 5 Task 5-6 for the canonical respawn command form).
- [ ] Legitimate daemon's process exits within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod` of the external-kill timestamp.
- [ ] Exit observed via `kill(pid, 0) == ESRCH` polling (or `os.Process.Wait` if the test owns the process — which it does, since Task 6-1 spawned it).
- [ ] Scrollback directory snapshot post-eject is byte-identical to pre-eject baseline (no `os.Exit(0)`-skipped deferred flush ran).
- [ ] `daemon.pid` file remains on disk post-eject (Component D intentional behaviour — no defer cleanup).
- [ ] `selfSupervisionHysteresisTicks` is referenced as the constant from `cmd/state_daemon.go`, not hardcoded — change-resistant.
- [ ] Test fails clearly with the measured elapsed time if the daemon does not exit within budget.
- [ ] Test fails clearly with the diff path if scrollback changed across the eject window.

**Tests**:
- `"composite D: legitimate daemon self-ejects within (N+1) tick intervals after external saver-pane kill"`
- `"composite D: scrollback directory byte-identical across self-eject window (no final flush)"`
- `"composite D: daemon.pid left stale after self-eject (handled by Component C pre-check)"`
- `"composite D: self-eject exit code is 0 (os.Exit(0), not signal-induced)"`

**Edge Cases**:
- The respawn-pane command itself fails (e.g., `_portal-saver` doesn't exist at the moment of invocation because the post-bootstrap state lost it) — pre-check at the start of this task verifies `_portal-saver` exists; if not, fail with "saver session absent before D test sequence — composition state corrupted".
- The daemon exits faster than expected (e.g., within 1 tick instead of N) — this would indicate `selfSupervisionHysteresisTicks` has been lowered below the measured value; log the actual tick count and pass the test (faster eject is not a failure) but Logf a notice.
- The daemon does not exit at all within budget — assertion fails with elapsed time; test runs cleanup to kill the daemon explicitly.
- Scrollback baseline includes a `.bin` that was mid-write at the moment of the external kill — atomic write semantics in the daemon's commit pipeline should prevent partial files; if a mid-write byte appears in the diff, the diff helper logs the path and the test fails (this is a real defect worth surfacing).
- The respawn-pane command sends SIGKILL to the legitimate daemon instead of triggering the membership probe (because `-k` was used) — the test then catches "daemon died via signal, not self-eject" via exit code: a signal-induced death leaves the process status as signal=KILL, but `os.Process.Wait` exposes this. Assert exit status indicates clean `os.Exit(0)` (no signal). If the test mistakenly uses the wrong respawn form, this assertion catches it.
- `selfSupervisionHysteresisTicks` value is large enough that `(N+1) * TickerPeriod` exceeds Go's default test timeout — confirm via Phase 5 task 5-1 that N is in `[3, 9]` per the spec ceiling; worst case ~10 s budget, well within default test timeouts.
```

**Proposed**:
```markdown
**Do**:
- Capture pre-eject baseline: snapshot `<stateDir>/scrollback/` immediately before the external mismatch event. Use the same fingerprint-diff helper from Task 6-4.
- Capture the legitimate daemon's PID from Task 6-3's survivor (this PID equals `_portal-saver`'s pane PID at this point — they are the same process).
- Trigger a saver-pane pid mismatch WITHOUT killing the legitimate daemon process. The mechanism: `tmux break-pane -d -s _portal-saver -t :=_portal-saver-detached` followed by `tmux new-window -t _portal-saver: 'sh -c "exec tail -f /dev/null"'`. The result:
  - The original daemon process is still alive (it was moved out of `_portal-saver` into the detached `_portal-saver-detached` session by `break-pane`).
  - `_portal-saver` now contains a fresh placeholder pane whose pid differs from the daemon's pid.
  - The daemon's next `saverMembershipProbe` tick will observe `pane_pid(_portal-saver) != os.Getpid()` and start incrementing its counter.
  - After `selfSupervisionHysteresisTicks` consecutive failing ticks, the daemon calls `osExit(0)` — the canonical self-eject path being tested.
  - If `break-pane` is unavailable on the test host's tmux version, fall back to: `tmux move-pane -s _portal-saver:0.0 -t _portal-saver-detached:` (creating the destination session first if needed via `tmux new-session -d -s _portal-saver-detached`). The structural outcome is identical: daemon is alive, `_portal-saver`'s pane pid no longer matches the daemon's pid.
- Poll for the daemon's exit via `kill(legitimatePID, 0) == ESRCH` every 100 ms, bounded to `(selfSupervisionHysteresisTicks + 1) * TickerPeriod + 2 s` from the external-mismatch timestamp. Read `selfSupervisionHysteresisTicks` from `cmd/state_daemon.go` (the test lives in a package that can reference the unexported constant directly).
- After observing exit, capture the exit status via `os.Process.Wait` — assert `ProcessState.Exited() == true` AND `ProcessState.ExitCode() == 0` (proves `os.Exit(0)` self-eject, NOT signal-induced death; a SIGKILL or SIGHUP would surface as `Exited() == false` and a non-zero ExitCode via signal).
- Snapshot `<stateDir>/scrollback/` and diff against the pre-eject baseline. Assert byte-identical (no new files, no deletions, no mtime/size/content changes) — verifies `os.Exit(0)` bypassed `daemonShutdownFunc`'s defer chain so no final `captureAndCommit` ran.
- Verify `daemon.pid` is left in place after exit (intentional per Component D — Component C's pre-check handles the stale value on the next acquire). Do NOT delete it; do NOT assert deletion.
- Cleanup: the harness `t.Cleanup` from Task 6-1 will tear down both `_portal-saver` and the synthetic `_portal-saver-detached` session.

**Acceptance Criteria**:
- [ ] External saver-pane pid mismatch is triggered via `tmux break-pane` (or `move-pane` fallback) — NOT via `respawn-pane -k`, which would SIGKILL the daemon and prevent the `os.Exit(0)` self-eject path from being exercised.
- [ ] The legitimate daemon's process is still alive immediately after the external mismatch event (verified via `kill(legitimatePID, 0)` returning nil before the polling begins).
- [ ] Legitimate daemon's process exits within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod + 2 s` of the external-mismatch timestamp.
- [ ] Exit observed via `os.Process.Wait`; `ProcessState.Exited() == true` AND `ProcessState.ExitCode() == 0` (proves `os.Exit(0)`, rules out signal-induced death).
- [ ] Scrollback directory snapshot post-eject is byte-identical to pre-eject baseline (no defer-driven flush ran).
- [ ] `daemon.pid` file remains on disk post-eject (Component D intentional behaviour — no defer cleanup).
- [ ] `selfSupervisionHysteresisTicks` is referenced as the constant from `cmd/state_daemon.go`, not hardcoded — change-resistant.
- [ ] Daemon log contains the INFO line matching the substring `"self-supervision: saver-membership lost for"` (the self-eject log emitted in Task 5-3).
- [ ] Test fails clearly with the measured elapsed time if the daemon does not exit within budget.
- [ ] Test fails clearly with the diff path if scrollback changed across the eject window.

**Tests**:
- `"composite D: legitimate daemon self-ejects via os.Exit(0) within (N+1) tick intervals after external break-pane"`
- `"composite D: scrollback directory byte-identical across self-eject window (no final flush)"`
- `"composite D: daemon.pid left stale after self-eject (handled by Component C pre-check)"`
- `"composite D: self-eject exit code is 0, ProcessState.Exited() == true (rules out signal-induced death)"`
- `"composite D: daemon log records the self-supervision INFO line at eject moment"`

**Edge Cases**:
- The `break-pane` / `move-pane` command itself fails (e.g., `_portal-saver` doesn't exist at the moment of invocation because the post-bootstrap state lost it) — pre-check at the start of this task verifies `_portal-saver` exists AND its pane pid matches the legitimate daemon's pid; if not, fail with "saver session absent or pid mismatch before D test sequence — composition state corrupted".
- The daemon exits faster than expected (e.g., within 1 tick instead of N) — this would indicate `selfSupervisionHysteresisTicks` has been lowered below the measured value; log the actual tick count and pass the test (faster eject is not a failure) but `t.Logf` a notice.
- The daemon does not exit at all within budget — assertion fails with elapsed time; test cleanup kills the daemon explicitly via SIGKILL.
- Scrollback baseline includes a `.bin` that was mid-write at the moment of the external mismatch — atomic-write semantics in the daemon's commit pipeline should prevent partial files; if a mid-write byte appears in the diff, the diff helper logs the path and the test fails (this is a real defect worth surfacing).
- The daemon dies via SIGKILL or another signal rather than self-eject (e.g., `break-pane` implementation accidentally killed the original pane process on this tmux version) — `ProcessState.Exited()` returns false, the test fails clearly with "daemon died via signal, not self-eject" naming the observed signal.
- `selfSupervisionHysteresisTicks` value is large enough that `(N+1) * TickerPeriod` exceeds Go's default test timeout — confirm via Phase 5 task 5-1 that N is in `[3, 9]` per the spec ceiling; worst case ~12 s wall-time (10 s for N=9 plus 2 s slack), well within default test timeouts.
- The synthetic `_portal-saver-detached` session leaks if the test panics before cleanup — the harness `t.Cleanup` from Task 6-1 tears down the tmux server entirely, which kills all sessions on the test socket regardless of name.
```

**Resolution**: Pending
**Notes**: This is the cleanest resolution because (a) it preserves the task's original Component-D scope (no scope expansion to A+D), (b) it leaves Acceptance Criteria, Tests, and Edge Cases internally consistent with the Do, and (c) it provides a single canonical mechanism (`break-pane` with `move-pane` fallback) rather than asking the implementer to choose. The cycle-1 proposed text accepted a scope change that the rest of the task did not absorb; this proposal undoes the scope change and provides a mechanism that actually exercises D's `os.Exit(0)` path in the composite live context.

---
