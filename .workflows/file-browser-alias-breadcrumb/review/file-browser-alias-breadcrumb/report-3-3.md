TASK: 3-3 — Run the final acceptance gate (build/test + zero references + two blocking manual checks)

ACCEPTANCE CRITERIA:
- go build ./... green; go test ./... green (or green-modulo known internal/tmux kill-barrier flake).
- Zero-references spot-check grep returns no compiled-code/test hit for removed symbols.
- internal/ui + internal/browser directories absent on disk.
- BLOCKING MANUAL CHECK 1: Projects-page b is a visible no-op (falls through to projectList.Update, opens no view).
- BLOCKING MANUAL CHECK 2: Sessions/Projects/Preview pages, alias CLI, projects-modal alias editor unchanged/functional.
- No new test added.

STATUS: Complete

SPEC CONTEXT: The two manual checks are the fix's only behavioural verification (no new tests added). Verifier has no shell — verified by grep/glob/source inspection + coherence with committed gate-green record (commit 84b2b82e).

IMPLEMENTATION:
- Status: Implemented (verified by grep/glob/source; live build/test + interactive checks not runnable in verifier context)
- Directory-absence: Glob internal/ui/** and internal/browser/** → no files.
- Zero-reference grep: full symbol list across `*.go` → zero hits; bare-token "ui"/"browser"/"browse" in `*.go` → zero hits; README/CLAUDE prose → zero hits; all 90 markdown token hits inside `.workflows/` (expected, out of scope).
- MANUAL CHECK 1 preconditions confirmed: projectHelpKeys (model.go:698-707) has no b binding; updateProjectsPage (L1567-1613) handles only q/s/x/n/d/e/Esc/Enter; grep for isRuneKey(msg,"b") zero hits; b falls through to projectList.Update (L1617) — opens no view. commandPendingHelpKeys doc comment dropped stale b.
- MANUAL CHECK 2 confirmed in source: PageSessions/PageProjects/pagePreview present; preview update arm (L1520), view arm (L2224), Space→pagePreview (L1994) intact; cmd/alias.go aliasRmCmd (audited DeleteAndSave)/aliasListCmd intact; aliasEditor (model.go:181), AliasEditor.SetAndSave (L103), persist via DeleteAndSave (L1897)/SetAndSave (L1914); createSession (L1529) survives; cmd/open.go WithCWD(cfg.cwd) (L360) + cwd: cwd (L504) survive.
- No new test added (removal only).

BUILD/TEST CAVEAT: verifier cannot run go build/go test (no shell). Source coherence assessed: no dangling reference would red the build; both deleted packages have zero importers; consistent with the recorded "final acceptance gate green" commit 84b2b82e. The two manual checks require interactive TUI launch + CLI round-trip not performable in verifier context, but all source-level preconditions hold.

TESTS:
- Status: Adequate (removal task; gate is existing green build/test + two manual checks).

CODE QUALITY:
- N/A (verification task).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
