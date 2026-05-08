TASK: session-scrollback-preview-2-7 — Production adapters wired at TUI construction

ACCEPTANCE CRITERIA:
- scrollbackReaderAdapter exists in internal/tui and satisfies ScrollbackReader.
- scrollbackReaderAdapter.Tail resolves via state.ScrollbackFile + state.TailScrollback(path, 1000).
- *tmux.Client satisfies TmuxEnumerator (compile-time assertion present).
- stateDir captured exactly once at TUI construction; no per-open re-resolution.
- previewTailLines == 1000 per spec.
- Pane key derivation uses state.SanitizePaneKey(session, window_index, pane_index) matching daemon writer.
- internal/tui/pagepreview_test.go does not import tmuxtest.
- pagepreview_test.go uses t.Parallel() safely (no package-level mutable state to clean up).

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-cutting Seams > State Package API Reuse > stateDir resolution mandates "stateDir is captured once and stable for the Portal process lifetime; not re-resolved per preview-open." Spec § Architecture Summary > Test seams mandates production-code adapters wire *tmux.Client and the tail-N helper to the seams at TUI construction.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - internal/tui/preview_adapter.go — scrollbackReaderAdapter, NewProductionScrollbackReader, compile-time assertions for both seams.
  - internal/tui/preview_adapter.go:39-42 — Tail resolves via state.ScrollbackFile + state.TailScrollback.
  - internal/tui/preview_adapter.go:48-49 — compile-time assertions for *tmux.Client and scrollbackReaderAdapter.
  - cmd/open.go:376-380, 392 — stateDir resolved exactly once via state.Dir() in openTUI.
  - internal/tui/pagepreview.go:130 — state.SanitizePaneKey(m.session, rawWindow, rawPane).
  - internal/state/capture.go:121 — daemon writer SanitizePaneKey(ps.Name, pw.Index, pp.Index) — verbatim match.
  - internal/tui/pagepreview_test.go imports only internal/state and internal/tmux — no tmuxtest.

TESTS:
- Status: Adequate
- Location: internal/tui/preview_adapter_test.go
- Coverage:
  - Three Tail shapes: bytes/nil, nil/nil (missing file), nil/err (permission-denied with errors.Is(err, fs.ErrPermission)).
  - previewTailLines == 1000 regression test.
  - Duplicated compile-time assertions.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Clean — single responsibility, segregated interface, constructor-injected.
- Complexity: Trivially low (3-line behaviour body).
- Modern idioms: errors.Is + fs.ErrPermission, runtime.GOOS guard, t.TempDir, t.Cleanup, root-bypass skip.
- Readability: Doc comments anchor each declaration to spec sections.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Compile-time assertions duplicated between preview_adapter.go (production) and preview_adapter_test.go (test). The production-file assertion alone suffices; comment justifies the duplication; defensible but worth a follow-up.
- [idea] preview_adapter_test.go does not declare t.Parallel(). Spec acceptance reads "uses t.Parallel() safely". Adapter tests have no package-level mutable seams, so they could parallelise. Not violated since adding t.Parallel() is optional.
- [quickfix] Doc comment on scrollbackReaderAdapter claims n is "supplied at construction so the helper has no implicit dependency on the package-level previewTailLines constant" — but the only production constructor hardcodes previewTailLines. Flexibility exists at struct-literal level (used in tests) but the prose overstates independence for the production path.
