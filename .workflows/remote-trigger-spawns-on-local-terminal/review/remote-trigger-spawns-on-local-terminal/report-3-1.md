TASK: remote-trigger-spawns-on-local-terminal-3-1 (T3-1) — Collapse the seven happy-path detectInsideTmux subtests into one table-driven test

ACCEPTANCE CRITERIA:
- The seven happy-path subtests are replaced by one table-driven test; each original scenario survives as a named row with its exact input slice and expected outcome (NULL vs bundle/name), including the deterministic first-listed-on-tie assertion and the max-by-activity assertions for the higher client listed both first and second.
- The session-passthrough assertion (lister.calls == [dev]) is preserved.
- The three error-path subtests (list-clients failure, single-client walk failure, winner-walk transient → ErrDetectTransient + preserved underlying cause) remain separate and behaviourally unchanged.
- The zero-clients clean-NULL coverage remains (in the table or standalone).
- No production code changes — internal/spawn/detect_inside.go is untouched; test-file-only edit.
- The spec-pinned invariants all still hold: pure-remote→NULL, single-local→drive, 2+ locals highest-activity wins, exact tie first-listed wins, remote-most-active→NULL, local-most-active→drive.

STATUS: complete

SPEC CONTEXT:
This is a chore-type, test-only structural refactor added during analysis cycle 3 of the remote-trigger-spawns-on-local-terminal bugfix. The underlying feature (detectInsideTmux) implements select-winner-then-locality-gate: enumerate a session's tmux clients, pick the most-active (the burst trigger, first-listed wins a tie), walk ONLY that winner's process tree, and drive/no-op/fail-safe on its locality. The bug being fixed was that a local bystander was driven when the most-active client was remote (spawning windows on the wrong machine). T3-1 does not change any of that behaviour — it only collapses the seven near-duplicate happy-path subtests exercising the winner-select-then-locality-gate matrix into one table-driven test, per the project's Go testing conventions.

IMPLEMENTATION:
- Status: Implemented (faithful, 1:1)
- Location: internal/spawn/detect_inside_test.go:45-165 (table + shared body); error paths at :167-245
- Notes: Verified against the pre-commit version (d8456bb3^) line-by-line. All seven happy-path scenarios survive as named rows with byte-identical input slices and expected outcomes:
  1. all remote/mosh {601,100},{602,200} → wantNull (test.go:58-65) ✓
  2. single local {501,Activity:0} → com.mitchellh.ghostty / Ghostty (:66-73) ✓
  3. two locals, higher listed SECOND {501,100},{502,200} → com.apple.Terminal (:74-83) ✓
  4. two locals, higher listed FIRST {502,200},{501,100} → com.apple.Terminal (:84-91) ✓
  5. exact tie {501,150},{502,150} → first-listed com.mitchellh.ghostty (:92-99) ✓
  6. remote most-active + local bystander {601,9999},{501,1} → wantNull, the bug's shape (:100-114) ✓
  7. local most-active + idle remote bystander (remote listed FIRST) {601,50},{501,200} → com.mitchellh.ghostty / Ghostty (:115-130) ✓
  Zero-clients clean-NULL folded into the table as row 8 (clients: nil, wantNull) at :131-135 — coverage preserved.
  Production file internal/spawn/detect_inside.go is NOT in the commit (git show d8456bb3 --stat lists only .tick/tasks.jsonl, the plan manifest.json, and detect_inside_test.go). Untouched — confirmed.

TESTS (assessed by reading — not executed, per verifier constraints):
- Status: Adequate — refactor preserves coverage exactly, no net assertion change
- Coverage: Shared per-row body (:138-165) performs the single localWalkSeams() + detectInsideTmux("dev", ...) call, asserts err == nil, then the IsNull-or-bundle branch. wantName is asserted only when non-empty, matching each original subtest's assertion shape (rows that asserted BundleID only keep BundleID-only; rows that asserted BundleID+Name assert both). The session-passthrough assertion (len(lister.calls)==1 && calls[0]=="dev") is now hoisted into the shared body so it runs for EVERY row — a strict superset of the original (which only asserted it in the all-remote subtest). This holds for the zero-clients row too, since detectInsideTmux calls ListClients before the empty-list early return.
- The three error-path subtests remain separate and byte-identical to the pre-commit versions (verified against d8456bb3^): list-clients failure (:167-185), single-client walk failure (:187-210), most-active-winner walk transient (:212-245). Each still asserts errors.Is(err, ErrDetectTransient), the preserved underlying cause (errors.Is(err, listFailure/psFailure)), and got.IsNull().
- Compilation sanity (by reading): row struct fields {name, clients, wantNull, wantBundleID, wantName} all consumed; fakeWalker/fakeReader/fakeProc/fakeBundle are defined in walk_test.go (same package); errors import still used by the error paths; ClientActivity / detectInsideTmux / ErrDetectTransient all exist. No unused symbols introduced.
- Notes: One assertion was implicitly narrowed but not lost — scenario 7's original explicit !got.IsNull() check is gone, but it is fully subsumed by the wantBundleID == "com.mitchellh.ghostty" assertion (a NULL identity has an empty BundleID, so a NULL result would fail the BundleID check). No coverage loss.

CODE QUALITY:
- Project conventions: Followed. Table-driven with named subtests is best-practice #1 in .claude/skills/golang-testing and the idiomatic form the same package already uses (walk_test.go, detect_test.go, and 7 other spawn _test.go files). No t.Parallel() added — correct for this project (CLAUDE.md prohibits it; the cmd/mock injection rationale applies package-wide by convention).
- SOLID principles: N/A (test code); single-behaviour-per-table is well-scoped.
- Complexity: Low. One loop, one branch on wantNull, one conditional Name assertion.
- Modern idioms: Yes — standard t.Run(tt.name, ...) table pattern; no loop-var capture hazard (Go 1.22+ semantics; project is well past that).
- Readability: Good. Per-scenario rationale comments (max-by-activity listed-first/second, tie determinism, bug shape and its mirror) are carried onto the rows verbatim, so the "why" of each fixture survives the collapse.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/detect_inside_test.go:22 — Stale doc comment: "// ghosttyProc/terminalProc are single-hop ancestries that resolve to a .app." references identifiers ghosttyProc/terminalProc that do not exist; the vars are ghosttyCommand/ghosttyAppPath/terminalCommand/terminalAppPath. PRE-EXISTING (outside this task's diff — the hunk starts at line 43); flagged for a trivial wording fix, not a regression from T3-1.
