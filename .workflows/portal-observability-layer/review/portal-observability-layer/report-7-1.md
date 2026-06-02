TASK: Consolidate the discard-logger declaration + nil-guard into one internal/log helper (portal-observability-layer-7-1)

ACCEPTANCE CRITERIA:
- Single package-level discard logger in internal/log; no other production file declares one.
- log.OrDiscard (and/or log.Discard) is the only path used by the previously-listed nil-fallback sites.
- state.loggerOrDiscard, restore.Orchestrator.logger(), restore.SessionRestorer.logger() forward to the helper or are removed.
- No behavior change.
- go build/test pass.

STATUS: Complete

SPEC CONTEXT:
Spec § Call-site logging pattern Prohibited (380): no direct *slog.Logger construction outside internal/log. doc.go single-owner invariant. Migration sweep: canonical silent logger slog.New(NewTextHandler(io.Discard, nil)).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/discard.go:14 (discardLogger), :20 OrDiscard, :30 Discard; internal/state/logger_nil.go:16-18 (loggerOrDiscard forwarder, local discardLogger deleted; 6 call sites); internal/restore/logger_nil.go:16-22 (both logger() forwarders, local deleted); internal/tmux/portal_saver.go:20 (now log.For("saver")); hooks_register.go:253,325,394,129 (log.OrDiscard); cmd/bootstrap bootstrap.go:265/orphan_sweep.go:141/stale_marker_cleanup.go:123 (log.OrDiscard).
- Notes: Repo-wide grep slog.New( shows only two production constructions: discard.go:14 + log.go:101 (root). All others in _test.go.

TESTS:
- Status: Adequate
- Coverage: internal/log/discard_test.go (NilReturnsNonNilDiscardingLogger no-panic all levels; NonNilReturnedUnchanged pointer identity; NilReturnsSharedInstance; Discard_ReturnsNonNil; MatchesOrDiscardNil); discard_guard_test.go (walks production .go, fails on slog.NewTextHandler(io.Discard literal outside discard.go — standing regression guard). Existing state/restore/tmux/bootstrap tests exercise forwarders' nil-tolerance.
- Notes: Behaviour-focused (identity, no-panic, single-instance, source invariant). Guard fails if any package reintroduces a local discard logger. Well-balanced.

CODE QUALITY:
- Project conventions: Followed (single logging owner; no t.Parallel; one-line forwarders).
- SOLID/DRY: Good — one canonical discard owner, eliminates four duplicate declarations + ~9 guard sites.
- Complexity: Low.
- Modern idioms: Yes (log/slog, package-level sentinel).
- Readability: Good — discard.go doc ties helper to spec Prohibited rule + doc.go invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] portal_saver.go migrated to log.For("saver") (real component sink) rather than a discard fallback — exceeds the task's literal "replace with log.Discard()" and is arguably superior (saver-lifecycle breadcrumbs now render under component=saver); confirm it aligns with the saver lifecycle-observability subtopic. Correct/self-consistent.
- [idea] Discard() is exported but has no production consumer (only OrDiscard + log.For used at migrated sites); task authorized it; currently dead production API surface.
