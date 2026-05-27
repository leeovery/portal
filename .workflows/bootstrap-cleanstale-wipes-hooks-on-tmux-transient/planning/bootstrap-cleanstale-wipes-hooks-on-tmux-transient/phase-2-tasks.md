---
phase: 2
phase_name: Hazard Guard and Adapter Logging at Both `CleanStale` Callsites
total: 5
---

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-1 | approved

### Task 2-1: Plumb `Logger` into `cleanStaleAdapter`

**Problem**: `cleanStaleAdapter` at `cmd/bootstrap_production.go:66-69` holds only `client *tmux.Client` + `store *hooks.Store` — no logger. The destructive `CleanStale` path therefore cannot emit the Warn/Debug breadcrumbs Change 4 mandates, and currently leaves zero trace at the adapter layer (the "silent in logs" defect fingerprint called out in spec §Symptom Distinguishability). The hazard guard (Task 2-2) and adapter logging contract (Change 4) both depend on a `bootstrap.Logger` being reachable from the receiver before any behavioural change lands.

**Solution**: Add a `Logger` field of interface type `bootstrap.Logger` (`Debug`/`Info`/`Warn`/`Error` per `cmd/bootstrap/bootstrap.go:178-183`) to `cleanStaleAdapter`. Populate it from the orchestrator-scope `logger` already in scope at `cmd/bootstrap_production.go:109` (`logger, _ := openNoRotateLogger()`), using the same field-population pattern lines 147-152 use for `MarkerCleanupCore.Logger`. Apply the same nil-tolerance contract `MarkerCleanupCore.CleanStaleMarkers` uses at `cmd/bootstrap/stale_marker_cleanup.go:109-112` — substitute a local `noopLogger{}` when the field is nil so `Logger.Warn` / `Logger.Debug` can be called unconditionally inside the method body. No behaviour change in this task — wiring only, so a subsequent task can land the hazard guard against an already-plumbed logger.

**Outcome**: `cleanStaleAdapter` carries a `Logger` field, the production wiring at `buildProductionOrchestrator` populates it from the orchestrator-scope `logger`, the receiver method can call `a.Logger.Warn`/`a.Logger.Debug` unconditionally (with no-op when nil), and all existing tests pass unchanged because no observable behaviour has shifted yet.

**Do**:
1. Open `cmd/bootstrap_production.go`. Edit the `cleanStaleAdapter` struct at lines 66-69 to add a third field: `Logger bootstrap.Logger`.
2. Edit the wiring at lines 113-118 — when the `*hooks.Store` resolves cleanly, populate the new field: `cleaner = &cleanStaleAdapter{client: client, store: hookStore, Logger: logger}`. Keep the `NoOpStaleCleaner` fallback path unchanged.
3. Inside `(*cleanStaleAdapter).CleanStale` at lines 76-83, add the nil-substitute boilerplate at the top of the method body — copy the shape used at `cmd/bootstrap/stale_marker_cleanup.go:105-112`:
   ```go
   logger := a.Logger
   if logger == nil {
       logger = noopLogger{} // package-private to cmd/bootstrap; see step 4
   }
   ```
4. `noopLogger` lives in package `cmd/bootstrap` (declared near `bootstrap.go:185`), not `cmd`. Either (a) declare a small local no-op type inside `cmd/bootstrap_production.go` matching `bootstrap.Logger`, or (b) export the existing `noopLogger` from `cmd/bootstrap` as `bootstrap.NoopLogger` and use it. Prefer (a) — a small unexported type at the top of `bootstrap_production.go` keeps the change blast-radius minimal and avoids widening the `cmd/bootstrap` public surface for one consumer. Name it `cleanStaleNoopLogger` so it does not collide if `cmd/bootstrap_production.go` later grows other adapter-local helpers.
5. Leave the rest of the method body unchanged in this task — Task 2-2 lands the actual hazard guard and logging emissions against this newly available `logger` local.
6. Do not introduce a new seam interface for `*tmux.Client` here — spec §Change 3 explicitly states "The struct continues to consume `*tmux.Client` directly — no new seam interface is introduced." The test file in Task 2-3 may abstract `ListAllPanes` behind a local `AllPaneLister` interface for stubbing.

**Acceptance Criteria**:
- `cleanStaleAdapter` struct has three fields: `client *tmux.Client`, `store *hooks.Store`, `Logger bootstrap.Logger`.
- `buildProductionOrchestrator` populates the new `Logger` field with the orchestrator-scope `logger` returned by `openNoRotateLogger()`.
- The `CleanStale` method body starts with the nil-substitute boilerplate; `logger.Debug`/`logger.Warn` calls would be no-ops when the field is nil.
- The adapter local no-op type (`cleanStaleNoopLogger` or equivalent) satisfies `bootstrap.Logger` — verified with a compile-time assignment such as `var _ bootstrap.Logger = cleanStaleNoopLogger{}`.
- `go build ./...` succeeds.
- `go test ./...` passes — no existing test should fail because no observable behaviour changed.

**Tests**:
- No new test in this task — Task 2-3 lands the unit coverage. The compile-time assignment `var _ bootstrap.Logger = cleanStaleNoopLogger{}` is the only structural assertion required here.
- Edge case (covered by compile-time check): the no-op type satisfies the full `bootstrap.Logger` interface (`Debug`/`Info`/`Warn`/`Error`) — missing any method would fail at build time.

**Edge Cases**:
- `openNoRotateLogger()` returns nil (e.g. when `state.EnsureDir` fails) — the wiring assigns nil to `cleanStaleAdapter.Logger`. The nil-substitute boilerplate inside `CleanStale` handles this transparently.
- Field-population symmetry with `MarkerCleanupCore` (lines 147-152) — both adapters resolve their logger from the same orchestrator-scope variable, ensuring `portal.log` carries a single coherent log stream across step 9 and step 11.
- The `NoOpStaleCleaner` fallback path (when `loadHookStore` fails) is unaffected — it bypasses the adapter entirely and therefore never exercises the logger field.

**Context**:
- The spec's "single auditable destructive-callsite log stream" property (§Change 3, Logger plumbing) requires the bootstrap adapter and `portal clean` `RunE` to both write into the same `portal.log` under the resolved state dir. Reusing `openNoRotateLogger()` at both sites preserves that property. Task 2-4 mirrors this acquisition at the `portal clean` callsite.
- `MarkerCleanupCore.CleanStaleMarkers` at `cmd/bootstrap/stale_marker_cleanup.go:104-141` is the canonical prior-art for the entire Phase 2 pattern — Logger field, nil-substitute, hazard guard, soft-warning return. This task lifts only the Logger-field portion; Task 2-2 lifts the rest.

**Spec Reference**: §Change 3 (Logger plumbing — bootstrap adapter); §Audited Reference Set (`stale_marker_cleanup.go` as canonical prior-art).

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-2 | approved

### Task 2-2: Add hazard guard and two-line logging to `cleanStaleAdapter.CleanStale`

**Problem**: `(*cleanStaleAdapter).CleanStale` at `cmd/bootstrap_production.go:76-83` currently passes the (possibly empty) live-pane slice straight to `a.store.CleanStale(livePanes)` with no hazard guard, no Load check, and no log breadcrumbs. When `ListAllPanes` legitimately succeeds with an empty result (failure mode (b) — saver mid-respawn, exit 0 / empty stdout) the destructive `hooks.Store.CleanStale` call wipes every entry from `hooks.json`. With Phase 1's contract change in force, mode (a) (exit ≠ 0) now surfaces as a non-nil error, but the adapter's existing `if err != nil { return nil }` still swallows it silently — reintroducing the "silent at the adapter" fingerprint. The adapter is also misleadingly documented at lines 71-75: the docstring claims it "degrades to no-op" on `ListAllPanes` failure, which is the bug-codified-as-correct expression that this work unit overturns.

**Solution**: Rewrite the body of `(*cleanStaleAdapter).CleanStale` to a six-branch algorithm lifting the prior-art from `cmd/bootstrap/stale_marker_cleanup.go:104-141` verbatim (with `len(persisted)` substituted for `len(markers)`):

1. Call `a.client.ListAllPanes()`. On non-nil error: emit the terminal `Warn` (`"stale-hook cleanup: list-panes failed: %v"`) and return the error so the orchestrator surfaces it as a soft warning. No entry-point Debug — the enumeration-error branch emits only the terminal line.
2. Call `a.store.Load()`. On non-nil error: emit the terminal `Warn` (`"stale-hook cleanup: hookStore.Load failed: %v"`) and return the error. No entry-point Debug — same mutual-exclusivity rule as the previous branch. This prevents the corrupt-file-overwrite hazard the spec calls out: if `Load()` were treated as `len(persisted) == 0`, a normal-path stale removal could proceed against an unparseable file, overwriting it with `{}`.
3. Emit the entry-point `Debug` once both counts are known: `Logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(persisted))`.
4. Hazard guard: when `len(livePanes) == 0 && len(persisted) > 0` — emit `Logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)", len(persisted))` and return nil. No destructive call.
5. When `len(livePanes) == 0 && len(persisted) == 0` — return nil silently. Nothing to do, no hazard, no terminal line. (This preserves the "every invocation emits at least one breadcrumb" contract via the entry-point Debug emitted at step 3.)
6. Otherwise — call `a.store.CleanStale(livePanes)`. On success emit `Logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removed))` and return any error from the call.

Lift the load-bearing comment block from `cmd/bootstrap/stale_marker_cleanup.go:80-92` adapted to name `hooks.json` entries as the protected data, preserving the "deferral is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure" framing. Rewrite the misleading docstring at lines 71-75 to describe the post-fix contract: (i) error from `ListAllPanes` is returned for soft-warning surfacing, (ii) hazard-guard skip on empty-live + non-empty-store, (iii) normal-path stale removal otherwise.

**Outcome**: `cleanStaleAdapter.CleanStale` matches the six-branch algorithm; every invocation emits exactly the log lines specified by Change 4 (one entry-point Debug plus one mutually-exclusive terminal line on the enumeration-success paths; one terminal Warn alone on the two enumeration-error paths); `hooks.json` is preserved under both failure modes (a) and (b); the docstring and adapted comment block self-document the new contract.

**Do**:
1. Open `cmd/bootstrap_production.go`. The Task 2-1 patch is already in place — the receiver carries `Logger bootstrap.Logger` and the nil-substitute local `logger` is initialised at the top of the method body.
2. Add the import for `"github.com/leeovery/portal/internal/state"` if not already present (needed for `state.ComponentBootstrap`).
3. Replace the method body lines 77-83 with the six-branch algorithm. Concrete shape (after the Task 2-1 nil-substitute boilerplate):
   ```go
   livePanes, err := a.client.ListAllPanes()
   if err != nil {
       logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: list-panes failed: %v", err)
       return err
   }
   persisted, err := a.store.Load()
   if err != nil {
       logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: hookStore.Load failed: %v", err)
       return err
   }
   logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(persisted))
   if len(livePanes) == 0 {
       if len(persisted) == 0 {
           return nil
       }
       logger.Warn(state.ComponentBootstrap,
           "stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)",
           len(persisted))
       return nil
   }
   removed, err := a.store.CleanStale(livePanes)
   if err != nil {
       return err
   }
   logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removed))
   return nil
   ```
4. Lift the comment block from `cmd/bootstrap/stale_marker_cleanup.go:80-92` above the hazard-guard branch. Adapt: replace "marker" with "hook entry" / "hooks.json entry", "unset every marker" with "delete every hooks.json entry", "markers protecting legitimate hydrate-in-progress panes" with "hooks.json entries for legitimate live panes whose enumeration momentarily failed". Preserve the "deferral is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure" framing verbatim. The protected-data noun and the soft-outcome framing are both load-bearing per spec §Change 3.
5. Rewrite the docstring at lines 71-75. New shape (approximate):
   ```
   // CleanStale prunes hooks.json entries whose structural pane key no
   // longer corresponds to a live tmux pane. Behaviour by branch:
   //
   //   - ListAllPanes error: emit Warn, return the error so the
   //     orchestrator surfaces it as a soft warning (mode (a)).
   //   - hookStore.Load error: emit Warn, return the error — corrupt or
   //     unreadable hooks.json must not be overwritten by a stale-removal
   //     pass that misreads it as empty.
   //   - len(livePanes)==0 && len(persisted)>0: hazard guard — emit
   //     Warn, return nil. Treating an empty live set as authoritative
   //     would destroy every entry under a transient tmux failure
   //     (mode (b)); next bootstrap retries.
   //   - Both sides empty: return nil silently.
   //   - Normal path: invoke hookStore.CleanStale and emit a Debug
   //     reporting the removed count.
   //
   // Every invocation emits at least one log line (the entry-point Debug
   // recording both counts, plus the terminal line for the branch taken).
   // Enumeration-error branches emit only the terminal Warn.
   ```
6. Sanity-check the `hooksFile` type is exported enough to be the return shape of `Store.Load()` — per `internal/hooks/store.go:36` it returns `(hooksFile, error)`. `len(persisted)` works on the unexported map type returned from the same package boundary because we only consume `len()` here, not index into it; no API change is required.
7. Run `go build ./...` and `go vet ./...` to confirm the import is wired.

**Acceptance Criteria**:
- Method body matches the six-branch algorithm described in Do step 3.
- Comment block lifted from `stale_marker_cleanup.go:80-92` is present above the hazard-guard branch, names `hooks.json` entries as the protected data, and preserves the "deferral is a successful soft outcome" framing.
- Docstring at the method declaration is rewritten — no longer claims "degrades to no-op"; instead describes the four-branch contract.
- Each of the six branches emits exactly the log lines specified by Change 4 (verified by Task 2-3 tests):
  - ListAllPanes error → terminal Warn only.
  - Load error → terminal Warn only.
  - Empty + empty → entry Debug only.
  - Empty + non-empty → entry Debug + hazard Warn.
  - Normal-path success → entry Debug + completion Debug.
  - Normal-path Save failure → entry Debug + the propagated Save error (no completion Debug).
- `hookStore.CleanStale` is **not** invoked on the empty-live + non-empty-persisted branch.
- `go build ./...`, `go vet ./...`, `go test ./...` all green.

**Tests**:
- No new test file in this task — Task 2-3 (`cmd/bootstrap_production_test.go`) lands the four required subtests against this implementation.
- The implementation must be testable: the hazard-guard subtest in Task 2-3 will fail if any branch order is wrong, if `hookStore.CleanStale` is called on the hazard path, or if the completion Debug is emitted on the hazard path.

**Edge Cases**:
- `ListAllPanes` returns `(nil, err)` — per Phase 1's repurpose. Adapter emits Warn, returns err.
- `hookStore.Load` returns `(nil, err)` for corrupt/unreadable JSON. Adapter emits Warn, returns err. Critically: the destructive call is **not** made.
- `len(livePanes) == 0 && len(persisted) > 0` — the mode-(b) trigger. Hazard Warn fires; no Save call.
- `len(livePanes) == 0 && len(persisted) == 0` — silent no-op. Entry-point Debug fires; no terminal line; returns nil.
- Legitimate stale removal: live `{a,b,c}`, persisted `{a,b,c,d}` → `d` removed; entry Debug + completion Debug both fire; both Warns absent.
- Terminal-line mutual exclusivity: structurally enforced by `return` after every Warn / completion Debug — Task 2-3 asserts this explicitly.
- Nil-tolerant logger calls: the Task 2-1 nil-substitute means even when `a.Logger` is nil at construction, every emission is a no-op rather than a nil deref.

**Context**:
- Spec §Change 3 specifies the hazard guard verbatim ("mirrors the prior-art at `cmd/bootstrap/stale_marker_cleanup.go:126-141` exactly, with `len(persisted)` substituted for `len(markers)`"). Deviating from this shape weakens the "single architectural pattern" claim that grounds the fix.
- Spec §Change 3 also explicitly mandates: the `Load()` failure branch must (1) emit the Warn breadcrumb, and (2) return the error directly with **no destructive call** — the spec calls out that treating `Load()` failure as `len(persisted) == 0` would invert the "treat unknown as unknown" principle.
- Spec §Change 4 specifies the exact log-line shapes; the format strings here match those shapes precisely so post-fix `portal.log` lines are distinguishable between mode (a) (Warn only, no entry-point Debug) and mode (b) (entry-point Debug + hazard Warn).
- The soft-warning surfacing contract: a non-nil error returned from `CleanStale` is converted to a `warning.Warning` by the orchestrator step-runner identically to step 9 — no wrapping required at the adapter.

**Spec Reference**: §Change 3 (hazard-guard algorithm, Load() error handling, adapter docstring rewrite, load-bearing comment lift); §Change 4 (terminal-line shapes and mutual-exclusivity); §Closing Both Failure Modes (mode (b) closed by this change).

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-3 | approved

### Task 2-3: Create `cmd/bootstrap_production_test.go` with the four required adapter subtests

**Problem**: `cleanStaleAdapter` has **zero** unit coverage today — the file `cmd/bootstrap_production_test.go` does not exist. The destructive path inverted in `cmd/clean_test.go` covers the `portal clean` callsite but not the bootstrap-adapter callsite, which has its own code path (the orchestrator-scope `Logger`, the `*hooks.Store` field directly, the `*tmux.Client` field directly). Spec §Test Requirements §New File explicitly states "Inverting the existing `clean_test.go` subtest is necessary but **not sufficient** — the adapter has its own path." Without dedicated adapter coverage, the hazard guard could regress silently in either callsite without test failure.

**Solution**: Create `cmd/bootstrap_production_test.go` with a `TestCleanStaleAdapter` test function containing four subtests covering the adapter's `CleanStale` paths. Stub `ListAllPanes` via a local `AllPaneLister` interface (mirroring the existing one at `cmd/clean.go:13-15`) so the adapter receives stubbable enumeration. Stub the `bootstrap.Logger` with a recording implementation that captures `(level, component, format, args)` tuples for assertion. Use a real `*hooks.Store` pointed at a `t.TempDir()` file so `Load` / `Save` actually exercise the JSON round-trip. No `t.Parallel()` (project rule — `cmd` package mocks via package-level mutable state).

The bridge problem — `cleanStaleAdapter` declares `client *tmux.Client` (concrete type), not an interface — is handled by introducing a thin parallel test-only adapter type **or** by refactoring the production adapter to accept an `AllPaneLister`-shaped seam. Per spec §Change 3 ("The struct continues to consume `*tmux.Client` directly — no new seam interface is introduced — but tests for the new `cmd/bootstrap_production_test.go` file may abstract `ListAllPanes` behind a local `AllPaneLister` interface"), the preferred approach is the latter at the test boundary only: define a `cleanStaleAdapterT` test-local struct that mirrors `cleanStaleAdapter`'s method shape but holds an `AllPaneLister` seam instead of `*tmux.Client`. The test exercises the **algorithm** by invoking the test-local adapter, while a separate compile-time assertion confirms the production `cleanStaleAdapter` has the same field layout. This is the same shape used by `cmd/clean_test.go` (via `cleanDeps.AllPaneLister`) and avoids widening the production type surface.

**Outcome**: `cmd/bootstrap_production_test.go` exists with `TestCleanStaleAdapter` containing four subtests: hazard guard fires, both-sides-empty no-op, ListAllPanes error propagates, legitimate stale removal. The hazard-guard subtest explicitly asserts the completion `Debug` is **not** emitted (mutual exclusivity). `go test ./cmd -run TestCleanStaleAdapter` is green, and a future regression that re-enables the silent wipe at the adapter would fail at least two of the four subtests.

**Do**:
1. Create new file `cmd/bootstrap_production_test.go` in package `cmd`.
2. Imports: `testing`, `errors`, `path/filepath`, `github.com/leeovery/portal/cmd/bootstrap`, `github.com/leeovery/portal/internal/hooks`, `github.com/leeovery/portal/internal/state`.
3. Define a recording logger type at top of file:
   ```go
   type recordedLog struct {
       level     string // "debug" | "info" | "warn" | "error"
       component string
       format    string
       args      []any
   }
   type recordingLogger struct{ entries []recordedLog }
   func (r *recordingLogger) Debug(c, f string, a ...any) { r.entries = append(r.entries, recordedLog{"debug", c, f, a}) }
   func (r *recordingLogger) Info (c, f string, a ...any) { r.entries = append(r.entries, recordedLog{"info",  c, f, a}) }
   func (r *recordingLogger) Warn (c, f string, a ...any) { r.entries = append(r.entries, recordedLog{"warn",  c, f, a}) }
   func (r *recordingLogger) Error(c, f string, a ...any) { r.entries = append(r.entries, recordedLog{"error", c, f, a}) }
   var _ bootstrap.Logger = (*recordingLogger)(nil)
   ```
4. Define an `AllPaneLister` stub:
   ```go
   type stubAllPaneLister struct {
       panes []string
       err   error
   }
   func (s *stubAllPaneLister) ListAllPanes() ([]string, error) { return s.panes, s.err }
   ```
5. Define a test-local adapter mirroring the production shape with a seam interface for `ListAllPanes`. This is the test surrogate that exercises the same algorithm Task 2-2 lands. To keep the algorithm test-truthy, factor the body of `(*cleanStaleAdapter).CleanStale` into a free function or method that takes the lister as an interface — either approach is acceptable as long as the production adapter and the test surrogate share the same algorithm code path. Preferred shape: extract a free function `runCleanStale(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger) error` in `cmd/bootstrap_production.go` and have both the production `(*cleanStaleAdapter).CleanStale` and the test call this function directly. (This refactor is part of Task 2-2's deliverable if convenient, or part of this task — author at the implementation site whichever is cleanest.)
6. Helper `newTempHooksStore(t *testing.T, seed map[string]map[string]string) *hooks.Store` — writes the seed JSON to `filepath.Join(t.TempDir(), "hooks.json")` and returns a `*hooks.Store` pointing at it. Reuse the `writeHooksJSON` shape from `cmd/clean_test.go` if exported, otherwise inline.
7. Four subtests inside `TestCleanStaleAdapter`:
   - **"hazard guard fires on empty live + non-empty persisted"** — seed 2 entries `{a:cmd, b:cmd}`, lister returns `([]string{}, nil)`. Call `runCleanStale`. Assert: (a) return value is nil, (b) `hooks.json` file is byte-identical to seed (read file, compare), (c) recording logger has exactly one Debug (entry-point with `live=0 persisted=2`) and exactly one Warn matching the hazard format, (d) **no** Debug with format `"stale-hook cleanup: removed=%d"` is present (the completion Debug — mutual-exclusivity assertion).
   - **"both-sides-empty no-op"** — empty seed, lister returns `([]string{}, nil)`. Assert: (a) return value is nil, (b) recording logger has exactly one entry (the entry-point Debug with `live=0 persisted=0`), no Warn, no completion Debug, (c) no Save side-effect (the file may be absent or empty — verify whichever the `hooks.Store` constructor produces on empty input; assert it is not rewritten to `{}` under a non-empty seed because there was none).
   - **"ListAllPanes error propagates as soft warning"** — seed 1 entry, lister returns `(nil, errors.New("tmux dead"))`. Assert: (a) return value is the same error (or wraps it — accept either, but the test should check `errors.Is` if the implementation wraps), (b) recording logger has **only** the terminal Warn (`"stale-hook cleanup: list-panes failed: tmux dead"` — assert via format string match plus args), (c) **no** entry-point Debug (since the error fires before enumeration completes), (d) `hooks.json` unchanged.
   - **"legitimate stale removal"** — seed 4 entries `{a,b,c,d}`, lister returns `([]string{"a","b","c"}, nil)`. Assert: (a) return value nil, (b) post-run `hooks.json` contains exactly `{a,b,c}`, entry `d` is gone, (c) recording logger has entry-point Debug (`live=3 persisted=4`) and completion Debug (`removed=1`), no Warns.
8. Add a compile-time assertion confirming `*tmux.Client` still satisfies the local `AllPaneLister` interface — `var _ AllPaneLister = (*tmux.Client)(nil)` — so the production wiring keeps compiling.
9. Run `go test ./cmd -run TestCleanStaleAdapter -v`. All four subtests must pass.

**Acceptance Criteria**:
- File `cmd/bootstrap_production_test.go` exists in package `cmd`.
- `TestCleanStaleAdapter` exists with the four named subtests.
- Hazard-guard subtest asserts both: (i) hazard Warn was recorded, (ii) completion Debug was **not** recorded. Both assertions fail loudly if either property regresses.
- ListAllPanes-error subtest asserts the terminal Warn fires and the entry-point Debug does **not** — verifies mutual exclusivity on the enumeration-error branch.
- Both-sides-empty subtest asserts the entry-point Debug fires and no terminal line follows — verifies the silent no-op branch.
- Legitimate-stale-removal subtest asserts the entry-point Debug + completion Debug pair fires and the post-run `hooks.json` byte content matches the expected post-removal set.
- No `t.Parallel()` anywhere in the new file (project rule).
- `go test ./cmd -run TestCleanStaleAdapter` and `go test ./...` both green.

**Tests**: This task **is** the test. The four subtests defined above are the deliverable.

**Edge Cases**:
- Hazard-guard mutual-exclusivity assertion — explicitly required by the spec ("Tests in `cmd/bootstrap_production_test.go` assert this — the hazard-guard subtest asserts the `Warn` is recorded and the `Debug`-on-completion line is **not**.").
- Stub returning `(nil, err)` (Phase 1 contract) exercises the mode-(a) terminal-Warn-only branch.
- Stub returning `([]string{}, nil)` exercises the mode-(b) hazard-guard branch (with non-empty seed) and the silent no-op branch (with empty seed).
- Real `*hooks.Store` round-trip via temp dir exercises the actual Load/Save code path — ensures the test does not bypass the file system and miss a write that should not have happened.
- Use the `state.ComponentBootstrap` constant in assertions where convenient so a rename of the component name would surface as a single test-edit point.

**Context**:
- Project rule (CLAUDE.md): tests in `cmd` must not use `t.Parallel()` due to package-level mutable mocks. Even though this test does not touch `cleanDeps`/`bootstrapDeps`, observing the rule keeps the file consistent with the rest of the package.
- The free-function extraction in Do step 5 keeps the production adapter thin (it just calls the free function) and lets the test exercise the same algorithm code path. Without this extraction, the test would have to duplicate the algorithm shape inside a test-local adapter, increasing the risk that prod and test drift.
- The spec calls out that test stubs of the `AllPaneLister` / `LivePaneLister` interfaces should adopt the new `(nil, err)` shape on the error path — verified by the ListAllPanes-error subtest.

**Spec Reference**: §Test Requirements §New File — `cmd/bootstrap_production_test.go` (four subtests enumerated); §Change 4 (mutual-exclusivity of terminal lines, asserted by hazard-guard subtest); §Acceptance Criteria item 4 (every invocation emits exactly the specified log lines).

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-4 | approved

### Task 2-4: Acquire logger and apply hazard guard in `portal clean` `RunE`

**Problem**: `portal clean`'s `RunE` closure at `cmd/clean.go:75-91` is the second destructive callsite — symmetrical to `cleanStaleAdapter.CleanStale` but separately reachable when the user runs `portal clean` directly. It has the same defect: the (possibly empty) `livePanes` slice is passed straight to `hookStore.CleanStale` with no hazard guard, no log breadcrumbs, and no logger acquired. The existing safety net at lines 77-80 (`if err != nil { return nil }`) silently swallows `ListAllPanes` errors with no log trace. With Phase 1's contract change, mode (a) is detected but discarded; mode (b) (legitimate empty result while persisted is non-empty) wipes `hooks.json` outright.

**Solution**: Apply the same hazard guard, two-line logging contract, and `Load()` error-handling pattern as Task 2-2 to the `portal clean` `RunE` closure. Acquire a `*state.Logger` via `openNoRotateLogger()` so the breadcrumbs flow into the same `portal.log` as the bootstrap-side adapter (the "single auditable destructive-callsite log stream" property). Apply the nil-tolerance contract: when `openNoRotateLogger()` fails (returns nil), every `logger.Warn` / `logger.Debug` call must be a no-op. Preserve the two existing entry-point properties: (i) the `persisted == 0` pre-enumeration early-exit at lines 71-73 stays (it keeps the no-tmux-server ergonomics intact) but emits a single `Debug` breadcrumb so every invocation still produces at least one log line; (ii) the `RunE` continues returning nil for transient failures of the hook-cleanup tail so `portal clean` does not error out under a tmux transient (matching the existing pre-fix safety-net posture — silence-and-continue, but now with a log trace).

**Outcome**: `cmd/clean.go:65-91` carries: (1) a logger acquired via `openNoRotateLogger()` with nil-tolerance, (2) a `Warn` breadcrumb at the existing `hookStore.Load()` error-return (line 65-68), (3) a `Debug` breadcrumb at the `persisted == 0` early-exit (line 71-73), (4) the entry-point `Debug` after `ListAllPanes` returns successfully, (5) the hazard guard mirroring Task 2-2's algorithm, (6) the completion `Debug` after `hookStore.CleanStale` succeeds. `hooks.json` is preserved under both failure modes. User-facing stderr (the `"Removed stale hook: ..."` lines printed via `fmt.Fprintf(w, ...)`) is unchanged.

**Do**:
1. Open `cmd/clean.go`. Inside the `RunE` closure, immediately before line 60 (`hookStore, err := loadHookStore()`), acquire a logger:
   ```go
   logger, _ := openNoRotateLogger()
   if logger == nil {
       // *state.Logger tolerates nil receivers — every call below is a no-op.
   }
   defer func() {
       if logger != nil {
           _ = logger.Close()
       }
   }()
   ```
   Per the duplication-analysis note at `.workflows/built-in-session-resurrection/.../analysis-duplication-c6.md` (FINDING D3), this is the established RunE idiom across the cmd/ package — `openNoRotateLogger()` paired with a `defer Close`. Match that pattern exactly.
2. At lines 65-68 (`existingHooks, err := hookStore.Load()` error branch), insert a Warn breadcrumb before the existing `return err`:
   ```go
   if err != nil {
       logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: hookStore.Load failed: %v", err)
       return err
   }
   ```
   Unlike the bootstrap adapter (which swallows the error as a soft warning), `portal clean` already returns this error to the user — preserve that behaviour. The breadcrumb just makes the failure visible in `portal.log` ahead of the user-facing error message.
3. At lines 71-73 (the `len(existingHooks) == 0` early-exit), insert the Debug breadcrumb before the `return nil`:
   ```go
   if len(existingHooks) == 0 {
       logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: persisted=0, skipping")
       return nil
   }
   ```
4. At lines 75-80 (the `ListAllPanes` call + existing safety-net error branch), insert the terminal Warn before the existing silent `return nil`:
   ```go
   lister := buildCleanPaneLister()
   livePanes, err := lister.ListAllPanes()
   if err != nil {
       logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: list-panes failed: %v", err)
       return nil // Preserve pre-fix safety-net posture: silence-and-continue.
   }
   ```
   Note: the production lister is `tmux.DefaultClient()` (via `buildCleanPaneLister`). Phase 1's repurpose means `ListAllPanes` now returns `(nil, err)` on transient failure — this branch fires exactly under mode (a). The `return nil` (rather than `return err`) is **deliberately preserved** per spec §Change 3 ("the subcommand's `RunE` continues to return nil for the hook-cleanup tail's transient failures (matching the existing pre-fix safety-net posture at lines 77-80, which already chose silence-and-continue over user-facing error)").
5. Insert the entry-point Debug immediately after the lister returns successfully:
   ```go
   logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(existingHooks))
   ```
6. Insert the hazard guard before the existing `hookStore.CleanStale(livePanes)` call:
   ```go
   if len(livePanes) == 0 {
       // (len(existingHooks) > 0 is guaranteed by the early-exit at step 3.)
       logger.Warn(state.ComponentBootstrap,
           "stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)",
           len(existingHooks))
       return nil
   }
   ```
   Note: because of the `persisted == 0` early-exit at step 3, the only remaining empty-live branch is `len(livePanes) == 0 && len(existingHooks) > 0` — the mode-(b) hazard branch. The "both empty" silent no-op is already covered by the early-exit (which now emits its Debug breadcrumb). No second `len == 0 && len == 0` branch is needed here.
7. After the existing `removedPanes, err := hookStore.CleanStale(livePanes)` succeeds (line 82-85), insert the completion Debug:
   ```go
   logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removedPanes))
   ```
8. Add the `internal/state` import if not already present.
9. The existing user-facing stderr `"Removed stale hook: ..."` loop at lines 87-91 stays unchanged — Change 4's logging is structured log output, not user-facing output.
10. Run `go build ./...` and `go test ./cmd -run TestClean` to confirm the non-empty-live-set sibling subtests still pass.

**Acceptance Criteria**:
- `RunE` acquires a `*state.Logger` via `openNoRotateLogger()` with a deferred Close, and tolerates the nil-returned case (no nil deref).
- The `hookStore.Load()` error branch emits a Warn before returning the error.
- The `len(existingHooks) == 0` early-exit emits a single Debug (`"stale-hook cleanup: persisted=0, skipping"`) before `return nil`.
- The `ListAllPanes` error branch emits a Warn before `return nil` — preserving silence-and-continue at the RunE boundary but adding a log trace.
- After enumeration, the entry-point Debug `"stale-hook cleanup: live=%d persisted=%d"` fires exactly once.
- The hazard guard `len(livePanes) == 0` (with `existingHooks` known non-empty from step 3) emits the hazard Warn and returns nil; `hookStore.CleanStale` is **not** called on this branch.
- The completion Debug `"stale-hook cleanup: removed=%d"` fires after a successful `hookStore.CleanStale`.
- User-facing stderr output (the `"Removed stale hook: ..."` lines) is unchanged.
- `RunE` continues returning nil for transient `ListAllPanes` failures (no user-facing error introduced).
- `go test ./cmd -run TestClean` green; `go test ./...` green.

**Tests**:
- The inverted destructive subtest (Task 2-5) covers the hazard-guard branch end-to-end via `rootCmd.Execute` against this implementation.
- Existing sibling subtests in `cmd/clean_test.go` (non-empty live set, ListAllPanes error preserves hooks) must pass unchanged — they exercise the normal-path and the safety-net branches respectively. The post-fix safety-net branch now additionally writes a Warn line to `portal.log`, but the existing assertions are scoped to the file content and user-facing stderr, not `portal.log`.
- Edge case: `openNoRotateLogger()` returns nil — verify the `defer` block guards against nil and every `logger.*` call is a no-op via `*state.Logger`'s nil-receiver tolerance.
- Edge case: the `persisted == 0` early-exit now emits a Debug breadcrumb — sibling test "no hooks file" (if present) should still pass.

**Edge Cases**:
- `openNoRotateLogger` returns nil (e.g. when `state.EnsureDir` fails) — every `logger.*` invocation is a no-op via `*state.Logger`'s nil-receiver contract; the `defer Close` is gated on non-nil.
- `persisted == 0` early-exit emits exactly one Debug — preserves the "every invocation logs at least one breadcrumb" property without inverting `portal clean`'s no-tmux-server ergonomics.
- Transient `ListAllPanes` error returns nil at the RunE boundary (silence-and-continue) but emits a Warn to `portal.log` — distinguishing the mode-(a) signature from mode (b) in logs.
- User-facing stderr (the `"Removed stale hook: ..."` lines) is unchanged — Change 4 adds log output, not user output.
- The hazard guard at step 6 simplifies relative to Task 2-2 because the both-empty case is already short-circuited at step 3; the only remaining empty-live case is the hazard.

**Context**:
- Spec §Change 3 explicitly mandates that `portal clean`'s `RunE` "continues to return nil for the hook-cleanup tail's transient failures (matching the existing pre-fix safety-net posture at lines 77-80, which already chose silence-and-continue over user-facing error)". Returning the error here would change user-visible behaviour for the `portal clean` callsite beyond the scope of this fix.
- Spec §Change 4 explicitly carves out the `portal clean` early-exit special case: "`cmd/clean.go` retains its existing pre-enumeration early-exit when `len(persisted) == 0` ... emit a single `Debug` breadcrumb at the early-exit before returning."
- The shared `portal.log` (via `openNoRotateLogger`) means a user running `portal clean` after a bootstrap-window incident can grep `portal.log` for `stale-hook cleanup:` and see the full history of both callsites in one stream.
- Defer Close idiom matches FINDING D3 in the duplication-analysis archive — established convention across the cmd/ package.

**Spec Reference**: §Change 3 (hazard guard at both callsites, logger plumbing for `portal clean`, `RunE` returns nil for transient failures); §Change 4 (`portal clean` early-exit special case Debug breadcrumb, entry-point + terminal log lines).

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-5 | approved

### Task 2-5: Invert `cmd/clean_test.go:327-368` destructive subtest

**Problem**: The subtest `"zero live panes prunes every hook entry"` at `cmd/clean_test.go:327-368` codifies the destructive behaviour as **correct**, with a comment block (lines 333-335) stating: *"Phase 4: CleanStale runs unconditionally. With no live panes, every hooks.json entry is genuinely orphaned and must be pruned."* That mental model — empty live set ⇒ genuinely orphaned ⇒ destructive prune — **is** the bug expressed as a positive test. Leaving this subtest in place would (a) cause Task 2-4 to fail (the new hazard guard refuses the wipe), and (b) re-codify the destructive interpretation if Task 2-4 were ever reverted. The subtest must be inverted, not deleted: the structural coverage it provides ("what happens when `ListAllPanes` returns an empty slice") is valuable and must be preserved.

**Solution**: Invert the subtest per the structural-preserve-flip-assert shape established by commit `7e33c04b` (`impl(hooks-skip-bootstrap): T1-2 — invert hooks list test`). Preserve setup (temp dir, projects/hooks files, `mockCleanPaneLister{panes: []string{}}` injection, `rootCmd.Execute()` call) verbatim. Flip the asserted outcome from "every hook entry pruned" to **"no entry removed, hooks file unchanged, output reports the deferral."** Rewrite the comment block at lines 333-335 to capture the new mental model: **"empty live set is *ambiguous*, not authoritative — the hazard guard treats it as a transient signal and defers."** Optionally rename the subtest from `"zero live panes prunes every hook entry"` to `"zero live panes refuses wipe (hazard guard)"` so the test name itself reflects the post-fix contract.

**Outcome**: The inverted subtest passes against the Task 2-4 implementation; the asserted outcome is "no entry removed, hooks file byte-identical to seed, user-facing stderr contains no `Removed stale hook:` lines for the seeded entries"; the comment block at lines 333-335 carries the new mental model in prose. A future regression that re-enables the silent wipe at `portal clean` would fail this subtest.

**Do**:
1. Open `cmd/clean_test.go` at lines 327-368.
2. Preserve verbatim the setup (lines 328-339): `t.TempDir`, projects-file env var, hooks-file env var, the seeded `hooks.json` with two entries.
3. Preserve verbatim the injection (lines 341-344): `cleanDeps = &CleanDeps{AllPaneLister: &mockCleanPaneLister{panes: []string{}}}` and the cleanup.
4. Preserve verbatim the invocation (lines 346-353): `rootCmd.SetArgs([]string{"clean"})`, `rootCmd.Execute()`, and the `unexpected error` check (the subtest still expects no error — `portal clean` returns nil even when the hazard guard fires).
5. Replace the assertion block at lines 356-367 with the inverted assertion:
   ```go
   // After the hazard guard fires, the user-facing stderr must NOT
   // contain any "Removed stale hook:" line for the seeded entries.
   out := buf.String()
   if strings.Contains(out, "Removed stale hook:") {
       t.Errorf("expected no hook removals when live-pane set is empty (hazard guard); got %q", out)
   }

   // hooks.json content must be byte-identical to the seed — the
   // empty live-pane set is ambiguous, not authoritative.
   data := readHooksJSON(t, hooksFile)
   if len(data) != 2 {
       t.Errorf("expected hooks file to retain both seeded entries; got %v", data)
   }
   if _, ok := data["my-session:0.1"]; !ok {
       t.Error("expected my-session:0.1 to be preserved (hazard guard refused wipe)")
   }
   if _, ok := data["other-session:1.0"]; !ok {
       t.Error("expected other-session:1.0 to be preserved (hazard guard refused wipe)")
   }
   ```
6. Rewrite the comment block at lines 333-335. New text:
   ```go
   // Hazard guard: when the live-pane set is empty but hooks.json holds
   // entries, the cleanup treats the live set as *ambiguous* (a transient
   // tmux failure or a saver-respawn race produces this signal), not
   // *authoritative* (no live panes therefore every entry is orphaned).
   // The destructive wipe is refused; next bootstrap retries. Empty live
   // set ⇒ unknown, not empty.
   ```
7. Optionally rename the subtest header at line 327 from `t.Run("zero live panes prunes every hook entry", ...)` to `t.Run("zero live panes refuses wipe (hazard guard)", ...)` so the test name reflects the post-fix contract. Renaming is preferred — it forecloses any future reader assuming the original name still describes the test.
8. Run `go test ./cmd -run TestCleanCommand -v` to confirm the inverted subtest and all sibling subtests pass.
9. Run `go test ./...` to confirm no other test in the repository depends on the prior destructive behaviour.

**Acceptance Criteria**:
- Subtest at `cmd/clean_test.go:327-368` (post-edit line numbers may shift) preserves the original setup, injection, and invocation block verbatim.
- Asserted outcome flipped: `hooks.json` retains both seeded entries; no `"Removed stale hook:"` line appears in user-facing stderr for those entries.
- Comment block at lines 333-335 rewritten to capture the "empty live set is ambiguous, not authoritative" mental model.
- Subtest renamed (preferred) or its docstring/header updated so future readers do not misread it as the original destructive expectation.
- `go test ./cmd -run TestCleanCommand` green; non-empty-live-set sibling subtests (e.g., "ListAllPanes error preserves hooks (safety net)" at line 370) pass unchanged.
- `go test ./...` green.

**Tests**: This task **is** the test edit.

**Edge Cases**:
- Comment block must carry the new mental model in prose — per spec, "the new mental model — **'empty live set is *ambiguous*, not authoritative'** — is captured in test prose alongside the assertions." A code-only change that flips the assertions but leaves the old comment in place would be incomplete.
- Sibling subtest at line 370 (`"ListAllPanes error preserves hooks (safety net)"`) already asserts the correct outcome for mode (a) (stub returns `(nil, err)` → hooks preserved). It should continue passing unchanged after Task 2-4 — verify this explicitly.
- The inverted subtest must continue to expect `rootCmd.Execute() == nil` (no user-facing error) because Task 2-4 deliberately preserves the "silence-and-continue" RunE posture under the hazard guard.
- Hazard `Warn` recording: the existing test scaffold writes to a buffer attached to `rootCmd.SetOut(buf)` — that buffer captures stdout (the user-facing `Removed stale hook:` lines), **not** `portal.log`. The post-fix `Warn` lands in `portal.log` (via `openNoRotateLogger`), which this test does not currently inspect. Asserting the Warn appears in `portal.log` would require additional plumbing — defer that to the integration test in Phase 3 per spec §Test Requirements §Coverage Matrix. This unit subtest scopes its assertion to the file/stderr surface only.

**Context**:
- Spec §Test Requirements §Inverted Subtest specifies the structural-preserve-flip-assert shape and points to commit `7e33c04b` as the canonical inversion pattern (`impl(hooks-skip-bootstrap): T1-2 — invert hooks list test, add hooks set test`).
- The pre-fix destructive subtest comment is quoted in the spec under "Layer 2 — Unguarded Destructive Consumer" — it is the textual evidence that the bug was codified-as-correct in tests. Inverting it removes that codification.
- Spec §Out of Scope explicitly rules out migration messaging for users who relied on `portal clean` as "kill all hooks when no tmux." The inversion does not provide an alternative for that workflow — users wanting to clear all hooks should use `portal hooks rm` explicitly. This task does not need to add a deprecation notice.

**Spec Reference**: §Test Requirements §Inverted Subtest (structural-preserve-flip-assert shape, commit `7e33c04b` reference); §Root Cause §Layer 2 (the destructive comment block as bug-codified-as-correct); §Out of Scope (no migration messaging).
