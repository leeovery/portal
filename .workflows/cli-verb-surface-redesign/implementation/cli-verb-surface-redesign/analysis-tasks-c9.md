---
topic: cli-verb-surface-redesign
cycle: 9
total_proposed: 1
---
# Analysis Tasks: CLI Verb-Surface Redesign (Cycle 9)

## Task 1: Extract a store-owned staleness predicate so doctor's diagnosis and the prune cannot drift
status: pending
severity: medium
sources: duplication, architecture

**Problem**: The "is this entry stale?" classification exists in two independent copies for each store, and `doctor`'s strictly read-only diagnosis routes through the reimplemented copies while the mutation path (daemon automation + `doctor --fix`) routes through the stores' `CleanStale`.
- Hooks: `hooks.Store.CleanStale` computes `removed = persisted key ∉ liveSet` (internal/hooks/store.go:262-271), while `cmd/doctor.go`'s `checkStaleHooks`/`countStaleHookKeys` build their own `liveSet` and re-derive the identical ∉ count (cmd/doctor.go:448-460).
- Projects: `project.Store.CleanStale` runs a three-way `os.Stat` switch (nil→keep, `ErrNotExist`→remove, other→keep) at internal/project/store.go:202-213, and `checkStaleProjects` re-implements the identical switch read-only (cmd/doctor.go:477-488; its comment even says "It mirrors project.Store.CleanStale's os.Stat classification").
Nothing forces the two to agree. If a store's staleness rule ever changes (new key-matching semantics, different handling of a symlinked/broken dir), `doctor`'s report silently drifts from what `--fix` actually prunes — a latent correctness inconsistency in the one tool whose job is to report health accurately. Because the hazard guard exists for data-loss safety (never wipe non-reconstructable user on-resume hooks on a down server), a silent green/prune disagreement has real stakes.

**Solution**: Make each store the single owner of its staleness classification, and have both the prune (`CleanStale`) and doctor's read-only diagnosis derive their stale set from that one predicate. Extract ONLY the ∉ / `os.Stat` classification — do NOT fold in or relocate the mass-deletion hazard guard.
- Hooks: extract the ∉ classification into a single store-owned predicate in `internal/hooks` (e.g. a pure `func StaleKeys(persisted map[string]map[string]string, live []string) []string`, or an equivalent operating on the store's internal `hooksFile` type). Redefine `hooks.Store.CleanStale` to select removals via that predicate, and replace `countStaleHookKeys` in doctor with a read-only call to the same predicate.
- Projects: extract the `os.Stat` tri-state classification into a store-owned predicate in `internal/project` (e.g. `func (s *Store) StaleEntries() ([]Project, error)` that Loads and classifies). Redefine `project.Store.CleanStale` in terms of it, and have `checkStaleProjects` count via the same classifier read-only.

**Outcome**: "What is stale" for hooks and for projects each lives in exactly one place, inside the owning store package. Doctor's diagnosis and the prune path provably share that predicate and cannot silently drift, while doctor's diagnosis stays strictly read-only (no Save/prune) and the mass-deletion hazard guard is unchanged and still in force.

**Do**:
1. In `internal/hooks/store.go`, add a single staleness predicate that computes the set of persisted keys absent from a supplied live-key set (the current ∉ logic at store.go:262-271). Keep the store package the owner. Prefer a pure function/method that takes an already-loaded persisted map + live keys so doctor can reuse it without a second load.
2. Rewrite `hooks.Store.CleanStale` to compute `removed` via that predicate (no behaviour change — same keys removed, same zero-removal no-op, same batched Save, same summary emission).
3. In `cmd/doctor.go`, delete `countStaleHookKeys` (doctor.go:445-460) and have `checkStaleHooks` count stale entries via the hooks predicate. Preserve the load-bearing guard ORDER exactly: still Load `persisted`, still enumerate `live`; on `err` enumerating live → `checkNotEvaluable`; on `len(live)==0` with `len(persisted)>0` → `checkNotEvaluable` ("zero live panes with hooks present (not evaluable)"); `len(live)==0` with no persisted → `checkPass` ("no hooks"). Only the past-the-guard ∉ count delegates to the shared predicate. Doctor must NOT prune or Save.
4. In `internal/project/store.go`, add a store-owned predicate returning the stale (`os.ErrNotExist`) entries using the exact tri-state semantics at store.go:202-213 (nil→live, `ErrNotExist`→stale, any other error e.g. permission-denied→retained/NOT stale). Rewrite `project.Store.CleanStale` in terms of it (no behaviour change — same removed set, same zero-removal no-op, same Save, same DEBUG/summary emission).
5. In `cmd/doctor.go`, delete the duplicated `os.Stat` switch in `checkStaleProjects` (doctor.go:477-488) and count staleness via the project predicate read-only. Preserve `checkNotEvaluable` on a load error and the `checkFail`/`checkPass` detail strings verbatim (`pluralCount(stale, "stale project", "stale projects")` / "no stale projects").
6. Do NOT touch the mass-deletion hazard guard as a shared concern: leave the empty-live-set deferral where it lives in `checkStaleHooks` (doctor.go:429-437) and `runHookStaleCleanup` (cmd/run_hook_stale_cleanup.go:119-126). It is a cmd-layer repair-safety policy, not a store predicate — consolidating it is explicitly out of scope for this task.
7. Update the now-stale doc comments on `checkStaleHooks`/`checkStaleProjects` so they state they DERIVE the stale set from the store's predicate rather than "mirror"/"re-implement" it.

**Acceptance Criteria**:
- The hooks ∉ staleness classification exists in exactly one place in `internal/hooks`; both `hooks.Store.CleanStale` and `cmd/doctor.go`'s `checkStaleHooks` derive their stale set from it. `countStaleHookKeys` is deleted.
- The project `os.Stat` tri-state staleness classification exists in exactly one place in `internal/project`; both `project.Store.CleanStale` and `cmd/doctor.go`'s `checkStaleProjects` derive from it. The duplicated switch in doctor is deleted.
- `doctor`'s stale-hook and stale-project checks remain strictly read-only — no `Save`, no prune, no side effects — and return identical `checkResult` status/detail strings to today for every input.
- The mass-deletion hazard guard is behaviourally unchanged: empty-or-errored live-pane enumeration with hooks present yields `checkNotEvaluable` in doctor and a deferral (no delete) in `runHookStaleCleanup`; it is NOT moved into the shared predicate.
- Project staleness still treats permission-denied / non-`ErrNotExist` `os.Stat` errors as retained (NOT stale) on both the diagnosis and prune paths.
- `go build ./...`, `go test ./...` (unit lane), and `golangci-lint run` all pass.

**Tests**:
- Hooks predicate unit test: given a persisted map and a live-key set, the shared predicate returns exactly the persisted keys absent from the live set (including empty-result and all-stale cases), and `hooks.Store.CleanStale` removes exactly that set.
- Doctor stale-hooks parity: a table test proving `checkStaleHooks`' stale count equals the shared predicate's result for representative inputs, and that the hazard-guard paths (live enumeration error; `len(live)==0` with persisted present; `len(live)==0` with none persisted) still map to `checkNotEvaluable`/`checkPass` as before with no prune.
- Projects predicate unit test: the store predicate classifies a present dir as live, a missing dir (`ErrNotExist`) as stale, and a permission-denied/other-error dir as retained; `project.Store.CleanStale` removes exactly the stale set.
- Doctor stale-projects parity: `checkStaleProjects` reports the same stale count as the shared project predicate for present/missing/error-dir fixtures, remains read-only (projects.json unmodified after the check), and preserves `checkNotEvaluable` on a load error.
- Existing `doctor` and `--fix` behaviour tests (including the down-server hook-prune safety test) continue to pass unchanged.
