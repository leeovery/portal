# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 5)

- topic: killed-sessions-resurrect-on-restart
- cycle: 5
- total_proposed: 2

---

## Task 1: Delete shellQuoteSingle and emit bare hydrate invocation
status: approved
severity: low
sources: architecture

**Problem**: After T3-1's wrapper drop, `buildHydrateCommand` (`/Users/leeovery/Code/portal/internal/restore/session.go:426-433`) emits a bare `portal state hydrate ...` invocation with no outer `'…'` single-quoted envelope, yet still routes each value-arg through `shellQuoteSingle` (lines 438-440). `shellQuoteSingle`'s body `strings.ReplaceAll(s, "'", "'\\''")` produces the `'\''` close/escape/reopen idiom, which is only well-formed shell INSIDE an outer single-quoted envelope. With the envelope removed, the escape is a latent bug: an apostrophe-bearing session name (e.g. `it's-cool` — `sanitizeSessionName` at `/Users/leeovery/Code/portal/internal/state/panekey.go:41-58` only filters `/`, `\`, `\0`) would flow through into fifo basename + hookKey and produce unbalanced-quote tokens that tmux's internal `/bin/sh -c <command>` parses as malformed input. The doc-comment at session.go:424-425 and the white-box test sub-case at `/Users/leeovery/Code/portal/internal/restore/session_build_hydrate_test.go:30-40` both justify the call with an incoherent "defensive parity with the prior call-site contract" rationale — parity with a context (outer `'…'` envelope) that T3-1 deleted.

**Solution**: Delete `shellQuoteSingle` entirely and simplify `buildHydrateCommand` to a direct `fmt.Sprintf` with raw value interpolation. Acknowledge in the docstring that `'`-bearing inputs would break shell parsing and Portal does not currently produce them.

**Outcome**: One helper deleted, one docstring corrected, one test sub-case removed. The bare-form invocation shape is preserved for all non-pathological inputs (AC1-AC8 success cases unaffected). The contract becomes self-describing — no half-broken escape masquerading as defence-in-depth.

**Do**:
1. In `/Users/leeovery/Code/portal/internal/restore/session.go`:
   - Delete the `shellQuoteSingle` function at lines 435-440.
   - Replace `buildHydrateCommand`'s body (lines 427-432) with:
     ```go
     return fmt.Sprintf(
         "portal state hydrate --fifo %s --file %s --hook-key %s",
         fifo, file, hookKey,
     )
     ```
   - Update the docstring at lines 424-425: replace the "shellQuoteSingle still escapes ... defensive parity" sentence with a note that inputs containing `'` would break shell parsing under the bare form and that Portal's sanitization does not currently produce such inputs.
   - Remove the `"strings"` import if it becomes unused.
2. In `/Users/leeovery/Code/portal/internal/restore/session_build_hydrate_test.go`:
   - Delete the sub-test at lines 30-40 ("single-quote bearing inputs round-trip through shellQuoteSingle").
   - Keep the three remaining sub-tests (typical, empty hookKey, no sh -c envelope) unchanged.
3. Run `go build -o portal .` and `go test ./internal/restore/...` to confirm.

**Acceptance Criteria**:
- `shellQuoteSingle` no longer exists in internal/restore/session.go.
- `buildHydrateCommand` emits raw value-arg interpolation with no `shellQuoteSingle` wrap.
- The white-box test no longer contains the `single-quote bearing inputs round-trip` sub-case; the negative-assertion sub-test (`no sh -c envelope`) remains and passes.
- `grep -rn "shellQuoteSingle" --include='*.go'` returns zero hits.
- Existing `TestBuildHydrateCommand_BareForm` sub-tests for typical inputs, empty hookKey, and the sh-c negative-assertion continue to pass.
- `go build ./...` succeeds; `go test ./internal/restore/...` passes.

**Tests**:
- Existing `TestBuildHydrateCommand_BareForm` sub-tests pass (typical, empty hookKey, no sh -c envelope).
- Integration test `TestSessionRestorer_HydrateCommandFormat` continues to pass.
- No new test required — the deletion is itself a documentation correction; the negative-assertion sub-test continues to guard against wrapper re-introduction.

---

## Task 2: Migrate buildReattachOrchestrator to NewRestoreAdapter and shared OpenTestLogger helper
status: approved
severity: low
sources: duplication

**Problem**: `buildReattachOrchestrator` in `/Users/leeovery/Code/portal/cmd/reattach_integration_test.go` (lines 165-185) open-codes two boilerplate patterns that canonical helpers exist to consolidate:

1. **RestoreAdapter two-step preamble** (lines 173-184): builds `restoreInner := &restore.Orchestrator{Client, StateDir, Logger}` and wraps it in `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}` instead of the single `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` call introduced by T6-2 (cycle 3) and already adopted at four sibling integration-test sites. The constructor docstring at `/Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:97-102` names only `cmd/bootstrap_production.go` as a deliberate non-migrated site (inline-struct adapter parity); the reattach site is not on that exemption list and has no inline-struct adapter to preserve parity with (its surrounding adapter is a one-liner `&bootstrapadapter.RestoringMarker{Client: client}`). T4-3 actively rewrote the surrounding orchestrator-construction code on the same lines, so this is not a pre-existing untouched site.

2. **OpenLogger + Cleanup boilerplate** (lines 167-171): re-implements the six-line `state.OpenLogger + t.Fatalf + t.Cleanup` pattern that `openTestLogger` (`/Users/leeovery/Code/portal/cmd/bootstrap/orchestrator_builder_test.go:116-124`) exists to consolidate. The helper is currently `package bootstrap_test`-scoped and not importable from `package cmd`. Twelve sites in cmd/bootstrap_test use the helper; cmd/reattach_integration_test.go is the one cmd-package site that re-implements it inline. Same cross-package-test-helper-after-extraction pattern applied in cycle 1 (`restoretest.WaitForFileExists`) and cycle 2 (`statetest.RecordingFIFOSignaler`).

**Solution**: Apply both consolidations in one mechanical pass. Promote `openTestLogger` to an exported helper `OpenTestLogger(t, stateDir) *state.Logger` in `internal/restoretest`, then rewrite `buildReattachOrchestrator` to consume both `bootstrapadapter.NewRestoreAdapter` and the promoted helper.

**Outcome**: `buildReattachOrchestrator` shrinks from ~20 lines to ~8 lines. The `"github.com/leeovery/portal/internal/restore"` import is dropped from cmd/reattach_integration_test.go. `openTestLogger`'s in-package definition is replaced with a thin wrapper around the promoted helper (preserving call-site stability across the existing twelve invocations). The canonical helper pattern is uniformly applied at all integration-test sites that need a real logger.

**Do**:

1. Create the promoted helper:
   - Add `OpenTestLogger(t *testing.T, stateDir string) *state.Logger` to `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go` (or a new file `internal/restoretest/logger.go` if logical grouping is cleaner). Body is byte-equivalent to the existing `openTestLogger`: `state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)` → `t.Fatalf` on error → `t.Cleanup(func() { _ = logger.Close() })` → return logger.
   - Docstring should reference the consolidation rationale (matches `WaitForSkeletonMarkersCleared` / `SeedSessionsJSON` / `WaitForFileExists` precedent).

2. Update `/Users/leeovery/Code/portal/cmd/bootstrap/orchestrator_builder_test.go`:
   - Replace the body of the existing `openTestLogger` (lines 116-124) with a one-line delegate: `return restoretest.OpenTestLogger(t, stateDir)`. This preserves the twelve existing call sites without churn.
   - Add `"github.com/leeovery/portal/internal/restoretest"` import if not already present.
   - Alternative: delete `openTestLogger` entirely and update the twelve call sites to invoke `restoretest.OpenTestLogger(t, stateDir)` directly. Either path is acceptable; prefer the delegate approach if it minimizes diff.

3. Update `/Users/leeovery/Code/portal/cmd/reattach_integration_test.go` `buildReattachOrchestrator` (lines 165-185):
   - Replace lines 167-171 (the `state.OpenLogger + Cleanup` block) with `logger := restoretest.OpenTestLogger(t, stateDir)`.
   - Replace lines 173-184 (the `restoreInner` block + the `bootstrap.WithRestore(&bootstrapadapter.RestoreAdapter{Inner: restoreInner})` wrap) with:
     ```go
     return bootstrap.NewWithDefaults(
         client,
         stateDir,
         logger,
         &bootstrapadapter.RestoringMarker{Client: client},
         bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)),
     )
     ```
   - Drop the now-unused `"github.com/leeovery/portal/internal/restore"` import.
   - Add `"github.com/leeovery/portal/internal/restoretest"` import.
   - Drop `"path/filepath"` and/or `"github.com/leeovery/portal/internal/state"` imports if they become unused.

4. Run `go build -o portal .` and `go test ./cmd/... ./internal/...` to confirm.

**Acceptance Criteria**:
- `restoretest.OpenTestLogger` exists, is exported, and is invoked from at least cmd/reattach_integration_test.go.
- `cmd/bootstrap/orchestrator_builder_test.go`'s `openTestLogger` (or its replacement) continues to satisfy the twelve existing call sites — either as a thin delegate or via direct call-site replacement.
- `buildReattachOrchestrator` no longer references `restore.Orchestrator` or `bootstrapadapter.RestoreAdapter` literal-struct construction; it calls `bootstrapadapter.NewRestoreAdapter` instead.
- The `"github.com/leeovery/portal/internal/restore"` import is absent from cmd/reattach_integration_test.go.
- `buildReattachOrchestrator` body shrinks to ~8 lines.
- `go build ./...` succeeds; `go test ./cmd/... ./internal/...` passes.

**Tests**:
- All existing tests in cmd/reattach_integration_test.go continue to pass — the change is mechanical and call-site-preserving.
- All twelve `openTestLogger` invocations in cmd/bootstrap_test continue to pass.
- No new test required — both consolidations are extract-and-reuse cleanups against existing covered surfaces.
