---
status: in-progress
created: 2026-05-22
cycle: 1
phase: Plan Integrity Review
topic: Slow Open Empty Previews And Zombie Sessions
---

# Review Tracking: Slow Open Empty Previews And Zombie Sessions - Integrity

## Findings

### 1. Task 5-3 Do section contains unresolved planning deliberation in implementer-facing text

**Severity**: Important
**Plan Reference**: `phase-5-tasks.md` Task 5-3 (Integrate per-tick self-check before captureAndCommit with os.Exit(0) eject), `Do` section.
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The Do section of Task 5-3 contains a stream-of-consciousness deliberation that visibly shifts position mid-instruction:

> Decide counter location: prefer a closure-scoped variable inside `defaultDaemonRun` to avoid expanding `daemonDeps`'s shape. ...
> At the top of `tick(ctx, deps)` — BEFORE the existing `IsRestoringSet` check — call the probe and update the counter. **Rationale for "before IsRestoringSet"**: ...
> Actually — re-read spec: "The check runs **before** the existing `captureAndCommit`". Place the self-check at the **start of `tick`**, before the `restoring` and `dirty`/`gap` early-returns. ...

The "Actually — re-read spec:" phrasing is planning-time deliberation, not implementer instruction. An implementer reading this is left uncertain whether the counter lives inside `tick` or inside the ticker `for` loop in `defaultDaemonRun` — because the prose first puts it "at the top of `tick`" and then in the next paragraph describes the counter as living in the ticker loop scope, even showing pseudo-code with the counter declared outside `tick`. The pseudo-code at the bottom shows the counter at the ticker-loop scope and the probe call interleaved with `tick(ctx, deps)`, which contradicts "at the top of `tick`".

This is the load-bearing wiring step of Component D; the contradiction needs to be resolved into a single canonical instruction before implementation.

**Current**:
```markdown
**Do**:
- Edit `cmd/state_daemon.go`:
  - Decide counter location: prefer a closure-scoped variable inside `defaultDaemonRun` to avoid expanding `daemonDeps`'s shape. The counter is reset on every match and never persisted across daemon process lifetimes.
  - At the top of `tick(ctx, deps)` — BEFORE the existing `IsRestoringSet` check — call the probe and update the counter. **Rationale for "before IsRestoringSet"**: a daemon that has lost saver-membership should self-eject even during a restoring window; staying alive to "respect" `@portal-restoring` is the wrong behaviour when the daemon is structurally an orphan. (If the legitimate daemon is ticking during a real restore, its probe returns true and the counter stays at 0.)
  - Actually — re-read spec: "The check runs **before** the existing `captureAndCommit`". Place the self-check at the **start of `tick`**, before the `restoring` and `dirty`/`gap` early-returns. The `restoring` early-return must not bypass the self-check, otherwise a divergent daemon stuck during a restore window would not self-eject. Document this ordering choice in a code comment.
  - Pseudo-code shape inside the ticker `for` loop of `defaultDaemonRun` (where the counter lives):
    ```go
    var consecutiveAbsenceTicks int
    for {
        select {
        case <-ticker.C:
            if saverMembershipProbe(deps.Client, os.Getpid()) {
                consecutiveAbsenceTicks = 0
            } else {
                consecutiveAbsenceTicks++
                if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks {
                    deps.Logger.Info(state.ComponentDaemon,
                        "self-supervision: saver-membership lost for %d consecutive ticks, exiting",
                        consecutiveAbsenceTicks)
                    osExit(0) // package-level seam over os.Exit
                }
            }
            tick(ctx, deps)
        case <-ctx.Done():
            return daemonShutdownFunc(deps)
        }
    }
    ```
  - Introduce a package-level seam `var osExit = os.Exit` so unit tests can intercept the eject without terminating the test process. Production wires to `os.Exit` directly. Tests overwrite via `t.Cleanup`. The seam takes the same `int` argument as `os.Exit`.
  - Add an INFO-level method on `state.Logger` if one does not exist. Inspect `internal/state/logger.go`: if only `Warn` and `Error` exist, add a sibling `Info` that follows the same shape. The spec uses `INFO` explicitly — do not downgrade to WARN.
  - Code comment immediately above the self-check block, citing:
    - Spec ordering "before captureAndCommit" — and the further ordering choice "before IsRestoringSet early-return" with rationale.
    - Why `os.Exit(0)` (not `return ctx.Err()`-style cancellation): bypasses `daemonShutdownFunc`'s defer chain so no final `captureAndCommit` runs.
    - Why `daemon.pid` is intentionally not deleted (Phase 4 Component C pre-check handles the stale value on next acquire; deleting would be racy against a concurrent pre-check and would invert the layered-enforcement contract).
```

**Proposed**:
```markdown
**Do**:
- Edit `cmd/state_daemon.go`:
  - The self-check lives in the ticker `for` loop of `defaultDaemonRun`, NOT inside `tick`. Place it in the `case <-ticker.C:` arm BEFORE the call to `tick(ctx, deps)`. This satisfies the spec's "before `captureAndCommit`" ordering (because `captureAndCommit` runs inside `tick`) AND ensures the self-check is not skipped by any early-return inside `tick` (notably the `IsRestoringSet` short-circuit) — a divergent orphan must not gain immunity by virtue of a concurrent restore window.
  - The consecutive-failing-tick counter is a closure-scoped variable inside `defaultDaemonRun`, declared immediately before the ticker `for` loop. The counter is reset on every probe-true result and never persisted across daemon process lifetimes.
  - Introduce a package-level seam `var osExit = os.Exit` so unit tests can intercept the eject without terminating the test process. Production wires to `os.Exit` directly. Tests overwrite via `t.Cleanup`.
  - Add an `Info` method on `state.Logger` if one does not exist. Inspect `internal/state/logger.go`: if only `Warn` and `Error` exist, add a sibling `Info` that follows the same shape. The spec uses `INFO` explicitly — do not downgrade to WARN.
  - Pseudo-code shape (canonical):
    ```go
    var consecutiveAbsenceTicks int
    for {
        select {
        case <-ticker.C:
            if saverMembershipProbe(deps.Client, os.Getpid()) {
                consecutiveAbsenceTicks = 0
            } else {
                consecutiveAbsenceTicks++
                if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks {
                    deps.Logger.Info(state.ComponentDaemon,
                        "self-supervision: saver-membership lost for %d consecutive ticks, exiting",
                        consecutiveAbsenceTicks)
                    osExit(0) // package-level seam over os.Exit
                }
            }
            tick(ctx, deps)
        case <-ctx.Done():
            return daemonShutdownFunc(deps)
        }
    }
    ```
  - Add a code comment immediately above the self-check block citing:
    - Spec ordering "before `captureAndCommit`", satisfied by placement before `tick(ctx, deps)` in the ticker arm.
    - Why the self-check is NOT inside `tick`: any early-return inside `tick` (e.g., `IsRestoringSet`) would mask a divergent orphan during a restore window.
    - Why `os.Exit(0)` (not `return ctx.Err()`-style cancellation): bypasses `daemonShutdownFunc`'s defer chain so no final `captureAndCommit` runs.
    - Why `daemon.pid` is intentionally not deleted (Phase 4 Component C pre-check handles the stale value on next acquire; deleting would be racy against a concurrent pre-check and would invert the layered-enforcement contract).
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 6-6 Do section contains unresolved respawn-pane mechanism deliberation

**Severity**: Important
**Plan Reference**: `phase-6-tasks.md` Task 6-6 (Assert Component D self-eject in live context after external saver-pane kill), `Do` section.
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The Do section visibly switches position mid-instruction about how to externally trigger the saver-pane mismatch:

> Execute the external kill: `tmux respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'` ...  This SIGHUPs the saver pane process (the legitimate daemon), but Component D's design intentionally bypasses the SIGHUP handler ...  Actually — `respawn-pane -k` sends SIGKILL by default (via the `-k` flag), so the legitimate daemon will be killed by the respawn itself, not by the self-check. Therefore the canonical D test sequence uses `respawn-pane` WITHOUT `-k` so the original pane process survives and the new pid mismatch triggers the self-check. Re-check Phase 5 task 5-6 for the exact mechanism used there and mirror it.
>   - **Important correction**: per Phase 5 Task 5-6 the mechanism is "daemon spawned then saver pane replaced with sh -c 'exec tail -f /dev/null'" — the daemon's own pid is no longer the saver's pane pid, triggering self-eject. ...

This leaves the implementer with two contradictory instructions and a "consult another task" pointer. The acceptance criteria then say "consult Phase 5 Task 5-6 for the canonical respawn command form" — but Phase 5 Task 5-6 itself prefers the variant "pre-create the saver with the placeholder, then spawn the daemon as a non-saver subprocess", which is NOT applicable in the composite test (the daemon IS the saver-pane process at the start of 6-6). The composite path requires a different mechanism than 5-6 used, and the task is asking the implementer to figure it out.

Resolve to a single canonical mechanism. The correct mechanism in the composite live state is: invoke `tmux respawn-pane -t _portal-saver 'sh -c "exec tail -f /dev/null"'` WITHOUT `-k`, which leaves the original process running but replaces what tmux reports as the pane's command/PID — actually no, `respawn-pane` without `-k` refuses when the pane is still running. The portable approach: use `tmux split-window` + `kill-pane` to migrate the saver session's pane to a new process whose PID differs from the daemon's, OR more simply, use `tmux kill-session _portal-saver && tmux new-session -d -s _portal-saver 'sh -c "exec tail -f /dev/null"' && tmux set-option -t _portal-saver destroy-unattached off` from the test — the daemon survives (parent was test, not the saver pane), and `HasSession` flips false→true with a different pane pid. Either way, the implementer must not have to choose.

**Current**:
```markdown
**Do**:
- Capture pre-eject baseline: snapshot `<stateDir>/scrollback/` immediately before the external kill. Use the same fingerprint-diff helper from Task 6-4.
- Capture the legitimate daemon's PID from Task 6-3's survivor (also equals `_portal-saver`'s pane PID at this point).
- Execute the external kill: `tmux respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'` via the test's tmux socket. This SIGHUPs the saver pane process (the legitimate daemon), but Component D's design intentionally bypasses the SIGHUP handler — the daemon's tick loop catches the saver-membership failure first via `tmux list-panes` returning a pane pid that no longer matches `os.Getpid()`. Actually — `respawn-pane -k` sends SIGKILL by default (via the `-k` flag), so the legitimate daemon will be killed by the respawn itself, not by the self-check. Therefore the canonical D test sequence uses `respawn-pane` WITHOUT `-k` so the original pane process survives and the new pid mismatch triggers the self-check. Re-check Phase 5 task 5-6 for the exact mechanism used there and mirror it.
  - **Important correction**: per Phase 5 Task 5-6 the mechanism is "daemon spawned then saver pane replaced with sh -c 'exec tail -f /dev/null'" — the daemon's own pid is no longer the saver's pane pid, triggering self-eject. The Phase 5 test stages `daemon.pid` with a known-dead PID so Component C's pre-check proceeds; Phase 6 doesn't have that constraint because we want the live `daemon.pid` to remain so Component C's pre-check naturally refuses any subsequent acquire (cross-check with Task 6-5's invariant).
  - Therefore: use the EXACT respawn-pane invocation Phase 5 Task 5-6 uses (consult that file for the canonical form), confirm via observation that the legitimate daemon's process survives the respawn but its `tmux list-panes` query starts returning a mismatched pid, and the self-check increments to N within N ticks.
- Poll for the daemon's exit via `kill(legitimatePID, 0) == ESRCH` every 100 ms, bounded to `(selfSupervisionHysteresisTicks + 1) * TickerPeriod` from the external-kill timestamp. Read `selfSupervisionHysteresisTicks` from `cmd/state_daemon.go` (it's an exported-or-unexported `const`; if unexported, the test lives in the same package and can reference it directly — Phase 5 task 5-9 establishes the constant access pattern).
- After observing exit, snapshot `<stateDir>/scrollback/` and diff against the pre-eject baseline. Assert byte-identical (no new files, no deletions, no mtime/size/content changes) — verifies the no-final-flush invariant.
- Verify `daemon.pid` is left in place after exit (intentional per Component D — Component C's pre-check handles the stale value on the next acquire). Do NOT delete it.
```

**Proposed**:
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
```

**Resolution**: Pending
**Notes**: The current task conflates Component A (SIGKILL-bypasses-final-flush) and Component D (os.Exit(0)-bypasses-final-flush) — both produce the same observable, but the mechanism the test exercises matters. Proposed text scopes the composite assertion to the observable end-state and references Phase 5 for the in-isolation D path.

---

### 3. Task 5-1 minimum-N floor inconsistent between Solution text and Acceptance Criteria

**Severity**: Minor
**Plan Reference**: `phase-5-tasks.md` Task 5-1 (Measure legitimate transient durations and lock in selfSupervisionHysteresisTicks with in-source provenance), `Solution` and `Acceptance Criteria` sections.
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The Solution section says: "Take `N = max(measured_worst_per_scenario) × 2`, clamp to the single-digit-ticks ceiling (≤9), and floor at the spec's starting estimate of 3."

The Acceptance Criteria say: "Value satisfies `3 ≤ selfSupervisionHysteresisTicks ≤ 9` (single-digit-ticks ceiling)."

Task 5-9's lower-bound test, however, only asserts `>= 1`, because the spec explicitly says "actual value-vs-measurement justification is enforced by code review, not test."

The 5-1 acceptance constraint `>= 3` is a self-imposed planning-phase floor without spec backing — the spec only requires "single-digit ticks" ceiling and rejects N=1 in prose, but does not mandate N >= 3 as a hard floor. If empirical measurement returns all-zero worst cases (legitimate steady state), the 2× safety factor would yield N=0, then clamp at 1 (the test's actual lower bound). Forcing N >= 3 has rationale (the spec's "starting estimate of 3") but is not load-bearing relative to the spec.

This isn't a blocker — the floor=3 is a reasonable planning decision — but the Solution text should note that 3 is the *planning-chosen* floor, not a spec mandate, so a future maintainer re-running the harness understands why they can't go lower than 3 unless they amend the plan.

**Current**:
```markdown
- Compute `N = clamp(ceil(max_observed × 2), 3, 9)`. If `max_observed × 2 > 5` flag in the memo as "evidence of upstream defect" per the Risk Summary, but still pick the clamped value.
```

**Proposed**:
```markdown
- Compute `N = clamp(ceil(max_observed × 2), 3, 9)`. The floor of 3 is the spec's starting-estimate value (Component D, "Hysteresis N: 3 consecutive ticks" rationale) — NOT the spec's hard minimum. The spec's hard minimum is N >= 1 (per Task 5-9). The floor-of-3 is chosen at planning time to give the legitimate daemon at least one safety tick of headroom over the spec's stated "single tmux-command hiccup" failure mode. If a future re-measurement makes the case to lower the floor, update both this task and the in-source comment in the same commit. If `max_observed × 2 > 5` flag in the memo as "evidence of upstream defect" per the Risk Summary, but still pick the clamped value.
```

**Resolution**: Pending
**Notes**:

---

### 4. Phase 1 Task 1-4 audit deliverable path missing from plan-tree layout

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` Task 1-4 (Audit and migrate existing test helpers to isolated env), `Do` section.
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-4 prescribes "produce `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/audit-G-test-helpers.md`". This is a peer file to the planning.md and per-phase task files. The plan's other tasks reference `.workflows/...` paths consistently, and this is fine — but the file path is buried in the `Do` section and not surfaced anywhere else. An implementer scanning the Acceptance Criteria can find it (the criteria do mention the path), so this is a no-op finding — verified.

After re-reading, the acceptance criteria DO surface the path: "Audit file exists at `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/audit-G-test-helpers.md`...". No change needed.

**Current**:
n/a — this finding withdrawn after closer reading.

**Proposed**:
n/a

**Resolution**: Withdrawn (no change required — the path is in the Acceptance Criteria as required).
**Notes**: Left in tracking file for cycle audit completeness.

---

### 5. Task 4-4 nil-OrphanSweeper handling left undecided

**Severity**: Minor
**Plan Reference**: `phase-4-tasks.md` Task 4-4 (Wire SweepOrphanDaemons as orchestrator step 4), `Acceptance Criteria` and `Edge Cases`.
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The acceptance criteria say:

> `nil OrphanSweeper field panics on Run (consistent with existing nil-interface treatment of other steps)` — or, alternative, gate behind a noop default; match the convention used for other required steps.

The Edge Cases section then says:

> `OrphanSweeper` field is nil at orchestrator construction time — match the convention used for the other mandatory step fields (existing code dereferences without nil-guard; production wiring is responsible for populating). If the convention is to noop-default, follow it. Document the choice in the field's doc comment.

This is an open decision presented to the implementer. The other 9 step fields in `cmd/bootstrap/bootstrap.go` (Server, Hooks, Restoring, Saver, Restorer, Hydrator, Cleanup, MarkerCleaner, FIFOSweeper) follow a single convention — the implementer just needs to mirror it. The plan should state explicitly which convention is used (or instruct the implementer to follow whatever the prevailing one is, without alternatives) so the test design is locked in.

**Current**:
```markdown
**Acceptance Criteria**:
- [ ] `Orchestrator.OrphanSweeper` field exists and is invoked exactly once in `Run`, between `Restoring.Set()` and `Saver.EnsureSaver()`.
- [ ] Orchestrator package docstring documents 11 steps with the new step 4.
- [ ] All debug-log step labels match the new numbering (`"step N (Name): entering"` for N=4..11).
- [ ] Production adapter `NewOrphanSweeper` exists in `internal/bootstrapadapter` and wires real `pgrep` / `tmux list-panes` / `state.IdentifyDaemon` / `syscall.Kill`.
- [ ] Production wiring in the orchestrator construction site (root.go / bootstrap_production.go) populates `OrphanSweeper`.
- [ ] `CLAUDE.md` "Server bootstrap" section reflects 11-step ordering.
- [ ] On step error: WARN logged, orchestrator continues — never aborts and never produces a fatal `*FatalError`.
- [ ] Existing orchestrator unit tests (e.g., `cmd/bootstrap/bootstrap_test.go`) updated to inject a no-op `OrphanSweeper` and continue to pass.
```

**Proposed**:
```markdown
**Acceptance Criteria**:
- [ ] `Orchestrator.OrphanSweeper` field exists and is invoked exactly once in `Run`, between `Restoring.Set()` and `Saver.EnsureSaver()`.
- [ ] Orchestrator package docstring documents 11 steps with the new step 4.
- [ ] All debug-log step labels match the new numbering (`"step N (Name): entering"` for N=4..11).
- [ ] Production adapter `NewOrphanSweeper` exists in `internal/bootstrapadapter` and wires real `pgrep` / `tmux list-panes` / `state.IdentifyDaemon` / `syscall.Kill`.
- [ ] Production wiring in the orchestrator construction site (root.go / bootstrap_production.go) populates `OrphanSweeper`.
- [ ] `CLAUDE.md` "Server bootstrap" section reflects 11-step ordering.
- [ ] On step error: WARN logged, orchestrator continues — never aborts and never produces a fatal `*FatalError`.
- [ ] Existing orchestrator unit tests (e.g., `cmd/bootstrap/bootstrap_test.go`) updated to inject a no-op `OrphanSweeper` and continue to pass.
- [ ] Nil-field handling matches the prevailing convention for other mandatory step fields in `Orchestrator` (Server, Hooks, Restoring, Saver, Restorer, Hydrator, Cleanup, MarkerCleaner, FIFOSweeper). The implementer reads one of these existing fields' Run-time treatment and mirrors it for `OrphanSweeper`; the field's doc comment records the convention used.
```

**Resolution**: Pending
**Notes**: Edge Cases section already gives this guidance correctly; the change pulls it into the load-bearing Acceptance Criteria so it's not interpretive.

---

### 6. Task 6-5 Edge Cases describes `daemon.pid` absent window as racing pre-check vs EWOULDBLOCK without resolution guidance

**Severity**: Minor
**Plan Reference**: `phase-6-tasks.md` Task 6-5 (Assert fresh-process AcquireDaemonLock refuses with ErrDaemonLockHeld post-bootstrap), `Edge Cases`.
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The Edge Cases section says:

> `daemon.pid` is briefly absent because the daemon is mid-tick rewriting it — `state.WritePIDFile` uses `fileutil.AtomicWrite` per the existing helper, so the absent window is bounded by a single rename — unlikely but if observed, the pre-check would treat it as "no recorded daemon" and proceed to acquire, which would then hit EWOULDBLOCK. This is correct behaviour; the test does NOT need to disambiguate the pre-check vs EWOULDBLOCK path beyond logging.

But the Acceptance Criteria say the subprocess "MUST refuse via the pre-check path" and the Tests include "pre-check path exercised, not EWOULDBLOCK fallback (daemon.pid was consulted)". These are in tension: the edge-case says the test does not need to disambiguate, but a top-line test asserts the pre-check path was used. If the daemon happens to be mid-rename when the subprocess runs, the test would fail not because Component C regressed but because of normal AtomicWrite timing.

Either (a) the test should retry the subprocess on EWOULDBLOCK to catch the pre-check path on a stable observation, or (b) the test should accept EITHER ErrDaemonLockHeld outcome (pre-check or EWOULDBLOCK) since both prove singleton enforcement. Pick one.

**Current**:
```markdown
- The legitimate daemon dies between Task 6-4 and Task 6-5 (e.g., a flaky tmux command) — `pgrep` pre-check at the start of this task catches this; if the count is already 0, fail with "no legitimate daemon to refuse against" before invoking the subprocess.
- `daemon.pid` is briefly absent because the daemon is mid-tick rewriting it — `state.WritePIDFile` uses `fileutil.AtomicWrite` per the existing helper, so the absent window is bounded by a single rename — unlikely but if observed, the pre-check would treat it as "no recorded daemon" and proceed to acquire, which would then hit EWOULDBLOCK. This is correct behaviour; the test does NOT need to disambiguate the pre-check vs EWOULDBLOCK path beyond logging.
- The fresh-process subprocess is itself killed by a stray signal (e.g., OS-level OOM) — exit status would not be 42; the test logs the actual exit status and fails.
```

**Proposed**:
```markdown
- The legitimate daemon dies between Task 6-4 and Task 6-5 (e.g., a flaky tmux command) — `pgrep` pre-check at the start of this task catches this; if the count is already 0, fail with "no legitimate daemon to refuse against" before invoking the subprocess.
- `daemon.pid` is briefly absent because the daemon is mid-tick rewriting it — `state.WritePIDFile` uses `fileutil.AtomicWrite` per the existing helper, so the absent window is bounded by a single rename. In this rare case the pre-check observes "no recorded daemon" and proceeds, then the existing EWOULDBLOCK fallback path inside `AcquireDaemonLock` still returns `ErrDaemonLockHeld` because the legitimate daemon holds the flock. Both outcomes satisfy the test's primary assertion (`errors.Is(err, state.ErrDaemonLockHeld) == true`). The "pre-check path was exercised, not EWOULDBLOCK" sub-assertion is therefore best-effort: if the subprocess's stderr-logged observation reports `daemon.pid` was consulted, the assertion holds; if the pid was briefly absent during the window, log the observation via `t.Logf` and skip the sub-assertion for this run. Do NOT retry the subprocess — the primary assertion is what proves Component C's singleton enforcement.
- The fresh-process subprocess is itself killed by a stray signal (e.g., OS-level OOM) — exit status would not be 42; the test logs the actual exit status and fails.
```

**Resolution**: Pending
**Notes**: Also requires aligning the Tests list — see the line `"composite C: pre-check path exercised, not EWOULDBLOCK fallback (daemon.pid was consulted)"` which becomes best-effort. The Acceptance Criteria entry that says "Pre-check path exercised" should be downgraded to "Pre-check path exercised (best-effort — falls back to EWOULDBLOCK in rare AtomicWrite-mid-rename windows; both outcomes return ErrDaemonLockHeld and satisfy the primary assertion)" — but the proposed text already documents this. The implementer should update the Tests list and Acceptance Criteria entry consistently with the Edge Case wording above.

---

### 7. Phase 2 Task 2-2 deliberate horizontal slice — annotation worth flagging

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` Task 2-2 (Thread `*state.Logger` parameter into `CaptureStructure` (no behaviour change)).
**Category**: Vertical Slicing
**Change Type**: update-task

**Details**:
Task 2-2 is explicitly a horizontal slice — signature plumbing with no behaviour change, deferring the actual usage to Task 2-3. The task self-documents this:

> No behavioural change in this task — the new parameter is accepted and stored locally but no `logger.Warn` call is added yet (that arrives in Task 2.3). Keep the diff minimal.

Per `task-design.md`, this is suboptimal vertical slicing, but the deliberate decoupling has a defensible rationale (diff hygiene — separate "signature churn" from "behaviour change"). However, the Tests block reads:

> `"existing CaptureStructure unit tests continue to pass when invoked with a nil logger"` — the existing suite at `internal/state/capture_test.go` is the regression backstop; no new test is required for this task (the behaviour is unchanged).
> `"CaptureStructure compiles with a *state.Logger argument"` — implicit via `go build`; no explicit test needed.

This means the task has no own-test cycle — it's purely a refactor task. Per task-design.md, "If you struggle to articulate a clear Problem for a task, this signals the task may be ... Too granular: Merge with a related task." This task is on the edge of "too small" but the Problem statement does justify its existence (sequence small TDD cycles, isolate concerns).

Recommendation: keep the task as-is, but acknowledge in the task it's a deliberate refactor cycle without an own test — this prevents an implementer or future reviewer from flagging the missing tests as a problem.

**Current**:
```markdown
**Tests**:
- `"existing CaptureStructure unit tests continue to pass when invoked with a nil logger"` — the existing suite at `internal/state/capture_test.go` is the regression backstop; no new test is required for this task (the behaviour is unchanged).
- `"CaptureStructure compiles with a *state.Logger argument"` — implicit via `go build`; no explicit test needed.
```

**Proposed**:
```markdown
**Tests**:

This is a deliberate refactor cycle (signature plumbing) with no behavioural change and therefore no own-test addition. The existing CaptureStructure unit suite at `internal/state/capture_test.go` serves as the regression backstop: it must continue to pass after the signature update, with every test invocation passing `nil` as the trailing `*state.Logger`. `go build ./...` covers the type-level assertion that `CaptureStructure` accepts `*state.Logger` as its new trailing parameter. The behavioural assertion (per-session WARN on error) is owned by Task 2.3.

- `"existing CaptureStructure unit tests continue to pass when invoked with a nil logger"` — regression backstop via the existing test suite.
- `"CaptureStructure compiles with a *state.Logger argument"` — type-level assertion via `go build`.
```

**Resolution**: Pending
**Notes**: This is a polish change to make the task's deliberate refactor-cycle nature explicit, so a future plan-reviewer doesn't flag "no tests" as a defect.

---
