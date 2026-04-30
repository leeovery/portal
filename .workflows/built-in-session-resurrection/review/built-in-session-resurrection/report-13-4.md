# Review Report: built-in-session-resurrection-13-4

**TASK**: Use `state.SanitizePaneKey` for round-trip hook fixtures and tighten the marker-suppression test scope godoc

**ACCEPTANCE CRITERIA**:
- Replace hard-coded paneKeys (e.g. "alpha:0.0") in round-trip test fixtures with `state.SanitizePaneKey(name, win, pane)` calls so `saveBase != 0` cannot silently drift.
- Tighten the marker-suppression integration test scope godoc to clarify it is Restore-side only (no behaviour change).

**STATUS**: Complete

**SPEC CONTEXT**:
Cycle-6 documentation/correctness remediation. Spec § "What Survives a Reboot" + § "Save-Side Architecture → Triggers & Serialization → Properties → Restoration guard" pin the contract: paneKey is a derived structural identifier whose canonical form is `state.SanitizePaneKey` for FIFO/scrollback/skeleton-marker namespaces, and `tmux.PaneTarget` (colon form) for hook-store keys.

**IMPLEMENTATION**:
- Status: Implemented
- Locations:
  - `cmd/bootstrap/reboot_roundtrip_test.go:286-287` — `hookPaneKey := state.SanitizePaneKey("alpha", cfg.saveBase+0, cfg.savePaneBase+0)`.
  - `cmd/bootstrap/reboot_roundtrip_test.go:215-216` — `savedHookKey := tmux.PaneTarget("alpha", cfg.saveBase+0, cfg.savePaneBase+0)` (drives the hooks.Store key, which uses the colon form).
  - `cmd/bootstrap/reboot_roundtrip_test.go:503` — `key := state.SanitizePaneKey(sess.Name, w.Index, p.Index)` in captureAndCommit's per-pane scrollback loop.
  - `internal/restore/integration_full_test.go:161, 260` — `state.SanitizePaneKey(fx.name, w, p)` in scrollback fixture seeding and ANSI verification loops.
  - `internal/restoretest/restoretest.go:182` — DriveSignalHydrate uses `state.SanitizePaneKey(session, p.Window, p.Pane)`.
  - `cmd/bootstrap/phase5_marker_suppression_integration_test.go:50-70` — godoc rewritten with explicit `SCOPE — this test exercises Restore-side write discipline only` block plus a `What this test deliberately does NOT cover` section.
- Notes: Distinction between SanitizePaneKey and tmux.PaneTarget preserved correctly. Hook-store keys are tmux.PaneTarget; FIFO/scrollback/skeleton-marker namespaces are SanitizePaneKey. Both keys derived from `cfg.saveBase` / `cfg.savePaneBase`.

**TESTS**:
- Status: Adequate
- Coverage: Round-trip behaviour exercised by `TestPhase5RebootRoundTripEndToEnd` (saveBase=0/0, restoreBase=0/0) and `TestPhase5RebootRoundTripBaseIndexDrift` (saveBase=0/0, restoreBase=1/1). The drift sub-test would silently regress if a hard-coded paneKey leaked back in.
- Notes: Task 13-4 is itself a refactor + godoc-only change; behaviour is already exercised by existing round-trip suite.

**CODE QUALITY**:
- Project conventions: Followed.
- SOLID: Good. SanitizePaneKey vs PaneTarget distinction is documented in-source where it matters.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Comment at lines 205-214 explicitly documents why hook-key formatter differs from SanitizePaneKey.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- None.
