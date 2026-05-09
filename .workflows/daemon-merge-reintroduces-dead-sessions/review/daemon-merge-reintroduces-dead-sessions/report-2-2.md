TASK: Add paneKey normalisation correctness fixture (2-2)

ACCEPTANCE CRITERIA:
- Unit fixture mixing tmux `session:window.pane` form with canonical `session__window.pane` form, asserting same logical pane recognised across both sides.
- Negative test: two paneKeys differing only by separator must NOT be treated as equivalent.
- Edge cases: rightmost-`:` split; `strconv.Atoi` for window/pane; positive recognition; negative non-equivalence.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix Component B (Adapter Wiring → PaneKey conversion) and §Testing Requirements require live-pane line conversion via `state.SanitizePaneKey` BEFORE diffing. Test must guard against three regressions: dropping conversion, applying to wrong side, naive string equality.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/stale_marker_cleanup_test.go:235-322` — `TestStaleMarkerCleanup_PaneKeyNormalisation` with three sub-tests. Production parser at `cmd/bootstrap/stale_marker_cleanup.go:177-210` (`parseLivePaneSet`).
- Notes: Fixture exercises contract through public `MarkerCleanupCore.CleanStaleMarkers` with real `state.SanitizePaneKey`.

TESTS:
- Status: Adequate
- Coverage:
  - **Positive recognition** (236-261): marker canonical via `SanitizePaneKey("my-session", 0, 1)` → `"my-session__0.1"`; live `"my-session:0.1"`; expects 0 unsets.
  - **Negative non-equivalence guard** (263-293): marker raw `"my-session:0.1"`; live raw same form. After live-side sanitisation, raw marker absent → exactly 1 unset call.
  - **Rightmost-colon split** (295-321): session `host:1234`; marker canonical `"host:1234__0.0"`; live `"host:1234:0.0"`. Rightmost-`:` recovers correctly; expects 0 unsets.
- Notes: All three exercise production `parseLivePaneSet` (rightmost-colon, dot split, `strconv.Atoi`, `SanitizePaneKey`) via `CleanStaleMarkers`. `strconv.Atoi` parse-failure variants well-covered in sibling `TestStaleMarkerCleanup_SoftWarningPosture`.

CODE QUALITY:
- Project conventions: Followed. `t.Run`, no `t.Parallel()`.
- SOLID: Good.
- Complexity: Low (~20-25 lines per sub-test).
- Modern idioms: Yes. Uses `state.SanitizePaneKey` to derive expected canonical form.
- Readability: Good. Sub-test 2's comment makes regression target unmistakable.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Sub-test 3 uses `host:1234`; consider future fixture pairing rightmost-split with sanitiser-handled colon. Optional.
- [idea] Sub-test 3 name reads as describing the parser; alternative framing could tighten contract framing. Cosmetic.
