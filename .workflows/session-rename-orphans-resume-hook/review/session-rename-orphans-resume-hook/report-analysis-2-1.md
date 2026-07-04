TASK: analysis-2-1 (tick-722bfe) — Add a fast tmux-less guard binding session.PortalIDOption to the hook-key format string (cmd/portal_id_binding_guard_test.go)

ACCEPTANCE CRITERIA:
- A new tmux-less guard test exists in the cmd package asserting both session.PortalIDOption == "@portal-id" and strings.Contains(tmux.HookKeyFormat, session.PortalIDOption).
- The test is NOT //go:build integration-tagged and does NOT call SkipIfNoTmux; it runs and passes under `go test ./cmd` with no tmux server present.
- Mutating session.PortalIDOption to any other value causes the new test to fail fast without tmux.
- `go test ./...` remains green; no production code, existing test, or the two cycle-1 guards are altered.

STATUS: Complete

SPEC CONTEXT:
The fix's central invariant requires three independent embeddings of the literal "@portal-id" to stay byte-identical: the source-of-truth constant session.PortalIDOption (internal/session/create.go:29), the literal inside tmux.HookKeyFormat (internal/tmux/tmux.go:849), and the literal inside state.captureFormat (internal/state/capture.go). internal/tmux and internal/state cannot import internal/session (import cycle), so the literals are duplicated. Two cycle-1 static guards (internal/tmux/hookkey_test.go, internal/state/portal_id_literal_guard_test.go) each pin only their own format string to a LOCAL copy const portalIDLiteral = "@portal-id"; neither compares against session.PortalIDOption. The residual gap: nothing pins the source-of-truth constant to that literal in a fast tmux-less path, so a change to the constant (e.g. to @portal-uid) would pass both cycle-1 guards yet silently orphan every stamped session's resume hook — the spec's named "Missed key-producing site" drift class. Only the //go:build integration + SkipIfNoTmux end-to-end tests catch a constant change today, and those are skipped under the default tmux-less `go test ./...`.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/portal_id_binding_guard_test.go (new file, added by commit 2134c99e; test TestPortalIDOptionBindsHookKeyFormat at lines 47-54)
- Notes:
  - package cmd_test (line 1) — the preferred external-facing form per the task; the file's first line is the package clause with no build-tag line above it.
  - Imports exactly strings, testing, internal/session, internal/tmux (lines 3-9) — matches the prescribed import set.
  - Assertion 1 (lines 48-50): t.Fatalf if session.PortalIDOption != "@portal-id" — catches a change to the source-of-truth constant, correctly using Fatalf (a wrong constant makes the second check meaningless).
  - Assertion 2 (lines 51-53): t.Errorf if !strings.Contains(tmux.HookKeyFormat, session.PortalIDOption) — ties the constant to the tmux format-string embedding.
  - Verified cycle-free: internal/session/dirresolve.go:10 imports internal/tmux, so a cmd_test importing both session and tmux introduces no import cycle.
  - No production code touched: git show --stat of commit 2134c99e shows only the new test file plus tick metadata (.tick/tasks.jsonl, manifest.json). git diff of internal/tmux/hookkey_test.go, internal/state/portal_id_literal_guard_test.go, internal/session/create.go, internal/tmux/tmux.go, internal/state/capture.go across the commit is empty — the two cycle-1 guards, the constant, and both format strings are untouched.
  - The two "//go:build integration"/"SkipIfNoTmux" occurrences in the file are inside the doc-comment (lines 26-28) describing the sibling end-to-end guards, NOT active directives.

TESTS:
- Status: Adequate
- Coverage: Both acceptance-criteria assertions are present and each is meaningful. Assertion 1 fails fast if the constant drifts (satisfies the "mutating PortalIDOption fails without tmux" criterion by reasoning: the equality check reads only the in-process constant, no tmux). Assertion 2 fails if HookKeyFormat stops embedding the constant's value. Combined with the two cycle-1 guards (each pinning its own format string to "@portal-id"), the transitive chain constant == "@portal-id" == HookKeyFormat literal == captureFormat literal is closed without tmux, as the task intends.
- Notes:
  - Not over-tested: two focused assertions, no redundant cases, no mocking, no setup. A single non-table test is the right shape for a two-assertion invariant tripwire (a table would be ceremony here).
  - Not under-tested: captureFormat is unexported and correctly NOT reached from cmd; its cycle-1 sibling guard already pins it to the shared literal, so the transitive chain holds — the task explicitly directs NOT to reach captureFormat from cmd, and the implementation obeys.
  - No t.Parallel() — correct: CLAUDE.md prohibits t.Parallel() in the cmd package (package-level mutable mock state). This test uses no such state, but omitting t.Parallel() keeps it conformant with the package rule.

CODE QUALITY:
- Project conventions: Followed. package cmd_test for an external binding assertion (as recommended), no t.Parallel() (cmd-package rule), no build tag / no SkipIfNoTmux so it runs under plain `go test ./cmd`.
- SOLID principles: N/A (a static assertion test).
- Complexity: Low — two guarded conditionals, no branching beyond the assertions.
- Modern idioms: Yes — idiomatic t.Fatalf/t.Errorf with %q-quoted actual/expected in messages; Fatalf-then-Errorf ordering is deliberate and correct.
- Readability: Excellent. The doc comment (lines 11-46) thoroughly explains WHY the guard lives in cmd (only cycle-free importer of both packages), the exact drift class caught (constant change silently orphaning stamped sessions' hooks), the deliberate tmux-less design, and names both cycle-1 sibling guards with their test names — giving a future reader the full three-way picture the task asked for.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
