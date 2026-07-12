---
status: complete
created: 2026-07-12
cycle: 1
phase: Plan Integrity Review
topic: restore-host-terminal-windows
---

# Review Tracking: restore-host-terminal-windows - Integrity

Reviewed the full plan (planning.md) and all six per-phase task bodies
(phase-1-tasks.md … phase-6-tasks.md), end to end, with particular attention to
cross-phase integration soundness (types/seams a phase DEFINES vs a later phase
CONSUMES) and the consistency of the two callers of the shared spawn service (the
`portal spawn` CLI and the in-process picker). Pre-existing symbols the plan builds
on (`buildSessionConnector`/`SessionConnector`, `AttachDeps`/`buildAttachDeps`,
`configFilePath(envVar, filename) (string, error)`, `session.NewNanoIDGenerator` →
`IDGenerator = func() (string,error)`, `ShowAllServerOptions`/`SetServerOption`/
`UnsetServerOption`/`HasSession`/`CurrentSessionName`, `ListSkeletonMarkers`,
`SkeletonMarkerPrefix`, `resolver.ExpandTilde`, `log.CombinedOutputWithContext`,
`buildTUIModel`/`tui.Build`/`WithInitialFlash`, `processTUIResult`/
`evaluateDefaultPage`, theme tokens `AccentViolet`/`AccentBlue`/`AccentOrange`/
`StateRed`/`TextDetail`, `flashWarningGlyph`, `tmux.InsideTmux`) were all verified to
exist with the signatures the tasks assume. The plan is structurally strong; the
findings below are cross-phase wiring/consistency gaps an implementer would hit.

## Findings

### 1. Picker silently ignores `terminals.json` — config-resolver divergence from the CLI

**Severity**: Critical
**Plan Reference**: Phase 6, task restore-host-terminal-windows-6-3 (cross-refs Phase 4 task restore-host-terminal-windows-4-6)
**Category**: Dependencies and Ordering / Task Self-Containment (cross-phase integration: two callers of one service must resolve identically)
**Change Type**: update-task

**Details**:
The two callers of the shared spawn service resolve adapters **differently**, which
defeats Phase 4's entire purpose for the picker.

- The CLI wires its resolver config-aware (task 4-6, `cmd/spawn.go`):
  `path,_ := configFilePath("PORTAL_TERMINALS_FILE","terminals.json")` →
  `cfg := spawn.NewTerminalsStore(path).Load()` → `resolver := spawn.NewResolver(cfg)`
  → default `Resolve` seam = `resolver.Resolve` (reads `terminals.json`).
- The picker (task 6-3) wires its default `Resolve` seam to **`spawn.ResolveAdapter`**.
  Task 4-6 explicitly defines `ResolveAdapter` as the **zero-config wrapper**:
  `func ResolveAdapter(id Identity)(Adapter,Resolution){ return NewResolver(TerminalsConfig{}).Resolve(id) }`
  — i.e. it passes an **empty** config and never reads `terminals.json`.

Net effect: a custom/unknown terminal configured via the `terminals.json` escape
hatch (argv or script recipe) works with `portal spawn <sessions…>` but is silently
ignored by the in-picker multi-select N≥2 burst — the burst falls straight through to
native→unsupported. That directly contradicts planning.md's Phase 4 rationale ("It
comes before the picker so the picker's resolution is complete when the burst wires
in") and the spec's "one service, two callers" model. Task 6-3's own Context bullet
mis-states this ("spawn.ResolveAdapter (Task 2.2 + config Task 4.6)"), betraying the
author's belief that `ResolveAdapter` is config-aware — it is not.

Fix: the picker's default `Resolve` seam must build the same config-aware resolver as
the CLI, wired in `cmd/open.go` where the other spawn seams (`Detector`, `AckChannel`)
are constructed, degrading to an empty config on a `configFilePath` error exactly like
the CLI (task 4-6).

**Current** (task 6-3, **Do** — the spawn-seam injection bullet):
- Inject the spawn seams into the model via `tui.Deps` + `With*` options (nil-tolerant; nil in the harness): `Resolve func(spawn.Identity)(spawn.Adapter,spawn.Resolution)` (default `spawn.ResolveAdapter`), `SessionExists func(string)bool` (default `client.HasSession`), `AckChannel spawn.AckChannelFull` (default `spawn.NewServerOptionAckChannel(client,client)`), `SpawnExe spawn.ExecutableResolver` (default `os.Executable`), `SpawnGetenv func(string)string` (default `os.Getenv`). Wire all in `cmd/open.go` `buildTUIModel`/`tuiConfig`. Packaging (individual fields vs a `SpawnDeps`-style struct) is an implementation choice — mirror `cmd`'s `SpawnDeps` shape for consistency.

**Proposed** (task 6-3, **Do** — replace that bullet; plus the Context correction and a new AC below):
- Inject the spawn seams into the model via `tui.Deps` + `With*` options (nil-tolerant; nil in the harness): `Resolve func(spawn.Identity)(spawn.Adapter,spawn.Resolution)`, `SessionExists func(string)bool` (default `client.HasSession`), `AckChannel spawn.AckChannelFull` (default `spawn.NewServerOptionAckChannel(client,client)`), `SpawnExe spawn.ExecutableResolver` (default `os.Executable`), `SpawnGetenv func(string)string` (default `os.Getenv`). Wire all in `cmd/open.go` `buildTUIModel`/`tuiConfig`. **The default `Resolve` seam must be the config-aware resolver — the SAME one the CLI builds in task 4-6, not the zero-config `spawn.ResolveAdapter`** — so the in-picker burst honours `terminals.json` identically to `portal spawn`: in `cmd/open.go`, resolve `path, err := configFilePath("PORTAL_TERMINALS_FILE", "terminals.json")`; on `err` degrade to an **empty** config (native-only, fail-safe, exactly as the CLI does); then `cfg := spawn.NewTerminalsStore(path).Load()`, `resolver := spawn.NewResolver(cfg)`, and set the default `Resolve` to `resolver.Resolve`. (Load `terminals.json` once at TUI construction, alongside `Detector`.) Packaging (individual fields vs a `SpawnDeps`-style struct) is an implementation choice — mirror `cmd`'s `SpawnDeps` shape for consistency.

Also in task 6-3, **Context** — replace the mis-stating clause:

  Current: "The picker reuses `spawn.PreflightMissing` (Task 3.4), `spawn.ResolveAdapter` (Task 2.2 + config Task 4.6), the `spawn.Burster`, and `spawn.AckChannelFull.Clean` (Task 3.2) — the same pieces `cmd/spawn.go`'s `runSpawn` composes"

  Proposed: "The picker reuses `spawn.PreflightMissing` (Task 3.4), the **config-aware** `spawn.NewResolver(cfg).Resolve` (native + `terminals.json` tier, Task 4.6 — NOT the zero-config `spawn.ResolveAdapter` wrapper, which never reads config), the `spawn.Burster`, and `spawn.AckChannelFull.Clean` (Task 3.2) — the same pieces `cmd/spawn.go`'s `runSpawn` composes, so a `terminals.json` recipe resolves identically in the picker and the CLI"

Also in task 6-3, **Acceptance Criteria** — add:
- [ ] The picker's default `Resolve` seam is the config-aware `spawn.NewResolver(terminals.json).Resolve` (built once in `cmd/open.go`, degrading to empty config on a `configFilePath` error), so an identity matching a valid `terminals.json` entry resolves to the config adapter + `ResolutionConfig` in the picker burst — identical to `portal spawn` (a regression test injects a config-matched `Resolve` and asserts the config adapter is used).

And add a corresponding **Test** to task 6-3:
- `"it resolves the burst adapter through the config-aware terminals.json resolver, matching the CLI"`

**Resolution**: Fixed
**Notes**:

---

### 2. `Opening n/N…` denominator is inconsistent (N vs N−1) — band overwrites the total and contradicts the capture fixture

**Severity**: Important
**Plan Reference**: Phase 6, tasks restore-host-terminal-windows-6-5 (render), restore-host-terminal-windows-6-3 (progress total), restore-host-terminal-windows-6-11 (fixture)
**Category**: Acceptance Criteria Quality / intra-phase consistency (a counter rendered one way in one task, differently in another)
**Change Type**: update-task

**Details**:
Three Phase-6 tasks disagree on the `Opening n/N…` denominator:

- Task 6-3 sets `m.burstTotal = len(ordered)` = **N** at dispatch (N = all marked
  sessions, incl. the trigger self-attach target), and extends `Burster.Run` to call
  `progress(i+1, len(external))` — a progress `total` of **len(external) = N−1**.
- Task 6-5's `case spawnProgressMsg:` arm then does `m.burstTotal = msg.Total`,
  **overwriting** the dispatch-time N with N−1 on the first progress message. So the
  rendered denominator flips from `Opening 0/N…` to `Opening n/(N−1)…` mid-burst.
- Task 6-11 seeds the capture fixture `initialBurstOpening = {2,3}` → `Opening 2/3…`
  for a **3**-session batch (external = 2). With the overwrite, the real runtime code
  would render `Opening 1/2…`, `Opening 2/2…` — never `2/3` — so the committed fixture
  cannot be reproduced and would fail the 6-11 visual gate.
- The `spawn` log summary (task 6-10) fixes `total := len(m.burstExternal) + 1` = **N**
  and `opened N/N` on full success — so the log uses N while the band would use N−1.

The intended, self-consistent behaviour (matching the spec's `Opening n/N…` /
`opened 11/14`, the fixture's `Opening 2/3`, and the "no N/N ✓ nag"): the denominator
is **N** (kept from dispatch), and `burstDone` advances 0…N−1 as external windows
confirm — the last frame before the silent self-attach reads `Opening N−1/N…` (never
`N/N`). The bug is task 6-5 overwriting `burstTotal` from `msg.Total`.

**Current** (task 6-5, **Do** — the progress-fold bullet):
- Fold progress into the counter: in the `case spawnProgressMsg:` arm, `m.burstDone = msg.Done; m.burstTotal = msg.Total; return m, m.burstPipe.receiver()` (re-issue the receiver to pull the next event — the standard single-blocking-receive loop from `bootstrap_progress.go`).

**Proposed** (task 6-5, **Do** — replace that bullet):
- Fold progress into the counter: in the `case spawnProgressMsg:` arm, advance **only** the done count — `m.burstDone = msg.Done; return m, m.burstPipe.receiver()` (re-issue the receiver to pull the next event — the standard single-blocking-receive loop from `bootstrap_progress.go`). Do **not** overwrite `m.burstTotal` from `msg.Total`: the denominator is fixed at dispatch to `len(ordered)` = **N** (all marked sessions incl. the trigger self-attach target, set in 6-3), matching the `opened N/N` log summary (6-10) and the `Opening 2/3…` capture fixture (6-11). `msg.Total` from `Burster.Run` is `len(external)` = N−1 (the per-window progress total) and is intentionally ignored for the denominator; `burstDone` advances 0…N−1 as external windows confirm, so the band's final frame before the silent self-attach reads `Opening N−1/N…` (never `N/N` — consistent with "no '14/14 ✓' nag").

Also in task 6-5, **Acceptance Criteria** — replace:

  Current: "[ ] Each `spawnProgressMsg{Done,Total}` advances `BurstDone()`/`BurstTotal()`, and the section-header row renders `Opening <done>/<total>…`."

  Proposed: "[ ] Each `spawnProgressMsg` advances `BurstDone()` only; `BurstTotal()` stays at the dispatch-time N (= marked-set size, incl. the trigger). The section-header row renders `Opening <burstDone>/<N>…`, so a 3-session batch renders `Opening 1/3…` then `Opening 2/3…` (never `2/2`), and never reaches `3/3` (the trigger self-attaches silently)."

And add a **Test** to task 6-5:
- `"it holds the Opening denominator at N (marked-set size) across progress messages"`

**Resolution**: Fixed
**Notes**:

---

### 3. `spawn.Burster.Run` signature change (Phase 6) does not list the required CLI call-site update

**Severity**: Minor
**Plan Reference**: Phase 6, task restore-host-terminal-windows-6-3 (modifies Phase 3 task restore-host-terminal-windows-3-5's `cmd/spawn.go` call site)
**Category**: Task Self-Containment / Dependencies and Ordering (cross-phase modification of already-approved code)
**Change Type**: add-to-task

**Details**:
Task 6-3 changes the approved Phase-3 `spawn.Burster.Run(external)` (task 3-5) to
`Run(ctx context.Context, external []string, progress func(done, total int))`. The
CLI's `runSpawn` (task 3-5, `cmd/spawn.go`) calls `burster.Run(external)`. Changing a
function signature breaks every caller in the same commit, but task 6-3's Do steps
only touch `internal/tui` and `internal/spawn` — they never state that `cmd/spawn.go`'s
call site must be updated to the new signature. The intent is buried in a Context
parenthetical ("the CLI passes `context.Background()` + `nil` progress, preserving
Phase-2/3 behaviour"). An implementer working the Do list would ship a broken build
(caught by the compiler, hence Minor), but the task detail should make the required
edit explicit so the change is self-contained.

**Current** (task 6-3, **Do** — the `Burster.Run` extension bullet):
- Extend `spawn.Burster.Run` to `Run(ctx context.Context, external []string, progress func(done, total int)) (batch string, results []spawn.WindowResult, err error)` — an **additive** integration seam: call `progress(i+1, len(external))` after each window's ack classification (nil-tolerant, like restore's `Progress`), and check `ctx.Err()` between windows and inside the ack poll to abandon remaining spawns on cancellation (6-8 drives the cancel; the CLI passes `context.Background()` + `nil` progress, preserving Phase-2/3 behaviour). Document the callback + ctx like restore's `Progress func(n, m int)`.

**Proposed** (task 6-3, **Do** — replace that bullet):
- Extend `spawn.Burster.Run` to `Run(ctx context.Context, external []string, progress func(done, total int)) (batch string, results []spawn.WindowResult, err error)` — an **additive** integration seam: call `progress(i+1, len(external))` after each window's ack classification (nil-tolerant, like restore's `Progress`), and check `ctx.Err()` between windows and inside the ack poll to abandon remaining spawns on cancellation (6-8 drives the cancel). Document the callback + ctx like restore's `Progress func(n, m int)`. **Because this changes the signature of an approved Phase-3 seam, update its existing call site in the same change: in `cmd/spawn.go` `runSpawn`, change `burster.Run(external)` to `burster.Run(context.Background(), external, nil)` — the nil progress + `context.Background()` preserve the exact Phase-2/3 CLI behaviour (no progress streaming, no cancellation).** Any Phase-3 `internal/spawn/burst_test.go` call sites that invoke `Run(external)` are updated the same way.

**Resolution**: Fixed
**Notes**:

---

### 4. `spawn.AckChannelFull` interface is consumed by two phases but never explicitly declared

**Severity**: Minor
**Plan Reference**: Phase 3, task restore-host-terminal-windows-3-2 (home of the ack interfaces); consumed by restore-host-terminal-windows-3-5 and restore-host-terminal-windows-6-3
**Category**: Task Self-Containment (a referenced type is never declared by any task)
**Change Type**: add-to-task

**Details**:
`spawn.AckChannelFull` is referenced as a type in task 3-5 (`SpawnDeps.Ack spawn.AckChannelFull`)
and task 6-3 (`tui.Deps.AckChannel spawn.AckChannelFull`, `spawn.AckChannelFull.Clean`),
but no task's Do section declares it. Task 3-2 declares the sibling consumer interfaces
`AckCollector`, `AckCleaner`, `AckWriter` and their satisfier `*ServerOptionAckChannel`,
yet stops short of the combined `Collect`+`Clean` interface both later tasks name. The
type is inferable, but leaving a cross-phase-consumed interface undeclared forces the
implementer to invent its exact shape/location — pin it where the other ack interfaces
live so both consumers reference one canonical declaration.

**Current** (task 3-2, **Do** — the consumer-interfaces bullet):
- Define the narrow **consumer** interfaces where they are used (Go idiom), or export them here for reuse: `type AckCollector interface { Collect(batch string) (map[string]struct{}, error) }`, `type AckCleaner interface { Clean(batch string) error }`, `type AckWriter interface { Write(batch, token string) error }`. `*ServerOptionAckChannel` satisfies all three.

**Proposed** (task 3-2, **Do** — replace that bullet):
- Define the narrow **consumer** interfaces where they are used (Go idiom), or export them here for reuse: `type AckCollector interface { Collect(batch string) (map[string]struct{}, error) }`, `type AckCleaner interface { Clean(batch string) error }`, `type AckWriter interface { Write(batch, token string) error }`, and the combined `type AckChannelFull interface { AckCollector; AckCleaner }` — the `Collect`+`Clean` seam the burst orchestrators depend on (`SpawnDeps.Ack` in task 3-5 and `tui.Deps.AckChannel` in task 6-3 both reference `spawn.AckChannelFull`). `*ServerOptionAckChannel` satisfies all four; `spawntest.FakeAckChannel` satisfies `AckChannelFull`.

**Resolution**: Fixed
**Notes**:

---
