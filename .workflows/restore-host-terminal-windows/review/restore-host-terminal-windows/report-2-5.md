TASK: restore-host-terminal-windows-2-5 — Ghostty driver: thin osascript exec boundary + outcome mapping (manual live-Mac gate)

ACCEPTANCE CRITERIA:
- OpenWindow passes exactly ghosttyOpenArgv(command) to the runner (asserted via the fake runner's recorded argv).
- A fabricated clean exit (err=nil, exitCode=0) maps to OutcomeSuccess with a non-empty opaque Detail.
- A fabricated non-zero exit (exitCode=1, out with an AppleScript error) maps to OutcomeSpawnFailed with the opaque Detail carrying that output.
- A fabricated execution error (osascript not found) maps to OutcomeSpawnFailed (never a panic, never Success).
- No unit test executes real osascript; the real-window verification is manual/live-Mac only and excluded from the default lanes.
- The mapping never returns OutcomePermissionRequired in Phase 2 (permission-code mapping deferred to Phase 3).

STATUS: Complete

SPEC CONTEXT:
Spec "Testing Strategy → Driver split for testability" prescribes splitting each terminal driver into pure command-construction (Task 2.4), error-mapping (fabricated osascript outcome → typed Result), and a thin exec boundary (real osascript + TCC modal) that is manual/integration-gated only. "Irreducible manual/integration residue" makes the real-window-opens check a live-Mac manual step, not automated CI. "Permissions & Error Quarantine → Defensive net" keeps all osascript/AppleEvent specifics inside the driver, translated into the generic Result taxonomy; "Observability → detail" requires the OS-specific text to ride up only as the opaque Detail. Phase 2 scope distinguishes only success vs spawn-failed; the -1712/-1743 → permission-required code mapping is explicitly deferred to Phase 3.

Important cross-phase note: the codebase is now complete through Phase 10, so mapGhosttyResult has since gained the Phase-3 -1743/-1712 → PermissionRequired branch. This is the planned layering, not drift from Task 2.5 — the 2.5 exec-boundary + success/spawn-failed machinery is intact underneath it, and the Phase-2 unit tests remain valid because they exercise non-permission error bodies.

IMPLEMENTATION:
- Status: Implemented (with expected Phase-3 layering on top of the mapping function)
- Location:
  - internal/spawn/ghostty.go:57-59 — osascriptRunner 1-method DI seam (Run(argv) (out, exitCode, err)).
  - internal/spawn/ghostty.go:64-75 — execOsascriptRunner production impl, delegating to the shared runArgvCombined exec boundary.
  - internal/spawn/ghostty.go:80-89 — ghosttyAdapter struct + newGhosttyAdapter() constructor (the Task 2.2 registry entry, internal/spawn/resolver.go:35).
  - internal/spawn/ghostty.go:94-97 — OpenWindow: runs ghosttyOpenArgv(command) through the runner, delegates classification to mapGhosttyResult.
  - internal/spawn/ghostty.go:109-117 — mapGhosttyResult: clean → Success(successDetail), (Phase-3) -1743/-1712 → PermissionRequired, else → SpawnFailed(failureDetail).
  - internal/spawn/ghostty.go:131-145 — successDetail / failureDetail keep Detail opaque and never-empty.
  - internal/spawn/exec_boundary.go:21-33 — runArgvCombined: exec.Command through log.CombinedOutputWithContext, exitCode derived from *exec.ExitError, non-exit failure surfaced as err. Matches the plan's "clean → (stdout,0,nil); non-zero → (combined,code,nil); missing binary → err" contract exactly.
- Notes: The plan text specified newGhosttyAdapter returning &execOsascriptRunner{} (pointer); the code uses the value execOsascriptRunner{} with a value receiver on Run and a value-based interface assertion (ghostty.go:66). This is a benign improvement — execOsascriptRunner is a zero-field struct, so value semantics are idiomatic and avoid a pointless allocation; the seam contract is unchanged. The plan said to call exec.Command through log.CombinedOutputWithContext directly; the implementation factors that plumbing into the shared runArgvCombined so the Phase-4 recipe runner reuses it — a sound DRY improvement, with the interfaces/adapters kept deliberately separate.

TESTS:
- Status: Adequate
- Coverage (internal/spawn/ghostty_openwindow_test.go, unit lane):
  - "it hands the osascript argv to the runner" (26-38) — fake records argv; slices.Equal against ghosttyOpenArgv(cmd). Covers AC-1.
  - "it maps a clean osascript exit to success with an opaque detail" (40-52) — OutcomeSuccess + non-empty trimmed Detail. Covers AC-2.
  - "it maps a non-zero osascript exit to spawn-failed with the opaque output" (54-67) — uses body "-1728" (a genuine non-permission failure), asserts OutcomeSpawnFailed + Detail carries the body. Covers AC-3. Correctly chosen so the Phase-3 permission branch does not intercept it.
  - "it maps an osascript execution error to spawn-failed" (69-82) — exec-not-found error → OutcomeSpawnFailed, non-empty Detail. Covers AC-4 (never panic, never Success).
  - internal/spawn/ghostty_openwindow_manual_test.go — //go:build manual, so compiled in NEITHER `go test ./...` NOR `-tags integration ./...`; documents the exact `go test -tags manual` invocation and the human eyes-on step. Covers AC-5.
  - TestMapGhosttyResult (85-155) — Phase-3 test pinning -1743/-1712 → PermissionRequired plus three regression sub-tests re-asserting the preserved Phase-2 catch-all (non-permission non-zero → spawn-failed, exec error → spawn-failed, clean → success).
- Notes: AC-6 ("never returns OutcomePermissionRequired") held in Phase 2 and is now intentionally superseded by TestMapGhosttyResult's permission case; the 2.5 unit tests still verify the 2.5 contract because they use a non-permission body. No real osascript is ever executed by an automated test (fake runner + manual build tag). No over-testing: TestGhosttyOpenWindow exercises the adapter→runner→mapping wiring, TestMapGhosttyResult pins the pure function including the Phase-3 branch; the small overlap between them is justified as different seams (integration boundary vs pure regression anchors). Would fail if the feature broke (argv mismatch, wrong Outcome, empty Detail, or a panic on exec error each trip an assertion).

CODE QUALITY:
- Project conventions: Followed. 1-method interface DI seam (Portal's small-interface pattern), stderr-preserving log.CombinedOutputWithContext boundary helper, opaque Detail honouring the driver-quarantine rule, compile-time interface assertions (var _ osascriptRunner / var _ Adapter). Consistent with golang-design-patterns / golang-error-handling skills.
- SOLID principles: Good. OpenWindow (I/O orchestration) is cleanly separated from mapGhosttyResult (pure classification); the runner seam is dependency-inverted for testability; single-responsibility held per function.
- Complexity: Low. mapGhosttyResult is two guard clauses + fall-through; runArgvCombined a single errors.As branch.
- Modern idioms: Yes. slices.Equal in the test, errors.As for exit-code extraction, value receiver on a zero-size struct.
- Readability: Good. Doc comments explain the load-bearing escape order, the seam separation from recipeRunner, and the quarantine intent; the manual test documents the exact run command and why it is fenced.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
