TASK: Instrument project.Store Upsert/Rename/Remove/CleanStale (op vocabulary, project/value/via/error_class) (portal-observability-layer-3-4)

ACCEPTANCE CRITERIA:
- Upsert not-yet-remembered → INFO projects: set value=<name> via=<via>; existing → op=modify.
- Rename found → INFO projects: modify value=<newName> via=cli; absent → nothing, no Save.
- Remove → INFO projects: rm via=cli no value; absent still INFO.
- CleanStale N>0 → N DEBUG + one INFO projects: clean-stale entries=N via=internal took=<d>; zero → nothing, no Save.
- WARN paths carry error_class from AtomicWrite phase space.
- via=internal for PrepareSession Upsert, via=cli for TUI rename/delete; interfaces + mocks updated.
- project attr value choice documented; single-batched-Save + LastUsed-bump-vs-set-noop ambiguities flagged in comments.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). projects.json component projects; store-method seam. One INFO success / WARN failure. Closed op space + error_class on WARN. Batch ops per-entry DEBUG + INFO summary. Closed vocab defines project="project name" and path="filesystem path" as two distinct allowed keys.

IMPLEMENTATION:
- Status: Implemented (deliberate documented divergence from task literal AC text — see Notes)
- Location: internal/project/store.go:33 (logger), 109-150 (Upsert), 190-236 (CleanStale via storelog.EmitCleanStaleSummary), 248-269 (Rename), 282-310 (Remove). Callers session/prepare.go:51 (via=internal), tui/model.go:965 Remove + :1465 Rename (via=cli). Interfaces session/create.go:38, tui/model.go:54-58,76-78. Mocks updated.
- Notes: Task "Do"/AC said project=PATH, name in value; impl resolved the ambiguity to project=NAME, path=filesystem path (separate attr), value=name — faithful to the spec's closed-vocabulary definitions (both keys allowed), documented store.go:23-26. Better choice, NOT scope creep, but the literal AC string project=<path> isn't satisfied (AC itself was flagged-loose). All behavioural requirements met. op from pre-write Load found flag. Rename absent = true no-op. Remove always Saves/emits. CleanStale zero-removal skips. No Sprintf; Save/AtomicWrite audit-unaware. set-noop unreachable (LastUsed always bumps) + single-batched-Save [needs-info] flagged in comments.

TESTS:
- Status: Adequate
- Location: internal/project/store_logging_test.go
- Coverage: set/modify Upsert; Rename found + absent (zero records + mtime); Remove no value + absent-still-emits; CleanStale N (2 DEBUG + summary entries/via/took, entries_failed absent) + zero (zero records + mtime); WARN+error_class=write-failed-temp-create all four methods; TestSaveDoesNotLog. Caller via-threading asserted in session/create_test.go + tui/model_test.go.
- Notes: Behaviour-focused. mtime assertions verify no-Save. Tests assert per the impl's path/project convention.

CODE QUALITY:
- Project conventions: Followed (log.For("projects"); storelog shared; no t.Parallel).
- SOLID: Good — store-method seam; small interfaces; via threaded.
- Complexity: Low.
- Modern idioms: Yes (slices.DeleteFunc, error value not .Error()).
- Readability: Good — every non-obvious decision documented.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Deliberate divergence from task literal (project=<path>, value=<name>) → spec-faithful (project=name, path=path, value=name), well-documented store.go:23-26. Sound/arguably superior resolution of the ambiguity the task flagged, but contradicts the AC string → warrants human ack that the divergence is accepted and consistent across hooks/aliases/projects key-vs-value conventions.
- [idea] Upsert carries value=name on the WARN path too; confirm intended failure-line attr set matches hooks/aliases for cross-component consistency.
- [quickfix] store.go:25-26 doc "verbatim new value for set/modify" — Rename (modify) + Upsert both emit it; consistent, only flag if a reader expects value exclusively on set.
