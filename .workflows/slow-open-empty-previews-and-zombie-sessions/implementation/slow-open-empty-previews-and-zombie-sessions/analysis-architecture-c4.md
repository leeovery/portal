# Architecture Analysis — Cycle 4

AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 3

SUMMARY: Three cycles of cleanup have addressed most surface, composition, and seam-naming issues. Three low-severity polish items remain — none block the implementation.

---

## Finding 1: bootstrapadapter.saverPanePID wrapper degrades the tri-state contract from tmux.SaverPanePIDOrAbsent

SEVERITY: low

FILES:
- `/Users/leeovery/Code/portal/internal/bootstrapadapter/orphan_sweep.go:66-75`
- `/Users/leeovery/Code/portal/cmd/bootstrap/orphan_sweep.go:60-67,141-148`

DESCRIPTION: The `OrphanSweepCore.SaverPanePID` seam is shaped `func() (int, error)`. The production adapter wraps the richer `tmux.SaverPanePIDOrAbsent` (which returns `(pid, present, err)`) by collapsing `!present → (0, nil)`. The consumer at orphan_sweep.go:141 then handles three observable shapes — `saverErr != nil` (warn), `saverPID > 0` (legitimate), and the unstated default `(0, nil) = absent` — but the seam's type signature does not encode "0 with nil error means absent". A future implementer returning a real PID of 0 (defensively, on error) would silently flip "absent" into "legitimate empty" with no compile-time signal. The tri-state already exists at the helper layer; flattening it at the seam boundary is information loss for no testability gain (Identify/Kill are seamed separately).

RECOMMENDATION: Either widen the seam to `func() (pid int, present bool, err error)` matching the underlying helper and let the core's switch read `case !present:` explicitly, OR drop the adapter wrapper entirely and inline `tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)` at the seam closure site so the convention lives next to its single owner. Either change deletes one indirection and pins the absent-vs-error distinction at the type level.

EFFORT: small

---

## Finding 2: tmux.SaverPanePID is exported but has no remaining out-of-package consumer

SEVERITY: low

FILES:
- `/Users/leeovery/Code/portal/internal/tmux/saver_pane_pid.go:45-65`

DESCRIPTION: Since the T9-2 promotion of `SaverPanePIDOrAbsent`, every production consumer (Component D probe at cmd/state_daemon.go:106, orphan-sweep adapter at internal/bootstrapadapter/orphan_sweep.go:67) now routes through `SaverPanePIDOrAbsent` rather than the rich-sentinel `SaverPanePID`. The lower-level function remains exported with a fully documented `(int, error)` contract surfacing `ErrNoSuchSession` / `ErrEmptyPaneList` / `ErrPanePIDParse` — sentinels that no remaining out-of-package caller decodes. Keeping it exported invites future consumers to reach past the centralized "any-error → absent" rule by accident.

RECOMMENDATION: Either unexport to `saverPanePID` (callers within the package keep working; `SaverPanePIDOrAbsent` is the sole external entry point), OR add a one-line doc note marking it "low-level — prefer SaverPanePIDOrAbsent" to set the expectation. Unexporting is the structural answer.

EFFORT: trivial

---

## Finding 3: state.daemon_lock seam family uses bare-var idiom while internal/tmux saver-side seams use the SaverSeams struct

SEVERITY: low

FILES:
- `/Users/leeovery/Code/portal/internal/state/daemon_lock.go:26-49`
- `/Users/leeovery/Code/portal/internal/tmux/portal_saver.go:238-272`

DESCRIPTION: T9-6 consolidated five `Saver*Seams` package-level vars into the single `SaverSeams` struct (`saver`) with a uniform setter idiom. The sibling daemon-lock primitive in `internal/state/` retains five independent bare-var seams (`lockAcquire`, `lockAcquireReadPIDFile`, `lockAcquireIdentifyDaemon`, `lockAcquireFstat`, `lockAcquireStat`) that conceptually belong to the same operation (one `AcquireDaemonLock` call). Two adjacent packages now express the same "group of test seams driving one operation" pattern two different ways. Not a defect — the existing pattern is internally consistent and tested — but it is the kind of drift later contributors will copy-paste in either direction.

RECOMMENDATION: Either leave as-is and accept the divergence (the bare-var form has shipped longer and the seam count is small), OR mirror the SaverSeams approach with an `acquireSeams` struct so future seam additions land in one place. Polish-grade — only worth doing if daemon_lock.go is opened for unrelated work.

EFFORT: trivial
