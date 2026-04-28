---
agent: architecture
cycle: 1
findings_count: 6
---
# Architecture Analysis (Cycle 1)

## Summary

Eight-step bootstrap composition is sound, but several adjacent seams have accreted duplication or stranded surface area — a parallel restoring-marker API, an informal Restorer contract, duplicated FIFO→paneKey helpers, an inert migrate-rename hook, a misnamed file in internal/hooks, and scattered skeleton-marker set/unset.

---

## Findings

### FINDING: Dead-code parallel restoring-marker API in internal/restore
- **Severity**: medium
- **Files**: `internal/restore/restore_marker.go:1-61`, `cmd/bootstrap_production.go:41-54`, `internal/state/markers.go:15`
- **Description**: Two complete implementations of the @portal-restoring marker lifecycle exist. The bootstrap.Orchestrator calls into restoreOrchestratorAdapter.Restore() (the bare Restore on the inner *restore.Orchestrator) and manages the marker via restoringMarkerAdapter.Set/Clear in cmd/bootstrap_production.go using state.RestoringMarkerName. Meanwhile internal/restore/restore_marker.go ships SetRestoring / ClearRestoring / RestoreWithMarker on *restore.Orchestrator with its own private const restoringMarker = "@portal-restoring" — an isolated string copy that the rest of the codebase no longer references. Nothing in production wires RestoreWithMarker; it is dead code that duplicates the marker-name string and the set/clear ordering. Two seams compete for ownership of the same invariant; future maintainers reading restore_marker.go will reasonably assume the package owns the marker and route through it, contradicting the orchestrator's actual contract.
- **Recommendation**: Delete restore_marker.go (and its tests) so the bootstrap orchestrator is the single owner of the @portal-restoring lifecycle. The package-level const state.RestoringMarkerName already exists for the marker string; restore-side code does not need its own. If a future caller wants a wrapped Restore-with-marker convenience, give it a constructor that accepts the orchestrator's RestoringMarker abstraction rather than reaching into client.SetServerOption directly.

### FINDING: Restorer contract leaks classification responsibility to implementations
- **Severity**: medium
- **Files**: `cmd/bootstrap/bootstrap.go:59-61,160-188`, `internal/restore/restore.go:38-82`
- **Description**: bootstrap.Restorer is declared as `Restore() error`, but the orchestrator's behaviour depends on a contract the interface does not express: implementations MUST return errors wrapped with state.ErrCorruptIndex when sessions.json is unparseable, and MUST return nil for every other soft per-session failure. internal/restore/restore.go currently honours this informal contract — but Run treats any non-corrupt error from Restore() as a fatal-ish error that aborts PersistentPreRunE (`fmt.Errorf("step 5 (Restore): %w", restoreErr)` propagates as a generic non-FatalError that cobra still surfaces as the command's exit error). The spec's "degrade locally, log, continue" principle says no per-session failure should abort. Today the path is unreachable because internal/restore conforms; tomorrow a refactor or a second implementation could break the assumption silently — a latent fatal-on-soft-error bug surface.
- **Recommendation**: Make the contract self-enforcing by either (a) narrowing Restorer to `Restore() (corrupt bool, err error)` so the orchestrator does not have to know which sentinel to look up, or (b) keeping the sentinel but documenting on the interface (and asserting in a contract test) that any other error class is invalid input. Either way, the orchestrator's catch-all branch should not silently elevate "implementation drift" to a fatal command-aborting return when the spec says the step is fundamentally degrade-locally.

### FINDING: paneKeyFromFIFO* helper duplicated across packages
- **Severity**: low
- **Files**: `cmd/state_hydrate.go:71-77`, `internal/state/fifo_sweep.go:57-64`
- **Description**: Two functions invert FIFOPath: paneKeyFromFIFOPath in cmd/state_hydrate.go and paneKeyFromFIFOFilename in internal/state/fifo_sweep.go. Both strip "hydrate-" and ".fifo" with the same logic. internal/state already owns FIFOPath (the forward direction) and SanitizePaneKey; the inverse belongs there too so the canonical pair lives together. As written, a future change to FIFOPath's filename shape (extra prefix, sub-directory, format change) requires synchronised edits in two packages — exactly the kind of seam that drifts.
- **Recommendation**: Move the inverse into internal/state/paths.go (alongside FIFOPath) as a single exported PaneKeyFromFIFOPath helper that handles both an absolute path and a bare filename via filepath.Base. Replace both call sites with the shared helper.

### FINDING: migrate-rename hook is registered but functionally inert
- **Severity**: low
- **Files**: `internal/tmux/hooks_register.go:32-67`, `cmd/state_migrate_rename.go:41-81`
- **Description**: RegisterPortalHooks unconditionally registers a session-renamed hook firing `portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'` (old and new the same), and runMigrateRename short-circuits when no keys match the prefix — so the hook does nothing useful, ever, in v1. Yet it is still wired into the global hook table on every bootstrap, fires `command -v portal` and spawns a `portal state migrate-rename` subprocess on every session rename, and adds three constants (migrateRenameEvents / migrateRenameCommand / migrateRenameSubstring) plus a third dedupe-category list in the registration / unregistration plumbing. This is "register a no-op hook now, plan to fix later" — the hook contributes operational cost (per-rename subprocess spawn + tmux hook traversal) for zero functional value, and complicates the registration table for plumbing that is currently unreachable from its actual purpose.
- **Recommendation**: Defer hook registration until the rename-delta source is wired (per spec, daemon-side last-seen-names tracking is post-v1). Drop migrateRenameEvents / migrateRenameCommand / migrateRenameSubstring from RegisterPortalHooks; keep cmd/state_migrate_rename.go as the future-proof endpoint. UnregisterPortalHooks's union of three slices collapses back to two, which also closes the latent bug where a future event added only to migrateRenameEvents would silently leak Portal-owned hooks on cleanup (the deduped union was added precisely because of this risk — remove the risk by removing the category).

### FINDING: internal/hooks/tmux.go is misnamed and contains a stranded interface
- **Severity**: low
- **Files**: `internal/hooks/tmux.go:1-8`
- **Description**: internal/hooks/tmux.go is a 7-line file declaring a single interface (AllPaneLister) that has one consumer (cmd/clean.go) and no implementation in this package. The filename suggests tmux integration but the file does not import internal/tmux and is not used by any code in internal/hooks itself — it exists solely as a parameter shape for cmd-side dependency injection. Stranding an interface in its own file where it is neither implemented nor consumed by the owning package muddies package boundaries — a reader scanning internal/hooks expects every file to be load-bearing for hook persistence.
- **Recommendation**: Move AllPaneLister to cmd/clean.go (the sole consumer), or fold it into internal/hooks/lookup.go alongside LookupOnResume which is the other lookup primitive. Either choice removes a 7-line file whose existence forces a reader to chase across packages to understand why it lives where it does.

### FINDING: Skeleton-marker set/unset scattered across three packages
- **Severity**: low
- **Files**: `internal/state/markers.go:5-15`, `internal/restore/session.go:313-318`, `cmd/state_hydrate.go:189-193,322-325`
- **Description**: state.SkeletonMarkerPrefix is the canonical name for the @portal-skeleton- marker, but three call sites construct the option name by string-concatenating `SkeletonMarkerPrefix + paneKey` and calling client.SetServerOption / UnsetServerOption directly: restore-side set in restore/session.go, hydrate success-path unset in cmd/state_hydrate.go, and hydrate file-missing unset in the same file. The state package owns ListSkeletonMarkers (the read side) and the prefix constant; the write side has no canonical helper, so the convention drifts across three files. A future change to marker convention (server-option → window-option, name format change, value semantics) would have to be hunted through three packages. The asymmetry — read primitive in state, write primitive nowhere — is the architectural smell.
- **Recommendation**: Add SetSkeletonMarker(client, paneKey) and UnsetSkeletonMarker(client, paneKey) helpers in internal/state/markers.go alongside ListSkeletonMarkers (same narrow-seam interface pattern). Replace the three direct set/unset sites with calls into those helpers so the marker convention has a single owner.
