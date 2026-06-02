TASK: Align the project attr with its closed-vocabulary definition (name, not path) (portal-observability-layer-7-6)

ACCEPTANCE CRITERIA:
- Project-store mutation lines carry project=<project name> and path=<filesystem path>.
- Closed-vocabulary definitions for project, path, value honored.
- Documented-inversion comment removed/updated.
- go build / go test ./internal/project/... pass.

STATUS: Complete

SPEC CONTEXT:
Spec closed vocab (175-194): project = "project name from projects.json", path = "filesystem path", value = "verbatim new value for set/modify". Store-mutation breadcrumb (677-693): emit project (key) + optional value/via. Original defect: projects store inverted this (project carried PATH, value carried name).

IMPLEMENTATION:
- Status: Implemented (correctly aligned)
- Location: internal/project/store.go:143,148 (Upsert), :258,262 (Rename), :303,308 (Remove), :220 (CleanStale per-entry DEBUG); convention comment :23-26.
- Notes: All four emitters now emit "project",<name> + "path",<path>. Upsert/Rename emit "value",<name> (verbatim new value = name for this store, so project & value coincide — expected, not residual inversion); Remove/CleanStale omit value per spec. Remove resolves removed entry's name before deletion (:288-296); absent path → name empty, path carries lookup path. Inversion comment gone (grep invert/inversion clean). storelog.EmitCleanStaleSummary carries only op/entries/via/took/error/error_class — never had the inversion.

TESTS:
- Status: Adequate
- Location: internal/project/store_logging_test.go
- Coverage: Upsert set (project=portal, path=/code/portal, value=portal, via=internal); Upsert modify; Upsert WARN; Rename found + WARN; Remove (project=name, path, value absent); Remove absent path (path=/code/absent, project=""); Remove WARN; CleanStale per-entry DEBUG (project names + path values) + summary/zero/WARN.
- Notes: Split project-vs-path assertions would fail under the old inversion. Behaviour-focused via logtest.Sink. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (log.For("projects"); closed vocab; storelog/logtest seams; no t.Parallel).
- SOLID: Good — store single mutation chokepoint; batch-summary delegated to storelog.
- Complexity: Low (attr-argument realignment).
- Modern idioms: Yes (slog variadics, slices.DeleteFunc, errors.Is).
- Readability: Good — updated convention comment makes the now-correct mapping explicit.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Convention comment (:23-26) references the match key under path ("which is also the addressable match key") — accurate/useful, but a one-line note that the prior path-under-project inversion was removed could aid future archaeology. Cosmetic; correct as-is.
