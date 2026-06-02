TASK: Emit migrateConfigFile INFO per migrated file (op=migrate, via=migrate, owning component) (portal-observability-layer-3-6)

ACCEPTANCE CRITERIA:
- Successful hooks.json migration → exactly one INFO under component hooks with op=migrate via=migrate + path; aliases→aliases; projects.json→projects.
- No-op migration (old absent, new exists, stat error branch) → nothing.
- os.Rename/os.MkdirAll failure → one WARN with error + documented error_class.
- owning component correctly threaded from filename→component mapping; no migration logs under wrong component.
- No other caller and not AtomicWrite emits a migrate breadcrumb (single sanctioned non-store emitter).
- per-entry-key [needs-info] + migrate-WARN error_class [needs-info] resolved explicitly + recorded in comments + PR description; PR-1-window-unlogged caveat noted.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). migrateConfigFile is the ONE sanctioned non-store emitter: directory-to-directory move, emits one INFO per migrated file under owning component (hooks/aliases/projects) with op=migrate via=migrate. AtomicWrite stays audit-unaware. PR-timing caveat: lands PR 2; PR-1-window migration unlogged (accepted).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/config.go:18-22 (configFileComponents closed map); :49-80 (migrateConfigFile(old, new, component): INFO :74-76, MkdirAll WARN :60-63 error_class write-failed-temp-create, Rename WARN :67-70 error_class write-failed-rename; empty-component guard wraps emissions); :105-107 (component derived from filename at single configFilePath site); log.go:141-143 (log.For dynamic).
- Notes: Both [needs-info] resolved+documented (config.go:30-44): (a) no per-entry key for whole-file move (component+path); (b) error_class reuses write-failed-* by analogy. PR-1-window caveat at :46-48. op verb both message and op attr. No-op early returns precede all emissions. MkdirAll WARN uses path=Dir(newPath), Rename/INFO use path=newPath (defensible — operand of failed op).

TESTS:
- Status: Adequate
- Location: cmd/config_migrate_logging_test.go
- Coverage: INFO hooks (asserts hook_key absent, validates decision (a)); table aliases/projects component; no-op for absent-old/occupied-new/stat-error (zero records); WARN write-failed-rename (level/msg/op/component/via/path/error_class/error + old file present); WARN write-failed-temp-create (MkdirAll); empty-component suppression (zero records, move still happens); TestConfigFilePathThreadsComponent. "not via AtomicWrite/other callers" structurally guaranteed (Grep finds only config.go; fileutil AST test forbids internal/log import).
- Notes: Behaviour-focused via logtest.Sink. config_test.go retains move-mechanics; old stderr assertion removed with pointer comment. Clean separation.

CODE QUALITY:
- Project conventions: Followed (log.For; closed value spaces; no t.Parallel; logtest.Sink).
- SOLID: Good — single static map source of truth; generic mover with threaded component (avoids per-caller duplication anti-pattern).
- Complexity: Low.
- Modern idioms: Yes (os.IsNotExist, slog attrs, direct error attr preserving *os.PathError).
- Readability: Good — doc explains design + both [needs-info] inline.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The `if component != "" { log.For(component)... }` guard is duplicated at three emission sites; a tiny local emit helper would DRY it. Low value either way (no unmapped entries today; guard is defensive-only).
- [idea] INFO/Rename-WARN use path=newPath while MkdirAll-WARN uses path=Dir(newPath); intentional/correct but a one-line comment explaining why would aid a reader greppping path= for consistency.
