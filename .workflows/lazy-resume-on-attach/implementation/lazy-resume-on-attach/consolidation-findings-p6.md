# Consolidation Findings: lazy-resume-on-attach (Phase 6)

## Findings

### F1: Doctor's resume-mode line restates the hydrate helper's install-mode resolution instead of calling it
- **Class**: near-miss
- **Failure**: `portal doctor`'s `resume mode` line is the only place a user can read back the install-wide resume mode. `hook list` deliberately leaves it out, and nothing in Portal writes it. The line exists to answer one question: will my restored panes wait? The helper that actually decides this at restore time resolves the mode in `installResumeMode`. Task 6-3 wrote its own copy of that rule in `checkResumeMode`:
  - no store: `resumemode.Default`;
  - a failed read: the `Default` that `LoadResumeMode` returns alongside its error;
  - otherwise: the parsed mode.

  The two copies agree today, and nothing ties them together. The doctor tests pin doctor's copy against `prefs.json` fixtures, and the hydrate tests pin the helper's copy. Suppose the helper's resolution moves — a different policy for an unreadable `prefs.json`, a different loader, a new source for the install mode. Doctor would keep printing the old answer. A user who reads `lazy` from doctor would then reboot into panes that fire eagerly (or the reverse), and would only find out at that reboot. No test, compile error or visible symptom connects the two in the meantime.
- **Evidence**:
  - `cmd/doctor.go:365-372`: `checkResumeMode`, the nil-store fallback plus `store.LoadResumeMode()` with its error discarded.
  - `cmd/state_hydrate.go:202-217`: `installResumeMode`, the same rule. The prefs store is built through `loadPrefsStoreNoMigrate`, a build failure or nil store falls back to `resumemode.Default`, and the read error is discarded.
  - `cmd/doctor.go:130-138`: doctor builds its store through the same `loadPrefsStoreNoMigrate` and turns a build failure into a nil `PrefsStore`. This is the other half of the helper's fallback, spread across `resolveDoctorDeps`.
  - The two near-identical comments that each justify the discard: `cmd/doctor.go:364` and `cmd/state_hydrate.go:214`.
- **Proposed shape**:
  - Extract one resolver in `cmd`, e.g. `func installResumeModeOf(store *prefs.Store) resumemode.Mode`: nil gives `resumemode.Default`, anything else gives `LoadResumeMode` with the error discarded. It carries the single "the error is discarded because the mode beside it is already the default" line.
  - `installResumeMode` loads its store and hands the result to it, passing nil on a load error.
  - `checkResumeMode` becomes `detail: installResumeModeOf(store).String()`.

  After this, the doctor line and the helper's decision cannot answer differently for the same `prefs.json`. The change is behaviour-preserving.

### F2: The pending dot adds an `accent.attention`-on-`bg.selection` pairing that no contrast guard, swatch or theme doc covers — and the light built-in measures 3.64:1 on it
- **Class**: behaviour
- **Failure**: The pending dot keeps its `accent.attention` foreground on the cursor row, over `bg.selection`, just as the attached dot keeps `state.positive`.
  - **Existing coverage.** Portal's contrast guard lists every foreground-on-tint pairing the UI renders, and it holds `state.positive` on `bg.selection` to `floorNormal` (4.5). The theme files are corrected against the same rule; `tokyo-night-day.theme` says "a single token renders both on the canvas and on the selected row, so it must clear 4.50 against both".
  - **The gap.** This phase made `accent.attention` a token of that kind and enrolled the new pairing nowhere.
  - **Measured (WCAG relative luminance) for `accent.attention` on `bg.selection`:**
    - `tokyo-night-day`: 3.64:1 (`#9A5200` on `#D0C6F0`). The attached dot beside it on the same tint is held to 4.5 and measures 4.65.
    - `tokyo-night`: 7.36:1.
    - `nord`: 5.52:1.
  - **Consequences.**
    - A light-theme user's highlighted pending row shows a dot below the floor its sibling must clear.
    - A drop-in theme can take this pairing below any floor with no failure anywhere.
    - `docs/theming.md` tells drop-in authors that `accent.attention` is "the `/` filter query, edit mode, the `⚠` warning glyph", so they have no reason to tune it against `bg.selection`.
    - The contrast swatch is the human surface for fg-on-tint judgements. Its selection band still renders the retired `● attached` worded marker as the state.positive sample, and shows no pending dot.
  - **Open floor question.** A lone indicator glyph may deserve `floorLargeUI` (3.0). That is the glyph/bar floor `accent.primary` is held to, and 3.64 clears it. But that is a floor decision the project has not taken for this pairing. Either way, the pairing is unguarded.
- **Evidence**:
  - `internal/tui/session_item.go:321-324`: the cluster carries `pending` into `indicatorCluster`.
  - `internal/tui/session_item.go:341-357`: `indicator` renders through `rowToken(…, tok, selected)`, which is `bg.selection` on the cursor row.
  - `internal/theme/contrast_test.go:188-209`: `TestForegroundOnTintPairings` has `state.positive`/`bg.selection` at `:200` and no `accent.attention`/`bg.selection`.
  - `internal/theme/contrast_test.go:212-219`: `TestStatePositiveClearsCanvasAndSelection`.
  - `internal/theme/contrast_test.go:16-17`: the two floors.
  - `internal/theme/builtins/tokyo-night-day.theme:84`: `accent.attention = #9A5200`.
  - `internal/theme/builtins/tokyo-night-day.theme:85-88`: the two-surface rule.
  - `internal/theme/builtins/tokyo-night-day.theme:110`: `bg.selection = #D0C6F0`.
  - `internal/capture/swatch.go:109`: `swatchAttached = "● attached"`.
  - `internal/capture/swatch.go:131`: the selection-band caption.
  - `internal/capture/swatch.go:180-190`: `selectionBand`.
  - `internal/capture/swatch_test.go:216-244`: pins the swatch's pairing set.
  - `docs/theming.md:73`: the `accent.attention` role row.
- **Proposed shape**:
  1. Enrol `{"accent.attention", "bg.selection", …}` in `TestForegroundOnTintPairings` at the floor the project assigns a lone indicator glyph. If that floor is `floorNormal`, `tokyo-night-day`'s `accent.attention` needs a correction, with its `#` note, the way `state.positive` got one.
  2. Update the swatch's selection band to render the row as it now is: a bare `state.positive` `●` and an `accent.attention` `●`, replacing `● attached`. Add the new pairing to its caption and to `TestSwatchCoversForegroundOnTintPairings`.
  3. Add the pending dot, which sits on both the canvas and the selected row, to the `accent.attention` row in `docs/theming.md`.

### F3: On the cold reboot route the picker takes its one pending read before the hydrate helpers have marked their panes
- **Class**: behaviour
- **Failure**: This comes from reading the code; I have not measured it.

  The case the pending dot exists for is the reboot. After a reboot the first bare `x` finds no tmux server, so it takes the cold concurrent route. On that route the picker's first visible list is the post-restore refetch in `completeLoadingDismissal`. It reads sessions and the pending set once. Afterwards, the pending set is re-read only on a kill, a rename or a preview dismissal.

  Bootstrap step 7 only writes each helper's signal byte. It does not wait for the helper. Each helper then marks its pane `@portal-resume-pending` after two things:
  - the full scrollback copy into the pane, plus a 100 ms settle sleep;
  - in the signal-timeout tail, after the 3 s `hydrateTimeout`.

  Steps 8–10 are a handful of tmux calls. When the restore itself runs past `LoadingMinDuration` (1.2 s), the loading gate dismisses on `BootstrapCompleteMsg` and the refetch lands a few milliseconds after the last signal. That is before any helper's 100 ms settle has even elapsed. Every pane still replaying at that moment reads as not pending.

  The result: the picker opened straight after a reboot shows fewer pending dots — possibly none — than there are waiting panes. It stays that way until the user kills, renames or previews something. Re-opening the picker shows them all. The phase's landed surface makes the specification's "a row carries that dot when any pane in the session is waiting" concrete, and this route doesn't meet it.
- **Evidence**:
  - `internal/tui/model.go:1495-1500`: `refetchSessionsAfterRestore`.
  - `internal/tui/model.go:1533-1537`: `completeLoadingDismissal`.
  - `internal/tui/model.go:1704-1706`: the gate dismisses on `BootstrapCompleteMsg` once the pad has elapsed.
  - `internal/tui/pending_resume.go:18-35`: the one pending read, taken with the session enumeration.
  - The only other re-reads: `internal/tui/model.go:1381`, `:2815` and `:2878`.
  - `cmd/bootstrap/eager_signal_hydrate.go:29-45`: fire-and-forget signal write.
  - `cmd/state_hydrate.go:32-33`: `hydrateTimeout` 3 s and `hydrateSettleSleep` 100 ms.
  - `cmd/state_hydrate.go:153-172`: marks only after the copy and the settle.
  - `cmd/state_hydrate.go:272-285`: the timeout tail.
  - `cmd/state_hydrate.go:314-322`: every tail sets pending before it unsets its `@portal-skeleton-*` marker.
- **Proposed shape**: Use the ordering the helper already guarantees. Every tail marks pending before it unsets its skeleton marker. So once no `@portal-skeleton-*` marker remains, the pending set is final.
  - On the cold route only (`progressReceiver != nil`), after the post-restore refetch, re-take the pending read on a short tick for as long as `state.ListSkeletonMarkers` reports any marker.
  - Apply each reading through the existing `applySessions` path, or a pending-only variant that leaves the session list alone.
  - Bound the ticking by `hydrateTimeout` plus a margin, so a helper that never finishes cannot keep it running.
  - This needs a skeleton-marker reader seam beside `PendingResumeReader` in `tui.Deps`.
  - The warm route needs nothing: its helpers finished before the picker ran.

## Comment Corrections

- internal/tui/model.go:112 — `Err` is not the session read's alone: `killAndRefresh` and `renameAndRefresh` put their own failures in it (`:2813`, `:2876`)
  OLD: // Pending is the pending-resume set read with Sessions; Err is the session
// read's alone, so a failed pending read never quits the picker.
  NEW: // Pending is the pending-resume set read with Sessions. Err never carries the
// pending read's failure, so a failed pending read never quits the picker.
