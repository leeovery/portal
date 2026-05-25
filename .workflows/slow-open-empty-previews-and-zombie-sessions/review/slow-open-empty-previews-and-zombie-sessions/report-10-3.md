TASK: 10-3 — Unexport tmux.SaverPanePID since SaverPanePIDOrAbsent is the sole production entry point

STATUS: Complete

SPEC CONTEXT: c4 architecture Task 3. Since T9-2 introduced `SaverPanePIDOrAbsent` centralizing "any-error → absent" sentinel collapse, every production consumer routes through it. Keeping lower-level `SaverPanePID` exported invited bypass. Unexport structurally enforces sole external entry point.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/saver_pane_pid.go:48` — `saverPanePID` (lowercased)
  - `internal/tmux/saver_pane_pid.go:91` — `SaverPanePIDOrAbsent` unchanged
  - `internal/tmux/export_test.go:38` — test-only re-export `var SaverPanePID = saverPanePID`
- Production callers verified using OrAbsent:
  - `cmd/state_daemon.go:106` (Component D probe)
  - `internal/bootstrapadapter/orphan_sweep.go:42` (Component B adapter)
  - `cmd/bootstrap/composition_e2e_self_eject_integration_test.go:270` (integration test)
- Grep for `tmux\.SaverPanePID\b` returns only `internal/tmux/saver_pane_pid_test.go` hits via `export_test.go` test alias; no production matches
- Godoc on `saverPanePID` updated to internal framing
- Test re-export godoc explains why rich-sentinel form remains reachable for unit tests

TESTS:
- Status: Adequate
- All seven existing `TestSaverPanePID` subtests at `internal/tmux/saver_pane_pid_test.go` continue to drive rich-sentinel classifications via test re-export
- `SaverPanePIDOrAbsent`'s three-shape behavior covered indirectly by Component D / B consumer tests

CODE QUALITY:
- Project conventions: Followed; test-only re-export via `export_test.go` is canonical project idiom
- SOLID: Good; interface segregation tightened — one entry point with one contract (tri-state)
- Complexity: Low; zero logic change; signature surface narrowed
- Modern idioms: Yes
- Readability: Good; three docstrings, one consistent rationale chain

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
