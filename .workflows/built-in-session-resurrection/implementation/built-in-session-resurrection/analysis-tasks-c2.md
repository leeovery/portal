---
topic: built-in-session-resurrection
cycle: 2
total_proposed: 10
---
# Analysis Tasks: built-in-session-resurrection (Cycle 2)

## Task 1: Update CLAUDE.md to remove pre-resurrection mechanisms
status: approved
severity: high
sources: standards

**Problem**: CLAUDE.md still documents removed internals — `WaitForSessions` in the tmux Client method list (L42); the `hooks` package row mentions `ExecuteHooks` and `@portal-active-<pane>` volatile markers (L49); the server-bootstrap section claims `PersistentPreRunE` calls `bootstrapWait()` with a "1–6s window" (L63); an entire "Resume hooks" paragraph (L65-67) describes send-keys-driven attach-time firing with `@portal-active-<pane>` dual-level tracking. `grep -rn "ExecuteHooks\|WaitForSessions\|bootstrapWait" --include="*.go"` returns zero hits. CLAUDE.md is the first thing a new contributor or future Claude session reads.

**Solution**: Rewrite the four affected regions to describe the resurrection-era architecture (`bootstrap.Orchestrator`, hydrate-helper-driven hook firing, skeleton/scrollback two-phase restore, `internal/state` and `internal/restore` packages, `_portal-saver` daemon session).

**Outcome**: CLAUDE.md accurately describes the current architecture; no references to deleted symbols remain.

**Do**:
1. CLAUDE.md L42 (tmux Client method list): drop `WaitForSessions`. Add `RespawnPane`, `SetSessionOption`, `IsRestoringSet`, and any other tmux methods introduced by the resurrection work.
2. CLAUDE.md L49 (`hooks` package row): rewrite to describe the package as the JSON-backed `Store` only. Drop `ExecuteHooks` and `@portal-active-<pane>`; note that hook firing now lives in the hydrate helper's exec chain.
3. CLAUDE.md L63 (server bootstrap section): rewrite to describe the multi-step `bootstrap.Orchestrator` and the TUI loading-page minimum pad. Remove the "1–6s polling window" claim and the `bootstrapWait()` reference.
4. CLAUDE.md L65-67 (Resume hooks paragraph): replace with a paragraph stating hooks fire only inside the hydrate helper's exec chain after skeleton-restore (reboot recovery), not on every detach/reattach. Drop the `@portal-active-<pane>` dual-level tracking description.
5. Add a brief mention of the new `internal/state` and `internal/restore` packages and the `_portal-saver` detached session hosting `portal state daemon`.

**Acceptance Criteria**:
- `grep -n "ExecuteHooks\|WaitForSessions\|bootstrapWait\|@portal-active" CLAUDE.md` returns no hits.
- The tmux package row lists current methods; the hooks row describes the Store only; the bootstrap section describes the Orchestrator; the resume-hooks paragraph describes hydrate-time firing.
- New `internal/state`, `internal/restore`, and `_portal-saver` mentions appear.

**Tests**:
- N/A (documentation change). Verify via `grep` against deleted symbol names.

---

## Task 2: Promote skipIfNoTmux to internal/tmuxtest
status: approved
severity: high
sources: duplication

**Problem**: The 5-line helper `func skipIfNoTmux(t *testing.T)` is defined verbatim in `internal/restore/integration_test.go:35-40` and `cmd/bootstrap/phase5_integration_test.go:39-44`. The phase5 file even comments "Mirrors internal/restore/integration_test.go's helper of the same name". Both files already import `internal/tmuxtest`, the home for cross-package integration-test scaffolding established in T7-7. Future skip-semantics changes (TMUX_VERSION env override, OS gate) would need to touch both copies.

**Solution**: Move the helper into `internal/tmuxtest` as `tmuxtest.SkipIfNoTmux(t *testing.T)`; delete both local copies; update call sites.

**Outcome**: One canonical skip helper for tmux-absent CI environments; no drift risk.

**Do**:
1. Add `SkipIfNoTmux(t *testing.T)` to `internal/tmuxtest` (e.g. in `internal/tmuxtest/skip.go`).
2. Delete `func skipIfNoTmux` from `internal/restore/integration_test.go:35-40`.
3. Delete `func skipIfNoTmux` (and its mirror comment) from `cmd/bootstrap/phase5_integration_test.go:39-44`.
4. Replace each `skipIfNoTmux(t)` call site in both files with `tmuxtest.SkipIfNoTmux(t)`.
5. Verify `internal/tmuxtest` is already imported by both files; add the import if needed.

**Acceptance Criteria**:
- `grep -rn "func skipIfNoTmux" --include="*.go"` returns zero hits.
- `grep -rn "tmuxtest.SkipIfNoTmux" --include="*.go"` returns at least the two converted call sites.
- `go test ./internal/restore/... ./cmd/bootstrap/...` passes.
- On a system without tmux, both integration test files still skip cleanly.

**Tests**:
- Existing integration tests cover the call paths; no new tests required.

---

## Task 3: Update specification to describe respawn-pane -k arming
status: approved
severity: medium
sources: standards

**Problem**: The spec still describes the hydrate helper as the pane's *initial* process at L632, L730-740, L750, and L1022. The chosen implementation arms the helper via `tmux respawn-pane -k` after creating the pane with a default shell (T7-9). Comments at `internal/restore/session.go:165-170` and `:491-495` already acknowledge that `respawn-pane` is "load-bearing for the spec's 'helper pre-shell' contract" — i.e., the spec is preserved semantically but not literally.

**Solution**: Update the spec passages to describe the respawn-pane-arming mechanism directly, preserving the semantic guarantee that no shell output races the helper.

**Outcome**: Spec text matches implementation; no ambiguity between "initial process" wording and `respawn-pane -k` reality.

**Do**:
1. `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:632`: change the `new-session -d -s <name> -c <root_cwd> "<hydrate command for first pane>"` example to a default-shell form (omit the trailing command), and add a sentence stating the helper is dispatched via `tmux respawn-pane -k` immediately after pane creation.
2. L730-740: rephrase "Each skeleton-restored pane is created with a shell-pipeline command **as its initial process**" to "Each skeleton-restored pane is **respawned** immediately after creation with a shell-pipeline command via `respawn-pane -k`. respawn-pane atomically kills the default shell and replaces the pane's process with the helper, so the shell does not produce output before the helper runs." Keep the L750 "shell does not exist yet when the bytes are written" claim — `respawn-pane -k` is atomic.
3. L1022 (Bootstrap Flow step 5.2): replace the literal `new-session ... "sh -c 'portal state hydrate ...'"` example with the default-shell form and append a sub-step describing the `respawn-pane -k` invocation.
4. Optionally: add a short "Implementation Note: respawn-pane arming" subsection cross-referencing `internal/restore/session.go`.

**Acceptance Criteria**:
- Spec L632, L730-740, and L1022 describe respawn-pane-based arming.
- The semantic guarantee ("shell does not produce output before the helper runs") is preserved.
- The phrase "as its initial process" no longer appears in skeleton-restore context.

**Tests**:
- N/A (specification-only change). Verify by reading the three updated regions in context.

---

## Task 4: Centralise AtomicWrite + chmod 0600 doublet
status: approved
severity: medium
sources: duplication

**Problem**: Two production writers in `internal/state` perform the identical pair: `fileutil.AtomicWrite(path, data)` immediately followed by `_ = os.Chmod(path, 0o600)` — at `internal/state/commit.go:47-50` and `internal/state/scrollback.go:107-110`. Each repeats a comment justifying chmod as "defensive against umask". A future writer that forgets the chmod silently regresses file permissions. `internal/state/daemon_state.go`'s `WritePIDFile` / `WriteVersionFile` intentionally skip the explicit chmod, so two inconsistent shapes coexist within one package.

**Solution**: Add `fileutil.AtomicWrite0600(path string, data []byte) error` (or a private helper inside `internal/state`) that wraps `AtomicWrite` + post-write chmod. Centralise the umask-defence comment on the helper.

**Outcome**: Both call sites collapse to a single helper call; future state-writers needing 0600 use one canonical helper.

**Do**:
1. Decide placement:
   - If `fileutil` is the right home: add `fileutil.AtomicWrite0600(path string, data []byte) error` to `internal/fileutil/`. Document the umask-defence rationale in the doc comment.
   - If state-only: add a private helper `atomicWrite0600` in `internal/state/`. Same doc comment.
2. Replace `internal/state/commit.go:47-50` with a single call to the new helper.
3. Replace `internal/state/scrollback.go:107-110` with a single call to the new helper.
4. Leave `internal/state/daemon_state.go`'s `WritePIDFile` / `WriteVersionFile` unchanged; add a brief comment cross-referencing the new helper to make the intentional divergence obvious.

**Acceptance Criteria**:
- `grep -rn "os.Chmod.*0o?600" internal/state/` returns no hits in commit.go or scrollback.go.
- Both writers call the new helper.
- `go test ./internal/state/...` passes; written files still have 0600 permissions on a system with a permissive umask.

**Tests**:
- Add a test for the new helper that writes a file under a temporarily-relaxed umask (e.g. `syscall.Umask(0)`) and asserts the resulting file's mode is exactly 0600.

---

## Task 5: Extract unsetSkeletonMarkerOrLog helper for hydrate paths (incl. FIFO→marker convenience)
status: approved
severity: medium
sources: duplication, architecture

**Problem**: Two hydrate-path call sites perform the same operation. `cmd/state_hydrate.go:178-180` (signal-arrived path in runHydrate) and `cmd/state_hydrate.go:302-305` (file-missing recovery path in handleHydrateFileMissing) each derive `livePaneKey` from `cfg.FIFO`, call `state.UnsetSkeletonMarker(cfg.Client, livePaneKey)`, and on error log a WARN. Architecturally, there's a load-bearing convention — `FIFOPath` embeds paneKey, `PaneKeyFromFIFOPath` inverts it (`internal/state/paths.go:97-112`), the hydrate helper uses that inverse to find the right marker — that's never expressed as a single derived relationship.

**Solution**: Introduce `state.UnsetSkeletonMarkerForFIFO(w ServerOptionWriter, fifoPath string) error` that composes `PaneKeyFromFIFOPath` + `UnsetSkeletonMarker`. Add a private `unsetSkeletonMarkerOrLog(cfg hydrateConfig)` in `cmd/state_hydrate.go` that calls the new state helper and emits the WARN on error. Replace both hydrate call sites.

**Outcome**: The FIFO→marker invariant lives in one helper; the two hydrate sites collapse to a single call each; the WARN format is defined once.

**Do**:
1. Add `func UnsetSkeletonMarkerForFIFO(w ServerOptionWriter, fifoPath string) error` to `internal/state/markers.go`. Body: `return UnsetSkeletonMarker(w, PaneKeyFromFIFOPath(fifoPath))`.
2. Doc-comment notes it encodes the FIFOPath ⇄ paneKey invariant; cross-reference `PaneKeyFromFIFOPath`.
3. In `cmd/state_hydrate.go`, add a private helper `unsetSkeletonMarkerOrLog(cfg hydrateConfig)` that calls `state.UnsetSkeletonMarkerForFIFO(cfg.Client, cfg.FIFO)` and on error calls `cfg.Logger.Warn(...)` with a single canonical message format.
4. Replace `cmd/state_hydrate.go:178-180` with a single call to `unsetSkeletonMarkerOrLog(cfg)`.
5. Replace `cmd/state_hydrate.go:302-305` with a single call to `unsetSkeletonMarkerOrLog(cfg)`. Remove the now-unused local `livePaneKey` recompute at lines 302-303 if both sites only used it for the unset call.
6. Verify `livePaneKey` at `cmd/state_hydrate.go:99` is still needed for other purposes; if not, remove it too.

**Acceptance Criteria**:
- `grep -n "UnsetSkeletonMarker(" cmd/state_hydrate.go` returns no direct hits — all goes through the new helper.
- `state.UnsetSkeletonMarkerForFIFO` exists and is exercised by hydrate tests.
- Both former call-site blocks are replaced with single helper calls.
- `go test ./internal/state/... ./cmd/...` passes.

**Tests**:
- Add a unit test for `state.UnsetSkeletonMarkerForFIFO` using a mock `ServerOptionWriter`: assert it derives the correct paneKey from a known FIFOPath and calls `UnsetSkeletonMarker` with that key.
- Existing hydrate tests must still pass; update WARN-format assertions if the consolidated helper's message changed.

---

## Task 6: Simplify ApplySkeletonMarkers signature (drop error + predicted-base params)
status: approved
severity: medium
sources: architecture

**Problem**: `ApplySkeletonMarkers` (`internal/restore/session.go:356,385`) is declared as `(...) error` but every code path returns nil — per-pane setSkeletonMarker failures are logged-and-swallowed (session.go:433-437), the count-mismatch path warns but doesn't error. The single caller in `internal/restore/restore.go:144` has a permanently-dead `if err := ...; err != nil` branch. Additionally, after T7-9, `predictedBase` and `predictedPaneBase` no longer drive any tmux call target — they survive only so the function can compute `predictedKey` and emit a drift WARN on mismatch (session.go:370-371). This couples a write primitive to a diagnostic concern.

**Solution**: Drop the `error` return from `ApplySkeletonMarkers` and remove the `predictedBase` / `predictedPaneBase` parameters. Move the drift-comparison + `warnOnPaneKeyDrift` loop into `restoreOne` (or a helper called from `restoreOne`).

**Outcome**: `ApplySkeletonMarkers` becomes a pure write primitive with an honest signature; the drift diagnostic lives next to the prediction it diagnoses.

**Do**:
1. In `internal/restore/session.go:356-374,419-428,433-437`: change `ApplySkeletonMarkers`' signature from `(...) error` to `(...)` (no return). Drop `predictedBase` and `predictedPaneBase` parameters; remove the body's `predictedKey` computation and drift-WARN block.
2. Remove the count-mismatch return-nil tail; fold any retained warn into the no-return body.
3. In `internal/restore/restore.go` (or a helper called from `restoreOne` near lines 137-145): after `PredictLiveIndices` returns the predicted indices and live indices are queried, compute the drift comparison and call `warnOnPaneKeyDrift` for each mismatched pane. Copy the logic verbatim from session.go:370-371 and the helper it currently calls.
4. Update the lone caller in `internal/restore/restore.go:144` to drop the `if err := ...; err != nil` branch — call `sr.ApplySkeletonMarkers(...)` as a statement.
5. Update test callers (if any) to match the new signature.

**Acceptance Criteria**:
- `ApplySkeletonMarkers`' signature has no `error` return and no `predictedBase` / `predictedPaneBase` parameters.
- `restoreOne` (or its helper) emits the drift WARN — verified by an existing or new test that fakes a base-index mismatch.
- No `if err := sr.ApplySkeletonMarkers` remains anywhere.
- `go test ./internal/restore/...` passes.

**Tests**:
- Existing tests covering ApplySkeletonMarkers' write behaviour should pass after the signature change with their assertions trimmed.
- Verify (or add) a test that asserts the drift WARN fires when predicted vs live indices differ — now targeting the `restoreOne` path, not `ApplySkeletonMarkers` directly.

---

## Task 7: Collapse isolated-socket tmux argument prefix into a helper
status: approved
severity: medium
sources: duplication

**Problem**: `internal/tmuxtest/socket.go` contains three sites that rebuild the same `[]string{"-S", socketPath, "-f", "/dev/null"}` prefix: `Socket.cmd` (lines 70-73), `socketCommander.Run` (lines 113-119), `socketCommander.RunRaw` (lines 124-130). Single-file three-place copy-paste. Future changes (adding `-2`, honouring TMUX_TMPDIR) must touch three blocks.

**Solution**: Add a small private helper `func socketArgs(socketPath string, args ...string) []string` returning `append([]string{"-S", socketPath, "-f", "/dev/null"}, args...)`. Delegate all three call sites.

**Outcome**: One place to change the isolated-socket prefix; three call sites become single-line.

**Do**:
1. In `internal/tmuxtest/socket.go`, add `func socketArgs(socketPath string, args ...string) []string` near the top of the file or next to the first user.
2. Replace the prefix construction at `Socket.cmd` (lines 70-73) with `socketArgs(s.path, args...)`.
3. Replace the prefix construction in `socketCommander.Run` (lines 113-119) with `socketArgs(c.path, args...)`.
4. Replace the prefix construction in `socketCommander.RunRaw` (lines 124-130) with `socketArgs(c.path, args...)`.
5. Verify byte-identical behaviour by running `internal/tmuxtest`'s own tests plus cross-package integration tests using the harness.

**Acceptance Criteria**:
- `grep -n '"-S"' internal/tmuxtest/socket.go` returns one hit (inside the new helper).
- All three former call sites delegate to `socketArgs`.
- `go test ./internal/tmuxtest/... ./internal/restore/... ./cmd/bootstrap/...` passes.

**Tests**:
- Existing socket-harness tests cover the call paths; no new tests required.

---

## Task 8: Extract bootstrap.Orchestrator fatal-message helper
status: approved
severity: low
sources: duplication

**Problem**: Four step sites in `cmd/bootstrap/bootstrap.go` (lines 151, 156, 161, 193) all build their fatal user message the same way: `o.fatal("Portal failed to <verb> ...: "+err.Error(), err)`. The prefix and trailing `": "+err.Error()` are mechanical wrapping; only the verb phrase varies. The spec mandates this exact format — a future step that omits the prefix or drops `err.Error()` will diverge silently.

**Solution**: Add `func (o *Orchestrator) fatalf(verb string, err error)` that builds `"Portal failed to " + verb + ": " + err.Error()` and calls the existing `o.fatal`. Replace the four call sites.

**Outcome**: Format is defined in one place; four call sites collapse to a single short line each.

**Do**:
1. In `cmd/bootstrap/bootstrap.go`, add a method `func (o *Orchestrator) fatalf(verb string, err error)` next to `o.fatal`. Body: `o.fatal("Portal failed to "+verb+": "+err.Error(), err)`.
2. Replace each of the four sites at lines 151, 156, 161, 193 with `o.fatalf("<verb>", err)`.
3. Verify byte-identical message at each call site.

**Acceptance Criteria**:
- `grep -n '"Portal failed to ' cmd/bootstrap/bootstrap.go` returns one hit (inside the new helper).
- All four former call sites delegate to `fatalf`.
- `go test ./cmd/bootstrap/...` passes.

**Tests**:
- Existing bootstrap tests cover the fatal-error path; no new tests required.

---

## Task 9: Move bootstrap noop step types into cmd/bootstrap as canonical sources
status: approved
severity: low
sources: architecture

**Problem**: `cmd/bootstrap_production.go:102-109` declares a private `noopStaleCleaner` used as a fallback when `loadHookStore` fails. The bootstrap package's integration tests (`cmd/bootstrap/phase5_integration_test.go:81-109`) independently declare four sibling types (`noopSaver`, `noopRestorer`, `noopCleaner`, `noopHooks`) — the test-side `noopCleaner` is a verbatim duplicate of the production-side `noopStaleCleaner`. With each new bootstrap step interface, this pattern multiplies.

**Solution**: Move the noop step types into `cmd/bootstrap/noop.go` as exported types: `NoOpServer`, `NoOpHooks`, `NoOpRestoringMarker`, `NoOpSaver`, `NoOpRestorer`, `NoOpStaleCleaner`. Tests across all packages get one canonical source.

**Outcome**: Single source of truth for noop step implementations; production fallback and integration tests share the same types.

**Do**:
1. Create `cmd/bootstrap/noop.go`. Declare exported zero-value types satisfying each existing `bootstrap.Step{*}` interface as no-ops: `NoOpServer`, `NoOpHooks`, `NoOpRestoringMarker`, `NoOpSaver`, `NoOpRestorer`, `NoOpStaleCleaner`. Each method returns the zero/no-error result.
2. Update `cmd/bootstrap_production.go:102-109` to import and use `bootstrap.NoOpStaleCleaner` instead of the local `noopStaleCleaner`. Delete the local declaration.
3. Update `cmd/bootstrap/phase5_integration_test.go:81-109` to use the new exported types in place of the four sibling `noopXxx` declarations. Delete the test-local declarations.
4. Audit the rest of the bootstrap package's tests for other private noop step types that can now be retired; convert them to use the canonical types.

**Acceptance Criteria**:
- `cmd/bootstrap/noop.go` exists with the six exported types.
- `cmd/bootstrap_production.go` no longer declares `noopStaleCleaner`; uses `bootstrap.NoOpStaleCleaner`.
- `cmd/bootstrap/phase5_integration_test.go` no longer declares `noopSaver`, `noopRestorer`, `noopCleaner`, `noopHooks`.
- `go build ./...` and `go test ./cmd/...` pass.

**Tests**:
- Existing tests cover the bootstrap orchestration paths; no new tests required.

---

## Task 10: Extract daemon-state read template
status: approved
severity: low
sources: duplication

**Problem**: `ReadPIDFile` (`internal/state/daemon_state.go:36-50`) and `ReadVersionFile` (`internal/state/daemon_state.go:101-110`) follow the same template: `os.ReadFile(path)` → on error return `(zero, ErrXxxAbsent)` for `fs.ErrNotExist` or wrap with a "read <name>" prefix; on success do a single trim/parse step. Adding a third daemon-state file would invite a third copy.

**Solution**: Add a small private helper `readDaemonFile(path string, absentSentinel error) ([]byte, error)` collapsing the open + ENOENT classification. Keep the per-reader trim/parse step at the call site.

**Outcome**: Both readers shrink to roughly three lines apiece; ENOENT classification has one canonical implementation.

**Do**:
1. In `internal/state/daemon_state.go`, add a private helper `func readDaemonFile(path string, absentSentinel error) ([]byte, error)`:
   - calls `os.ReadFile(path)`;
   - on `errors.Is(err, fs.ErrNotExist)` returns `(nil, absentSentinel)`;
   - on other error returns `(nil, fmt.Errorf("read %s: %w", filepath.Base(path), err))` (match existing prefix style);
   - on success returns `(data, nil)`.
2. Refactor `ReadPIDFile` (lines 36-50) to call `readDaemonFile`, then perform the existing trim + parse step on the returned bytes.
3. Refactor `ReadVersionFile` (lines 101-110) the same way.

**Acceptance Criteria**:
- `readDaemonFile` exists and is used by both readers.
- Both readers' bodies are trim/parse-only after the helper call.
- `go test ./internal/state/...` passes; absent-file behaviour and parse-error wrapping remain unchanged.

**Tests**:
- Existing daemon-state reader tests cover the absent / present / malformed paths; they should pass unchanged.
- Optionally add a unit test for `readDaemonFile` exercising the ENOENT branch and the generic-error wrapping.
