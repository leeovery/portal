---
phase: 3
phase_name: Saver Creation Ordering (Component F)
total: 6
---

## slow-open-empty-previews-and-zombie-sessions-3-1 | approved

### Task 3-1: Split saver command constants into placeholder and daemon variants

**Problem**: `internal/tmux/portal_saver.go` exposes a single `portalSaverCommand = "portal state daemon"` constant that wires both session-creation and pane-process responsibilities into the same string. Component F decouples those two roles — session creation must run a benign placeholder so `destroy-unattached=off` can be applied before the daemon ever starts. We need named constants for both shapes before reordering the call sites in 3-2.

**Solution**: Replace `portalSaverCommand` with two named, in-source-documented constants: `portalSaverPlaceholderCommand = "sh -c 'exec tail -f /dev/null'"` (used at create time) and `portalSaverDaemonCommand = "portal state daemon"` (used at respawn time). This is a constants-only change — no call sites change behaviour in this task; existing call sites are repointed at `portalSaverDaemonCommand` to keep current behaviour byte-identical.

**Outcome**: Both constants exist with doc-comments capturing (a) why the placeholder is `tail -f /dev/null` rather than `sleep infinity` (BSD `sleep` rejects `infinity` on macOS and exits immediately, recreating the very race F closes — spec § Component F, paragraph on placeholder rationale) and (b) what the daemon variant is for. `portalSaverCommand` is gone or aliased; `go build ./...` and `go test ./internal/tmux/...` pass with no behavioural diff vs. main.

**Do**:
- Edit `internal/tmux/portal_saver.go`: replace the single `portalSaverCommand` constant (currently line 33) with two constants:
  - `portalSaverPlaceholderCommand = "sh -c 'exec tail -f /dev/null'"`
  - `portalSaverDaemonCommand = "portal state daemon"`
- Doc-comment the placeholder constant explaining: (1) `exec tail -f /dev/null` blocks indefinitely on both macOS and Linux; (2) `sleep infinity` was rejected because BSD `sleep` on macOS treats `infinity` as a parse error and exits immediately, reproducing the race; (3) the placeholder is structurally incapable of writing to the state directory or contending for the daemon lock; (4) it lives until killed by `respawn-pane -k` or `tmux kill-session`.
- Doc-comment the daemon constant explaining it is the real pane process installed by `respawn-pane -k` after `destroy-unattached=off` is in effect.
- Update the existing call site in `createPortalSaverWithRetry` (line 399) to use `portalSaverDaemonCommand` so the diff is constants-only — no semantic change in this task.
- Leave `PortalSaverName` and all other exported names untouched.

**Acceptance Criteria**:
- [ ] `portalSaverPlaceholderCommand` exists with the literal value `"sh -c 'exec tail -f /dev/null'"` and a doc-comment citing the macOS BSD `sleep` rationale.
- [ ] `portalSaverDaemonCommand` exists with the literal value `"portal state daemon"`.
- [ ] No symbol named `portalSaverCommand` remains (or it is replaced — `grep -n portalSaverCommand internal/tmux/portal_saver.go` returns nothing).
- [ ] `createPortalSaverWithRetry` references `portalSaverDaemonCommand` (no behavioural change from main).
- [ ] `go build ./...` succeeds; `go test ./internal/tmux/...` passes unchanged from main.
- [ ] `PortalSaverName` constant unchanged (still exported as `_portal-saver`).

**Tests**:
- `"it exposes portalSaverPlaceholderCommand as the literal sh -c 'exec tail -f /dev/null'"` — unit assertion against the constant value (a single-line constant-value test in `portal_saver_test.go`).
- `"it exposes portalSaverDaemonCommand as the literal portal state daemon"` — unit assertion against the constant value.
- `"existing portal_saver unit tests pass unchanged"` — verifies no behavioural regression from the constants-only rename.

**Edge Cases**:
- macOS BSD `sleep infinity` rejection — rationale captured in-source so future maintainers don't "simplify" the placeholder back to `sleep infinity`.
- `PortalSaverName` export must remain untouched — downstream packages and tests import it by name.
- This task is constants-only: no behaviour change. Reordering happens in 3-2.

**Context**:
> Spec § Component F (paragraph beginning "The placeholder choice"): "`sleep infinity` was considered and rejected because macOS' BSD `sleep` requires a numeric argument and exits immediately when given `infinity` — which would recreate exactly the race this component is meant to close. `tail -f /dev/null` blocks indefinitely on both platforms, does NOT exit on terminal-signal artefacts, and is widely available; it lives until killed by `respawn-pane -k` or `tmux kill-session`."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F (approx lines 344–394).

## slow-open-empty-previews-and-zombie-sessions-3-2 | approved

### Task 3-2: Reorder BootstrapPortalSaver to create-placeholder, set-option, respawn-daemon

**Problem**: `BootstrapPortalSaver` (lines 266–288) creates `_portal-saver` with `portal state daemon` as the initial pane process AND then sets `destroy-unattached=off`. If the daemon exits between those two tmux calls (notably the lock-loser case) tmux self-destroys the session before the option is applied, producing the `"no such session: _portal-saver"` log noise and a recovery doom-loop. The fix is to decouple session creation from daemon launch: create with the placeholder, set the option on the now-stable session, then respawn the pane with the real daemon command.

**Solution**: Rework the create branch of `BootstrapPortalSaver` (and `createPortalSaverWithRetry`) to use the three-step ordering. Step (1) `createPortalSaverWithRetry` passes `portalSaverPlaceholderCommand` (from 3-1) to `NewDetachedSessionNoCwd`. Step (2) the existing `SetSessionOption(PortalSaverName, "destroy-unattached", "off")` call runs against the placeholder session — guaranteed-alive because `tail -f /dev/null` does not exit. Step (3) `c.RespawnPane(PortalSaverName, portalSaverDaemonCommand)` replaces the placeholder with the real daemon. `RespawnPane`'s existing signature (`RespawnPane(target, command string) error` at `internal/tmux/tmux.go:708`) is reused verbatim — no signature change. The readiness barrier from 3-3 is inserted as a follow-on after the respawn.

**Outcome**: After `BootstrapPortalSaver` returns successfully on a clean bootstrap, `tmux list-panes -t _portal-saver -F '#{pane_pid}'` plus `ps -o args= -p <pid>` shows `portal state daemon`, and `tmux show-options -t _portal-saver destroy-unattached` reports `off`. Lock-loser daemon exit does not destroy the session — the next bootstrap finds `_portal-saver` present. Zero `"no such session: _portal-saver"` log entries during the bootstrap window.

**Do**:
- In `internal/tmux/portal_saver.go`, edit `createPortalSaverWithRetry` (line 396): pass `portalSaverPlaceholderCommand` to `NewDetachedSessionNoCwd`. The retry/concurrency-race logic (HasSession re-probe on attempt failure) is preserved unchanged.
- In `BootstrapPortalSaver` (line 266), restructure the create flow to the three-step order:
  1. (If creation needed) `createPortalSaverWithRetry(c)` — creates with placeholder.
  2. `c.SetSessionOption(PortalSaverName, "destroy-unattached", "off")` — moved/kept so it runs against the placeholder session, BEFORE the respawn. Preserve the existing error-wrapping (`fmt.Errorf("bootstrap _portal-saver: set destroy-unattached: %w", err)`).
  3. New call: `c.RespawnPane(PortalSaverName, portalSaverDaemonCommand)`. On error, wrap as `fmt.Errorf("bootstrap _portal-saver: respawn daemon: %w", err)` and return — the respawn is structurally required for the daemon to start; failing it must surface to the orchestrator.
- The "session-already-present-and-alive" branch (sessionPresent=true, BootstrapAliveCheck=true) must NOT execute the respawn — that path is a no-op happy path and the existing daemon is already running.
- Doc-comment the function with the new three-step ordering and the rationale (Component F race fix).
- The readiness barrier (Task 3-3) is layered on after the respawn, so leave a clearly-named extension point (e.g., a TODO referencing 3-3 or a stub call) — but do not implement it in this task.
- Update `portal_saver_test.go` to reflect the new ordering at unit level: a fake `Commander` should observe the call sequence `new-session ... 'sh -c exec tail -f /dev/null'` → `set-option ... destroy-unattached off` → `respawn-pane -k ... 'portal state daemon'`.

**Acceptance Criteria**:
- [ ] `createPortalSaverWithRetry` calls `NewDetachedSessionNoCwd(PortalSaverName, portalSaverPlaceholderCommand)`.
- [ ] In the create branch of `BootstrapPortalSaver`, the tmux call order observed by a recorder is: create-session → set-option destroy-unattached=off → respawn-pane.
- [ ] `RespawnPane` is called with `(PortalSaverName, portalSaverDaemonCommand)` and its existing signature is unchanged.
- [ ] If `SetSessionOption` fails, the function returns wrapped error; if `RespawnPane` fails, the function returns a wrapped `"respawn daemon"` error.
- [ ] The session-present-and-alive happy path does NOT issue a respawn (idempotency preserved).
- [ ] Concurrent-bootstrap race (HasSession returns true after a failed new-session) still resolves as success — `createPortalSaverWithRetry`'s existing retry logic is untouched.
- [ ] `go vet ./...` and `go test ./internal/tmux/...` pass.

**Tests**:
- `"BootstrapPortalSaver issues create-then-set-option-then-respawn in order on a clean bootstrap"` — uses a recording `Commander` to assert the exact argv sequence.
- `"BootstrapPortalSaver does not respawn when session is present and daemon is alive"` — idempotent happy path; no respawn-pane call observed.
- `"BootstrapPortalSaver returns wrapped error when SetSessionOption fails"` — fault injection on the set-option call.
- `"BootstrapPortalSaver returns wrapped error when RespawnPane fails"` — fault injection on respawn-pane; error message contains `"respawn daemon"`.
- `"createPortalSaverWithRetry uses placeholder command"` — recording Commander asserts the new-session argv carries `sh -c 'exec tail -f /dev/null'`.
- `"concurrent-bootstrap race still treats existing session as success"` — HasSession returns true after first new-session failure; function returns nil.

**Edge Cases**:
- Lock-loser daemon exit between respawn and the next bootstrap: `destroy-unattached=off` is already set, so the session persists for the next bootstrap to evaluate.
- Concurrent bootstraps: the HasSession re-probe in `createPortalSaverWithRetry` still resolves the race; the respawn step runs unconditionally in the post-creation block and is idempotent enough (re-respawning a fresh daemon would briefly kill-and-restart it, but that path only fires when our own bootstrap created the session — concurrent winners exit before reaching respawn).
- Session-present-and-alive path skips respawn — daemon already running and healthy.
- `RespawnPane` signature unchanged — if the existing site uses different shape, scaffold a check at the start of implementation to confirm; spec authorises adapting only if shape differs.

**Context**:
> Spec § Component F, "New behaviour" enumeration: "1. Create the saver with a benign placeholder command... 2. Set `destroy-unattached=off`... 3. Respawn the pane with the real command: `tmux respawn-pane -k -t {PortalSaverName} 'portal state daemon'`."
>
> Spec § Component F, "Why this ordering is safe": "The placeholder is structurally incapable of running portal logic — it cannot write to the state directory or contend for the lock. The window between create and respawn is bounded by two tmux command latencies (likely <50 ms) during which no portal-daemon work happens."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F.

## slow-open-empty-previews-and-zombie-sessions-3-3 | approved

### Task 3-3: Add post-respawn readiness barrier polling daemon.pid + state.IdentifyDaemon

**Problem**: After `respawn-pane -k` installs the daemon in step (3) of the new ordering (Task 3-2), the daemon needs a finite warm-up window before `daemon.pid` is written and the lock is acquired. Subsequent bootstrap steps (Restore, EagerSignalHydrate) assume a healthy daemon. Without a readiness barrier they race the respawn — observing transient absent-pid or "not yet identified" states that mask real daemon failures.

**Solution**: After the respawn call in `BootstrapPortalSaver`, poll for `daemon.pid` to exist AND for `state.IdentifyDaemon` against its contents to return `IdentifyIsPortalDaemon`. Bounded to 2 s total at 50 ms cadence. On success return nil. On timeout log WARN under `ComponentBootstrap` with the literal message form `"saver respawn: daemon did not come up within 2s"` and return nil (best-effort — bootstrap continues). On transient `IdentifyDaemon` errors retry until the deadline; on `IdentifyDead` retry (daemon will rewrite the PID file post-fork) until the deadline; on `IdentifyNotPortalDaemon` only the deadline resolves (recycled PID belonging to another process — rare; spec mandates timeout resolution).

**Outcome**: `BootstrapPortalSaver`'s create branch returns only after the daemon has written `daemon.pid` and `state.IdentifyDaemon` confirms the PID is a live `portal state daemon`, OR after 2 s have elapsed (in which case a single WARN is emitted and the function still returns nil). Subsequent orchestrator steps observe a stable daemon in the happy path.

**Do**:
- Add a new helper to `internal/tmux/portal_saver.go`: `waitForSaverDaemonReady(stateDir string) error`. It must use exported package-level seams so unit tests can stub timing and identity behaviour without spawning real processes.
- Add the following package-level seams (mirroring the kill-barrier pattern at lines 113–134):
  - `saverReadinessReadPID = state.ReadPIDFile`
  - `saverReadinessIdentify = state.IdentifyDaemon`
  - `saverReadinessPollInterval = 50 * time.Millisecond` (`var`, shrinkable in tests)
  - `saverReadinessTimeout = 2 * time.Second` (`var`, shrinkable in tests)
- Reuse `killBarrierLogger` (the existing `BarrierLogger`) as the WARN sink — Component F's readiness barrier is the same observable shape (one WARN on timeout).
- Implementation skeleton:
  1. Compute `deadline := time.Now().Add(saverReadinessTimeout)`.
  2. Loop with `time.NewTicker(saverReadinessPollInterval)`:
     - Read `pid, err := saverReadinessReadPID(stateDir)`. On `ErrPIDFileAbsent` or any read error: continue (not ready yet).
     - Call `result, err := saverReadinessIdentify(pid)`. If `err != nil` (transient ps failure): continue. If `result == state.IdentifyIsPortalDaemon`: return nil. Otherwise (IdentifyDead, IdentifyNotPortalDaemon): continue.
     - Check `time.Now().Before(deadline)`; if not, emit WARN via `killBarrierLogger.Warn(state.ComponentBootstrap, "saver respawn: daemon did not come up within %v", saverReadinessTimeout)` and return nil.
- Call `waitForSaverDaemonReady(stateDir)` from `BootstrapPortalSaver` immediately after the successful `RespawnPane` call. The error return is always nil under the best-effort contract; capturing the return is purely defensive.
- Doc-comment the helper with the spec's success/timeout contract.

**Acceptance Criteria**:
- [ ] `waitForSaverDaemonReady(stateDir string) error` exists in `internal/tmux/portal_saver.go`.
- [ ] Polls at 50 ms cadence; total wait bounded to 2 s via a deadline computed once at entry.
- [ ] Returns nil immediately when `state.ReadPIDFile` returns a PID AND `state.IdentifyDaemon` returns `IdentifyIsPortalDaemon`.
- [ ] On timeout, emits exactly one WARN via `killBarrierLogger.Warn` under `state.ComponentBootstrap` with a message containing `"saver respawn: daemon did not come up within"` and returns nil.
- [ ] Treats `IdentifyDead`, `IdentifyNotPortalDaemon`, `ErrPIDFileAbsent`, transient ps errors, and PID-file read errors as "not ready" — continues polling until deadline.
- [ ] Called from `BootstrapPortalSaver` immediately after `RespawnPane` (only in the create branch; not in the session-present-and-alive branch).
- [ ] Package-level seams `saverReadinessReadPID`, `saverReadinessIdentify`, `saverReadinessPollInterval`, `saverReadinessTimeout` exist and are individually overridable from tests.

**Tests**:
- `"waitForSaverDaemonReady returns nil immediately when PID present and identifies as portal daemon"` — stubbed seams return `(pid, nil)` and `IdentifyIsPortalDaemon` on first tick.
- `"waitForSaverDaemonReady retries while daemon.pid is absent then succeeds"` — first k probes return `ErrPIDFileAbsent`; subsequent probe returns a valid PID + `IdentifyIsPortalDaemon`.
- `"waitForSaverDaemonReady retries on transient IdentifyDaemon ps failure"` — IdentifyDaemon returns `(_, errPSExec)` for first 3 probes, then `(IdentifyIsPortalDaemon, nil)`.
- `"waitForSaverDaemonReady retries on IdentifyDead until next pid write"` — IdentifyDaemon returns `IdentifyDead` then `IdentifyIsPortalDaemon`.
- `"waitForSaverDaemonReady times out and emits WARN when daemon never identifies"` — IdentifyDaemon always returns `IdentifyNotPortalDaemon`; assert recording BarrierLogger captures exactly one WARN under `ComponentBootstrap` with the expected message; function returns nil.
- `"waitForSaverDaemonReady total wall-clock is bounded by saverReadinessTimeout"` — shrink the timeout to 100 ms in the test; assert elapsed ≤ ~150 ms.
- `"BootstrapPortalSaver invokes waitForSaverDaemonReady after RespawnPane on the create path"` — call-order assertion via stubbed seams.

**Edge Cases**:
- `daemon.pid` missing in early ticks — counted as not-ready; loop continues.
- `IdentifyDaemon` transient `ps` failure — retried until deadline.
- `IdentifyNotPortalDaemon` (recycled PID belonging to a non-portal process) — only the deadline resolves; spec explicitly requires this.
- `IdentifyDead` — retried; daemon may rewrite the PID file shortly after fork.
- Best-effort return on timeout: WARN is logged, nil is returned, orchestrator proceeds.
- The 2 s ceiling must be enforced by a `deadline := time.Now().Add(saverReadinessTimeout)` computed once, not by counting ticks — clock-skew safe.

**Context**:
> Spec § Component F, paragraph 4: "Readiness barrier. After `respawn-pane`, `BootstrapPortalSaver` polls for `daemon.pid` to exist AND for `state.IdentifyDaemon` against its contents to return `IdentifyIsPortalDaemon`. Bounded to **2 s total** with **50 ms poll cadence**. On timeout: log WARN (`"saver respawn: daemon did not come up within 2s"`) and return — best-effort, the bootstrap continues. On success: return."
>
> Phase 1 has shipped `state.IdentifyDaemon(pid int) (IdentifyResult, error)` with the three-result contract (`IdentifyIsPortalDaemon` / `IdentifyNotPortalDaemon` / `IdentifyDead`) plus a transient-error case. This task depends on that primitive.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F (paragraph 4 — Readiness barrier).

## slow-open-empty-previews-and-zombie-sessions-3-4 | approved

### Task 3-4: Compose unhealthy-saver recreate path with new ordering

**Problem**: When `BootstrapPortalSaver` encounters an existing saver session with a dead daemon (lines 269–275), it calls `killSaverAndWaitForDaemonFn` and falls through to recreate. After 3-2 the recreate path is "create-placeholder, set-option, respawn-daemon" — but we also need the unhealthy-saver path to compose cleanly with that new ordering, including the case where a prior bootstrap crashed mid-respawn and the saver currently hosts the placeholder process (not a real daemon). The `BootstrapAliveCheck` correctly reports this as unhealthy (no live `daemon.pid`), so the existing kill-and-recreate path runs — but we need to verify the composition works end-to-end and that the `EnsurePortalSaverVersion` callers still delegate correctly.

**Solution**: Audit the unhealthy-saver branch of `BootstrapPortalSaver` and ensure it falls through to the new create-placeholder → set-option → respawn-daemon → readiness-barrier sequence built in 3-2/3-3. No new code path required if 3-2 already routes the recreate fall-through through `createPortalSaverWithRetry` and the subsequent set-option + respawn + readiness-barrier block. Add explicit unit coverage for the placeholder-leak scenario (prior bootstrap crashed between F's steps 2 and 3) and verify `EnsurePortalSaverVersion` still delegates through `BootstrapPortalSaver` unchanged.

**Outcome**: A saver session lingering with just the placeholder (no live daemon) is detected by `BootstrapAliveCheck` (since `daemon.pid` is absent or stale), killed via `killSaverAndWaitForDaemonFn`, and recreated via the new ordering. No persistent placeholder leak survives any single recovery bootstrap. `EnsurePortalSaverVersion` remains a thin wrapper around `BootstrapPortalSaver` — its kill+delegate flow inherits the new ordering by composition.

**Do**:
- Read `BootstrapPortalSaver` after 3-2/3-3 changes have landed. Confirm the unhealthy-saver branch (lines 269–275 in current main) falls through to the new ordering. If the fall-through still routes to `createPortalSaverWithRetry` + set-option + respawn + readiness, no code change needed; the task is verifying composition + adding coverage.
- If a code change IS needed (e.g., the existing branch shape needs tweaking to ensure set-option runs on the placeholder and not on a now-dead session), apply the minimal patch and document it inline.
- Confirm `EnsurePortalSaverVersion` (line 328) still delegates to `BootstrapPortalSaver` and inherits the new ordering automatically. No signature changes; the matrix-driven kill decision is unchanged.
- Update or add `internal/tmux/portal_saver_test.go` unit cases for:
  - "Saver exists with placeholder process only" — BootstrapAliveCheck stubbed to return false; assert kill → create-placeholder → set-option → respawn → readiness sequence.
  - "kill-session on unhealthy saver targets the placeholder" — argv-recording fake observes the kill-session call before the new-session call.
  - "EnsurePortalSaverVersion routes unhealthy-saver kill through new ordering" — version mismatch + alive=false (no kill row) and alive=true+mismatch (kill row); both eventually reach BootstrapPortalSaver and observe the new ordering.

**Acceptance Criteria**:
- [ ] Unhealthy-saver branch falls through cleanly to the 3-2 ordering — verified by call-order assertion in unit test.
- [ ] Placeholder-only-lingering saver (no live `daemon.pid`) is recycled via `killSaverAndWaitForDaemonFn` → recreate path; no infinite-loop or panic.
- [ ] `EnsurePortalSaverVersion` continues to delegate to `BootstrapPortalSaver` with unchanged signature and inherits the new ordering.
- [ ] `kill-session` argv recorded before the subsequent `new-session 'sh -c exec tail -f /dev/null'` argv.
- [ ] No persistent placeholder leak — a single recovery bootstrap restores the daemon.
- [ ] `go test ./internal/tmux/...` passes including the new placeholder-leak coverage.

**Tests**:
- `"BootstrapPortalSaver recycles a placeholder-only saver via kill+recreate with new ordering"` — BootstrapAliveCheck stubbed to false; argv recorder asserts kill-session → new-session(placeholder) → set-option(destroy-unattached=off) → respawn-pane(daemon) → readiness-barrier seam invoked.
- `"EnsurePortalSaverVersion alive=true+mismatch flows through new BootstrapPortalSaver ordering"` — version-mismatch matrix kill row; argv recorder confirms the post-kill recreate uses the placeholder.
- `"EnsurePortalSaverVersion alive=false+anything skips kill and still uses new ordering"` — no-kill row; the existing-session-absent branch creates with placeholder.
- `"No persistent placeholder leak across single recovery cycle"` — simulate a crashed prior bootstrap (saver present, daemon.pid absent) → next BootstrapPortalSaver returns with daemon healthy (readiness barrier reports IdentifyIsPortalDaemon on stubbed seams).

**Edge Cases**:
- Placeholder-only saver from a crashed prior bootstrap presents as unhealthy via `BootstrapAliveCheck` (daemon.pid absent or stale) — existing alive-check handles it; no new logic required.
- `kill-session` targets the placeholder (the only pane process at that moment) — `KillSession` doesn't care which pane process is running, it just tears the session down.
- `EnsurePortalSaverVersion` is not modified — the kill decision matrix remains correct, and the delegation to `BootstrapPortalSaver` carries the new ordering for free.
- Tolerance of `KillSession` errors in the unhealthy-saver branch is preserved (the leading `_ =` discard is intentional).

**Context**:
> Spec § Component F, "Interaction with kill-barrier (Components A and B)": "When `BootstrapPortalSaver` encounters an existing saver with a dead daemon (lines 269-275 — `BootstrapAliveCheck` returns false), it calls `killSaverAndWaitForDaemonFn` and falls through to recreate. With Components A and B in place, the kill phase is reliable, and the recreate path now uses the placeholder-then-respawn ordering."
>
> Spec § Component F, "No persistent placeholder leak": "Even if a bootstrap crashes between F's steps 2 and 3, the next bootstrap sees the unhealthy saver (no live daemon.pid) and recovers via the existing path."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F (paragraphs on Interaction with kill-barrier and No persistent placeholder leak).

## slow-open-empty-previews-and-zombie-sessions-3-5 | approved

### Task 3-5: Integration test for clean bootstrap end-state and lock-loser persistence

**Problem**: Unit tests with argv recorders prove the call sequence is correct but do not prove the actual end-state against a real tmux server. Spec acceptance criteria require: (a) zero `"no such session: _portal-saver"` log entries during clean bootstrap; (b) `tmux show-options -t _portal-saver destroy-unattached` reports `off` AND the pane process is `portal state daemon`; (c) lock-loser daemon exit does NOT destroy the session. These are integration-level invariants.

**Solution**: Add a real-tmux integration test in `internal/tmux/portal_saver_integration_test.go` (or a sibling file) that uses `tmuxtest.New` to spin up an isolated tmux server, `portalbintest.BuildPortalBinary` + PATH-staging so `portal state daemon` resolves, and `portaltest.NewIsolatedStateEnv` (Phase 1) so the test never touches the developer's real state directory. The test invokes `BootstrapPortalSaver` against a freshly-created state dir, asserts end-state observables, then simulates a lock-loser scenario by seeding a competing daemon (via `state.WritePIDFile` of the test process PID + a held flock, or by running a second daemon to acquisition then introducing the new bootstrap) and asserts the session persists after the lock-loser daemon exits.

**Outcome**: Two real-tmux integration test functions land in `internal/tmux/portal_saver_integration_test.go`:
- `TestBootstrapPortalSaver_CleanBootstrap_EndState` — proves the clean-bootstrap end-state.
- `TestBootstrapPortalSaver_LockLoser_SessionPersists` — proves the lock-loser daemon exit does not destroy the session.
Both pass on a developer machine with tmux installed and fail when the saver-creation ordering regresses.

**Do**:
- Add `TestBootstrapPortalSaver_CleanBootstrap_EndState` to `internal/tmux/portal_saver_integration_test.go`:
  1. `tmuxtest.SkipIfNoTmux(t)`; build the portal binary via `portalbintest.BuildPortalBinary(t)`; stage on PATH via `portalbintest.StagePortalBinary(t, bin)`.
  2. `env, stateDir := portaltest.NewIsolatedStateEnv(t)` — Phase 1 helper for state-dir isolation.
  3. `sock := tmuxtest.New(t)`; construct a `*tmux.Client` against the socket; ensure the spawned tmux server inherits the isolated env (PATH + XDG_CONFIG_HOME).
  4. Pre-condition: assert `_portal-saver` not present.
  5. Call `tmux.BootstrapPortalSaver(client, stateDir)`; assert no error.
  6. Post-condition assertions (poll up to 2 s for daemon readiness to settle if needed — the readiness barrier already does this, but allow tmux's option-write to flush):
     - `client.HasSession(PortalSaverName)` returns true.
     - `tmux show-options -t _portal-saver destroy-unattached` output contains `off` (use `client` raw command or `tmux.Commander.RunRaw`).
     - `tmux list-panes -t _portal-saver -F '#{pane_pid}'` returns a pid; `ps -o args= -p <pid>` output contains `portal state daemon` (and not `tail -f /dev/null`).
     - Log scan: capture the test's WARN/ERROR logger output and assert zero entries matching the substring `"no such session: _portal-saver"`.
  7. Cleanup via `t.Cleanup` (tmuxtest registers kill-server already; the isolated state env auto-asserts no developer-state mutation).
- Add `TestBootstrapPortalSaver_LockLoser_SessionPersists`:
  1. Same scaffolding as above.
  2. Pre-seed a competing daemon scenario: spawn `portal state daemon` against the same isolated stateDir (using `exec.Command` with `cmd.Env = env`), wait until it has written `daemon.pid` and identifies via `state.IdentifyDaemon` (loop ≤2 s).
  3. Invoke `tmux.BootstrapPortalSaver(client, stateDir)` against the same stateDir. The new daemon, started inside `_portal-saver`'s pane via respawn, will lose the lock (Component C / existing AcquireDaemonLock) and exit cleanly.
  4. Wait up to 2 s for the respawned daemon to exit (poll `tmux list-panes -t _portal-saver -F '#{pane_pid}'` and `ps` for absence of `portal state daemon` whose pid was not the seeded competing daemon).
  5. Assert `client.HasSession(PortalSaverName)` still returns true — `destroy-unattached=off` prevents auto-destroy.
  6. Cleanup: kill the seeded competing daemon (or rely on `tmuxtest.New`'s `t.Cleanup` killing the tmux server, which SIGHUPs everything connected).
- Place the tests under the existing `cmd/integration` / `tmux_test` package convention used by `portal_saver_integration_test.go` (no `t.Parallel`).

**Acceptance Criteria**:
- [ ] `TestBootstrapPortalSaver_CleanBootstrap_EndState` exists in `internal/tmux/portal_saver_integration_test.go` (or sibling) and skips when tmux is absent via `tmuxtest.SkipIfNoTmux`.
- [ ] Uses `portaltest.NewIsolatedStateEnv(t)` for state-dir isolation — assertion that the developer's real `~/.config/portal/state/` is untouched on test exit is automatic via Phase 1's t.Cleanup backstop.
- [ ] Asserts: HasSession returns true; `tmux show-options -t _portal-saver destroy-unattached` contains `off`; pane pid resolves to `ps args` containing `portal state daemon`; zero `"no such session: _portal-saver"` log entries in captured logger output.
- [ ] `TestBootstrapPortalSaver_LockLoser_SessionPersists` exists and proves the lock-loser daemon exit does not destroy the saver session.
- [ ] Both tests fail if 3-2's ordering is reverted (verify by temporarily reordering to expose the regression during implementation).
- [ ] No `t.Parallel` (portal-wide convention).

**Tests**:
- `"TestBootstrapPortalSaver_CleanBootstrap_EndState verifies destroy-unattached=off and daemon pane process on clean bootstrap"` — the test itself.
- `"TestBootstrapPortalSaver_CleanBootstrap_EndState emits zero 'no such session: _portal-saver' log entries"` — log-scan sub-assertion within the same test.
- `"TestBootstrapPortalSaver_LockLoser_SessionPersists keeps the saver alive after a lock-loser daemon exits"` — the test itself.
- `"both integration tests skip cleanly when tmux is absent"` — verified by `tmuxtest.SkipIfNoTmux`.

**Edge Cases**:
- Test runs without tmux on PATH — skipped via `tmuxtest.SkipIfNoTmux`.
- Test runs without `portal` binary — built fresh via `portalbintest.BuildPortalBinary` and PATH-staged so tmux resolves it.
- Developer real state dir not touched — guaranteed by `portaltest.NewIsolatedStateEnv` plus its fingerprint-diff t.Cleanup.
- Lock-loser daemon exit timing: the seeded competing daemon must be confirmed alive AND owning the lock before the bootstrap fires; otherwise the bootstrap's daemon could win the lock and the test asserts a different invariant.
- The respawned daemon's pane pid is observed via `list-panes -F '#{pane_pid}'` after the readiness barrier (which in this case fails to identify and times out per spec — log-scan assertion accommodates the timeout WARN, which is distinct from `"no such session"`).
- `destroy-unattached=off` must be set BEFORE the lock-loser daemon exits — that's the invariant being proved by `TestBootstrapPortalSaver_LockLoser_SessionPersists`.

**Context**:
> Spec § Component F, Acceptance criteria: "No 'no such session' log line on create… destroy-unattached=off is set before daemon process can exit… Lock-loser daemon does not destroy the session. Simulate a lock-loser scenario (another daemon already holds the singleton): the new bootstrap creates `_portal-saver`, applies `destroy-unattached=off`, respawns the daemon, and the daemon exits cleanly as lock-loser — `_portal-saver` remains present after the daemon exits. Verified by integration test."
>
> Phase 1 has shipped `portaltest.NewIsolatedStateEnv(t)`; consume it here for state isolation. `portalbintest.BuildPortalBinary` + `StagePortalBinary` are the canonical helpers per the CLAUDE.md `portalbintest` description.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F, Acceptance criteria bullet 1 (no "no such session" log line on create), bullet 2 (destroy-unattached=off set before daemon can exit), bullet 3 (lock-loser daemon does not destroy the session).

## slow-open-empty-previews-and-zombie-sessions-3-6 | approved

### Task 3-6: Integration test for environment inheritance parity across respawn

**Problem**: Component F changes the saver's initial pane process from `portal state daemon` to a placeholder + later respawn. Spec § Component F "Environment inheritance across respawn" requires that the respawned daemon sees the same environment it would have seen as the original initial pane command pre-F. If `respawn-pane` somehow did NOT inherit the session's env (or if a future change to `NewDetachedSessionNoCwd` introduced `-e KEY=VAL` overrides), the daemon could behave differently — e.g., reading the wrong `XDG_CONFIG_HOME` and corrupting a different state directory.

**Solution**: Add a real-tmux integration test that verifies `tmux show-environment -t _portal-saver` output for daemon-relevant variables (`XDG_CONFIG_HOME`, `HOME`, `PATH`) is identical between (a) a pre-F-style baseline (session created with the daemon command directly, captured before the respawn would have fired) and (b) the post-F path (session created with placeholder, then respawned). The two environments must match exactly for those keys.

**Outcome**: A new integration test `TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn` lives in `internal/tmux/portal_saver_integration_test.go`. It computes a pre-F baseline (env captured from a freshly-created session before any respawn) and the post-F observed env (after the full create-placeholder → set-option → respawn flow), and asserts byte-for-byte equality on the `XDG_CONFIG_HOME`, `HOME`, `PATH` lines. Test fails if `NewDetachedSessionNoCwd` ever grows env overrides that diverge across respawn.

**Do**:
- Add `TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn` to `internal/tmux/portal_saver_integration_test.go`:
  1. `tmuxtest.SkipIfNoTmux(t)`; build + PATH-stage `portal` via `portalbintest`.
  2. `env, _ := portaltest.NewIsolatedStateEnv(t)`; the env is used both as the spawned tmux server's environment AND as the implicit input for verification.
  3. `sock := tmuxtest.New(t)`; tmux server inherits the isolated env.
  4. Compute the **pre-F baseline**: create a throwaway session via `client.NewDetachedSessionNoCwd("_env-baseline", "sh -c 'exec tail -f /dev/null'")` (a placeholder, used here purely to capture the session-env that tmux would have used for ANY initial command including `portal state daemon` pre-F). Read `tmux show-environment -t _env-baseline` via `Commander.RunRaw` and parse the lines for the three keys. Kill `_env-baseline` immediately after capture.
  5. Compute the **post-F observed**: call `tmux.BootstrapPortalSaver(client, stateDir)` which executes the full create-placeholder → set-option → respawn flow. After the readiness barrier returns, read `tmux show-environment -t _portal-saver`. Parse the same three keys.
  6. Assertion: the three key/value pairs in baseline equal the three key/value pairs in observed. Build an error message that pretty-prints both maps on diff.
  7. Independently assert that `NewDetachedSessionNoCwd` was called WITHOUT `-e KEY=VAL` overrides: inspect the argv path through a recorder or by reading `NewDetachedSessionNoCwd`'s source/argv shape (the function at `internal/tmux/tmux.go:372` currently constructs `new-session -d -s NAME [SHELLCMD]` — assert no `-e` flag is present in the argv). If a recorder isn't wired at integration level, capture this assertion as a unit test in `internal/tmux/portal_saver_test.go` complementing the integration test.
- Use the existing `tmux.Commander.RunRaw` for `show-environment` (it returns verbatim output suitable for line-parsing).
- Place under the existing integration test conventions; no `t.Parallel`.

**Acceptance Criteria**:
- [ ] `TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn` exists and skips when tmux is absent.
- [ ] Test captures `XDG_CONFIG_HOME`, `HOME`, `PATH` from a pre-F baseline session AND from `_portal-saver` after the full create-placeholder → respawn flow; asserts byte-equality of all three pairs.
- [ ] Test fails with a clear diff message if any of the three keys differ.
- [ ] A complementary unit test (or in-test argv inspection) asserts `NewDetachedSessionNoCwd` constructs `new-session -d -s NAME [SHELLCMD]` with NO `-e KEY=VAL` overrides — guarding against future env-override regressions.
- [ ] Uses `portaltest.NewIsolatedStateEnv` so the developer's real state dir is not touched.
- [ ] `go test ./internal/tmux/...` passes with tmux available.

**Tests**:
- `"TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn asserts XDG_CONFIG_HOME / HOME / PATH parity between pre-F baseline and post-F observed"` — the integration test itself.
- `"NewDetachedSessionNoCwd argv contains no -e KEY=VAL env override"` — unit-level argv inspection complementing the integration test.

**Edge Cases**:
- `tmux show-environment` output for variables that are unset is reported with a leading `-VARNAME` (no `=`) — when parsing, treat unset symmetrically (both baseline and observed must agree on unset-ness).
- `PATH` may include test-staged binary directories (from `portalbintest.StagePortalBinary`) — this is fine as long as both baseline and observed inherit the same tmux server's environment. The test depends on both sessions running under the same tmux server (which inherits the env once at startup).
- Baseline session must be killed BEFORE the saver bootstrap fires, so it doesn't interfere with the `_portal-saver` session-name lookup or with any global-env mutations.
- Equality compare is on a per-key map, not on raw `show-environment` byte-equality — `show-environment` may interleave additional vars set by tmux itself or by user dotfiles in the test environment; we only assert the three daemon-relevant keys.
- The `NewDetachedSessionNoCwd` source today (line 372–380) constructs args = `["new-session", "-d", "-s", name, shellCommand]` with no `-e` — the assertion future-proofs that.

**Context**:
> Spec § Component F, "Environment inheritance across respawn": "On all supported tmux versions, `respawn-pane` runs the new process with the session's environment (preserved from `new-session` time, plus any `set-environment` updates applied since). The current `createPortalSaverWithRetry` calls `NewDetachedSessionNoCwd` which does NOT pass `-e KEY=VAL` overrides — the saver session inherits the tmux server's environment as-is. Component F preserves this behaviour: no new env overrides are introduced at create-time, and the respawned daemon sees the same environment it would have seen as the initial pane command pre-F. Acceptance scenario: after Component F lands, `tmux show-environment -t _portal-saver` produces an output identical to the pre-F baseline for any environment variable the daemon reads (`XDG_CONFIG_HOME`, `HOME`, `PATH`)."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component F (Environment inheritance across respawn) and Acceptance criteria bullet on environment-inheritance parity.
