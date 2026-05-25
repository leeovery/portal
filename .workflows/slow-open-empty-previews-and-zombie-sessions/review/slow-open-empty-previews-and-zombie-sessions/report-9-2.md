TASK: 9-2 — Add tmux.SaverPanePIDOrAbsent helper centralizing any-error-to-absent rule

STATUS: Complete

SPEC CONTEXT: Phase 9 analysis-cycle refactor. Bootstrap step 4 (orphan-sweep adapter) and Component D (saver-membership probe) independently consumed rich-sentinel `SaverPanePID` and re-derived sentinel collapse. Tighten tri-state seam contract at type level.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/saver_pane_pid.go:48-68` — unexported `saverPanePID` (rich-sentinel primitive)
  - `internal/tmux/saver_pane_pid.go:91-100` — exported `SaverPanePIDOrAbsent` wrapper
  - `internal/bootstrapadapter/orphan_sweep.go:41-43` — Component B adapter delegates
  - `cmd/state_daemon.go:102-111` — Component D `defaultSaverMembershipProbe` delegates (err|!present → absent)
  - `internal/tmux/export_test.go:30-38` — test-only re-export of `saverPanePID`
- Wrapper logic matches acceptance shape exactly via `errors.Is` on ErrNoSuchSession and ErrEmptyPaneList
- Both required call sites delegate; T10-3 (commit deea01a7) made `SaverPanePIDOrAbsent` sole production entry point

TESTS:
- Status: Adequate
- `saverPanePID` primitive has six-shape coverage at `internal/tmux/saver_pane_pid_test.go:25` `TestSaverPanePID`
- Wrapper exercised through Component D integration (`composition_e2e_self_eject_integration_test.go:270`) and orphan-sweep adapter integration
- No dedicated unit test pins wrapper's three-shape collapse directly; integration coverage reasonable for 10-line wrapper

CODE QUALITY:
- Project conventions: Followed; `=`-prefix exact-match
- SOLID: Good; thin single-responsibility shim
- Complexity: Low; 10-line wrapper
- Modern idioms: multi-`%w` on parse failure; `errors.Is` at wrapper boundary
- Readability: Good; tri-state return documented

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Focused 3-case table unit test for `SaverPanePIDOrAbsent` (ErrNoSuchSession→(0,false,nil); ErrEmptyPaneList→(0,false,nil); generic err→(0,false,err)) would pin collapse contract independently
