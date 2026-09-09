TASK: resume-hooks-silently-lost-8-11 — "The Teardown-Guard Coverage Rule Checks Call Presence, Never Call Order"
(tick-fc8976, phase 8, implementation-analysis cycle)

ACCEPTANCE CRITERIA:
- A file starting a server before isolating, or before registering the guard, fails the rule.
- A fixture reaching both helpers through a package-local arrange qualifies and passes; one whose arrange omits the guard fails.
- The rule still fails when it scans nothing.
- The tree passes under the extended rule.

STATUS: issues_found

SPEC CONTEXT:
Phase 8 is an implementation-analysis cycle, so the task body is its own authority — the specification
(.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md) says nothing about
the teardown guard or its coverage rule (grepped: no match for "teardown guard", "RegisterStateDirTeardownGuard" or
"LIFO"), and none of its ten corrigenda touch this area. The binding convention is CLAUDE.md's test-isolation
section, which states the ordering the rule now encodes: RegisterStateDirTeardownGuard is "registered after
IsolateStateForTest, before tmuxtest.New", so the bounded quiescence wait runs between kill-server and the TempDir
RemoveAll. Cleanups run LIFO, so a guard registered after the server waits at the wrong moment — which is what the
old presence-only rule could not see, and what this task closes.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/portaltest/teardown_guard_coverage_test.go:28-42 — fixtureCalls now carries ServerLine/IsolateLine/
    GuardLine positions plus LocalCalls, replacing the four booleans.
  - internal/portaltest/teardown_guard_coverage_test.go:49-73 — scannedFunc records one fixtureCalls per lexical
    scope (decl body + each func literal) and aggregate() flattens them for presence judging.
  - internal/portaltest/teardown_guard_coverage_test.go:101-112 — orderDefect, the new inverted-order arm.
  - internal/portaltest/teardown_guard_coverage_test.go:120-137 — throughLocalArrange, the single package-local hop;
    server start, isolation and guard registration all fold, each attributed to the caller's call line.
  - internal/portaltest/teardown_guard_coverage_test.go:293-322 — auditFixtureCoverage/firstDefect: presence judged
    on the folded aggregate, order judged per scope.
  - internal/portaltest/teardown_guard_coverage_test.go:338-350 — coverageFailure, with the scanned-nothing tripwire
    retained verbatim.
- Notes:
  - AC1 met: orderDefect fails both inversions and names the offending lines
    ("starts a tmux server at line N before isolating at line M" / "… before registering the teardown guard at
    line M").
  - AC2 met: throughLocalArrange follows exactly one hop, resolved within the caller's own package
    (scannedFunc.Pkg is filepath.Dir(rel)+":"+pkg at :169-170, so an identically named arrange in another directory
    or in the sibling _test package cannot answer).
  - AC3 met: coverageFailure:339-342 still fatals on scanned == 0.
  - AC4 verified by reading, not by running: I read every tmuxtest.New( occurrence in the tree (91 textual matches
    across 48 test files, of which the rule test's own fixture string constants are not calls) together with the
    IsolateStateForTest / RegisterStateDirTeardownGuard* / PORTAL_STATE_DIR sites in the same functions. No
    qualifying function inverts the order or omits a call. The thirteen non-Test functions that start a server are
    setupAbridgedEnv, setupCompositeHarness, runEagerSignalMultiSessionAC1, runRebootRoundTrip,
    setupConcurrentColdBootEnv, newDivergentRebootFixture, setupReattachEnv, newSymptomFixture, newHarness,
    setupExitClosesPane, newRebootFixture, seedRealTmuxServer and attachClient; the first eleven isolate and
    register the guard before their tmuxtest.New, and the two internal/tmux ones (seedRealTmuxServer,
    attachClient) have no caller that isolates or names PORTAL_STATE_DIR, so they stay outside the rule.
    cmd/bootstrap/helpers_integration_test.go:18's newIntegrationStateDir isolates (line 20), names PORTAL_STATE_DIR
    (line 21) and registers the guard (line 24) before returning, and all thirteen of its call sites across six
    files (phase5_integration_test.go, phase2_hook_fire_integration_test.go,
    eager_signal_hydrate_integration_test.go, scrollback_resumption_test.go,
    phase5_marker_suppression_integration_test.go, reboot_roundtrip_test.go) call it before their tmuxtest.New — so
    the five-plus bootstrap fixtures the task named are now judged and pass.
  - The three fixtures this phase repaired by hand hold the repaired order:
    internal/restore/armed_restore_integration_test.go:22/29/31 and :90/97/99,
    internal/restore/integration_test.go:53/60/62, cmd/reattach_integration_test.go:70/77/79.
  - The per-file → per-function narrowing loses no live coverage: no function in the tree starts a server through a
    method (which declKey:201-206 deliberately keeps out of the hop's reach), and every server-starting helper
    either isolates and guards itself or has no state-dir-bearing caller.

TESTS:
- Status: Adequate
- Coverage: internal/portaltest/teardown_guard_coverage_rule_test.go drives the rule over staged source fixtures
  rather than real files, as the task's Do item 4 asks. The five named tests are present under this file's existing
  top-level naming convention:
  - "starts a server before isolating" → TestCoverageRuleFailsAFixtureStartingAServerBeforeIsolating:216, which
    pins the exact defect string including both line numbers.
  - "registers the teardown guard after the server" → TestCoverageRuleFailsAFixtureRegisteringTheTeardownGuardAfterTheServer:228.
  - "counts a fixture that isolates through a package-local arrange" → TestCoverageRuleCountsAFixtureIsolatingThroughAPackageLocalArrange:240 (scanned == 1, no defects).
  - "fails a package-local arrange that omits the guard" → TestCoverageRuleFailsAPackageLocalArrangeOmittingTheGuard:257.
  - "fails when no fixture qualifies" → TestCoverageRuleFailsWhenNothingQualifies:276, asserting the "stopped
    looking" wording.
  Three further boundary tests earn their place rather than padding: :313 pins that order is judged within one
  scope only (sibling closures), :350 pins the mirror hole — a server taken from a same-package helper is folded, so
  moving the server into a helper is not a way out — and :382 pins that the hop does not resolve across packages.
  The tree-wide arm is TestTeardownGuardCoversEveryServerHostingFixture:160 itself.
- Notes:
  - Each test would fail if its arm broke: the two order tests compare the whole defect string, so a fold that
    reported the wrong line fails them, and the hop tests assert on scanned as well as defects, so a hop that
    stopped qualifying the caller fails rather than passing vacuously.
  - Not over-tested: no two fixtures exercise the same arm, and the fixture constants are the minimum file that
    expresses each shape.
  - guardPIDCall (RegisterStateDirTeardownGuardWithPIDSource, internal/portaltest/teardown_guard.go:48) has no
    staged fixture of its own; it is exercised only through the live tree
    (cmd/bootstrap/composition_e2e_harness_integration_test.go:121). That constant predates this task and no
    acceptance criterion reaches it, so it is context rather than a gap to close here.

CODE QUALITY:
- Project conventions: Followed. The guard is untagged and so runs in the unit lane while still scanning the
  integration-tagged files (stated at :184-185), it routes the repo-wide scan through
  sourceguardtest.RepoSources(t, sourceguardtest.TestSources) rather than re-authoring the walk, and the staged
  fixtures parse under sourceguardtest.ParseMode. No comment references a task id, phase or spec section (grepped).
- SOLID principles: Good. fixtureCalls owns the three rule questions as separate small methods (qualifies,
  presenceDefect, orderDefect, throughLocalArrange), scannedFunc owns the scope flattening, and the scan, the audit
  and the rendering stay separate — which is exactly what let the rule's own tests drive auditFixtureCoverage
  directly over staged records.
- Complexity: Low. The deepest nesting is scopesIn's single ast.Inspect closure; every judging method is a flat
  switch.
- Modern idioms: Yes. sort.Strings is retained from the pre-existing code and matches the repo's prevailing usage.
- Readability: Good. Every non-obvious choice carries its reason — why order is per scope (:44-48), why the fold
  attributes to the call line (:114-119), why LIFO makes a late guard useless (:97-100).
- Issues: one comment-accuracy finding below; nothing else.

BLOCKING ISSUES:
- None. All four acceptance criteria are met in substance and the tree passes the extended rule.

FINDINGS:
- [in-scope] [contained] internal/portaltest/teardown_guard_coverage_test.go:156-159 — the doc comment states the
  rule's blind spots as a closed pair ("What stays out of reach is an arrange in another package, and an inversion
  split across two scopes"), but a third exists and the sentence should name it: an arrange that starts the server
  and registers the guard while neither isolating nor naming PORTAL_STATE_DIR. Such an arrange is not judged on its
  own — qualifies() (:78-80) needs NamesStateDir or IsolateLine — and its caller folds both the server and the
  guard onto the same call line (:126-134), so orderDefect (:101-112) compares equal lines and reports nothing,
  while presenceDefect (:84-94) sees both calls made. Amend the sentence to include it (or, if the reach is to be
  widened instead, compare the arrange's own ServerLine against its GuardLine when folding). No fixture in the tree
  has this shape today: all thirteen server-starting non-Test functions either isolate themselves or have no
  state-dir-bearing caller, so this is the comment's claim being wider than the code, not a live escape. — FAILS:
  the next contributor reading the guard's stated reach concludes the inverted-LIFO shape is caught wherever it is
  written, and a fixture that registers its guard inside a non-isolating server helper passes the very rule this
  task added to catch that inversion.
