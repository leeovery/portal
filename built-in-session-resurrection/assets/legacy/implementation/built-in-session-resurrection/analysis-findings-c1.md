# Analysis Cycle 1 — Findings Summary

Three analysis agents (architecture, standards, duplication) returned findings via inline summary rather than writing files. Captured here for traceability.

## CRITICAL — production never wires the eight-step Orchestrator

**Files**: `cmd/root.go:95-105, 117-136, 180-213` + `cmd/bootstrap/bootstrap.go:99-189`

The full `bootstrap.Orchestrator` is implemented and integration-tested but `buildBootstrapDeps` returns `ensureServerRunner{client: client}` whose `Run` only calls `client.EnsureServer()` (step 1). RegisterHooks runs as a separate inline call afterwards. Steps 3 (Set @portal-restoring), 4 (EnsureSaver), 5 (Restore), 6 (Clear @portal-restoring), 7 (CleanStale) NEVER execute in production.

Net effect on a real install:
- saver session never created
- hooks fire `portal state notify` but no daemon consumes the dirty flag
- no restoration happens at boot
- no skeleton markers are set
- no orphan FIFOs swept
- CleanStale never runs

The spec's central acceptance criterion ("when a user reboots, their tmux sessions come back") is NOT met by production despite all supporting infrastructure being implemented and tested in isolation.

**Fix**: Construct a real `bootstrap.Orchestrator` in `buildBootstrapDeps` with production adapters: `Server: client`, `Hooks: hookRegistrarAdapter`, `Restoring: restoringMarkerAdapter`, `Saver: saverAdapter` (wrapping `EnsurePortalSaverVersion`), `Restore: &restore.Orchestrator{...}`, `Clean: cleanStaleAdapter`, `Logger`. Drop `ensureServerRunner` and the inline `registerHooks` call.

## MAJOR — `SweepOrphanFIFOs` never invoked

`internal/state.SweepOrphanFIFOs` is implemented and tested but no production caller exists. Spec mandates "additional state-dir scan on bootstrap removes any stale FIFOs not matching a restored pane." Wire between step 5 (Restore) and step 6 (Clear @portal-restoring), passing `liveMarkerKeys = ListSkeletonMarkers(client)` as the keep-set.

## MAJOR — session-renamed migrate-rename hook is structural no-op

`migrateRenameCommand` passes `#{hook_session_name}` twice; runMigrateRename receives identical old/new names; finds keys for `<new>:` prefix; rewrites them to `<new>:` (identity). Hook keys for renamed sessions are silently orphaned. Either implement daemon-side last-name tracking OR remove the hook + scaffolding and document v2 deferral.

## MAJOR — UnregisterPortalHooks event coverage relies on incidental overlap

`portalEvents = saveTriggerEvents ++ hydrationTriggerEvents` omits `migrateRenameEvents`. Works today only because session-renamed lives in both lists. Compute as union of all three.

## MAJOR — paneKey helper duplicated

`paneKeyFromFIFOPath` (cmd/state_hydrate.go) and `paneKeyFromFIFOFilename` (internal/state/fifo_sweep.go). Promote to single `state.PaneKeyFromFIFOPath` next to `state.FIFOPath`.

## MAJOR — Skeleton-marker prefix literal duplicated

Three sites (`cmd/state_hydrate.go:190, :322`, `internal/restore/session.go:314`) build `"@portal-skeleton-" + key` instead of using `state.SkeletonMarkerPrefix`. Future rename would silently break round-trip.

## MAJOR — Pervasive redundant `if Logger != nil` guards

22+ sites wrap `*state.Logger` calls in nil-checks despite the type being documented nil-safe. ~50 lines of dead code. Inconsistent with cleanup/daemon code that already trusts the contract.

## MAJOR — tmuxSocket integration-test harness duplicated

~150 lines of near-identical scaffolding in `internal/restore/integration_test.go` and `cmd/bootstrap/phase5_integration_test.go`. Promote to `internal/tmuxtest`.

## MINOR — restore.RestoreWithMarker duplicates bootstrap orchestrator's marker discipline

Two functions own the same `@portal-restoring` lifecycle at different scopes. Delete `SetRestoring`/`ClearRestoring`/`RestoreWithMarker` from internal/restore once production wires bootstrap orchestrator.

## MINOR — Indices use prediction not post-creation re-query (per spec)

`internal/restore/session.go` predicts FIFO/hydrate paths from `base-index` server option rather than re-querying `list-panes` after creation. Works today under deterministic tmux behavior but contradicts spec mandate.

## MINOR — 1 MB implemented as 1 MiB

`logger.go:43` `LogRotateThreshold = 1 << 20` (1,048,576) vs spec wording "1 MB" (1,000,000). 4.86% inflation; non-load-bearing.

## MINOR — `ensureServerRunner` and `noopRunner` scaffolding

Production-side placeholder types in `cmd/root.go` should disappear once the real orchestrator is wired. `noopRunner` should move to `_test.go`.

## MINOR — `SaverDownError`, `LastSaverErr`, `SaverDownWarning` triple-encode

Three parallel surfaces describe the same EnsureSaver failure event. Delete `LastSaverErr` once production reads warnings slice.

## MINOR — `bootstrap.Logger` interface theater

Already references `state.ComponentBootstrap`; abstraction adds indirection without breaking the dependency. Drop the interface, accept `*state.Logger` directly.

## MINOR — DeleteServerOption vs UnsetServerOption duplication

Both invoke `set-option -su <name>`. Pick one name.

## MINOR — Three near-identical Commander mocks across test packages

Could fold into a shared `internal/tmuxtest` package alongside the socket harness.

## MINOR — xdgConfigBase implemented twice (cmd vs internal/state)

Acknowledged in source comment.

## MINOR — `--purge` ordering vs daemon final flush

No wait between `kill-session` and `os.RemoveAll`. AtomicWrite bounds partial-state loss; not blocking but worth a poll loop.
