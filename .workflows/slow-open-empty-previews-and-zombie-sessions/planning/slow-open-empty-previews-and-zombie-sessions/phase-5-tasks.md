---
phase: 5
phase_name: Daemon Self-Supervision (Component D)
total: 9
---

## slow-open-empty-previews-and-zombie-sessions-5-1 | approved

### Task 5-1: Measure legitimate transient durations and lock in selfSupervisionHysteresisTicks with in-source provenance

**Problem**: Component D's hysteresis constant `N` is the only tuning knob in this bugfix, and the Risk Summary marks empirical measurement of legitimate `_portal-saver` transient durations as a **required** mitigation (not optional). Locking `N` without measurement risks two failure modes: too low → false-positive self-eject of the legitimate daemon during a normal transient; too high → orphan-daemon lifetime drifts beyond the spec's "single-digit ticks" ceiling and the inter-bootstrap orphan window stays wide. The spec further mandates that the chosen `N` carry **in-source provenance** — a comment block citing measured worst-case transient ticks across four specific scenarios, the 2× safety factor, and the measurement date + binary version — so future maintainers can re-validate without rediscovering the rationale.

**Solution**: Run a measurement harness against a real tmux server (via `tmuxtest`) that exercises the four scenarios the spec enumerates — steady-state ticking, attach/detach cycles, hook-driven `client-attached` events, and the bootstrap kill-and-recreate sequence — and records, for each scenario, the maximum number of consecutive ticks during which a hypothetical self-check would have observed a failing saver-membership condition (saver absent OR saver pane pid != current pid). Take `N = max(measured_worst_per_scenario) × 2`, clamp to the single-digit-ticks ceiling (≤9), and floor at the spec's starting estimate of 3. Persist a measurement memo at `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/component-d-hysteresis-measurement.md` capturing per-scenario raw numbers, the binary version (from `cmd.version` ldflag), the measurement date, the tmux version, and the OS. Introduce the constant `selfSupervisionHysteresisTicks` in `cmd/state_daemon.go` with a comment block that (a) names the four scenarios and their measured worst-case tick counts, (b) cites the 2× safety factor and resulting `N`, (c) records the measurement date and binary version, and (d) references the memo path.

**Outcome**: `cmd/state_daemon.go` exports/uses a constant `selfSupervisionHysteresisTicks` whose value is justified by an in-source comment that traces back to a committed measurement memo. The constant satisfies `3 ≤ selfSupervisionHysteresisTicks ≤ 9`. Subsequent tasks (5-3, 5-5, 5-6, 5-7, 5-8) consume this constant as the eject threshold and as the "(N+1) tick intervals" budget in their assertions.

**Do**:
- Author a measurement harness (a `go test` with the existing integration build tag, e.g., `cmd/state_daemon_hysteresis_measurement_test.go`, gated so it can run on demand but is not part of the default `go test ./...` budget if expensive — follow the existing integration tag pattern in the repo).
- The harness, for each of the four scenarios, spawns a real `portal state daemon` subprocess (via `portalbintest.BuildPortalBinary` + `portaltest.IsolateStateForTest` so the developer state dir is untouched), drives the scenario, and records — at the granularity of the daemon's `TickerPeriod` (currently `1 * time.Second` in `cmd/state_daemon.go:333`) — how many consecutive ticks would have observed saver-membership failure. Use a polling probe that mirrors what `saverMembershipProbe` will check in Task 5-2: `tmux has-session -t _portal-saver` plus `tmux list-panes -t _portal-saver -F '#{pane_pid}'` compared against the daemon subprocess's PID.
- Scenarios:
  1. **Steady-state**: daemon running inside a healthy `_portal-saver` for ≥30 s, no external interaction. Expected worst-case ≈ 0 ticks.
  2. **Attach/detach**: attach to a user session and detach repeatedly while the daemon runs. Expected worst-case ≈ 0 ticks.
  3. **`client-attached` hook**: trigger the registered `client-attached` hook (`portal state signal-hydrate`) by attaching a client. Expected worst-case = transient duration of the hook command, in ticks.
  4. **Bootstrap kill-and-recreate**: run `BootstrapPortalSaver`'s unhealthy-saver recreate path against a live setup (composes with Phase 3's placeholder-then-respawn ordering). Measure the count of consecutive ticks during which a new daemon (hypothetically existing during the gap) would have observed absence-or-mismatch.
- Write per-scenario raw measurements (min/max/median across at least 5 runs per scenario) to `component-d-hysteresis-measurement.md` under `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/`. Include tmux version (`tmux -V`), OS (`uname -srm`), binary version (the value of the ldflag-injected `cmd.version`), date.
- Compute `N = clamp(ceil(max_observed × 2), 3, 9)`. The floor of 3 is the spec's starting-estimate value (Component D, "Hysteresis N: 3 consecutive ticks" rationale) — NOT the spec's hard minimum. The spec's hard minimum is N >= 1 (per Task 5-9). The floor-of-3 is chosen at planning time to give the legitimate daemon at least one safety tick of headroom over the spec's stated "single tmux-command hiccup" failure mode. If a future re-measurement makes the case to lower the floor, update both this task and the in-source comment in the same commit. If `max_observed × 2 > 5` flag in the memo as "evidence of upstream defect" per the Risk Summary, but still pick the clamped value.
- Add the constant near the top of `cmd/state_daemon.go` (above `defaultDaemonRun`):
  ```go
  // selfSupervisionHysteresisTicks is Component D's consecutive-failing-tick threshold.
  // Measured worst-case transients (date YYYY-MM-DD, binary vX.Y.Z, tmux V):
  //   steady-state:           N1 ticks
  //   attach/detach:          N2 ticks
  //   client-attached hook:   N3 ticks
  //   bootstrap kill/recreate:N4 ticks
  // 2x safety factor applied; clamped to single-digit ticks.
  // Memo: .workflows/.../component-d-hysteresis-measurement.md
  const selfSupervisionHysteresisTicks = N
  ```
- Do NOT introduce a runtime override (env var or flag) for this constant — it is compile-time per spec; only Task 5-9's unit test guards the lower bound.

**Acceptance Criteria**:
- [ ] Constant `selfSupervisionHysteresisTicks` exists in `cmd/state_daemon.go` as a `const` (compile-time literal, not a `var`).
- [ ] Comment block above the constant names all four scenarios with their measured tick counts, the 2× safety factor calculation, the measurement date, the binary version, and the memo path.
- [ ] Measurement memo committed at `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/component-d-hysteresis-measurement.md` with per-scenario raw numbers, tmux version, OS, binary version, date.
- [ ] Value satisfies `3 ≤ selfSupervisionHysteresisTicks ≤ 9` (single-digit ticks ceiling).
- [ ] If `max_observed × 2 > 5`, the memo flags this as evidence of upstream defect per Risk Summary guidance.
- [ ] Measurement harness uses `portaltest.IsolateStateForTest` (Phase 1) — no developer-state mutations on test exit.
- [ ] Measurement harness uses `portalbintest.BuildPortalBinary` and `tmuxtest` real-tmux scaffolding — no mocks for tmux or the daemon binary in the measurement runs.

**Tests**:
- The measurement harness itself doubles as a re-runnable verification artifact. The harness emits scenario worst-case values and asserts each is `≤ selfSupervisionHysteresisTicks / 2` (the safety-factor invariant). Re-running the harness against a future code change that worsens transients fails loudly.
- No separate unit test in this task — the value-vs-measurement justification is enforced by code review (per spec), and the lower-bound guard is owned by Task 5-9.

**Edge Cases**:
- Scenario 3 (`client-attached` hook) is bounded by `portal state signal-hydrate` latency — record raw values even if zero, the comment block must list all four scenarios.
- Scenario 4 measures the gap during bootstrap kill-and-recreate; with Phase 3's placeholder-then-respawn already shipped, this should be very short (<1 tick), but record it anyway.
- If running on CI without an interactive tmux client, scenario 2 (attach/detach) can be simulated via `tmux attach -d` from a parallel goroutine; document the simulation in the memo.
- If a future implementer re-runs the harness and gets a larger worst-case, they MUST re-update both the memo and the in-source comment in the same commit — this task establishes that audit-trail expectation.

**Context**:
> Spec § Component D — Daemon Self-Supervision Against the Saver Session, "Hysteresis N: 3 consecutive ticks" rationale and "Measurement artefact for N" acceptance criterion.
>
> Spec § Risk Summary: "Component D's hysteresis (N=3) is the only tuning knob. Planning phase **MUST** empirically measure the legitimate `_portal-saver` create/recreate transient duration before locking N — this is a required mitigation, not optional."
>
> Spec further: "Target ceiling remains 'single-digit ticks' — if the measured transient exceeds ~5 ticks, treat that as evidence of an upstream defect (e.g., slow tmux command latency or a real recreate-spanning-window) rather than tuning N higher."
>
> The daemon's `TickerPeriod` is currently `1 * time.Second` (cmd/state_daemon.go:333). "(N+1) tick intervals" in subsequent tasks references this same constant — i.e., approximately `(N+1) * 1s`.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D — Daemon Self-Supervision Against the Saver Session; § Risk Summary.

## slow-open-empty-previews-and-zombie-sessions-5-2 | approved

### Task 5-2: Extract saverMembershipProbe seam and add tmux.SaverPanePID helper

**Problem**: Component D's self-check needs to call `tmux has-session -t _portal-saver` and `tmux list-panes -t _portal-saver -F '#{pane_pid}'` every tick and compare the pane pid to `os.Getpid()`. The existing `*tmux.Client` exposes `HasSession(name string) bool` (`internal/tmux/tmux.go:126`) but no helper for fetching a specific session's single-pane pid. Without a seam, the unit test for counter-reset behaviour (Task 5-4) cannot stub the membership signal without taking a hard dependency on a real tmux server or the `Commander` interface at a level deeper than the rest of `cmd/state_daemon.go` tests use.

**Solution**: Introduce two artefacts:
1. **`tmux.SaverPanePID(c *Client, sessionName string) (int, error)`** in `internal/tmux/` — a thin helper that shells out to `tmux list-panes -t {sessionName} -F '#{pane_pid}'`, parses the first line, returns the integer pid, and surfaces classified errors (no such session → wraps `tmux.ErrNoSuchSession` from Phase 2; empty output → distinct sentinel; parse failure → distinct error). Production use is "ask for `_portal-saver`'s pane pid"; the function signature stays generic for testability and future reuse (e.g., the bootstrap orchestrator's Component B already builds the legitimate set from the same tmux query — that wiring is owned by Phase 4 Task 4-3 and is NOT changed here).
2. **`saverMembershipProbe` seam** in `cmd/state_daemon.go` — a package-level function variable with signature `func(c *tmux.Client, selfPID int) (membershipOK bool)` that production-wires to a default implementation calling `c.HasSession(tmux.PortalSaverName)` and `tmux.SaverPanePID(c, tmux.PortalSaverName)`. Tests overwrite the seam via `t.Cleanup` to inject deterministic absent/present/mismatch sequences.

**Outcome**: The daemon's self-check (Task 5-3) calls `saverMembershipProbe(deps.Client, os.Getpid())` once per tick. Tests for counter reset (5-4) stub the seam to return scripted sequences. The `tmux.SaverPanePID` helper is reusable, has its own unit-test coverage in `internal/tmux/`, and classifies failure modes consistently with Phase 2's `tmux.ErrNoSuchSession` sentinel.

**Do**:
- Add `tmux.SaverPanePID` in `internal/tmux/portal_saver.go` (or a new file `internal/tmux/saver_pane_pid.go` if cohesion improves) with the signature `func SaverPanePID(c *Client, sessionName string) (int, error)`.
  - Implementation: `c.commander.Run("tmux", "list-panes", "-t", "="+sessionName, "-F", "#{pane_pid}")` (use the existing exact-match `=` prefix convention seen elsewhere in `tmux.go`).
  - Parse the first line of stdout as `strconv.Atoi`.
  - If stderr contains `"no such session"` wrap with `tmux.ErrNoSuchSession` from Phase 2.
  - If stdout is empty (no pane?), return a wrapped `ErrEmptyPaneList` sentinel (new) — distinct from `ErrNoSuchSession`.
  - If parse fails, return a wrapped `ErrPanePIDParse` sentinel (new).
  - Multi-line stdout: take the first non-empty line; defensively log via the caller (helper does not log).
- Add `saverMembershipProbe` to `cmd/state_daemon.go` near the other seam vars (`daemonRunFunc`, `daemonShutdownFunc`, `acquireDaemonLock`):
  ```go
  // saverMembershipProbe answers: is this process still the legitimate
  // _portal-saver pane process? Production wires it to a thin wrapper over
  // HasSession + tmux.SaverPanePID. Tests overwrite it via t.Cleanup.
  var saverMembershipProbe = defaultSaverMembershipProbe
  ```
  Default implementation:
  ```go
  func defaultSaverMembershipProbe(c *tmux.Client, selfPID int) bool {
      if !c.HasSession(tmux.PortalSaverName) {
          return false
      }
      pid, err := tmux.SaverPanePID(c, tmux.PortalSaverName)
      if err != nil {
          return false
      }
      return pid == selfPID
  }
  ```
  Note: any tmux error (including `ErrNoSuchSession` from a race between `HasSession` and `SaverPanePID`) is treated as "absent" per spec — "Treat any error (not just 'session not found') as 'absent' for this tick — tmux command failures are evidence the daemon's view is unreliable."
- Do NOT integrate the probe into the tick loop in this task — Task 5-3 owns integration. This task ships the seam, the default implementation, and the helper, plus unit tests for `tmux.SaverPanePID`.

**Acceptance Criteria**:
- [ ] `tmux.SaverPanePID(*Client, string) (int, error)` exists with the documented error classification (wraps `ErrNoSuchSession`, returns distinct sentinels for empty pane list and parse failure).
- [ ] `tmux.SaverPanePID` unit tests use the existing `Commander` mock pattern in `internal/tmux/` and cover: success, no-such-session-wraps-ErrNoSuchSession, empty-pane-list, parse-failure, multi-line-takes-first, generic-exec-error.
- [ ] `saverMembershipProbe` seam exists in `cmd/state_daemon.go` with the documented signature; production wiring points at `defaultSaverMembershipProbe`.
- [ ] `defaultSaverMembershipProbe` returns `false` on any error path (not just no-such-session), per spec.
- [ ] `defaultSaverMembershipProbe` returns `true` only when `HasSession` is true AND `SaverPanePID` returns `pid == selfPID` with no error.
- [ ] No call to the probe from the tick loop in this task (integration is Task 5-3).

**Tests**:
- `internal/tmux/saver_pane_pid_test.go`:
  - `"SaverPanePID returns pid on healthy session"`
  - `"SaverPanePID wraps ErrNoSuchSession when stderr says no such session"`
  - `"SaverPanePID returns ErrEmptyPaneList on empty stdout"`
  - `"SaverPanePID returns ErrPanePIDParse on non-numeric stdout"`
  - `"SaverPanePID takes the first line when stdout has multiple panes"`
  - `"SaverPanePID returns generic exec error for unrecognized tmux failures"`
- `cmd/state_daemon_test.go`:
  - `"defaultSaverMembershipProbe returns false when HasSession is false"`
  - `"defaultSaverMembershipProbe returns false when SaverPanePID errors"`
  - `"defaultSaverMembershipProbe returns true when pid matches selfPID"`
  - `"defaultSaverMembershipProbe returns false when pid != selfPID"`

**Edge Cases**:
- `HasSession` returns true but the session is destroyed by the user before `SaverPanePID` runs — the resulting `ErrNoSuchSession` is treated as absent (probe returns false). This is the "tmux command failures are evidence the daemon's view is unreliable" branch.
- Multi-line `list-panes` output (defensive — `_portal-saver` should only have one pane) — take the first non-empty line.
- `strconv.Atoi("")` on whitespace-only output — wrap with `ErrPanePIDParse` so the caller can distinguish from "no such session".
- Tests must not use `t.Parallel()` per CLAUDE.md "Tests **must not** use `t.Parallel()`".

**Context**:
> Spec § Component D, self-check sequence steps 1–3: `tmux has-session -t _portal-saver` → if present, `tmux list-panes -t _portal-saver -F '#{pane_pid}'` compared against `os.Getpid()`. Treat any error (not just "session not found") as "absent" for this tick.
>
> Phase 2 has shipped `tmux.ErrNoSuchSession` as the typed sentinel returned at the `internal/tmux/` boundary; Task 5-2 reuses it rather than substring-matching stderr in the daemon layer.
>
> The existing exact-match `=<session>` prefix is documented in `internal/tmux/tmux.go:335` ("Uniform use of `=<session>` across HasSession / SelectWindow / SelectPane / attach-session") — `SaverPanePID` follows the same convention.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D — Daemon Self-Supervision Against the Saver Session, self-check sequence; § Files affected.

## slow-open-empty-previews-and-zombie-sessions-5-3 | approved

### Task 5-3: Integrate per-tick self-check before captureAndCommit with os.Exit(0) eject

**Problem**: The daemon ticks forever after acquiring `daemon.lock` until it receives SIGHUP or a context cancellation. There is no per-tick check that the daemon is still bound to a live `_portal-saver` pane. Between bootstraps (e.g., the user closes their laptop and returns hours later), orphan daemons accumulate freely — the reporter's install had a 13-hour orphan lifetime. Without an in-loop self-eject, Components A+B only address orphans at bootstrap time, leaving the inter-bootstrap window uncovered.

**Solution**: Insert a saver-membership self-check at the **start of every tick**, **before** the existing `captureAndCommit` call in `cmd/state_daemon.go`'s `tick`. Use the `saverMembershipProbe` seam from Task 5-2. Maintain a consecutive-failing-tick counter in `daemonDeps` (or a closure variable inside `defaultDaemonRun`'s loop scope — implementation detail). On every tick: if `saverMembershipProbe(deps.Client, os.Getpid())` returns true, reset the counter to 0; if false, increment the counter. When the counter reaches `selfSupervisionHysteresisTicks` (Task 5-1), log INFO under `ComponentDaemon` and call `os.Exit(0)`. Crucially, `os.Exit(0)` skips all deferred handlers — `defaultShutdownFlush` does NOT run — so the divergent-view daemon does NOT execute one more `captureAndCommit` / `gcOrphanScrollback` cycle on its way out. The stale `daemon.pid` is left in place by design (Phase 4 Component C pre-check handles it on the next acquire).

**Outcome**: A daemon whose saver-membership condition fails for `selfSupervisionHysteresisTicks` consecutive ticks exits via `os.Exit(0)` within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod` of the first failing tick, without running any shutdown handler, without writing to scrollback or sessions.json, and without deleting `daemon.pid`.

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

**Acceptance Criteria**:
- [ ] `tick` (or the ticker case in `defaultDaemonRun`) calls `saverMembershipProbe(deps.Client, os.Getpid())` exactly once per tick, before any other tick work — including before the `IsRestoringSet` early-return.
- [ ] The counter resets to 0 on every probe-true result; the counter does NOT merely decrement — a single recovery tick zeroes the count.
- [ ] On the tick where the counter reaches `selfSupervisionHysteresisTicks`, an INFO log line under `state.ComponentDaemon` is emitted with the literal substring `"self-supervision: saver-membership lost for"` and includes the consecutive tick count.
- [ ] Immediately after the INFO log, the daemon calls `osExit(0)` (the package-level seam) — no further code in the tick body executes.
- [ ] No deferred handler runs on eject — `daemonShutdownFunc` / `defaultShutdownFlush` is NOT invoked. (Verified structurally because `os.Exit` bypasses defers.)
- [ ] `daemon.pid` is NOT deleted before the eject — no `os.Remove(state.PIDFile(deps.Dir))` call is added; no defer that would do so.
- [ ] `state.Logger.Info` exists (added if needed) and is used here.
- [ ] When `selfSupervisionHysteresisTicks` is e.g. 3, three consecutive `false` returns from the probe trigger the eject; two `false` then one `true` resets the counter and the daemon continues.

**Tests**:
- `cmd/state_daemon_self_supervision_test.go` (new) — unit-level coverage owned partially by Task 5-4, but this task adds:
  - `"self-check runs before IsRestoringSet early-return — eject fires even when @portal-restoring is set"`
  - `"self-check runs before captureAndCommit — counter increments do not trigger captureAndCommit calls"`
  - `"eject calls osExit(0) and stops further tick body execution"`
  - `"eject does not call daemonShutdownFunc"` (verified by recording invocations on a swapped seam)
  - `"eject does not delete daemon.pid"` (assert file still exists post-eject in test)
  - `"counter increments only on probe-false; probe-true resets to 0 every time"`
- Tests overwrite `saverMembershipProbe`, `osExit`, and `daemonShutdownFunc` seams via `t.Cleanup`. No `t.Parallel()`.

**Edge Cases**:
- Self-check during `@portal-restoring` window: probe still runs, counter still increments — divergent orphan daemons must not gain immunity by virtue of a concurrent restore. The legitimate daemon's probe returns true so the counter stays at 0 in the legitimate case.
- `selfSupervisionHysteresisTicks == 1` (lower bound): a single failing tick ejects. Acceptable per spec; production value will be `≥3` from Task 5-1.
- Probe returning false transiently for k < N ticks then true: counter resets cleanly, no eject. (Covered by Task 5-4.)
- `osExit(0)` skipping `daemon.pid` removal is INTENTIONAL — there is no test that asserts `daemon.pid` is deleted (the spec explicitly forbids that cleanup).
- Logger's `Info` method may not exist today — adding it is in scope here.

**Context**:
> Spec § Component D, self-check sequence step 4: "Log INFO under `ComponentDaemon`: `"self-supervision: saver-membership lost for N consecutive ticks, exiting"`. **Skip the final flush.** Exit immediately via `os.Exit(0)` (bypassing any deferred shutdown handler)…"
>
> Spec further: "Stale `daemon.pid` after self-eject is intentional. `os.Exit(0)` skips any defer that would clean up `daemon.pid`. The stale value is handled correctly by Component C's pre-check on the next acquire (the recorded PID is dead, pre-check proceeds). Implementers MUST NOT add cleanup logic to delete `daemon.pid` before the eject — such logic would be racy against a concurrent pre-check and would invert the layered-enforcement contract."
>
> The "self-check runs before `IsRestoringSet`" ordering is a planning-phase resolution of a small spec ambiguity: the spec says "before `captureAndCommit`" but does not explicitly say "before the `restoring` early-return". The conservative reading — that an orphan must not gain immunity during a restore window — sets the ordering. Documented in-source.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D — Daemon Self-Supervision Against the Saver Session, self-check sequence steps 4–5.

## slow-open-empty-previews-and-zombie-sessions-5-4 | approved

### Task 5-4: Unit test counter reset on transient-then-recover via stubbed probe

**Problem**: The legitimate daemon must not exit on a single transient tmux-command hiccup. The spec rejects `N=1` precisely because a one-tick tmux hiccup would unnecessarily kill the legitimate daemon. Component D's correctness depends on the counter zeroing fully on the first recovered tick, not merely decrementing. A buggy implementation (e.g., `counter--` instead of `counter = 0`) would still pass a naive "eject on N consecutive failures" test but fail "transient-then-recover" — the spec explicitly calls out this edge.

**Solution**: Write a unit test that overrides `saverMembershipProbe` with a scripted sequence: returns `false` for `k < selfSupervisionHysteresisTicks` consecutive ticks, then returns `true`, then continues. The test drives `defaultDaemonRun` (or whatever helper exposes the tick body) through enough ticks to exercise multiple absent-then-present cycles and asserts (a) `osExit` is never called, (b) counter behavior is consistent with full reset, and (c) the daemon would survive arbitrarily long under repeated near-miss cycles.

**Outcome**: Regression protection: any future change that switches counter-reset to counter-decrement (or otherwise weakens the reset semantics) immediately fails this test.

**Do**:
- Place test in `cmd/state_daemon_self_supervision_test.go` (or alongside 5-3's tests in the same file).
- Use a fast `TickerPeriod` (e.g., `1 * time.Millisecond` — matching the precedent at `cmd/state_daemon_run_test.go:195`) so the test completes quickly.
- Set up a probe stub backed by a `chan bool` or an indexed `[]bool` of return values. The test scripts:
  - For each round in `[0, 5)`:
    - Push `N-1` `false` returns (where `N = selfSupervisionHysteresisTicks`).
    - Push 1 `true` return.
  - Then push 100 `true` returns to verify steady state.
- Override `osExit` to record the call and panic the test (so any eject is immediate visible failure).
- Override `daemonShutdownFunc` to no-op so the test can drive ticks past the survival window.
- Run `defaultDaemonRun` in a goroutine; cancel its context after `5*N + 100` ticks worth of wall time; assert `osExit` was never called.
- Also test the **boundary case `k = N-1` exactly** — N-1 falses then a true must not eject, even when probe later flips back to false repeatedly. Construct: `N-1 false`, `1 true`, `N-1 false`, `1 true`, ... and assert no eject across many cycles.
- Also test **probe returning transient errors counted as absent**: since the probe seam's signature is `bool` (not `(bool, error)`), the test merely asserts the seam returns `false` and verifies counter increments. The mapping from "tmux error" → `false` is owned by the default probe implementation in Task 5-2; this test ensures the counter logic treats `false` uniformly.

**Acceptance Criteria**:
- [ ] Test runs without `t.Parallel()` (CLAUDE.md convention).
- [ ] Test exercises at least 5 absent-then-recover cycles with `k = N-1` and asserts `osExit` is never invoked.
- [ ] Test exercises the exact boundary `k = N-1` (not just `k < N-1`) — one tick short of eject, recovered, repeat.
- [ ] Test uses a fast `TickerPeriod` to keep runtime under ~200 ms total.
- [ ] All seams (`saverMembershipProbe`, `osExit`, `daemonShutdownFunc`) restored via `t.Cleanup`.
- [ ] Test verifies that **after a single `true` return**, the next `N-1` `false` returns also do not eject — proving full reset, not decrement.

**Tests**:
- `"counter resets fully on first probe-true after k<N absent ticks"`
- `"counter resets at exact boundary k=N-1 absent then 1 present"`
- `"counter resets across many absent-present cycles, daemon never exits"`
- `"counter increments uniformly on probe-false regardless of cause"`

**Edge Cases**:
- `N = 1` would make this test trivial — but the lower-bound guard in Task 5-9 forbids zero, and production `N >= 3` from Task 5-1. The test should be parameterized on `selfSupervisionHysteresisTicks` (read at test setup) so a future bump in `N` does not require test rewrites.
- The probe stub must be drained safely if the daemon ticks faster than scripted entries are consumed — pad the script generously and assert the daemon did not consume past the script end (test fails loudly on underrun).
- `daemonShutdownFunc` swap must restore the production default to avoid leaking into sibling tests in the same package.

**Context**:
> Spec § Component D acceptance: "**No false-positive exit on legitimate transient.** Stub the saver-existence check to return 'absent' for k < N consecutive ticks then 'present': daemon does NOT exit, counter resets. Verified by unit test through a `saverMembershipProbe` seam."
>
> Spec § Component D, "Hysteresis N: 3 consecutive ticks" rationale: "N=1 was considered but rejected: a single tmux-command hiccup would unnecessarily kill the legitimate daemon mid-session (extremely rare but possible)." This test is the regression guard for that rationale.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D — Daemon Self-Supervision Against the Saver Session, acceptance criterion "No false-positive exit on legitimate transient".

## slow-open-empty-previews-and-zombie-sessions-5-5 | approved

### Task 5-5: Integration test self-eject when _portal-saver absent

**Problem**: Unit tests with stubbed probes prove the counter logic but cannot prove that the production `defaultSaverMembershipProbe` wired to real tmux commands actually drives the eject. The first integration acceptance — "Self-eject on absent saver" — requires spawning a real `portal state daemon` subprocess against a real tmux server that does NOT have a `_portal-saver` session and observing the process exit within `(N+1) * TickerPeriod`.

**Solution**: Add an integration test that (a) uses `portaltest.IsolateStateForTest` to keep the developer's real state directory untouched, (b) uses `portalbintest.BuildPortalBinary` to build the daemon binary under test, (c) starts a real tmux server fixture via `tmuxtest` with NO `_portal-saver` session, (d) stages the state directory with no `daemon.pid` (so Phase 4 Component C's pre-check skips and proceeds), (e) spawns the daemon binary as a subprocess (bypassing the bootstrap orchestrator so Phase 4 Component B's sweep does not preempt the test), and (f) asserts the subprocess exits within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod` wall time, plus a generous slack for tmux + ps latency.

**Outcome**: A failing eject (e.g., a future regression that re-orders the self-check after `captureAndCommit`, or accidentally short-circuits on `@portal-restoring`) fails this test loudly via timeout.

**Do**:
- Place the test under the existing integration build tag pattern. Suggested file: `cmd/state_daemon_self_supervision_integration_test.go`.
- Setup:
  1. `env, stateDir := portaltest.IsolateStateForTest(t)` — Phase 1 helper. The returned env contains an isolated `XDG_CONFIG_HOME` and the `t.Cleanup` fingerprint-diff backstop is registered.
  2. Build the daemon binary: `binPath := portalbintest.BuildPortalBinary(t)`.
  3. Start a real tmux server via `tmuxtest` with a custom socket (so it cannot collide with the developer's tmux). Do NOT create `_portal-saver` — the absence is the test condition.
  4. Verify the staging satisfies Phase 4 Component C's pre-check:
     - Confirm `<stateDir>/daemon.pid` does NOT exist (the helper-issued tempdir is freshly created so this is the default).
     - Confirm `<stateDir>/daemon.lock` does NOT exist (Phase 4's `AcquireDaemonLock` will create it).
  5. Spawn the daemon: `cmd := exec.Command(binPath, "state", "daemon")`; `cmd.Env = env`; set `TMUX` env to point at the test socket so the daemon's `tmux.DefaultClient()` calls hit the test server.
- Action:
  - Start the subprocess.
  - Wait for the process to exit. The expected exit is via `os.Exit(0)` — exit code 0, no `defaultShutdownFlush` invocation, no stderr stack trace.
- Assertion:
  - Process exits within `(selfSupervisionHysteresisTicks + 1) * 1s` (TickerPeriod) + a 2 s slack for process startup, tmux command latency, and OS scheduling.
  - Exit code is 0.
  - The daemon's log file (under `<stateDir>/daemon.log` or whatever path `state.NewLogger` writes to — verify path) contains an INFO line matching the substring `"self-supervision: saver-membership lost for"`.
  - `<stateDir>/daemon.pid` either still exists with the dead PID, or never existed (depending on whether the daemon wrote it before the first self-check tick fired) — the spec is explicit that no defer deletes it, so if it was written, it is intentionally stale. Assert: if present, contents == the subprocess's PID. Do NOT assert deletion.
- Teardown:
  - Kill the tmux test server.
  - `portaltest`'s `t.Cleanup` runs the fingerprint-diff backstop automatically.

**Acceptance Criteria**:
- [ ] Test is tagged with the existing integration build tag pattern (see `cmd/state_daemon_integration_test.go` for the precedent).
- [ ] Test uses `portaltest.IsolateStateForTest` — no developer-state mutations on exit.
- [ ] Test stages `<stateDir>/daemon.pid` as absent so Phase 4 Component C's pre-check proceeds.
- [ ] Test spawns the daemon binary directly (not via `portal open` / not via the bootstrap orchestrator) so Phase 4 Component B's sweep does not preempt setup.
- [ ] Test does NOT create `_portal-saver` on the real tmux server.
- [ ] Daemon exits within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod + 2s` with exit code 0.
- [ ] Daemon log contains the INFO substring `"self-supervision: saver-membership lost for"`.
- [ ] `<stateDir>/daemon.pid`, if present post-exit, has not been deleted — assert presence with stale PID, never assert deletion.

**Tests**:
- `"self-eject: portal state daemon exits within (N+1) ticks when _portal-saver is absent"`
- `"self-eject: exit code is 0, no panic/error stack on stderr"`
- `"self-eject: daemon.pid is intentionally not deleted before exit"`
- `"self-eject: daemon log records the self-supervision INFO line"`

**Edge Cases**:
- The daemon binary may race: spawn → write daemon.pid → first tick fires → probe → probe-false → counter=1 → ... → counter=N → osExit(0). The TickerPeriod is 1 s, so wall time to eject is bounded but not instantaneous. Test slack of 2 s on top of `(N+1) * 1s` absorbs this.
- The test must NOT call `tmux kill-server` until after the subprocess has exited and been waited on — killing tmux first could make the probe error differently and obscure failures.
- If `portaltest.IsolateStateForTest` is not yet wired (Phase 1 incomplete), this task is blocked. Per the planning brief, Phase 1 has shipped — the helper is available.
- Confirm the daemon honours `TMUX` env or whatever mechanism `tmux.DefaultClient()` uses to discover the socket. If the production client always uses `tmux` from PATH against the default socket, the test must invoke the binary with `TMUX_TMPDIR` or similar to redirect. Verify during initial scaffolding.

**Context**:
> Spec § Component D acceptance: "**Self-eject on absent saver.** Spawn `portal state daemon` against a tmux server that has no `_portal-saver` session. The daemon exits within (N + 1) tick intervals. Verified by integration test."
>
> Spec § Test staging note: "D's integration tests intentionally violate the saver-pane-process invariant; to reach the tick loop, they must satisfy Component C's lock-acquire pre-check. Tests stage the state directory with either (i) no `daemon.pid` file (pre-check skips and proceeds), or (ii) a `daemon.pid` referencing a known-dead PID. The tests spawn the daemon directly (bypassing the bootstrap orchestrator) so Component B's sweep does not preempt the test setup."
>
> Phase 1 has shipped `portaltest.IsolateStateForTest`; Phase 4 has shipped the `AcquireDaemonLock` pre-check.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D, acceptance criterion "Self-eject on absent saver" and "Test staging note".

## slow-open-empty-previews-and-zombie-sessions-5-6 | approved

### Task 5-6: Integration test self-eject on saver pane pid mismatch via respawn-pane -k

**Problem**: The "saver absent" case (Task 5-5) is only one of two trigger conditions. The second — and the more realistic real-world trigger for an orphan — is that `_portal-saver` exists but its pane process has been replaced (e.g., by a previous bootstrap's recreate path) so the running daemon's PID no longer matches `pane_pid`. Without an explicit integration test for this case, a regression that confuses "session exists" with "I am still the pane process" could slip past the absent-saver test.

**Solution**: Add an integration test that (a) creates `_portal-saver` on a real tmux server with the daemon binary as the initial pane command, (b) stages `<stateDir>/daemon.pid` with a known-dead PID so Phase 4 Component C's pre-check proceeds and the new daemon can acquire the lock, (c) spawns the daemon subprocess (bypassing the bootstrap orchestrator so Phase 4 Component B does not preempt), (d) externally replaces the saver pane's process via `tmux respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'`, (e) observes the daemon subprocess exiting within `(N+1) * TickerPeriod`.

**Outcome**: The pid-mismatch trigger is independently covered — even if the absent-saver test passes, a regression that only honours `HasSession == false` (ignoring `pane_pid` divergence) fails this test.

**Do**:
- Place the test alongside Task 5-5's in `cmd/state_daemon_self_supervision_integration_test.go`.
- Setup:
  1. `env, stateDir := portaltest.IsolateStateForTest(t)`.
  2. `binPath := portalbintest.BuildPortalBinary(t)`.
  3. Start real tmux server via `tmuxtest` against a custom socket.
  4. Stage `<stateDir>/daemon.pid` with a known-dead PID. Use a PID known to be unused (e.g., spawn a short-lived process, capture its PID, wait for it to exit, then write that PID into `daemon.pid` via `state.WritePIDFile`). Phase 1's `state.IdentifyDaemon` will resolve this as `IdentifyDead` so Phase 4 Component C's pre-check proceeds.
  5. Create `_portal-saver` directly via `tmux new-session -d -s _portal-saver 'sh -c "exec tail -f /dev/null"'` (NOT the daemon binary — the daemon will be spawned separately below). Set `destroy-unattached=off` on the session immediately.
- Spawn the daemon:
  - `cmd := exec.Command(binPath, "state", "daemon")`; `cmd.Env = env`.
  - Wait until the daemon has acquired the lock (poll for `daemon.pid` to contain the subprocess's PID, bounded ~2 s).
  - Verify: the daemon's PID ≠ `_portal-saver`'s pane PID (use `tmux list-panes -t _portal-saver -F '#{pane_pid}'`). This is the structural divergence the daemon should detect.
- Action:
  - Wait one tick (~1 s) to let the daemon's first self-check fire and observe the mismatch.
  - Optionally re-confirm the divergence by `respawn-pane`-ing the saver pane to a fresh `sh -c "exec tail -f /dev/null"` mid-test to ensure the mismatch persists across at least N ticks.
- Assertion:
  - The daemon subprocess exits within `(selfSupervisionHysteresisTicks + 1) * 1s + 2s` slack with exit code 0.
  - Daemon log contains the `"self-supervision: saver-membership lost for"` INFO line.
  - `_portal-saver` session still exists post-eject (eject does not destroy the saver).
- Teardown:
  - `tmux kill-server` against the test socket.
  - `portaltest` `t.Cleanup` fingerprint check runs.

**Acceptance Criteria**:
- [ ] Test is tagged with the integration build tag pattern.
- [ ] Test uses `portaltest.IsolateStateForTest` and `portalbintest.BuildPortalBinary`.
- [ ] Test stages `<stateDir>/daemon.pid` with a known-dead PID (not absent) — the alternative staging path from the spec's Test staging note, complementing Task 5-5's "absent" staging.
- [ ] `_portal-saver` is pre-created with a placeholder process (`sh -c 'exec tail -f /dev/null'`) so the daemon's PID cannot match `pane_pid`.
- [ ] Daemon exits with code 0 within `(N+1) * TickerPeriod + 2s`.
- [ ] Daemon log contains the self-supervision INFO line.
- [ ] `_portal-saver` session still exists after the daemon exits (composes with Phase 3's `destroy-unattached=off` so the placeholder remains).

**Tests**:
- `"self-eject: pid mismatch — daemon exits within (N+1) ticks when _portal-saver pane process is not the daemon"`
- `"self-eject: pid mismatch — daemon log records INFO self-supervision line"`
- `"self-eject: pid mismatch — _portal-saver session survives the eject (destroy-unattached=off composes)"`
- `"self-eject: pid mismatch — exit code is 0, no shutdown handler invocation visible"`

**Edge Cases**:
- The staged dead PID must be reliably dead. The reliable pattern is `cmd := exec.Command("true"); cmd.Run(); deadPID := cmd.Process.Pid`. After `Run` returns, the process is reaped and the PID is dead. Re-use the existing pattern from Phase 4 Component C's lock-acquire tests if one exists.
- If `tmux respawn-pane -k` is used mid-test (optional reinforcement), it requires the existing `RespawnPane` method on `*tmux.Client` or equivalent shell invocation. The test can call the tmux binary directly via `exec.Command("tmux", "-S", socket, "respawn-pane", ...)` rather than depending on the portal client's wrapper.
- A subtle confound: if the daemon happens to be the pane process at startup (e.g., test misconfigured the saver setup), the self-check passes and the test hangs. Pre-action verification that `daemon.PID != pane_pid` is REQUIRED — the test must explicitly assert this divergence exists before waiting for the eject.
- Test must NOT call any portal bootstrap code path — only the raw daemon subprocess, plus direct `tmux` invocations.

**Context**:
> Spec § Component D acceptance: "**Self-eject on saver pane pid mismatch.** Spawn the daemon, then externally replace the `_portal-saver` pane process (e.g., `respawn-pane` to a different process). Daemon exits within (N + 1) tick intervals. Verified by integration test."
>
> Spec § Test staging note explicitly suggests `tmux respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'` for this case. The variant chosen in this task is "pre-create the saver with the placeholder, then spawn the daemon as a non-saver subprocess" — structurally equivalent (the daemon's PID differs from `pane_pid`) and simpler than spawning-then-respawning.
>
> Phase 3 has shipped placeholder-then-respawn saver creation ordering; Phase 4 has shipped Component C's pre-check (the dead-PID staging path); Phase 1 has shipped `portaltest.IsolateStateForTest`.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D, acceptance criterion "Self-eject on saver pane pid mismatch" and "Test staging note".

## slow-open-empty-previews-and-zombie-sessions-5-7 | approved

### Task 5-7: Integration test bytes-identical scrollback dir snapshot across self-eject

**Problem**: Component D's correctness hinges on the divergent-view daemon NOT executing one more `captureAndCommit` / `gcOrphanScrollback` cycle on its way out — that final flush against a divergent view is the destructive operation the `os.Exit(0)` eject exists to prevent. A code-level review of "we used `os.Exit(0)` not a graceful shutdown" is insufficient; the spec mandates an empirical assertion that the scrollback directory is byte-identical at the first failing tick (before the counter increments to N) and immediately after the eject.

**Solution**: Add an integration test that spawns a real `portal state daemon` subprocess under conditions guaranteed to trigger self-eject (e.g., the "absent saver" condition from Task 5-5), snapshots the scrollback directory (`<stateDir>/scrollback/`) at the moment the daemon's first failing tick fires (before counter reaches N), waits for the daemon to exit, snapshots again immediately after exit, and asserts the two snapshots are byte-identical: no new `.bin`, no removed `.bin`, no size/mtime/content delta on existing `.bin` files. Use `os.ReadDir` polling (acceptable per spec's "fsnotify or a polled `os.ReadDir` snapshot — either is acceptable").

**Outcome**: A failing snapshot diff immediately surfaces any future regression that inadvertently lets the eject path run one more capture/commit/GC cycle (e.g., adding a `defer captureAndCommit(...)` somewhere upstream, or replacing `os.Exit(0)` with a graceful shutdown that fires `defaultShutdownFlush`).

**Do**:
- Place the test alongside Tasks 5-5 and 5-6 in `cmd/state_daemon_self_supervision_integration_test.go`.
- Setup mirrors Task 5-5 (absent-saver trigger):
  1. `env, stateDir := portaltest.IsolateStateForTest(t)`.
  2. `binPath := portalbintest.BuildPortalBinary(t)`.
  3. Start real tmux server via `tmuxtest` against a custom socket; do NOT create `_portal-saver`.
  4. Stage `<stateDir>/daemon.pid` as absent (per the "no daemon.pid" staging path).
- Snapshot helper:
  ```go
  type fileSnap struct{ size int64; mtimeNS int64; sum [32]byte }
  func snapshotDir(t *testing.T, dir string) map[string]fileSnap { ... }
  ```
  For each regular file under `<stateDir>/scrollback/` (recursive, lstat semantics), record size, mtime nanoseconds, and SHA-256 of contents. Use `filepath.WalkDir` rooted at `<stateDir>/scrollback/`. If the directory does not exist (no scrollback ever written), the snapshot is the empty map — empty pre-snapshot is still valid per the task table's edge cases.
- Diff helper: compare two snapshots; return a list of deltas (added files, removed files, size changes, mtime changes, content changes). Test fails on any non-empty delta list with a descriptive error.
- Action:
  1. Spawn the daemon subprocess.
  2. Wait for the first probe-false tick to land: detect via the daemon's log file — poll the daemon log for the first WARN/INFO emission that is NOT a self-supervision eject line. Alternatively, use a fixed delay of `1 * TickerPeriod + 200ms` — robust enough since the spec's TickerPeriod is 1 s.
  3. Take `snapBefore := snapshotDir(t, scrollbackDir)`.
  4. Wait for the daemon process to exit (poll `os.FindProcess` + `Signal(0)` for ESRCH, bounded to `(N+1) * TickerPeriod + 2s`).
  5. Take `snapAfter := snapshotDir(t, scrollbackDir)`.
  6. Assert `diff(snapBefore, snapAfter)` is empty.
- The snapshot is intentionally taken DURING the failing-tick window (counter > 0 but < N) so the test catches even a single per-tick capture/commit slip. The spec is explicit: "Snapshot the scrollback directory at the moment the daemon's self-check first registers a failing tick, and again immediately after `os.Exit(0)`."

**Acceptance Criteria**:
- [ ] Test is tagged with the integration build tag pattern.
- [ ] Test uses `portaltest.IsolateStateForTest` and `portalbintest.BuildPortalBinary`.
- [ ] `snapshotDir` records size, mtime nanoseconds, and SHA-256 for every regular file under `<stateDir>/scrollback/` (recursive); uses lstat semantics (no symlink following).
- [ ] `snapBefore` is captured at or after the first failing-tick observation, before the counter reaches N.
- [ ] `snapAfter` is captured immediately after the daemon process exits (verified via `Signal(0)` ESRCH).
- [ ] Diff returns an empty delta list — no added files, no removed files, no size/mtime/content changes.
- [ ] Test asserts the daemon's exit code is 0 (composing with Tasks 5-5/5-6's assertions but redundantly verified here).
- [ ] Empty pre-snapshot (no scrollback ever written) is a valid baseline — the diff against an empty post-snapshot still passes.

**Tests**:
- `"self-eject: scrollback directory is bytes-identical at first-failing-tick and post-exit (absent-saver trigger)"`
- `"self-eject: empty pre-snapshot remains empty post-exit (no spurious writes on eject path)"`

**Edge Cases**:
- The window between "first failing tick" and "process exit" is short (`(N-1) * TickerPeriod` at most). Polling cadence for snapshot timing must be fast enough to land inside the window — use `50 ms` polling on the log or a fixed-delay variant if log polling is unreliable.
- mtime granularity on macOS HFS+/APFS is sub-second; the SHA-256 + size comparison is the load-bearing assertion. mtime is supplementary — log it for debug but accept some test flakiness if mtime granularity differs across filesystems. The spec wording is "no `.bin` file deletions or unexpected new files" — that is captured by size + content + name set.
- If `<stateDir>/scrollback/` does not exist at all, `snapshotDir` returns `nil`/empty; the diff against empty is also empty — test passes legitimately.
- The first failing tick may NOT produce any scrollback file (the daemon has no `_portal-saver` and no live sessions to capture) — the pre-snapshot may be empty. That is still a valid test — the assertion is "no change", not "non-empty initial state".

**Context**:
> Spec § Component D acceptance: "**No final flush on self-eject.** Snapshot the scrollback directory at the moment the daemon's self-check first registers a failing tick, and again immediately after `os.Exit(0)`. The two snapshots must be identical (no new files, no deletions, no mtime/size changes). Verified by integration test that uses fsnotify or a polled `os.ReadDir` snapshot during the eject window."
>
> Mirrors Phase 4's Task 4-2 ("no-final-flush snapshot test for escalation-killed orphans") — same testing pattern, applied to Component D's `os.Exit(0)` path rather than Component A's SIGKILL path.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D, acceptance criterion "No final flush on self-eject".

## slow-open-empty-previews-and-zombie-sessions-5-8 | approved

### Task 5-8: Integration test legitimate first-tick self-check inside fresh _portal-saver

**Problem**: The self-check must NOT false-positive on the legitimate daemon's very first tick. The legitimate daemon is the saver pane process of a freshly-created `_portal-saver` — `os.Getpid()` matches the pane pid by construction. If the self-check is wired incorrectly (e.g., off-by-one in the counter, probe miscompares pids as strings vs ints, or `HasSession` race during early saver bootstrap), the legitimate daemon would self-eject on tick 1, breaking the entire Portal experience. This is the "false-positive on cold-start" regression guard.

**Solution**: Add an integration test that composes with Phase 3's placeholder-then-respawn saver creation ordering — bootstrap a fresh `_portal-saver` via the production path (or a test-bench equivalent), wait for the respawned daemon to become healthy, then observe the daemon ticking normally for at least `(N+2) * TickerPeriod` without exiting. The test asserts the daemon is still alive, the counter (inferred from absence of the self-supervision INFO line in logs) has stayed at 0, and `daemon.pid` reflects the live daemon's PID matching `_portal-saver`'s pane pid.

**Outcome**: Any regression where the legitimate daemon's first tick fails its own self-check fails this test before it can ship. Composes with Phase 3 — if Phase 3's placeholder-then-respawn introduces a transient window in which the pane pid disagrees with the daemon's PID at tick 1, the spec's "single-digit ticks" hysteresis (≥3) absorbs the transient and this test still passes.

**Do**:
- Place the test alongside Tasks 5-5/5-6/5-7 in `cmd/state_daemon_self_supervision_integration_test.go` (or a separate file if cohesion improves).
- Setup:
  1. `env, stateDir := portaltest.IsolateStateForTest(t)`.
  2. `binPath := portalbintest.BuildPortalBinary(t)`.
  3. Start real tmux server via `tmuxtest` against a custom socket.
  4. Invoke `BootstrapPortalSaver` (via a test entry point, or by spawning a short-lived helper invocation of `portal open --no-tui` or equivalent — verify the cleanest seam during scaffolding). The goal is to exercise the **production** saver-creation path so Phase 3's placeholder-then-respawn ordering is on the test path.
  5. Wait for the readiness barrier (Phase 3 Task 3-3) to confirm the daemon is healthy: `<stateDir>/daemon.pid` exists, points at a live PID, identifies as a `portal state daemon`, and matches `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
- Action:
  - Let the daemon tick freely for at least `(selfSupervisionHysteresisTicks + 2) * TickerPeriod` (≥ 5 s with N=3 and TickerPeriod=1s).
- Assertion:
  - The daemon process is still alive at the end of the observation window (poll `Signal(0)` returns nil, not ESRCH).
  - The daemon's log file does NOT contain any `"self-supervision: saver-membership lost for"` line.
  - `<stateDir>/daemon.pid` still references the same PID it did at startup (no rotation).
  - `tmux list-panes -t _portal-saver -F '#{pane_pid}'` still returns the daemon's PID.
- Teardown:
  - Kill the daemon via `tmux kill-session _portal-saver` (the production shutdown path).
  - `tmux kill-server` against the test socket.
  - `portaltest` `t.Cleanup` fingerprint check runs.

**Acceptance Criteria**:
- [ ] Test is tagged with the integration build tag pattern.
- [ ] Test uses `portaltest.IsolateStateForTest` and `portalbintest.BuildPortalBinary`.
- [ ] Test uses Phase 3's `BootstrapPortalSaver` path (placeholder-then-respawn ordering) — does NOT manually `tmux new-session -s _portal-saver ...` bypassing the production flow.
- [ ] Daemon remains alive for at least `(selfSupervisionHysteresisTicks + 2) * TickerPeriod` after the Phase 3 readiness barrier resolves healthy.
- [ ] Daemon log contains zero `"self-supervision: saver-membership lost for"` entries during the observation window.
- [ ] `daemon.pid` is unchanged from immediately-after-readiness to end-of-observation-window.
- [ ] `tmux list-panes -t _portal-saver -F '#{pane_pid}'` matches `daemon.pid` at the end of the observation window.

**Tests**:
- `"first-tick self-check passes inside freshly-bootstrapped _portal-saver"`
- `"daemon survives (N+2) tick intervals without self-eject in legitimate steady state"`
- `"daemon.pid matches _portal-saver pane pid throughout observation window"`

**Edge Cases**:
- Phase 3's placeholder-then-respawn introduces a transient window where the pane process is `sh -c 'exec tail -f /dev/null'` (not the daemon). The Phase 3 readiness barrier resolves this transient before the daemon is considered "ready". The test must NOT begin its observation window until the Phase 3 readiness barrier returns successfully — otherwise the daemon might not be ticking yet, or might be the placeholder.
- The test must use a real tmux server (via `tmuxtest`), not a `Commander` mock — the whole point is end-to-end legitimate-path coverage.
- If `BootstrapPortalSaver` is not directly callable from a test (no exported test entry point), spawning a `portal open` subprocess with `--no-tui` (or whatever bootstrap-only invocation pattern exists) is acceptable; document the choice in a code comment.

**Context**:
> Spec § Component D acceptance: "**Skipped check on first tick is benign.** The legitimate daemon, ticking for the first time inside a freshly-created `_portal-saver`, passes the self-check on tick 1 (pane pid matches its pid). Verified by integration test in the existing daemon-saver suite."
>
> Spec § Component D, "Hysteresis N: 3 consecutive ticks" rationale: "The legitimate daemon never observes a transient 'saver absent' condition." This task is the empirical verification of that claim under the Phase 3 saver-creation ordering.
>
> Phase 3 has shipped placeholder-then-respawn saver creation ordering. Phase 4 has shipped Component C's pre-check. Phase 1 has shipped `portaltest.IsolateStateForTest`. Together these allow the test to exercise the **full** legitimate cold-start path.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D, acceptance criterion "Skipped check on first tick is benign".

## slow-open-empty-previews-and-zombie-sessions-5-9 | approved

### Task 5-9: Unit test selfSupervisionHysteresisTicks >= 1

**Problem**: Component D's `selfSupervisionHysteresisTicks` is a tuning knob. A future maintainer or a sloppy merge could accidentally zero it (or set it negative), turning the self-check into "eject on first failing tick" — which the spec explicitly rejects ("N=1 was considered but rejected: a single tmux-command hiccup would unnecessarily kill the legitimate daemon mid-session"). The spec mandates "A unit test asserts `selfSupervisionHysteresisTicks >= 1` to prevent accidental zeroing; the actual value-vs-measurement justification is enforced by code review, not test."

**Solution**: Add a single tiny unit test in `cmd/state_daemon_test.go` (or `cmd/state_daemon_self_supervision_test.go` if Task 5-3/5-4 grouped the self-supervision tests there) that compiles a guard: `if selfSupervisionHysteresisTicks < 1 { t.Fatalf("selfSupervisionHysteresisTicks must be >= 1 to satisfy spec; got %d", selfSupervisionHysteresisTicks) }`. Since the constant is `const`, this also catches accidental conversion to `var` with a zero default at compile or test time.

**Outcome**: A future commit that accidentally drops the constant below 1 fails this test loudly on the next `go test ./cmd`.

**Do**:
- Add a test function `TestSelfSupervisionHysteresisTicksLowerBound` in `cmd/state_daemon_test.go` (or wherever Task 5-3's tests live).
- Body:
  ```go
  func TestSelfSupervisionHysteresisTicksLowerBound(t *testing.T) {
      if selfSupervisionHysteresisTicks < 1 {
          t.Fatalf("selfSupervisionHysteresisTicks must be >= 1 to prevent false-positive eject on a single transient tick; got %d", selfSupervisionHysteresisTicks)
      }
  }
  ```
- No `t.Parallel()` (CLAUDE.md convention).
- Do NOT also assert an upper bound here — that is owned by Task 5-1's code review and measurement memo. The spec is explicit: the actual value justification is enforced by code review, not test.
- This is the smallest possible TDD cycle; the test exists purely to prevent regression of the constant.

**Acceptance Criteria**:
- [ ] Test `TestSelfSupervisionHysteresisTicksLowerBound` exists in the cmd package.
- [ ] Test fails if `selfSupervisionHysteresisTicks < 1` (zero, negative).
- [ ] Test passes for `selfSupervisionHysteresisTicks == 1` and above.
- [ ] Test does NOT assert any upper bound (per spec — the upper bound is "single-digit ticks" enforced by code review on Task 5-1).
- [ ] Test does not use `t.Parallel()`.
- [ ] Test is independent of `saverMembershipProbe`, `osExit`, and `daemonShutdownFunc` seams — it asserts a property of the constant alone.

**Tests**:
- `"selfSupervisionHysteresisTicks must be >= 1 to prevent accidental zeroing"`

**Edge Cases**:
- If a future maintainer changes the constant to a `var`, this test still works (the comparison is value-based, not type-based).
- If a future maintainer makes the constant runtime-overridable (NOT recommended), this test still catches the lower-bound violation at default value.
- The test deliberately does NOT validate the measurement memo's existence or content — that is a documentation requirement enforced by code review, not by automated test.

**Context**:
> Spec § Component D acceptance: "**Measurement artefact for N.** … A unit test asserts `selfSupervisionHysteresisTicks >= 1` to prevent accidental zeroing; the actual value-vs-measurement justification is enforced by code review, not test."
>
> This is the minimal viable regression guard, intentionally weak by spec design. Stronger checks (upper bound, ≥3, etc.) would couple the test to Task 5-1's specific measured value and force test rewrites on every legitimate re-measurement.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component D, acceptance criterion "Measurement artefact for N".
