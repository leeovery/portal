# Component D Hysteresis Measurement Memo

**Task:** slow-open-empty-previews-and-zombie-sessions-5-1
**Date:** 2026-05-23
**Spec reference:** specification.md § Component D ("Hysteresis N: 3 consecutive ticks" rationale + "Measurement artefact for N"); § Risk Summary (empirical measurement REQUIRED).

## Environment

| Field | Value |
|-------|-------|
| Host OS | macOS Darwin 25.3.0 (arm64, T6000) |
| Tmux | 3.6b (`/opt/homebrew/bin/tmux`) |
| Go | go1.26.3 darwin/arm64 |
| Portal binary | `dev` build via `go build .` against repo HEAD (no goreleaser ldflags) |
| Runs per scenario | 5 (matches spec minimum) |
| Sample period | 1s (matches production `TickerPeriod` in `cmd/state_daemon.go`) |
| Probe shape | `tmux has-session -t _portal-saver` + `tmux list-panes -t _portal-saver -F '#{pane_pid}'`; compare against `state.ReadPIDFile(stateDir)` |

The probe is the inline measurement-harness equivalent of Task 5-2's `saverMembershipProbe` seam — same shape, same false-discriminator. False means "the daemon recorded in daemon.pid is NOT the pane pid of the live `_portal-saver` session".

## Scenarios and raw measurements

For each scenario the harness samples the probe at 1s cadence for the noted observation window, then records the longest consecutive run of false-probe observations within that window. The result per run is that worst-case count.

### Scenario 1 — Steady-state

- **Setup:** legitimate daemon as saver pane process, no interaction.
- **Window:** 30s per spec.
- **Expectation:** ≈0 ticks (the membership invariant holds for the entire window).

| Run | 1 | 2 | 3 | 4 | 5 | min | median | max |
|----:|--:|--:|--:|--:|--:|----:|-------:|----:|
| Consecutive failures | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

### Scenario 2 — Attach/detach cycles

- **Setup:** legitimate daemon + a probe session, `tmux refresh-client` issued at 200ms cadence from a parallel goroutine.
- **Window:** 15s.
- **Substitution note:** the spec describes `tmux attach -d` from a parallel goroutine OR `switch-client`/`refresh-client`. Without an interactive PTY in `go test`, `tmux attach` exits immediately and would not exercise the attach/detach lifecycle meaningfully. The harness substitutes `refresh-client` which is the closest available approximation that touches client state without requiring a PTY.
- **Expectation:** ≈0 ticks (client state changes do not perturb the saver pane's pid).

| Run | 1 | 2 | 3 | 4 | 5 | min | median | max |
|----:|--:|--:|--:|--:|--:|----:|-------:|----:|
| Consecutive failures | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

### Scenario 3 — `client-attached` hook fires

- **Setup:** legitimate daemon + production hook table installed via `tmux.RegisterPortalHooks(client, nil)`. A parallel goroutine fires `tmux run-shell -b true` plus periodic attach attempts at 500ms cadence to dispatch the `client-attached` hook payload.
- **Window:** 15s.
- **Substitution note:** without an interactive PTY, a true `tmux attach` cannot complete and the production `client-attached` event fires only opportunistically. The harness drives the hook's subprocess invocation path via `run-shell -b` as the closest available approximation.
- **Expectation:** transient bounded by hook command duration. Production hook is `portal state signal-hydrate`, which is a brief subprocess call — sub-tick on healthy hardware.

| Run | 1 | 2 | 3 | 4 | 5 | min | median | max |
|----:|--:|--:|--:|--:|--:|----:|-------:|----:|
| Consecutive failures | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

### Scenario 4 — Bootstrap kill-and-recreate

- **Setup:** legitimate daemon as saver pane process. ~2s into the window the harness invokes the real `BootstrapPortalSaver` unhealthy-saver recreate path: `tmux kill-session -t _portal-saver` followed by `tmux.BootstrapPortalSaver(client, stateDir)`. This composes Task 3-1/3-2/3-3's create branch: placeholder (`sh -c 'exec tail -f /dev/null'`) → `set-option destroy-unattached=off` → `respawn-pane` to `portal state daemon` → `waitForSaverDaemonReady` poll for `daemon.pid` + identity check.
- **Window:** 10s.
- **Expectation:** this is the only scenario likely to produce a non-zero transient. The recreate has a ~2s readiness-barrier ceiling plus tmux respawn settle. Per the spec's rationale, observed transient should still be in the single-digit-ticks range.

| Run | 1 | 2 | 3 | 4 | 5 | min | median | max |
|----:|--:|--:|--:|--:|--:|----:|-------:|----:|
| Consecutive failures | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

## Aggregate

| Field | Value |
|-------|-------|
| Max observed across all scenarios | 0 ticks |
| Max × 2 (safety factor) | 0 ticks |
| Clamped to [3, 9] | **3 ticks** |
| `selfSupervisionHysteresisTicks` (cmd/state_daemon.go) | **3 ticks** |
| Upstream-defect flag (max × 2 > 5) | **false** |

## Rationale for N = 3

The measurement returned 0 worst-case consecutive failures across every scenario, including the bootstrap kill-and-recreate path that is the recreate-window stress case. The mathematically-derived 2× safety factor is therefore 0 — well below the spec's lower clamp of 3.

Per the task, when `max × 2 < 3`, N is clamped to the floor of 3. The choice of 3 as the floor reflects the spec's rationale for absorbing transient tmux command flakiness without significantly extending orphan lifetime; N=1 would risk a single tmux-hiccup mid-tick triggering a false-positive self-eject. With N=3 the orphan-lifetime ceiling under the production `TickerPeriod` of 1s is ~3–4s of additional drift after a saver-membership condition first fails — well inside the user's "bound to one tick *between* bootstraps" target framing.

## Upstream-defect assessment

`max × 2 = 0` does NOT exceed the spec's 5-tick warning threshold, so the upstream-defect flag is **false**. The legitimate daemon and the bootstrap recreate path both observably converge fast enough that the recreate transient is undetectable at 1s sampling.

If a future regression lengthens the bootstrap recreate transient past ~2.5 ticks (so that `max × 2 > 5`), the harness's safety-factor assertion fires before the constant is bumped past the clamp ceiling of 9, and the memo regenerated at that point would set the flag to true — surfacing the regression as evidence of upstream defect per spec § Risk Summary.

## Harness location

`cmd/state_daemon_hysteresis_measurement_test.go` (build tag `integration`). The harness is re-runnable via:

```
go test -tags integration -run TestSelfSupervisionHysteresisMeasurement ./cmd/ -v
```

The harness assertion checks `selfSupervisionHysteresisTicks >= max-observed × 2` AND `3 ≤ selfSupervisionHysteresisTicks ≤ 9`. A regression that lengthens any scenario's worst-case transient past `ceil(selfSupervisionHysteresisTicks / 2)` fails the harness — providing a structural regression guard that future-proofs the chosen N.

## Notes on host environment during measurement

The development host had a leaked test-fixture tmux server at `/tmp/test_hook_debug2/s` running (the same trigger described in specification.md § Root Cause → "Trigger on this install"). Because that server's `session-closed` hooks fire `portal state commit-now` against the developer's real `~/.config/portal/state/`, the `portaltest.NewIsolatedStateEnv` backstop reported observations of mtime/ctime mutations on `save.requested` in the dev state dir during the test run. These mutations are external to the measurement harness — they originate from the leaked fixture's continued operation and do NOT affect the measured probe values (which are taken against the harness's isolated socket and isolated state dir).

The measured worst-case counts (0 across every scenario / every run) are reliable: the probe reads from the harness's isolated `daemon.pid` and queries the isolated tmux socket, both of which are unaffected by activity in the dev state dir.

Per spec § Transitional Recovery, the leaked fixture can be cleaned up via:

```
pkill -9 -x 'portal state daemon'
rm -f ~/.config/portal/state/daemon.lock ~/.config/portal/state/daemon.pid ~/.config/portal/state/daemon.version
tmux -S /tmp/test_hook_debug2/s kill-server 2>/dev/null || true
rm -rf /tmp/test_hook_debug2/
```

This is a one-shot manual procedure on the developer's host; it is NOT part of the shipped fix.
