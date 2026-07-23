TASK: remote-trigger-spawns-on-local-terminal-1-1 — Reproduce the bug and invert the locality gate in detectInsideTmux

ACCEPTANCE CRITERIA:
- [x] Production change confined to internal/spawn/detect_inside.go — detect.go, detect_outside.go, and the three Detect() consumers (cmd/open_burst_run.go, internal/tui/spawn_detect.go, cmd/doctor.go) untouched.
- [x] detectInsideTmux(session, lister, walker, reader) signature and clientLister seam unchanged.
- [x] Winner selection is max-Activity across ALL clients; exact tie → first-listed; only the winner is walked (no all-clients scan loop remains).
- [x] Mixed: remote most-active + idle local → NULL identity, nil error (bug fixed).
- [x] Winner walk transient-fails → NULL + ErrDetectTransient-wrapped error also wrapping the cause.
- [x] Exact activity tie → first-listed client wins.
- [x] ListClients failure → NULL + ErrDetectTransient-wrapped error.
- [x] Empty client list → clean NULL, nil error (no transient).
- [x] Single-client walk failure → NULL + ErrDetectTransient-wrapped error (retained, not deleted).
- [x] Docstring rewritten: two inverted sentences removed, new algorithm + winner-only walk + fail-safe trade described, no stale scan-all/firstWalkErr text.
- [x] :133 and :196 subtests inverted/reframed in place; combined with code change in one commit (red-before-green).
- [ ] go test / go build green — verified by reading only (reviewer does not execute; see TESTS).

STATUS: complete

SPEC CONTEXT:
Root cause (spec "Root Cause"): detectInsideTmux gated locality in the wrong order — locality as a pre-filter (drop remote/mosh NULL walks), client_activity as a tiebreak only among surviving locals. The triggering remote client (highest client_activity) is dropped by its NULL walk before its activity is consulted, so a local bystander wins and the burst drives windows onto a machine the user isn't at. The fix (spec "The Fix") inverts to select-winner-then-locality-check: pick the max-client_activity client across ALL clients (first-listed on tie), walk ONLY that winner, and branch on its locality. Spec "Owned Behaviour Change" mandates the docstring own the deliberately-dropped walk-resilience property (a transient winner walk now fails safe to NULL + WARN rather than falling back to a lower-activity local). Behaviour change is "sometimes no-op where it used to drive, never drive where it shouldn't" — no new false-drive possible.

IMPLEMENTATION:
- Status: Implemented (correct)
- Location: internal/spawn/detect_inside.go:92-126 (detectInsideTmux + selectTriggeringClient); docstring :49-91.
- Notes:
  - selectTriggeringClient (:118-126) seeds winner = clients[0] and replaces only on strictly-greater Activity (`client.Activity > winner.Activity`), correctly implementing max-across-all with first-listed-on-tie. No walking during selection.
  - detectInsideTmux (:92-112): ListClients error → transient(...) (:95); empty slice guard → Identity{}, nil (:98-101) — this guard also prevents a clients[0] panic in selectTriggeringClient; then walks ONLY the winner via `return walkToBundle(winner.PID, walker, reader)` (:111).
  - The old scan-all loop with best/bestActivity/localFound/firstWalkErr is fully gone (confirmed via git show 57a503cb).
  - The current file collapses T1-1's explicit three-branch winner propagation into a direct `return walkToBundle(...)` passthrough. This is the later T2-1 cleanup (flagged in the review notes as a separate task), and is behaviourally identical: walkToBundle's three-shape return (resolved/nil, NULL/nil, NULL/ErrDetectTransient) is exactly the three outcomes the explicit branching produced. No 1-1 regression.
  - Docstring (:49-91) fully rewritten: describes select-winner-then-locality-check, winner-only walk, and OWNS the dropped-walk-resilience trade explicitly (:71-77). Both inverted sentences ("NULL-filtering is the primary signal", "client_activity is used ONLY to disambiguate among host-local clients — never as a cross-client primary signal") are removed. No stale scan-all/firstWalkErr contract text remains.
  - Signature and clientLister seam unchanged. Confinement confirmed: commit 57a503cb touched only detect_inside.go + detect_inside_test.go (plus .tick/tasks.jsonl and manifest.json task-tracking artifacts) — detect.go, detect_outside.go, and the three consumers untouched by this task.

TESTS:
- Status: Adequate
- Coverage (internal/spawn/detect_inside_test.go):
  - Inverted :133 → "it returns NULL when the most-active client is remote even with a local bystander" (:108) — remote 601 Activity 9999 + local 501 Activity 1 → got.IsNull(), nil error. Primary regression for the reported bug.
  - Reframed :196 → "it fails safe to NULL when the most-active winner walk transiently fails" (:212) — flaky 601 Activity 100 (ProcessInfo → psFailure) + resolvable local 501 Activity 50 → got.IsNull(), errors.Is(err, ErrDetectTransient), errors.Is(err, psFailure). Locks the dropped-resilience fail-safe.
  - Retained green invariants all present: pure-remote NULL (:59), single-local drive (:67), 2+ locals highest-activity listed-second (:77), higher-activity listed-first (:85), exact-tie first-listed (:93), list-clients failure transient (:167), single-client walk failure transient (:187, retained — NOT deleted as a :196 duplicate), empty → clean NULL (:132).
  - Session passthrough asserted in the shared harness (:149-151): lister asked exactly once about "dev".
  - git show 57a503cb confirms the two subtest transforms and the code inversion landed in the SAME commit, so the transforms go red against pre-fix code and green against post-fix code (red-before-green satisfied for a combined test+code change).
- Not over-tested: the two order-variant happy rows (:77 listed-second, :85 listed-first) each pin a distinct property (not-last-wins vs not-first-wins) and are not redundant. Each subtest maps to exactly one contract row.
- Not under-tested: every spec "Edge Contracts to Pin" and "Existing invariants" row has a pinning subtest.
- Note: go test was NOT executed (reviewer constraint — no test execution); test/implementation consistency verified by reading. Each assertion matches the code's actual return for its seeded inputs; all subtests would pass against the current code.

CODE QUALITY:
- Project conventions: Followed. 1-method DI seams (clientLister, ProcessWalker, BundleReader) preserved; table-driven happy path + separate error-path subtests match golang-testing conventions; no t.Parallel(); error wrapping via transient(...) preserves the cause in the chain (errors.Is reachable), consistent with golang-error-handling.
- SOLID principles: Good. selectTriggeringClient is a pure single-responsibility helper (selection only, no walking); detectInsideTmux orchestrates enumerate → guard → select → walk.
- Complexity: Low. Linear scan for the winner; single winner walk; three clear return shapes delegated to walkToBundle.
- Modern idioms: Yes. Idiomatic Go slice iteration and error wrapping.
- Readability: Good. Docstring is thorough and explicitly owns the resilience trade; inline comments explain the empty-slice guard and the winner-only-walk rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
