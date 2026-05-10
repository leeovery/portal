# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 3)

- topic: killed-sessions-resurrect-on-restart
- cycle: 3
- total_proposed: 2

---

## Task 1: Refresh stale doc-comment cross-references to renamed/relocated primitives
status: approved
severity: low
sources: standards, architecture

**Problem**: Three source-file doc-comments name primitives that no longer exist. `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:28` documents `EagerSignalCore.Signaler` as "Tests inject recordingFIFOSignaler" — but cycle-2 T5-1 promoted that private fake to `internal/statetest` and renamed it `statetest.RecordingFIFOSignaler`; the private symbol no longer exists (`grep -rn "recordingFIFOSignaler" /Users/leeovery/Code/portal --include="*.go"` returns only this single doc hit). `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go:152` describes `DriveSignalHydrate` as "byte-equivalent to `writeFIFOSignal` in `cmd/state_signal_hydrate.go`", and `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go:256` describes `openAndSignalFIFO` as "Byte-equivalent to `cmd/state_signal_hydrate.writeFIFOSignal`" — but T1-1 relocated that helper into `internal/state` (now `state.WriteFIFOSignal` and its no-seam wrapper `state.SendHydrateSignal`); the cmd-side primitive named in both docstrings has not existed for the visible commit history of this work unit. All three references describe load-bearing seam relationships — exactly the bridges a maintainer follows to understand seam quality. T5-3 fixed the same class of drift in CLAUDE.md; these three source-file surfaces were out of T5-3's scope.

**Solution**: Apply one-line doc-comment edits at each of the three sites to name the post-T1-1 / post-T5-1 primitives.

**Outcome**: Every doc-comment that references the FIFO-signaler fake or the FIFO-signal helper names a symbol that currently exists in the repo. `grep -rn "recordingFIFOSignaler" /Users/leeovery/Code/portal --include="*.go"` returns no hits; `grep -rn "writeFIFOSignal" /Users/leeovery/Code/portal --include="*.go"` returns no hits outside analysis docs.

**Do**:
1. Edit `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go` line 28: replace `Tests inject recordingFIFOSignaler.` with `Tests inject statetest.RecordingFIFOSignaler.`
2. Edit `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go` line 152: replace ``byte-equivalent to `writeFIFOSignal` in `cmd/state_signal_hydrate.go``` with ``byte-equivalent to `state.WriteFIFOSignal` / `state.SendHydrateSignal` in `internal/state```.
3. Edit `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go` line 256: replace ``Byte-equivalent to `cmd/state_signal_hydrate.writeFIFOSignal``` with ``Byte-equivalent to `state.WriteFIFOSignal` in `internal/state```.
4. Run `grep -rn "recordingFIFOSignaler" /Users/leeovery/Code/portal --include="*.go"` and `grep -rn "writeFIFOSignal" /Users/leeovery/Code/portal --include="*.go"` to confirm zero remaining hits outside analysis docs.
5. Run `go build ./...` to confirm doc-comment placement is intact.

**Acceptance Criteria**:
- `cmd/bootstrap/eager_signal_hydrate.go:28` names `statetest.RecordingFIFOSignaler` (a symbol that exists) and no longer names `recordingFIFOSignaler`.
- `internal/restoretest/restoretest.go:152` names `state.WriteFIFOSignal` / `state.SendHydrateSignal` in `internal/state`; no longer names `writeFIFOSignal` or `cmd/state_signal_hydrate.go`.
- `internal/restoretest/restoretest.go:256` names `state.WriteFIFOSignal` in `internal/state`; no longer names `cmd/state_signal_hydrate.writeFIFOSignal`.
- `grep -rn "recordingFIFOSignaler" /Users/leeovery/Code/portal --include="*.go"` returns zero hits.
- `grep -rn "writeFIFOSignal" /Users/leeovery/Code/portal --include="*.go"` returns zero hits.
- `go build ./...` succeeds.

**Tests**:
- No new tests required — doc-comment-only edits with no behavioural change.
- Existing `go test ./...` suite continues to pass unchanged.

---

## Task 2: Extract `bootstrapadapter.NewRestoreAdapter` constructor and adopt at four new integration-test sites
status: approved
severity: low
sources: duplication

**Problem**: Every bootstrap-integration test added by this work unit that exercises a real `RestoreAdapter` open-codes the same preamble: `restoreInner := &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}` immediately followed by `Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner}` inside the `buildIntegrationOrchestrator` opts literal. Four new sites added by this work unit are byte-equivalent at the field-set level (only variable scoping differs): `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:193-197`, `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:332-336`, `/Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:162-166`, `/Users/leeovery/Code/portal/cmd/bootstrap/phase5_marker_suppression_integration_test.go:118-122`. Counting pre-existing sites the same shape also appears at `cmd/bootstrap/phase5_integration_test.go:169` / `:263`, `cmd/bootstrap/reboot_roundtrip_test.go:328` / `:939` / `:1191`, `cmd/reattach_integration_test.go:173`, and `cmd/bootstrap_production.go:111` — eleven total, four added by this work unit. Same "single-shape preamble at every integration site" pattern cycle 1's `SeedSessionsJSON` extraction addressed.

**Solution**: Add a `NewRestoreAdapter(client *tmux.Client, stateDir string, logger *state.Logger) *RestoreAdapter` constructor to `internal/bootstrapadapter` that builds the inner `*restore.Orchestrator` and wraps it in `*RestoreAdapter` in one call. Adopt at the four new sites added by this work unit. The helper has no logic — purely struct-literal-and-wrap — so behaviour cannot drift independently of the wrapped types.

**Outcome**: The four new integration-test sites collapse from a five-line preamble to a single `Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` field. Net deletion ~16 lines across the four new sites. The pre-existing seven sites are unchanged and can opt in over time. All bootstrap integration tests continue to pass.

**Do**:
1. In `/Users/leeovery/Code/portal/internal/bootstrapadapter/` (sibling to the existing `RestoreAdapter`) add a constructor in the same file the `RestoreAdapter` type lives in (or a new sibling file if more appropriate to package layout):
   ```go
   // NewRestoreAdapter constructs a RestoreAdapter wrapping a fresh restore.Orchestrator.
   // Equivalent to: &RestoreAdapter{Inner: &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}}.
   func NewRestoreAdapter(client *tmux.Client, stateDir string, logger *state.Logger) *RestoreAdapter {
       return &RestoreAdapter{Inner: &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}}
   }
   ```
   Verify parameter types (especially `*state.Logger`) against the actual types already used at the eleven open-coded sites before finalising the signature — read the existing `RestoreAdapter` definition and the open-coded preamble at `cmd/bootstrap/phase5_integration_test.go:169`.
2. Replace the open-coded preamble at each of the four new sites: drop the local `restoreInner := &restore.Orchestrator{...}` line, replace `Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner}` with `Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` at:
   - `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:193-197`
   - `/Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:332-336`
   - `/Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:162-166`
   - `/Users/leeovery/Code/portal/cmd/bootstrap/phase5_marker_suppression_integration_test.go:118-122`
3. Leave the seven pre-existing sites untouched — out of scope for this work unit.
4. Run `go build ./...` and `go test ./cmd/bootstrap/...`.
5. Run `go test ./...` to confirm no broader regression.

**Acceptance Criteria**:
- `internal/bootstrapadapter` exposes `NewRestoreAdapter(client *tmux.Client, stateDir string, logger *state.Logger) *RestoreAdapter` returning `&RestoreAdapter{Inner: &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}}` (parameter types match the actual types used at the open-coded sites).
- The four new sites listed above no longer declare a local `restoreInner := &restore.Orchestrator{...}`; each calls `bootstrapadapter.NewRestoreAdapter(...)` inline in the opts literal.
- `go build ./...` succeeds.
- `go test ./cmd/bootstrap/...` passes.
- `go test ./...` passes.
- The seven pre-existing sites (`phase5_integration_test.go:169` / `:263`, `reboot_roundtrip_test.go:328` / `:939` / `:1191`, `reattach_integration_test.go:173`, `bootstrap_production.go:111`) remain unchanged.

**Tests**:
- No new tests required — the constructor has no logic, only struct construction, and is exercised transitively by the four migrated integration tests on every run.
- Existing `go test ./cmd/bootstrap/...` continues to cover all four migrated call sites end-to-end.
