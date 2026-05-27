# Plan: Bootstrap CleanStale Wipes Hooks On Tmux Transient

## Phases

### Phase 1: Repurpose `ListAllPanes` to Propagate Errors

status: approved
approved_at: 2026-05-27

**Goal**: Replace the error-swallow body of `(*tmux.Client).ListAllPanes` with a thin wrapper around `ListAllPanesWithFormat` so transient tmux failures surface as `(nil, err)` instead of `([]string{}, nil)`. Closes failure mode (a) at the source. Rewrite the helper docstring to describe the new error-propagating contract.

**Why this order**: This is the lowest seam in the stack. Every downstream change in Phase 2 (hazard guard, adapter logging, error propagation through `cleanStaleAdapter` and `portal clean`) depends on the helper returning a real error on transient failure. Doing it first means Phase 2 work consumes the already-correct contract; reversing the order would force Phase 2 to mock a contract that does not yet exist in production. The two known callers (audited in spec) check `err` and treat `nil`/empty slices identically for range/len, so the contract shift is safe to land independently — non-empty-live-set paths in `cmd/clean_test.go` remain green.

**Acceptance**:
- [ ] `(*tmux.Client).ListAllPanes` delegates to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` and returns `(nil, err)` on non-nil helper error
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing, describes the error-propagating contract
- [ ] New unit test asserts `(nil, err)` is returned when the underlying `Commander` returns exit ≠ 0 on `list-panes -a`
- [ ] New unit test asserts `([]string{}, nil)` is returned only when `list-panes -a` legitimately returns exit 0 with empty stdout
- [ ] Existing non-empty-live-set tests in `cmd/clean_test.go` pass unchanged
- [ ] `go test ./...` is green

---

### Phase 2: Hazard Guard and Adapter Logging at Both `CleanStale` Callsites

status: approved
approved_at: 2026-05-27

**Goal**: Add the mass-deletion hazard guard and two-line logging contract to both destructive consumers — `cleanStaleAdapter.CleanStale` (`cmd/bootstrap_production.go:76-83`) and the `portal clean` hook-cleanup tail (`cmd/clean.go:75-91`). Closes failure mode (b). Lifts the prior-art guard from `cmd/bootstrap/stale_marker_cleanup.go:126-141` verbatim (with `len(persisted)` substituted for `len(markers)`). Plumbs a logger into `cleanStaleAdapter` and the `portal clean` `RunE` closure (via `openNoRotateLogger`). Inverts the `cmd/clean_test.go:327-368` destructive subtest so the existing destructive interpretation is replaced by the "refuse + warn" assertion, and creates the new `cmd/bootstrap_production_test.go` covering the adapter's four required paths (hazard guard fires, both-sides-empty no-op, error propagates as soft warning, legitimate stale removal). Rewrites the `cleanStaleAdapter.CleanStale` docstring to describe the new contract; lifts and adapts the load-bearing comment block from `stale_marker_cleanup.go:80-92` naming `hooks.json` entries as the protected data.

**Why this order**: Phase 1's contract change is the prerequisite for the error-propagation branch of Change 3 — without it the adapter's "error from `ListAllPanesWithFormat` → soft warning" assertion cannot be tested deterministically at the adapter layer. Both callsites are tightly cohesive: same guard shape, same logging contract, same load-bearing comment, same docstring rewrite directive — splitting bootstrap vs `portal clean` into separate phases would duplicate code review surface without producing independent checkpoints. The inverted `cmd/clean_test.go` subtest belongs here (not Phase 1) because it asserts the post-guard outcome ("no entry removed, hooks file unchanged, output reports the deferral"), not the post-helper outcome.

**Acceptance**:
- [ ] `cleanStaleAdapter` struct gains a `Logger` field, populated from the orchestrator-scope logger via the same field-population pattern at `cmd/bootstrap_production.go:147-152`; nil-tolerance applied (no-op substitute when nil)
- [ ] `cleanStaleAdapter.CleanStale` calls `hookStore.Load()`, emits a `Warn` and returns the error on `Load()` failure (no destructive call), and applies the hazard guard `len(livePanes) == 0 && len(persisted) > 0` before invoking `hookStore.CleanStale`
- [ ] `portal clean` `RunE` closure acquires a logger via `openNoRotateLogger`, applies the same hazard guard at `cmd/clean.go:75-91`, and continues to return nil for transient failures of the hook-cleanup tail (matching pre-fix safety-net posture)
- [ ] Both callsites emit exactly the log lines specified in Change 4: one `Debug` after enumeration (`"stale-hook cleanup: live=<N> persisted=<M>"`) plus exactly one terminal line (`Warn` on hazard-guard skip, `Warn` on enumeration error, `Warn` on `Load()` failure, or `Debug` on completion with `removed=<K>`); enumeration-error and `Load()`-failure branches emit only the terminal line
- [ ] `portal clean`'s pre-enumeration `persisted == 0` early-exit emits the single `Debug` breadcrumb `"stale-hook cleanup: persisted=0, skipping"`
- [ ] Adapted load-bearing comment block lifted from `stale_marker_cleanup.go:80-92` with `hooks.json` entries named as the protected data and the "deferral is a successful soft outcome" framing preserved
- [ ] `cleanStaleAdapter.CleanStale` docstring rewritten to describe the post-fix contract (error surfacing, hazard-guard skip, normal-path stale removal)
- [ ] New file `cmd/bootstrap_production_test.go` exists with the four required subtests: hazard guard fires on empty live + non-empty persisted, both-sides-empty no-op, error from `ListAllPanesWithFormat` propagates, legitimate stale removal removes only orphaned entries
- [ ] `cmd/clean_test.go:327-368` subtest inverted: same setup and call shape, asserted outcome flipped to "no entry removed, hooks file unchanged, deferral reported"; comment block at lines 333-335 rewritten to capture the "empty live set is ambiguous, not authoritative" mental model
- [ ] Hazard-guard subtest asserts the `Warn` is recorded and the `Debug`-on-completion line is **not** (mutual exclusivity)
- [ ] `go test ./...` is green

---

### Phase 3: Integration Coverage for Tmux Transient and `portal clean`

status: approved
approved_at: 2026-05-27

**Goal**: Land the two end-to-end integration tests required by the spec's coverage matrix. The first reproduces the original incident shape (real tmux server, populated `hooks.json`, kill `_portal-saver` mid-bootstrap, `Commander` stub forces `list-panes -a` exit ≠ 0) and asserts `hooks.json` is unchanged at the end of bootstrap. The second exercises the same posture against the `portal clean` callsite.

**Why this order**: Integration coverage validates that Phases 1 and 2 jointly close both failure modes against a real tmux server — a property the unit subtests cannot verify because they bypass the orchestrator's soft-warning wiring and the bootstrap step-ordering interaction with `EnsureSaver` / `Restore`. Both integration tests depend on the new logging contract (they assert `portal.log` lines as the post-fix fingerprint distinguishing modes (a) and (b)) and on the hazard-guard being in place. Running them earlier would force speculative skeleton tests; running them here verifies the acceptance criteria against the end state of the fix.

**Acceptance**:
- [ ] New integration test spawns a real tmux server, populates `hooks.json` with ≥ 1 user-session entry, arranges for `list-panes -a` to return exit ≠ 0 during bootstrap via `Commander` injection at the `tmux.Client` boundary, runs a bootstrap-triggering command, and asserts `hooks.json` content is byte-identical before and after
- [ ] Integration test also asserts `portal.log` contains the propagated-error `Warn` from Change 4 (post-fix distinguishability fingerprint for mode (a))
- [ ] New integration test for the `portal clean` callsite: same `Commander` stub posture, asserts `hooks.json` is unchanged and `portal.log` contains the propagated-error `Warn` (or hazard-guard `Warn` for the empty-stdout variant)
- [ ] Both integration tests use `portaltest.IsolateStateForTest(t)` and `cmd.Env = env` on every spawned subprocess, per the project's daemon-test isolation rule
- [ ] Acceptance Criteria 1-5 from the specification verified end-to-end by these integration tests (hazard guard, error propagation, normal-path preservation, log breadcrumbs, bootstrap posture preserved)
- [ ] `go test ./...` green, integration-tagged suite green
