---
status: in-progress
created: 2026-07-12
cycle: 2
phase: Plan Integrity Review
topic: restore-host-terminal-windows
---

# Review Tracking: restore-host-terminal-windows - Integrity

Cycle-2 follow-up. Re-read the full plan (planning.md) and all six per-phase task
bodies (phase-1-tasks.md … phase-6-tasks.md) end to end, as a fresh full pass, with
particular attention to (a) whether the four cycle-1 fixes landed and stayed internally
consistent without introducing new gaps, and (b) cross-phase define→consume soundness
(types/signatures/seams/markers/flags/log attrs), every "deferred to phase N" promise
having a real task, any referenced-but-undeclared type, and the two callers of the
shared spawn service (the `portal spawn` CLI and the in-process picker) resolving and
behaving identically.

## Cycle-1 fixes — verified coherent

All four cycle-1 findings are present and internally consistent, and none introduced a
new inconsistency:

1. **Config-resolver divergence (Critical)** — task 6-3 now wires the picker's default
   `Resolve` seam to the **config-aware** `spawn.NewResolver(terminals.json).Load→Resolve`
   built in `cmd/open.go` (degrading to empty config on a `configFilePath` error), the
   Context clause is corrected, and the AC + test were added. This agrees with task 4-6
   (same `NewTerminalsStore(path).Load()` → `NewResolver(cfg)` → `resolver.Resolve`, same
   fail-safe degradation, same `PORTAL_TERMINALS_FILE` env key).
2. **`Opening n/N…` denominator (Important)** — task 6-5 no longer overwrites
   `m.burstTotal` from `msg.Total`; the denominator is held at the dispatch-time N
   (`len(ordered)`, set in 6-3), `burstDone` advances 0…N−1, the AC + test were updated.
   This agrees with 6-3 (`m.burstTotal = len(ordered)` = N; `progress(i+1, len(external))`
   supplies an intentionally-ignored N−1), 6-10 (`total := len(m.burstExternal)+1` = N,
   `opened N/N`), and the 6-11 fixture (`initialBurstOpening = {2,3}` = `Opening 2/3…`,
   now reproducible for a 3-session batch).
3. **`Burster.Run` signature change (Minor)** — task 6-3 now explicitly lists the
   `cmd/spawn.go` call-site update (`burster.Run(external)` →
   `burster.Run(context.Background(), external, nil)`) plus the Phase-3 `burst_test.go`
   sites, so the signature change is self-contained.
4. **`AckChannelFull` undeclared (Minor)** — task 3-2 now declares
   `type AckChannelFull interface { AckCollector; AckCleaner }` and notes both consumers
   (`SpawnDeps.Ack` in 3-5, `tui.Deps.AckChannel` in 6-3) plus its satisfiers
   (`*ServerOptionAckChannel`, `spawntest.FakeAckChannel`). Matches every downstream use.

The fresh pass surfaced one new cross-phase integrity defect (below) that cycle 1 did
not catch. It is the same class of issue as cycle-1 Finding 1 — a CLI-vs-picker
divergence in how the shared spawn service is consumed — but on the *resolution
classification* axis rather than the config axis.

## Findings

### 1. Picker gates the unsupported banner + N≥2 no-op on `IsNull()`, but "unsupported" is a *resolution* verdict — a non-NULL undriven terminal (Apple Terminal, the design's own example) is mis-classified

**Severity**: Critical
**Plan Reference**: Phase 6, tasks restore-host-terminal-windows-6-1 (caches identity but not resolution), restore-host-terminal-windows-6-2 (proactive banner gate), restore-host-terminal-windows-6-3 (N≥2 dispatch branch), restore-host-terminal-windows-6-9 (N≥2 unsupported no-op), restore-host-terminal-windows-6-11 (capture fixture). Cross-refs Phase 1 task 1-1 / Phase 2 tasks 2-2 & 2-7 (CLI parity).
**Category**: Dependencies and Ordering / Task Self-Containment (cross-phase integration: the two callers of one service must classify "unsupported" identically) + Acceptance-Criteria Quality (an AC contradicts its own task's Do)
**Change Type**: update-task

**Details**:
The picker decides "is this terminal unsupported?" with `m.detectIdentity.IsNull()`,
but **NULL and unsupported are not the same set**. Detection (Phase 1) returns the
terminal's *actual* macOS bundle id; only remote/mosh resolves to NULL. A **local but
undriven** terminal resolves to a **non-NULL** identity that the *resolver* (Phase 2/4)
maps to `ResolutionUnsupported`:

- Task 1-1 AC: `NewIdentity("com.example.MyTerm", "")` (an unknown bundle id) "returns a
  non-NULL passthrough identity … never NULL."
- Task 2-2 AC: `NewIdentity("com.apple.Terminal", "Apple Terminal")` "returns
  `(nil, ResolutionUnsupported)`" — i.e. **non-NULL yet unsupported**.

So "unsupported" is a *resolution* verdict (`resolution == ResolutionUnsupported`,
covering NULL **and** non-NULL-undriven), not `IsNull()` (which covers only NULL). The
CLI gets this right: task 2-7 gates its atomic N≥2 no-op on
`resolution == spawn.ResolutionUnsupported` and only branches the *message wording* on
`IsNull()` ("no host-local terminal" vs "unsupported terminal — name · bundle id"). The
picker uses `IsNull()` as the whole test, which breaks three ways:

1. **The proactive banner never shows for the design's own example.** Task 6-2 gates the
   banner on `m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode`. The
   delivered frame (`design/sessions-unsupported-terminal.png`) and 6-2's own copy show
   `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` — a **non-NULL**
   identity for which `IsNull()` is **false**, so the banner would never render. Worse,
   the only case the `IsNull()` gate *does* fire (a NULL remote/mosh identity) has empty
   `Name`/`BundleID`, so `renderUnsupportedHeader(name, bundleID, …)` would render
   `⚠ unsupported terminal —  · ` (blanks) — the wrong copy for NULL (which should be the
   honest "no host-local terminal" line). The gate fires exactly when the copy is wrong
   and stays silent exactly when it is right.
2. **Task 6-2 AC #1 contradicts task 6-2's Do.** AC #1 requires the row to render
   `⚠ unsupported terminal — <name> · <bundleID>` (a *named*, therefore non-NULL,
   identity), which the `IsNull()`-gated Do can never produce. The task cannot satisfy
   its own acceptance criterion.
3. **The N≥2 Enter branch has an undefined gap and can dispatch a nil adapter.** Task
   6-9's branch conditions are non-complementary: "supported = `!IsNull()` **and**
   `resolve(...)` non-unsupported" vs "unsupported/NULL = `IsNull()`". A non-NULL
   undriven identity (`!IsNull() && resolve == unsupported`, e.g. Apple Terminal or any
   unknown terminal with no native/config adapter) matches **neither** branch. Task 6-3's
   simpler dispatch gate ("resolved supported → dispatch; resolved NULL → 6-9 no-op",
   branching on `IsNull()`) would treat such an identity as *supported* and dispatch the
   burst, then `adapter, resolution := m.resolve(m.detectIdentity)` returns a **nil**
   adapter → `Burster.Run` calls `OpenWindow` on nil → panic.
4. **The 6-11 capture fixture is internally contradictory and un-renderable.** The
   `WithInitialDetection` seed and `sessionsUnsupportedTerminalFixture()` seed
   `Name:"Apple Terminal", BundleID:"com.apple.Terminal"` while calling it "a NULL
   identity for the unsupported frame" — but an identity carrying a bundle id is **not**
   NULL (`IsNull() == BundleID == ""`). With the `IsNull()`-gated banner, this fixture
   would render the standard `Sessions ··· N` header, **not** the banner — so the capture
   could never match `testdata/vhs/reference/sessions-unsupported-terminal-mv.png` and
   the 6-11 visual gate would fail.

Fix (one root cause): the picker must classify on **resolution**, not `IsNull()`. Cache
the resolution alongside the identity in 6-1 (resolution is a pure function of the
identity + the once-loaded `terminals.json`, so resolve-and-cache when detection lands),
expose a single `DetectUnsupported()` predicate, gate the 6-2 banner and the 6-3/6-9 N≥2
branch on that predicate, and branch only the *message/copy* on `IsNull()` (honest
"no host-local terminal" for NULL; "unsupported terminal — name · bundle id" for a named
identity) — exactly mirroring CLI task 2-7. This restores CLI↔picker parity and makes
the design's Apple Terminal banner (and the 6-11 fixture) reachable.

Note: this makes 6-1 consume the config-aware `Resolve` seam (a `Model` field wired in
`cmd/open.go`; the same seam task 6-3 injects, corrected by cycle-1 Finding 1). Ensure
that field + its `cmd/open.go` wiring are in place by 6-1 so the resolution can be cached
when the `terminalDetectedMsg` lands.

**Current** (task 6-1, **Do** — the model-fields bullet):
- In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `detectIdentity spawn.Identity`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`.

**Proposed** (task 6-1, **Do** — replace that bullet):
- In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `detectIdentity spawn.Identity`, `detectResolution spawn.Resolution`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`, and `func (m Model) DetectUnsupported() bool { return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported }` — the single "this terminal cannot spawn host windows" predicate, **true for a NULL remote/mosh identity AND a non-NULL recognised-but-undriven identity (e.g. Apple Terminal → `com.apple.Terminal`)**. This is the resolution-based unsupported test the proactive banner (6-2) and the N≥2 gate (6-3/6-9) share; `IsNull()` alone is NOT the unsupported test (a non-NULL undriven identity is unsupported by *resolution*, not by NULL-ness — same as CLI task 2-7).

**Current** (task 6-1, **Do** — the `terminalDetectedMsg` arm bullet):
- Define `type terminalDetectedMsg struct { identity spawn.Identity }` and add an Update arm: `case terminalDetectedMsg: m.detectIdentity = msg.identity; m.detectResolved = true; return m, nil` (6-9 later extends this arm to resolve a deferred N≥2 Enter).

**Proposed** (task 6-1, **Do** — replace that bullet):
- Define `type terminalDetectedMsg struct { identity spawn.Identity }` and add an Update arm that caches the identity **and its resolution** (resolution is a pure function of the identity + the once-loaded `terminals.json`, so resolving here caches it for the banner and the Enter gate — no re-walk, no re-load): `case terminalDetectedMsg: m.detectIdentity = msg.identity; _, m.detectResolution = m.resolve(msg.identity); m.detectResolved = true; return m, nil` — where `m.resolve` is the config-aware `Resolve` seam wired in `cmd/open.go` (the same `Model` field task 6-3 injects, per cycle-1 Finding 1; ensure the field + wiring exist by this task). Caching `detectResolution` (not just `IsNull()`) is load-bearing: a recognised-but-undriven terminal is non-NULL yet resolves `unsupported`, so the banner (6-2) and the N≥2 no-op (6-9) key on `DetectUnsupported()`, not on `IsNull()`. (6-9 later extends this arm to resolve a deferred N≥2 Enter against the cached resolution.)

**Current** (task 6-2, **Do** — step 3 of the `applySectionHeader` precedence insert):
  3. **new:** `m.detectResolved && m.detectIdentity.IsNull()` → `renderUnsupportedHeader(m.detectIdentity.Name, m.detectIdentity.BundleID, m.contentWidth(), m.canvasMode, m.colourless)` (first-line replacement, same mechanism as the FilterApplied query header). Gated on `detectResolved` so an **in-flight** identity shows the standard header, not the banner.

**Proposed** (task 6-2, **Do** — replace step 3):
  3. **new:** `m.DetectUnsupported()` (resolved AND `detectResolution == ResolutionUnsupported` — covers a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal, per 6-1) → `renderUnsupportedHeader(m.detectIdentity.Name, m.detectIdentity.BundleID, m.contentWidth(), m.canvasMode, m.colourless)` (first-line replacement, same mechanism as the FilterApplied query header), where `renderUnsupportedHeader` renders the honest `⚠ no host-local terminal` line when `m.detectIdentity.IsNull()` (empty Name/BundleID, remote/mosh) and the `⚠ unsupported terminal — <name> · <bundleID>` line for a named identity — the same copy-branch as CLI task 2-7. Gated on `detectResolved` (via `DetectUnsupported()`) so an **in-flight** identity shows the standard header, not the banner. Do **not** gate on `m.detectIdentity.IsNull()`: that hides the banner for the design's own Apple Terminal example (a non-NULL identity) and fires only for NULL — where the "unsupported terminal — · " copy is wrong.

**Current** (task 6-2, **Do** — the `activeNoticeBand` gating bullet, `unsupportedBannerActive` helper):
- In `internal/tui/notice_band.go` `activeNoticeBand`, gate the `byTagSignpost` arm off when the unsupported banner owns the section-header row: `if m.byTagSignpost && !m.multiSelectMode && !m.unsupportedBannerActive() { return bandInfo, byTagSignpostText, true }` — add a small `func (m Model) unsupportedBannerActive() bool { return m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode }` helper (also consumed by `applySectionHeader` step 3 so the two reads can't drift). The transient-flash arm stays first (unchanged).

**Proposed** (task 6-2, **Do** — replace that bullet):
- In `internal/tui/notice_band.go` `activeNoticeBand`, gate the `byTagSignpost` arm off when the unsupported banner owns the section-header row: `if m.byTagSignpost && !m.multiSelectMode && !m.unsupportedBannerActive() { return bandInfo, byTagSignpostText, true }` — add a small `func (m Model) unsupportedBannerActive() bool { return m.DetectUnsupported() && !m.multiSelectMode }` helper (also consumed by `applySectionHeader` step 3 so the two reads can't drift). `DetectUnsupported()` (6-1) is the resolution-based test — true for a NULL remote/mosh identity and a non-NULL undriven identity alike — **not** `IsNull()`. The transient-flash arm stays first (unchanged).

**Current** (task 6-3, **Do** — the "Detection gate" bullet of the rewired N≥2 branch):
  - Detection gate: if `!m.detectResolved` → **defer**: stash a `pendingBurstEnter bool` (+ the captured `ordered`/`trigger`/`external` snapshot) and return `m, nil`; the `terminalDetectedMsg` arm (6-1) resolves it — supported → dispatch the burst below; NULL → the 6-9 no-op. If resolved supported → dispatch now. If resolved NULL → the 6-9 no-op. (6-3 owns the supported dispatch + the defer plumbing; 6-9 owns the NULL branch.)

**Proposed** (task 6-3, **Do** — replace that bullet):
  - Detection gate: if `!m.detectResolved` → **defer**: stash a `pendingBurstEnter bool` (+ the captured `ordered`/`trigger`/`external` snapshot) and return `m, nil`; the `terminalDetectedMsg` arm (6-1) resolves it. If `m.detectResolved`, branch on the cached **resolution**, not `IsNull()`: if `m.DetectUnsupported()` (`detectResolution == ResolutionUnsupported`, covering NULL remote/mosh **and** a non-NULL undriven identity like Apple Terminal / an unknown terminal with no native or config adapter) → the 6-9 atomic no-op; else (a supported `native`/`config` resolution) → dispatch the burst below (the adapter is then guaranteed non-nil). (6-3 owns the supported dispatch + the defer plumbing; 6-9 owns the unsupported branch.) **Do not** branch on `IsNull()` — a non-NULL undriven identity is non-NULL yet unsupported, so an `IsNull()` gate would fall into the "supported" arm and dispatch the burst with a nil adapter → `Burster.Run` calls `OpenWindow` on nil.

**Current** (task 6-9, **Do** — the three-way detection-gate branch):
  - resolved **supported** (`!IsNull()` and `resolve(...)` returns a non-unsupported resolution) → dispatch the burst (Task 6.3).
  - resolved **unsupported/NULL** (`m.detectIdentity.IsNull()`) → this task's atomic no-op:
    - Construct no pipe, resolve no adapter, call no adapter method, do not set `m.selected`, do not `tea.Quit`.
    - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where `unsupportedFlashText` composes `⚠ unsupported terminal — <name> · <bundleID>` (same identity naming as the 6-2 banner; a transient flash because the multi-select banner owns the section-header row in mode). A NULL-with-no-identity (`Name`/`BundleID` empty, remote/mosh) folds to `⚠ no host-local terminal — nothing opened` (the honest no-op line).
    - Stay in multi-select mode; leave `m.selectedSessions` intact (no prune — nothing was gone, only unsupported).

**Proposed** (task 6-9, **Do** — replace those branches; branch on resolution, copy on IsNull):
  - resolved **supported** (`!m.DetectUnsupported()`, i.e. `detectResolution` is `native` or `config`) → dispatch the burst (Task 6.3).
  - resolved **unsupported** (`m.DetectUnsupported()` — `detectResolution == ResolutionUnsupported`; this covers BOTH a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven identity like Apple Terminal, and any unknown passthrough terminal with no native or config adapter) → this task's atomic no-op:
    - Construct no pipe, resolve no adapter, call no adapter method, do not set `m.selected`, do not `tea.Quit`.
    - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where the **copy branches on `IsNull()`** (mirroring CLI Task 2.7): a named identity (non-NULL — e.g. Apple Terminal) yields `⚠ unsupported terminal — <name> · <bundleID>`; a NULL-with-no-identity (`Name`/`BundleID` empty, remote/mosh) folds to `⚠ no host-local terminal — nothing opened`. (A transient flash because the multi-select banner owns the section-header row in mode.)
    - Stay in multi-select mode; leave `m.selectedSessions` intact (no prune — nothing was gone, only unsupported).

Also in task 6-9, **Do** — the deferred-Enter resolution bullet:

  Current: "In the `terminalDetectedMsg` arm (Task 6.1), if `m.pendingBurstEnter` is set: clear it and re-run the N≥2 branch decision against the now-resolved identity (supported → dispatch; unsupported → this no-op). A transient-error identity resolves to `Identity{}` (`IsNull()`), so it takes the no-op."

  Proposed: "In the `terminalDetectedMsg` arm (Task 6.1), if `m.pendingBurstEnter` is set: clear it and re-run the N≥2 branch decision against the now-cached **resolution** (`!DetectUnsupported()` → dispatch; `DetectUnsupported()` → this no-op). A transient-error identity resolves to `Identity{}` (NULL → unsupported), and a non-NULL undriven identity resolves to `ResolutionUnsupported`, so both take the no-op."

Also in task 6-9, **Acceptance Criteria** — replace the two NULL-only criteria:

  Current: "[ ] N≥2 Enter on a resolved-unsupported/NULL terminal opens nothing …" and "[ ] A transient-error identity (`Identity{}`) is treated identically to a clean NULL (atomic no-op)."

  Proposed: add/replace so both the NULL and the non-NULL undriven cases are asserted:
  - [ ] N≥2 Enter on any resolved-**unsupported** terminal (`DetectUnsupported()`) — a NULL remote/mosh identity **or** a non-NULL recognised-but-undriven identity (e.g. Apple Terminal `com.apple.Terminal`) — opens nothing (no `burstProgressPipe`, no adapter resolve/call), does not self-attach, and stays in multi-select mode with the selection intact.
  - [ ] A non-NULL undriven identity re-asserts `⚠ unsupported terminal — <name> · <bundleID>`; a bare-NULL identity re-asserts `⚠ no host-local terminal — nothing opened` (copy branches on `IsNull()`, gate branches on resolution — matching CLI Task 2.7).
  - [ ] A transient-error identity (`Identity{}`, NULL) is treated identically to any other unsupported resolution (atomic no-op).

And add a **Test** to task 6-9:
- `"it treats a non-NULL recognised-but-undriven identity (Apple Terminal) as unsupported (atomic no-op, banner re-asserted)"`

And add a **Test** to task 6-2:
- `"it renders the unsupported banner for a non-NULL undriven identity (Apple Terminal com.apple.Terminal)"`

**Current** (task 6-11, **Do** — the `WithInitialDetection` seed seam bullet):
  - `Deps.InitialDetection *spawn.Identity` + `WithInitialDetection(id)` — seeds `detectResolved = true` and `detectIdentity = *id` (a NULL identity for the unsupported frame; a supported identity for a supported render). Capture-only.

**Proposed** (task 6-11, **Do** — replace that bullet):
  - `Deps.InitialDetection *spawn.Identity` + `WithInitialDetection(id)` — seeds `detectResolved = true`, `detectIdentity = *id`, **and `detectResolution` by resolving `*id`** (e.g. via the zero-config `spawn.ResolveAdapter(*id)`, which is sufficient for the capture fixtures — a non-NULL undriven identity like Apple Terminal resolves `ResolutionUnsupported` regardless of config). Seeding `detectResolution` (not just `IsNull()`) is required so `DetectUnsupported()` is true and the proactive banner renders for the **non-NULL** Apple Terminal frame. Capture-only.

**Current** (task 6-11, **Do** — the unsupported-terminal fixture bullet):
  - `sessionsUnsupportedTerminalFixture()` — `initialDetection` = a NULL/unsupported identity carrying `Name:"Apple Terminal"`, `BundleID:"com.apple.Terminal"` (the frame's identity); normal mode (no multi-select); `initialMode: prefs.ModeFlat`. Maps into `Deps.InitialDetection`.

**Proposed** (task 6-11, **Do** — replace that bullet):
  - `sessionsUnsupportedTerminalFixture()` — `initialDetection` = a **non-NULL, resolves-unsupported** identity `Name:"Apple Terminal"`, `BundleID:"com.apple.Terminal"` (the frame's identity — note it is NOT NULL; it is unsupported by *resolution*, so `WithInitialDetection` seeds `detectResolution = ResolutionUnsupported` and `DetectUnsupported()` is true, driving the banner); normal mode (no multi-select); `initialMode: prefs.ModeFlat`. Maps into `Deps.InitialDetection`.

**Resolution**: Pending
**Notes**:

---
