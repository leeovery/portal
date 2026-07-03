---
topic: skip-bootstrap-when-warm
cycle: 1
total_proposed: 2
---
# Analysis Tasks: Skip Bootstrap When Warm (Cycle 1)

## Task 1: Consolidate duplicated scaffolding in the abridged bootstrap tests
status: approved
severity: medium
sources: duplication

**Problem**: The new abridged-bootstrap test suite carries two distinct duplications that hit the rule-of-three threshold and will drift as the feature evolves.
(a) Three structurally identical ~25-line latch-verdict "full bootstrap" tests — `TestPersistentPreRunE_FullBootstrap_WhenLatchAbsent`, `_OnVersionMismatch`, and `_OnLatchReadError` in `cmd/abridged_route_test.go` (lines ~85-171) — differ only in the `show-option` RunFunc return (`optionAbsentErr()` / `"v1.0.0"` / `errors.New("tmux socket connect failed")`), an optional version override, and the assertion failure string. Every other line (resetBootstrapOnce, recordingCommander client construction, recordingRunner{started:false}, bootstrapDeps wiring + Cleanup, installMockList, resetRootCmd, SetArgs([]string{"list"}), Execute, and the `runner.calls != 1` assertion) is copy-pasted verbatim.
(b) The same "saver absent, revive fails" recordingCommander RunFunc (`list-panes` -> `noSuchSessionErr()`, `has-session` -> "can't find session", `new-session` -> "create denied", plus `show-option` -> version on the PersistentPreRunE paths) is inlined three times across two files: `cmd/abridged_route_test.go` twice (byte-identical, in `_Abridged_EmitsWarningsToStderrOnCLIPath` and `_LeavesWarningsForOpenTUIOnTUIPath`) and `cmd/abridged_saver_test.go` once (`TestEnsureSaverLiveness_FunnelsSaverDownWarning`, the same switch minus the `show-option` arm). The same package already extracts sibling fixtures for exactly this reason (`satisfiedLatchAliveSaverCommander()`, `notSatisfiedLatchClient()`), so this scenario is the odd one left inline.

**Solution**:
(a) Collapse the three latch-verdict tests into one table-driven test (e.g. `TestPersistentPreRunE_FullBootstrap_WhenNotSatisfied`) whose cases carry `name`, the `show-option` return `(value string, err error)`, an optional version override, and the assertion message, sharing one body (setup -> Execute -> assert calls==1). Reuse the existing `optionAbsentErr()` helper for the absent case.
(b) Add a shared `saverAbsentReviveFailsCommander()` fixture mirroring `satisfiedLatchAliveSaverCommander()`, returning the `*recordingCommander` with the list-panes/has-session/new-session arms. The two route tests wrap it with a `show-option` -> version arm (or accept a bool for the latch arm); the abridged_saver test uses the base fixture directly (it drives `ensureSaverLiveness`, which never reads the latch).

**Outcome**: The abridged test suite holds a single authoritative copy of each scaffold; a future bootstrapDeps field, routing tweak, or saver-revive sequence change is applied once instead of three times, and the two files can no longer silently drift out of lockstep.

**Do**:
- In `cmd/abridged_route_test.go`, replace the three functions (~85-171) with one table-driven `TestPersistentPreRunE_FullBootstrap_WhenNotSatisfied`; each case supplies the `show-option` `(value string, err error)` return, an optional version override, and the assertion message; keep the single shared body and reuse `optionAbsentErr()` for the absent case.
- Add `saverAbsentReviveFailsCommander()` alongside the existing sibling fixtures, returning the `*recordingCommander` with the list-panes/has-session/new-session arms; provide a way to add the `show-option` -> version arm (a bool param or a thin wrap).
- Update `_Abridged_EmitsWarningsToStderrOnCLIPath` and `_LeavesWarningsForOpenTUIOnTUIPath` to use the fixture plus the `show-option` arm; update `TestEnsureSaverLiveness_FunnelsSaverDownWarning` to use the base fixture directly.
- Change no asserted behaviour — this is a pure consolidation.

**Acceptance Criteria**:
- The three latch-verdict tests are one table-driven test with at least three cases covering latch-absent, version-mismatch, and latch-read-error, each asserting `runner.calls == 1`.
- The saver-absent-revive-fails commander exists as one shared fixture in the cmd test package; no inline copy of that RunFunc remains in `abridged_route_test.go` or `abridged_saver_test.go`.
- No change to what any test asserts; `go test ./cmd` passes.

**Tests**:
- `go test ./cmd -run TestPersistentPreRunE_FullBootstrap` — the collapsed table cases (absent / mismatch / read-error) all green.
- `go test ./cmd -run TestPersistentPreRunE_Abridged` and `go test ./cmd -run TestEnsureSaverLiveness` — green using the shared fixture.

## Task 2: Decouple daemon capture startup from best-effort hooks-cleanup store resolution
status: approved
severity: low
sources: architecture

**Problem**: The daemon gained hooks stale-cleanup as an explicitly best-effort responsibility — `maybeRunHookCleanup` logs WARN and swallows every cleanup error, per the documented "never crash the daemon" posture. But its dependency wiring is fatal: `loadHookStore()` failing in the daemon `RunE` (`cmd/state_daemon.go:698-701`) returns an error that aborts daemon startup before the tick loop begins, so an unresolvable hooks-store path takes down the daemon's PRIMARY responsibility (scrollback capture + resurrection state), not just the inert cleanup feature. This is a posture inconsistency (fatal wiring vs soft runtime for the same non-critical subsystem) that couples capture availability to a secondary feature's dependency. The trigger is currently low-probability — `loadHookStore()` only fails on path resolution, which shares XDG resolution with `state.EnsureDir()` run earlier in `RunE` and would already have aborted — so this is a latent structural/robustness concern rather than a live bug, but the coupling is real and inverts the subsystem's own "degrade, don't crash" intent.

**Solution**: Degrade cleanup rather than the daemon. On `loadHookStore()` failure, log one WARN (so the disabled-cleanup state stays observable per the existing comment's stated intent) and proceed with a nil/absent store; make `maybeRunHookCleanup` no-op when the store is unavailable. This keeps capture (the daemon's reason for existing) independent of the cleanup feature's wiring and makes the startup posture consistent with the runtime "never crash the daemon" posture.

**Outcome**: A hooks-store path-resolution failure disables only cleanup (with an observable WARN) and never aborts the daemon; the capture/commit tick loop starts regardless of the hooks-store outcome.

**Do**:
- In `cmd/state_daemon.go` `RunE`, change the `loadHookStore()` call site (~698-701) so a resolution error logs one WARN (component `daemon`, `error` attr) and leaves the store nil, instead of returning the error and aborting startup.
- Guard `maybeRunHookCleanup` (~413-421) to no-op when the store is nil — skip the throttle body without disturbing `lastCleanup` semantics or the capture path.
- Keep the existing `runHookStaleCleanup` argument set unchanged on the store-present path.

**Acceptance Criteria**:
- `loadHookStore()` failure no longer returns an error from daemon `RunE`; the daemon proceeds to its tick loop with cleanup disabled.
- Exactly one WARN is emitted on the disabled-cleanup path, using only the closed vocabulary (`error` attr under the `daemon` component) — no new attr or event invented.
- `maybeRunHookCleanup` is a no-op when the store is nil; capture/commit and the self-supervision probe are unaffected.
- Existing state_daemon tests (run, hook-cleanup) remain green.

**Tests**:
- Unit: with `loadHookStore` forced to fail, `RunE` reaches the tick loop (does not return the resolution error) and emits the disabled-cleanup WARN once.
- Unit: `maybeRunHookCleanup` with a nil store no-ops (no panic, no throttle mutation surprise) and does not touch the capture path.
- `go test ./cmd -run TestStateDaemon` (daemon run + hook-cleanup suites) green.
