# Analysis Cycle 1 — Proposed Tasks

## Task 1: Extract shared `runHookStaleCleanup` helper and adopt `AllPaneLister` in `cleanStaleAdapter`
status: approved
severity: medium
sources: duplication, architecture

**Problem**: The six-branch hazard-guarded stale-hook cleanup algorithm (open no-rotate logger → load hooks store → enumerate live panes → emit entry-point Debug → apply zero-live-panes hazard guard → call `store.CleanStale` → emit completion Debug) is duplicated verbatim across three sites: `cmd/bootstrap_production.go:129-180` (`cleanStaleAdapter.CleanStale`), `cmd/clean.go:65-141` (`portal clean` RunE), and `cmd/bootstrap_production_test.go:90-133` (`cleanStaleAdapterT` test mirror). The load-bearing log format strings (`"stale-hook cleanup: live=%d persisted=%d"`, `"stale-hook cleanup: removed=%d"`, `"stale-hook cleanup: list-panes failed: %v"`, `"stale-hook cleanup: hookStore.Load failed: %v"`, multi-line hazard-guard warn) are repeated byte-identically. Integration tests substring-match those strings — drift between callsites silently passes against the un-drifted callsite. The test-local mirror exists only because `cleanStaleAdapter` holds `client *tmux.Client` directly rather than the `AllPaneLister` interface already declared in `cmd/clean.go:13-15`.

**Solution**: Extract a package-private helper `runHookStaleCleanup(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger, policy listErrorPolicy, onRemoved func(string)) error` in package `cmd`. Policy axes (`returnError` for adapter, `swallow` for `portal clean`) and optional `onRemoved` callback (nil for adapter, stdout writer for `portal clean`) cover the full delta. Simultaneously change `cleanStaleAdapter.client *tmux.Client` to `cleanStaleAdapter.lister AllPaneLister`; production wiring still passes `*tmux.Client` since it satisfies the interface. The `persisted==0` early-exit and stdout "Removed stale hook:" lines stay at the `portal clean` callsite.

**Outcome**: Each log format string declared once; integration tests across both callsites assert against a single source of truth; `cleanStaleAdapterT` mirror deleted; drift between production callsites becomes compile-time impossible.

**Do**:
1. Add `listErrorPolicy` type with `returnError` / `swallow` values in package `cmd`.
2. Create `runHookStaleCleanup(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger, policy listErrorPolicy, onRemoved func(string)) error`. Body: `ListAllPanes` → entry Debug → hazard guard (warn + bail when `live==0 && persisted>0`) → `store.CleanStale` → completion Debug → invoke `onRemoved(name)` per removed entry when non-nil. Honour policy on `ListAllPanes` error.
3. Change `cleanStaleAdapter` field `client *tmux.Client` → `lister AllPaneLister`; update `buildProductionOrchestrator` wiring.
4. Reduce `cleanStaleAdapter.CleanStale` to load hooks store + delegate `runHookStaleCleanup(..., returnError, nil)`.
5. Reduce `portal clean` RunE's CleanStale block to `persisted==0` early-exit + `runHookStaleCleanup(..., swallow, stdoutWriter)` where `stdoutWriter` prints `"Removed stale hook: <name>"`.
6. Delete `cleanStaleAdapterT` from `cmd/bootstrap_production_test.go`. Re-point its four subtests to drive the shared helper directly with a stub `AllPaneLister` + recording logger, or drive `cleanStaleAdapter` with a `stubAllPaneLister`.
7. Re-run both integration test files to confirm substring assertions still match.

**Acceptance Criteria**:
- Exactly one declaration of each of the five load-bearing log format strings in package `cmd`.
- `cleanStaleAdapter` field is `AllPaneLister`, not `*tmux.Client`.
- `cleanStaleAdapterT` deleted.
- `cleanStaleAdapter.CleanStale` and `portal clean`'s CleanStale block are thin wrappers (≤ ~10 lines beyond the helper call).
- All existing unit and integration tests pass unmodified.
- `go test ./...` green.

**Tests**:
- Unit tests on `runHookStaleCleanup` covering: hazard guard fires when `live==0 && persisted>0`; `ListAllPanes` error under `returnError` propagates; under `swallow` logs+returns nil; `onRemoved` invoked once per removed entry and noop when nil; happy-path entry/completion Debug logging.
- Re-run both transient-listpanes integration tests unmodified to confirm log substrings still match across both callsites.

---

## Task 2: Promote shared transient-list-panes test scaffolding to `internal/transienttest`
status: approved
severity: medium
sources: duplication, architecture

**Problem**: `failureMode`, `transientListPanesCommander`, `socketCommander`, `seedHooksJSON`, `hooksJSONBytes`, `resolveHooksFilePathFromEnv` are declared in `package bootstrap_test` at `cmd/bootstrap/transient_listpanes_helpers_integration_test.go:60-296` and re-declared in `package cmd` at `cmd/cleanstale_transient_listpanes_integration_test.go:95-297` — ~280 lines of duplicated test scaffolding. Bodies already diverge (the `cmd` copy strips OneShot and smoke fixtures). The argv-matching predicate and simulated error message are load-bearing for assertions in both packages; silent drift is a real risk. Go's prohibition on `_test.go` symbol export across packages is the only thing forcing the duplication.

**Solution**: Lift shared types/helpers into a new test-only package `internal/transienttest`, analogous to `tmuxtest`/`portaltest`/`restoretest`/`portalbintest`. Both integration-test files import the shared symbols; OneShot and smoke fixtures become available to both callsites.

**Outcome**: One canonical declaration of `FailureMode`, `TransientListPanesCommander`, `SocketCommander`, `SeedHooksJSON`, `HooksJSONBytes`, `ResolveHooksFilePathFromEnv`. Drift across packages is compile-time impossible. ~280 lines of duplicated test code deleted.

**Do**:
1. Create directory `internal/transienttest/`.
2. Move shared symbols into production-syntax `.go` files (follow precedent of `internal/tmuxtest`/`internal/portaltest`/`internal/restoretest`/`internal/portalbintest`).
3. Capitalise symbol names per Go convention.
4. Update `cmd/bootstrap/transient_listpanes_helpers_integration_test.go` to import `internal/transienttest` and remove local declarations.
5. Update `cmd/cleanstale_transient_listpanes_integration_test.go` likewise.
6. Update `cmd/cleanstale_transient_listpanes_clean_integration_test.go` likewise.
7. Add new package to the "test-only helpers" row of CLAUDE.md alongside `restoretest`/`tmuxtest`/`portalbintest` — note production code must not import.

**Acceptance Criteria**:
- `internal/transienttest` package exists with exported `FailureMode`, `TransientListPanesCommander`, `SocketCommander`, `SeedHooksJSON`, `HooksJSONBytes`, `ResolveHooksFilePathFromEnv`.
- No declarations of these symbols remain in the three integration test files.
- `go test -tags integration ./cmd/... ./cmd/bootstrap/...` green.
- CLAUDE.md test-only-packages row mentions `transienttest`.

**Tests**:
- Re-run all three integration tests under the `integration` build tag.
- Confirm `go build ./...` succeeds and `internal/transienttest` is not transitively imported by any non-test production package.

---

## Task 3: Export `bootstrap.NoopLogger` and delete `cleanStaleNoopLogger`
status: approved
severity: low
sources: architecture

**Problem**: `cmd/bootstrap_production.go:48-69` defines `cleanStaleNoopLogger` as a byte-identical four-method no-op stand-in for the unexported `noopLogger` in `cmd/bootstrap/bootstrap.go:185-202`. The duplication will recur whenever a future cleanup-style adapter in `cmd/` or `internal/bootstrapadapter` needs a nil-tolerant `bootstrap.Logger`.

**Solution**: Export the existing `noopLogger` as `bootstrap.NoopLogger`; replace `cleanStaleNoopLogger` usage in `cmd/bootstrap_production.go` with `bootstrap.NoopLogger{}`; delete the local type.

**Outcome**: Single canonical no-op `bootstrap.Logger` implementation.

**Do**:
1. In `cmd/bootstrap/bootstrap.go:185-202`, rename `noopLogger` → `NoopLogger`.
2. Update in-package usages.
3. Replace `cleanStaleNoopLogger` references in `cmd/bootstrap_production.go` with `bootstrap.NoopLogger{}`.
4. Delete `cleanStaleNoopLogger` type from `cmd/bootstrap_production.go:48-69`.

**Acceptance Criteria**:
- `bootstrap.NoopLogger` exported; only no-op `Logger` implementation in the repo.
- `cleanStaleNoopLogger` no longer exists.
- All existing tests pass unmodified.

**Tests**:
- No new tests required.
- `go test ./cmd/... ./cmd/bootstrap/...` green.

---

## Task 4: Fix stale comparative docstring on `ListAllPanesWithFormat`
status: approved
severity: low
sources: standards

**Problem**: `internal/tmux/tmux.go:657-658` — the `ListAllPanesWithFormat` docstring reads "Unlike `ListAllPanes`, this method propagates the underlying error so callers can distinguish 'no panes' from 'tmux failed'." Post-spec-Change-1, `ListAllPanes` is now a thin wrapper over `ListAllPanesWithFormat` and also propagates errors; the "Unlike `ListAllPanes`" framing is misleading. The genuine difference post-fix is format-string flexibility, not error policy.

**Solution**: Rewrite the second sentence to describe the actual post-fix differentiator (raw tmux output for callers needing a non-default format string; caller-side parsing) vs. `ListAllPanes` as the structural-key-format convenience wrapper. Drop the obsolete "Unlike `ListAllPanes`" comparison.

**Outcome**: Both docstrings accurately describe their post-fix relationship; no false implication of a divergent error contract.

**Do**:
1. In `internal/tmux/tmux.go:657-658`, replace the "Unlike `ListAllPanes`..." sentence with a sentence framing the difference as format-string flexibility vs. structural-key convenience wrapper.
2. Confirm consistency with the `ListAllPanes` docstring at `tmux.go:683-700`.

**Acceptance Criteria**:
- "Unlike `ListAllPanes`" no longer appears in `ListAllPanesWithFormat`'s docstring.
- New sentence accurately characterises the post-fix difference.
- `go vet ./...` and `go build ./...` green.

**Tests**:
- No tests required (docstring-only change).
