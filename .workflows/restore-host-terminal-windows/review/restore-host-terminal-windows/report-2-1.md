TASK: 2.1 — Adapter interface, typed result taxonomy & fake adapter seam (restore-host-terminal-windows-2-1 / tick-407756)

ACCEPTANCE CRITERIA:
- internal/spawn and internal/spawntest compile; go test ./internal/spawn/... ./internal/spawntest/... passes.
- Success, Unsupported, SpawnFailed, PermissionRequired yield four distinct Outcome values; only Success(...).OK() is true.
- PermissionRequired("evt -1743", "grant Automation for Ghostty") round-trips both Detail and Guidance unchanged.
- FakeAdapter.OpenWindow records the exact command argv passed (defensive copy) into Calls in call order, returns scripted Results[i] for call i, defaulting to Success once exhausted.
- No field or method on Result requires general code to parse Detail/Guidance to classify an outcome; Outcome alone is sufficient.

STATUS: Complete

SPEC CONTEXT:
The Adapter is the single-capability quarantine seam (OpenWindow(command []string) Result) that keeps every OS/terminal specific concern (AppleScript, osascript, AppleEvent codes, TCC) behind a driver, so Portal general code switches on a generic typed Result and never sees an OS-specific string/number. Spec "Permissions & Error Quarantine → Architectural boundary" (spec lines 399-403) lists the driver's typed-result taxonomy as permission-required / unsupported / spawn-failed. However spec "Detection is separate from the adapter" (lines 301-307) states unsupported is a RESOLVER-tier outcome ("a resolver maps identity → adapter … A NULL/unmatched identity → unsupported. It is not an adapter method"). These two spec passages are in tension over where "unsupported" lives; the implementation resolves it toward the resolver tier. Detail is opaque OS text riding up only as a log `detail` attr; Guidance carries permission hint text populated only on the permission path (mapping deferred to Phase 3, field exists now).

IMPLEMENTATION:
- Status: Implemented — with a deliberate, documented deviation from the written taxonomy (see below).
- Location: internal/spawn/adapter.go:9-88 (Adapter, Outcome, Result, Success/SpawnFailed/PermissionRequired, OK); internal/spawntest/adapter.go:1-102 (package doc, FakeAdapter, OpenWindow, confirmed, parseSpawnAck, compile-time guard).
- Notes:
  - DEVIATION FROM WRITTEN AC (deliberate): the AC specifies FOUR distinct outcomes incl. OutcomeUnsupported and an Unsupported(detail) constructor. The code defines THREE real outcomes (OutcomeSuccess/OutcomeSpawnFailed/OutcomePermissionRequired) plus a zero-value-invalid OutcomeUnknown sentinel; there is no OutcomeUnsupported and no Unsupported() constructor (confirmed absent tree-wide). adapter.go:21-32 documents the rationale: whether a terminal has a driver is a resolution-tier decision (resolver.go returns ResolutionUnsupported before any OpenWindow call), so OpenWindow — only ever run on a resolved supported adapter — must never report "unsupported". This is spec-consistent with "Detection is separate from the adapter", is coherently built upon by Tasks 2.2/2.6/2.7 (which gate on resolution == ResolutionUnsupported), and avoids a dead enum member + orphaned constructor. Assessed as a superior design choice, not a defect — but it leaves the task-doc AC #2 and the task title ("unsupported/spawn-failed/permission-required") describing a taxonomy the code intentionally did not build.
  - IMPROVEMENT beyond plan: OutcomeUnknown (zero value) means a bare Result{} fails OK() and can never be silently mistaken for success gating a self-attach (mirrors RecipeKind zero-invalid). Genuinely valuable safety property the plan did not call for.
  - The classify-on-Outcome-alone invariant holds: OK() reads only Outcome; Detail/Guidance are pure pass-through payloads. AC #5 fully met.
  - FakeAdapter carries later-phase (Phase 3) additions — Ack/Confirm/parseSpawnAck/FakeAckChannel wiring (adapter.go:46-99). These are correct extensions of the same seam across phases (file mtime post-dates the 2.1 close), not scope creep against 2.1; referenced symbols (spawn.ParseSpawnAckFlag, spawntest.FakeAckChannel) exist. Defensive copy via slices.Clone; recording guarded by sync.Mutex; used-by-pointer contract documented.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/adapter_test.go — four tests: three-distinct-outcomes + per-constructor stamping (correctly asserts THREE, matching the implemented taxonomy), OK()-true-only-for-Success, zero-value-is-Unknown-not-Success (covers the added sentinel), and Detail/Guidance round-trip using the exact AC fixture PermissionRequired("evt -1743", "grant Automation for Ghostty") plus non-permission constructors leaving Guidance empty. internal/spawntest/adapter_test.go (external package spawntest_test, avoids import cycle) — argv-recorded-per-call-in-order, defensive-copy (mutate caller slice after call), scripted-results-then-default-Success, default-Success-when-Results-empty; plus a compile-time spawn.Adapter guard.
- Notes:
  - All AC-mapped behaviours and the two edge cases (classify-on-Outcome-only, defensive copy) are covered and would fail if the feature broke (wrong-outcome stamping, OK() leniency, shared-backing-array recording, mis-indexed scripting).
  - Not over-tested: each test targets a distinct property; no redundant happy-path duplication; no unnecessary mocking.
  - The fake's Phase-3 Ack/Confirm branch has no direct self-test here, but it is out of Task 2.1 scope and is exercised by its Phase-3 consumers (burst_test.go). Not a 2.1 deficiency.

CODE QUALITY:
- Project conventions: Followed. Small single-method interface (golang DI style); test-only package with package doc naming the transienttest/restoretest precedent and the DI-seam role; compile-time interface guards; slices.Clone / sync.Mutex idioms; OutcomeUnknown mirrors the codebase's RecipeKind zero-invalid convention.
- SOLID principles: Good. Single-capability Adapter (ISP); Result is a plain data carrier; the fake depends only on the spawn contract.
- Complexity: Low. All functions are trivial constructors/predicates or a short record-and-replay loop.
- Modern idioms: Yes (slices.Clone, iota enum, table-driven tests).
- Readability: Good. Doc comments are precise and encode the non-obvious rationale (why unsupported is not an Outcome; why the zero value is invalid; defensive-copy intent).
- Issues: None functional.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/spawn/adapter.go:33-53 & .workflows/restore-host-terminal-windows/planning/restore-host-terminal-windows/phase-2-tasks.md:20,32,46 — The implemented taxonomy (three outcomes + OutcomeUnknown sentinel; "unsupported" owned by the resolver tier) deliberately diverges from the task-doc AC #2 / task title, which still describe four outcomes incl. an Unsupported constructor. Reconcile the converged plan record to the implemented three-outcome design (or explicitly annotate it as a superseded historical AC) so the plan artifact does not misdescribe the shipped contract. Decision needed on whether to amend a completed/converged plan doc, hence idea rather than a straight doc edit.
