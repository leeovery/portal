# Architecture Analysis — Cycle 3 (independent re-scan)

STATUS: findings
FINDINGS_COUNT: 4

## Finding 1: Saver-pane PID "treat any error as absent" rule reimplemented at every call site

SEVERITY: medium

FILES:
- `internal/tmux/saver_pane_pid.go:44`
- `internal/bootstrapadapter/orphan_sweep.go:130-139`
- `cmd/state_daemon.go:99-108`

DESCRIPTION: The "what PID owns the `_portal-saver` pane?" question is asked from three sites. Each decodes `tmux.SaverPanePID`'s rich return contract differently: the B adapter collapses `(ErrNoSuchSession, ErrEmptyPaneList) → (0, nil)`; the D probe collapses *all* errors to `false`. The "treat any error as absent" rule lives in prose comments rather than in a single typed helper.

RECOMMENDATION: Add a single helper in `internal/tmux` (e.g. `SaverPanePIDOrAbsent(c, name) (pid int, present bool, err error)`) that owns the sentinel-collapse and consume it from both consumers.

## Finding 2: Saver-side seam structs sprawl into five package-level mutables without composing shared primitives

SEVERITY: low

FILES:
- `internal/tmux/portal_saver.go:101-267`

DESCRIPTION: `SaverSharedSeams`, `SaverBarrierSeams`, `SaverReadinessSeams`, `SaverVersionSeams`, and `SaverOperationSeams` split the saver-side surface into five package-level seam vars. `SaverSharedSeams` exists precisely because Barrier and Readiness both need `ReadPID + IdentifyDaemon` — yet the structs remain peers rather than embedded. Tests must know to swap fields across two structs.

RECOMMENDATION: Embed `SaverSharedSeams` into `SaverBarrierSeams` and `SaverReadinessSeams` so each consumer references one struct; or collapse all five into a single `SaverSeams` with grouped sub-fields. Polish-grade — only worth doing if the file is opened for unrelated work.

## Finding 3: NewIsolatedStateEnv mutates caller-process env as a load-bearing side effect hidden behind a `New*` name

SEVERITY: medium

FILES:
- `internal/portaltest/isolated_env.go:56-66`

DESCRIPTION: The exported `NewIsolatedStateEnv` name suggests a pure constructor, but the very first action is `t.Setenv("HOME", t.TempDir())` and `t.Setenv("XDG_CONFIG_HOME", "")` — mutating the calling test process's globals. T8-9 added a "SIDE EFFECT" docstring leader. Architectural concern: the API SHAPE still misleads at the call site — a contributor reading `env, stateDir := portaltest.NewIsolatedStateEnv(t)` has no syntactic cue that HOME just changed.

RECOMMENDATION: Either rename to communicate the mutation (`IsolateStateForTest(t)`, `MustIsolateStateDir(t)`), or split into a side-effecting `ScrubHostEnv(t)` plus a pure `BuildIsolatedEnv(t)`.

## Finding 4: Pgrep enumeration maintained as two independent implementations across production adapter and test helper

SEVERITY: low

FILES:
- `internal/bootstrapadapter/orphan_sweep.go:75-109`
- `internal/portaltest/pgrep.go:37-68`

DESCRIPTION: Both shell out via `exec.Command("pgrep", "-fx", state.PortalDaemonArgvPattern)` with identical exit-1-empty-stdout handling and identical per-line `strconv.Atoi` skip behaviour. The shared regex constant pins the pattern, but the run/parse logic is duplicated.

RECOMMENDATION: Extract a single `state.PgrepPortalDaemons() ([]int, error)` and have both consumers wrap it.

(Overlaps with Duplication Finding 1 — same root cause.)
