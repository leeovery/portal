---
phase: 6
phase_name: Composite End-to-End Verification
total: 8
---

## slow-open-empty-previews-and-zombie-sessions-6-1 | approved

### Task 6-1: Build composite test harness with three-daemon scenario setup

**Problem**: The spec mandates exactly one composite integration test that reconstructs the reporter's failure scenario end-to-end and asserts the converged healthy state across A+B+C+D+E+F. Per-component tests cannot catch composition regressions, and Phase 4 task 4-10 only covers the A+B+C subset. The harness for the broader test does not yet exist — every subsequent Phase 6 task (6-2 through 6-8) needs a shared harness that brings up a real tmux server with `_portal-saver` plus user sessions, spawns three `portal state daemon` processes (1 saver-pane legitimate + 2 orphans — one with a `daemon.pid` reference, one without), and tears everything down on exit.

**Solution**: Author a single integration test file with a `setupCompositeHarness(t)` helper that (a) starts a real tmux server via `tmuxtest`, (b) creates `_portal-saver` via the production Phase-3 placeholder-then-respawn flow, (c) spawns 2 orphan daemons as direct subprocesses with parents differing from the saver pane process, (d) seeds at least one user session with non-trivial pane output so scrollback observations later are meaningful, and (e) registers a robust `t.Cleanup` that kills every spawned daemon (SIGKILL + wait) and the tmux server even on assertion failure. Subsequent tasks layer assertions onto the harness.

**Outcome**: A reusable `setupCompositeHarness(t)` returning `(env []string, stateDir string, tmuxSocket string, legitimatePID int, orphanWithPIDFile int, orphanWithoutPIDFile int)` plus a `cleanup` callback registered with `t.Cleanup`. The harness is callable from Tasks 6-2 through 6-8 with no further setup work — each downstream task just adds its own assertion block.

**Do**:
- Create `cmd/bootstrap/composition_full_integration_test.go` (or `internal/restoretest/composition_full_integration_test.go` if `tmuxtest` scaffolding there is more convenient — the planning task list explicitly allows either; pick the one that minimises new dependency imports). Tag with the existing integration build tag pattern used by Phase-4 task 4-10's composition test (`//go:build integration`).
- Author `setupCompositeHarness(t *testing.T)`:
  1. Call `portaltest.NewIsolatedStateEnv(t)` to obtain `env, stateDir`.
  2. Call `portalbintest.BuildPortalBinary(t)` to obtain the binary path; stage via `portalbintest.StagePortalBinary` so `portal` is on `PATH` for child processes.
  3. Start a real tmux server via `tmuxtest` against a per-test socket; pass the isolated env to every `tmux` invocation.
  4. Create `_portal-saver` via the production `BootstrapPortalSaver` (Phase 3 ordering: placeholder `sh -c 'exec tail -f /dev/null'` → `set destroy-unattached=off` → `respawn-pane -k 'portal state daemon'` → readiness barrier). This is the legitimate saver-pane daemon; record its PID via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
  5. Create 2 user tmux sessions (e.g., `usrA`, `usrB`), each with a single pane running `printf 'seed-output\n'; exec sh` so subsequent scrollback observations have content to oscillate over.
  6. Wait until the legitimate daemon has written `daemon.pid` (poll up to 2 s at 50 ms cadence).
  7. Spawn orphan daemon #1 as a direct `exec.Command` subprocess with the isolated env; let it briefly run, then overwrite `daemon.pid` with its own PID (simulating the reporter's "orphan with daemon.pid reference" case). Use `state.WritePIDFile` from the test goroutine to keep the atomic write semantics consistent.
  8. Spawn orphan daemon #2 as a direct subprocess with the isolated env; do NOT touch `daemon.pid` (so this orphan is only visible via `pgrep`). Both orphans must have a parent PID equal to the test process, NOT the saver pane process — this matches the spec's "different parent processes" requirement and makes them unreachable via `tmux kill-session _portal-saver`.
  9. Register `t.Cleanup` that: (i) `kill -9`s `orphanWithoutPIDFile`, `orphanWithPIDFile`, and `legitimatePID` in that order; (ii) waits for each to reap (`os.Process.Wait`); (iii) tears down the tmux server via `tmuxtest` teardown; (iv) leaves the `portaltest.NewIsolatedStateEnv` fingerprint backstop to run last (it is the outermost cleanup — see Task 6-8). All cleanup runs even if earlier assertions failed.
- Add a sentinel constant `compositeHarnessOrphanCount = 2` and use it in pgrep assertions so the test's intent stays explicit.

**Acceptance Criteria**:
- [ ] Test file exists at the chosen path with the integration build tag, and is skipped by default `go test ./...` without `-tags integration`.
- [ ] `setupCompositeHarness(t)` returns successfully under integration tag with all six values populated (env, stateDir, tmuxSocket, legitimatePID, orphanWithPIDFile, orphanWithoutPIDFile).
- [ ] Legitimate daemon's PID equals the `_portal-saver` pane PID from `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
- [ ] `orphanWithPIDFile != legitimatePID` AND `orphanWithoutPIDFile != legitimatePID` AND `orphanWithPIDFile != orphanWithoutPIDFile` (three distinct PIDs).
- [ ] Both orphan PIDs have a parent PID equal to the test process (`os.Getpid()`), not the saver pane process — asserted via `ps -o ppid= -p <pid>`.
- [ ] `daemon.pid` after setup contains `orphanWithPIDFile`'s PID (not the legitimate daemon's PID).
- [ ] `t.Cleanup` runs to completion even when an earlier assertion fails (verified by a deliberate `t.Fatal` in a meta-test that confirms all three daemons are dead on cleanup).
- [ ] Harness uses `portaltest.NewIsolatedStateEnv` for state-dir isolation (Phase 1 helper).
- [ ] No `t.Parallel()` (per project convention — cmd package mocks via package-level state).

**Tests**:
- `"composite harness sets up three distinct portal state daemon PIDs"`
- `"composite harness's orphan parents differ from the saver pane process"`
- `"composite harness writes orphan-with-pidfile's PID into daemon.pid"`
- `"composite harness t.Cleanup kills all spawned daemons even when an assertion fails earlier in the test"`
- `"composite harness skips cleanly when pgrep is unavailable on the test host"`

**Edge Cases**:
- One orphan crashes during setup (e.g., the daemon exits as lock-loser before the test can observe it) — pre-state assertion in Task 6-2 catches this; the harness itself does NOT retry, it surfaces the failure with a clear diagnostic citing which PID died.
- The legitimate daemon writes `daemon.pid` after orphan #1 overwrites it (race) — harness waits for the legitimate `daemon.pid` write FIRST, then sleeps a brief settle window (≥100 ms) before letting orphan #1 overwrite, eliminating the race deterministically.
- `pgrep` not installed on the test host — `t.Skip` with a clear message rather than a false-green run.
- `tmuxtest` socket leaks if test panics — harness uses `t.Cleanup` (which runs on panic too) rather than `defer`, ensuring socket cleanup even on `t.Fatal`.
- Both orphans land on PIDs that, by coincidence, equal the saver pane PID across a recycle — extremely unlikely under low PID pressure; if it occurs, the distinct-PID assertion fails loudly rather than producing a silent false pass.

**Context**:
> Spec § Composite End-to-End Verification scenario setup:
> 1. Start a real tmux server with `_portal-saver` plus some user sessions.
> 2. Spawn three `portal state daemon` processes against the same state directory: one as the legitimate saver-pane process, two as orphans (different parent processes; one with a `daemon.pid` reference, one without).
> 3. Confirm the pre-fix state reproduces.
>
> Phase 4 task 4-10 already covers the A+B+C-scoped subset of this harness — review `cmd/bootstrap/composition_abc_integration_test.go` (or wherever 4-10 landed) and lift shared scaffolding where possible. Do NOT duplicate `tmuxtest` startup or `portalbintest.BuildPortalBinary` wiring; if 4-10 exports a helper, reuse it; otherwise extract during this task.
>
> Phase 3's `BootstrapPortalSaver` already implements the placeholder-then-respawn ordering plus the 2 s / 50 ms readiness barrier — invoke it directly rather than reimplementing the sequence in the harness.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (scenario setup); Phase 1 (portaltest.NewIsolatedStateEnv contract); Phase 3 (BootstrapPortalSaver ordering).

---

## slow-open-empty-previews-and-zombie-sessions-6-2 | approved

### Task 6-2: Assert pre-fix three-daemon state reproduces under harness

**Problem**: Before invoking bootstrap, the composite test must prove that the harness from Task 6-1 actually reproduces the reporter's broken state — three live `portal state daemon` processes against one state directory, with the scrollback directory exhibiting the GC-race oscillation symptom across ticks. Without this pre-assertion the post-bootstrap convergence check is meaningless (we'd have no evidence the broken state was ever there). The spec explicitly calls for confirming the pre-fix reproduction: "Confirm the pre-fix state reproduces: `pgrep -fxc 'portal state daemon'` returns 3, scrollback directory oscillates 0–1 `.bin` file across ticks."

**Solution**: Add a pre-bootstrap assertion block that (a) verifies `pgrep -fxc 'portal state daemon' == 3` using the canonical pgrep form from Phase 4 Component B, and (b) samples the scrollback directory at multiple intervals across at least 3 daemon tick periods, asserting that the `.bin` file set is observably non-stable (oscillates or shows deletions between samples) — direct evidence of the divergent-daemon GC race.

**Outcome**: The composite test fails with a clear diagnostic if the harness fails to reproduce the broken state (e.g., one orphan died during startup, or the daemons aren't actually competing). When it passes, the subsequent post-bootstrap convergence assertion has clear meaning: we went from a confirmed broken state to a confirmed healthy state.

**Do**:
- After `setupCompositeHarness(t)` returns in the composite test body:
  1. Assert `pgrep -fxc 'portal state daemon' == 3` using the canonical form `pgrep -fx '^portal state daemon( |$)'` (matching Phase 4 Component B's enumeration form). Use `exec.Command("pgrep", "-fx", "^portal state daemon( |$)")` and count returned lines; do NOT use `pgrep -fxc` because the test asserts the count via Go and surfaces the actual PIDs on failure.
  2. Verify each of the three expected PIDs (`legitimatePID`, `orphanWithPIDFile`, `orphanWithoutPIDFile`) appears in the pgrep output; failure surfaces the missing PID by name.
  3. Sample the scrollback directory across at least 3 daemon tick intervals to observe the GC race:
     - Sample 0: snapshot `<stateDir>/scrollback/` immediately (path-keyed fingerprint map matching the format from Phase 4 Task 4-2 / Phase 5 Task 5-7).
     - Sleep `TickerPeriod + 200ms` (where `TickerPeriod = 1s` per `cmd/state_daemon.go`).
     - Sample 1: snapshot again.
     - Sleep `TickerPeriod + 200ms`.
     - Sample 2: snapshot again.
     - Sleep `TickerPeriod + 200ms`.
     - Sample 3: snapshot again.
  4. Assert that the four samples are NOT all identical — at least one pairwise diff (Sample N vs Sample N+1) shows a `.bin` deletion or content/size change. This is the oscillation symptom. If all four samples are identical, the harness failed to reproduce the GC race and the test fails with a clear diagnostic ("expected scrollback oscillation across 4 samples spanning ~3.6 s but observed bytes-identical snapshots — three-daemon competition not actually present"). Use the multi-sample approach to avoid the 0-1 boundary flake where a single pre-bootstrap snapshot happens to land on a stable moment.
- Place these assertions in a `requirePreFixState(t, env, stateDir, legitimatePID, orphanWithPIDFile, orphanWithoutPIDFile)` helper so it reads cleanly in the composite test body.

**Acceptance Criteria**:
- [ ] `pgrep -fx '^portal state daemon( |$)'` returns exactly 3 PIDs matching the three harness PIDs (no more, no fewer).
- [ ] Each expected PID is asserted present individually; failure cites the missing PID, not just a count mismatch.
- [ ] Scrollback directory sampled at 4 points across ~3.6 s; at least one pairwise sample diff shows a `.bin` deletion or content/size change.
- [ ] Helper `requirePreFixState` fails the test with a diagnostic citing both the pgrep state and the scrollback oscillation evidence if either invariant breaks.
- [ ] Pre-fix assertion runs BEFORE any bootstrap invocation (Task 6-3) — no false-positive convergence is possible because bootstrap has not yet executed.
- [ ] Test fails loudly (not silently passes) if pgrep yields a count of 1 or 2 rather than 3 — proves the harness setup is wrong, not that the fix already works.

**Tests**:
- `"pre-fix state shows three live portal state daemon PIDs matching the harness"`
- `"pre-fix state shows scrollback directory oscillation across at least one of three consecutive tick intervals"`
- `"pre-fix assertion fails clearly when one orphan has died during setup"`
- `"pre-fix assertion fails clearly when scrollback is stable across all four samples (no GC race present)"`

**Edge Cases**:
- An orphan exits between harness return and the pgrep assertion (e.g., it crashed) — the pgrep count drops to 2; assertion fails with the missing PID identified.
- pgrep returns 4 PIDs because a stray daemon from a prior test leaked into the integration host — the isolated env should preclude this, but if it occurs the assertion fails with the surplus PID listed.
- Scrollback dir is empty for all four samples because the legitimate daemon hasn't ticked yet — harness includes a settle window (Task 6-1 seeds user sessions; daemons should have ticked at least once before this assertion runs); if all four samples are empty, the assertion fails with a "no scrollback activity observed" diagnostic to distinguish this from the "stable scrollback" case.
- The GC race happens to produce only renames (mtime updates) and no deletions during the sampling window — fingerprint diff should still catch mtime changes; if it doesn't, the diff function is faulty (reuse the diff helper from Phase 4 Task 4-2 / Phase 5 Task 5-7 which is mtime-aware).
- Sample timing lands on a 0-1 boundary where all four samples coincidentally happen to be 0 or all 1 — mitigated by the 4-sample window spanning ~3.6 s and the `+200ms` offset breaking exact tick alignment.

**Context**:
> Spec § Composite End-to-End Verification step 3: "Confirm the pre-fix state reproduces: `pgrep -fxc 'portal state daemon'` returns 3, scrollback directory oscillates 0–1 `.bin` file across ticks."
>
> Symptom mechanism: "gcOrphanScrollback race between divergent daemons deleting each other's `.bin` writes; further amplified by the CaptureStructure abort-on-error path when any single session enumeration fails." This is the symptom we expect to observe in the pre-fix snapshots.
>
> The canonical pgrep enumeration form is fixed by Phase 4 Component B: `pgrep -fx '^portal state daemon( |$)'`. Subtle behavioural differences (whitespace splitting, anchor semantics) make alternate forms non-equivalent — use this exact form to match the production sweep's view.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 3 — pre-fix state reproduction); § Symptom → mechanism mapping (empty previews → GC race oscillation).

---

## slow-open-empty-previews-and-zombie-sessions-6-3 | approved

### Task 6-3: Invoke bootstrap and assert pgrep convergence to 1 within 6 s

**Problem**: After the pre-fix assertion proves the broken state, the test must invoke the production bootstrap orchestrator against the new binary and assert that the A+B composition converges the daemon population to exactly one within the 6 s ceiling specified by the spec (A's 5 s session-kill poll + 1 s SIGKILL escalation + B's sub-second sweep). Without measuring from the correct timing anchor (EnsureSaver entry) and asserting that the surviving daemon is the saver-pane process (not just any one of the three), composition regressions could pass silently.

**Solution**: Invoke `Orchestrator.Run` directly (preferred — gives precise timing instrumentation around `EnsureSaver`) or `portal open` as a subprocess if direct invocation is impractical. Start the 6 s budget timer at `EnsureSaver` entry, poll `pgrep -fx '^portal state daemon( |$)'` until it returns exactly one PID, and assert that the surviving PID equals the post-bootstrap `_portal-saver` pane PID.

**Outcome**: The composite test fails if the A+B+F composition fails to converge to exactly one daemon within 6 s, OR if the surviving daemon is not the saver-pane process (e.g., A's escalation accidentally killed the legitimate daemon).

**Do**:
- Capture an `EnsureSaverEntryTime` marker. Two acceptable approaches:
  1. **Preferred — direct orchestrator invocation.** Construct a production `Orchestrator` via the existing `cmd/bootstrap/` wiring (matching the pattern in Phase 4 task 4-10), wrap the `EnsureSaver` step to record `time.Now()` immediately before its `Run`, then invoke `Orchestrator.Run(ctx)`.
  2. **Fallback — subprocess invocation.** Spawn `portal open` as a subprocess with the isolated env. Without direct EnsureSaver instrumentation, capture the timer at the moment `Orchestrator.Run` starts (which is at-most a few ms before EnsureSaver entry — the preceding steps EnsureServer, RegisterPortalHooks, Set `@portal-restoring`, SweepOrphanDaemons are all sub-second). The 6 s budget then becomes a slightly-conservative measurement; document the choice in the test preamble.
- After orchestrator return (or `portal open` exit), poll `pgrep -fx '^portal state daemon( |$)'` every 100 ms until exactly one PID is returned, bounded to 6 s from `EnsureSaverEntryTime`. If the bound is hit with N != 1, fail with the actual measured time and the surviving PIDs.
- Identify the survivor: read `_portal-saver`'s pane PID via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`. Assert this equals the lone surviving PID from pgrep — this rules out the failure mode where the legitimate daemon was accidentally killed and one of the orphans somehow survived.
- Drain any bootstrap warnings (the orchestrator returns an accumulated warning slice — see `cmd/bootstrap/` orchestrator return contract). Log them via `t.Logf` for diagnostic purposes; do NOT fail the test on warnings unless they reference an unexpected error class. Specifically check that NO warnings reference `"no such session: _portal-saver"` (Component F acceptance) and NO warnings reference `"prior daemon did not exit within 5s"` (Component A acceptance).

**Acceptance Criteria**:
- [ ] Timer starts at `EnsureSaver` entry (preferred) or at `Orchestrator.Run` entry (fallback) — the choice is documented in-test.
- [ ] `pgrep -fx '^portal state daemon( |$)'` returns exactly one PID within 6 s of `EnsureSaverEntryTime`.
- [ ] The surviving PID equals `_portal-saver`'s pane PID via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
- [ ] Drained bootstrap warnings do NOT contain `"no such session: _portal-saver"` entries (Component F observable).
- [ ] Drained bootstrap warnings do NOT contain `"prior daemon did not exit within 5s"` entries (Component A escalation succeeded).
- [ ] Test fails with the measured elapsed time if the 6 s budget is exceeded by any margin — do NOT add slack to the budget (matching Phase 4 task 4-10's edge-case treatment).
- [ ] Test fails with the actual vs. expected PIDs if the survivor is not the saver-pane daemon.
- [ ] The new portal binary built by `portalbintest.BuildPortalBinary` is used (not the developer's installed binary).

**Tests**:
- `"composite A+B: three daemons converge to exactly one within 6 s of EnsureSaver entry"`
- `"composite: surviving daemon's PID matches _portal-saver's pane PID (legitimate daemon, not an orphan)"`
- `"composite: bootstrap emits no 'no such session: _portal-saver' warning (Component F)"`
- `"composite: bootstrap emits no 'prior daemon did not exit within 5s' warning (Component A escalation)"`

**Edge Cases**:
- The legitimate daemon dies during bootstrap (A's escalation accidentally kills the wrong PID) — survivor PID assertion catches this and fails with actual vs expected.
- pgrep returns 0 PIDs after bootstrap (everything died, including the saver-pane daemon) — assertion fails with "expected 1, observed 0" and surfaces the cause via the drained warnings.
- Convergence completes in well under 6 s (e.g., 1.5 s because the orphans were SIGKILL-reachable directly) — that's correct behaviour; assert the count, not a lower bound on time.
- Budget exceeded by a small margin (e.g., 6.5 s) — test fails with the measured time; do NOT add slack (matches Phase 4 task 4-10 edge-case handling).
- Subprocess `portal open` is used and it blocks waiting for the TUI to attach — invoke it with an environment variable or flag that exits after bootstrap (e.g., a bootstrap-only test entry point), OR use the direct orchestrator invocation path. Do NOT add a timeout that kills `portal open` mid-bootstrap.
- A new `portal state daemon` spawns AFTER convergence to 1 (e.g., the saver pane crashes and tmux respawns it) — pgrep poll could observe 2 transiently; document that the poll terminates at the first observed N=1 within budget rather than waiting for a stable plateau (the scrollback stability check in Task 6-4 covers post-convergence stability).

**Context**:
> Spec § Composite End-to-End Verification step 5: "`pgrep -fxc 'portal state daemon'` returns 1 within 6 s of bootstrap entering `EnsureSaver` (Component A's escalation budget + Component B's sweep latency)."
>
> Spec § End-State Verification: "Daemon log is quiet under steady state. No `"another daemon holds the lock"` entries, no `"prior daemon did not exit within 5s"` entries, no `"no such session: _portal-saver"` entries."
>
> The 6 s budget is fixed by Phase 4 acceptance and reaffirmed by the composite spec. Tighter budgets are out of scope; looser budgets are evidence of a regression and must fail the test.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 5); § End-State Verification (daemon-log quietness); § Component A (kill-barrier escalation budget); § Component B (sweep latency).

---

## slow-open-empty-previews-and-zombie-sessions-6-4 | approved

### Task 6-4: Assert scrollback directory stability across 10x1 s observations post-bootstrap

**Problem**: Post-bootstrap convergence to a single daemon (Task 6-3) is necessary but not sufficient — if Component E's per-session error tolerance regresses, or if some unforeseen race lets a transient orphan briefly run gc, the scrollback directory could still oscillate. The spec mandates 10 consecutive 1 s observations of the scrollback directory with no `.bin` deletions or unexpected new files (Components A+B+E composition). This catches both the multi-daemon GC race regression and any per-tick capture-pipeline corruption.

**Solution**: Starting after the first post-bootstrap tick has completed (so the legitimate single daemon has had a chance to write its initial scrollback state), snapshot `<stateDir>/scrollback/` every 1 s for 10 consecutive observations. Distinguish legitimate per-tick `.bin` updates (content/mtime changes for sessions that are still capturing) from the failure mode (deletions or unexpected new files). The first observation establishes the baseline path set; subsequent observations must contain the same path set, with content/mtime allowed to evolve.

**Outcome**: The composite test fails if any of the 10 post-bootstrap observations shows a `.bin` deletion or an unexpected new `.bin` file — direct evidence that the GC race symptom would still surface to the user as "empty previews".

**Do**:
- Wait for the first post-convergence tick to complete: after Task 6-3 asserts `pgrep == 1`, sleep `TickerPeriod + 500ms` (≈1.5 s) to let the legitimate daemon write its first uncontested scrollback state.
- Snapshot `<stateDir>/scrollback/`: build a `map[paneKey]fileFingerprint` capturing (existence, size, mtime, SHA-256 ≤1 MiB) for every `.bin` file. Call this `baseline`.
- Loop 10 times: sleep 1 s, snapshot again, diff against `baseline`. Diff rule:
  - **Deletion**: a path present in `baseline` but absent in the current snapshot — FAIL the test with the deleted path.
  - **Unexpected new file**: a path absent from `baseline` but present now — FAIL the test with the new path. (Rationale: the legitimate daemon's per-tick writes update existing `.bin` files in place; new `.bin` files appear only when a new pane is created, which does NOT happen during this passive observation window.)
  - **Content/mtime update on an existing path**: ALLOWED — this is the legitimate single daemon capturing scrollback per tick. Do NOT fail on this.
- After all 10 observations complete without failure, log a summary via `t.Logf` listing the baseline path set and how many of them changed mtime during the window (informational only).
- Reuse the fingerprint-diff helper introduced in Phase 4 Task 4-2 / Phase 5 Task 5-7 so the diff semantics match across the work unit.

**Acceptance Criteria**:
- [ ] Observation window starts after `TickerPeriod + 500ms` post-Task-6-3 convergence (waits for first uncontested tick).
- [ ] 10 consecutive 1 s observations performed; each diffs against the baseline snapshot.
- [ ] Deletion of any `.bin` path present in baseline FAILS the test with the deleted path identified.
- [ ] Unexpected new `.bin` path (not present in baseline) FAILS the test with the new path identified.
- [ ] Content/mtime/size updates on existing paths are ALLOWED (legitimate single-daemon capture activity).
- [ ] The diff distinguishes "update" from "oscillation" — a path that is deleted in observation N then reappears in observation N+1 still counts as a deletion failure (we never want the GC race symptom to flash, even transiently).
- [ ] Reuses the fingerprint-diff helper from Phase 4 Task 4-2 / Phase 5 Task 5-7 (no parallel implementation).
- [ ] Total observation window is approximately 10 s; test runtime budget is respected.

**Tests**:
- `"composite A+B+E: scrollback directory paths stable across 10×1 s observations post-bootstrap"`
- `"composite: per-tick mtime updates on existing .bin files are not flagged as instability"`
- `"composite: transient .bin deletion in any observation window fails the test with the deleted path"`
- `"composite: unexpected new .bin file appearing during passive observation fails the test"`

**Edge Cases**:
- Baseline snapshot is empty because no user sessions had panes with output by the first post-convergence tick — Task 6-1 seeds at least one user session with `printf 'seed-output\n'; exec sh`, so the daemon's first tick should produce at least one `.bin`. If baseline is empty, fail with "scrollback baseline empty after first post-bootstrap tick — capture pipeline may be broken or seed activity insufficient".
- Diff between observations 7 and 8 shows a 0-byte `.bin` size — this is a content update (capture happened to produce empty output for that pane), not a deletion. Existence is the path-level invariant; size of 0 is allowed.
- Observation window crosses a daemon tick boundary precisely at observation time, causing a brief read-during-write — fingerprint helper should tolerate partial reads via the same approach Phase 4 Task 4-2 used (read complete file, retry once on size mismatch between stat and read).
- A new user session is created externally during the 10 s observation window (e.g., a stray test process) — would manifest as an unexpected new `.bin`. The isolated env prevents this in practice; if it occurs, fail clearly so the test surface remains honest.
- `<stateDir>/scrollback/` directory is missing entirely — fail with "scrollback dir does not exist" rather than skipping; missing dir is itself a symptom of the broken state.

**Context**:
> Spec § Composite End-to-End Verification step 6: "Scrollback directory is stable across 10 consecutive 1 s observations — no `.bin` file deletions or unexpected new files (Components A+B+E composition)."
>
> The failure mechanism this test catches: "gcOrphanScrollback race between divergent daemons deleting each other's `.bin` writes" — if A+B regressed and an orphan survived, OR if E regressed and a single bad session aborted capture (collapsing the index to empty and triggering a destructive GC on the surviving sessions' `.bin` files), the stability check would catch it.
>
> Legitimate per-tick updates on existing `.bin` files are expected: the daemon captures pane scrollback every tick, mtime updates are normal. The failure signature is deletions or unexpected new paths, not mtime drift.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 6); § Symptom → mechanism mapping (empty previews); § Component E (CaptureStructure per-session log-and-continue).

---

## slow-open-empty-previews-and-zombie-sessions-6-5 | approved

### Task 6-5: Assert fresh-process AcquireDaemonLock refuses with ErrDaemonLockHeld post-bootstrap

**Problem**: Component C's `daemon.pid` pre-acquire liveness check is the structural defence against the inode-replacement gap that lets divergent daemons coexist. The composite test must verify C's pre-check works against the live post-bootstrap state — not just in unit tests with synthetic state. The fresh-process invocation is essential: it exercises the production pre-check path (read `daemon.pid` → identity-check the recorded PID → return `ErrDaemonLockHeld`) rather than the EWOULDBLOCK fallback path, and it proves that any future `portal state daemon` spawn (e.g., a leaked test fixture, or a manual invocation) would be refused cleanly without destructive coexistence.

**Solution**: After Task 6-4's stability assertion confirms the healthy steady state, invoke `state.AcquireDaemonLock(stateDir)` from a fresh process (not the test goroutine — a separate subprocess that has not previously opened `daemon.lock`) and assert it returns `state.ErrDaemonLockHeld`. The fresh-process aspect matters because the test goroutine itself may already hold inherited fds or have cached state that could mask the pre-check path.

**Outcome**: Component C's pre-check is verified end-to-end against the live converged state, proving that the singleton invariant is enforced by the primary structural defence (not just the bootstrap-time orphan sweep).

**Do**:
- Build a tiny test-only `cmd` helper or use the `portal` binary directly with a debug subcommand if one exists; if not, write a minimal Go program at test time via `t.TempDir()` + `go build` that does:
  ```go
  func main() {
      _, _, err := state.AcquireDaemonLock(os.Getenv("PORTAL_STATE_DIR"))
      if errors.Is(err, state.ErrDaemonLockHeld) {
          os.Exit(42) // sentinel for "refused via pre-check"
      } else if err != nil {
          fmt.Fprintf(os.Stderr, "unexpected error: %v\n", err)
          os.Exit(2)
      }
      os.Exit(0) // unexpected acquire success
  }
  ```
  Build it via `go build` into the test's tempDir, then execute with `PORTAL_STATE_DIR=<stateDir>` and the isolated env.
- Assert the subprocess exits with status 42. If it exits with 0, the pre-check failed to refuse (Component C regression). If it exits with 2 or any other status, log the stderr and fail with the unexpected error.
- Verify the refusal path used the pre-check rather than EWOULDBLOCK: the pre-check returns `ErrDaemonLockHeld` WITHOUT opening `daemon.lock`. To exercise this distinction, the test fixture program could log via stderr whether it observed `daemon.pid` (it should), and the test asserts that line is present. Optional but documented.
- Confirm via `pgrep` immediately before and after the fresh-process invocation that the daemon count remains 1 — the fresh-process invocation must not create a destructive coexistence even transiently.

**Acceptance Criteria**:
- [ ] Fresh-process subprocess built via `go build` (or equivalent) and exec'd from the test — NOT a direct in-test `state.AcquireDaemonLock` call (which could share fds/state).
- [ ] The subprocess exits with the documented sentinel status (42) indicating `errors.Is(err, state.ErrDaemonLockHeld)` succeeded.
- [ ] The subprocess does NOT successfully acquire (exit 0) — that would be a Component C regression.
- [ ] The subprocess exits cleanly without leaving a held `daemon.lock` fd.
- [ ] `pgrep -fx '^portal state daemon( |$)'` count remains 1 before AND after the fresh-process invocation (no destructive coexistence).
- [ ] The legitimate daemon's `daemon.pid` is fresh (post-bootstrap-written, not a stale pre-test value) — verified by reading `daemon.pid` and confirming it matches `_portal-saver`'s pane PID.
- [ ] Test fails clearly if `state.ErrDaemonLockHeld` is unexported or renamed — the sentinel-status pattern surfaces this as exit 2.

**Tests**:
- `"composite C: fresh-process AcquireDaemonLock against live state refuses with ErrDaemonLockHeld"`
- `"composite C: pre-check path exercised, not EWOULDBLOCK fallback (daemon.pid was consulted)"`
- `"composite C: fresh-process refusal does not create destructive coexistence (pgrep count unchanged)"`
- `"composite C: legitimate daemon.pid matches _portal-saver's pane PID after bootstrap convergence"`

**Edge Cases**:
- The fresh-process subprocess inherits the test's `XDG_CONFIG_HOME` accidentally — pass the isolated env explicitly via `exec.Cmd.Env`; do NOT rely on inheritance.
- The Go toolchain is not on `PATH` for the test host (e.g., minimal CI) — `t.Skip` with a clear message if `go build` is unavailable.
- The legitimate daemon dies between Task 6-4 and Task 6-5 (e.g., a flaky tmux command) — `pgrep` pre-check at the start of this task catches this; if the count is already 0, fail with "no legitimate daemon to refuse against" before invoking the subprocess.
- `daemon.pid` is briefly absent because the daemon is mid-tick rewriting it — `state.WritePIDFile` uses `fileutil.AtomicWrite` per the existing helper, so the absent window is bounded by a single rename. In this rare case the pre-check observes "no recorded daemon" and proceeds, then the existing EWOULDBLOCK fallback path inside `AcquireDaemonLock` still returns `ErrDaemonLockHeld` because the legitimate daemon holds the flock. Both outcomes satisfy the test's primary assertion (`errors.Is(err, state.ErrDaemonLockHeld) == true`). The "pre-check path was exercised, not EWOULDBLOCK" sub-assertion is therefore best-effort: if the subprocess's stderr-logged observation reports `daemon.pid` was consulted, the assertion holds; if the pid was briefly absent during the window, log the observation via `t.Logf` and skip the sub-assertion for this run. Do NOT retry the subprocess — the primary assertion is what proves Component C's singleton enforcement.
- The fresh-process subprocess is itself killed by a stray signal (e.g., OS-level OOM) — exit status would not be 42; the test logs the actual exit status and fails.

**Context**:
> Spec § Composite End-to-End Verification step 7: "A subsequent test-bench invocation of `AcquireDaemonLock` from a fresh process refuses with `ErrDaemonLockHeld` (Component C pre-check verifies on the live state)."
>
> Spec § Component C — Pre-check refuses on live recorded daemon: "Given a live identity-checkable `portal state daemon` referenced by `daemon.pid`, `AcquireDaemonLock` returns `ErrDaemonLockHeld` without opening `daemon.lock`."
>
> Phase 4 task 4-10 already verifies this against the A+B+C composition. Phase 6 task 6-5 layers the same assertion onto the FULL A+B+C+D+E+F composition — the value-add is that all six components are simultaneously active, so any composition-induced regression (e.g., Component D's stale-pid-by-design interacting badly with C's pre-check) would surface here.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 7); § Component C (pre-acquire daemon.pid liveness check); § Component D (stale daemon.pid after self-eject is intentional — handled correctly by Component C's pre-check).

---

## slow-open-empty-previews-and-zombie-sessions-6-6 | approved

### Task 6-6: Assert Component D self-eject in live context after external saver-pane kill

**Problem**: Component D (per-tick saver-membership self-check) is verified in isolation by Phase 5's integration tests, but those tests bypass the bootstrap orchestrator and stage the state directory directly. The composite test must verify D fires correctly inside the full A+B+C+E+F composition's live state — specifically, after bootstrap has converged to one daemon, externally killing the legitimate daemon's `_portal-saver` pane must cause the daemon to self-eject within `(selfSupervisionHysteresisTicks + 1)` tick intervals. This proves D's live-context behaviour and verifies that the stale `daemon.pid` it leaves behind composes correctly with Component C's pre-check (Tasks 6-5 already validated the pre-check; this task adds the eject-then-stale-pid sequence).

**Solution**: After Task 6-5's pre-check assertion, externally replace the `_portal-saver` pane process via `tmux respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'` (the canonical placeholder from Phase 3). Observe the legitimate daemon's PID via `os.Process.Wait` (or a polling `kill(pid, 0)` check), bounded to `(selfSupervisionHysteresisTicks + 1) * TickerPeriod`. After the daemon exits, snapshot the scrollback directory immediately and assert it is byte-identical to the pre-eject baseline (no final-flush GC ran).

**Outcome**: Component D's self-eject is verified in the live composite context, including the no-final-flush snapshot invariant. The test fails if the daemon does not exit within the budget, OR if the scrollback directory changed across the eject window.

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

**Context**:
> Spec § Composite End-to-End Verification step 8: "After externally killing the legitimate daemon's `_portal-saver` pane (simulating an out-of-band saver loss), the daemon self-ejects within (N+1) tick intervals (Component D in the live context)."
>
> Spec § Component D — No final flush on self-eject: "Snapshot the scrollback directory at the moment the daemon's self-check first registers a failing tick, and again immediately after `os.Exit(0)`. The two snapshots must be identical (no new files, no deletions, no mtime/size changes)."
>
> Spec § Component D — Stale `daemon.pid` after self-eject is intentional: "`os.Exit(0)` skips any defer that would clean up `daemon.pid`. The stale value is handled correctly by Component C's pre-check on the next acquire."
>
> Phase 5 Task 5-6's `respawn-pane` invocation is the canonical form — consult that test for the exact command and mirror it here.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 8); § Component D (full section, especially Self-check sequence, Hysteresis N, and No-final-flush acceptance); Phase 5 Task 5-6 (canonical respawn-pane form).

---

## slow-open-empty-previews-and-zombie-sessions-6-7 | approved

### Task 6-7: Assert Component F end-state observables on _portal-saver

**Problem**: Component F's saver-creation ordering (placeholder → set `destroy-unattached=off` → respawn to `portal state daemon`) is verified per-component by Phase 3's integration tests, but the composite test must observe the FINAL end-state on `_portal-saver` after the full A+B+C+D+E+F composition has run: (a) the pane process is `portal state daemon` (not the placeholder, not a lock-loser-exited dead process), and (b) `tmux show-options -t _portal-saver destroy-unattached` reports `off`. If F regresses or composes badly with A's escalation or D's self-eject, these observables would surface the regression.

**Solution**: After Task 6-3's convergence assertion and Task 6-5's pre-check assertion (which confirm the legitimate daemon is the saver-pane process), but BEFORE Task 6-6's external kill (which deliberately destroys this state), capture the F observables: (1) read the saver's pane PID via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`, then `ps -o args= -p <pid>` to confirm the process command line is `portal state daemon`; (2) read `tmux show-options -t _portal-saver destroy-unattached` and assert the value is `off` (parsed from the tmux output with quote/whitespace stripping per the `tmuxout` package conventions).

**Outcome**: Component F's end-state is verified in the live composite context. The test fails clearly if the pane process is anything other than `portal state daemon` (e.g., the placeholder leaked through) or if `destroy-unattached` is unset, on, or otherwise misconfigured.

**Do**:
- Place this assertion block AFTER Task 6-3 (convergence) and Task 6-5 (pre-check verification) but BEFORE Task 6-6 (external kill) — the Component F observables only make sense when the legitimate daemon is alive in its saver pane.
- Capture the pane PID: `tmux list-panes -t _portal-saver -F '#{pane_pid}'` (single-line output, parse to int).
- Capture the process args: `ps -o args= -p <pid>` (BSD/Linux compatible). Trim whitespace, then assert the trimmed value equals `portal state daemon` (or has it as a strict prefix tolerating any argv tail). Phase 3 Task 3-5 documents the exact ps form to use — mirror it for byte-identical semantics.
- Capture the option value: `tmux show-options -t _portal-saver destroy-unattached`. Parse via `tmuxout.StripMatchedOuterQuotes` and whitespace trim (the option output format is `destroy-unattached off` or `destroy-unattached "off"` depending on tmux version). Assert the parsed value is exactly `off` (case-sensitive — tmux normalises to lowercase).
- Add a settle window: this task runs AFTER Task 6-3 confirms convergence, which means Phase 3's 2 s readiness barrier already polled `daemon.pid` to existence. No additional sleep is needed, but if the assertion races a transient tmux command failure, allow up to 3 retries at 50 ms cadence (matching the readiness barrier's cadence). Fail clearly if retries are exhausted.

**Acceptance Criteria**:
- [ ] `tmux list-panes -t _portal-saver -F '#{pane_pid}'` returns a single PID that matches the surviving daemon from Task 6-3.
- [ ] `ps -o args= -p <pid>` returns args indicating `portal state daemon` (exact match or strict prefix per Phase 3 Task 3-5's convention).
- [ ] `tmux show-options -t _portal-saver destroy-unattached` parses to exactly `off`.
- [ ] Assertions run after Task 6-3's readiness barrier has succeeded; no additional sleep needed unless retries are required.
- [ ] Assertions run BEFORE Task 6-6's external saver-pane kill (which destroys the F observables).
- [ ] Assertion failure surfaces the actual observed value (pane PID, args, or option value) for diagnosis.
- [ ] Reuses `tmuxout.StripMatchedOuterQuotes` for option-value parsing — no parallel implementation.

**Tests**:
- `"composite F: _portal-saver pane process is portal state daemon after composition"`
- `"composite F: tmux show-options -t _portal-saver destroy-unattached reports off"`
- `"composite F: pane PID matches the convergence survivor from Task 6-3"`
- `"composite F: assertion fails clearly if placeholder leaked through (pane process is sh -c 'exec tail -f /dev/null')"`

**Edge Cases**:
- `_portal-saver` is absent at the moment of the assertion (e.g., destroyed by Component A's escalation accidentally killing the saver session) — fail with "saver session absent during F observable check — Component F or A composition regression".
- `tmux show-options` returns the option value with surrounding quotes that differ across tmux versions — `tmuxout.StripMatchedOuterQuotes` handles this; if the parsed value is `"off"` literally with quotes, the helper is buggy and the test surfaces this.
- The pane process appears as `portal state daemon -flag value` (extra args) — the prefix-match approach tolerates this; only fail if the prefix `portal state daemon` is missing.
- ps shows the process as `[portal] <defunct>` because the daemon has died between Task 6-3 and this task — assertion fails with the actual ps output for diagnosis; document that this indicates an A/D composition regression where the legitimate daemon died unexpectedly.
- tmux 3.x vs 3.5 vs 3.6 output format drift on `show-options` (e.g., key=value vs key value) — Phase 3 Task 3-5's integration test already established the parsing form; mirror it.

**Context**:
> Spec § Composite End-to-End Verification step 9: "`_portal-saver`'s pane process is `portal state daemon` AND `tmux show-options -t _portal-saver destroy-unattached` reports `off` (Component F)."
>
> Spec § Component F — destroy-unattached=off is set before daemon process can exit: "After `BootstrapPortalSaver` returns successfully, `tmux show-options -t _portal-saver destroy-unattached` reports `off`, AND the pane process is `portal state daemon` (verified via `tmux list-panes -t _portal-saver -F '#{pane_pid}'` and `ps -o args= -p <pid>`)."
>
> Phase 3 Task 3-5 already verifies these in isolation. Phase 6 Task 6-7 verifies the same observables AFTER all other components have run (A may have escalated against an orphan, B may have swept, D's tick loop has been running for the duration of the test). If any composition step destabilises F's end-state, this assertion catches it.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (step 9); § Component F (full section, especially "destroy-unattached=off is set before daemon process can exit" acceptance); Phase 3 Task 3-5 (parsing form precedent).

---

## slow-open-empty-previews-and-zombie-sessions-6-8 | approved

### Task 6-8: Assert portaltest cleanup fingerprint backstop reports clean on test exit

**Problem**: Component G's `portaltest.NewIsolatedStateEnv` registers a `t.Cleanup` fingerprint-diff backstop that fails the test if the developer's real `~/.config/portal/state/` was touched during the test. This is the canonical defence against the exact failure mode the bugfix exists to address (leaked test daemon corrupting the developer's live install). The composite test exercises the full daemon lifecycle and could in principle violate the isolation invariant if any subprocess accidentally inherits the developer's `XDG_CONFIG_HOME` or if a test path constructs an unisolated `state.AcquireDaemonLock` call. This task validates that the backstop runs to completion AND reports clean (no developer-state mutations), proving that the composite test itself does not regress the isolation contract.

**Solution**: Ensure the test's `t.Cleanup` callbacks are structured so that (a) the harness teardown (kill daemons, tear down tmux server) completes BEFORE the fingerprint backstop walks the developer's state directory, and (b) no test code writes to the developer's state directory after the assertions but before cleanup. The backstop itself is implemented in Phase 1 — Task 6-8's job is to verify it fires correctly in the composite context and to add a meta-assertion that confirms the cleanup completed without late-write races.

**Outcome**: The composite test exits cleanly with the Phase 1 fingerprint backstop reporting no developer-state mutations. Any leak — e.g., a subprocess that inherited `XDG_CONFIG_HOME` accidentally — fails the test loudly with the leaked path identified.

**Do**:
- In the composite test body, after all assertion blocks (Tasks 6-2 through 6-7) have completed and the test would naturally return, add an explicit `t.Cleanup` registration ordering check. `portaltest.NewIsolatedStateEnv` was called FIRST in Task 6-1's harness, which means its `t.Cleanup` is the OUTERMOST (last to run per LIFO). All harness cleanups (daemon kills, tmux teardown) run BEFORE the fingerprint walk — this is structurally correct but worth documenting in the test preamble.
- Add a meta-assertion at the end of the test body: synchronously call `t.Helper()` plus a marker `t.Logf("composite test body complete — backstop fingerprint walk will follow in t.Cleanup")`. This ensures the assertion phase has completed cleanly; the actual backstop assertion is owned by Phase 1's helper.
- Ensure no test code writes to `~/.config/portal/state/` directly. Audit the test source for any path that could accidentally write to the developer's state dir:
  - All `state.AcquireDaemonLock` calls in the test must take the isolated `stateDir` from `portaltest.NewIsolatedStateEnv` (not a default-path version).
  - All subprocess spawns must use `exec.Cmd.Env = isolatedEnv` (the env returned by `portaltest.NewIsolatedStateEnv`).
  - The fresh-process subprocess in Task 6-5 must pass `PORTAL_STATE_DIR=<isolated>` explicitly AND set `Env` to the isolated env.
- Add a positive control: before the test returns, log via `t.Logf` the contents of `<isolatedStateDir>` (file count and total size) to confirm the test wrote to the isolated dir, not the developer's. This makes a successful backstop result more interpretable.
- Document in the test's preamble comment block that the fingerprint backstop's failure indicates one of: (i) a subprocess inherited the developer's `XDG_CONFIG_HOME`, (ii) a direct file write bypassed the env, or (iii) the helper's snapshot semantics changed; the test does NOT need to disambiguate — the backstop's error message identifies the leaked path.

**Acceptance Criteria**:
- [ ] `portaltest.NewIsolatedStateEnv` is called as the FIRST step in `setupCompositeHarness(t)` (Task 6-1), ensuring its `t.Cleanup` is the outermost (last LIFO).
- [ ] All harness cleanups (daemon kills, tmux teardown) are registered AFTER `NewIsolatedStateEnv` and therefore run BEFORE the fingerprint backstop walk.
- [ ] The test body completes its assertion phase before returning — no late writes (e.g., a deferred file write) that could race the backstop.
- [ ] Every `state.AcquireDaemonLock` call in the test uses the isolated `stateDir`; audit grep `state.AcquireDaemonLock` in the test file confirms zero unisolated call sites.
- [ ] Every subprocess spawn in the test sets `Env` to the isolated env explicitly; audit grep `exec.Command` / `exec.CommandContext` in the test file confirms zero spawns inheriting `os.Environ()` directly.
- [ ] The fresh-process subprocess from Task 6-5 sets both `PORTAL_STATE_DIR` and `Env` to isolated values.
- [ ] A positive control logs the isolated state dir contents at test-body completion for diagnostic visibility.
- [ ] The fingerprint backstop reports clean on test exit (verified by the meta-test `"composite test exits with portaltest fingerprint backstop reporting clean"`).
- [ ] Test preamble documents the three possible backstop failure causes for future debugging.

**Tests**:
- `"composite G: fingerprint backstop reports clean on test exit"`
- `"composite G: every state.AcquireDaemonLock call in the composite test uses the isolated stateDir"`
- `"composite G: every subprocess spawn in the composite test sets Env to the isolated env"`
- `"composite G: harness teardown completes before fingerprint walk runs (cleanup LIFO ordering)"`
- `"composite G (meta): deliberately writing to ~/.config/portal/state/ in a test that uses the helper fails with a clear backstop diagnostic"` (this meta-test lives separately — it is NOT part of the composite test itself, but it validates the backstop's failure path; reference it from the composite test's preamble)

**Edge Cases**:
- A subprocess writes to `~/.config/portal/state/` after the test goroutine has returned but before the subprocess is killed by cleanup — the late-write race. Mitigation: the cleanup kills all subprocesses (SIGKILL + wait) BEFORE the backstop walk runs. Confirm via cleanup ordering: subprocess kill registered in Task 6-1 is INNER to `NewIsolatedStateEnv` registration, so it runs FIRST per LIFO; backstop runs LAST after all subprocesses are dead.
- A `t.Cleanup` callback itself writes to `~/.config/portal/state/` (e.g., a poorly-written cleanup that touches files) — audit cleanup callbacks for writes; none of the harness cleanups should touch the developer's state dir.
- The Phase 1 helper changes its fingerprint snapshot semantics in a future refactor and produces a false positive — the composite test's positive control (logging isolated state dir contents at end) helps diagnose this; if the backstop reports a path that the test never wrote, the bug is in the helper, not the test.
- `~/.config/portal/state/` does not exist on the test host (e.g., a CI runner without prior portal usage) — Phase 1 helper handles this case (empty pre-test snapshot; any post-test file is a delta). Composite test does not need to special-case this.
- The fingerprint backstop is slow (e.g., the developer's state dir has hundreds of `.bin` files) and times out — Phase 1's helper bounds content hashing to files ≤1 MiB; total walk should be sub-second. If observed slowness becomes a problem, raise with the Phase 1 helper, not the composite test.

**Context**:
> Spec § Composite End-to-End Verification — final acceptance: "Test uses `portaltest.NewIsolatedStateEnv` for state-dir isolation (Phase 1 helper); no developer-state mutations on test exit."
>
> Spec § Component G — Helper exists with the documented signature: "Registers `t.Cleanup` to verify on test exit that the developer's real `~/.config/portal/state/` was untouched."
>
> Spec § Component G — Audit grep completion criterion: `grep -rn "exec.Command.*portal\b"` returning zero unisolated call sites. This task applies the same audit discipline to the composite test file itself.
>
> The composite test is the highest-risk site for accidental isolation violations because it spawns the most subprocesses (build helper, three daemons, fresh-process AcquireDaemonLock subprocess, tmux server). Verifying the backstop fires clean here gives high confidence that the rest of the test suite — which spawns fewer subprocesses per test — is also clean.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (final acceptance — "Test uses `portaltest.NewIsolatedStateEnv` ... no developer-state mutations on test exit"); § Component G (full section, especially mtime backstop fires on violation acceptance and audit grep completion criterion).
