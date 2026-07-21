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

- **H1 — Persistent NULL banner** [suspected]
  The section-header swap gates only on `DetectUnsupported()` (resolution-based, TRUE for NULL/remote), with no identity-shape discriminator, so the NULL/remote resolution claims the header row permanently and drops `Sessions ··· N`.
  _Basis:_ `unsupportedBannerActive()` = `DetectUnsupported() && !multiSelectMode` (model.go:4681); `DetectUnsupported` = `detectResolved && detectResolution == ResolutionUnsupported` (spawn_detect.go:117), true for both NULL and named-unsupported.

- **H2 — `m` enters unconditionally** [suspected]
  `handleMultiSelectToggle`'s entry branch has NO `DetectUnsupported()` gate; the only unsupported gate is downstream at `decideBurst`'s N≥2 Enter (the reactive `— nothing opened` no-op).
  _Basis:_ model.go:3508-3524 (entry, ungated); burst_progress.go:425 (reactive arm).

- **H3 — `m` shows in `?` help unconditionally** [suspected]
  The help modal is fed the static `sessionsKeymap()` with no detection-aware filtering; `m` is a help-only (non-Core) entry, so the FOOTER is unaffected — only the help modal.
  _Basis:_ keymap.go:89-105; help modal call model.go:4547.

- **H4 — (contributing/edge) async detection keeps the reactive backstop load-bearing** [suspected]
  Detection is ASYNC — `DetectUnsupported()` is false until resolved, so an entry-time `m`-block cannot fully replace the reactive `decideBurst` no-op: the in-flight→resolve race keeps the reactive backstop load-bearing, and raises a UX fork (eject from the mode when detection resolves unsupported mid-mode?).
  _Basis:_ async dispatch spawn_detect.go:83-92; `TestBurstUnsupported_DeferredThenUnsupported`.

### Trace lines (agreed order)

1. Banner predicate chain — `unsupportedBannerActive` → `applySectionHeader` AND `activeNoticeBand` (the single predicate both read); confirm an `IsNull()` discriminator drops NULL while keeping named.
2. `m`-entry handler — `handleMultiSelectToggle` entry vs toggle branch; confirm the live entry point is the only ungated site (`WithInitialMultiSelect` is capture-harness only).
3. Help-modal descriptor feed — `sessionsKeymap()` → `renderHelpModalOnClearedCanvas`; confirm `keymap_dispatch_guard_test` tolerates a conditionally-absent `m`.
4. Reactive backstop + async race — why the entry-block does NOT remove `decideBurst`'s unsupported arm; scope the in-flight window.
5. Flash lifecycle + copy — `setFlash` self-clears on next actionable key (model.go:3328); the blocked-entry copy variant vs `spawn.UnsupportedNoopMessage`'s `— nothing opened` burst-response wording (design fork for the spec).

### Code Trace

_(to be populated)_

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
