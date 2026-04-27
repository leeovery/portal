---
status: complete
created: 2026-04-27
cycle: 2
phase: Plan Integrity Review
topic: built-in-session-resurrection
---

# Review Tracking: built-in-session-resurrection - Integrity

## Findings

### 1. Task 6-4 / 6-5 / 6-7 reference `state.ResolveDir()` / `ReadSessionsJSON()` that don't exist in the plan

**Severity**: Critical
**Plan Reference**: `phase-2-tasks.md` task 2-1 (`built-in-session-resurrection-2-1`); `phase-3-tasks.md` task 3-1 (`built-in-session-resurrection-3-1`); `phase-6-tasks.md` tasks 6-4, 6-5, 6-7 (`built-in-session-resurrection-6-4`, `-6-5`, `-6-7`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Two cross-task API names are inconsistent and would block implementation:

1. **`state.ResolveDir()` does not exist.** Task 2-1 defines `state.Dir() (string, error)` (resolve only) and `state.EnsureDir() (string, error)` (resolve + mkdir). Task 6-5's `RunE` calls `state.ResolveDir()` ("no create — status should be read-only"), and task 6-7's `purgeStateDir` resolves `dir` "via `state.ResolveDir()`". An implementer following 6-5 / 6-7 will not find the symbol — it must be `state.Dir()` per task 2-1.

2. **`ReadSessionsJSON(dir)` does not exist.** Task 3-1 defines `ReadIndex(dir string) (Index, bool, error)` — a three-return shape with an explicit `skip` flag. Task 6-4's `CollectStatus` body calls `idx, err := ReadSessionsJSON(dir)` — a two-return shape with `*Index`. The signatures are incompatible AND the function name is wrong. The body's `if err == nil && idx != nil { ... idx.Sessions ... }` shape also assumes a pointer return; task 3-1's `ReadIndex` returns a value type.

These are not stylistic — they are compile-breaking and the implementer cannot proceed without making a planning decision. Cycle 1 finding #7 (the `SaverDownError` phantom type) was the same class of cross-phase phantom-symbol issue; this finding catches the remaining instances.

**Current** (in `phase-6-tasks.md`, task 6-5, the "Do" `RunE` body):

```
RunE: func(cmd *cobra.Command, args []string) error {
    dir, err := state.ResolveDir()
    if err != nil {
        return fmt.Errorf("resolve state dir: %w", err)
    }
    now := time.Now()
    r, err := state.CollectStatus(dir, now)
    if err != nil {
        return fmt.Errorf("collect status: %w", err)
    }
    renderStatus(cmd.OutOrStdout(), r, now)
    if isUnhealthy(r, now) {
        return ErrStatusUnhealthy
    }
    return nil
},
```

**Proposed**:

```
RunE: func(cmd *cobra.Command, args []string) error {
    dir, err := state.Dir()
    if err != nil {
        return fmt.Errorf("resolve state dir: %w", err)
    }
    now := time.Now()
    r, err := state.CollectStatus(dir, now)
    if err != nil {
        return fmt.Errorf("collect status: %w", err)
    }
    renderStatus(cmd.OutOrStdout(), r, now)
    if isUnhealthy(r, now) {
        return ErrStatusUnhealthy
    }
    return nil
},
```

(Task 2-1 already defines `state.Dir()` as the no-create resolver — exactly what `status` needs. `state.EnsureDir()` is a sibling that adds mkdir; `status` should not create the dir.)

**Current** (in `phase-6-tasks.md`, task 6-7, the relevant Do bullet):

```
- After the hook-removal step, add:
    ```go
    if purge {
        if err := purgeStateDir(dir, logger); err != nil {
            errs = append(errs, fmt.Errorf("purge state dir: %w", err))
        }
    }
    ```
    where `dir` was resolved at the top of `RunE` via `state.ResolveDir()`.
```

**Proposed**:

```
- After the hook-removal step, add:
    ```go
    if purge {
        if err := purgeStateDir(dir, logger); err != nil {
            errs = append(errs, fmt.Errorf("purge state dir: %w", err))
        }
    }
    ```
    where `dir` was resolved at the top of `RunE` via `state.Dir()` (the no-create resolver from task 2-1).
```

**Current** (in `phase-6-tasks.md`, task 6-7, the Context block):

```
> Spec "Scope & Constraints → Storage Location": state is under `~/.config/portal/state/` resolved via Portal's existing `configFilePath` mechanism. The resolved path is what `--purge` targets. Env-var overrides (`PORTAL_STATE_DIR`, if defined in Phase 2) are respected automatically because `state.ResolveDir()` honours them.
```

**Proposed**:

```
> Spec "Scope & Constraints → Storage Location": state is under `~/.config/portal/state/` resolved via Portal's existing `configFilePath` mechanism. The resolved path is what `--purge` targets. Env-var overrides (`PORTAL_STATE_DIR`, defined in Phase 2 task 2-1) are respected automatically because `state.Dir()` honours them.
```

**Current** (in `phase-6-tasks.md`, task 6-4, the `CollectStatus` body):

```
      // Last save + counts from sessions.json
      idx, err := ReadSessionsJSON(dir) // Phase 3 task 3-1 reader
      if err == nil && idx != nil {
          r.HasLastSave = true
          r.LastSaveAt = idx.SavedAt
          r.SessionsCount = len(idx.Sessions)
          for _, s := range idx.Sessions {
              for _, w := range s.Windows {
                  r.PanesCount += len(w.Panes)
              }
          }
      }
```

**Proposed**:

```
      // Last save + counts from sessions.json (Phase 3 task 3-1 reader)
      idx, skip, ridxErr := ReadIndex(dir)
      if !skip && ridxErr == nil {
          r.HasLastSave = true
          r.LastSaveAt = idx.SavedAt
          r.SessionsCount = len(idx.Sessions)
          for _, s := range idx.Sessions {
              for _, w := range s.Windows {
                  r.PanesCount += len(w.Panes)
              }
          }
      }
      // skip=true (missing file or unparseable) → HasLastSave stays false; counts stay 0.
      // ridxErr is intentionally swallowed for status — task 3-1's reader returns
      // a non-nil err on corrupt/permission cases alongside skip=true, and status
      // surfaces that condition through RecentWarnings rather than failing the call.
```

**Resolution**: Fixed
**Notes**: Both fixes are purely naming/signature alignment — the design intent (read-only path resolution; tolerant index read) is unchanged.

---

### 2. Task 3-13 still references the deleted `RestoreWithMarker()` composite

**Severity**: Critical
**Plan Reference**: `phase-3-tasks.md` task 3-7 (`built-in-session-resurrection-3-7`); `phase-3-tasks.md` task 3-13 (`built-in-session-resurrection-3-13`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Cycle 1 traceability finding #2 explicitly removed `RestoreWithMarker()` from task 3-7's API: the post-fix task body says "Do NOT introduce a composite `RestoreWithMarker()` wrapper" and Acceptance Criteria asserts "No composite `RestoreWithMarker()` wrapper exists". Task 3-13 (the integration test) was not updated to match — it still calls `orchestrator.RestoreWithMarker()` in two places (steps 11 and 19 of the primary test) plus references it in the test-seam description.

An implementer landing task 3-13 will write `orchestrator.RestoreWithMarker()` and the build will fail because no such method exists — task 3-7 deliberately removed it. The integration test must instead call the two primitives `SetRestoring()` → `Restore()` → `ClearRestoring()` directly, mirroring how Phase 5's bootstrap orchestrator (task 5-2) calls them.

**Current** (in `phase-3-tasks.md`, task 3-13, the test-seam description):

```
- Test seam: a `test-only` entry point in `internal/restore` that lets the integration test invoke `Orchestrator.RestoreWithMarker()` + `state.SweepOrphanFIFOs()` against a `tmux.Client` pointed at the isolated socket. Alternative: shell out to the built binary with `TMUX_STATE_DIR=<tempdir>` and appropriate env vars — more realistic but slower. Prefer the direct-call approach for this task's integration test to keep wall-clock under 5s per test; Phase 5 adds the full binary-subprocess test.
```

**Proposed**:

```
- Test seam: the integration test invokes the orchestrator's three primitives directly — `Orchestrator.SetRestoring()` → `Orchestrator.Restore()` → `Orchestrator.ClearRestoring()` (per task 3-7's no-composite-wrapper decision) — plus `state.SweepOrphanFIFOs()`, all against a `tmux.Client` pointed at the isolated socket. Alternative: shell out to the built binary with `TMUX_STATE_DIR=<tempdir>` and appropriate env vars — more realistic but slower. Prefer the direct-call approach for this task's integration test to keep wall-clock under 5s per test; Phase 5 adds the full binary-subprocess test.
```

**Current** (in `phase-3-tasks.md`, task 3-13, primary test step 11):

```
  11. Invoke the restore path: `orchestrator.RestoreWithMarker()`.
```

**Proposed**:

```
  11. Invoke the restore path as the spec's three-step sequence: `orchestrator.SetRestoring()`, `orchestrator.Restore()`, `orchestrator.ClearRestoring()` — the same primitive sequence Phase 5 task 5-2's bootstrap calls, exposed for direct invocation by integration tests per task 3-7's no-composite-wrapper decision.
```

**Current** (in `phase-3-tasks.md`, task 3-13, primary test step 19):

```
  19. Re-run restore: `orchestrator.RestoreWithMarker()` again. Assert: zero new tmux calls beyond `list-sessions` (live-skip path). Structural state unchanged.
```

**Proposed**:

```
  19. Re-run restore: invoke `orchestrator.SetRestoring()`, `orchestrator.Restore()`, `orchestrator.ClearRestoring()` again. Assert: zero new tmux calls beyond `list-sessions` (live-skip path). Structural state unchanged.
```

**Resolution**: Fixed
**Notes**: Mechanical rename — the test's behavioural intent is unchanged; only the API surface name needs updating to match task 3-7's post-cycle-1 shape.

---

### 3. Task 5-2's bootstrap orchestrator omits the FIFO-sweep step that task 3-12 promises is wired in Phase 5

**Severity**: Critical
**Plan Reference**: `phase-5-tasks.md` task 5-2 (`built-in-session-resurrection-5-2`); `phase-3-tasks.md` task 3-12 (`built-in-session-resurrection-3-12`)
**Category**: Dependencies and Ordering / Phase Structure
**Change Type**: update-task

**Details**:
Task 3-12 builds `state.SweepOrphanFIFOs(dir, liveMarkerKeys, logger)` and explicitly notes (Context block, last paragraph): "Phase 5 wires the sweep into bootstrap after Restore completes and before `CleanStale` (step 7)." The example caller code given inside task 3-12 even shows the `fetchSkeletonMarkers` → `liveKeys` → `SweepOrphanFIFOs` invocation as something Phase 5 will land.

But Phase 5 task 5-2's orchestrator step list is `EnsureServer → RegisterPortalHooks → Restoring.Set → EnsureSaver → Restore → Restoring.Clear → CleanStale` — seven calls, no FIFO sweep. The orchestrator's interfaces (`ServerBootstrapper`, `HookRegistrar`, `RestoringMarker`, `SaverBootstrapper`, `Restorer`, `StaleCleaner`) likewise have no `FIFOSweeper` member. The acceptance criterion `"Run executes 7 step calls in the order ..."` enumerates the seven without sweep.

If Phase 5 ships as-currently-written, the FIFO sweep code from task 3-12 is dead — never invoked from bootstrap. State directories accumulate orphan FIFOs from prior runs with different paneKeys (the exact case task 3-12 was designed to handle), defeating its purpose. An implementer landing task 5-2 in isolation has no signal that they need to add the sweep.

**Current** (in `phase-5-tasks.md`, task 5-2, the "Do" interfaces list):

```
  - Interfaces (each 1-2 methods, satisfying Portal's DI style):
    - `type ServerBootstrapper interface { EnsureServer() (bool, error) }` — re-uses existing contract.
    - `type HookRegistrar interface { RegisterPortalHooks() error }` — wraps `internal/tmux/hooks_register.go` (Phase 1 task 1-7 + Phase 4 task 4-4).
    - `type RestoringMarker interface { Set() error; Clear() error }` — `@portal-restoring` server-option. Backed by `*tmux.Client.SetServerOption("@portal-restoring", "1")` and `UnsetServerOption("@portal-restoring")`.
    - `type SaverBootstrapper interface { EnsureSaver() error }` — Phase 2 task 2-5 + 2-6 (idempotent `_portal-saver` bootstrap with version-marker-driven restart).
    - `type Restorer interface { Restore() error }` — Phase 3 task 3-6 `Restore()` orchestrator.
    - `type StaleCleaner interface { CleanStale() error }` — existing `hooks.CleanStale` (Phase 4 task 4-7 removes its empty-panes guard).
```

**Proposed**:

```
  - Interfaces (each 1-2 methods, satisfying Portal's DI style):
    - `type ServerBootstrapper interface { EnsureServer() (bool, error) }` — re-uses existing contract.
    - `type HookRegistrar interface { RegisterPortalHooks() error }` — wraps `internal/tmux/hooks_register.go` (Phase 1 task 1-7 + Phase 4 task 4-4).
    - `type RestoringMarker interface { Set() error; Clear() error }` — `@portal-restoring` server-option. Backed by `*tmux.Client.SetServerOption("@portal-restoring", "1")` and `UnsetServerOption("@portal-restoring")`.
    - `type SaverBootstrapper interface { EnsureSaver() error }` — Phase 2 task 2-5 + 2-6 (idempotent `_portal-saver` bootstrap with version-marker-driven restart).
    - `type Restorer interface { Restore() error }` — Phase 3 task 3-6 `Restore()` orchestrator.
    - `type FIFOSweeper interface { SweepOrphanFIFOs() error }` — wraps Phase 3 task 3-12 `state.SweepOrphanFIFOs(dir, liveMarkerKeys, logger)`. The implementation derives `liveMarkerKeys` from a fresh `fetchSkeletonMarkers` call so the sweep sees the post-Restore marker set.
    - `type StaleCleaner interface { CleanStale() error }` — existing `hooks.CleanStale` (Phase 4 task 4-7 removes its empty-panes guard).
```

**Current** (in `phase-5-tasks.md`, task 5-2, the "Do" `Run` step list and tests):

```
    6. `if err := o.Restoring.Clear(); err != nil` → log WARN and continue. The marker stays set; the daemon will skip ticks until the next server restart (volatile server-option, self-heals). This is degraded but bounded — better than failing the whole bootstrap at the tail of otherwise-successful work. Matches task 3-7's clear-failure-is-soft contract. The clear is also wrapped in a `defer` off of step 3 for safety (so a panic between 3 and 6 still attempts the clear); the deferred and explicit calls are both no-ops on success.
    7. `if err := o.Clean.CleanStale(); err != nil` → log, continue. CleanStale failure is soft (non-critical pruning step).
    8. Return `(serverStarted, restoreErr)` — if `Restore()` errored, surface it; otherwise `nil`.
```

**Proposed**:

```
    6. `if err := o.Restoring.Clear(); err != nil` → log WARN and continue. The marker stays set; the daemon will skip ticks until the next server restart (volatile server-option, self-heals). This is degraded but bounded — better than failing the whole bootstrap at the tail of otherwise-successful work. Matches task 3-7's clear-failure-is-soft contract. The clear is also wrapped in a `defer` off of step 3 for safety (so a panic between 3 and 6 still attempts the clear); the deferred and explicit calls are both no-ops on success.
    7. `if err := o.Sweeper.SweepOrphanFIFOs(); err != nil` → log WARN and continue. Per task 3-12, sweep failures are best-effort: the next bootstrap retries. Sweep runs AFTER `Restoring.Clear` and BEFORE `CleanStale` so live skeleton-marker FIFOs from the just-completed restore are preserved (they are referenced via the post-Restore marker set).
    8. `if err := o.Clean.CleanStale(); err != nil` → log, continue. CleanStale failure is soft (non-critical pruning step).
    9. Return `(serverStarted, restoreErr)` — if `Restore()` errored, surface it; otherwise `nil`.
```

(Renumbered: the spec's 8-step "PersistentPreRunE Sequence" already groups the FIFO sweep within step 7's broader "post-restore housekeeping" — task 5-2 documents it as a distinct call inside the orchestrator. Phase 5 acceptance bullet "`CleanStale` runs in step 7" still holds because the orchestrator's step-numbering is internal to the implementation.)

**Current** (in `phase-5-tasks.md`, task 5-2, the test list):

```
  - `"it executes steps 1 through 8 in spec order"`: construct `Orchestrator` with the recorder; call `Run`; assert the recorded log equals `["EnsureServer", "RegisterPortalHooks", "Restoring.Set", "EnsureSaver", "Restore", "Restoring.Clear", "CleanStale"]`. (Seven entries — step 8 is "return", no call.)
```

**Proposed**:

```
  - `"it executes steps 1 through 8 in spec order"`: construct `Orchestrator` with the recorder; call `Run`; assert the recorded log equals `["EnsureServer", "RegisterPortalHooks", "Restoring.Set", "EnsureSaver", "Restore", "Restoring.Clear", "SweepOrphanFIFOs", "CleanStale"]`. (Eight call entries — the spec's "step 8" is the implicit return.)
```

**Current** (in `phase-5-tasks.md`, task 5-2, the corresponding Acceptance Criteria bullet):

```
- [ ] `Run` executes 7 step calls in the order `EnsureServer → RegisterPortalHooks → Restoring.Set → EnsureSaver → Restore → Restoring.Clear → CleanStale`.
```

**Proposed**:

```
- [ ] `Run` executes 8 step calls in the order `EnsureServer → RegisterPortalHooks → Restoring.Set → EnsureSaver → Restore → Restoring.Clear → SweepOrphanFIFOs → CleanStale`.
- [ ] `SweepOrphanFIFOs` runs AFTER `Restoring.Clear` and BEFORE `CleanStale` (ordering asserted via recorder).
- [ ] `SweepOrphanFIFOs` failure logs WARN and does NOT short-circuit `Run` (subsequent `CleanStale` still executes).
```

Add to the **Tests** list:

```
- `"it runs SweepOrphanFIFOs between Restoring.Clear and CleanStale"`
- `"it logs and continues when SweepOrphanFIFOs fails"`
```

**Resolution**: Fixed
**Notes**: This finding mirrors task 3-12's promise into Phase 5's orchestrator. Without the sweep wired in, task 3-12's primitive is unreachable from the bootstrap path — a slow-burn integrity bug.

---

### 4. Task 6-8 contradicts itself on `SilenceErrors` setting (declaration vs handler-level)

**Severity**: Important
**Plan Reference**: `phase-6-tasks.md` task 6-8 (`built-in-session-resurrection-6-8`)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 6-8's "Do" section pins two contradictory `SilenceErrors` decisions in adjacent bullets:

- First bullet says: "Set `rootCmd.SilenceErrors = true` and `rootCmd.SilenceUsage = true` at declaration."
- The very next bullet says: "Recommended: keep `SilenceErrors = false` (Cobra default) but override in the `main.go` handler. The `errors.As` check intercepts `FatalError` first and emits the single line; all other errors fall through to Cobra's default path. This preserves usage-error UX (Cobra prints usage + 'Error: ...')."

These are mutually exclusive: either Cobra prints errors by default (`SilenceErrors = false`, allows usage errors to render via Cobra's normal path) OR Cobra is silenced and `main.go` is the sole writer. The acceptance criterion ("Cobra usage errors (unknown command, missing required flag) still print Cobra's default output") is satisfiable only with the second approach — which contradicts the first bullet.

An implementer reading the Do section sees `rootCmd.SilenceErrors = true` first and may set it that way, breaking the usage-error path the acceptance criterion requires. The plan must pick one and remove the other.

**Current** (in `phase-6-tasks.md`, task 6-8, the relevant "Do" section):

```
- Edit `cmd/root.go` (or `main.go` — wherever `cmd.Execute()` lives):
  - Set `rootCmd.SilenceErrors = true` and `rootCmd.SilenceUsage = true` at declaration.
  - In `main.go` (or `Execute()`):
    ```go
    if err := cmd.Execute(); err != nil {
        var fatal *bootstrap.FatalError
        if errors.As(err, &fatal) {
            fmt.Fprintln(os.Stderr, fatal.UserMessage)
            os.Exit(1)
        }
        // Non-fatal error: Cobra already swallowed printing due to SilenceErrors;
        // emit minimal diagnostic.
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
    ```
  - For Cobra usage errors (invalid args, missing required flag): preserve existing Cobra behaviour via conditional unwrap — if the error is a Cobra usage error (not a `*FatalError`), print usage and exit. Implementation: check `strings.HasPrefix(err.Error(), "unknown command") || strings.HasPrefix(err.Error(), "required flag")` etc., or rely on Cobra's own classification. Simpler: let Cobra's normal path handle usage errors by NOT setting `SilenceErrors = true` globally; instead, set it only on commands whose errors are already fully rendered (status, cleanup). For bootstrap: the FatalError path at the `Execute()` handler takes priority via `errors.As` check.
    - Recommended: keep `SilenceErrors = false` (Cobra default) but override in the `main.go` handler. The `errors.As` check intercepts `FatalError` first and emits the single line; all other errors fall through to Cobra's default path. This preserves usage-error UX (Cobra prints usage + "Error: ...").
```

**Proposed**:

```
- Edit `cmd/root.go` (or `main.go` — wherever `cmd.Execute()` lives):
  - Keep `SilenceErrors = false` on `rootCmd` (Cobra default) so usage errors render through Cobra's normal path. Set `SilenceErrors = true` only on commands whose `RunE` already wrote complete output (e.g., `stateStatusCmd` per task 6-5, `stateCleanupCmd` per task 6-6) so a non-zero exit there does not double-print.
  - In `main.go` (or `Execute()`):
    ```go
    if err := cmd.Execute(); err != nil {
        var fatal *bootstrap.FatalError
        if errors.As(err, &fatal) {
            fmt.Fprintln(os.Stderr, fatal.UserMessage)
            os.Exit(1)
        }
        // Non-fatal error: Cobra has already printed usage / error per its
        // default path (SilenceErrors=false on rootCmd). Just exit non-zero.
        os.Exit(1)
    }
    ```
  - The `errors.As` check intercepts `*FatalError` first — fatal-bootstrap path emits the single user-message line and exits, bypassing Cobra's printing. All other errors (Cobra usage errors, command-specific errors) fall through to Cobra's default path that already ran inside `Execute()` — `main.go` only needs to set the exit code.
  - For per-command silencing (task 6-5 `stateStatusCmd`, task 6-6 `stateCleanupCmd`): those commands set their own `SilenceErrors = true` + `SilenceUsage = true` because their `RunE` already wrote the user-facing output and the returned error is just an exit-code carrier.
```

Add to **Acceptance Criteria** (replacing the implicit / contradictory bullets):

```
- [ ] `rootCmd.SilenceErrors` is left at the Cobra default (`false`) so usage errors render via Cobra's standard path.
- [ ] `rootCmd.SilenceUsage` is left at the Cobra default (`false`) for the same reason.
- [ ] The `errors.As(&fatalErr)` check in `main.go` runs BEFORE Cobra's own printing would surface the error, so the fatal user-message is the sole stderr signal on the fatal path. (Implementation note: `cmd.Execute()` returns the error; the handler runs after Cobra's own `SilenceErrors=false` printing. For `*FatalError`, set `SilenceErrors = true` on the originating command — typically not needed since `PersistentPreRunE` errors are returned before any RunE prints — to suppress Cobra's default error line.)
- [ ] `*FatalError`-typed errors from `PersistentPreRunE` are intercepted in the top-level handler and produce exactly one stderr line (the `UserMessage`) before `os.Exit(1)`.
- [ ] Per-command silencing (`stateStatusCmd`, `stateCleanupCmd`) is the only place `SilenceErrors`/`SilenceUsage` are flipped to true.
```

**Resolution**: Fixed
**Notes**: The two contradictory bullets are an unresolved planning negotiation. The "Recommended" alternative is the one that satisfies the existing acceptance criterion about usage errors; pinning it as the chosen approach removes the contradiction and clarifies which path the implementer should take.

---

### 5. Task 3-6 calls SessionRestorer methods that task 3-3 declares with different signatures and visibility

**Severity**: Important
**Plan Reference**: `phase-3-tasks.md` tasks 3-3 (`built-in-session-resurrection-3-3`), 3-6 (`built-in-session-resurrection-3-6`), 3-4 (`built-in-session-resurrection-3-4`), 3-5 (`built-in-session-resurrection-3-5`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-3 declares `SessionRestorer` and its methods with one shape; tasks 3-4, 3-5, and especially 3-6 (the orchestrator) reference different shapes. Concretely:

1. **Struct shape mismatch.** Task 3-3 declares `type SessionRestorer struct { Client *tmux.Client; StateDir string }` (two fields). Task 3-6's orchestrator constructs it as `&SessionRestorer{Client: o.Client, StateDir: o.StateDir, Logger: o.Logger}` (three fields). The `Logger` field is not declared by task 3-3.

2. **`Restore` arity mismatch.** Task 3-3 declares `func (r *SessionRestorer) Restore(sess state.Session) error` — one argument. The body of `Restore` reads server options internally via `predictLiveIndices`. Task 3-6 calls `sr.Restore(sess, baseIdx, paneBaseIdx)` — three arguments — and prefaces it with a separate `sr.PredictLiveIndices(sess)` call that returns the indices.

3. **`predictLiveIndices` visibility mismatch.** Task 3-3 declares `func (r *SessionRestorer) predictLiveIndices(...)` (lowercase = unexported). Task 3-6 calls `sr.PredictLiveIndices(sess)` (uppercase = exported). Same package use would only matter if the orchestrator is in `internal/restore/restore.go` — and it is — so both could co-exist if lowercase, but task 3-6 calls the uppercase form which doesn't exist.

The two designs reflect different splits of "who reads the server options" — task 3-3 hides them inside `Restore`, task 3-6 lifts them out. Either is fine in isolation but they must agree, and task 3-3 is the canonical declaration site.

An implementer landing 3-3 first then 3-6 hits a compile error on every call site in the orchestrator. The fix is to reconcile on a single shape — recommend lifting `PredictLiveIndices` out (matches the spec's discoverability ethos and what 3-6's orchestrator already implements) and adding the `Logger` field.

**Current** (in `phase-3-tasks.md`, task 3-3, the "Do" SessionRestorer declaration):

```
- Create `internal/restore/session.go` with:
  - `type SessionRestorer struct { Client *tmux.Client; StateDir string }`.
  - `func (r *SessionRestorer) Restore(sess state.Session) error` — orchestrates creation of one session.
  - `func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string` — returns the `sh -c '...'` invocation with every interpolated value POSIX-shell-safe. Implementation: use the "close-quote / escaped-quote / re-open-quote" pattern (`'` → `'\''`) on every argument before interpolation. The fifoPath and scrollbackAbs are paneKey-sanitized so they never contain `'`, but the hookKey carries the **raw** session name per spec "Save Format & Schema → Helper hook lookup under index drift" — session names can contain `'` (tmux permits it). A naive single-quote concatenation breaks the outer `sh -c '...'` body and either fails the helper launch or, worse, executes shell fragments from the session name. Concrete shape:
    ```go
    func quoteForSingleQuoted(s string) string {
        return strings.ReplaceAll(s, "'", `'\''`)
    }
    func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string {
        return fmt.Sprintf("sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
            quoteForSingleQuoted(fifoPath),
            quoteForSingleQuoted(scrollbackAbs),
            quoteForSingleQuoted(hookKey))
    }
    ```
    The `quoteForSingleQuoted` helper is shared with task 4-3's `migrate-rename` body if that task ever needs it (it does not in v1; document the helper here).
  - `func (r *SessionRestorer) predictLiveIndices(saved state.Session) (baseIdx, paneBaseIdx int, err error)` — reads `@base-index` and `@pane-base-index` via `tmux.Client.GetServerOption` (or `show-options -gv` with fallback to defaults 0, 0 if unset). Predict: window_live_index[N] = baseIdx + N (0-based N across saved windows in order); pane_live_index[M] = paneBaseIdx + M within each window.
```

**Proposed**:

```
- Create `internal/restore/session.go` with:
  - `type SessionRestorer struct { Client *tmux.Client; StateDir string; Logger *log.Logger }` — `Logger` is the standard-library logger Phase 3 uses; Phase 6 task 6-2 retrofits to `*state.Logger` as part of the cross-component logger migration.
  - `func (r *SessionRestorer) PredictLiveIndices(saved state.Session) (baseIdx, paneBaseIdx int, err error)` — exported because the orchestrator in task 3-6 calls it before invoking `Restore`. Reads `@base-index` and `@pane-base-index` via `tmux.Client.GetServerOption` (or `show-options -gv` with fallback to defaults 0, 0 if unset). Predict: window_live_index[N] = baseIdx + N (0-based N across saved windows in order); pane_live_index[M] = paneBaseIdx + M within each window.
  - `func (r *SessionRestorer) Restore(sess state.Session, baseIdx, paneBaseIdx int) error` — takes the predicted indices as arguments rather than re-reading them, so task 3-6's orchestrator can pass the same `(baseIdx, paneBaseIdx)` pair to `Restore` → `ApplyWindowGeometry` → `ApplySkeletonMarkers` without three separate option reads.
  - `func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string` — returns the `sh -c '...'` invocation with every interpolated value POSIX-shell-safe. Implementation: use the "close-quote / escaped-quote / re-open-quote" pattern (`'` → `'\''`) on every argument before interpolation. The fifoPath and scrollbackAbs are paneKey-sanitized so they never contain `'`, but the hookKey carries the **raw** session name per spec "Save Format & Schema → Helper hook lookup under index drift" — session names can contain `'` (tmux permits it). A naive single-quote concatenation breaks the outer `sh -c '...'` body and either fails the helper launch or, worse, executes shell fragments from the session name. Concrete shape:
    ```go
    func quoteForSingleQuoted(s string) string {
        return strings.ReplaceAll(s, "'", `'\''`)
    }
    func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string {
        return fmt.Sprintf("sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
            quoteForSingleQuoted(fifoPath),
            quoteForSingleQuoted(scrollbackAbs),
            quoteForSingleQuoted(hookKey))
    }
    ```
    The `quoteForSingleQuoted` helper is shared with task 4-3's `migrate-rename` body if that task ever needs it (it does not in v1; document the helper here).
```

**Current** (in `phase-3-tasks.md`, task 3-3, the "Flow of Restore" intro):

```
- Flow of `Restore(sess state.Session)`:
  1. `baseIdx, paneBaseIdx, err := r.predictLiveIndices(sess)` → on error, return wrapped.
```

**Proposed**:

```
- Flow of `Restore(sess state.Session, baseIdx, paneBaseIdx int)`:
  1. (No internal index-prediction call — the orchestrator in task 3-6 is responsible for calling `PredictLiveIndices` once per session and threading the result into `Restore`, `ApplyWindowGeometry`, and `ApplySkeletonMarkers`.)
```

(Renumber the remaining steps `2..9` → `1..8` to drop the now-removed prediction step.)

**Current** (in `phase-3-tasks.md`, task 3-3, the relevant Acceptance Criteria bullets):

```
- [ ] FIFO paths use the *live* paneKey (base-index + N / pane-base-index + M predicted from saved structural position).
- [ ] Live indices are predicted once per session from `tmux show-options -gv base-index` / `pane-base-index`, defaulting to 0 when unset.
```

**Proposed**:

```
- [ ] FIFO paths use the *live* paneKey (base-index + N / pane-base-index + M predicted from saved structural position).
- [ ] Live indices are predicted by `PredictLiveIndices` (exported, callable from the task 3-6 orchestrator) which reads `tmux show-options -gv base-index` / `pane-base-index` once per session, defaulting to 0 when unset.
- [ ] `Restore(sess, baseIdx, paneBaseIdx)` accepts the predicted indices as arguments — does NOT re-read tmux options internally — so the orchestrator can pass the same pair to `ApplyWindowGeometry` and `ApplySkeletonMarkers` without redundant reads.
- [ ] `SessionRestorer` carries a `Logger *log.Logger` field (standard library logger for Phase 3; migrated to `*state.Logger` in Phase 6 task 6-2).
```

**Resolution**: Fixed
**Notes**: This finding aligns task 3-3's declaration with task 3-6's call sites. The `(baseIdx, paneBaseIdx)` lift-out is the simpler design — the option read happens once per session in the orchestrator, not three times across `Restore` / `ApplyWindowGeometry` / `ApplySkeletonMarkers` (tasks 3-4 and 3-5 already take the pair as arguments).

---

## Notes

Cycle 2 surfaces five findings, all of which are cross-task / cross-phase API mismatches that cycle 1 either missed or that emerged from cycle-1 fixes themselves (finding #2 specifically). None are fundamental design issues — every finding is an alignment fix where one task's declaration disagrees with another task's call site. Severity:

- **Critical (3)**: Findings #1, #2, #3 — would block compilation or leave functionality unreachable.
- **Important (2)**: Findings #4, #5 — implementer would either need to make a planning decision (for #4) or guess an API shape (for #5) before proceeding.

The two intentional `[needs-info]` deferrals (task 4-4 argv source, Phase 4 acceptance #5) are not re-raised — they remain valid open questions for user resolution.
