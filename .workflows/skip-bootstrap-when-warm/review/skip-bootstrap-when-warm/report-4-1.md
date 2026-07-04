TASK: skip-bootstrap-when-warm-4-1 — Consolidate duplicated scaffolding in the abridged bootstrap tests (Phase 4, Analysis Cycle 1; chore/refactor)

ACCEPTANCE CRITERIA:
- (a) The three latch-verdict tests are one table-driven test with >=3 cases covering latch-absent, version-mismatch, latch-read-error, each asserting `runner.calls == 1`.
- (b) The saver-absent-revive-fails commander exists as one shared fixture in the cmd test package; no inline copy of that RunFunc remains in `abridged_route_test.go` or `abridged_saver_test.go`.
- No change to what any test asserts; `go test ./cmd` passes.

STATUS: Complete

SPEC CONTEXT: The skip-bootstrap-when-warm feature adds an abridged bootstrap path taken when the `@portal-bootstrapped` latch reads SATISFIED (running version stored), diverting warm commands to a liveness-only saver check instead of the full 11-step orchestrator; every not-satisfied verdict (absent / version-mismatch / read-error / nil client) folds into the full-bootstrap route. This task is pure test hygiene over that feature's new test suite — no production behaviour is in scope. It reached the rule-of-three duplication threshold on two axes: (a) three ~25-line latch-verdict tests, (b) a "saver absent, revive fails" recordingCommander RunFunc inlined three times across two files.

IMPLEMENTATION:
- Status: Implemented (commit 862ec3e0, "T4-1 — consolidate abridged bootstrap test scaffolding")
- Location:
  - cmd/abridged_route_test.go:131-192 — the collapsed table-driven `TestPersistentPreRunE_FullBootstrap_WhenNotSatisfied` (3 named cases: "latch absent" / "version mismatch" / "latch read error"), one shared setup->Execute->assert body, each asserting `runner.calls == 1` (line 187).
  - cmd/abridged_route_test.go:55-69 — shared `saverAbsentReviveFailsCommander()` fixture (list-panes->noSuchSessionErr, has-session->"can't find session", new-session->"create denied"), mirroring the sibling `satisfiedLatchAliveSaverCommander()`.
  - cmd/abridged_route_test.go:75-85 — `satisfiedLatchSaverAbsentCommander()` thin wrapper layering the satisfied-latch `show-option->version` arm over the base fixture.
  - cmd/abridged_route_test.go:203,238 — both route tests now consume the wrapper.
  - cmd/abridged_saver_test.go:185,213 — `TestEnsureSaverLiveness_FunnelsSaverDownWarningWhenReviveFails` and `...LogsWarnWithUnderlyingErrorWhenReviveFails` consume the base fixture directly.
- Notes: The shared fixture backs 4 call sites (2 route tests via wrapper + 2 saver tests direct), a stronger dedupe than the 3 inline copies the task named. Verified no residual copies: `grep "create denied"` returns only the fixture (route_test.go:64); the three old test-function names (`_WhenLatchAbsent`, `_OnVersionMismatch`, `_OnLatchReadError`) are fully gone. The two other saver tests that inline has-session/new-session arms (RevivesViaBootstrapPortalSaverWhenAbsent, TreatsProbeTransientErrorAsAbsentAndRevives) drive the DISTINCT revive-SUCCEEDS scenario (new-session->nil, respawn-pane->fail) and are correctly left untouched — not copies of the consolidated RunFunc.

TESTS:
- Status: Adequate (this task's deliverable IS the tests; correctness = behaviour preservation)
- Coverage: Byte-for-byte behaviour preservation verified against the pre-consolidation source in the diff:
  - version-mismatch case retains version="v2.0.0" override + show-option="v1.0.0" (route_test.go:146-149,161-165).
  - All three assertion `reason` strings match the originals verbatim ("full bootstrap" / "full bootstrap re-stamps" / "folds into full bootstrap").
  - The `satisfiedLatchSaverAbsentCommander` show-option->version + list-panes/has-session/new-session arms reproduce the two route tests' former inline commanders exactly.
  - Table uses named subtests via `t.Run(tc.name, ...)` (route_test.go:158), satisfying the golang-testing skill's table-driven rule; per-subtest version cleanup is order-safe because the file forbids t.Parallel (package-level mutable mocks).
- Notes: One necessary strictness reduction — the original inline commander in `TestEnsureSaverLiveness_FunnelsSaverDownWarningWhenReviveFails` had a `t.Fatalf("unexpected tmux call")` default; the shared fixture defaults to `("", nil)` instead (it must, because the route path issues additional ops the saver test does not). This drops a defensive unexpected-call guard from the two saver-test consumers but changes no asserted criterion. Non-blocking.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel; mocks the Commander interface not a concrete type; docstrings explain each fixture's scenario and the wrap rationale in the codebase's established prose style.
- SOLID principles: Good. `satisfiedLatchSaverAbsentCommander` composing the base via delegation (route_test.go:76,82) is clean open/closed reuse rather than a second copy.
- Complexity: Low. Table + shared body removes ~75 duplicated lines (net -19 across the two files); each fixture is a single switch.
- Modern idioms: Yes. Idiomatic Go table-driven test with a typed case struct and named subtests.
- Readability: Good. Case fields (showOptValue/showOptErr/versionOverride/reason) are self-documenting with inline comments; `versionOverride == ""` sentinel for "leave version untouched" is clear.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/abridged_saver_test.go:185,213 — The two saver-test consumers of the shared `saverAbsentReviveFailsCommander()` lost the original `t.Fatalf` unexpected-call guard (the shared fixture defaults to `("", nil)` so the route tests' extra ops pass). A future unrecognised tmux op inside `ensureSaverLiveness` would now go unnoticed in these two tests instead of failing loudly. If that guard is worth keeping, decide whether to add a `countOp`-based allowlist assertion on `cmder.Calls` after the call, or keep a stricter local commander for the saver tests. Accepted tradeoff of consolidation; low value.
