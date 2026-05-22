---
phase: 4
phase_name: Daemon Singleton Enforcement (Components A + B + C)
total: 10
---

## slow-open-empty-previews-and-zombie-sessions-4-1 | approved

### Task 4-1: Add SIGKILL escalation to killSaverAndWaitForDaemon with identity-check

**Problem**: When the recorded `daemon.pid` references a `portal state daemon` that is NOT the `_portal-saver` pane process (an orphan with a different parent tmux server), `tmux kill-session _portal-saver` cannot reach it. The current `killSaverAndWaitForDaemon` polls `killBarrierIsAlive(priorPID)` for the full 5 s window, times out, and proceeds — adding a guaranteed 5 s ceiling to every `portal open` invocation whenever such an orphan exists. There is no SIGTERM/SIGKILL escalation today.

**Solution**: Extend the existing kill-barrier flow with a post-poll escalation arm. If `priorPID` is still alive after the 5 s session-kill poll, identity-check the PID via `state.IdentifyDaemon` immediately before sending SIGKILL directly to the PID. SIGTERM is skipped on purpose — the orphan's view is divergent and we must NOT give its shutdown handler the chance to fire one more destructive `captureAndCommit` / `gcOrphanScrollback` cycle. After SIGKILL, poll `killBarrierIsAlive(priorPID)` at the existing 50 ms cadence for up to 1 s. On persistent aliveness log WARN and proceed (best-effort).

**Outcome**: A leaked orphan daemon whose PID the kill-barrier recorded is dead within ~6 s of `killSaverAndWaitForDaemon` entry (5 s session-kill poll + 1 s SIGKILL poll), with no final-flush GC cycle running on it. The 5 s timeout-and-proceed ceiling is eliminated for the orphan case.

**Do**:
- Edit `internal/tmux/portal_saver.go` — extend `killSaverAndWaitForDaemon` (lines 212-248).
- After the existing session-kill poll loop times out (replace the `WARN and return` at lines 238-244 with the escalation path), execute, in order:
  1. Call `state.IdentifyDaemon(priorPID)`. If the result is anything other than `state.IdentifyIsPortalDaemon` (including `IdentifyDead`, `IdentifyNotPortalDaemon`, OR `err != nil` per the spec's "Component A: treat transient error as skip SIGKILL" rule) — emit a single WARN (`"prior daemon (pid=%d) not identity-checked as portal state daemon; skipping SIGKILL"`) via `killBarrierLogger`, and return nil. No signal is sent.
  2. If `IdentifyIsPortalDaemon`: immediately call `syscall.Kill(priorPID, syscall.SIGKILL)` (or `unix.Kill`). No statement may sit between the `IdentifyDaemon` return and the `Kill` call other than the syscall itself, to minimise the recycle-between-check-and-kill window.
  3. Re-enter a bounded poll loop: 50 ms ticker (reuse `killBarrierPollInterval`), 1 s budget (introduce a new package-level `killBarrierEscalationTimeout = 1 * time.Second` var so tests can shrink it). On `!killBarrierIsAlive(priorPID)` return nil. On deadline reached, emit WARN (`"prior daemon (pid=%d) survived SIGKILL escalation within %v"`) and return nil.
- Add a package-level test seam for the SIGKILL syscall — e.g., `var killBarrierSendSIGKILL = func(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }` — so unit tests can record/inject errors without sending real signals.
- Add a package-level seam over `state.IdentifyDaemon` — e.g., `var killBarrierIdentifyDaemon = state.IdentifyDaemon` — for the same reason.
- Reset all new seams via `t.Cleanup` in tests; follow the existing `killBarrier*` seam conventions in `portal_saver.go`.
- Production legitimate-daemon path is NOT changed: SIGHUP from `tmux kill-session` still drives `defaultShutdownFlush` for the saver-pane daemon as today, because that daemon's PID is reachable via the session-kill and exits inside the 5 s poll window.

**Acceptance Criteria**:
- [ ] `killSaverAndWaitForDaemon` returns within ~6 s when `priorPID` is an orphan (5 s session-kill poll + 1 s SIGKILL poll).
- [ ] SIGKILL is sent only when `state.IdentifyDaemon(priorPID) == IdentifyIsPortalDaemon`; all other identity-check outcomes (Dead, NotPortalDaemon, transient error) skip the signal and return nil.
- [ ] Identity-check is the immediate previous statement to the SIGKILL syscall — no other work intervenes.
- [ ] Post-SIGKILL poll uses 50 ms cadence (`killBarrierPollInterval`) and 1 s total budget (`killBarrierEscalationTimeout`).
- [ ] Persistent aliveness after SIGKILL poll logs exactly one WARN via `killBarrierLogger` and returns nil — never escalates to a fatal abort.
- [ ] Legitimate saver-pane daemon shutdown path (SIGHUP via `tmux kill-session`) is bytes-identical to pre-task behaviour — verified by existing daemon-saver tests passing without modification.
- [ ] No SIGTERM is ever sent; the only direct signal sent by this helper is SIGKILL.

**Tests** (in `internal/tmux/portal_saver_test.go`):
- `"escalation: identity-checks-as-portal-daemon then SIGKILLs after session-kill poll times out"`
- `"escalation: identity-check returns IdentifyDead, no SIGKILL sent, returns nil quickly"`
- `"escalation: identity-check returns IdentifyNotPortalDaemon (PID recycled), no SIGKILL sent"`
- `"escalation: identity-check returns transient error, no SIGKILL sent (Component A skip-SIGKILL semantics)"`
- `"escalation: SIGKILL succeeds, post-kill poll observes death within 1 s budget"`
- `"escalation: SIGKILL succeeds but process survives full 1 s budget, WARN logged and helper returns nil"`
- `"escalation: identity-check is the immediate predecessor of the kill syscall (call-order recorded by stub)"`
- `"escalation: never sends SIGTERM (stub records all signals; SIGTERM count is 0 in every test)"`
- `"legitimate path: priorPID dies during session-kill poll, no escalation runs"`
- `"legitimate path: no daemon.pid file (read error), tolerant kill issued, no escalation poll"`

**Edge Cases**:
- PID recycled between session-kill poll and identity-check — covered by the identity-check refusal.
- PID recycled between identity-check and SIGKILL — accepted residual µs-scale window per spec.
- `state.IdentifyDaemon` returns a transient `err != nil` — Component A semantics: skip SIGKILL.
- SIGKILL syscall itself returns ESRCH (process already dead) — treat as success.
- Legitimate daemon SIGHUP path: pane process dies inside the existing 5 s poll, escalation arm never runs.

**Context**:
> Spec § Component A — Kill-Barrier Escalation. The SIGKILL-over-SIGTERM choice is structural: the daemon's signal handler at `cmd/state_daemon.go:340-345` runs `defaultShutdownFlush` → `captureAndCommit` → one final destructive GC cycle on shutdown. For an orphan being killed because its view of state is divergent, that final flush is the destructive operation we're escaping from. SIGKILL bypasses the handler entirely.
>
> Phase 1 has already shipped `state.IdentifyDaemon` with the three-result contract (`IdentifyIsPortalDaemon` / `IdentifyNotPortalDaemon` / `IdentifyDead`) plus the transient-error case. This task consumes it.
>
> The 50 ms cadence and 1 s budget match the existing `killBarrierPollInterval` and the spec's "bounded short window (1 s total) at 50 ms cadence".

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component A — Kill-Barrier Escalation.

## slow-open-empty-previews-and-zombie-sessions-4-2 | approved

### Task 4-2: Add no-final-flush snapshot test for escalation-killed orphans

**Problem**: Component A's correctness hinges on the orphan NOT running any final `captureAndCommit` / `gcOrphanScrollback` cycle on its way out — that final flush against a divergent view is exactly the destructive operation the SIGKILL path exists to prevent. A code-level review of "we used SIGKILL not SIGTERM" is insufficient; the spec mandates an empirical assertion that the scrollback directory is bytes-identical immediately before SIGKILL and 200 ms after the orphan exits.

**Solution**: Add an integration test that spawns a real orphan `portal state daemon` subprocess against an isolated state directory, drives it into a state where its next tick would write/delete scrollback files, snapshots the scrollback directory (`<stateDir>/scrollback/`) immediately before issuing SIGKILL, polls for process death, snapshots again 200 ms after death, and asserts the two snapshots are byte-identical (no new `.bin`, no removed `.bin`, no size/mtime/content delta on existing `.bin` files).

**Outcome**: A failing snapshot diff immediately surfaces any future regression that inadvertently re-routes the escalation through a path that lets the daemon run one more capture/commit/GC cycle.

**Do**:
- Place the test in `internal/tmux/portal_saver_escalation_integration_test.go` (or `cmd/bootstrap/escalation_no_flush_integration_test.go` if the orchestrator entry point is cleaner) under the existing integration build tag pattern used elsewhere in the repo.
- Use `portaltest.NewIsolatedStateEnv(t)` (Phase 1 deliverable) so the developer's real `~/.config/portal/state/` is untouched — the helper also installs the fingerprint-diff `t.Cleanup` backstop.
- Use `portalbintest.BuildPortalBinary` to build the binary under test.
- Spawn a `portal state daemon` subprocess with the isolated env, against a real tmux server fixture (`tmuxtest`) that has a `_portal-saver` session whose pane process is some OTHER process (the orphan must NOT be the pane process — the whole point of escalation).
- Wait until the orphan has produced at least one `.bin` file in `<stateDir>/scrollback/` (poll `os.ReadDir` until non-empty, bounded to ~3 s).
- Snapshot the scrollback directory: a `map[string]fileFingerprint` keyed by relative path, where each fingerprint captures (existence, size, mtime ns, ctime ns, SHA-256 of contents). Reuse the fingerprint struct introduced in Phase 1 (`portaltest`) — do NOT redefine it locally.
- Send SIGKILL to the orphan's PID directly (mimicking the escalation path; do NOT go through the kill-barrier helper — the test isolates the no-flush guarantee from the helper's plumbing).
- Poll `os.FindProcess` + `syscall.Kill(pid, 0)` until the process is gone, then sleep exactly 200 ms.
- Snapshot the scrollback directory again. Assert deep equality with the first snapshot using `reflect.DeepEqual` (or per-field comparison emitting a precise delta on failure: which path changed, which field).
- Test must FAIL clearly when introduced against the current pre-Task-4-1 code with the escalation pathing accidentally routed through SIGTERM — the SIGTERM path would invoke `defaultShutdownFlush` and produce a snapshot delta.

**Acceptance Criteria**:
- [ ] Test compiles under the existing integration build tag and runs only when that tag is enabled.
- [ ] Test uses `portaltest.NewIsolatedStateEnv` — no writes to the developer's real state dir.
- [ ] Pre-SIGKILL snapshot is non-empty (at least one `.bin`); the test does NOT silently pass against an empty scrollback dir.
- [ ] Post-SIGKILL snapshot is taken exactly 200 ms after the orphan's death is observed.
- [ ] Snapshot equality is asserted across all five fingerprint fields (existence, size, mtime ns, ctime ns, SHA-256); any delta fails the test with a path-and-field diagnostic.
- [ ] Test passes against Task 4-1's SIGKILL-based escalation and FAILS against a hypothetical SIGTERM-with-marker variant.

**Tests**:
- `"escalation-killed orphan produces no scrollback delta in the 200 ms post-death window"`
- `"snapshot diff fails on a deliberately-modified `.bin` (meta-test guarding the diff implementation)"`

**Edge Cases**:
- Orphan exits before the snapshot is taken (PID gone immediately) — test fails with a clear "snapshot was never taken" message rather than silent green.
- `_portal-saver` session disappears between fixture setup and snapshot — the test's tmux fixture must keep the session pinned (`destroy-unattached=off` set on the fixture or use the placeholder-pane approach already in `tmuxtest`).
- Filesystem clock granularity is coarser than 1 ms on some platforms — `mtime` comparison must be at the ns granularity Go reports (`os.FileInfo.ModTime().UnixNano()`); a no-write window will show identical ns timestamps.
- Symlinks in the scrollback dir — the fingerprint uses lstat semantics (per the Phase 1 helper spec).

**Context**:
> Spec § Component A acceptance criterion: "No final-flush GC cycle runs on orphans being escalation-killed. Verified by snapshotting the scrollback directory immediately before SIGKILL and again 200 ms after the orphan exits; the two snapshots must be identical (no new `.bin` files, no deleted `.bin` files, no mtime/size changes on existing `.bin` files). The observation harness uses fsnotify or a polled `os.ReadDir` snapshot; either is acceptable."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component A — Kill-Barrier Escalation acceptance criteria.

## slow-open-empty-previews-and-zombie-sessions-4-3 | approved

### Task 4-3: Implement SweepOrphanDaemons core (pgrep + legitimate-set + identity + kill)

**Problem**: Component A only handles the single PID recorded in the kill-barrier file (`daemon.pid`). When multiple orphan daemons exist concurrently (as on the reporter's install — three concurrent daemons each holding flock on a different inode), A cannot reach the orphans beyond the recorded one. There is currently no bootstrap-time enumeration that handles the *full* set of `portal state daemon` processes.

**Solution**: Implement the core `SweepOrphanDaemons` logic as a standalone function that (1) enumerates candidate PIDs via `pgrep -fx '^portal state daemon( |$)'`, (2) builds the legitimate set from the `_portal-saver` pane's PID (empty when `_portal-saver` is absent), (3) for each non-legitimate PID identity-checks it via `state.IdentifyDaemon` and SIGKILLs survivors, (4) logs INFO per killed PID and WARN-and-swallow on every error path. Best-effort by construction — never aborts bootstrap.

**Outcome**: A reusable, dependency-injected sweeper that, given three concurrent daemons (one legitimate + two orphans), kills the two orphans and leaves the legitimate one untouched; given a clean steady state, sends zero signals and emits no INFO entries.

**Do**:
- Create a new file `cmd/bootstrap/orphan_sweep.go` defining a `OrphanSweeper` interface and a concrete `*OrphanSweepCore` implementation, mirroring the existing pattern used by `MarkerCleaner` / `FIFOSweeper` (see `cmd/bootstrap/stale_marker_cleanup.go` for the precedent).
- The interface signature: `type OrphanSweeper interface { SweepOrphanDaemons() error }`.
- `*OrphanSweepCore` accepts injected seams:
  - `Pgrep func() ([]int, error)` — enumerates candidate PIDs via `pgrep -fx '^portal state daemon( |$)'`. Production wiring (Task 4-4) provides the `os/exec` implementation in `internal/bootstrapadapter`.
  - `SaverPanePID func() (int, error)` — returns the pane PID of `_portal-saver`. On `_portal-saver` absent OR the tmux call failing, returns `0, nil` (NOT an error) — the legitimate set is empty and the sweep proceeds. Production wiring uses `tmux.Client.ListPanesInSession` or a new `tmux.Client.SaverPanePID` helper.
  - `Identify func(pid int) (state.IdentifyResult, error)` — defaults to `state.IdentifyDaemon`.
  - `Kill func(pid int) error` — defaults to `syscall.Kill(pid, syscall.SIGKILL)`.
  - `Logger bootstrap.Logger` — emits INFO/WARN.
- Algorithm (`SweepOrphanDaemons` method body):
  1. Call `Pgrep()`. On error: log WARN (`"sweep: pgrep failed: %v"`) and return nil (best-effort).
  2. Call `SaverPanePID()`. On error: log WARN (`"sweep: list-panes _portal-saver failed, legitimate set empty: %v"`) and treat the legitimate set as empty. Build `legitimate := map[int]struct{}{}` (or a single-int variable since the legitimate set has at most one member).
  3. For each candidate PID NOT in the legitimate set AND not equal to `os.Getpid()` (defensive — should never appear in pgrep output, but cheap to assert):
     1. Call `Identify(pid)`. If err != nil: log WARN (`"sweep: identity-check pid=%d failed, skipping: %v"`) and continue. If the result is anything other than `IdentifyIsPortalDaemon`: log DEBUG (`"sweep: pid=%d not identity-checked as portal daemon, skipping"`) and continue.
     2. Call `Kill(pid)`. If err != nil: log WARN (`"sweep: kill pid=%d failed: %v"`) and continue. On success: log INFO (`"sweep: killed orphan daemon pid=%d"`).
  4. Return nil unconditionally — step is best-effort.
- The pgrep enumeration is the canonical form `pgrep -fx '^portal state daemon( |$)'`; document this in the production adapter's docstring (Task 4-4). Do NOT use a ps-based form — the spec rejects it as non-equivalent.
- Add unit tests in `cmd/bootstrap/orphan_sweep_test.go` driving every branch via the seams (no real subprocesses, no real tmux).

**Acceptance Criteria**:
- [ ] `OrphanSweeper` interface and `*OrphanSweepCore` concrete type exist in `cmd/bootstrap/orphan_sweep.go`.
- [ ] All four functional seams (`Pgrep`, `SaverPanePID`, `Identify`, `Kill`) are injectable; production defaults set in the adapter (Task 4-4), test code overrides via struct fields.
- [ ] On `Pgrep` error: WARN logged, returns nil.
- [ ] On `SaverPanePID` error or `_portal-saver` absent: legitimate set is empty, sweep proceeds against ALL pgrep results.
- [ ] For each non-legitimate PID with `IdentifyIsPortalDaemon`: SIGKILL is sent, INFO entry `"sweep: killed orphan daemon pid=%d"` emitted.
- [ ] For each non-legitimate PID with any other identity result: NO signal is sent.
- [ ] All errors swallowed; method never returns a non-nil error.
- [ ] `os.Getpid()` is never SIGKILLed (defensive self-skip).

**Tests** (in `cmd/bootstrap/orphan_sweep_test.go`):
- `"sweep: kills the two orphans and leaves the legitimate saver-pane PID alone (3-daemon scenario)"`
- `"sweep: _portal-saver absent → legitimate set empty → kills all pgrep results that identity-check"`
- `"sweep: pgrep error logs WARN and returns nil (best-effort)"`
- `"sweep: list-panes error logs WARN and treats legitimate set as empty"`
- `"sweep: IdentifyDead skipped, no signal sent"`
- `"sweep: IdentifyNotPortalDaemon (recycled PID) skipped, no signal sent"`
- `"sweep: Identify transient error skipped, no signal sent (Component B semantics)"`
- `"sweep: Kill error logs WARN, continues to next PID"`
- `"sweep: clean state (pgrep returns only the legitimate saver-pane PID) emits zero INFO entries"`
- `"sweep: never SIGTERMs (signal recorder shows only SIGKILL across all branches)"`
- `"sweep: defensive os.Getpid() skip — own PID never in kill set even if it appeared in pgrep"`

**Edge Cases**:
- `pgrep` returns an empty list (no daemons running) — sweep is a no-op, returns nil.
- `pgrep` returns the legitimate PID only — sweep is a no-op, returns nil, zero INFO entries.
- Legitimate set is empty (no `_portal-saver`) and pgrep returns one PID that identity-checks: that PID IS killed (correct under spec — no `_portal-saver` means no legitimate daemon can exist).
- A new daemon spawns between `Pgrep()` and `Kill()` — out of scope per spec's "Concurrency note" (B is non-atomic by design; next bootstrap will sweep).
- `pgrep -fx` exits with status 1 when zero matches: treat as empty list, NOT an error.

**Context**:
> Spec § Component B — Bootstrap-Time Orphan Sweep. The canonical enumeration form `pgrep -fxc 'portal state daemon'` (acceptance criterion) maps to the implementation form `pgrep -fx '^portal state daemon( |$)'` — both are the same canonical pgrep behaviour. The ps-based alternative is rejected.
>
> Phase 1 has already shipped `state.IdentifyDaemon`. This task consumes it.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component B — Bootstrap-Time Orphan Sweep.

## slow-open-empty-previews-and-zombie-sessions-4-4 | approved

### Task 4-4: Wire SweepOrphanDaemons as orchestrator step 4 between Set @portal-restoring and EnsureSaver

**Problem**: The `*OrphanSweepCore` from Task 4-3 is dead code until it is wired into the bootstrap orchestrator. The spec requires it to run as a new step inserted between current step 3 (Set `@portal-restoring`) and current step 4 (EnsureSaver), shifting all later steps by one — the orchestrator becomes 11 steps. Production adapters must supply the `Pgrep` / `SaverPanePID` / `Identify` / `Kill` seams.

**Solution**: Add an `OrphanSweeper` field to the `Orchestrator` struct in `cmd/bootstrap/bootstrap.go`. Invoke it in `Run` between step 3 and step 4 (renumbering 4-10 → 5-11 in the package docstring and inline comments). Wire production seams in `internal/bootstrapadapter/adapters.go`. Update `CLAUDE.md`'s bootstrap section to reflect the new 11-step ordering.

**Outcome**: `portal open` invocations at runtime run the orphan sweep *before* `EnsureSaver`, so the new saver-pane daemon's first tick is uncontested — orphans have stopped writing to the state directory before any new write begins.

**Do**:
- Edit `cmd/bootstrap/bootstrap.go`:
  - Add `OrphanSweeper OrphanSweeper` field to the `Orchestrator` struct.
  - Update the package docstring (lines 1-30) to list the new 11-step ordering.
  - In `Run`, insert the new step after step 3 (`Restoring.Set()`) and before the existing step 4 (`Saver.EnsureSaver()`):
    ```go
    // Step 4 — SweepOrphanDaemons (best-effort).
    o.Logger.Debug(state.ComponentBootstrap, "step 4 (SweepOrphanDaemons): entering")
    if err := o.OrphanSweeper.SweepOrphanDaemons(); err != nil {
        o.Logger.Warn(state.ComponentBootstrap, "step 4 (SweepOrphanDaemons) failed: %v", err)
        // Continue per spec — best-effort.
    }
    ```
  - Renumber EnsureSaver to step 5, Restore to step 6, EagerSignalHydrate to step 7, Clear `@portal-restoring` to step 8, CleanStaleMarkers to step 9, SweepOrphanFIFOs to step 10, CleanStale to step 11. Update both the inline debug log strings AND the docstring.
- Update the existing inline comments that reference the old step numbers (search `internal/` and `cmd/` for `"step N "` references in package docs / comments) — only the comment text in this file changes; downstream callers don't reference step numbers programmatically.
- Edit `internal/bootstrapadapter/adapters.go` (or add a new `orphan_sweep.go` alongside it):
  - Add a `NewOrphanSweeper(client *tmux.Client, logger *state.Logger) bootstrap.OrphanSweeper` constructor.
  - Production `Pgrep`: `exec.Command("pgrep", "-fx", "^portal state daemon( |$)")` → parse newline-separated PIDs from stdout. Exit status 1 with empty stdout means "no matches" (NOT an error). Any other non-zero exit is an error.
  - Production `SaverPanePID`: invoke `client.ListPanesInSession(tmux.PortalSaverName)` and return the first pane's PID, or a small helper `client.SaverPanePID()` if added. On `_portal-saver` absent (e.g., `client.HasSession` returns false), return `(0, nil)` — the empty-legitimate-set semantics from Task 4-3.
  - Production `Identify`: default to `state.IdentifyDaemon` (no wrapping needed).
  - Production `Kill`: default to `syscall.Kill(pid, syscall.SIGKILL)`.
  - Logger: forward the existing `*state.Logger` through `bootstrap.Logger`.
- Edit `cmd/root.go` (or wherever the production `Orchestrator` is constructed — likely `cmd/bootstrap_production.go`) to populate the new `OrphanSweeper` field via the adapter constructor.
- Update `CLAUDE.md`'s "Server bootstrap" section: change the heading from "ten-step `bootstrap.Orchestrator`" to "eleven-step `bootstrap.Orchestrator`", insert the new step 4 entry, and shift the subsequent numbered list entries by one. Keep all existing inter-step invariant prose intact.
- Use existing `bootstrap.Warning` / `WriteLines` only if a user-facing warning is desired; the spec says step is best-effort and "any error is logged WARN and swallowed", so no warning is added to the orchestrator's `warnings` slice — only the Logger.Warn line.

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

**Tests** (in `cmd/bootstrap/bootstrap_test.go`):
- `"orchestrator: step 4 invokes OrphanSweeper.SweepOrphanDaemons exactly once"`
- `"orchestrator: step 4 runs after Restoring.Set and before Saver.EnsureSaver (call-order recorded by spy)"`
- `"orchestrator: step 4 returning non-nil error logs WARN and does not abort (Run returns nil err)"`
- `"orchestrator: step 4 returning nil emits no WARN entries"`
- `"orchestrator: nil OrphanSweeper field panics on Run (consistent with existing nil-interface treatment of other steps)"` — or, alternative, gate behind a noop default; match the convention used for other required steps.
- `"orchestrator: existing step ordering 1-3 and 5-11 preserved after insertion"`

**Edge Cases**:
- `OrphanSweeper` field is nil at orchestrator construction time — match the convention used for the other mandatory step fields (existing code dereferences without nil-guard; production wiring is responsible for populating). If the convention is to noop-default, follow it. Document the choice in the field's doc comment.
- Adapter's `pgrep` exits with status 1 and empty stdout — return empty slice, no error.
- Adapter's `SaverPanePID` invoked when `HasSession(_portal-saver)` is false — return `(0, nil)`.
- A future re-numbering risk: the inline log strings hard-code "step 4". Acceptable per project convention.

**Context**:
> Spec § Component B inter-step invariants: "`SweepOrphanDaemons` runs *before* `EnsureSaver` and does not interact with `@portal-restoring`, `client-attached` hooks, or any post-Restore state." The post-insertion 11-step list is exactly:
> 1. EnsureServer
> 2. RegisterPortalHooks
> 3. Set `@portal-restoring`
> 4. SweepOrphanDaemons (new)
> 5. EnsureSaver
> 6. Restore
> 7. EagerSignalHydrate
> 8. Clear `@portal-restoring`
> 9. CleanStaleMarkers
> 10. SweepOrphanFIFOs
> 11. CleanStale
>
> Existing inter-step invariants preserved: "EagerSignalHydrate runs while @portal-restoring is still set", "Clear must precede CleanStaleMarkers".

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component B — Bootstrap-Time Orphan Sweep.

## slow-open-empty-previews-and-zombie-sessions-4-5 | approved

### Task 4-5: Integration test for SweepOrphanDaemons (3 daemons converge to 1, clean state sends zero signals)

**Problem**: Task 4-3's unit tests use injected seams and prove the logic, but the spec also requires an end-to-end integration test that exercises the real `pgrep` + real `state.IdentifyDaemon` + real SIGKILL pipeline against real subprocesses, against an isolated state directory. Without this, a future regression in the production adapter wiring (Task 4-4) could ship without unit-test coverage exposing it.

**Solution**: Add an integration test that spawns three real `portal state daemon` subprocesses against an isolated state directory and a real tmux server fixture, then invokes the production `SweepOrphanDaemons` step and asserts `pgrep -fxc 'portal state daemon'` converges to 1. A second case exercises clean state (only the legitimate saver-pane daemon) and asserts the sweep emits zero `"sweep: killed orphan daemon"` log entries.

**Outcome**: End-to-end coverage of the orphan-sweep behaviour against the real pgrep/identity/kill pipeline — regressions in either the adapter wiring or the underlying primitives surface as a failing integration test.

**Do**:
- Place in `cmd/bootstrap/orphan_sweep_integration_test.go` under the existing integration build tag.
- Use `portaltest.NewIsolatedStateEnv(t)` for state-dir isolation.
- Use `portalbintest.BuildPortalBinary` to build the binary under test.
- Use `tmuxtest` to start a real tmux server with `_portal-saver` plus one user session.
- Scenario A — "3 daemons converge to 1":
  - Start the legitimate saver-pane daemon by creating `_portal-saver` with `portal state daemon` as its pane command (or via `BootstrapPortalSaver` post-Phase-3).
  - Spawn 2 orphan daemons as direct subprocesses (not via tmux), each with the isolated env, against the same `stateDir`. Their parent process is the test, NOT the saver pane — they ARE orphans by construction.
  - Confirm `pgrep -fxc 'portal state daemon'` returns 3 in the pre-state.
  - Construct the production `OrphanSweepCore` via `internal/bootstrapadapter.NewOrphanSweeper` against the real tmux client and state dir.
  - Invoke `SweepOrphanDaemons()`.
  - Poll `pgrep -fxc 'portal state daemon'` until it returns 1, bounded to 3 s. Fail with the current count if it doesn't converge.
  - Assert the legitimate saver-pane daemon's PID is the survivor by comparing against `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
- Scenario B — "clean state, zero signals":
  - Start only the legitimate saver-pane daemon.
  - Capture the test's logger output (use a recording `bootstrap.Logger` rather than the default `*state.Logger` so log entries can be asserted on).
  - Invoke `SweepOrphanDaemons()`.
  - Assert ZERO log entries match `"sweep: killed orphan daemon"`.
  - Assert `pgrep -fxc 'portal state daemon'` still returns 1.
- Scenario C — "recycled PID refusal":
  - Spawn a non-daemon process (e.g., `sleep 30`) and arrange via a custom `Pgrep` seam to pretend its PID is in the candidate set. Real `state.IdentifyDaemon` against that PID returns `IdentifyNotPortalDaemon`. Assert no SIGKILL is sent (e.g., assert the sleep is still alive 200 ms post-sweep).

**Acceptance Criteria**:
- [ ] Test runs only under the integration build tag.
- [ ] Test uses `portaltest.NewIsolatedStateEnv` — no writes to developer state dir; fingerprint-diff cleanup verifies on exit.
- [ ] Test uses real `portal state daemon` subprocesses (built via `portalbintest.BuildPortalBinary`), real tmux server (via `tmuxtest`), real production `OrphanSweepCore` via `NewOrphanSweeper`.
- [ ] Scenario A converges to `pgrep -fxc == 1` within 3 s of `SweepOrphanDaemons()` returning.
- [ ] Scenario A: the survivor PID equals the `_portal-saver` pane PID.
- [ ] Scenario B: zero `"sweep: killed orphan daemon"` entries in the recording logger.
- [ ] Scenario C: a non-daemon PID is never SIGKILLed (identity-check refusal exercised end-to-end).
- [ ] All spawned subprocesses are cleaned up via `t.Cleanup` regardless of test outcome.

**Tests**:
- `"integration: 3 daemons (1 saver-pane + 2 orphans) converge to pgrep -fxc == 1 after SweepOrphanDaemons"`
- `"integration: clean steady state (only saver-pane daemon) produces zero killed-orphan log entries"`
- `"integration: recycled-PID refusal — sleep process survives a sweep that names its PID"`
- `"integration: cleanup ensures no leaked daemons after test exit"`

**Edge Cases**:
- Orphan subprocesses exit before the sweep runs (e.g., they segfault) — test fails with a clear diagnostic; do NOT silently skip.
- `pgrep` is not on PATH (CI image without procps/pgrep) — test skips via `t.Skip` with a clear reason; document this in the test's preamble. (On macOS/Linux dev machines pgrep is universally available.)
- `tmuxtest` fixture's `_portal-saver` is created but the pane process dies between setup and sweep — re-probe and retry up to once before failing.
- Race between `Pgrep` and orphan spawn — the test waits for `pgrep -fxc` to report 3 before invoking the sweep, so the race is closed.

**Context**:
> Spec § Component B acceptance criterion: "Given N concurrent `portal state daemon` processes where N-1 are orphans (parent ≠ saver pane process; or no saver session exists), bootstrap step `SweepOrphanDaemons` kills N-1 of them. Verified by `pgrep -fxc 'portal state daemon'` returning 1 (the legitimate saver-pane daemon) after the step completes. ... Given only the legitimate saver-pane daemon, the sweep sends zero signals. Verified by audit log: no `\"sweep: killed orphan daemon\"` entries on a clean-state bootstrap."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component B — Bootstrap-Time Orphan Sweep acceptance criteria.

## slow-open-empty-previews-and-zombie-sessions-4-6 | approved

### Task 4-6: Add pre-acquire daemon.pid liveness check to AcquireDaemonLock

**Problem**: `flock` excludes per-inode, not per-path. If `daemon.lock` is unlinked + recreated between two daemon spawns (older code path, manual `rm`, leaked test scaffolding), two daemons end up `flock`ing different inodes — both succeed and the singleton invariant is silently broken. Components A and B reduce the probability of this happening, but they don't close the structural mechanism. The spec's primary singleton enforcer for steady-state contention is a pre-acquire `daemon.pid` liveness check that uses a stable file whose content we control, sidestepping `flock`'s per-inode limitation.

**Solution**: Augment `state.AcquireDaemonLock` with a pre-acquire check that reads `daemon.pid` via `state.ReadPIDFile`, identity-checks the recorded PID via `state.IdentifyDaemon`, and returns `ErrDaemonLockHeld` if the recorded PID is alive AND identity-checks as a `portal state daemon`. Absent/stale/wrong-identity PIDs fall through to the existing open + flock path.

**Outcome**: A second daemon attempting to acquire the lock while a live identity-checkable `portal state daemon` is recorded in `daemon.pid` is refused at the pre-check — *regardless* of whatever inode `daemon.lock` currently resolves to. The existing EWOULDBLOCK fallback remains in place for the small startup window before `daemon.pid` is written.

**Do**:
- Edit `internal/state/daemon_lock.go` — augment `AcquireDaemonLock` (lines 55-77).
- Insert the pre-check BEFORE the existing `os.OpenFile(path, ...)` call:
  1. Read `daemon.pid` via `state.ReadPIDFile(stateDir)`. On `os.IsNotExist` OR any other read error: treat as "no holder", skip the pre-check and proceed to the existing open + flock path. (Spec: "If absent: skip; proceed.")
  2. If `ReadPIDFile` returns a valid PID: call `state.IdentifyDaemon(pid)`. Branch on the result:
     - `IdentifyIsPortalDaemon` (err == nil): return `ErrDaemonLockHeld` IMMEDIATELY — do not open `daemon.lock`.
     - `IdentifyDead`, `IdentifyNotPortalDaemon`: proceed to the existing open + flock path. The pre-check found no live holder.
     - `err != nil` (transient identity-check failure): per Component C spec rationale — "treat transient error as 'not a portal daemon' — proceed with acquire. Rationale: the flock EWOULDBLOCK fallback still catches real contention; biasing toward 'let legitimate succession proceed' is safer than spuriously blocking startup." Proceed.
- Add a package-level seam over `state.IdentifyDaemon` — e.g., `var lockAcquireIdentifyDaemon = IdentifyDaemon` — for testability, following the existing `lockAcquire` seam precedent on line 25.
- Add a package-level seam over `state.ReadPIDFile` — e.g., `var lockAcquireReadPIDFile = ReadPIDFile` — for testability.
- Document the new pre-check in the `AcquireDaemonLock` docstring, explaining:
  - When it returns `ErrDaemonLockHeld` via the pre-check vs. via the existing EWOULDBLOCK path.
  - The "transient identity error → proceed" rationale.
  - The layered enforcement note: pre-check is primary for steady-state contention, flock EWOULDBLOCK is the fallback for the small startup window.

**Acceptance Criteria**:
- [ ] Pre-check runs before `os.OpenFile(path, O_RDWR|O_CREATE, ...)` — verified by reading the function body / via the AST test in Task 4-8 if relevant.
- [ ] `daemon.pid` absent → pre-check skipped, proceeds to existing open + flock.
- [ ] `daemon.pid` present, recorded PID dead (`IdentifyDead`) → pre-check proceeds.
- [ ] `daemon.pid` present, recorded PID alive but `IdentifyNotPortalDaemon` (recycled) → pre-check proceeds.
- [ ] `daemon.pid` present, recorded PID alive AND `IdentifyIsPortalDaemon` → returns `ErrDaemonLockHeld` WITHOUT opening `daemon.lock` (verified by stubbing `lockAcquire` to assert it was never called).
- [ ] `daemon.pid` present, identity-check returns transient error → pre-check proceeds (consistent with "let legitimate succession proceed" rationale).
- [ ] `ReadPIDFile` read error (non-IsNotExist; e.g., permission denied) → treated as "no holder", pre-check proceeds.
- [ ] Existing EWOULDBLOCK / open(2) / FD_CLOEXEC paths are bytes-identical to pre-task behaviour.

**Tests** (in `internal/state/daemon_lock_test.go`):
- `"pre-check: daemon.pid absent → proceeds, opens daemon.lock, acquires"`
- `"pre-check: daemon.pid records dead PID → proceeds, acquires"`
- `"pre-check: daemon.pid records live PID that identity-checks as portal state daemon → returns ErrDaemonLockHeld without opening daemon.lock"`
- `"pre-check: daemon.pid records live PID that identity-checks as IdentifyNotPortalDaemon (recycled) → proceeds, acquires"`
- `"pre-check: daemon.pid records live PID, identity-check returns transient err → proceeds, acquires (Component C semantics)"`
- `"pre-check: ReadPIDFile returns non-IsNotExist read error → proceeds (treated as no holder)"`
- `"pre-check: does NOT open daemon.lock when returning ErrDaemonLockHeld — verified by stubbed os.OpenFile / lockAcquire counter"`
- `"existing EWOULDBLOCK path: pre-check finds no holder, flock fails with EWOULDBLOCK → returns ErrDaemonLockHeld (regression coverage)"`

**Edge Cases**:
- `daemon.pid` exists but is empty or malformed — `ReadPIDFile`'s existing parse-error behaviour propagates; treat as "no holder" and proceed.
- `daemon.pid` records the current process's own PID (rare degenerate case where caller already wrote pidfile out-of-band) — `IdentifyDaemon` runs against `os.Getpid()`, likely returns `IdentifyIsPortalDaemon` if this daemon is actually running as a `portal state daemon` argv → `ErrDaemonLockHeld`. This is acceptable degenerate behaviour; production code never writes the pidfile before acquiring the lock (Task 4-8 enforces ordering).
- The recorded PID has been recycled to a process whose argv coincidentally contains "portal state daemon" — extreme corner case; acceptable false-positive per spec ("worst-case false-positive consequence is degraded but not destructive").

**Context**:
> Spec § Component C step 1 — "Pre-acquire daemon.pid liveness check". Phase 1 ships `state.IdentifyDaemon` with the three-result contract.
>
> The spec's deviation note: "The investigation's described shape was 'Open with `O_EXCL|O_CREAT`, then `fstat` the fd and `stat` the path, and refuse if inodes differ'. The spec deviates by retaining `O_RDWR|O_CREAT` and introducing the pre-acquire `daemon.pid` liveness check as the primary singleton enforcer instead. ... The `daemon.pid` check achieves the same correctness guarantee without changing the lockfile lifecycle — what matters for singleton enforcement is whether a live identity-checkable daemon is recorded, not which inode the lockfile currently resolves to."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component C — Stabilise the `daemon.lock` Singleton Against Inode Replacement, step 1 ("Pre-acquire daemon.pid liveness check").

## slow-open-empty-previews-and-zombie-sessions-4-7 | approved

### Task 4-7: Add post-flock fstat-vs-stat inode cross-check with bounded retry

**Problem**: Even after Task 4-6's pre-check, a small race window remains: between our `os.OpenFile(daemon.lock)` and our `unix.Flock`, a third party could unlink + recreate `daemon.lock`. Our fd then references one inode while the path resolves to another — and we hold flock on the wrong inode. This is the secondary defence the spec calls for: an fstat-vs-stat inode comparison post-flock, with bounded retry on mismatch.

**Solution**: After `unix.Flock` succeeds, `fstat` the fd and `stat` the path; compare the inode numbers. On mismatch: close the fd (releasing the flock), sleep 10 ms, and retry the whole acquire (pre-check + open + flock + cross-check) up to 3 total attempts. On persistent mismatch after the bound: return a wrapped error (not `ErrDaemonLockHeld` — this is the daemon-internal status-1-exit path).

**Outcome**: A daemon that successfully `flock`s an inode that no longer matches the path either (a) succeeds on a subsequent retry after the inode-replacing daemon stabilises, or (b) exits status 1 with a WARN under `ComponentDaemon` after the bounded retry — refusing to operate on a structurally unsound lock.

**Do**:
- Edit `internal/state/daemon_lock.go` — augment `AcquireDaemonLock`.
- Restructure the function into an outer loop bounded by 3 attempts:
  1. Run the pre-check (Task 4-6) — on `ErrDaemonLockHeld` return immediately, no retry.
  2. Run the existing `os.OpenFile(path, O_RDWR|O_CREATE, 0o600)`.
  3. Run the existing `lockAcquire(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)` — on EWOULDBLOCK return `ErrDaemonLockHeld` (existing behaviour, no retry).
  4. **New: inode cross-check.**
     - `fstat` the fd: `var fdStat unix.Stat_t; unix.Fstat(int(f.Fd()), &fdStat)` → `fdInode := fdStat.Ino`.
     - `stat` the path: `var pathStat unix.Stat_t; unix.Stat(path, &pathStat)` → `pathInode := pathStat.Ino`.
     - On `fstat` OR `stat` syscall error: close fd, return wrapped error immediately (do NOT retry — syscall errors at this level are structural).
     - If `fdInode == pathInode`: lock acquired, run the existing FD_CLOEXEC step, return `(f, nil)`.
     - If `fdInode != pathInode`: close fd (releasing flock), sleep 10 ms, increment attempt counter, continue outer loop.
  5. Existing `unix.FcntlInt(..., FD_CLOEXEC)` runs only after the inode cross-check passes.
- On exit from the outer loop with attempts exhausted (3 mismatches in a row): return a wrapped error with a clear message — e.g., `fmt.Errorf("daemon.lock inode mismatch after 3 attempts: fd inode != path inode")`. This is NOT `ErrDaemonLockHeld`; callers must NOT match it via `errors.Is(err, ErrDaemonLockHeld)`. The daemon's `defaultDaemonRun` already treats wrapped errors as ERROR-and-exit-non-zero (see `cmd/state_daemon.go:296-298`); per the spec, this is exit status 1 with a WARN log line.
- Add package-level seams for `Fstat` and `Stat` for testability — e.g., `var lockAcquireFstat = unix.Fstat` and `var lockAcquireStat = unix.Stat`. Document seam reset via `t.Cleanup` in tests.
- Bound the retry loop so total worst-case delay is `< 100 ms` (3 attempts × 10 ms sleep + 3 negligible syscall costs ≈ 30 ms). Use `time.Sleep(10 * time.Millisecond)` between attempts; do NOT introduce jitter (the spec specifies a fixed 10 ms).

**Acceptance Criteria**:
- [ ] `fstat(fd).Ino == stat(path).Ino` is verified after every successful flock.
- [ ] On mismatch on attempt 1: close fd, sleep 10 ms, retry. On match on attempt 2: succeed.
- [ ] On mismatch on attempts 1, 2, 3: return a wrapped error after attempt 3. Total measured wall-time delay < 100 ms.
- [ ] On `unix.Fstat` or `unix.Stat` syscall error: close fd, return wrapped error immediately (no retry).
- [ ] Match on attempt 1 (happy path): no sleep, no retry, no observable delay.
- [ ] EWOULDBLOCK path: returns `ErrDaemonLockHeld` from inside the first attempt's flock call — no retry, no inode check.
- [ ] Pre-check `ErrDaemonLockHeld` path (Task 4-6): returns immediately with no retry.
- [ ] FD_CLOEXEC step only runs after the inode cross-check passes.
- [ ] Wrapped error returned on persistent mismatch is NOT matched by `errors.Is(err, ErrDaemonLockHeld)` — callers can distinguish.

**Tests** (in `internal/state/daemon_lock_test.go`):
- `"inode-check: happy path — fd inode == path inode on first attempt, succeeds"`
- `"inode-check: mismatch on attempt 1, match on attempt 2, succeeds (retry-on-mismatch)"`
- `"inode-check: mismatch on attempts 1, 2, 3 → returns wrapped non-ErrDaemonLockHeld error within 100 ms total"`
- `"inode-check: Fstat returns syscall error → returns wrapped error immediately, no retry"`
- `"inode-check: Stat returns syscall error → returns wrapped error immediately, no retry"`
- `"inode-check: mismatch closes the fd before retry — verified by recording close-call count"`
- `"inode-check: FD_CLOEXEC is not called when inode mismatch is persistent"`
- `"inode-check: bounded retry total wall-time < 100 ms (measured)"`
- `"regression: EWOULDBLOCK path still returns ErrDaemonLockHeld via existing flock failure path, inode check is not reached"`
- `"regression: pre-check ErrDaemonLockHeld (Task 4-6) skips the inode check entirely"`

**Edge Cases**:
- File replaced between attempts by a fast competitor — retry observes the new inode; if it stabilises, attempt 2 or 3 succeeds.
- File deleted by competitor between `flock` and `fstat` — `fstat` on the fd still returns the original inode (the fd holds a reference even after unlink). `stat` on the path returns ENOENT. The ENOENT is a syscall error → immediate wrapped-error return per the spec's "fstat/stat syscall error" handling.
- Two daemons in lockstep racing each other — both retry up to 3 times; at most one wins on any given attempt, and the loser falls into pre-check or EWOULDBLOCK on the next attempt.

**Context**:
> Spec § Component C step 3 — "Post-flock inode cross-check". "Bounded to 3 retries with a 10 ms sleep between attempts. On persistent mismatch after the bound, return a wrapped error." Exit semantics: "the daemon's `runDaemonE` (or equivalent) treats this wrapped error like any other open(2)/flock failure today — log WARN under `ComponentDaemon` and exit with status 1. ... distinct from the `ErrDaemonLockHeld` path which exits status 0. The lock-loser status 0 path is retained for the pre-check `ErrDaemonLockHeld` case."
>
> "Because the daemon's pane is configured with `destroy-unattached=off` (Component F), a status 1 exit does NOT trigger a restart loop — `_portal-saver` persists with a dead pane process and the next bootstrap evaluates the unhealthy-saver path normally."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component C — Stabilise the `daemon.lock` Singleton Against Inode Replacement, step 3 ("Post-flock inode cross-check").

## slow-open-empty-previews-and-zombie-sessions-4-8 | approved

### Task 4-8: AST-walking test asserts WritePIDFile immediately follows acquireDaemonLock in defaultDaemonRun

**Problem**: Component C's correctness depends on the window between `AcquireDaemonLock` returning and `WritePIDFile` completing being bounded by a single function call — implementers MUST NOT insert other work between them. A naive code-review-only enforcement of this invariant is fragile: a future commit could insert a "harmless" line (e.g., a log entry or a stat call) between the two without anyone noticing, re-opening the race window. The spec explicitly requires "A unit test asserts that the source ordering is preserved by walking the function's AST and checking that the call statement immediately following `acquireDaemonLock` is `WritePIDFile`."

**Solution**: Add a unit test that parses `cmd/state_daemon.go` via `go/parser` + `go/ast`, locates the `defaultDaemonRun` function, identifies the assignment statement whose RHS is a call to `acquireDaemonLock`, and asserts that the next statement in source order (allowing for a guarded-equivalent `if err != nil { return }` sandwich) contains a call to `state.WritePIDFile`. The test fails if any new statement intrudes between them.

**Outcome**: A future intruder commit that adds a statement between `acquireDaemonLock` and `WritePIDFile` is caught at test-run time with a clear diagnostic naming the intruding statement.

**Do**:
- Add a new test file `cmd/state_daemon_lock_pid_ordering_test.go` (no integration build tag — this is a pure source-walking unit test).
- The test:
  1. Reads `cmd/state_daemon.go` from disk via `os.ReadFile` (relative path resolution against the test's package directory).
  2. Parses it via `go/parser.ParseFile(fset, "state_daemon.go", src, parser.ParseComments)`.
  3. Walks the AST via `ast.Inspect` to find the `*ast.FuncDecl` whose Name is `defaultDaemonRun`.
  4. Within that function body, walk the `Body.List` statement slice to find the index `i` at which `Body.List[i]` is an `*ast.AssignStmt` whose RHS is a `*ast.CallExpr` to an identifier named `acquireDaemonLock`.
  5. Assert the statement following the lock-acquire is an `*ast.IfStmt` matching the `if err != nil { ... return ... }` shape (the guarded-equivalent sandwich permitted by the spec).
  6. Assert the statement at `i+2` is an `*ast.IfStmt` whose body contains a call to `state.WritePIDFile`. (Pattern: `if err := state.WritePIDFile(dir, os.Getpid()); err != nil { return ... }`.)
  7. Allow comments between statements (comments don't show up as `Body.List` entries — the AST naturally permits this).
  8. Fail with a precise diagnostic naming the intruding statement type if `Body.List[i+2]` is anything other than the expected `WritePIDFile` call.
- Document in the test's preamble that the spec contract is "next call statement after a successful `acquireDaemonLock` is `WritePIDFile`", with `if err != nil { return }` guards permitted between them.
- The test must also verify that `defaultDaemonRun` exists in the expected file path; if the function is moved or renamed in a future refactor, the test fails fast with a clear "function not found" message rather than silently passing.
- Add a `grep`-based companion assertion: assert that `AcquireDaemonLock` (the public name) has exactly one production call site by walking the entire `cmd/` package via `go/packages` or via a simpler `os.ReadFile`-and-string-scan over the `cmd/` directory. The spec asserts "No other production call site of `AcquireDaemonLock` exists". This is a defence against a future caller being added without an ordering check.

**Acceptance Criteria**:
- [ ] Test compiles and runs as a normal unit test (no integration build tag).
- [ ] Test parses `cmd/state_daemon.go` via `go/parser` and locates `defaultDaemonRun` via AST walk.
- [ ] Test asserts the statement following the `acquireDaemonLock` assignment matches the `if err != nil { return }` guarded-equivalent shape.
- [ ] Test asserts the next statement after the guard is the `state.WritePIDFile` call (wrapped in its own `if err != nil { return }` guard, matching current source shape at `state_daemon.go:301-303`).
- [ ] If any other statement is inserted between the lock-acquire and the pid-write, the test fails with a diagnostic naming the intruding statement's AST type and line number.
- [ ] Companion assertion: `AcquireDaemonLock` has exactly one production call site (in `cmd/state_daemon.go`'s `defaultDaemonRun`); the test fails if a second production call site is added.
- [ ] Test passes against the current source.

**Tests** (in `cmd/state_daemon_lock_pid_ordering_test.go`):
- `"AST ordering: acquireDaemonLock is immediately followed by its err-guard and then state.WritePIDFile in defaultDaemonRun"`
- `"AST ordering: test produces a clear diagnostic naming the intruding statement when a synthetic mutation injects an unrelated statement between the two calls (meta-test)"`
- `"AST ordering: AcquireDaemonLock has exactly one production call site in cmd/"`

**Edge Cases**:
- Function renamed from `defaultDaemonRun` — test fails with "function not found" rather than passing silently.
- `state` package prefix dropped (e.g., `WritePIDFile(...)` instead of `state.WritePIDFile(...)`) — match either form via a name-only check on the call's function expression, not the qualified name.
- Comments between the two calls — comments don't appear in `Body.List`, so they're inherently permitted.
- A future refactor introduces `acquireDaemonLock` calls inside other functions (e.g., a `recoverLock` helper) — the companion call-site count assertion catches this.
- The lock-acquire is wrapped in a different control-flow shape (e.g., moved into a helper) — the AST walk doesn't find it inside `defaultDaemonRun` → test fails with "lock acquire not found in defaultDaemonRun body".

**Context**:
> Spec § Component C step 4 — "Post-acquire daemon.pid write". "The daemon must write `daemon.pid` as the next statement after the successful `acquireDaemonLock` return in `cmd/state_daemon.go`'s `defaultDaemonRun` (the existing function at line 70 that hosts the acquire call at line 290 and the pid write at line 301; the two calls are already consecutive). No other production call site of `AcquireDaemonLock` exists; the spec contract is 'production daemon's `defaultDaemonRun` only'. The window between acquire and pid-write must remain bounded by a single `state.WritePIDFile` call — implementers MUST NOT insert other work between them. A unit test asserts that the source ordering is preserved by walking the function's AST and checking that the call statement immediately following `acquireDaemonLock` is `WritePIDFile` (or a guarded equivalent)."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component C — Stabilise the `daemon.lock` Singleton Against Inode Replacement, step 4 ("Post-acquire daemon.pid write").

## slow-open-empty-previews-and-zombie-sessions-4-9 | approved

### Task 4-9: Integration test for Component C upgrade-path two-binary scenario

**Problem**: Component C must handle the real-world upgrade landmine: a v(N) `portal state daemon` is already running and holds the lock; the user upgrades to v(N+1) and invokes `portal open`. The new binary's bootstrap spawns its own daemon — but A and B will sweep the prior daemon, so by the time the new daemon calls `AcquireDaemonLock`, the prior `daemon.pid` is stale. The spec mandates an integration test that exercises this two-binary scenario end-to-end against real subprocesses.

**Solution**: Add an integration test that builds two distinct `portal` binaries (treated as v(N) and v(N+1) via a version-injection lever or simply two builds from the same source), spawns v(N)'s daemon, then invokes v(N+1)'s bootstrap and asserts the converged end state: exactly one `portal state daemon` survives, and a fresh-process `AcquireDaemonLock` invocation from the test refuses with `ErrDaemonLockHeld` (proving Component C's pre-check sees the new live daemon).

**Outcome**: A failing test surfaces any regression that would let a v(N+1) bootstrap silently coexist with a v(N) daemon — the destructive-coexistence scenario Component C exists to prevent.

**Do**:
- Place the test in `internal/state/daemon_lock_upgrade_integration_test.go` (or `cmd/bootstrap/upgrade_path_integration_test.go` if real-tmux scaffolding is more convenient) under the existing integration build tag.
- Use `portaltest.NewIsolatedStateEnv(t)` for state-dir isolation.
- Use `portalbintest.BuildPortalBinary` to build the binary. The "two binaries" can be:
  - **Option A** (preferred): build the same source twice into two distinct paths (`portalA`, `portalB`) so they exec from different inodes; PID-recycle and identity-check semantics are identical.
  - **Option B**: build once and use the same binary twice; mark the two daemons by env var (`PORTAL_DAEMON_TAG=A` vs `B`) so test diagnostics can distinguish them. Either is acceptable — the spec's "v(N) / v(N+1)" framing is illustrative, not literal version semantics.
- Use `tmuxtest` to start a real tmux server. Create `_portal-saver` with a placeholder pane process (post-Phase-3 ordering — `tail -f /dev/null`), then set `destroy-unattached=off`, then `respawn-pane -k` to launch the v(N) daemon as the pane process.
- Wait until the v(N) daemon has written `daemon.pid` (poll up to 3 s).
- Now invoke v(N+1)'s bootstrap. Two equivalent forms:
  - **Form A**: invoke `portal open` (the new binary) as a subprocess against the same tmux server + isolated state dir. The subprocess runs the full bootstrap orchestrator (including Components A+B+C+F).
  - **Form B**: construct the production `Orchestrator` directly from the test code (via `internal/bootstrapadapter.NewProductionOrchestrator` or equivalent) and call `Run`. This is the cleaner test surface and avoids subprocess output capture.
- Assert convergence:
  - `pgrep -fxc 'portal state daemon'` returns 1 within 6 s of bootstrap entry.
  - The surviving PID is NOT the v(N) PID — A/B swept it; the new saver-pane daemon (from Component F's respawn) is the survivor.
  - `daemon.pid` on disk references the survivor.
- Assert Component C pre-check from a third (test-side) process: invoke `state.AcquireDaemonLock(stateDir)` directly from the test goroutine. It must return `state.ErrDaemonLockHeld` — the new daemon's pre-check refuses the test's acquire.
- Alternative branch (no-A/B coverage hypothesis): also exercise the case where the v(N) daemon was NOT swept (e.g., temporarily disable Component B's sweep via a seam). In that case, the v(N+1) daemon's `AcquireDaemonLock` must refuse cleanly via the pre-check — no destructive coexistence. This branch exercises C's structural defence in isolation.

**Acceptance Criteria**:
- [ ] Test runs only under the integration build tag.
- [ ] Test uses `portaltest.NewIsolatedStateEnv` and `portalbintest.BuildPortalBinary`.
- [ ] Scenario A — full A+B+C composition: v(N) daemon is swept by B, v(N+1) daemon acquires cleanly, `pgrep -fxc == 1` within 6 s.
- [ ] Scenario B — C in isolation (Sweep disabled via seam): v(N) daemon survives the bootstrap window, v(N+1)'s `AcquireDaemonLock` refuses via the pre-check (`ErrDaemonLockHeld`), no destructive coexistence (sessions.json not overwritten by the loser).
- [ ] Test-side `state.AcquireDaemonLock(stateDir)` from a fresh goroutine after bootstrap returns `ErrDaemonLockHeld` (proves Component C's pre-check is live).
- [ ] All spawned daemons cleaned up via `t.Cleanup` regardless of outcome.

**Tests**:
- `"upgrade-path: v(N) running, v(N+1) bootstrap converges to a single daemon via A+B+C composition"`
- `"upgrade-path: C in isolation (Sweep disabled) — v(N+1) refuses cleanly via pre-check, no destructive coexistence"`
- `"upgrade-path: post-bootstrap fresh-process AcquireDaemonLock returns ErrDaemonLockHeld"`

**Edge Cases**:
- v(N) daemon exits between bootstrap entry and `AcquireDaemonLock` call (e.g., A's escalation runs successfully and the daemon dies just before C's pre-check) — the pre-check sees `IdentifyDead`, proceeds to acquire, succeeds. This is the legitimate-succession case; test must not fail it.
- v(N) daemon's `daemon.pid` is stale (the daemon exited earlier without cleanup) — pre-check sees a dead PID, proceeds. Test exercises this.
- A/B disabled in scenario B — the test must wire the orchestrator with no-op Sweep / no-op Escalation seams while leaving C live. Confirm via a directed test-deps construction in the bootstrap-construction site.
- `pgrep` not on PATH — `t.Skip` with a clear reason, consistent with Task 4-5.

**Context**:
> Spec § Component C acceptance criterion — "Upgrade-path scenario": "Simulate the real-world upgrade landmine: spawn a v(N) `portal state daemon` that holds the lock, then invoke a v(N+1) binary bootstrap (the existing in-flight daemon was launched by the prior binary; the new binary's bootstrap spawns its own daemon). With Components A+B+C, the new bootstrap's daemon either acquires cleanly (because A/B swept the prior daemon and `daemon.pid` is no longer live) or refuses cleanly via the pre-check (no destructive coexistence). Verified by integration test that constructs the two-binary scenario."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Component C — Stabilise the `daemon.lock` Singleton Against Inode Replacement, "Upgrade-path scenario" acceptance criterion.

## slow-open-empty-previews-and-zombie-sessions-4-10 | approved

### Task 4-10: Composite integration test — A+B+C converge to pgrep -fxc == 1 within 6 s

**Problem**: Tasks 4-1 through 4-9 prove the components individually. The spec's Phase 4 acceptance also calls for an end-to-end composition check: given the reporter's failure scenario (one legitimate daemon + two orphans, with the orphan profile mirroring the install — one with a `daemon.pid` reference, one without), the post-bootstrap end state must be exactly one live `portal state daemon` within 6 s, with scrollback stable across 10 consecutive 1-second observations, and Component C's pre-check verifiable from a fresh process. This is the phase-internal composition test (Phase 6 provides the broader composite that also incorporates D, E, F).

**Solution**: Add an integration test that reconstructs the three-daemon reporter scenario, invokes the production bootstrap orchestrator with Components A+B+C active, and asserts the converged end state across `pgrep`, scrollback stability, and a fresh-process `AcquireDaemonLock` refusal.

**Outcome**: A failing composition test surfaces any regression in the A+B+C interaction — a class of bug that per-component tests cannot catch.

**Do**:
- Place the test in `cmd/bootstrap/composition_abc_integration_test.go` (or `internal/restoretest/` if the real-tmux scaffolding there is more convenient — the planning task list explicitly allows either) under the integration build tag.
- Use `portaltest.NewIsolatedStateEnv(t)` for state-dir isolation.
- Use `portalbintest.BuildPortalBinary` to build the binary.
- Use `tmuxtest` to start a real tmux server with `_portal-saver` plus 2 user sessions.
- Setup phase:
  1. Create `_portal-saver` with the legitimate daemon as its pane process (post-Phase-3 ordering — placeholder → set `destroy-unattached=off` → respawn to `portal state daemon`).
  2. Wait until the legitimate daemon has written `daemon.pid`.
  3. Spawn 2 orphan daemons against the same `stateDir`, isolated env. Orphan 1: starts after the legitimate daemon and overwrites `daemon.pid` with its own PID (simulating the "orphan with a daemon.pid reference" reporter case). Orphan 2: starts but does NOT write `daemon.pid` — it's only visible via `pgrep`. Both are direct test subprocesses (not tmux pane processes), so their parent is the test, NOT the saver pane process.
  4. Assert pre-state: `pgrep -fxc 'portal state daemon' == 3`.
- Bootstrap phase:
  5. Invoke the production `Orchestrator.Run` directly (preferred) or `portal open` as a subprocess. Start a timer at orchestrator entry.
- Post-bootstrap assertions:
  6. Poll `pgrep -fxc 'portal state daemon'` until it returns 1, bounded to 6 s from orchestrator entry. The 6 s ceiling is A's escalation budget (5 s session-kill + 1 s SIGKILL) + B's sweep latency (sub-second).
  7. Identify the survivor: its PID must equal `_portal-saver`'s pane PID (i.e., the saver-pane daemon respawned by Component F during bootstrap step 5, OR if Phase 3 is the predecessor of this task, the originally-spawned saver-pane daemon — either is acceptable since the post-state is the same).
  8. Observe scrollback stability: every 1 second for 10 consecutive observations, snapshot `<stateDir>/scrollback/` (path-keyed fingerprint map). Assert every observation is byte-identical to the first observation — no `.bin` deletions, no unexpected new files, no size/mtime changes on existing `.bin` files.
  9. Fresh-process Component C pre-check verification: from the test goroutine, call `state.AcquireDaemonLock(stateDir)`. Assert it returns `state.ErrDaemonLockHeld` — the survivor's `daemon.pid` is live and identity-checks, so the pre-check refuses.
- Cleanup:
  10. `t.Cleanup` kills all daemon processes, tears down the tmux server, and runs the fingerprint-diff check against the developer's real state dir (via the Phase 1 helper).

**Acceptance Criteria**:
- [ ] Test runs only under the integration build tag.
- [ ] Test uses `portaltest.NewIsolatedStateEnv` and `portalbintest.BuildPortalBinary`.
- [ ] Pre-state assertion: `pgrep -fxc 'portal state daemon' == 3` before bootstrap.
- [ ] Post-bootstrap: `pgrep -fxc 'portal state daemon' == 1` within 6 s of orchestrator entry.
- [ ] Survivor PID matches `_portal-saver`'s pane PID via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
- [ ] Scrollback stability: 10 consecutive 1-second snapshots are byte-identical; any delta fails the test with a path-and-field diagnostic (reuse the fingerprint diff from Task 4-2).
- [ ] Fresh-process `state.AcquireDaemonLock(stateDir)` from the test returns `state.ErrDaemonLockHeld`.
- [ ] All spawned subprocesses cleaned up via `t.Cleanup`.
- [ ] Test fails (not silently passes) if `pgrep` is unavailable — `t.Skip` with a clear message rather than a green run.

**Tests**:
- `"composition A+B+C: 3 daemons converge to 1 within 6 s after orchestrator entry"`
- `"composition A+B+C: scrollback directory stable across 10×1 s post-bootstrap observations"`
- `"composition A+B+C: fresh-process AcquireDaemonLock refuses with ErrDaemonLockHeld via Component C pre-check"`
- `"composition A+B+C: survivor is the saver-pane daemon, not one of the orphans"`

**Edge Cases**:
- One orphan exits naturally before bootstrap (e.g., it crashes) — pre-state assertion catches this and fails clearly rather than silently proceeding with N=2.
- The legitimate daemon dies during bootstrap (e.g., A's escalation accidentally kills the wrong PID) — survivor mismatch assertion catches this; the test fails with the actual vs. expected PID.
- Scrollback dir is empty at the start of the 10×1s observation window — the test must seed activity (e.g., create a user pane with non-trivial output) so the snapshot is meaningful; an empty-then-empty pair is not informative.
- 6 s budget exceeded by a small margin (e.g., 6.5 s) — test fails with the measured time; do NOT add slack to the budget without a spec update.
- Subprocess cleanup leaves zombie processes — `t.Cleanup` must `kill -9` everything it spawned and `wait` for reap.
- Composition with Phase 3 (F) — this test assumes Phase 3 has shipped, so saver creation uses the placeholder-then-respawn ordering. Document the phase dependency in the test's preamble.

**Context**:
> Spec § Composite End-to-End Verification (extracted Phase-4-scoped subset): the broader composite test in Phase 6 incorporates D, E, F as well. This Phase-4 task covers the A+B+C subset and verifies the "3 daemons converge to 1, scrollback stable, C pre-check live" assertions before D/E/F are layered in. The phase planning explicitly lists this as task 4-10.
>
> The 6 s budget is Component A's escalation budget (5 s session-kill poll + 1 s SIGKILL poll) plus B's sub-second sweep latency. Tighter budgets are out of scope.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` § Composite End-to-End Verification (Phase-4-scoped subset), and Phase 4 planning composition acceptance: "bootstrap against three concurrent daemons (1 legitimate + 2 orphans) converges to `pgrep -fxc 'portal state daemon' == 1` within ~6 s".
