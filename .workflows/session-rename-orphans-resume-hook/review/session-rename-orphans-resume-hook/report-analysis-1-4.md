TASK: analysis-1-4 (tick-8cde60) — Add a fast static byte-identity guard for the three @portal-id literals

ACCEPTANCE CRITERIA:
- A no-tmux unit test asserts HookKeyFormat contains the exact @portal-id literal.
- A no-tmux unit test asserts captureFormat contains the exact @portal-id literal.
- At least one assertion pins the canonical literal value (session.PortalIDOption == "@portal-id" if reachable without a cycle, otherwise a local literal-equality assertion).
- The guard runs and passes under plain `go test ./...` with no tmux present (does NOT depend on SkipIfNoTmux).
- No import cycle introduced; `go build ./...` passes.

STATUS: Complete

SPEC CONTEXT:
The change's correctness rests on three independent embeddings of the literal "@portal-id" staying byte-identical: session.PortalIDOption (internal/session/create.go:29), HookKeyFormat's conditional (internal/tmux/tmux.go:849), and captureFormat's trailing column (internal/state/capture.go:42). The two raw tmux format strings cannot share a const, and end-to-end consistency was previously exercised ONLY by real-tmux tests gated by SkipIfNoTmux(t) — so in a tmux-less environment they SKIP silently, letting a one-character typo (e.g. @portal_id) slip through uncaught. This task adds a fast static tripwire that runs unconditionally.

IMPLEMENTATION:
- Status: Implemented (co-located two-guard approach, exactly as the task's preferred option prescribes)
- Location:
  - internal/tmux/hookkey_test.go:29 — TestHookKeyFormatContainsPortalIDLiteral (package tmux_test, external; imports internal/tmux and references exported tmux.HookKeyFormat)
  - internal/state/portal_id_literal_guard_test.go:26 — TestCaptureFormatContainsPortalIDLiteral (package state, white-box; references the unexported captureFormat)
- Verified source strings genuinely contain the literal:
  - internal/tmux/tmux.go:849 HookKeyFormat = "#{?@portal-id,#{@portal-id},#{session_name}}:..." (contains @portal-id)
  - internal/state/capture.go:42 captureFormat = "...|||#{@portal-id}" (contains @portal-id)
  - internal/session/create.go:29 PortalIDOption = "@portal-id"
- Both guards spell out portalIDLiteral = "@portal-id" locally (const) and assert it equals "@portal-id" before the Contains check — satisfies the "pin the canonical literal" criterion via the local literal-equality path (session.PortalIDOption is unreachable from either package without a cycle).
- Import-cycle rationale confirmed correct: `go list -deps internal/session` shows session transitively depends on internal/state, so state importing session WOULD cycle; and internal/session imports internal/tmux (dirresolve.go:10), so tmux importing session WOULD cycle. The co-located white-box (state) + external-test (tmux_test) split is the correct, cycle-free placement. tmux_test importing internal/tmux is legal (external test package). `git log` confirms both files landed under commit b10df513 (T-analysis-1-4).
- Notes: In internal/tmux the const portalIDLiteral is declared once (hookkey_test.go:17) and reused by five sibling *realtmux*_test.go files in the same tmux_test package — clean single declaration, no duplication, no compile conflict.

TESTS:
- Status: Adequate
- Coverage: Both format strings covered; canonical literal pinned in both guards. The guards are the deliverable (this task IS the test). No SkipIfNoTmux gate present in either file (the only SkipIfNoTmux occurrences are inside explanatory comments) — so both run under plain `go test`.
- Failure-injection reasoning (not executed, per review rules): mutating captureFormat's trailing column to #{@portal_id} makes strings.Contains(captureFormat, "@portal-id") false → t.Errorf fires. Same holds for HookKeyFormat. The tripwire behaves as specified.
- Not over-tested: two focused guards, minimal assertions (one literal-equality + one Contains each), zero mocking/setup. The pre-existing TestHookKey / TestHookKey_DistinctSuffixesUnderOneID in the same file are prior coverage, out of this task's scope, and not redundant with the new guard.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (correct per CLAUDE.md package-wide prohibition). Fast, tmux-less, no build tag (correct — these are unit-level invariants, not integration tests). White-box placement for the unexported captureFormat is idiomatic.
- SOLID principles: N/A (static invariant guards)
- Complexity: Low — trivial straight-line assertions.
- Modern idioms: Yes — strings.Contains, standard testing, t.Fatalf/t.Errorf used appropriately (Fatalf on the canonical-literal precondition, Errorf on the substring check).
- Readability: Good. Both guards carry substantial doc comments explaining WHY the literal is repeated (import-cycle avoidance) and that it must stay byte-identical to session.PortalIDOption, mirroring the existing real-tmux constant comments — exactly as the task's step 5 requested.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/state/portal_id_literal_guard_test.go:11 — The comment says "internal/session imports internal/state, so internal/state cannot import internal/session." The dependency is transitive, not direct (session imports tmux/project/resolver; it reaches state only through the graph). The conclusion (state->session would cycle) is correct — `go list -deps internal/session` confirms state is in session's dependency set. Reword to "internal/session transitively depends on internal/state" for precision.
