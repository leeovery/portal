# Standards Analysis — Cycle 5

AGENT: standards
STATUS: clean
FINDINGS_COUNT: 0
FINDINGS: none

SUMMARY: Independent re-scan finds no remaining spec drift; T10-1 / T10-2 / T10-3 (the only post-c4 changes) are conforming refactors.

## Verification notes (deduplicated against c1–c4)

- **T10-1 (`internal/portaltest/spawn_daemon.go`)** — consolidates orphan-daemon spawn + reap test helpers into `portaltest`. Test-only (enforced structurally by the `*testing.T` first parameter per spec § Component G). Bare `"portal"` argv[0] preserved with explicit comment citing the Darwin `comm` truncation requirement that `state.IdentifyDaemon` depends on. Uses `PORTAL_STATE_DIR` for per-call isolation on top of the caller's `IsolateStateForTest`-supplied env, matching the existing override precedence in `internal/state/paths.go`.

- **T10-2 (`cmd/bootstrap/orphan_sweep.go:50-92`)** — tri-state `SaverPanePID func() (pid int, present bool, err error)` correctly distinguishes spec § Component B step 2's two legitimate-set cases: present → singleton legitimate set; absent → empty legitimate set (no warn). Error case preserves "log WARN and proceed" best-effort contract from spec § Component B step 4. Defensive `os.Getpid()` self-skip retained at line 166.

- **T10-3 (`internal/tmux/saver_pane_pid.go`)** — `SaverPanePIDOrAbsent` (exported) collapses `ErrNoSuchSession` and `ErrEmptyPaneList` into absent shape; other errors returned verbatim. Unexported `saverPanePID` keeps rich sentinel surface available for in-package callers. Spec's "treat any error as absent" rule for Component D self-check step 1 correctly applied at Component D probe (verified clean in c4); orphan-sweep adapter surfaces the error per Component B WARN-and-empty-set behaviour.

- **Component A–G** — all spec contracts verified intact. Identity-check→SIGKILL adjacency / 50ms cadence / 1s budget (A); canonical pgrep + literal INFO + nil-return contract (B); pre-check + post-flock fstat/stat cross-check + 3×10ms retry + WARN log level (C); probe-before-tick + N=3 hysteresis + literal INFO + osExit(0) bypassing daemonShutdownFunc (D); typed `tmuxerr.ErrNoSuchSession` sentinel in dependency-free leaf (E); placeholder command + ordering + readiness barrier (F); leaf `internal/portaltest` with `*testing.T` parameter (G).

Conventions verified: no `t.Parallel()` in cmd-package tests; `*Deps` mutable-seam pattern with `t.Cleanup`; `%w` wrapping (multi-`%w` at `wrapNoSuchSession`); integration tests carry `//go:build integration`; leaf-package discipline preserved; 11-step bootstrap sequence documented consistently in `cmd/bootstrap/bootstrap.go` and `CLAUDE.md`.

**Note (not a finding):** the planning-folder audit deliverable `audit-G-test-helpers.md` references the pre-T9-5 helper name `NewIsolatedStateEnv` in its prose. Planning artefact, not production code — doc-hygiene rather than spec drift.

Returning clean — no genuinely remaining gaps.
