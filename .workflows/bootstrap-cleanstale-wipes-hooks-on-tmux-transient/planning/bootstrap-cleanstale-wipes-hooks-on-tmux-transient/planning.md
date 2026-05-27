# Plan: Bootstrap CleanStale Wipes Hooks On Tmux Transient

## Phases

### Phase 1: Repurpose `ListAllPanes` to Propagate Errors

status: approved
approved_at: 2026-05-27

**Goal**: Replace the error-swallow body of `(*tmux.Client).ListAllPanes` with a thin wrapper around `ListAllPanesWithFormat` so transient tmux failures surface as `(nil, err)` instead of `([]string{}, nil)`. Closes failure mode (a) at the source. Rewrite the helper docstring to describe the new error-propagating contract.

**Why this order**: This is the lowest seam in the stack. Every downstream change in Phase 2 (hazard guard, adapter logging, error propagation through `cleanStaleAdapter` and `portal clean`) depends on the helper returning a real error on transient failure. Doing it first means Phase 2 work consumes the already-correct contract; reversing the order would force Phase 2 to mock a contract that does not yet exist in production. The two known callers (audited in spec) check `err` and treat `nil`/empty slices identically for range/len, so the contract shift is safe to land independently — non-empty-live-set paths in `cmd/clean_test.go` remain green.

**Acceptance**:
- [ ] `(*tmux.Client).ListAllPanes` delegates to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` and returns `(nil, err)` on non-nil helper error
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing; describes (a) enumeration via the error-propagating `ListAllPanesWithFormat` using the canonical `"#{session_name}:#{window_index}.#{pane_index}"` format, (b) `(nil, err)` on tmux failure, (c) `(parsePaneOutput(raw), nil)` on success, (d) that callers decide policy for empty/error results
- [ ] Existing subtest at `internal/tmux/tmux_test.go:1461-1473` (`"returns empty slice when no tmux server running"`) is inverted to assert `(nil, non-nil err)` on commander error and renamed to reflect the new contract
- [ ] New unit test asserts `(nil, err)` is returned when the underlying `Commander` returns exit ≠ 0 on `list-panes -a`
- [ ] New unit test asserts `([]string{}, nil)` is returned only when `list-panes -a` legitimately returns exit 0 with empty stdout
- [ ] Existing non-empty-live-set tests in `cmd/clean_test.go` pass unchanged
- [ ] `go test ./...` is green

#### Tasks

status: approved
approved_at: 2026-05-27

| Internal ID | Name | Description | Edge Cases | Acceptance |
|-------------|------|-------------|------------|------------|
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-1-1 | Propagate tmux errors from `ListAllPanes` | Replace the error-swallow body of `(*tmux.Client).ListAllPanes` in `internal/tmux/tmux.go` with a thin delegation to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` that returns `(nil, err)` on helper error and `parsePaneOutput(raw), nil` on success; rewrite the docstring to remove the "no tmux server" convenience framing and describe the new error-propagating contract. | Underlying `Commander` returns exit ≠ 0; wrapped tmux transport error; docstring framing no longer mentions the swallow | Failing unit test asserting `(nil, err)` on `Commander` exit ≠ 0 lands first and passes after the repurpose; `ListAllPanes` body delegates to `ListAllPanesWithFormat`; docstring rewritten; `go test ./...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-1-2 | Pin the legitimate-empty contract for `ListAllPanes` | Add a unit test in `internal/tmux/` asserting that when the underlying `Commander` returns exit 0 with empty stdout, `ListAllPanes` returns `([]string{}, nil)` — distinguishing "tmux failed" from "no panes exist" at the helper boundary. | `list-panes -a` exit 0 empty stdout; whitespace-only stdout coerces to empty slice via `parsePaneOutput` | New unit test exists and passes against the Task 1 implementation; covers both the empty-stdout and whitespace-only cases; `go test ./...` green |

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

#### Tasks

status: approved
approved_at: 2026-05-27

| Internal ID | Name | Description | Edge Cases | Acceptance |
|-------------|------|-------------|------------|------------|
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-1 | Plumb `Logger` into `cleanStaleAdapter` | Add a `Logger` field to the `cleanStaleAdapter` struct in `cmd/bootstrap_production.go` (interface shape matching `bootstrap.Logger` — `Debug`/`Warn`/`Error`), populate it from the orchestrator-scope logger using the same field-population pattern at `cmd/bootstrap_production.go:147-152` where `MarkerCleanupCore` already receives one, and apply the nil-tolerance contract so `Logger.Warn` / `Logger.Debug` are safe to call unconditionally (mirroring `MarkerCleanupCore.CleanStaleMarkers` at `stale_marker_cleanup.go:109-112`). No behaviour change yet — wiring only. | nil logger from `openNoRotateLogger` failure; no-op substitute behaviour; field population symmetry with `MarkerCleanupCore` | New struct field present; orchestrator wiring populates the field; calling `Logger.Warn` / `Logger.Debug` with the nil substitute is a no-op; existing tests pass unchanged; `go test ./...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-2 | Add hazard guard and two-line logging to `cleanStaleAdapter.CleanStale` | In `cmd/bootstrap_production.go:71-83`, rewrite `cleanStaleAdapter.CleanStale` to: (1) call `a.client.ListAllPanes()` and on non-nil error emit the terminal `Warn` and return the error; (2) call `a.store.Load()` and on non-nil error emit `"stale-hook cleanup: hookStore.Load failed: <error>"` Warn and return the error; (3) emit the entry-point `Debug` `"stale-hook cleanup: live=<N> persisted=<M>"`; (4) apply the hazard guard `len(livePanes) == 0 && len(persisted) > 0` — emit the hazard `Warn` and return nil; (5) `len==0 && len==0` returns nil silently with no extra terminal line; (6) otherwise call `a.store.CleanStale(livePanes)` and emit the `Debug` `"stale-hook cleanup: removed=<K>"`. Lift the load-bearing comment block from `cmd/bootstrap/stale_marker_cleanup.go:80-92` and adapt it to name `hooks.json` entries as the protected data while preserving the "deferral is a successful soft outcome" framing. Rewrite the existing docstring at lines 71-75 to describe the new contract (error surfacing, hazard-guard skip, normal-path stale removal) — remove the misleading "degrades to no-op" framing. | Propagated error from `ListAllPanesWithFormat`; `Load()` failure on corrupt `hooks.json`; `len(live)==0 && len(persisted)>0`; both-sides-empty silent no-op; legitimate stale removal path; terminal-line mutual exclusivity; nil-tolerant logger calls | Adapter body matches the six-branch algorithm; comment block lifted and adapted; docstring rewritten; verified by Task 2-3 unit tests; `go test ./...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-3 | Create `cmd/bootstrap_production_test.go` with the four required adapter subtests | Create new file `cmd/bootstrap_production_test.go` with four subtests: (i) **hazard guard fires** — empty live + N persisted → no `Save`, hazard `Warn` recorded, `Debug`-on-completion **not** emitted; (ii) **both-sides-empty no-op** — no Warn, no Save, no error; (iii) **`ListAllPanesWithFormat` error propagates as soft warning** — adapter returns error, emits only terminal Warn (no entry-point Debug); (iv) **legitimate stale removal** — live `{a,b,c}`, persisted `{a,b,c,d}` → `d` removed, `removed=1` Debug emitted. Stubs via local `AllPaneLister` interface. No `t.Parallel()`. | Mutual-exclusivity assertion across all four branches; mock logger captures Debug + Warn calls; stub returning `(nil, err)` for mode (a); stub returning `([]string{}, nil)` for mode (b); `Save` spy on hooks store | New file exists with all four subtests; hazard-guard subtest explicitly asserts the `Debug`-on-completion line is absent; `go test ./cmd -run TestCleanStaleAdapter` green; `go test ./...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-4 | Acquire logger and apply hazard guard in `portal clean` `RunE` | In `cmd/clean.go:75-91`, acquire a `*state.Logger` via `openNoRotateLogger()` (apply nil-tolerance). Preserve `len(persisted) == 0` pre-enumeration early-exit (line 71-73) and emit a single `Debug` `"stale-hook cleanup: persisted=0, skipping"`. Preserve `hookStore.Load()` failure error-return at lines 65-68 with same-shape Warn breadcrumb. After enumeration, emit the entry-point `Debug` `"stale-hook cleanup: live=<N> persisted=<M>"`, then apply the same hazard-guard algorithm from Task 2-2 (mutual-exclusivity-preserving terminal lines). RunE continues returning nil for transient failures of the hook-cleanup tail (existing pre-fix safety-net posture); user-facing stderr unchanged. | `openNoRotateLogger` returns nil; persisted==0 early-exit emits exactly one Debug; transient `ListAllPanes` error returns nil at RunE boundary; user-facing stderr unchanged | Logger acquired; hazard guard applied at lines 75-91; persisted==0 early-exit emits the single Debug breadcrumb; non-empty-live-set paths in `cmd/clean_test.go` pass unchanged; `go test ./cmd -run TestClean` green; `go test ./...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-5 | Invert `cmd/clean_test.go:327-368` destructive subtest | Invert the `"zero live panes prunes every hook entry"` subtest at `cmd/clean_test.go:327-368` so destructive interpretation is replaced by the "refuse + warn" assertion. Preserve setup and call shape. Flip asserted outcome to **"no entry removed, hooks file unchanged, output reports the deferral"** — assert `Save` was **not** called, `hooks.json` byte-identical, hazard `Warn` recorded by the test logger. Rewrite comment block at lines 333-335 to capture the **"empty live set is *ambiguous*, not authoritative"** mental model. Follow the structural-preserve-flip-assert shape from commit `7e33c04b`. | Test setup and call shape preserved; `Save` spy asserts no destructive write; `hooks.json` byte-identical before/after; hazard `Warn` recorded; comment captures the new mental model | Subtest inverted; comment block rewritten; non-empty-live-set sibling subtests pass unchanged; `go test ./cmd -run TestClean` green; `go test ./...` green |

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

#### Tasks

status: approved
approved_at: 2026-05-27

| Internal ID | Name | Description | Edge Cases | Acceptance |
|-------------|------|-------------|------------|------------|
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-1 | Shared integration scaffolding for transient `list-panes` reproduction | Build the reusable test helpers consumed by Tasks 3-2 and 3-3: real-tmux fixture with `_portal-saver` populated; `hooks.json` seeder; `Commander` injection that forces `list-panes -a` exit ≠ 0 or empty stdout; `portal.log` reader. Apply `portaltest.IsolateStateForTest(t)` for isolation. Decide whether the injection is one-shot (single failure then pass-through) or sticky (every `list-panes -a` fails until reset). | Commander pass-through for non-`list-panes -a` invocations; one-shot vs sticky failure injection; subprocess reap discipline; tmux server teardown between subtests | Helpers live in an `integration_test.go` or shared `*_test.go` file with the right build tag; the Commander stub correctly intercepts only `list-panes -a` invocations; `IsolateStateForTest` registers the fingerprint-diff backstop; helpers compile and pass a smoke-test invocation; `go test -tags integration ./...` builds clean |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-2 | Bootstrap end-to-end integration test for tmux transient `list-panes` failure | Spawn a real tmux server, seed `hooks.json` with ≥ 1 user-session entry, inject `list-panes -a` failure via the shared `Commander` stub, run a bootstrap-triggering command (e.g., `portal open <path>` or equivalent), assert `hooks.json` content is byte-identical before and after, and assert `portal.log` contains the propagated-error `Warn` from Change 4 (mode (a) fingerprint) or the hazard-guard `Warn` (mode (b) fingerprint). Cover both modes (a) and (b) as separate subtests if practical. | Kill `_portal-saver` mid-bootstrap to compound the transient (optional); mode (a) and mode (b) coverage; orchestrator soft-warning wiring exercised end-to-end; `cmd.Env = env` applied to every spawned subprocess; no zombie daemons left | Integration test exists in `cmd/` or `cmd/bootstrap/` with the right build tag; `hooks.json` byte-identical assertion holds for both mode (a) and mode (b); `portal.log` contains the propagated-error Warn or hazard-guard Warn as expected; bootstrap surfaces the warning without aborting; `portaltest.IsolateStateForTest(t)` applied and fingerprint backstop passes; `go test -tags integration -run TestBootstrap...` green |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-3 | `portal clean` end-to-end integration test for tmux transient `list-panes` failure | Same posture as Task 3-2 against the `portal clean` callsite — seed `hooks.json`, inject transient `list-panes -a` failure, run `portal clean`, assert `hooks.json` byte-identical and `portal.log` contains the expected Warn line. Cover both mode (a) and mode (b). Verify `RunE` continues to return nil and user-facing stderr is unchanged (no `Removed stale hook:` lines for the seeded entries). | Mode (a) and mode (b) both asserted; `RunE` continues returning nil for transient failures; stderr unchanged for success-by-deferral outcome; persisted-empty early-exit not conflated with hazard-guard path | Integration test exists with the right build tag; `hooks.json` byte-identical assertion holds for both modes; `portal.log` contains the expected Warn line; `portal clean` exits 0 (no user-visible error); user-facing stderr does not contain `Removed stale hook:` lines for seeded entries; `portaltest.IsolateStateForTest(t)` applied; `go test -tags integration -run TestPortalClean...` green |

---

### Phase 4: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-4-1 | Extract shared `runHookStaleCleanup` helper and adopt `AllPaneLister` in `cleanStaleAdapter` | Log format strings remain byte-identical across both callsites; integration substring assertions still match; `cleanStaleAdapterT` test mirror deleted; `cleanStaleAdapter.client *tmux.Client` → `lister AllPaneLister`; `persisted==0` early-exit and stdout "Removed stale hook:" lines remain at `portal clean` callsite |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-4-2 | Promote shared transient-list-panes test scaffolding to `internal/transienttest` | Capitalised symbol names per Go convention; OneShot and smoke fixtures available to both callsites; production code must not transitively import the package; CLAUDE.md test-only-packages row updated |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-4-3 | Export `bootstrap.NoopLogger` and delete `cleanStaleNoopLogger` | In-package usages of `noopLogger` updated post-rename; only one no-op `bootstrap.Logger` implementation remains repo-wide |
| bootstrap-cleanstale-wipes-hooks-on-tmux-transient-4-4 | Fix stale comparative docstring on `ListAllPanesWithFormat` | "Unlike `ListAllPanes`" framing removed; new framing describes format-string flexibility vs. structural-key convenience wrapper; consistency confirmed with `ListAllPanes` docstring |
