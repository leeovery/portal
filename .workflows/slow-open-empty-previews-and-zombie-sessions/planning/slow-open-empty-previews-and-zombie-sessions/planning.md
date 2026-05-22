# Plan: Slow Open Empty Previews And Zombie Sessions

## Phases

### Phase 1: Foundations — Daemon Identity Primitive & Test Isolation
status: approved
approved_at: 2026-05-22

**Goal**: Land the shared daemon-identity check used by Components A/B/C and the test-isolation helper required to safely test all daemon-spawning fixes without corrupting the developer's real state directory.

**Why this order**: The identity primitive is a transitive dependency of Components A, B, C; the test-isolation helper is a transitive dependency of every later phase that spawns `portal state daemon`. Strongest foundation first — both must exist before downstream phases can be implemented or tested safely.

**Acceptance**:
- [ ] `state.IdentifyDaemon(pid int) (IdentifyResult, error)` exists at `internal/state/daemon_identity.go` with the three-result contract (`IdentifyIsPortalDaemon` / `IdentifyNotPortalDaemon` / `IdentifyDead`) plus the transient-error case
- [ ] Identity check uses `ps -o comm=,args= -p <pid>` matching `comm == "portal"` AND argv against `^portal state daemon( |$)`; unit tests cover live-match, recycled-PID-no-match, dead-PID, and transient-`ps`-failure cases
- [ ] `portaltest.NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string)` exists in new leaf package `internal/portaltest/`; returned `env` contains `XDG_CONFIG_HOME=<tempDir>/config` and does NOT contain the developer's pre-test `XDG_CONFIG_HOME` value
- [ ] `t.Cleanup` registered by the helper takes a pre-test fingerprint (existence, size, mtime ns, ctime ns, SHA-256 ≤1 MiB) of `~/.config/portal/state/`, walks again post-test using lstat semantics, and fails on any delta with a clear error citing the changed path and delta type
- [ ] Audit of `internal/portalbintest`, `internal/tmuxtest`, `internal/restoretest` enumerates every helper that spawns `portal` / `portal state daemon` and updates each to take (or call) the isolated env; audit deliverable captured in PR description with the `grep` completion criterion satisfied
- [ ] `CLAUDE.md` (or new `TESTING.md`) documents the test-isolation contract under "DI / testing pattern", locatable via search for "test isolation" or "XDG_CONFIG_HOME"
- [ ] Existing integration test suite passes after helper updates

#### Tasks
status: approved
approved_at: 2026-05-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| slow-open-empty-previews-and-zombie-sessions-1-1 | Implement state.IdentifyDaemon primitive | dead PID, recycled PID, unrelated process, ps exec failure, malformed/empty ps output, whitespace in comm/args |
| slow-open-empty-previews-and-zombie-sessions-1-2 | Implement portaltest.NewIsolatedStateEnv helper | pre-existing XDG_CONFIG_HOME unset, HOME preserved, env usable for exec.Cmd.Env, test-only signature via *testing.T |
| slow-open-empty-previews-and-zombie-sessions-1-3 | Add fingerprint-diff t.Cleanup backstop to isolation helper | missing pre-test state dir, symlink target change via lstat, files >1 MiB (skip content hash), nested file changes, sibling dirs out of scope |
| slow-open-empty-previews-and-zombie-sessions-1-4 | Audit and migrate existing test helpers to isolated env | helpers building but not spawning (out of scope tag b), indirect spawn wrappers, inline subprocess calls outside helpers |
| slow-open-empty-previews-and-zombie-sessions-1-5 | Document test-isolation contract in CLAUDE.md | section searchable for "test isolation" / "XDG_CONFIG_HOME", no lint claim |

### Phase 2: Capture Pipeline Hardening (Component E)
status: approved
approved_at: 2026-05-22

**Goal**: Stop a single per-session `ShowEnvironment` failure from aborting the whole tick and poisoning capture for every later session in the same tick.

**Why this order**: Smallest, most surgical fix; independent of the identity primitive and the singleton surgery. Landing it early eliminates the abort-on-error path that amplifies the GC race, so subsequent phases benefit from a healthier capture pipeline during their own integration tests. Independently shippable.

**Acceptance**:
- [ ] Per-session loop in `CaptureStructure` (`internal/state/capture.go`) logs WARN under `ComponentDaemon` and continues on per-session error rather than returning
- [ ] New typed sentinel `tmux.ErrNoSuchSession` exists in `internal/tmux/`; per-session tmux calls wrap stderr `"no such session"` once at the package boundary; daemon-layer classification uses `errors.Is`
- [ ] Post-loop discriminator: when `len(keep) > 0 && len(sessions) == 0`, all-natural-churn proceeds with an empty index; any anomalous error returns a wrapped error so `captureAndCommit` skips Commit + GC
- [ ] Pre-loop calls (`ListSessionNames`, `ListAllPanesWithFormat`, `parsePaneRows`) remain fail-fatal — no regression
- [ ] Empty `keep` returns empty index without error (existing behaviour preserved)
- [ ] Unit tests cover: single-session failure with surviving siblings; all-anomalous abort with no Commit; all-natural-churn proceeds with empty Commit; pre-loop failure still aborts; logger receives WARN with session name and underlying error
- [ ] Logger plumbing through `CaptureStructure` chosen between the two spec-accepted options (parameter vs `WithLogger` variant); rationale captured in code or PR

#### Tasks
status: approved
approved_at: 2026-05-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| slow-open-empty-previews-and-zombie-sessions-2-1 | Introduce tmux.ErrNoSuchSession sentinel and wrap ShowEnvironment at the tmux boundary | stderr substring vs exact match, mixed-case "No such session", error already wrapped, non-zero exit without that substring, EOF/empty stderr |
| slow-open-empty-previews-and-zombie-sessions-2-2 | Thread logger parameter into CaptureStructure (no behaviour change) | nil logger guard, restore-package call sites in integration tests, daemon call site in cmd/state_daemon.go, capture_test.go fixtures |
| slow-open-empty-previews-and-zombie-sessions-2-3 | Replace abort-on-error with per-session log-and-continue plus natural-churn discriminator | mixed natural-churn + anomalous in same tick, single anomalous among many natural-churn, all sessions succeed (no discriminator path), empty keep short-circuit preserved, parseShowEnvironment of empty env |
| slow-open-empty-previews-and-zombie-sessions-2-4 | Lock in fail-fatal pre-loop regression coverage | malformed pane row vs tmux exec failure, partial pane output, keep populated but pane list call fails, empty keep skipping pre-loop pane fetch |
| slow-open-empty-previews-and-zombie-sessions-2-5 | Wire daemon call site to pass real ComponentDaemon logger | logger not yet initialised at very first tick, log entries during all-natural-churn tick, nil session name guard, log level filtering disabled |

### Phase 3: Saver Creation Ordering (Component F)
status: approved
approved_at: 2026-05-22

**Goal**: Decouple `_portal-saver` session creation from daemon launch so `destroy-unattached=off` is in effect before any daemon process can exit, eliminating the `"no such session: _portal-saver"` log noise and the recovery doom-loop.

**Why this order**: Independent of the identity primitive; reshapes `BootstrapPortalSaver` create→option→respawn flow. Lands before Phase 4 because Component A's escalation path (kill-and-recreate) relies on the placeholder-then-respawn ordering being correct; validating F in isolation removes a confounding variable from Phase 4 integration tests. Independently valuable — closes a user-visible log-noise symptom on its own.

**Acceptance**:
- [ ] `createPortalSaverWithRetry` (or its successor) creates `_portal-saver` with placeholder `sh -c 'exec tail -f /dev/null'` rather than `portal state daemon`
- [ ] `SetSessionOption("destroy-unattached", "off")` runs against the now-stable placeholder session — no `"no such session"` errors
- [ ] `tmux respawn-pane -k -t _portal-saver 'portal state daemon'` replaces the placeholder with the real daemon, reusing the existing `RespawnPane` method on `*tmux.Client` without signature changes
- [ ] Readiness barrier polls for `daemon.pid` to exist AND `state.IdentifyDaemon` against its contents to return `IdentifyIsPortalDaemon`, bounded to 2 s total at 50 ms cadence; on timeout logs WARN and returns (best-effort)
- [ ] Integration test: clean bootstrap produces `_portal-saver` with `destroy-unattached=off` AND pane process is `portal state daemon`, with zero `"no such session: _portal-saver"` log entries
- [ ] Integration test: lock-loser daemon exit does NOT destroy the session — session persists for next bootstrap to evaluate
- [ ] Environment-inheritance acceptance: post-F `tmux show-environment -t _portal-saver` output for daemon-relevant vars (`XDG_CONFIG_HOME`, `HOME`, `PATH`) is identical to pre-F baseline
- [ ] Existing daemon-saver integration tests pass without modification

#### Tasks
status: approved
approved_at: 2026-05-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| slow-open-empty-previews-and-zombie-sessions-3-1 | Split saver command constants into placeholder and daemon variants | macOS BSD sleep infinity rejection rationale captured in-source, PortalSaverName export untouched, no behaviour change in this task (constants only) |
| slow-open-empty-previews-and-zombie-sessions-3-2 | Reorder BootstrapPortalSaver to create-placeholder, set-option, respawn-daemon | createPortalSaverWithRetry passes placeholder, RespawnPane reused without signature change, SetSessionOption call site preserved, lock-loser exit no longer destroys session, concurrent-bootstrap HasSession success path preserved |
| slow-open-empty-previews-and-zombie-sessions-3-3 | Add post-respawn readiness barrier polling daemon.pid + state.IdentifyDaemon | daemon.pid missing in early ticks treated as not-ready, IdentifyDaemon transient ps failure retried, IdentifyNotPortalDaemon resolves only via timeout, IdentifyDead retried, 2 s ceiling via deadline at 50 ms cadence, timeout WARN per spec, best-effort return on timeout |
| slow-open-empty-previews-and-zombie-sessions-3-4 | Compose unhealthy-saver recreate path with new ordering | placeholder-only saver from a crashed prior bootstrap presents as unhealthy and is recycled, kill-session targets the placeholder, no persistent placeholder leak across crashes, EnsurePortalSaverVersion still delegates through BootstrapPortalSaver unchanged |
| slow-open-empty-previews-and-zombie-sessions-3-5 | Integration test for clean bootstrap end-state and lock-loser persistence | uses portaltest.NewIsolatedStateEnv from Phase 1, zero "no such session: _portal-saver" log entries during bootstrap window, pane process verified via list-panes -F '#{pane_pid}' + ps -o args=, destroy-unattached=off via show-options, lock-loser simulation seeds competing daemon, session persists after lock-loser exit |
| slow-open-empty-previews-and-zombie-sessions-3-6 | Integration test for environment inheritance parity across respawn | tmux show-environment -t _portal-saver output identical to pre-F baseline for XDG_CONFIG_HOME / HOME / PATH, NewDetachedSessionNoCwd still passes no -e overrides, respawn-pane inherits session env, baseline computed from pre-F control |

### Phase 4: Daemon Singleton Enforcement (Components A + B + C)
status: approved
approved_at: 2026-05-22

**Goal**: Make Portal's daemon-singleton invariant enforceable end-to-end through three composing defences: kill-barrier escalation that deterministically reaches any prior daemon (A), bootstrap-time orphan sweep that handles the full pgrep set (B), and inode-replacement-resistant `AcquireDaemonLock` with a `daemon.pid` pre-check that closes the structural mechanism (C).

**Why this order**: These three components are tightly cohesive — they all attack the same singleton invariant, all consume the Phase 1 identity primitive, and the spec's composition story (A handles the recorded PID, B sweeps the rest, C is the backstop for unforeseen triggers) only delivers user-visible value when all three ship together. Splitting them would create thin phases that aren't independently meaningful milestones. After this phase, the three reporter-facing symptoms (slow open, empty previews, zombie sessions) are resolved under the trigger conditions actually observed.

**Acceptance**:
- [ ] Component A: `killSaverAndWaitForDaemon` adds a post-poll SIGKILL escalation guarded by `state.IdentifyDaemon` immediately before the `kill(2)` syscall, polling at 50 ms cadence for up to 1 s; no SIGTERM-first; on persistent aliveness logs WARN under `ComponentBootstrap` and proceeds
- [ ] Component A: snapshot-pair test asserts no final-flush GC runs on escalation-killed orphans — scrollback directory bytes-identical immediately before SIGKILL and 200 ms after the orphan exits
- [ ] Component A: legitimate daemon's normal SIGHUP-from-`kill-session` shutdown path is unaffected — `defaultShutdownFlush` still runs
- [ ] Component A: under steady-state-with-orphan, total bootstrap time is reduced by ~5 s (kill-barrier no longer adds a 5 s ceiling)
- [ ] Component B: new bootstrap step `SweepOrphanDaemons` inserted between current step 3 (Set `@portal-restoring`) and current step 4 (EnsureSaver); orchestrator becomes 11 steps; new ordering documented in `CLAUDE.md` bootstrap section
- [ ] Component B: enumeration uses canonical `pgrep -fx '^portal state daemon( |$)'`; legitimate set built from `tmux list-panes -t _portal-saver -F '#{pane_pid}'` (empty when `_portal-saver` absent); each non-legitimate PID is identity-checked then SIGKILLed; all errors logged WARN and swallowed (best-effort)
- [ ] Component B: integration test — given 3 daemons (1 saver-pane + 2 orphans) the step kills N−1 such that `pgrep -fxc 'portal state daemon'` returns 1; given clean state, the step sends zero signals (no `"sweep: killed orphan daemon"` log entries)
- [ ] Component C: `AcquireDaemonLock` pre-acquire reads `daemon.pid` via `state.ReadPIDFile`; returns `ErrDaemonLockHeld` if recorded PID is alive AND identity-checks as a `portal state daemon`; stale or wrong-identity PID falls through to the existing acquire
- [ ] Component C: post-flock `fstat(fd).Ino == stat(path).Ino` cross-check; mismatch releases flock and retries up to 3 attempts with 10 ms sleep between; persistent mismatch returns a wrapped error → daemon exits status 1 with WARN under `ComponentDaemon`
- [ ] Component C: daemon's `defaultDaemonRun` writes `daemon.pid` as the immediate next statement after a successful `acquireDaemonLock`; AST-walking unit test asserts the source ordering is preserved
- [ ] Component C: unit tests cover pre-check refuses on live recorded daemon, ignores stale PID, ignores wrong-identity PID, retry-on-mismatch succeeds on second attempt, retry-bound returns wrapped error with bounded total delay <100 ms, EWOULDBLOCK fallback still works, upgrade-path two-binary scenario converges to a single live daemon
- [ ] Composition: bootstrap against three concurrent daemons (1 legitimate + 2 orphans) converges to `pgrep -fxc 'portal state daemon' == 1` within ~6 s (A's escalation budget + B's sweep latency)

#### Tasks
status: approved
approved_at: 2026-05-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| slow-open-empty-previews-and-zombie-sessions-4-1 | Add SIGKILL escalation to killSaverAndWaitForDaemon with identity-check | PID recycled between poll and check, IdentifyDead mid-escalation, IdentifyNotPortalDaemon, transient identity error skips kill, identity-check immediately precedes kill(2), legitimate SIGHUP path unaffected, 50 ms poll cadence + 1 s budget, persistent aliveness logs WARN and proceeds |
| slow-open-empty-previews-and-zombie-sessions-4-2 | Add no-final-flush snapshot test for escalation-killed orphans | bytes-identical scrollback dir pre-SIGKILL vs 200 ms post-exit, real orphan spawned against isolated state dir, no defaultShutdownFlush invocation observable |
| slow-open-empty-previews-and-zombie-sessions-4-3 | Implement SweepOrphanDaemons core (pgrep + legitimate-set + identity + kill) | pgrep -fx '^portal state daemon( \|$)' canonical form, _portal-saver absent yields empty legitimate set, list-panes failure logs WARN and treats legitimate set as empty, identity-check transient skips PID, kill failure logged WARN swallowed, INFO log per killed PID, never SIGTERM first |
| slow-open-empty-previews-and-zombie-sessions-4-4 | Wire SweepOrphanDaemons as orchestrator step 4 between Set @portal-restoring and EnsureSaver | 11-step ordering preserved, CLAUDE.md bootstrap section updated, bootstrapadapter wiring for pgrep + identity + kill seam, best-effort warnings via existing warning channel, all errors swallowed |
| slow-open-empty-previews-and-zombie-sessions-4-5 | Integration test for SweepOrphanDaemons (3 daemons converge to 1, clean state sends zero signals) | uses portaltest.NewIsolatedStateEnv, 1 saver-pane + 2 orphans converges to pgrep -fxc == 1, clean state produces zero "sweep: killed orphan daemon" entries, recycled-PID identity refusal exercised, real subprocess fixtures |
| slow-open-empty-previews-and-zombie-sessions-4-6 | Add pre-acquire daemon.pid liveness check to AcquireDaemonLock | daemon.pid absent → proceed, dead recorded PID → proceed, wrong-identity PID → proceed, live identity-checked daemon → return ErrDaemonLockHeld without opening daemon.lock, ReadPIDFile error treated as absent, identity transient → proceed, pre-check runs before O_RDWR\|O_CREAT open |
| slow-open-empty-previews-and-zombie-sessions-4-7 | Add post-flock fstat-vs-stat inode cross-check with bounded retry | mismatch releases flock + closes fd before retry, 3-attempt bound with 10 ms sleeps (<100 ms total delay), persistent mismatch returns wrapped error → daemon exits status 1, ErrDaemonLockHeld path preserved, match on first attempt is happy path, retry succeeds on second attempt, EWOULDBLOCK fallback unchanged |
| slow-open-empty-previews-and-zombie-sessions-4-8 | AST-walking test asserts WritePIDFile immediately follows acquireDaemonLock in defaultDaemonRun | source parsed via go/parser + go/ast, statement-level adjacency check, guarded-equivalent (if err != nil { return } sandwich permitted), comments permitted, no other production call site of AcquireDaemonLock exists, test fails if any new statement intrudes |
| slow-open-empty-previews-and-zombie-sessions-4-9 | Integration test for Component C upgrade-path two-binary scenario | v(N) daemon holds lock, v(N+1) bootstrap spawns own daemon, new daemon acquires cleanly or refuses via pre-check, no destructive coexistence, uses portaltest.NewIsolatedStateEnv, real subprocesses via portalbintest |
| slow-open-empty-previews-and-zombie-sessions-4-10 | Composite integration test: A+B+C converge to pgrep -fxc == 1 within 6 s | 1 legitimate + 2 orphans (one with daemon.pid reference, one without), AcquireDaemonLock from fresh process refuses with ErrDaemonLockHeld post-bootstrap, scrollback dir stable across 10×1 s observations, uses portaltest.NewIsolatedStateEnv, integration build tag |

### Phase 5: Daemon Self-Supervision (Component D)
status: approved
approved_at: 2026-05-22

**Goal**: Bound inter-bootstrap orphan-daemon lifetime to single-digit ticks by adding a per-tick saver-membership self-check to the daemon loop that self-ejects via `os.Exit(0)` (no final flush) when membership is lost for N consecutive ticks.

**Why this order**: Comes after the singleton surgery because D's integration tests intentionally violate the saver-pane-process invariant and must stage past Component C's `AcquireDaemonLock` pre-check (via no `daemon.pid` or a known-dead PID). The hysteresis tuning (N) requires planning-phase empirical measurement of legitimate transient durations — a required mitigation per the Risk Summary — sequencing this last among fix components avoids blocking earlier phases on measurement work.

**Acceptance**:
- [ ] Per-tick self-check runs in the daemon main loop in `cmd/state_daemon.go` BEFORE `captureAndCommit`; sequence is `tmux has-session -t _portal-saver` → if present, `tmux list-panes -t _portal-saver -F '#{pane_pid}'` compared against `os.Getpid()`
- [ ] Counter increments on absent saver, missing/errored pane query, or pid mismatch; resets to 0 on matched pid
- [ ] When counter ≥ N: log INFO under `ComponentDaemon` (`"self-supervision: saver-membership lost for N consecutive ticks, exiting"`) and call `os.Exit(0)` — skipping all deferred handlers, so no final `captureAndCommit` / `gcOrphanScrollback` runs
- [ ] Stale `daemon.pid` is left in place by design; no cleanup logic deletes it pre-eject; Phase 4 Component C pre-check handles the stale value on next acquire
- [ ] Hysteresis constant `selfSupervisionHysteresisTicks` documented in-source with a comment citing measured worst-case transient ticks across the four scenarios (steady-state, attach/detach, `client-attached`, bootstrap kill-and-recreate), the 2× safety factor, the date of measurement, and the binary version; unit test asserts the constant ≥ 1
- [ ] Empirical measurement of legitimate transient durations completed across the four scenarios; measurement memo stored (referenced by the in-source comment) and N set per measured-worst × 2 within the single-digit-ticks ceiling
- [ ] Integration test: daemon against a tmux server with no `_portal-saver` exits within (N+1) tick intervals
- [ ] Integration test: external `respawn-pane -k -t _portal-saver 'sh -c "exec tail -f /dev/null"'` triggers self-eject within (N+1) tick intervals
- [ ] Unit test: stubbed `saverMembershipProbe` returning absent for k < N ticks then present does NOT exit; counter resets
- [ ] Integration test: scrollback-dir snapshots at the first failing tick and immediately post-`os.Exit(0)` are bytes-identical
- [ ] Integration test: legitimate first-tick self-check inside a freshly-created `_portal-saver` passes (pane pid matches `os.Getpid()`)

### Phase 6: Composite End-to-End Verification
status: approved
approved_at: 2026-05-22

**Goal**: Land the spec-mandated single composite integration test that reconstructs the reporter's failure scenario end-to-end and asserts the converged healthy state across A+B+C+D+E+F composition.

**Why this order**: Final phase by construction — verifies composition across every prior phase's deliverable. Per-component tests cannot catch composition regressions; this is the ship-readiness gate. Also captures the End-State Verification observables documented in the spec.

**Acceptance**:
- [ ] Integration test (tagged with the existing integration build tag pattern; placement in `cmd/` or `internal/restoretest/` decided by reuse of existing real-tmux scaffolding) starts a real tmux server with `_portal-saver` plus user sessions, spawns three `portal state daemon` processes (1 legitimate saver-pane + 2 orphans — one with `daemon.pid` reference, one without), and confirms the pre-fix state reproduces (`pgrep -fxc 'portal state daemon' == 3`; scrollback dir oscillates 0–1 `.bin` across ticks)
- [ ] Test invokes `portal open` (or the bootstrap orchestrator test entry point) against the new binary
- [ ] Post-bootstrap: `pgrep -fxc 'portal state daemon' == 1` within 6 s of `EnsureSaver` entry
- [ ] Post-bootstrap: scrollback directory stable across 10 consecutive 1 s observations — no `.bin` deletions or unexpected new files (A+B+E composition)
- [ ] Post-bootstrap: a fresh-process `AcquireDaemonLock` invocation refuses with `ErrDaemonLockHeld` (Component C pre-check verified on live state)
- [ ] Post-bootstrap: externally killing the legitimate daemon's `_portal-saver` pane triggers self-eject within (N+1) tick intervals (Component D in live context)
- [ ] Post-bootstrap: `_portal-saver`'s pane process is `portal state daemon` AND `tmux show-options -t _portal-saver destroy-unattached` reports `off` (Component F)
- [ ] Test uses `portaltest.NewIsolatedStateEnv` for state-dir isolation (Phase 1 helper); no developer-state mutations on test exit
