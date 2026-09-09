TASK: resume-hooks-silently-lost-9-7 — pane_key Names Two Different Values Inside One Daemon Loop Body

ACCEPTANCE CRITERIA:
- Every `pane_key` attr emitted from the daemon capture loop carries a `SanitizePaneKey` value; none carries a `session:window.pane` coordinate.
- The `capture pane failed` WARN for a given pane carries the same `pane_key` value as that pane's `pane captured` DEBUG and its `write scrollback failed` WARN.
- The failed-capture WARN still carries its `error` attr unchanged.
- No new attr key is added to the closed vocabulary.

STATUS: complete

SPEC CONTEXT:
This is a phase-9 implementation-analysis task, so its own body is its authority. The closed attr vocabulary it defers to is the observability specification's attr table, which defines `pane_key` as "Structural pane key (canonical persisted form, e.g. `session__window.pane`)"
(.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md:195) — i.e. the sanitized key, not a tmux `-t` coordinate. The change moves the one divergent emission onto that documented meaning rather than away from it. The `resume-hooks-silently-lost` specification touches `pane_key` only for restore's `set pane token failed` WARN (specification.md:100), which is a different emission in a different package and is untouched here.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:287 (the `capture pane failed` WARN now logs `"pane_key", paneKey`); the sanitized key is computed once at cmd/state_daemon.go:272 and is the value on all four capture-loop emissions — cmd/state_daemon.go:277 (`pane captured`), :283 (`pane vanished`), :287 (`capture pane failed`), :293 (`write scrollback failed`). Commit 5a24ab4d.
- Notes:
  - The `coord := tmux.PaneTarget(...)` recomposition and its two-line comment are gone from the loop (confirmed against the commit diff and the current file: `grep -n "coord\|PaneTarget" cmd/state_daemon.go` returns only the `tmux.PaneTargetExact` capture target at :278, which is the read's addressing form and is not logged).
  - `tmux.PaneTarget` is not orphaned by the removal: it retains production callers at internal/tmux/tmux.go:825 and :839.
  - Sweep criterion holds. The only `pane_key` emissions reachable from a tick are the four loop lines above plus `internal/state/commit.go:108` (`gc remove scrollback failed`), whose value is the scrollback filename minus `.bin` — the sanitized key by construction. `cmd/state_hydrate.go:243` and `cmd/bootstrap/stale_marker_cleanup.go:83` are outside the capture loop and both already carry sanitized keys.
  - No attr key was added; the WARN's attr set shrank from nothing (same two keys, one value changed).
  - The two per-pane WARNs continue to emit through `deps.Logger` (component `daemon`) while the DEBUGs emit through `captureLogger` (component `capture`). That split is pre-existing and deliberately pinned — cmd/state_daemon_cycle_summary_test.go:212 asserts "per-pane WARN stays on daemon" — so it is not drift introduced here.

TESTS:
- Status: Adequate
- Coverage: cmd/state_daemon_capture_logging_test.go:148 `TestDaemonTick_CapturePaneFailureWarn` carries all three named cases:
  - :170 pins the sanitized key on the failed-capture WARN (`work__0.0`), which is the assertion that flipped — it was `pane_key=work:0.0` before.
  - :177 drives a two-pane fixture (one pane fails its capture, the other fails its scrollback write via `breakScrollbackDir`) and asserts the exact ordered list of every `pane_key`-bearing record in the sink, so a future emission introducing a second value shape fails rather than passes unnoticed. The list is deterministic: the loop walks slices, and `Commit`'s only failure line on this fixture (`gc orphan scrollback failed`, internal/state/commit.go:40) carries no `pane_key`.
  - :216 re-pins the `error` attr through `errors.Is` against the sentinel — this absorbed the deleted `TestDaemonTick_CapturePaneFailureErrorAttrIsWrappedError`, so criterion 3 did not lose its guard.
- Notes:
  - The rewrite also dropped the old test's `component=daemon` assertion on this WARN, but that property is still pinned at cmd/state_daemon_cycle_summary_test.go:212 — no net coverage loss.
  - Overlap between :170 and :177 is real but thin (:177 subsumes the single value, :170 names the failure mode in isolation) and both were explicitly requested by the task's Tests list. Not over-tested: no new mocks, the fixtures reuse the existing `daemonFakeCommander` and the shared `makeCaptureDeps` / `breakScrollbackDir` helpers rather than hand-rolling a `daemonDeps`, which the old version did.
  - Lane rule respected: unit-lane test, no binary built, no daemon spawned, `PORTAL_STATE_DIR` pinned to `t.TempDir()`, no `t.Parallel()`.

CODE QUALITY:
- Project conventions: Followed. Attr key and value shape now match the closed vocabulary's own definition; no call-site invention.
- SOLID principles: Good — one already-computed value serves the whole loop body.
- Complexity: Low. The change removes a local and a call from the failure branch.
- Modern idioms: Yes. The test uses `slices.Equal` over the collected emissions and the chained `logtest` query filters (`Records().WithMessage(...).AtExactLevel(...).Only(...)`) rather than substring-matching a rendered body, which is the direction CLAUDE.md documents for that package.
- Readability: Good. The `failedCaptureWarn` closure removes the duplicated fixture that two of the three subtests need.
- Comment accuracy: The surviving comments hold. The test doc at cmd/state_daemon_capture_logging_test.go:143-147 states what the emission now does; the in-body note at :196-197 ("one pane cannot both fail its capture and reach its write") correctly explains why the fixture uses two panes. The deleted comment at the old `coord` site described code that no longer exists and went with it.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
