# Analysis Cycle 2 — Proposed Tasks

## Task 1: Fix portal clean Load-error path to emit canonical Warn
status: approved
severity: medium
sources: standards

**Problem**: `cmd/clean.go:98` calls `hookStore.Load()` and discards the error with `_`. When `Load()` returns `(nil, err)` (e.g. permission-denied per `internal/hooks/store.go:36-51`), `len(nil) == 0` causes control to enter the early-exit branch at line 105, emitting `persisted=0, skipping` Debug and returning nil — bypassing `runHookStaleCleanup` and never emitting the spec-mandated `"stale-hook cleanup: hookStore.Load failed: %v"` Warn. Spec acceptance criterion 4 requires the canonical Warn on Load failure. The in-source comment at clean.go:87-97 asserts fall-through happens; actual control flow contradicts. Re-introduces the "silent at the adapter" defect-class fingerprint the spec set out to eliminate.

**Solution**: Capture the Load error at clean.go:98 and only take the early-exit when Load succeeded with zero entries. On Load failure, fall through to `runHookStaleCleanup`, which re-Loads, deterministically reproduces the failure, and emits the canonical Warn at its single declaration site.

**Outcome**: `portal clean` Load failures emit the canonical Warn fingerprint, matching spec acceptance criterion 4.

**Do**:
1. At `cmd/clean.go:98`, change `existingHooks, _ := hookStore.Load()` to `existingHooks, loadErr := hookStore.Load()`.
2. Change the early-exit at ~line 105 from `if len(existingHooks) == 0 { ... }` to `if loadErr == nil && len(existingHooks) == 0 { ... }`.
3. Verify the narrative comment (lines 87-97) now matches actual control flow; tighten if needed.
4. Add a non-integration unit test injecting a hooks-store stub that returns `(nil, errPermDenied)`; assert canonical `hookStore.Load failed` Warn fires and command returns nil.

**Acceptance Criteria**:
- When `hookStore.Load()` returns a non-nil error inside `portal clean`, the canonical `"stale-hook cleanup: hookStore.Load failed: %v"` Warn is emitted exactly once.
- When `Load()` returns `(nil, nil)` / empty slice, the existing `persisted=0, skipping` Debug early-exit fires unchanged.
- Normal cleanup path (non-empty hooks loaded) is unchanged.
- `go test ./cmd/...` passes.

**Tests**:
- New non-integration unit test injecting hooks-store stub returning `(nil, errors.New("permission denied"))`.
- Existing `persisted=0, skipping` early-exit tests continue to pass.

---

## Task 2: Promote structural-key format literal to a single exported constant
status: approved
severity: low
sources: duplication

**Problem**: After Change 1, `"#{session_name}:#{window_index}.#{pane_index}"` exists in two places: as `liveFormat` const at `cmd/bootstrap/stale_marker_cleanup.go:39` and inline at `internal/tmux/tmux.go:705`. Spec § Change 1 ("Format-string alignment") hinges on these matching exactly — drift would silently desync the two cleanup paths' interpretation of "what is a paneKey". Invariant enforced by convention only.

**Solution**: Promote the literal to a single exported constant `tmux.StructuralKeyFormat`. Both callsites reference it.

**Outcome**: Single source of truth for the structural-key format string.

**Do**:
1. Declare `const StructuralKeyFormat = "#{session_name}:#{window_index}.#{pane_index}"` in `internal/tmux/tmux.go` near `ListAllPanes`.
2. Replace the inline literal in `ListAllPanes` (tmux.go:705) with `StructuralKeyFormat`.
3. Replace `liveFormat` const in `cmd/bootstrap/stale_marker_cleanup.go:39` with a reference to `tmux.StructuralKeyFormat`; delete the local const.
4. Update format-string-pinning tests to assert against `tmux.StructuralKeyFormat`.

**Acceptance Criteria**:
- The literal `"#{session_name}:#{window_index}.#{pane_index}"` appears exactly once.
- `cmd/bootstrap/stale_marker_cleanup.go` and `internal/tmux/tmux.go` `ListAllPanes` reference `tmux.StructuralKeyFormat`.
- All existing tests pass.

**Tests**:
- Format-string-pinning tests updated to reference the constant.

---

## Task 3: Consolidate transient integration-test scaffolding (env helper + table-driven mode subtests)
status: approved
severity: low
sources: duplication

**Problem**: Two sibling integration-test files carry two layers of duplication:
1. Near-duplicate env-builder helpers (`setupTransientCleanStaleEnv` / `setupCleanTransientEnv`) — four invariant `IsolateStateForTest` + `t.Setenv` + XDG re-push steps byte-identical.
2. Mode_a / mode_b subtest bodies execute the same six-step shape (setup → seed → snapshot → install commander → invoke → assert byte-identity + log fingerprint) at both callsites, with seed maps and needle strings duplicated verbatim. Needles substring-match format strings now consolidated in `runHookStaleCleanup` — drift partially undoes cycle 1's win.

**Solution**: Extract a shared env-builder helper and a table-driven mode-subtest driver into one new shared test file.

**Outcome**: Drift between the two integration files structurally impossible.

**Do**:
1. Create `cmd/cleanstale_transient_listpanes_shared_test.go` (same `package cmd`, `//go:build integration`).
2. Move four invariant env steps into `isolateCleanStaleTestEnv(t)` returning env + stateDir.
3. Update `setupTransientCleanStaleEnv` / `setupCleanTransientEnv` to call it for shared steps; bootstrap-tail extras layered where needed.
4. Define `transientModeSpec` struct: failure-mode, entry-point invoker, post-run extra-assert closure.
5. Implement `runTransientCleanStaleModeSubtest(t, spec)` with the six-step shape; seed maps + needles declared once.
6. Replace the four mode_a / mode_b subtest bodies with three-line driver calls.

**Acceptance Criteria**:
- Four invariant Setenv/Isolate steps appear exactly once.
- Needle strings asserting `runHookStaleCleanup` format output appear exactly once.
- Mode_a and mode_b subtests in both files become short driver calls.
- `go test -tags integration ./cmd/...` passes.

**Tests**:
- All four transient mode subtests pass post-refactor.
- No new tests required — consolidation only.

---

## Task 4: Post-extraction polish on cleanStaleAdapter (logger rename, bool simplification, direct unit test)
status: approved
severity: low
sources: architecture

**Problem**: Three small post-extraction residuals:
1. `cleanStaleAdapter.Logger` (`cmd/bootstrap_production.go:72-76`) is exported while siblings `lister` / `store` are unexported — porting artefact; struct itself unexported.
2. `listErrorPolicy` (`cmd/run_hook_stale_cleanup.go:61-76,101-108`) is a two-value enum used at one branch site — a boolean in disguise.
3. `cleanStaleAdapter.CleanStale` has no direct unit coverage; composition specifics (`returnError`, nil `onRemoved`, own `Logger`) exercised only by integration tests. Regression flipping policy to `swallow` or accidentally adding non-nil `onRemoved` would slip past `go test ./...`.

**Solution**: Three mechanical cleanups bundled — rename field, replace enum with bool, add focused non-integration unit test.

**Outcome**: Internal field visibility consistent; policy parameter shape matches information content; adapter composition pinned by fast unit test.

**Do**:
1. Rename `cleanStaleAdapter.Logger` → `logger`; update struct literals (`cmd/bootstrap_production.go:32-37,164-189`) and method body.
2. Replace `policy listErrorPolicy` parameter on `runHookStaleCleanup` with `swallowListError bool`. Update branch site; delete enum + constants + docblock (replace with one-line bool semantics comment). Update callsites (`cleanStaleAdapter.CleanStale` passes `false`; `portal clean` passes `true`).
3. Add non-integration unit test for `cleanStaleAdapter.CleanStale`:
   - Construct adapter with stub lister returning non-nil list-panes error + temp hooks store + recording logger.
   - Invoke `CleanStale()`.
   - Assert: recording logger captured entry-point Debug (proves renamed `logger` field flowed); method returned non-nil err (proves `swallowListError=false` policy was passed).

**Acceptance Criteria**:
- `cleanStaleAdapter` has no exported fields.
- `listErrorPolicy` type and constants deleted; `runHookStaleCleanup` accepts `swallowListError bool`.
- A non-integration unit test for `cleanStaleAdapter.CleanStale` exists and passes under `go test ./cmd/...`.
- All existing tests (including `//go:build integration`) pass unchanged.

**Tests**:
- New non-integration adapter-composition test.
- Existing tests for steps 9/11 continue to pass.
