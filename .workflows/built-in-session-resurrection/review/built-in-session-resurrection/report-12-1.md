# Review Report: built-in-session-resurrection-12-1

**TASK**: Implement task 5-9 — end-to-end reboot round-trip integration test

**ACCEPTANCE CRITERIA**:
- New integration test at `cmd/bootstrap/reboot_roundtrip_test.go` with `//go:build integration` tag.
- At least two table variants differing in `base-index` / `pane-base-index`.
- Isolated `tmux -L <socket>` — never touches user sessions.
- Socket killed in `t.Cleanup` on pass and fail.
- Save phase: multi-session, multi-window, multi-pane fixture with per-session env, one zoomed pane, deterministic per-pane ANSI scrollback.
- Reboot simulated via `kill-server` + fresh server on the same socket.
- Orchestrator `Run` invoked for both save-warming and post-reboot restore.
- `client-attached` exercised via `tmux attach-session` (or documented fallback).
- `client-session-changed` exercised via `tmux switch-client` (or documented fallback).
- Structure, layout, zoom, CWDs, environment, resume hooks, and ANSI scrollback all verified post-restore.
- Resume hook fires exactly once per skeleton-restored pane.
- `@portal-skeleton-*` markers cleared post-attach.
- SGR/byte-level scrollback comparison.
- Test skipped under `testing.Short()`.

**STATUS**: Complete

**SPEC CONTEXT**:
- Phase 5 acceptance: save → kill-server → fresh server → restore → hydrate must preserve structure, layout, zoom, CWDs, per-session env, resume-hook firing, and ANSI scrollback.
- Spec § "Bootstrap Flow → Attach Flow (After Bootstrap)": `client-attached`/`client-session-changed` → `signal-hydrate` → FIFO unblock → helper exec chain.
- Spec § "Save Format & Schema → Helper hook lookup under index drift": helper invoked with `--hook-key` flag from saved indices so hooks survive base-index drift.
- Spec § "Layout Restoration → Per-Window Restoration Order": window → split × (N-1) → select-layout → select-pane → resize-pane -Z.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/cmd/bootstrap/reboot_roundtrip_test.go` (960 lines)
- Notes:
  - File gated `//go:build integration` (line 1); each test honours `testing.Short()` (lines 122, 154, 763).
  - Three top-level tests: `TestPhase5RebootRoundTripEndToEnd`, `TestPhase5RebootRoundTripBaseIndexDrift`, `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary`.
  - Drift coverage: variant 1 saves+restores at 0/0; variant 2 saves at 0/0 and restores at 1/1.
  - Isolated `tmux -S <socket>` via `tmuxtest.New` (line 228); `t.Cleanup` kills the server.
  - Save phase: alpha (multi-window/pane + zoomed pane + per-session env) + beta; deterministic ANSI fixture written post-`captureAndCommit` (lines 285-291) for byte-stable assertions.
  - Reboot: `ts.KillServer()` + post-kill `list-sessions` sanity (lines 301-304); restore re-runs orchestrator with production `RestoreAdapter`, `RestoringMarker`, `FIFOSweeper` (lines 327-344).
  - `client-attached` pathway: `restoretest.DriveSignalHydrateBinary` exec's the production `portal state signal-hydrate <session>` argv — argv-identical to what the production hook invokes via `run-shell`. Direct-FIFO `DriveSignalHydrate` retained for the drift variant only, with documented rationale (lines 145-152, 378-384). Honours the acceptance bullet's "PTY fragility may require fallback" tolerance.
  - `client-session-changed`: covered by `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` (lines 762-896) — sequences `signal-hydrate alpha` then `signal-hydrate beta`. Decision to not actually attach a real client is explicitly documented at lines 743-752.
  - Verification helpers: `verifyLiveStructure`, `verifyLayoutAndZoom`, `verifyCWDs` (with `EvalSymlinks` darwin normalisation), `verifyEnvironment`, `verifyANSIScrollback` (asserts `\x1b[31m`, "red", "before reboot"), `verifyHookFiredOnce` (`strings.Count`).
  - Skeleton markers cleared verified by `WaitForSkeletonMarkersCleared` (10s budget).
  - `_seed` underscore-prefixed session pattern (lines 236-237, 311-312) cleanly stages base-index changes before alpha/beta creation.

**TESTS**:
- Status: Adequate
- Coverage matrix: every acceptance dimension has a dedicated verifier (see implementation notes above).
- Notes:
  - Sanity guards: `verifyCapturedIndex` (524-559) prevents silent regression where capture mis-classifies a session; `markersBefore` check (846-853) guards against silent no-op when binary-driven sub-test runs.
  - Hook-fired-exactly-once via `strings.Count` on `>>`-appended file is exactly the right shape — both 0 and >1 firings produce distinct diagnostics.
  - Live-pane assertions run BEFORE signal-hydrate (lines 350-363) — comment explains why (post-shell `cd ~` from `.zshrc` would invalidate `pane_current_path`). Thoughtful mitigation.
  - ANSI substring matching documented as choice over byte-exact compare due to capture-pane wrapping/whitespace nondeterminism — fully aligned with acceptance bullet's "byte-level OR SGR-sequence-level" tolerance.

**CODE QUALITY**:
- Project conventions: Followed (no `t.Parallel()`, `t.TempDir`, `t.Setenv`, `t.Cleanup`; helpers via `tmuxtest.Socket`, `restoretest.*`).
- SOLID: Good — single-responsibility helpers; config structs keep call sites readable.
- Complexity: Acceptable — largest function `runRebootRoundTrip` ~230 lines, linear, well-commented.
- Modern idioms: Yes — `t.Setenv`, `t.TempDir`, `errors.Is`, `filepath.EvalSymlinks`.
- Readability: Good — every non-trivial assertion has a "why" comment.
- Issues: None significant.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] In-pane scrollback override (lines 285-291) writes the deterministic ANSI fixture AFTER `captureAndCommit` writes the empty placeholder. Works because the helper reads the file by name at hydrate time, but the in-memory `state.HashMap` for that file is stale vs disk. If a future variant adds a post-hydrate capture-and-commit step, dedup might skip rewriting due to stale hash. Worth a comment if such a variant lands.
- [idea] `verifyCWDs` cases (lines 616-624) hard-code resolution like `cfg.restoreBase+0`, `cfg.restorePaneBase+1`. Could be expressed via a small helper that builds `tmux.PaneTarget` from `(window, pane)` + cfg — minor tightening if the topology grows.
- [quickfix] Comment at lines 452-454 ("Make alpha:w1.p0 the active pane of its window") describes intent but no `select-pane` call follows; comment then notes single-pane windows have their sole pane active. Slightly confusing on first read — drop the dead comment or rephrase to make the no-op intent explicit.
- [idea] The 10s `WaitForSkeletonMarkersCleared` budget (line 391) is documented in `restoretest.go:152-158` as absorbing CI scheduling latency. Acceptable, but worth a TODO linking to CI budget config if runtime variance grows.
