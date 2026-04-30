# Review Report: built-in-session-resurrection-12-4

**TASK**: Expand task 3-13 — Phase 3 integration test multi-session/ANSI/marker coverage

**ACCEPTANCE CRITERIA**:
- Two sessions × multi-window × multi-pane fixture
- One zoomed pane preserved (`window_zoomed_flag` round-trip)
- ANSI SGR bytes byte-equal (per task 12-4 edge-case wording)
- Per-session environment round-trip (`show-environment`)
- Active-pane round-trip per window
- `@portal-restoring` marker cleared post-hydrate
- `@portal-skeleton-<paneKey>` markers cleared after `signal-hydrate` + helper dump
- Test gated by integration tag

**STATUS**: Complete (with one drift note on byte-equal vs. contains assertion)

**SPEC CONTEXT**:
- Spec "Phase 3 Acceptance" / "Scrollback Restore Mechanics → Validation Reference": isolated `tmux -L`/`-S` round-trip covering structure + ANSI scrollback fidelity.
- Spec "Save Format & Schema → Index Semantics and base-index / pane-base-index": structural relationships preserved.
- Spec "Restore-Side Architecture → Marker Coordination": both markers covered via `restoreWithMarker` and `WaitForSkeletonMarkersCleared`.
- Spec "Scrollback Restore Mechanics → Helper Behavior on Startup": helper unsets its skeleton marker after the 100ms settle sleep — `WaitForSkeletonMarkersCleared(... 10s)` covers this.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/restore/integration_full_test.go` (new file, ~510 lines, gated `//go:build integration`)
- Notes:
  - Builds production `portal` binary and prepends to PATH so the in-pane hydrate helper resolves (mirrors `cmd/bootstrap/reboot_roundtrip_test.go`).
  - Two `fixtureSession`s with asymmetric zoomed window / active-pane choices catch swap regressions.
  - Scrollback fixture seeded *to disk* AFTER capture, BEFORE persist — correct ordering with rationale in comments.
  - `restoreWithMarker` from `integration_test.go` reused (DRY) and asserts the `@portal-restoring` set+defer-clear contract.
  - `state.CaptureStructure` and `state.EncodeIndex` are the canonical entry points — no hand-built JSON.
  - Hooks store path isolated via `PORTAL_HOOKS_FILE` env so helper's `hooks.json` lookup does not touch user config.
  - `WaitForSkeletonMarkersCleared` (10s budget) gates ANSI assertions on every helper having reached its `set-option -su` step.

**TESTS**:
- Status: Adequate
- Coverage: topology shape, live structure, zoom, active pane, per-session env, restoring-marker cleared, skeleton-markers cleared, ANSI scrollback. Each acceptance bullet has a dedicated assertion.
- Notes:
  - Per-pane label encoding (`[fixture <s> w<w> p<p>]`) is a smart cross-wire detection.
  - Most checks use `t.Errorf` rather than `t.Fatalf` so all failures surface in one run.
  - Not over-tested: zero redundant assertions; each helper checks one invariant.

**CODE QUALITY**:
- Project conventions: Followed. Reuses `tmuxtest.New`, `restoretest.BuildPortalBinaryDir`, `restoretest.PrependPATH`, `restoretest.DriveSignalHydrate`, `restoretest.WaitForSkeletonMarkersCleared` — all the shared helpers consolidated in Phase 13. Build tag `//go:build integration`. `t.Helper()` applied in every helper. No `t.Parallel()`.
- SOLID: helper functions each do one thing.
- Complexity: Low. Orchestration body reads top-to-bottom as save→kill→restore→signal→assert.
- Modern idioms: `t.Setenv`, `t.TempDir`, `t.Cleanup`; table-style `[]fixtureSession` iteration.
- Readability: Excellent. Function godocs explain why each assertion matters.
- Issues: none functional.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] Plan task 12-4 edge-case bullet says "ANSI SGR bytes byte-equal", but the integration test uses substring contains (with thorough documented justification in `verifyANSIInScrollback`'s godoc — `capture-pane -e` normalises `\x1b[0m` → `\x1b[39m` when only fg state changed, and reflow padding breaks literal equality). The justification is solid and the substring scheme catches every regression byte-equal would. Consider either (a) recording this clarification in the plan body, or (b) citing the existing `cmd/state_hydrate_test.go` byte-equal check on helper `io.Copy` output in the integration godoc.
- [idea] `TestPhase3Integration_FullRoundTrip` is one top-level test (no sub-tests). Splitting into `t.Run("structure", ...)`, `t.Run("zoom", ...)`, etc. in a future expansion would surface which dimension drifted as a sub-test name.
- [quickfix] In the `fixtureSession` struct: the field comment `cwds [2][2]string // [windowIdx][paneIdx]` is helpful, but `zoomedW`/`zoomedP` could use a one-line clarification that `zoomedP` is a pane index *within* `zoomedW`.
