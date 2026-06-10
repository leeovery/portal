TASK: 1-2 — Reconcile each sweep hit against the Removal Manifest and record dispositions

ACCEPTANCE CRITERIA:
- Every hit has exactly one disposition.
- Every in-manifest hit names its manifest bullet.
- Any hit not in manifest explicitly reconciled (new-reconciled or justified false-positive).
- All named scope-boundary survivors flagged scope-boundary-keep / do-not-touch.
- Doc/comment hits in non-compiled contexts dispositioned.

STATUS: Complete

SPEC CONTEXT: Reconciliation task producing a per-hit disposition record consumed by 1-3. Verified via END STATE the reconciliation drove (Phases 2-3 ran on top of it).

IMPLEMENTATION:
- Status: Implemented (verified by end state)
- Scope-boundary survivors ALL INTACT (none over-deleted):
  - cmd/alias.go exists; aliasSetCmd/aliasRmCmd/aliasListCmd + AddCommand wirings intact (portal alias set/rm/list survives).
  - internal/tui/model.go: aliasEditor field, AliasEditor.SetAndSave, SetAndSave/DeleteAndSave/Load call sites (projects-modal alias editor survives).
  - createSession (model.go) with project-enter + createSessionInCWD callers.
  - cmd/open.go: cwd field, tui.WithCWD(cfg.cwd) (L360), cwd: cwd (L504) — line drift from manifest L371/L505 expected (adjacent WithDirLister/dirLister removed above); 1-3's responsibility, not a 1-2 finding.
- Removal completeness: both dirs gone; zero remaining `.go` references to any removed symbol; "browse" string and isRuneKey(msg,"b") dispatch gone; projectHelpKeys/commandPendingHelpKeys carry no b binding; doc comment updated.
- Gate-blind items caught: surface-audit "browser"/"ui" keys removed; README/CLAUDE prose clean; pagepreview comments clean; rename landed.

TESTS:
- Status: N/A — analysis task; per spec net test delta is removal.

CODE QUALITY: N/A (no code).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
