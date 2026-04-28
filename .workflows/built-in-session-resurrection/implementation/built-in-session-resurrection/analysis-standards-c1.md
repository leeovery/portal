---
agent: standards
cycle: 1
findings_count: 9
---
# Standards Analysis (Cycle 1)

## Summary

Two high-severity drifts dominate this cycle: (a) README still teaches the deleted send-keys/@portal-active hook firing model on line 177 directly above its replacement, leaving published documentation internally contradictory; (b) the migrate-rename hook is wired end-to-end but passes the new session name twice, so the spec-mandated atomic key migration on `tmux rename-session` is a structural no-op and renamed-session hooks silently orphan. Five medium findings cover paneKey-helper duplication, 26 redundant `if Logger != nil` guards despite the documented nil-safe contract, twin `DeleteServerOption`/`UnsetServerOption` wrappers over `set-option -su`, and the spec-explicit "re-query live indices post-creation" step being implemented as prediction-only — meaning `base-index`/`pane-base-index` drift between save and restore is undefended even though the spec called this out as load-bearing.

---

## Findings

### FINDING: README hooks section still describes the deleted send-keys + @portal-active firing model
- **Severity**: high
- **Files**: `README.md:177`
- **Description**: The spec ("Resume Hook Firing — What Is Deleted from the Previous Design") explicitly removes ExecuteHooks, send-keys hook firing, and the `@portal-active-<pane>` volatile marker. README.md line 177 still tells users: "Hooks fire via tmux send-keys when you attach/open a session. A volatile marker prevents duplicate execution within the same boot cycle — after a reboot the markers are gone and hooks re-fire." That paragraph directly contradicts the very next paragraph (lines 179-184), which correctly describes the new "fires only on reboot recovery via the hydrate helper" model. The "Documentation Deliverables → Existing User-Facing Documentation Updates" spec section mandates: "Hooks documentation … clarify that hooks fire on reboot recovery … Update any examples that assumed the old ExecuteHooks attach-time firing semantics." That update was applied in the new paragraph but the obsolete paragraph was not removed, leaving published guidance internally contradictory.
- **Recommendation**: Delete line 177 entirely. The "When hooks fire:" paragraph below already carries the correct model.

### FINDING: migrate-rename hook is a structural no-op (passes new session name twice)
- **Severity**: high
- **Files**: `internal/tmux/hooks_register.go:48-52`, `cmd/state_migrate_rename.go:41-80`
- **Description**: The spec ("Resume Hook Firing → Session Rename: Hook Key Migration") commits to: "hook keys are migrated atomically on rename events, the migration path is a distinct subcommand (not notify), and best-effort logging on failure." The current `migrateRenameCommand` is `portal state migrate-rename '#{hook_session_name}' '#{hook_session_name}'` — both args expand to the new name. `runMigrateRename` then computes `prefix = oldName + ":"` from arg1 and rewrites matched keys to `newName + ":"` from arg2. Because old == new, the function either matches nothing (newly-renamed sessions still carry the OLD prefix in hooks.json at this moment, but `prefix` is built from the new name so no match) or rewrites keys to themselves. Either way no migration occurs; the hooks.json key stays under the old session name and bootstrap step 7's `CleanStale` then prunes it as orphaned on the next bootstrap. Visible user impact: `portal hooks set --on-resume <cmd>` followed by `tmux rename-session work work-2026` silently loses the hook on the next bootstrap. The source comment in hooks_register.go:51 openly acknowledges this ("the migrate-rename body is a no-op when old == new. Daemon-side rename-delta tracking is post-v1.") — but the spec does not authorise post-v1 deferral; it requires migration "atomically on rename events." Wire-and-scaffolding that satisfies traceability checks but not the contract is worse than a documented gap.
- **Recommendation**: Either (a) implement daemon-side last-name tracking so the hook receives both prior and current session names, or (b) hold a small in-process map keyed by tick-time enumeration of live session names so a rename surfaces as a delta with both names. If neither is implementable in v1, remove the hook + scaffolding entirely and add a v2-deferral note to the spec.

### FINDING: `paneKeyFromFIFO*` helper duplicated across cmd and internal/state
- **Severity**: medium
- **Files**: `cmd/state_hydrate.go:69-77`, `internal/state/fifo_sweep.go:57-64`
- **Description**: Two structurally identical helpers — `paneKeyFromFIFOPath` in `cmd/state_hydrate.go` and `paneKeyFromFIFOFilename` in `internal/state/fifo_sweep.go` — both strip "hydrate-" prefix and ".fifo" suffix from a FIFO basename to recover the canonical paneKey. They form the inverse of `state.FIFOPath`, which lives in `internal/state/paths.go`. The two sites disagree subtly: the cmd version operates on `filepath.Base(fifoPath)` first, while the state version takes the basename directly. The public-facing inverse of `state.FIFOPath` belongs in the same package as `FIFOPath`. Code-quality.md "Compose, Don't Duplicate" applies.
- **Recommendation**: Promote a single `state.PaneKeyFromFIFOPath(path string) string` that wraps `filepath.Base` + the trim chain, drop both private helpers, update `cmd/state_hydrate.go` to call the shared function.

### FINDING: redundant `if Logger != nil` guards across 26 sites despite documented nil-safe contract
- **Severity**: medium
- **Files**: `internal/restore/restore.go` (7), `internal/restore/session.go` (6), `internal/restore/restore_marker.go` (1), `cmd/state_hydrate.go` (5), `cmd/state_signal_hydrate.go` (3), `cmd/bootstrap/bootstrap.go` (4)
- **Description**: `internal/state/logger.go:53-54` documents the contract: "A nil *Logger is a valid no-op: all methods bail early. This lets callers proceed when log opening fails without sprinkling nil checks at call sites." `Debug`/`Info`/`Warn`/`Error` all start with `if l == nil { return }`. Yet 26 production sites still wrap `*state.Logger` calls in defensive `if cfg.Logger != nil` / `if r.Logger != nil` / `if o.Logger != nil` checks. The cleanup-side code (cmd/state_cleanup.go) and the daemon (cmd/state_daemon.go) trust the contract and call methods unconditionally — the codebase is internally inconsistent. Roughly 40-50 lines of dead code; more critically, the inconsistency invites readers to assume the contract is unsafe, propagating the pattern.
- **Recommendation**: Remove the guards in restore.go, session.go, restore_marker.go, state_hydrate.go, state_signal_hydrate.go, and cmd/bootstrap/bootstrap.go. Trust the contract uniformly.

### FINDING: `DeleteServerOption` and `UnsetServerOption` are exact duplicates
- **Severity**: medium
- **Files**: `internal/tmux/tmux.go:440-462`
- **Description**: Both methods invoke `set-option -su <name>` and only differ in error-message format string (`%q` vs `%s`). The doc-comment on `UnsetServerOption` rationalises the duplication: "This method exists alongside DeleteServerOption to provide a Set/Unset-named pair for the @portal-restoring marker coordination." That is justification for two names of the same wire op, not justification for two implementations. Code-quality.md "Compose, Don't Duplicate" and the golang-pro skill's interface-segregation guidance both apply — two names for the same primitive invite call sites to drift over which to use.
- **Recommendation**: Pick one name (Unset reads more naturally with `set-option -u`), make the other a thin alias (or delete it), and migrate any remaining call sites.

### FINDING: index prediction does not re-query post-creation as the spec mandates
- **Severity**: medium
- **Files**: `internal/restore/session.go:114-134`, `internal/restore/session.go:350-368`
- **Description**: Spec ("Index Semantics and base-index / pane-base-index") states explicitly: "On restore, Portal creates windows and panes in saved-structural order, but does not assume the created tmux indices match the saved indices. After creating each window via new-window and each pane via split-window, Portal re-queries `list-panes -t <session>` to map saved-structure position → actual live tmux index. This mapping is used for: setting `@portal-skeleton-<paneKey>` markers on the correct live pane; computing FIFO paths for each live pane; passing `--file <scrollback>` to the correct helper at pane creation time." The implementation `buildPaneInfo` instead **predicts** live indices via `baseIdx + wi` and `paneBaseIdx + pj` from `PredictLiveIndices` (a pair of `show-options` reads), then bakes those predicted indices into the FIFO path and the pane-creation command before tmux is even called. `ApplySkeletonMarkers` later re-queries `list-panes` and warns on drift, but markers are still set against live indices while FIFO paths and the helper's `--file` argument were computed from predictions. Works under deterministic tmux behaviour where prediction == reality, but the spec is load-bearing here: it defends against `base-index`/`pane-base-index` changing between save and restore. Under prediction-only, the daemon's `signal-hydrate` will compute FIFO paths from live indices that drift from the predicted-index FIFO paths the helper is blocked on — silent hydration failure.
- **Recommendation**: After `NewSessionWithCommand`/`NewWindow`/`SplitWindow` for each pane, re-query `list-panes -t <session>:<window>` to obtain the actual live (window,pane) tuple; compute FIFO path and skeleton-marker key from those live indices. Restructure `paneInfo` into a two-phase flow: (1) collect saved-position metadata, (2) walk live `list-panes` output to assign FIFOs and skeleton markers under live indices.

### FINDING: production-side `noopRunner` scaffolding co-located with production code
- **Severity**: low
- **Files**: `cmd/root.go:113-120`
- **Description**: `noopRunner` is referenced only when `bootstrapDeps != nil` (test mode) but lives in the production file `cmd/root.go`. Test-only fallbacks belong in `_test.go` files where they cannot accidentally drift into production paths or mislead readers about real-mode semantics. The c1 cycle's CRITICAL finding (production never wires the eight-step Orchestrator) was resolved with the new `buildProductionOrchestrator`, but the rewrite left `noopRunner` in production code as residue.
- **Recommendation**: Move `noopRunner` to `cmd/bootstrap_orchestrator_test.go` or a new `cmd/root_test_helpers.go`. Production code should reference `bootstrap.Runner` only via a non-nil concrete `*bootstrap.Orchestrator`.

### FINDING: log rotation threshold is 1 MiB while spec says "1 MB per file"
- **Severity**: low
- **Files**: `internal/state/logger.go:43-44`
- **Description**: Spec ("Log Rotation") says: "Simple 2-file cap at 1 MB per file." Implementation defines `LogRotateThreshold = 1 * 1024 * 1024` (1,048,576 bytes = 1 MiB) and the comment openly admits the divergence: "Matches the spec's '1 MB per file' (interpreted as 1 MiB to match the binary growth pattern of log files)." That is reinterpretation of the spec, not implementation of it. The discrepancy is 4.86% (~48KB), non-load-bearing, but it is a unilateral spec edit by the implementer.
- **Recommendation**: Either change to `1_000_000` to match the spec literally, or update the spec to read "1 MiB per file" so future readers do not see a mismatch and assume drift.

### FINDING: `--purge` does not wait for daemon's final flush before `RemoveAll`
- **Severity**: low
- **Files**: `cmd/state_cleanup.go:75-103`, `cmd/state_cleanup.go:128-156`
- **Description**: Spec ("Save-Side Architecture → Signal Handling") promises: tmux closes the PTY, kernel delivers SIGHUP, daemon flushes final state via AtomicWrite, exits. With `--purge`, cleanup runs `killSaver` followed (synchronously, in the same goroutine) by `runPurge` → `os.RemoveAll`. There is no wait between the kill and the rmdir; the daemon's final flush could still be mid-write when `RemoveAll` starts. AtomicWrite atomicity bounds corruption — at most the unflushed-from-memory delta the daemon was about to commit is lost. Acceptable per spec ("Worst-case data loss: whatever accrued since the last dirty-flag check"). The spec does mandate "Partial failures still attempt subsequent actions — cleanup never aborts partway to leave mixed state" which is honoured.
- **Recommendation**: Optional — poll for `daemon.pid`'s process to exit (with a ~500ms cap) before `RemoveAll`. Not required for correctness; would guarantee the final flush lands in the directory before it disappears, nicer story for cleanup-then-reinstall.
