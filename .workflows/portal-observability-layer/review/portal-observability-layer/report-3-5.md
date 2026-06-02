TASK: Instrument alias.Store mutation+persist seam (alias/op/value/via, set-noop) (portal-observability-layer-3-5)

ACCEPTANCE CRITERIA:
- Setting a new alias → one INFO aliases: set alias=<name> value=<path> via=cli after successful persist.
- Existing alias to different path → op=modify with value; same path → DEBUG op=set-noop, skips persist.
- Removing existing alias → INFO aliases: rm alias=<name> via=cli no value; absent alias → nothing (CLI errors before persist).
- Persist failure → one WARN with error (wrapped) + error_class per documented phase mapping.
- via=cli from cmd/alias.go and TUI alias-editor; AliasEditor interface + mock updated.
- emission-point and error_class-phase [needs-info]s resolved explicitly + recorded in code comments + PR description.

STATUS: Issues Found (0 blocking; 1 non-blocking audit-coverage gap; all task-named ACs met)

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). aliases component; seam is store methods, NOT callers, NOT AtomicWrite ("single place per file where the breadcrumb can't be forgotten"). Required op/alias + error_class on WARN; optional value/via. set-noop → DEBUG. Verbatim privacy.

IMPLEMENTATION:
- Status: Implemented for the two named callers; one production mutation site outside them left un-instrumented (see notes).
- Location: internal/alias/store.go:28 (logger); :170-194 SetAndSave (absent→set, equal→set-noop DEBUG+skip Save, differ→modify; INFO/WARN with error+error_class); :208-222 DeleteAndSave (absent→no Save/no emit; present→Save then INFO rm/WARN); :109-127 Save (sentinel-wraps MkdirAll→ErrWriteTempCreate, WriteFile→ErrWriteWrite, reuses ClassifyWriteError). Callers cmd/alias.go:33 (rm), :81 (set) via="cli"; tui AliasEditor interface :86-89 + :1483/:1500 via="cli".
- Notes: value verbatim. Bare Set/Delete/Save emission-free. Both [needs-info] resolved+recorded: emission = option (a) combined method (154-166); error_class = manual WriteFile→write-failed-write / MkdirAll→write-failed-temp-create (91-108), AtomicWrite-migration option (b) flagged deferred.

TESTS:
- Status: Adequate
- Location: internal/alias/store_logging_test.go
- Coverage: INFO set+value+via (+persist side-effect); INFO modify; DEBUG set-noop + modtime-unchanged (proves skip-Save); INFO rm no value; absent-delete zero records + existed=false; WARN error_class=write-failed-write via 0500 read-only parent (exercises WriteFile phase). DeleteAndSave failure covered. TUI via=cli asserted model_test.go:5162.
- Notes: set-noop test effective (Sink.Enabled always true, OnlyRecord fails on zero records). Behaviour-focused. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (log.For("aliases"); op verb message+attr; reuses fileutil sentinels; no t.Parallel; logtest.Sink).
- SOLID: Good — combined-method seam single chokepoint; AliasEditor small (3 methods) DI-injected.
- Complexity: Low.
- Modern idioms: Yes (errors.Is classification, double-%w preserves *os.PathError).
- Readability: Good — [needs-info] resolutions documented inline.
- Issues: None at the instrumented-seam code level.

BLOCKING ISSUES:
- None. All six ACs for the task as scoped (two named callers) are met and tested.

NON-BLOCKING NOTES:
- [bug] internal/ui/browser.go:248-279 (handleAliasSave, the shared file-browser "save alias for highlighted dir" `a`-key flow wired into cmd/open.go) is a THIRD production alias-mutation site still using the un-audited two-step Set+Save (:272-273); its AliasSaver interface exposes only Load/Set/Save so this path emits NO breadcrumb. Out of the task's literal scope (Do names only cmd/alias.go + tui/model.go) but defeats the spec's "single place per file where the breadcrumb can't be forgotten" guarantee for file-browser-created aliases. Recommend threading this caller onto SetAndSave(name, path, "cli") in a follow-up so alias-audit coverage is complete.
- [idea] Option (b) (migrate alias.Store.Save to fileutil.AtomicWrite for atomicity + unified four-phase sentinels) flagged in-code as deferred; current in-place truncating os.WriteFile is non-atomic and could leave a partially-written aliases file on a mid-write crash.
