TASK: Inject per-record baseline attrs and render component-prefix text in the foundation handler (portal-observability-layer-1-3)

ACCEPTANCE CRITERIA:
- A record carrying component=hydrate renders as `... INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=<pid> version=<v> process_role=<role>` with baselines last and component NOT duplicated in attrs.
- Baseline attrs appear on a record from a logger obtained from For BEFORE the handler was constructed/swapped (per-record injection).
- Multi-word string attr quoted; single-token not.
- time.Duration renders via String() (3s, 1.2s), not nanoseconds.
- slog.Group("g", slog.String("k","v")) renders as g.k=v.
- Handler level-filters: at INFO a DEBUG record is dropped.

STATUS: Complete

SPEC CONTEXT:
Spec § Subsystem prefix taxonomy (Rendering mechanism, baseline attrs, text-mode rendering rule) + § Init/For contract. Configured handler renders `<RFC3339Nano> <LEVEL> <component>: <msg> <attrs>`, component as literal prefix not in attr list, contextual attrs in record order then pid/version/process_role injected per-record (not via root.With). Multi-word quoted, Duration via String(), Group flattened to dotted keys. Lifecycle-marker bypass deferred to Phase 2.

IMPLEMENTATION:
- Status: Implemented (Phase-2 work layered on top; Task 1-3 contract intact)
- Location:
  - internal/log/handler.go:73-108 (textHandler + newTextHandler storing baselines)
  - handler.go:131-183 (Handle: time/level/component-prefix/msg, WithAttrs+record attrs with component excluded, pid/version/process_role appended last)
  - handler.go:198-219 (component resolution from record attrs then accumulated WithAttrs chain)
  - handler.go:224-255 (WithAttrs/WithGroup, dotted group prefix, clone with independent slice)
  - handler.go:257-305 (writeAttr group flattening; formatValue duration-via-String + quoting; quoteIfMultiWord)
  - handler.go:118-121 (Enabled level gate)
  - log.go:107-143 (swap indirection + For; per-record injection works for cached-before-swap loggers)
- Notes: Per-record baseline injection correct (stored on struct, written inline in Handle, never via root.With). Golden-string test asserts the full spec example byte-for-byte. Phase-2 work (lifecycle bypass, bestEffortWrite stderr fallback) layered on without regressing Task 1-3; Enabled INFO-floor change guarded by regression assertion. Single io.WriteString per Handle honours unbuffered constraint.

TESTS:
- Status: Adequate
- Location: internal/log/handler_test.go
- Coverage: component-prefix+omitted-from-attrs (positive+negative), full acceptance line golden string, cached-before-swap injection, quoting both branches, duration via String (+negative no-nanosecond), group flattening, level drop (Enabled + Handle-level). Phase-2 tests reinforce level-gate without redundancy.
- Notes: 7 AC/test items map 1:1; negative assertions ensure regression detection. Behavioural, not implementation-probing. Obeys no-t.Parallel rule; cached-logger test uses snapshotHandler + t.Cleanup.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — textHandler single rendering responsibility; helpers free functions.
- Complexity: Low; writeAttr recursion natural for flatten requirement.
- Modern idioms: Yes — strings.Builder, atomic.Pointer, slog.Value.Resolve() (handles LogValuer).
- Readability: Good — comments explain per-record-injection and Enabled-vs-Handle authority split.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] quoteIfMultiWord only quotes on whitespace; a value containing `=` or `"` is emitted unquoted/unescaped, which can break naive key=value parsing. Out of scope for Task 1-3 (spec only mandates multi-word quoting); fine for grep use, but worth a deliberate decision if the format is ever machine-parsed. (handler.go:300-305)
- [idea] version/process_role baselines routed through quoteIfMultiWord while pid is not — minor asymmetry, harmless.
