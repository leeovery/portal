---
status: complete
created: 2026-07-02
cycle: 2
phase: Gap Analysis
topic: skip-bootstrap-when-warm
---

<!--
Both findings applied (finding_gate_mode: auto), verified against real code:
 1 (Important) — pinned daemon cleanup wiring: lister=daemonDeps.Client, store=loadHookStore()
   (same hooks.json / env-inheritance rule), swallowListError=true, onRemoved=nil; added to
   Operational contract + Affected Code Surface. Also corrected the daemon bullet's stale
   "skipped on idle fast-path" wording to the idle-branch placement (coherence with c1 finding 3).
 2 (Minor) — named the second TUI constant totalBootstrapSteps=11 (bar denominator) → 10, plus
   the drop-key-11 (not renumber) note for stepLabelTable/drift-guard.
-->


# Review Tracking: skip-bootstrap-when-warm - Gap Analysis

## Findings

### 1. Daemon-owned cleanup: dependency construction and call-arguments left unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Daemon-Owned Hooks Cleanup" → "Operational contract" (Reuse, don't reinvent) and "Affected Code Surface" → "Daemon (new cleanup home)"

**Details**:
The spec commits to homing hooks cleanup on the daemon by having it "call the existing shared `cmd/run_hook_stale_cleanup.go` `runHookStaleCleanup` helper," and correctly notes both live in package `cmd` so there is no import/cycle problem. But `runHookStaleCleanup` has a five-argument signature — `(lister AllPaneLister, store *hooks.Store, logger *slog.Logger, swallowListError bool, onRemoved func(string))` — and the spec never says how the daemon supplies the first two, nor which policy value it passes for the last two. An implementer is forced to make design decisions the spec should pin:

- **`AllPaneLister`** — satisfied by `*tmux.Client.ListAllPanes()`. The daemon already holds a `*tmux.Client` in `daemonDeps.Client`, so this is available, but the spec never states that this is the source (vs. constructing a fresh client, vs. a new seam). The daemon's `daemonDeps` struct today carries no lister field.
- **`*hooks.Store`** — the daemon's `daemonDeps` carries no hooks store. In package `cmd` the store is built via `loadHookStore()` → `hooksFilePath()` → `configFilePath("PORTAL_HOOKS_FILE", "hooks.json")`. The spec is silent on whether the daemon builds it once at startup (into `daemonDeps`) or per-cleanup, and — importantly — on the config-path-resolution implication: the daemon runs as the pane process of the hidden `_portal-saver` session, so whether it resolves the same `hooks.json` the user's foreground commands mutate depends on it seeing the same `PORTAL_HOOKS_FILE`/`XDG_CONFIG_HOME` env. This is the exact isolation class the CLAUDE.md daemon-env-inheritance guidance flags, yet the spec does not state the daemon reuses `loadHookStore()`/`configFilePath` so it lands on the same file.
- **`swallowListError`** — the two existing call sites diverge deliberately (bootstrap passes `false`, `portal clean` passes `true`). The daemon is a new third caller and the spec does not say which posture it takes. Given "log WARN and retry next cadence; a cleanup error never escalates or crashes the daemon," `true` is the natural choice, but this is left to the implementer to infer rather than stated.
- **`onRemoved`** — presumably `nil` (no user-facing stdout in a background daemon), but unstated.

Without these fixed, two implementers could wire the daemon cleanup differently (e.g. one resolving a different hooks.json than the foreground process), producing a cleanup that silently never touches the file the user actually edits — defeating the feature's stated purpose on a weeks-long server.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Approved
**Notes**: Priority: Important. All referenced primitives exist and compose cleanly (`*tmux.Client` satisfies `AllPaneLister`; `loadHookStore()` builds the store; same package → no cycle) — the gap is purely that the spec doesn't state the wiring, forcing a design decision, notably the same-hooks.json-resolution point.

---

### 2. `internal/tui/loading_progress.go` bar-fraction step-count constant not named in the 11→10 change

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Affected Code Surface" → "Orchestrator" (the `internal/tui/loading_progress.go` bullet)

**Details**:
The spec is meticulous about the orchestrator-side count change, explicitly naming the `cmd/bootstrap` `totalSteps = 11` constant as a "documented load-bearing contract" that must go to `10`. But the parallel constant on the TUI side is not named. `internal/tui/loading_progress.go` declares its own independent `totalBootstrapSteps = 11`, and it is the **denominator of the loading bar fraction** (`BarFraction = len(completedSteps) / totalBootstrapSteps`, used in both `View()` and `FailedView()`). The spec's TUI bullet says only: "removing `CleanStale` drops its table entry (and any mapping/drift-guard test that pins the step count). Verify the real-step→label mapping and the `N/M` counter … still hold at 10 steps." It mentions the `stepLabelTable` entry and the drift-guard test (`TestMappingCoversAllElevenStepsNoGaps`), but not the `totalBootstrapSteps` constant itself.

If an implementer follows the spec literally — drop table key 11, fix the test — but leaves `totalBootstrapSteps = 11`, the bar would advance by 1/11 per real step and top out at 10/11 ≈ 91%, never reaching 100% on a successful full bootstrap. Given the spec named the analogous `totalSteps` constant on the orchestrator side, the omission of `totalBootstrapSteps` on the TUI side is an asymmetry that could mislead. (Note: because steps 9 and 10 keep their indices and only step 11 is removed, the surviving table keys stay contiguous at 1..10 — so the change is a drop-key-11 + retune-denominator, not a renumber. The spec should say so explicitly to close the "does removal leave a gap?" question the drift-guard test's 1..11 assertion otherwise raises.)

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Approved
**Notes**: Priority: Minor. A competent implementer would likely find the constant while fixing the failing drift-guard test, but the spec's own precedent (naming `totalSteps`) makes the omission of `totalBootstrapSteps` a small consistency gap worth closing.

---
