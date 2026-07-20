TASK: cli-verb-surface-redesign-1-1 — Exact session-name match → attach outcome

ACCEPTANCE CRITERIA (task + phase-level bullets it satisfies):
- Task edge cases: internal `_`-prefixed sessions never matchable (filtered ListSessions view, NOT tmux HasSession); empty session set; inside-tmux attach via switch-client vs outside via exec attach; a name matching no session falls through to the directory chain.
- Phase-1 AC #2 (session-name hit attaches the existing session, never mints); AC #3 (all session-domain matching matches only the leading-underscore-filtered ListSessions view; internal sessions fall through/hard-fail as if absent).

STATUS: Complete

SPEC CONTEXT:
Spec § Grammar & Target Resolution: precedence chain is exact session name → path → alias → zoxide, first-match-wins; a session-name hit attaches (Axiom 2). Spec § "Session set — user-visible only" mandates all session-domain resolution match only the leading-underscore-filtered ListSessions view (same view as picker/completion); `_portal-saver`/`_portal-bootstrap` are never matchable and are treated as a miss. Connection is two-mode (spec/CLAUDE.md): outside tmux → exec `tmux attach-session`, inside tmux → `switch-client`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resolver/query.go:132-140 — Resolve() checks the session domain first via qr.sessions.ListSessionNames() + slices.Contains(names, query); exact hit → &SessionResult{Name, Domain:"session"}. Falls through to path/alias/zoxide/miss otherwise.
  - internal/resolver/query.go:97-112, 13-15 — QueryResolver + SessionLister interface (ListSessionNames), the user-visible-filtered seam.
  - cmd/open.go:345-363 — openResolved() dispatch: *SessionResult arm rejects a command (mint-scoped guard), writes the --ack marker, then attaches via openSessionFunc; *PathResult mints. Unknown result type is a defensive error.
  - cmd/open.go:143-149 — openSession() → buildSessionConnector(tmuxClient(cmd)).Connect(name).
  - cmd/open.go:134-141 — buildSessionConnector(): InsideTmux() → *SwitchConnector (switch-client); else *AttachConnector (exec attach). Correctly selects connector per inside/outside tmux.
  - cmd/open.go:84-86 / 100-132 — SwitchConnector.Connect (switch-client) and AttachConnector.Connect (syscall.Exec of `tmux attach-session -t =<name>`, exact-match `=` prefix).
  - internal/tmux/tmux.go:193-275 — ListSessions() applies the leading-`_` filter as its final step; ListSessionNames() delegates to it, so the production SessionLister is inherently the user-visible view. Resolution never calls HasSession (which would see internal sessions) — the task's "not tmux HasSession" requirement is satisfied.
- Notes: A lister error during bare resolution is deliberately collapsed to "no sessions" (query.go:138 `err == nil && …`), matching the spec's "a nil/empty slice or an error is treated by the resolver as 'no sessions'". Documented in-source and tested (query_test.go:503). No drift.

TESTS:
- Status: Adequate
- Coverage:
  - Resolver level (internal/resolver/query_test.go TestQueryResolver_Resolve_SessionDomain, 390-522): exact user-visible hit → SessionResult{Domain:"session"}; session-domain wins over alias/zoxide precedence; `_portal-saver` filtered → MissResult (never matchable); no-session-match falls through to the directory chain → PathResult; empty session set → no SessionResult; lister error → MissResult (no resolve error). Every task edge case is covered here.
  - cmd level (cmd/open_test.go): TestOpenCommand_SessionNameHit_RoutesToSessionConnector (214) — bare exact hit routes to openSessionFunc, not mint/picker; TestOpenCommand_BareProjectName_MintsNeverAttaches (2301) — name matching no session falls through and mints (never attaches); TestOpenSession_DelegatesToBuildSessionConnector (1143) — inside tmux issues switch-client -t =name; TestBuildSessionConnector (2884) — inside → *SwitchConnector, outside → *AttachConnector; TestAttachConnectorConnectArgv (2929) — outside exec argv `tmux attach-session -t =foo` with exact-match prefix; TestSwitchConnector (1181) — Connect success + error propagation.
  - The inside/outside connector split (task edge case) is proven end-to-end across TestBuildSessionConnector + TestOpenSession_DelegatesToBuildSessionConnector + TestAttachConnectorConnectArgv.
- Notes: Balanced — each subtest asserts a distinct behaviour (exact hit, precedence, underscore-filter, fall-through, empty set, lister error, connector selection, argv shape). No redundant/over-mocked cases. Tests would fail if the feature broke (e.g. flipping to HasSession would let `_portal-saver` match and break the underscore-filter subtest; wrong connector selection would break TestBuildSessionConnector).

CODE QUALITY:
- Project conventions: Followed. Small DI interfaces (SessionLister, SwitchClienter, execer), package-level *Func override seams restored via t.Cleanup, method-value pin dispatch table, user-visible session set threaded through the SessionLister seam per the tmux-boundary isolation rules. Exact-match `=` target prefix uniform with the rest of the codebase.
- SOLID principles: Good. openResolved is the single shared outcome switch for bare + all pins; connector selection is isolated in buildSessionConnector; resolver stays a pure, log-free library (matching side kept out of cmd).
- Complexity: Low. Resolve() is a linear precedence chain; dispatch is a type switch.
- Modern idioms: Yes (slices.Contains, strings.Cut elsewhere, errors.Is in sibling pins).
- Readability: Good. Intent-heavy comments cite the governing spec sections; the mint-scoped command guard placement (before the --ack write) is explained.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The lister-error → "no sessions" fall-through was considered; it is spec-governed and explicitly tested, so it proposes no concrete change.)
