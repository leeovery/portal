# Investigation: Persistent "no host-local terminal" banner on remote clients; multi-select blocked on unsupported terminals

## Symptoms

### Problem Description

**Expected behavior:**

On remote/unsupported terminals the picker should behave sensibly:

1. **Banner split by identity shape.** Drop the proactive "no host-local terminal" banner entirely for the **NULL/remote** identity (mosh/SSH, no host-local terminal) so the standard `Sessions ··· N` section header (count + grouping-mode suffix) renders as normal. **Keep** the proactive banner for *named* unsupported terminals (e.g. Apple Terminal) where it is actionable — it carries the bundle id (the copy-paste key for `terminals.json`) and the `see docs` hint.
2. **Multi-select fully disabled on any unsupported resolution** (named and NULL alike). Pressing `m` fails immediately rather than entering the mode and dead-ending at the N≥2 Enter. The blocked `m` surfaces a **transient error banner that self-clears on the next keypress** (a flash), and `m` does **not** appear in the `?` help keymap when unavailable.

**Actual behavior:**

- On a pure-remote client (mosh/SSH, NULL terminal identity from spawn detection) the §6.2 proactive unsupported-terminal banner shows **permanently** for the whole picker session and, because it *replaces* the standard section header (`unsupportedBannerActive` in `internal/tui/model.go`), the session count and grouping-mode suffix are lost. On a remote client this is pure noise — nothing the user can act on.
- Multi-select `m` still **enters** the mode on any unsupported terminal, letting the user mark sessions only to dead-end at the N≥2 Enter with a reactive no-op flash (`⚠ no host-local terminal — nothing opened`, per `internal/tui/burst_unsupported_noop_test.go`) — a misleading affordance for a burst that can never fire.

### Manifestation

- A permanent `⚠ no host-local terminal` warning band replacing the `Sessions ··· N` section header on every remote picker launch.
- A walkable-but-dead multi-select flow: `m` enters, sessions mark, N≥2 Enter no-ops with a reactive flash.

### Reproduction Steps

1. Log into the Mac over mosh from an iPad (pure-remote client; no host-local terminal attached to the tmux server).
2. Open the picker (`portal open` / `x`).
3. Observe: the `⚠ no host-local terminal` banner shows permanently; the `Sessions ··· N` header (count + grouping suffix) is gone.
4. Press `m`, mark ≥2 sessions, press Enter → atomic no-op with reactive flash, selection intact.

**Reproducibility:** Always (any picker launch where spawn detection resolves the NULL identity, or a named unsupported terminal for the multi-select dead-end).

### Environment

- **Affected environments:** local (developer machine, remote access)
- **Browser/platform:** tmux picker TUI; remote clients (mosh/SSH) resolve NULL identity; named unsupported terminals (e.g. Apple Terminal) resolve a bundle id but no adapter.
- **User conditions:** picker launched with spawn detection resolving an unsupported terminal (NULL/remote OR named-unsupported).

### Impact

- **Severity:** Low (cosmetic-but-constant + misleading affordance; no data loss)
- **Scope:** every remote picker launch; every unsupported-terminal multi-select attempt
- **Business impact:** reads as ugly/broken even though behaving as designed; wasted user effort on a guaranteed dead-end.

### References

- Seed: `seeds/2026-07-16-persistent-no-host-terminal-banner.md` (inbox:bug)
- Discovery: `discovery/sessions/session-001.md`
- Related in-flight: `cli-verb-surface-redesign` (owns CLI multi-target burst block — out of scope here)
- Related bug: `2026-07-15--remote-trigger-spawns-on-local-terminal` (once its trigger-locality gate is fixed, every remote login resolves NULL → this banner would appear on all remote use)

---

## Analysis

### Hypotheses

**Checkpoint depth:** straight-through — the bug is contained to `internal/tui`, the mechanism is near-confirmed from recon (discovery framed this a light confirm pass); the genuine design forks belong to the spec, not the trace.

- **H1 — Persistent NULL banner** [confirmed]
  The section-header swap gates only on `DetectUnsupported()` (resolution-based, TRUE for NULL/remote), with no identity-shape discriminator, so the NULL/remote resolution claims the header row permanently and drops `Sessions ··· N`.
  _Basis:_ `unsupportedBannerActive()` = `DetectUnsupported() && !multiSelectMode` (model.go:4681); `DetectUnsupported` = `detectResolved && detectResolution == ResolutionUnsupported` (spawn_detect.go:117), true for both NULL and named-unsupported.
  _Evidence:_ `applySectionHeader` (model.go:4780) swaps in `renderUnsupportedHeader` whenever `unsupportedBannerActive()` — and `renderUnsupportedHeader` (section_header.go:178) ALREADY branches on `bundleID == ""` to draw the NULL variant `⚠ no host-local terminal` (no identity, no `see docs`). The single predicate is also read by `activeNoticeBand` (notice_band.go:371) to suppress the By-Tag signpost, so the two consumers can never drift. Adding `!m.detectIdentity.IsNull()` to `unsupportedBannerActive()` drops NULL (standard header returns) while keeping named — the render already knows the split; only the gate lacks the discriminator.

- **H2 — `m` enters unconditionally** [confirmed]
  `handleMultiSelectToggle`'s entry branch has NO `DetectUnsupported()` gate; the only unsupported gate is downstream at `decideBurst`'s N≥2 Enter (the reactive `— nothing opened` no-op).
  _Basis:_ model.go:3508-3524 (entry, ungated); burst_progress.go:425 (reactive arm).
  _Evidence:_ the `!m.multiSelectMode` entry branch (model.go:3509) sets `multiSelectMode=true` + mark-on-entry with no detection read. The sole live entry point is this handler (dispatched at model.go:3433); `WithInitialMultiSelect` (model.go:1006) is a construction/capture-harness option, not a keypress path. The reactive gate at `decideBurst` (burst_progress.go:425) fires only at the N≥2 Enter — so the user walks enter → mark → Enter before hitting it.

- **H3 — `m` shows in `?` help unconditionally** [confirmed]
  The help modal is fed the static `sessionsKeymap()` with no detection-aware filtering; `m` is a help-only (non-Core) entry, so the FOOTER is unaffected — only the help modal.
  _Basis:_ keymap.go:89-105; help modal call model.go:4547.
  _Evidence:_ `m` is `{Key:"m", ..., }` with no `Core:true` (keymap.go:97), so `renderCondensedFooter` (footer.go:65) never lists it — the footer needs no change. The help modal renders EVERY descriptor entry (help_modal.go:141) from `sessionsKeymap()` passed at model.go:4547. **Suppression mechanism confirmed viable:** filtering `m` out of the descriptor slice passed to `renderHelpModalOnClearedCanvas` when `DetectUnsupported()` keeps `sessionsKeymap()` static, so `keymap_dispatch_guard_test.go` (which probes the static descriptor with an unwired-detection model → `m` supported → enters mode) still passes. A parameterised `sessionsKeymap()` that conditionally omits `m` would instead break that guard — so the call-site filter is the guard-safe shape.

- **H4 — (contributing/edge) async detection keeps the reactive backstop load-bearing** [confirmed]
  Detection is ASYNC — `DetectUnsupported()` is false until resolved, so an entry-time `m`-block cannot fully replace the reactive `decideBurst` no-op: the in-flight→resolve race keeps the reactive backstop load-bearing, and raises a UX fork (eject from the mode when detection resolves unsupported mid-mode?).
  _Basis:_ async dispatch spawn_detect.go:83-92; `TestBurstUnsupported_DeferredThenUnsupported`.
  _Evidence:_ detection runs on Bubble Tea's command goroutine (`maybeDispatchDetectionCmd`, spawn_detect.go:83), resolving later via `terminalDetectedMsg` (model.go:2460). Before it resolves, `detectResolved==false` → `DetectUnsupported()==false` → the entry-block does NOT fire, so the user CAN enter multi-select in the in-flight window. The `terminalDetectedMsg` arm (model.go:2471-2485) caches the resolution but does NOT eject an already-open multi-select mode (it only resolves a `pendingBurstEnter` deferral via `decideBurst`). So `decideBurst`'s unsupported arm remains the backstop for "entered before resolve, then Enter". The fix ADDS a proactive entry-block; it must RETAIN the reactive no-op.

### Trace lines (agreed order)

1. Banner predicate chain — `unsupportedBannerActive` → `applySectionHeader` AND `activeNoticeBand` (the single predicate both read); confirm an `IsNull()` discriminator drops NULL while keeping named.
2. `m`-entry handler — `handleMultiSelectToggle` entry vs toggle branch; confirm the live entry point is the only ungated site (`WithInitialMultiSelect` is capture-harness only).
3. Help-modal descriptor feed — `sessionsKeymap()` → `renderHelpModalOnClearedCanvas`; confirm `keymap_dispatch_guard_test` tolerates a conditionally-absent `m`.
4. Reactive backstop + async race — why the entry-block does NOT remove `decideBurst`'s unsupported arm; scope the in-flight window.
5. Flash lifecycle + copy — `setFlash` self-clears on next actionable key (model.go:3328); the blocked-entry copy variant vs `spawn.UnsupportedNoopMessage`'s `— nothing opened` burst-response wording (design fork for the spec).

### Code Trace

**Locus 1 — Banner (persistent NULL band).**
- `Model.View` (sessions arm) → `applySectionHeader(listView)` — model.go:4720.
- `applySectionHeader` §6.2 branch (model.go:4780): `if m.unsupportedBannerActive() → replaceHeaderLine(listView, renderUnsupportedHeader(name, bundleID, …))`. Replaces the FIRST list line (the title row) so the `Sessions ··· N` header is gone for the session.
- `unsupportedBannerActive()` (model.go:4681) = `DetectUnsupported() && !multiSelectMode` — **no identity-shape check.**
- `DetectUnsupported()` (spawn_detect.go:117) = `detectResolved && detectResolution == ResolutionUnsupported` → TRUE for NULL (remote/mosh) AND named-undriven alike.
- `renderUnsupportedHeader` (section_header.go:178) + `unsupportedLeftCluster` (section_header.go:223) already split on `bundleID == ""`: NULL → `⚠ no host-local terminal` (no identity, no `see docs`); named → `⚠ unsupported terminal — <name> · <bundleID>` + `see docs`.
- Second consumer: `activeNoticeBand` (notice_band.go:371) reads the SAME `unsupportedBannerActive()` to suppress the By-Tag signpost. Fixing the one predicate fixes both surfaces coherently.

**Locus 2 — `m`-entry (no proactive block).**
- Dispatch: `updateSessionList` `case isRuneKey(msg, "m") → handleMultiSelectToggle()` — model.go:3433.
- `handleMultiSelectToggle` entry branch (model.go:3509-3524): `if !m.multiSelectMode { multiSelectMode=true; … mark-on-entry … }` — **no `DetectUnsupported()` read.**
- Reactive gate downstream: `handleMultiSelectEnter` → `beginBurst`/`decideBurst` (burst_progress.go:414); `if m.DetectUnsupported() { …preflight…; emitUnsupportedNoop; setFlash(unsupportedFlashText(id)); return }` (burst_progress.go:425-445). Fires only at N≥2 Enter.
- Live entry point is unique: `WithInitialMultiSelect` (model.go:1006) is construction-time (capture harness), not a keypress.

**Locus 3 — `?` help lists `m` unconditionally.**
- Footer: `renderSessionsFooter → renderCondensedFooter(sessionsKeymap(), …)` (footer.go:65) lists only `Core` entries; `m` is non-Core (keymap.go:97) → **absent from footer already** (no footer change needed).
- Help modal: `Model.View` modalHelp arm → `renderHelpModalOnClearedCanvas(sessionsKeymap(), …)` (model.go:4547) → `helpModalBodyRows` renders every non-`RightAligned` entry (help_modal.go:141) → `m` always shown.
- Guard: `keymap_dispatch_guard_test.go:143` probes `m` against a plain `sessionsGuardModel` (detection unwired → supported) and asserts `MultiSelectActive()`. It reads the **static** `sessionsKeymap()` (line 184), so a call-site descriptor filter (drop `m` only in the help-modal feed when `DetectUnsupported()`) leaves the guard green; a parameterised keymap would not.

**Locus 4 — async race / reactive backstop.**
- `maybeDispatchDetectionCmd` (spawn_detect.go:83) dispatches `Detect()` on the command goroutine on reaching PageSessions; resolves later via `terminalDetectedMsg` (model.go:2460).
- The arm caches identity/adapter/resolution (model.go:2471-2473) and, only if `pendingBurstEnter`, calls `decideBurst` (model.go:2482-2484). It does **not** eject an open multi-select mode.
- Consequence: the proactive entry-block (gated on `DetectUnsupported()`) is inert during the in-flight window, so `decideBurst`'s unsupported arm stays load-bearing for "entered-before-resolve → Enter".

**Copy source.**
- `unsupportedFlashText(id)` (burst_progress.go:460) → `spawn.UnsupportedNoopMessage(id)` (message.go:77): NULL → `no host-local terminal — nothing opened`; named → `unsupported terminal — <name> · <bundleID> — nothing opened`. Same renderer feeds the CLI open burst (`cmd/open_burst_run.go:168`). The `— nothing opened` clause is a burst RESPONSE — semantically off for a pre-emptive `m`-entry block that attempts nothing (design fork for the spec).

**Flash lifecycle (self-clear on next keypress — already satisfied).**
- `setFlash` (model.go:1969) records `flashText`; `activeNoticeBand` (notice_band.go:361) gives the flash the §11 notice-band slot (a SEPARATE row below the title separator — co-renders with a named banner on the header row).
- Auto-clear: `updateSessionList` (model.go:3328) `if m.flashText != "" && isActionableKey(msg) { clearFlash() }` — an actionable key clears the flash and falls through to its handler. So a blocked-`m` flash self-clears on the next keypress with no new mechanism.

### Root Cause

_(to be populated)_

### Contributing Factors

_(to be populated)_

### Why It Wasn't Caught

_(to be populated)_

### Blast Radius

_(to be populated)_

---

## Fix Direction

_(to be populated after findings sign-off)_

---

## Notes

**Classification (from discovery):** Not strictly a regression — the banner and reactive-only `m` gating both work as designed. This is a *decided feature that turns out wrong in a case it didn't cover*. Settled as a **bugfix**: correcting wrong existing behaviour rather than building new capability.

**Scope boundary (from discovery):** The CLI multi-target `portal open <a> <b> …` (N≥2) burst block for unsupported/remote is owned by the in-flight `cli-verb-surface-redesign` feature, not this bug. That feature is expected to land before this bug is implemented. This bug stays **TUI-side** (banner split + `m`-entry block). Keep copy/UX consistent with the CLI's unsupported message.

**Investigation expectation (from discovery):** light confirm pass over the code loci (`unsupportedBannerActive` in `model.go`, the `m`-entry handler, `burst_unsupported_noop_test.go`, the help modal) — the spec does the real design work.

**Deferred design detail (from discovery):** exact flash copy (a variant suited to a blocked mode-entry vs the current `— nothing opened` burst wording) and the help-suppression mechanics.
