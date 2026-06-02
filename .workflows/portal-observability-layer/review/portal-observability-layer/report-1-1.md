TASK: Create internal/log package skeleton with swappable-handler indirection and For() (portal-observability-layer-1-1)

ACCEPTANCE CRITERIA:
- log.For("daemon") returns a non-nil *slog.Logger when called before any Init.
- The package init runs before any consumer's For call.
- A logger obtained via For before a handler swap routes its records to the swapped-in handler after the swap.
- The pre-Init default handler writes INFO-and-above to stderr as text (DEBUG dropped at the default level).
- Concurrent For calls and a concurrent handler swap do not data-race (go test -race).

STATUS: Complete

SPEC CONTEXT:
Spec § "The internal/log package" mandates internal/log as the single owner of all logging machinery — no `*slog.Logger` constructed anywhere else. Root logger built in package init over a custom handler whose inner delegate is replaceable (atomic.Pointer or mutex guarded). Init swaps the configured handler in; For-created loggers share the indirection so loggers cached at package-init route to the configured handler once Init lands. Before Init, the indirection holds a safe default handler writing INFO-and-above as text to stderr. Cost target: one synchronized read per Handle. Baseline attrs NOT bound via root.With at construction (deferred to the configured handler per-record).

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria and spec)
- Location:
  - internal/log/log.go:22-92 — swapHandler with atomic.Pointer[slog.Handler] indirection and deferred mod-chain (handlerMod) replay
  - internal/log/log.go:94-101 — package-level `swap` + `root = slog.New(swap)`, constructed at package init
  - internal/log/log.go:107-117 — newSwapHandler / defaultHandler (`slog.NewTextHandler(os.Stderr, {Level: LevelInfo})`)
  - internal/log/log.go:119-134 — unexported setHandler / currentHandler seams
  - internal/log/log.go:136-143 — For(component) returning `root.With("component", component)`, no empty-string special-casing
  - internal/log/doc.go:1-18 — single-owner invariant package comment
- Notes: Deferred `handlerMod` chain (log.go:35-50, 87-92) correctly solves the "With eagerly calls WithAttrs at For() time" problem: WithAttrs/WithGroup record the op and re-apply against the live inner handler at Enabled/Handle time, sharing the same atomic cell. Single-read-per-call cost target met. Baseline-attr injection correctly omitted from this skeleton (deferred per spec).

TESTS:
- Status: Adequate
- Coverage: log_test.go covers all 5 ACs 1:1 (For-before-Init non-nil; init-ordering via package-init var; cached-before-swap routing via setHandler+recordingHandler; For("") valid; race test 16x100 For+Info vs 16x100 setHandler; subprocess-isolated DEBUG-drop/INFO-emit stderr test).
- Notes: Behaviour-focused, not over-tested. Subprocess isolation for the stderr test is the right call.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel(), internal layout, atomic.Pointer behind pointer field).
- SOLID principles: Good — swapHandler single responsibility; For thin factory.
- Complexity: Low.
- Modern idioms: Yes — generic atomic.Pointer[T], log/slog, range-over-int in tests.
- Readability: Good — comments explain the deferred-mod-chain rationale.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] swapHandler.applyMods re-runs the full WithAttrs/WithGroup chain on every Enabled/Handle. Negligible for the For()-only single-element path; only worth revisiting if deep WithGroup nesting lands on a hot path.
- [idea] currentHandler() reads raw inner without applying the mod chain while snapshotHandler() applies mods; equivalent for the root swap (mods==nil), intentional asymmetry for the SetTestHandler save/restore seam.
