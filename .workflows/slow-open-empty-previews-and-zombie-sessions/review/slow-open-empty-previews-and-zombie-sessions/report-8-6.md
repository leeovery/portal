TASK: 8-6 — Consolidate portal_saver.go seams into seam structs with one setter idiom

STATUS: Complete

SPEC CONTEXT: Cosmetic refactor from architecture-c2 Finding 2 — `portal_saver.go` had ~18 mutable seams across three setter idioms. c3 reopened it; final form converged on option (b): one composite `SaverSeams` with grouped sub-fields.

IMPLEMENTATION:
- Status: Implemented (option b)
- Location: `internal/tmux/portal_saver.go`
  - `SaverSeams` struct (223-231): shared `ReadPID` + `IdentifyDaemon` at top level; `Barrier`, `Readiness`, `Version`, `Ops` sub-structs
  - Sub-structs: `SaverBarrierSeams` (132-139), `SaverReadinessSeams` (152-155), `SaverVersionSeams` (182-186), `SaverOperationSeams` (200-203)
  - Single instance: `var saver = SaverSeams{...}` (238-272), with `init()` wiring at 274-285 to break self-referential cycle
  - Production setters: `SetBarrierLogger` (298-303), `SetVersionWriterLogger` (318-323) — both nil-guarded
  - All production call sites use `saver.<Cluster>.<Field>` pattern
  - Production wiring: `internal/bootstrapadapter/adapters.go:86-87`
- 5-struct intermediate from c3 collapsed; no `SaverSharedSeams` remains

TESTS:
- Status: Adequate
- `export_test.go` exposes both struct-pointer accessors (`Saver()`, `SaverBarrier()`, etc.) for atomic cluster swaps AND per-field `*Seam()` accessors for `swapSeam` helper
- `portal_saver_test.go` covers `SetBarrierLogger`/`SetVersionWriterLogger` round-trip + nil-guard
- 64 references to new accessors

CODE QUALITY:
- Project conventions: Followed; DI/seam pattern matches codebase idiom
- SOLID: Good; SRP per sub-struct; shared top-level fields correctly identify primitives consumed by ≥2 consumers
- Complexity: Low; composite-literal defaults + `init()` wiring well-explained
- Modern idioms: Idiomatic Go composite + sub-struct grouping
- Readability: Good; every struct + field has docstring scoping consumer

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `SetBarrierLogger` + `SetVersionWriterLogger` could be folded into single `tmux.WireSaverLoggers(r.Logger)` if a third sink is added later
- [idea] `BootstrapAliveCheck` (70) and `PortalSaverRetryDelay` (74) remain as bare package-level vars outside `SaverSeams`; consider absorbing or note in `SaverSeams` docstring
