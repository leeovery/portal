---
topic: restore-host-terminal-windows
cycle: 4
total_proposed: 3
---
# Analysis Tasks: restore-host-terminal-windows (Cycle 4)

## Task 1: Cache the burst Adapter at detection time so dispatchBurst cannot re-resolve to a nil adapter and panic
status: pending
severity: medium
sources: architecture

**Problem**: The identity→adapter resolution is produced as a single `(Adapter, Resolution)` pair by the injected `m.resolve` seam, but the `terminalDetectedMsg` arm keeps only the Resolution (`_, m.detectResolution = m.resolve(msg.identity)`, internal/tui/model.go:2456) and discards the Adapter. `dispatchBurst` then calls `m.resolve(m.detectIdentity)` a SECOND time (internal/tui/burst_progress.go:464) to recover an adapter and trusts it non-nil — the doc comment claims it is "guaranteed non-nil on a supported resolution" — without a guard, immediately building `spawn.NewBurster(adapter, …)` (burst_progress.go:465) and launching the burst goroutine.

That guarantee only holds if `resolve` is deterministic between the two calls. It is NOT for the config-*script* recipe path: `newScriptRecipeAdapter` (internal/spawn/configadapter.go:116-128) does a live `os.Stat` of the script on every call. So the gate decision and the adapter derive from two separate resolve calls that are ASSUMED — but not guaranteed — to agree.

Assessment (requested — is this a REAL reachable defect or theoretical?): It is REAL and reachable, though the trigger window is narrow. Trace: (1) page-entry detection resolves a config-script terminal while the script exists+executable → cached `m.detectResolution = ResolutionConfig` (supported); (2) the user deletes the script or clears its exec bit before pressing Enter; (3) `decideBurst` reads the CACHED resolution via `DetectUnsupported()` (spawn_detect.go:113-115 — `m.detectResolution == ResolutionUnsupported`, still false) so the unsupported no-op gate is bypassed and it falls through to `dispatchBurst`; (4) `dispatchBurst`'s fresh `m.resolve` now hits the failing `os.Stat`, `resolveConfig` returns `(nil, false)`, `Resolve` falls through to native (no family match for a config-only identity) → `(nil, ResolutionUnsupported)`, so `adapter == nil`; (5) `spawn.NewBurster(nil, …)` is built and the goroutine's `b.Adapter.OpenWindow(argv)` (internal/spawn/burst.go:162) is invoked on a nil interface. With N≥2 the external slice always has ≥1 element, so `OpenWindow` is always reached. The goroutine is a bare `go func()` with NO recover (`burstProgressPipe.start`, burst_progress.go:104-108), so the nil-interface panic crashes the whole picker rather than degrading to the honest unsupported no-op the rest of the design is careful to reach. Verdict: genuine latent correctness defect worth a task; the argv-recipe and native paths are deterministic and unaffected, so only the config-script + mid-session-deletion combination triggers it.

**Solution**: Cache the Adapter alongside the Resolution from the SINGLE detection-time resolve — add an `m.detectAdapter spawn.Adapter` field set in the `terminalDetectedMsg` arm — and have `dispatchBurst` use the cached adapter instead of re-resolving. This makes the gate decision and the adapter derive from ONE resolve (so the "guaranteed non-nil on a supported resolution" comment becomes true), removes the redundant `os.Stat`, and eliminates the TOCTOU nil-adapter panic. If the script is deleted mid-session the burst then uses the cached (stale) adapter, whose `OpenWindow` fails cleanly through the existing partial-failure path rather than nil-panicking — the correct degradation. Add a defensive nil-guard in `dispatchBurst` as belt-and-braces so a nil adapter (e.g. an un-driven capture-harness model) routes to the unsupported no-op instead of constructing a burster.

**Outcome**: A supported-resolution burst dispatches with the exact adapter that made it supported, resolved once; a mid-session config-script deletion degrades to the honest unsupported no-op / partial-failure flash instead of crashing the picker; the redundant second `os.Stat` is gone.

**Do**:
1. In `internal/tui/model.go` add a `detectAdapter spawn.Adapter` field beside `detectResolution`/`detectResolved` (the §6 detection-cache block around model.go:463-467), documenting that it is cached from the SAME resolve that produced `detectResolution` so the gate and the adapter can never disagree.
2. In the `terminalDetectedMsg` arm (model.go:2455-2457) change `_, m.detectResolution = m.resolve(msg.identity)` to `m.detectAdapter, m.detectResolution = m.resolve(msg.identity)`, so both halves of the single resolve are retained.
3. In `internal/tui/burst_progress.go` `dispatchBurst` (burst_progress.go:460-465) replace `adapter, resolution := m.resolve(m.detectIdentity)` with `adapter, resolution := m.detectAdapter, m.detectResolution` so the burster is built from the detection-time adapter/resolution, not a fresh resolve.
4. Add a defensive guard at the top of `dispatchBurst` (or immediately after reading the cached adapter): if the cached adapter is nil, do NOT construct the burster — return the unsupported no-op path (emit the unsupported outcome + set the unsupported flash, mirroring `decideBurst`'s unsupported branch) so a nil adapter can never reach `spawn.NewBurster`/`OpenWindow`. Keep the existing empty-`ordered` guard in `decideBurst` untouched.
5. Update the `dispatchBurst` doc comment so "resolves the adapter from the cached identity (guaranteed non-nil on a supported resolution)" reflects that the adapter is now READ from the detection-time cache (one resolve), and delete the now-stale claim about a second resolve.
6. Confirm no other reader depends on `dispatchBurst` calling `m.resolve` a second time; `go build ./...` and `go test ./internal/tui/...` green.

**Acceptance Criteria**:
- `m.resolve` is invoked exactly once per detection (in the `terminalDetectedMsg` arm); `dispatchBurst` reads `m.detectAdapter`/`m.detectResolution` and no longer calls `m.resolve`.
- With a config-script terminal whose script is removed between detection and Enter, the N≥2 burst does NOT panic: it either uses the cached adapter (which fails cleanly through the partial-failure path) or, if the cached adapter is nil, routes to the unsupported no-op — never constructs `spawn.NewBurster(nil, …)`.
- The redundant second `os.Stat` on the config-script path is eliminated (one resolve per detection).
- Supported native/argv/config-script bursts behave identically to today for the un-mutated case; the empty-`ordered` no-op and the `DetectUnsupported` gate are unchanged.

**Tests**:
- Unit (tui): a `resolve` seam that returns a non-nil adapter at detection but `(nil, ResolutionUnsupported)` on a hypothetical second call — assert `dispatchBurst` builds the burster from the CACHED adapter (the seam is not called a second time) and never passes nil to the burster; drive a fake burster to confirm no panic.
- Unit (tui): a model whose `detectAdapter` is nil (undriven) reaching `dispatchBurst` routes to the unsupported no-op (no burster constructed) rather than panicking.
- Regression: existing N≥2 burst self-attach, partial-failure, permission, cancel, and deferred-Enter (`pendingBurstEnter`) suites pass unchanged.

## Task 2: Extract one shared production spawn-seam builder for the CLI and picker
status: pending
severity: medium
sources: duplication

**Problem**: The identical set of production spawn dependencies is wired from the shared `*tmux.Client` in two independent places — `cmd/spawn.go`'s `buildSpawnDeps` (the CLI path) and `cmd/open.go`'s `openConfig` population (the picker path). Both construct, seam-for-seam: the detector (`spawn.NewDetector(client)`), the resolver's `Resolve` (`buildResolver().Resolve`), the server-option ack channel (`spawn.NewServerOptionAckChannel(client, client)`), the executable resolver (`os.Executable`), `os.Getenv`, the has-session probe (`client.HasSession`), and the spawn-component logger (`log.For("spawn")`). These seven identical default constructions live at `cmd/spawn.go:274-306` (`buildSpawnDeps`' nil-defaulting) and `cmd/open.go:592-604` (`openConfig`). The `cmd/open.go` comment even states the picker seams "mirror the spawn CLI's SpawnDeps", acknowledging the parallel. It is copy-paste across a task boundary (the CLI command vs the picker-burst wiring): if the ack-channel constructor changes, a seam is added, or a default swaps (e.g. a different logger component), both sites must be edited in lockstep or the CLI and picker silently diverge in how they open windows — the exact drift this analysis targets. The compiler cannot catch it because `SpawnDeps` and `openConfig` are distinct struct shapes. `buildResolver()` and `buildSessionConnector()` already demonstrate the shared-helper pattern in the same package; this closes the remaining un-shared subset.

**Solution**: Add one cmd-package helper that returns the shared production spawn seams from a `*tmux.Client` — e.g. `func buildProductionSpawnSeams(client *tmux.Client) productionSpawnSeams` returning a small struct `{ Detector, Resolve, Ack, Exe, Getenv, Exists, Logger }` built once — and have both `buildSpawnDeps`' nil-defaulting and `cmd/open.go`'s `openConfig` population read their shared fields from it, so the CLI and picker provably wire the same adapters from a single construction site.

**Outcome**: The seven shared production spawn seams are constructed in exactly one place; adding or swapping a seam is a single edit that cannot let the CLI and picker diverge on how windows are opened; the picker-mirrors-CLI comment becomes a code fact rather than a hand-maintained parallel.

**Do**:
1. In the `cmd` package (alongside `buildResolver`/`buildSessionConnector`) add `func buildProductionSpawnSeams(client *tmux.Client)` returning a small struct with the seven shared production seams: `Detector = spawn.NewDetector(client)`, `Resolve = buildResolver().Resolve`, `Ack = spawn.NewServerOptionAckChannel(client, client)`, `Exe = os.Executable`, `Getenv = os.Getenv`, `Exists = client.HasSession`, `Logger = log.For("spawn")`.
2. In `cmd/spawn.go` `buildSpawnDeps` (spawn.go:269-308) build the seams once from `tmuxClient(cmd)` and have each nil-branch read the corresponding field: `Resolve`, `Ack`, `ExePath`, `Getenv`, `Exists`, `Logger` default from the shared struct. Preserve the injected-field precedence exactly (the `*deps = *spawnDeps` copy still wins; the shared builder is consulted ONLY for genuinely-unset fields). Keep `spawnDetector` as the standalone detector authority the `--detect` dry-run uses — either have the shared builder's `Detector` delegate to it, or leave `buildSpawnDeps`' Detector default routing through `spawnDetector` and source only the other six shared seams from the struct. Keep the `Connector` and lazy `NewBurster` defaults (CLI-only, not shared with the picker) exactly as they are.
3. In `cmd/open.go` `openConfig` population (open.go:592-604) build the seams once from the already-resolved `client` and set `detector`, `resolve`, `ackChannel`, `spawnExe`, `spawnGetenv`, `sessionExists`, and `spawnLogger` from the shared struct's fields.
4. Confirm the shared builder resolves the tmux client / `buildResolver()` at most as often as today (no extra client resolution or terminals.json load introduced), and that the CLI's test-injection seam (`spawnDeps`) still overrides every shared field.
5. `go build ./...`, `go test ./cmd/...`, and `go test ./internal/tui/...` green.

**Acceptance Criteria**:
- The seven shared production spawn seams (detector, resolve, ack channel, exe, getenv, exists, spawn logger) are constructed in exactly one helper; both `buildSpawnDeps` and `openConfig` read them from it.
- CLI test injection via `spawnDeps` still overrides every shared field (the shared builder is consulted only for unset fields); the `--detect` dry-run's detector resolution is unchanged.
- The CLI-only `Connector` and lazy `NewBurster` defaults, and the picker-only non-spawn config fields, are unchanged.
- The wired production seams (which detector, resolver, ack channel, exe/getenv, has-session probe, logger component) are byte-for-byte equivalent to today on both paths; `go build ./...` and the cmd + tui suites are green.

**Tests**:
- Unit (cmd): `buildProductionSpawnSeams` over a client produces the expected seams (spawn-component logger, `client.HasSession` probe, server-option ack channel); `buildSpawnDeps` with a partially-injected `spawnDeps` keeps the injected fields and fills only the unset ones from the shared builder.
- Regression: existing `cmd/spawn` deps-defaulting tests and the picker-burst wiring tests pass unchanged; the CLI and picker resolve the same adapter/logger/ack seams (a parity assertion that both paths' shared seams originate from the one builder).

## Task 3: Centralize the net-N split behind a shared spawn.SplitNetN helper
status: pending
severity: low
sources: architecture

**Problem**: `internal/spawn` deliberately centralizes every leaf decision the two callers share — `PreflightMissing`, `PartitionResults`, `FirstPermission`, the `GoneMessage`/`PartialFailureMessage`/`UnsupportedNoopMessage` renderers, and the `Log*`/count-semantics helpers — with the stated design goal that "the two paths cannot drift." The one load-bearing invariant NOT centralized is the net-N split itself: `cmd/spawn.go:120` computes `external := sessions[:n-1]; trigger := sessions[n-1]` and `internal/tui/burst_progress.go:461-462` independently computes `trigger := ordered[len-1]; external := ordered[:len-1]`. The spec elevates "net N windows, never N+1" to a hard anti-requirement, yet its mechanical realization is the sole shared invariant left duplicated across the two callers. It is trivially correct today (two slice expressions), so the risk is low — but it is conspicuously the one seam the shared-primitive philosophy skipped, sitting next to the related `Log*`/`Partition*` helpers that are already shared.

**Solution**: Add a small pure helper in `internal/spawn` — `func SplitNetN(ordered []string) (external []string, trigger string)` returning `ordered[:len-1]` / `ordered[len-1]` — and have both `runSpawn` (cmd/spawn.go) and `dispatchBurst` (tui/burst_progress.go) derive the split through it, so the net-N invariant lives with its sibling shared primitives and cannot drift. The control-flow-ordering parallelism between the sync CLI and async picker is left as-is (it is genuine sync-vs-async divergence, not copy-pasted logic); only the split is unified.

**Outcome**: The "net N windows, never N+1" split is computed in one place both callers call; the trigger/external partition cannot silently diverge between the CLI and picker; the helper sits with `PartitionResults`/`PreflightMissing` where the shared-primitive philosophy already lives.

**Do**:
1. In `internal/spawn` (alongside `PartitionResults`/`PreflightMissing`) add `func SplitNetN(ordered []string) (external []string, trigger string)` returning `ordered[:len(ordered)-1]`, `ordered[len(ordered)-1]`. Document the precondition (callers guarantee `len(ordered) >= 1` — the CLI's N≥2 spawn path and the picker's empty-set guard both ensure it) and that `external` is the N-1 opened windows while `trigger` is the self-attach target (the last row).
2. In `cmd/spawn.go` (~line 120) replace the inline `external := sessions[:n-1]; trigger := sessions[n-1]` with `external, trigger := spawn.SplitNetN(sessions)`, preserving the surrounding pre-flight / count-semantics logic unchanged.
3. In `internal/tui/burst_progress.go` `dispatchBurst` (~461-462) replace `trigger := ordered[len(ordered)-1]; external := ordered[:len(ordered)-1]` with `external, trigger := spawn.SplitNetN(ordered)`, keeping the existing empty-`ordered` guard in `decideBurst` (which prevents a zero-length slice from reaching `dispatchBurst`) unchanged.
4. `go build ./...`, `go test ./internal/spawn/...`, `go test ./cmd/...`, `go test ./internal/tui/...` green.

**Acceptance Criteria**:
- `spawn.SplitNetN` is the single computation of the net-N (external/trigger) split; neither `runSpawn` nor `dispatchBurst` hand-rolls the slice expressions.
- For any `ordered` with `len >= 1`, `external` is the leading N-1 and `trigger` is the last element — byte-identical to the two prior inline computations.
- The empty-set / N=1 guards on both callers are preserved; no caller passes a zero-length slice into `SplitNetN`.
- CLI and picker net-N behaviour (never N+1) is unchanged; the sync/async control-flow ordering is untouched.

**Tests**:
- Unit (spawn): `SplitNetN` over a 2-element, a 3-element, and a single-element slice returns the expected `external`/`trigger` (single element → empty external + that element as trigger).
- Regression: existing CLI spawn net-N and picker-burst dispatch suites pass unchanged; a cross-caller assertion that both paths derive the same external/trigger split from `SplitNetN` for a shared fixture.
