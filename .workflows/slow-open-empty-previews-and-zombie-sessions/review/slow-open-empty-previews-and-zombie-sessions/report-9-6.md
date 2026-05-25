TASK: 9-6 — Embed or collapse the 5 Saver*Seams structs

STATUS: Complete

SPEC CONTEXT: c3#2 architectural concern (low severity). T8-6 created 5 separate Saver*Seams structs with SaverSharedSeams forcing tests to reach across two structs. Goal: cleaner test ergonomics.

IMPLEMENTATION:
- Status: Implemented (option b: single composite SaverSeams)
- Location:
  - `internal/tmux/portal_saver.go:205-231` — SaverSeams composite; shared ReadPID + IdentifyDaemon at top level; Barrier/Readiness/Version/Ops as named sub-structs
  - `internal/tmux/portal_saver.go:238-272` — single saver package-level var with composite literal defaults; init() at 274-285 wires forward-referencing function defaults to break init cycles
  - `internal/tmux/portal_saver.go:132-203` — four sub-cluster types, each documenting "Embedded under SaverSeams.X"
- SaverSharedSeams type fully eliminated from production
- Shared seams promoted to top level (innovation on plan's option (a)/(b)) — strictly cleaner; zero indirection for shared fields
- Production references `saver.X.Field` directly

TESTS:
- Status: Adequate
- `internal/tmux/export_test.go:71-93` — `Saver()` + `SaverBarrier()/SaverReadiness()/SaverVersion()/SaverOps()` struct-pointer accessors for atomic sub-cluster swaps
- `internal/tmux/export_test.go:100-187` — per-field `*Seam()` accessors retained so existing `swapSeam` helper-driven tests continue without churn
- portal_saver_test.go has 67 references to Saver*() accessor pattern; no test references eliminated SaverSharedSeams
- Two accessor shapes is mild API-surface duplication but intentional pragmatism

CODE QUALITY:
- Project conventions: Followed; bare-var idiom replaced by composite struct without disturbing production call-site shape
- SOLID: Good; SRP preserved per sub-cluster; shared primitives correctly promoted; ISP via two accessor shapes
- Complexity: Low; pure data reorganization; no control-flow change
- Modern idioms: Composite literal for static defaults; init() only for cycle-breaking
- Readability: Good; heavy doc comments; init() rationale explicit

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Cycle-5 analysis noted analogous bare-var seam idiom remains in `internal/state/daemon_lock.go`; out of scope for 9-6
