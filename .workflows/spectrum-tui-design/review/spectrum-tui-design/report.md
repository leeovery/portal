# Implementation Review: Spectrum TUI Design (Modern Vivid reskin + concurrent cold-boot flip)

**Plan**: spectrum-tui-design
**QA Verdict**: Approve *(was Request Changes — the single blocking gap was closed during the review; see Resolved below)*

## Summary

This is a large, exceptionally well-executed implementation. All **64 leaf tasks** across the 10 phases (5 feature phases + 5 analysis cycles) are implemented and marked `done`; `go build`, `go vet ./internal/tui/...`, and the full `go test ./...` suite are all green. Per-task verification found the work faithful to the specification (including the 2026-06-22 corrigendum) and to project Go conventions: the closed ~20-token colour layer is enforced by a self-maintaining colour-literal guard, the owned-canvas paint preserves the one-row-per-delegate pagination invariant everywhere, the concurrent cold-boot flip is correctly scoped to the cold+TUI path with race-safe channel discipline (sole-sender-closes, ctx-guarded sends, no goroutine leak), the keymap revision is locked by a descriptor↔dispatch drift guard, and the five analysis cycles genuinely converged duplication and dead code with byte-identical-render golden tests proving no behaviour drift. The one blocking gap found — the §15.6 light-mode `vhs` captures for the three edit-modal states were absent (the code rendered correctly in light mode and was unit-tested; only the committed visual-eyeball artifact was missing) — was **produced and eyeballed during this review**: the grey→violet→orange focus/edit grammar reads cleanly on the light `#e1e2e7` canvas across all three states, closing the item. The verdict is therefore **Approve**. Everything else is non-blocking: a handful of spec-conformance decisions to record, a cluster of latent narrow-width overflow edges, and a long tail of stale doc comments left behind by the later refactor cycles (the zero-risk doc cluster was applied during the review's do-now pass). One latent bug from task 1-6 (stable since 2026-06-19) was also **found and fixed during the review** via interactive testing: `RestoreTerminalBackground` could re-apply the owned canvas colour on exit when the async OSC 11 query raced the canvas set and the terminal echoed the canvas back as its "original" — leaving the canvas stuck (reproduced in Ghostty with the no-tags-signpost fixture). The fix guards the set-back against re-applying the canvas colour; verified fixed by the user in Ghostty (commit `e5ac47a6`).

## QA Verification

### Specification Compliance

Implementation aligns with the specification and its corrigendum across all surfaces. Deviations found (all non-blocking, surfaced under Recommendations for a decision):

- **No-tags signpost on-band text token** (§11.3) — renders `text.on-selection`, but §11.3 and the plan acceptance criterion specify `text.strong`; the test encodes the deviation rather than catching it. Justified as a §2.9 co-tuning choice (the band sits on the `bg.selection` tint) but **not recorded** as a decision. (Report 4-3)
- **Phase-1 light token values vs the §2.9 table** — `theme.go` pins `text.detail` Light `#586093` and `text.dim` Light `#767DA2`, where the §2.9 table lists `#5A6296` / `#7C84AA`; `accent.violet` light `#8A3FD1` measures 4.37 against `#e1e2e7`, not the published 5.7 (the `#FFFFFF`-vs-`#e1e2e7` erratum). All clear their floors, but the table-vs-actual gap is undocumented for these tokens. (Reports 2-7, 1-4)
- **No-matches glyph** (§7.3) — ships `∅` (U+2205) where the spec byte-specifies `⌀` (U+2300); visually near-identical. (Report 2-9)

The §3.4 footer-glyph divergence and the Projects arrow-only-nav defect flagged earlier in the pipeline were both **fixed** during the analysis cycles (tasks 8-2 and 7-1 respectively), verified here with re-captured fixtures and a live-dispatch regression test.

### Plan Completion

- [x] Phase 1–10 acceptance criteria met (the §15.6 light-mode edit-modal captures, the lone gap, were produced during this review — see Resolved).
- [x] All 64 tasks completed (62 fully complete; 4-3 and 9-2 returned "Issues Found" with non-blocking notes; 3-9's blocking deliverable gap is now closed).
- [x] No scope creep. Two tasks beyond the original 1-1…5-8 plan (2-10 global content gutter; 3-10 rename shared-input-box) are legitimate UX/DRY refinements tracked in the plan, not unplanned additions; phases 6–10 are the planned analysis cycles.

**Process note (non-blocking):** the implementation manifest's `completed_tasks` list has drifted — it is missing `2-3…2-9` and `3-1/3-2` and lists renumbered IDs. Git history and the tick tree both show every task implemented and `done`, so this is a manifest-bookkeeping discrepancy, not a real coverage gap. Worth reconciling so future continue/review runs read the true set.

### Code Quality

No blocking code-quality issues. Highlights: small-interface DI and the seam pattern are respected; the concurrency in the cold-boot flip is correct (channel ownership, close discipline, ctx-guarded sends verified against the >63-session race that was fixed in 5-3); the analysis cycles consolidated real duplication (footer key-hint helper, destructive-confirm builder, row-style/left-bar helpers, cleared-canvas placement, right-anchored row assembler, pad-right geometry, leaf canvas-paint delegation) without render drift, each pinned by a golden test. The main residue is **stale documentation** left by the later refactors (a deleted `Token.Color()` still referenced in the package doc; "violet" input comments where the code now renders orange; "Phase 4 deferred"/"task 1-7 later" notes for work that has shipped) — clustered under Do-now.

### Test Quality

Tests adequately verify requirements and are notably disciplined: byte-exact SGR oracles for render surfaces, dispatch-layer (not descriptor-layer) tests for keymap behaviour, a non-vacuous descriptor↔dispatch guard, contrast floors re-derived numerically, daemon-spawning tests under `IsolateStateForTest`, and no `t.Parallel()` in `cmd`. Minor gaps (under Quick-fixes): no narrow-width/long-command band test, no projects-side empty-state one-row test, no direct drop-`n` dispatch test for the kill/delete modals, and a couple of vacuous/over-slow probes (COLORFGBG never-read; a ~100ms real sleep in the appearance timeout tests). One pre-existing daemon integration test isolates via raw `PORTAL_STATE_DIR` rather than the mandated helper (under Bugs).

### Required Changes — RESOLVED during review

1. ~~**Produce the §15.6 light-mode `vhs` captures for the three edit-modal states.**~~ **Done.** Added `edit-modal-{navigate-name,chip-focused,edit-in-place}-light.{tape,png}` (mirroring `rename-modal-light.tape` with `--appearance light`), ran the captures, and eyeballed all three against §13.1: the light `#e1e2e7` canvas renders the focus/edit grammar legibly — grey (`border.separator`) idle chips, **violet** (`accent.violet`) focused field label + focused chip border + rounded NAME input, **orange** (`accent.orange`) editing chip border + live cursor + `◉ EDIT MODE` badge, blue (`accent.blue`) footer glyphs. No frame-compare (there is no light Paper reference for the edit modal — §15.6 is a per-token legibility eyeball, and every token used is already light-validated on sibling frames). (Report 3-9)

## Recommendations

### Do now

1. `internal/tui/theme/theme.go` — stale package/token docs left by shipped or later tasks
   - :10-11 dangling reference to the deleted `Token.Color()` "convenience" — drop the sentence (every renderer resolves via `ColorFor(mode)`) (Reports 1-3, 6-4, 9-2)
   - :20-21 "Until OSC 11 detection lands (1-7)" — detection has shipped; reword to "Dark is the no-answer fallback" (Report 6-4)
   - :30, :36, :118 "filled by task 1-4 / wired by 1-7 / placeholder" phrasing for now-shipped Light variants + resolver — reword to past/shipped tense (Report 6-4)
   - :147 add the erratum note to `accent.violet` light (`#8A3FD1` measures 4.37 vs `#e1e2e7`, clears its 3.0 floor unremedied) so it matches the six already-annotated tokens (Report 1-4)
2. Rename-input "violet" comments now contradict the orange always-editing render
   - `rename_modal.go:37`, `:96` "violet" → "orange" (Report 3-10)
   - `model.go:3172` "violet-outlined" → "orange-outlined" (Report 3-6)
   - `modal.go:74` "violet-outlined input box" → "orange-outlined" (Report 3-6)
3. `internal/tui/help_modal.go` — stale notes for shipped work
   - :31-36 "Phase 4 deferred" Preview-help note — now wired by 4-7; reword to past tense (Report 4-7)
   - :190 helpKeyGlyph "the footer NEVER calls this" — the command-pending footer does; scope the claim to the condensed footers (Report 7-4)
4. `cmd/concurrent_bootstrap_gate_test.go` / `concurrent_bootstrap_route_test.go` — header comments attribute the tests to 5-2 (they exercise the 5-1 gate); fix attribution and the dangling `errorClient`→`coldClient` name (Report 5-1)
5. `internal/tui/notice_band.go` doc tidies — :182-191 "single-line band" lead-in contradicted at :196 (say "single-line when it fits, wrapping otherwise"); :354 name a `noActiveBand` sentinel for the don't-care role on `ok=false` (Report 4-1)
6. `internal/tui/keys.go:19` — drop the never-defined `keyIsCtrlU`/`keyIsCtrlD` from the package doc (Report 1-2)
7. `cmd/open.go:502-503` — "honouring it … is a later task" is stale; 1-7 honours the `appearance` pin via `Build → armAppearanceDetection` (Report 1-7)
8. `internal/tui/model.go:415-417` — `byTagSignpost` cites a stale spec-section name; replace with "§11.3 / §5.3" (Report 4-3)
9. `internal/tui/colour_literal_guard_test.go:18` — closed-vocabulary/no-hex rule is §2.9, not §2.8; align the reference (Report 7-4)
10. `internal/tui/loading_view.go:84-94` — const-block doc describes a `loadingBlockWordmarkWidth` not in the block; re-point to `loadingBlockBannerWidth` (Report 5-5)
11. `internal/tui/pagepreview_resize_test.go:13-15` — comment still says overhead "= 2"/one-row-one-column; the joined panel is now 6 (Report 4-6)
12. `internal/tui/content_inset_test.go:266-286` — `...AppliesOnProjectsPreviewLoading` overstates coverage (no Preview subtest); rename or add the subtest (Report 2-10)
13. `testdata/vhs/LOCK-IN.md:158-163` + `contrast-validation-light.tape:9-12` — the on-selection-green remedy text was superseded in Phase 2 (folded into global `state.green`); sync the DECISION bullet + tape header (Report 1-9)
14. Smaller one-offs: `section_header.go:90-91` eager-hint render note (Report 2-3); `destructive_confirm.go:57` nameTrailer wording (Report 3-7); `modal.go` §14.6 ADAPT-decision comment made self-contained (Report 3-1); `prefs/store.go:196-200` note the intentionally-discarded read-modify-write bool (Report 1-5); `keymap.go:23-37` drop the stale HelpKey transition narrative (Report 7-3); `theme/contrast_test.go:331` cross-ref the overlapping light-variant test (Report 1-4); `testdata/vhs/sessions-flat.tape:33` annotate the `Sleep` first-paint pad (Report 1-1)

### Quick-fixes

15. Write-only / reader-only-in-test dead state
    - `model.go:323-326,2029` `latestProgress` field is written, never read; remove field + assignment (Reports 5-5, 8-1)
    - `cmd/bootstrap/progress_emitter.go:36` + `bootstrap.go:284,378` `StepEvent.Name` now only read by a test assertion; drop or document as a retained diagnostic (Report 8-1)
    - `cmd/bootstrap_progress.go:182,284` `p.warnings`/`Warnings()` (and sibling `ServerStarted()`/`Err()`) unused for delivery; mark test-only or remove (Report 5-7)
16. Test-coverage additions
    - `command_pending_band_test.go` add a narrow-width/long-command case asserting the band does not exceed `w` (Reports 4-1, 4-4)
    - `empty_states_test.go:318` add a projects-mirror one-row-invariant assertion; `model.go:3861` test the `!commandPending` empty-projects suppression gate (Report 4-5)
    - add a `tea.KeyPressMsg "n"` dispatch test for the kill (`kill_modal_dispatch_test.go`) and delete (`model.go:2337-2360`) modals to lock drop-`n` (Reports 3-7, 3-1)
    - `appearance_detection_test.go` add a direct `BackgroundColorMsg{Color: nil}` → Dark case (Report 1-7)
17. `model.go:2786-2815` (`commitAlias`) — early-return when `aliasEditor == nil` (mirror `commitTag`'s `projectEditor` handling) so the unreachable nil-branch can't silently desync the in-memory chip from disk; optionally factor the shared `commitAlias`/`commitTag` skeleton if a third chip field appears (Report 3-8)
18. `appearance_detection_test.go:180-203` — the auto-mode timeout cmd makes each drain sleep ~50ms (~100ms total); detect the `appearanceTimeoutMsg` producer without executing the timer (Report 1-7)
19. `help_modal.go:201-203` — replace the `e.Key == "k" || "d"` glyph-literal destructive check with a structural `Destructive bool` on `keymapEntry` (Report 3-4)
20. `help_modal_frame_test.go:60-125` — fold `TestHelpModalDividerJoined` into the subsuming `TestHelpModalDividerConnectsToBorders` (Report 3-4)
21. `cmd/bootstrap_progress.go:265` — defensive nil-guard (or boundary contract comment) on `fatalMsgFromEvent`'s `ev.Fatal.Error()` deref (Report 5-6)
22. `cmd/concurrent_coldboot_integration_test.go:236-238` — replace per-receive goroutine spawning with one long-lived draining goroutine to avoid churn and the blocked-on-fatal leak (Report 5-8)
23. `modal.go:32-34` — `placeModalOnClearedCanvas(panel, width, height)` threads a w/h int pair everywhere; a typed `{w,h}` value would prevent transposition (only if such a type already exists) (Report 8-6)

### Ideas

24. Spec-conformance decisions to record (each contradicts literal spec/plan without a recorded decision)
    - Signpost on-band text token: reconcile to `text.strong` (verifying it clears 4.5:1 on the `bg.selection` tint) **or** amend §11.3 to record `text.on-selection` as the co-tuned token; then update `bytag_signpost_reskin_test.go:74-77` to match (Report 4-3)
    - Phase-1 light `text.detail`/`text.dim` (and `accent.violet`) values vs the §2.9 table — decide which is canonical and reconcile theme.go or the spec (Reports 2-7, 1-4)
    - No-matches glyph `∅` (U+2205) vs spec `⌀` (U+2300) — adopt the spec glyph or record the substitution (Report 2-9)
    - Focused `+ add` slot signals focus by recolouring bare text violet rather than the bordered-box grammar used elsewhere — decide whether to keep or box it (Report 3-9)
25. `internal/prefs/store.go:147-194` — `Load`/`LoadAppearance` each read `prefs.json`; a combined `LoadAll()` would halve launch I/O (the separate-loader form was a deliberate low-risk choice) (Report 1-5)
26. `theme/theme_test.go` — a mechanical role-discipline assertion (`state.green` never on a chip/decoration path) would lock the §2.9 green/red/faint reservation the way the hex guard locks literal-hex (Report 1-3)
27. Boundary-hardening calls: `model.go:3761` (`padLineToCanvasWidth`) defensively clamp the over-width branch to `contentW` rather than relying on upstream name truncation (Report 1-6)
28. Vacuous/optional test coverage: `appearance_detection_test.go:151-157` strengthen the COLORFGBG never-read test only if/when COLORFGBG is wired (Report 1-7); `keymap_dispatch_guard_test.go:108-112` strengthen the `^↑/↓` paging probe to assert the bound keys / drive a real page change (Report 7-3); a committed single-page dot-suppression capture (Report 2-5)
29. Small ergonomic/structural calls: `capturetool/main.go:126` shared NO_COLOR-convention helper (Report 1-8); `bootstrap_progress.go:159-176` remove duplicate ctx threading on `send()` (Report 5-2); `session_item.go:263-264` / `:394` + `project_item.go:141` heading-token symmetry helper and lazy `selectorStyle` construction on unselected rows (Reports 2-7, 6-3); `modal_footer_test.go:187` assert live footer producers, not just the helper (Report 6-1); `rename_modal.go:171` inline the last per-modal footer wrapper (Report 7-2)
30. Glyph/legibility judgment calls: loading wordmark renders "PORTALI" (violet caret flush against the L) — decide whether the caret needs a gap (Reports 5-6); preview help `←→` glyph compresses in JetBrains Mono — decide whether a help-body HelpKey override reads clearer (Report 4-7)
31. Minor literals/phrasing: `header_test.go` header-scoped nav/filter parity guard (Report 2-2); `footer.go:236` `fitLeftCluster` O(n²) re-render (negligible at ≤6 entries) (Report 2-4); `session_item.go:43` hard-coded `countSlotWidth` (Report 2-6); `empty_states.go:45` empty-projects "open-a-directory" phrasing (Report 4-5)

### Bugs

32. **Narrow-width overflow cluster** (the §2.7 "never overflow" invariant, which the spec scopes to name truncation — these edges sit just outside it; none trigger on normal data, each has a known one-location fix)
    - `section_header.go:96-97` — the left cluster (long inside-tmux `(current: <longname>)` at an extreme-narrow width) overflows because only the right hint is clamped; `ansi.Truncate` the left cluster in the `leftWidth >= w` branch (Report 2-3)
    - `notice_band.go:294-304` (`renderCommandBand`, the §11.4 banner) — a long pending command never wraps/truncates; `fillCanvas` does not truncate over-width lines, so the terminal soft-wraps and can push the view past `termH`. Parity with the pre-reskin plain status line. Truncate the chip with `…` or route through the wrapping `renderNoticeBand` path (Reports 4-1, 4-4)
    - `edit_modal.go:365` / `panel.go:59` — the joined panel auto-sizes to its widest row, so an extreme chip value/count grows the panel past the canvas width; clamp/elide the chip band to the canvas-bounded width (Report 3-9)
33. `internal/tui/bootstrap_warnings.go:68-87` — the only observable v1→v2 non-parity: on the warm/CLI-staged path soft warnings now surface at alt-screen teardown rather than mid-run (v2 removed the `ExitAltScreen`/`EnterAltScreen` commands). Content/order/single-flush unchanged; the cold/TUI path surfaces them in-TUI as a notice band. Not expressible in v2 as a command; flagged for awareness, no action required unless mid-run surfacing is a hard requirement (Reports 1-2, 5-7)
34. `cmd/state_daemon_integration_test.go:178-179,254-256` — this pre-existing daemon-spawning test isolates via raw `t.TempDir()` + `PORTAL_STATE_DIR` rather than `portaltest.IsolateStateForTest(t)`, bypassing the XDG_CONFIG_HOME scrub and the fingerprint-diff backstop CLAUDE.md mandates for daemon-spawning tests. Pre-existing, not introduced by 5-8; flagged because 5-8's discipline criterion says "every daemon-spawning test" (Report 5-8)
