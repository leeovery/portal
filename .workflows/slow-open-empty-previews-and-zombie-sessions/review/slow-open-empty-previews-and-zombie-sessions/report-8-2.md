TASK: 8-2 — Collapse SaverPanePID and FirstPanePIDInSession into one helper

STATUS: Complete (architecturally improved by T10-2/T10-3)

SPEC CONTEXT: c2 Task 2 — duplicated primitive integration hazard: `tmux.SaverPanePID` (T5-2) and `tmux.Client.FirstPanePIDInSession` (T4-4) both ran `list-panes -t =<name> -F '#{pane_pid}'` with divergent error contracts. Both call sites collapse every error to "absent".

IMPLEMENTATION:
- Status: Implemented (improved by cycle-5 T10-2/T10-3)
- Location:
  - `internal/tmux/saver_pane_pid.go:48-100` — primitive (`saverPanePID`) + exported `SaverPanePIDOrAbsent` centralizing "any-error → absent" rule
  - `internal/bootstrapadapter/orphan_sweep.go:38-46` — adapter forwards `tmux.SaverPanePIDOrAbsent` verbatim into core's tri-state seam
  - `cmd/bootstrap/orphan_sweep.go:77,153-162` — core consumes widened `func() (pid int, present bool, err error)` seam with explicit `case !saverPresent:` arm
- Original T8-2 said adapter would call `errors.Is(ErrNoSuchSession/ErrEmptyPaneList) → absent`. T10-2 (commit 346e935f) widened seam so sentinel collapse happens inside `SaverPanePIDOrAbsent` returning tri-state. T10-3 (commit deea01a7) unexported `saverPanePID`. Architecturally superior — encoded at type level rather than caller discipline
- `FirstPanePIDInSession` fully removed from production
- `HasSession` pre-check gone — no `HasSession` references in `internal/bootstrapadapter`

TESTS:
- Status: Adequate
- `internal/tmux/saver_pane_pid_test.go` covers all six observable shapes: success / `ErrNoSuchSession` / `ErrEmptyPaneList` (bare + whitespace) / `ErrPanePIDParse` / multi-line first-non-empty / generic exec-error
- `SaverPanePIDOrAbsent` lacks focused unit test but single sentinel-collapse branch is straightforward `errors.Is` plumbing over a pinned primitive

CODE QUALITY:
- Project conventions: Followed; adapter in own file; test re-exports gated
- SOLID: Good; single responsibility per layer; unexport enforces centralized collapse policy
- Complexity: Low; linear
- Modern idioms: `errors.Is`, `%w` wrapping
- Readability: Good; godoc on all three layers explains contract

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Drift resolved in downstream cycle (T10-2/T10-3) not directly under T8-2; consider annotating T8-2 in planning.md with "superseded by T10-2"
