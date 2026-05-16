# Implementation Review: Enter Attaches From Preview

**Plan**: enter-attaches-from-preview
**QA Verdict**: Comments Only

## Summary

All 19 tasks across the four phases (2 feature phases + 2 analysis-cycle cleanup phases) are implemented, tested, and conform to the specification. The four-call attach sequence is correctly authored as a three-call pre-select pipeline that emits a `previewAttachSelectedMsg`, with the connector handoff intentionally pushed post-TUI in `processTUIResult` (a deliberate Phase 3 restructure to fix an inside-tmux orphan-process defect surfaced during analysis). The exact-match `=` prefix is applied uniformly across HasSession / SelectWindow / SelectPane / SwitchClient / AttachConnector with a dedicated prefix-collision regression test. The Phase 2 flash infrastructure (state, render, tick, keystroke clear, dispatch, rapid-bail replacement) composes correctly through the generation-counter mechanism, verified end-to-end. Test coverage is thorough and proportionate throughout. No blocking issues. Several minor non-blocking notes — mostly stale docstrings and small follow-up ideas.

## QA Verification

### Specification Compliance

Implementation aligns with the specification on every load-bearing surface:

- Enter binding intercepts `tea.KeyEnter` in `previewModel.Update` and does NOT forward to viewport (internal/tui/pagepreview.go:282-300).
- Pre-select sequence uses raw tmux indices from `currentRawIndices()` (no slice-position math).
- `has-session` discriminator (`HasSessionProbe`) correctly separates `*exec.ExitError` (bail) from OS-layer errors (proceed).
- Exact-match `=` prefix applied uniformly across the five spec-pinned argv sites; `PaneTarget` (no prefix) correctly retained as hooks.json key formatter.
- Bail flash text is byte-exact: `session "<name>" no longer exists` (verified by `TestPreviewAttachBail_SetsFlashWithExactSpecWording`).
- Flash render-frame ordering uses `tea.Batch` (not `tea.Sequence`) so flash render is not gated on refresh completion.
- Chrome line includes `enter attach` token in the spec-pinned position, byte-identical across viewport content states.
- Rapid-bail replacement contract proven end-to-end via generation-counter integration tests.

Deviation noted (acceptable): Phase 3 task 3-1 restructured the pipeline to hand off the connector post-TUI rather than from inside `tea.Cmd`. This is a design improvement that fixes a previously latent inside-tmux orphan-process bug and is documented inline in `preview_attach.go:42-65`.

### Plan Completion

- [x] Phase 1 acceptance criteria met (8 tasks: Enter binding + pre-select + attach pipeline)
- [x] Phase 2 acceptance criteria met (6 tasks: flash infrastructure end-to-end)
- [x] Phase 3 acceptance criteria met (2 tasks: connector handoff restructure + shared teardown helper)
- [x] Phase 4 acceptance criteria met (3 tasks: previewLogger close, alias deletion, test helper extraction)
- [x] All 19 tasks completed
- [x] No scope creep

### Code Quality

No issues found. Project conventions (no `t.Parallel()`, Commander seam, DI-via-option) are followed throughout. New helpers (`PaneTargetExact`, `HasSessionProbe`, `previewAttachPipeline`, `exitPreviewToSessions`, `setFlash`/`clearFlash`, `flashTickCmd`, `isActionableKey`, `formatSessionGoneFlash`, `newSinglePaneEnumerator`) all carry single responsibilities. Godoc on the new surfaces consistently cites spec sections.

### Test Quality

Tests adequately verify requirements. Notable strengths:

- `TestHasSessionUsesExactMatchPrefix` (tmux_test.go:384-446) — documentary regression with a fake commander simulating tmux's exact-match resolution; proves dropping the `=` prefix would prefix-match `foo-2` instead of the killed `foo`.
- `TestPreviewEnter_DispatchesWithRawTmuxIndicesOnNonContiguousSession` — uses `WindowIndex=5, PaneIndices=[3]` to prove slice-position math is not used.
- `TestConnectInvokedAfterQuit` (`preview_attach_pipeline_handoff_test.go`) — load-bearing regression guard for the inside-tmux orphan-process defect.
- `sessions_flash_replacement_test.go` — end-to-end integration coverage of the rapid-bail replacement contract through five scenarios.
- Generation-guard tests prove both the stale-tick-no-op and the live-tick-clear semantics.

No over-testing. No load-bearing under-coverage.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. internal/tui/model.go:42-44 — `PreviewAttacher` godoc says `Run returns a tea.Cmd that executes the four-call sequence end-to-end`. After Phase 3, the pipeline executes only the three pre-select calls; the connector handoff is post-TUI. Reword to "three-call pre-select sequence; emits a selected/bail envelope; connector handoff is post-TUI in `processTUIResult`."
2. internal/tui/pagepreview.go:248-251 — `Update` godoc says Enter "dispatch[es] the four-call pre-select + attach pipeline". Same correction.
3. internal/tui/model.go:503-508 — `WithPreviewAttachPipeline` docstring says "closing over *tmux.Client + the resolved SessionConnector + a nullable *state.Logger" — the connector reference is stale. Drop the connector mention.
4. internal/tmux/tmux.go:496-511 — `PaneTargetExact` godoc enumerates "SelectPane, ResizePaneZoom" as callers. Broaden to "callers issuing a `-t` flag at the pane level" to reduce doc-drift hazard.
5. internal/tui/model.go:1729-1733 — comment claims `flashRowStyle is constructed fresh per render`, but it's a package-level `var` constructed once. Rewrite the comment ("package-level immutable style; lipgloss value semantics prevent mutation bleed") or move construction into `renderFlashRow` to match the existing wording.
6. internal/tui/model.go:1739 — `renderFlashRow` uses pointer receiver `(m *Model)` but performs no mutation. Could be value-receiver for consistency.
7. cmd/open.go:432 — `defer previewLogger.Close()` placed BEFORE the err-check on lines 433-435. Functions correctly but reads unconventionally — Go idiom is err-check first, then defer.

### Ideas

8. The exact-match `=` prefix is hardcoded inline at five sites. A single helper (e.g. `exactTarget(session string) string`) would centralise the policy.
9. internal/tmux/tmux.go:314-320 — `KillSession` still passes `name` bare to `-t`. Plan suggested uniform application. Same hazard shape — killing intended `foo` could prefix-match a coexisting `foo-2`. Worth a follow-up to make `=` truly uniform across every `-t <session>` site.
10. `RenameSession` similarly bare. Same reasoning, same follow-up candidate.
11. internal/tui/preview_attach_test.go could add an explicit allowlist assertion on `tm.calls` verbs to lock in the no-enumeration acceptance criterion against future fake-API expansions.
12. No cmd/open_test.go smoke test asserts `openTUI` constructs a non-nil `cfg.previewAttacher`. Would catch a future wiring regression at the cmd boundary.
13. `exitPreviewToSessions` mutates the receiver AND returns a cmd. Fine for two internal callers, but if a third caller emerges, splitting into a pure transition helper + refresh-cmd factory would reduce the capture-before-call footgun surface.
14. If a third caller of "insert chrome row after list title" appears (Sessions flash, Projects command-status, future N), extract a shared `insertChromeRow(listView, row string) string` helper.
15. FileBrowser/Preview isolation tests for the flash row would lock structural-impossibility against future View-dispatch refactors.
16. Helper name in task 4-3 diverges from plan task title (`singlePaneGroups()` vs `newSinglePaneEnumerator()`). The chosen name is arguably better, but if the planning system tracks helper names verbatim downstream, a one-line note in the implementation log would tighten plan-to-impl traceability.
