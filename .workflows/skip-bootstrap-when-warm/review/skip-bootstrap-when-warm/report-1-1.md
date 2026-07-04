TASK: skip-bootstrap-when-warm-1-1 — Latch read/verdict helper with version-aware three-way semantics

ACCEPTANCE CRITERIA:
- state.BootstrappedMarkerName equals "@portal-bootstrapped".
- BootstrappedLatchSatisfied returns true only when found==true AND val==runningVersion.
- Absent (found==false) -> false.
- Present but version-mismatch -> false.
- Read-error / down-server (err != nil) -> false (error swallowed; bare bool, not (bool, error)).
- Empty stored value with a non-empty running version -> false.
- Running version is a plain string parameter — no cmd.version import in internal/state, no global.
- go build / go test ./internal/state/... pass; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
specification.md "The Version-Stamped Latch" (§65-98) defines @portal-bootstrapped as a tmux server option whose VALUE is the binary version (not presence-only). "Satisfied" = present AND stored version == running cmd.version. The spec's outcome table (§79-88) enumerates: absent -> not satisfied; present+match -> satisfied; present+mismatch -> not satisfied (post-upgrade); read-error/down-server -> not satisfied. Both mismatch and unreadable deliberately fold into "not satisfied -> full bootstrap"; no separate ServerRunning() probe is needed because the read fails gracefully on a down server. Value format is parse-free plain equality in v1 (§90). The helper is the read-side primitive; the Phase 2 consumer (root.go) wants a single boolean verdict (§140-155).

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria and spec exactly)
- Location:
  - internal/state/markers.go:29 — BootstrappedMarkerName = "@portal-bootstrapped" const with the required doc comment (same mechanism as @portal-restoring; dies with server; VALUE load-bearing).
  - internal/state/markers.go:166-204 — BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool; body is exactly the specified TryGetServerOption read -> err?false -> !found?false -> val==runningVersion.
- Notes:
  - Reuses the existing RestoringChecker seam verbatim (markers.go:51) — no new interface added, per DO.
  - internal/state stays a leaf: `grep` confirms no `cmd` import; the only "cmd.version" occurrences are in doc comments (markers.go:177, 191). runningVersion is a plain parameter, no global.
  - Godoc (markers.go:166-194) documents the full four-outcome fold, the parse-free equality rationale, the empty-value-not-a-special-case note, and explicitly contrasts against IsRestoringSet's error-propagation ("Do not 'fix' this into a (bool, error) signature") — satisfying every DO godoc requirement.
  - Consumers wired correctly and consistently with the plan: cmd/root.go:173 (Phase 2 verdict, nil-guarded) and cmd/bootstrap/bootstrap.go:498 (latch write uses the same constant). No drift.
  - Minor terminology: godoc/task call it a "four-outcome verdict" while the spec §77 heading says "three-way outcome" (spec still lists 4 rows). Task DO explicitly mandated the four-outcome phrasing; this is intentional and not a defect.

TESTS:
- Status: Adequate
- Coverage (internal/state/markers_test.go:216-262, TestBootstrappedLatchSatisfied): all six required subtests present with the exact prescribed names and inputs —
  - present+match -> true (checkerMock{val:"1.2.3",found:true}, "1.2.3")
  - absent -> false (found:false)
  - version-mismatch -> false ("1.2.2" vs "1.2.3")
  - read-error/down-server -> false (err set)
  - empty stored value + non-empty running -> false (val:"",found:true)
  - name assertion -> BootstrappedMarkerName == "@portal-bootstrapped" AND gotName == the constant.
- Reuses the existing checkerMock (records gotName at line 31-34); mirrors the TestIsRestoringSet table style; no t.Parallel() — all per the TESTS directive.
- Would-fail-if-broken: yes. Flipping the equality, dropping the err/!found guards, or renaming the constant each fail a distinct subtest.
- Not over-tested: six focused subtests, one behaviour each; no redundant assertions, no unnecessary mocking. The name subtest bundles the const-value check with the gotName check, but both are load-bearing acceptance criteria, so this is not padding.

CODE QUALITY:
- Project conventions: Followed. Colocated with RestoringMarkerName/IsRestoringSet precedent; seam reuse and leaf-purity match the internal/state import-cycle discipline documented in CLAUDE.md and the golang-design-patterns/structs-interfaces skills. Godoc on every exported symbol per golang-code-style.
- SOLID principles: Good. Single responsibility (verdict only, no side effects), depends on the narrow RestoringChecker interface (DIP/ISP).
- Complexity: Low. Two guard returns + one equality; cyclomatic complexity 3.
- Modern idioms: Yes. Idiomatic guard-clause style; bare-bool return with swallowed error is a deliberate, documented contract, not an oversight.
- Readability: Good. Self-documenting; the godoc pre-empts the most likely future "bug fix" (converting to (bool, error)).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
