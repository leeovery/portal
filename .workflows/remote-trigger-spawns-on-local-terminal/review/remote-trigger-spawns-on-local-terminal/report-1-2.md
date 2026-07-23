TASK: remote-trigger-spawns-on-local-terminal-1-2 — Guard the fix against over-correction with a local-most-active regression test

ACCEPTANCE CRITERIA:
- [ ] A new subtest exists in internal/spawn/detect_inside_test.go for the local-most-active + remote-idle-bystander scenario.
- [ ] It seeds a most-active local client and a lower-activity remote bystander, with the local NOT first in the slice (so the pass proves max-by-activity, not first-listed luck).
- [ ] It asserts the local .app identity resolves (BundleID == "com.mitchellh.ghostty") with a nil error.
- [ ] No production files are modified by this task.
- [ ] The subtest passes against the post-fix detectInsideTmux (and — being an over-correction guard — also passes against pre-fix code).
- [ ] go test ./internal/spawn/... passes; go test ./... (unit lane) passes.

STATUS: complete

SPEC CONTEXT:
The bug: a spawn burst triggered from a remote tmux client while a host-local client shares the session used to resolve the local terminal as host, spawning windows on the wrong machine. Task 1-1 inverted the gate to select-winner-then-locality-check (most-active client is the trigger; walk only the winner). Spec Testing Requirements §"New coverage to add" names exactly ONE genuinely net-new scenario: "Local most-active, remote idle bystander → the local drives. This is the only genuinely net-new scenario (no existing subtest covers it) — it guards against an over-correction that would refuse a legitimate local spawn because a remote client is merely attached." The other two target scenarios (remote most-active → NULL; transient winner walk → NULL+transient) are covered by Task 1-1's transforms. This task adds the mirror-of-the-bug over-correction guard only.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/detect_inside_test.go:115-130 (the scenario as a table row inside TestDetectInsideTmux, harness at :138-165). Introduced test-only by commit 2cd831de as a standalone t.Run; T3-1 (d8456bb3) collapsed the seven happy-path subtests into the table, and this scenario survived 1:1 as a row.
- Notes:
  - Scenario survived the T3-1 refactor intact (verified against d8456bb3 --stat: test-only, 1:1 survival; the row's inputs/expectations match the original standalone subtest from 2cd831de).
  - Test-only confirmed: git show 2cd831de --stat touched only internal/spawn/detect_inside_test.go plus .tick/tasks.jsonl and the workflow manifest.json (workflow bookkeeping, not production/source). Zero production files modified. Acceptance criterion "No production files are modified" satisfied.
  - detectInsideTmux/detect_inside.go is unchanged by this task (supporting context only).

TESTS:
- Status: Adequate
- Coverage:
  - Row name: "it drives the local client when it is most-active despite an idle remote bystander".
  - Seeds via fakeClientLister: remote/mosh bystander {PID: 601, Activity: 50} listed FIRST, local Ghostty {PID: 501, Activity: 200} listed SECOND. Local is most-active AND not first-listed — so a green result proves max-by-activity selection (selectTriggeringClient at detect_inside.go:118 replaces only on strictly-greater Activity), not order-luck. Mirrors the existing :77 row's intent.
  - Seams wired via localWalkSeams() (501→Ghostty .app, 601→mosh-server→NULL); no bespoke fakes — matches the task's "reuse localWalkSeams()" instruction. Shared fakes (fakeWalker/fakeReader/fakeProc/fakeBundle) are defined in walk_test.go — package compiles.
  - Assertions (table harness): err != nil → Fatalf (nil-error requirement); session passthrough == [dev]; BundleID == "com.mitchellh.ghostty" (wantBundleID); Name == "Ghostty" (wantName).
  - Post-fix behaviour: winner = 501 (200 > 50) → walk 501 → Ghostty .app → resolved identity, nil error → BundleID/Name match. Passes.
  - Pre-fix behaviour: old filter-then-tiebreak drops the remote (NULL walk) and keeps local 501 → also resolves Ghostty → also passes. Correctly a prevention/over-correction guard, not the primary regression (consistent with the spec/plan characterisation).
- Notes:
  - The explicit !got.IsNull() check present in the original standalone subtest (commit 2cd831de) is no longer a distinct assertion in the table form, but is subsumed: a NULL identity yields BundleID == "" which fails the wantBundleID assertion. The task marked !IsNull() as optional, so no coverage is lost. Not a finding.
  - Not over-tested: single focused row for the sole net-new scenario; the two adjacent scenarios are deliberately covered elsewhere (1-1 transforms), so no near-duplicate exists.
  - Not under-tested: the discriminating property (max-by-activity vs first-listed) is genuinely exercised by the remote-first ordering.

CODE QUALITY:
- Project conventions: Followed — table-driven t.Run, no t.Parallel(), shared fixture reuse, matches the surrounding file's established patterns (mirrors the :77 "listed SECOND" convention).
- SOLID principles: Good (test scope; uses the existing 1-method DI seams).
- Complexity: Low — a single declarative table row.
- Modern idioms: Yes — idiomatic Go table test, per-row t.Run.
- Readability: Good — the row comment states why the remote is listed first (proves max-by-activity, not first-listed luck) and frames the scenario as the mirror of the reported bug.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

VERIFICATION NOTE: Per verifier constraints the suite was not executed; test adequacy and green-ness were assessed by reading. The package compiles (all referenced fakes/types resolve), and the row's logic passes against both the post-fix (detect_inside.go:92-126) and pre-fix selection paths as traced above.
