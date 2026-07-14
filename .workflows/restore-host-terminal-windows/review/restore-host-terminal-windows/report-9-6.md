TASK: restore-host-terminal-windows-9-6 — Derive burstAllConfirmed from the shared PartitionResults chokepoint (tick-284b53)

ACCEPTANCE CRITERIA:
- burstAllConfirmed derives all-confirmed from spawn.PartitionResults(...)'s failed==empty relationship; no residual `for … !r.Confirmed()` loop remains.
- The msg.Err == nil and len(msg.Results) == len(m.burstExternal) guards are preserved; the truth table is unchanged for all-confirmed, partial-failure, permission, error, and length-mismatch inputs.
- Unit (tui): burstAllConfirmed true only for error-free full-length all-AckConfirmed spawnCompleteMsg; false for any AckTimeout/AckFailed, a msg.Err, or a length mismatch.
- Cross-caller parity: a shared []spawn.WindowResult fixture table reaches the same terminal classification on CLI and picker paths, asserted against spawn.PartitionResults / spawn.FirstPermission.
- Regression: existing burst full-success self-attach and partial-failure suites pass unchanged.

STATUS: Complete

SPEC CONTEXT: classify.go (internal/spawn) is the spec-designated single count-semantics chokepoint: a WindowResult is "opened" exactly when Confirmed()==true, and "a batch is all-confirmed precisely when the returned failed slice is empty." Both callers — the CLI (cmd/spawn.go runSpawn) and the TUI picker — must derive every opened/failed/all-confirmed/permission decision from the three pure functions (Confirmed, PartitionResults, FirstPermission) so the two orchestration paths cannot drift. Before this task the picker's burstAllConfirmed used a parallel hand-rolled !r.Confirmed() loop; the CLI already gated on PartitionResults' failed==empty. This task closes that residual drift risk.

IMPLEMENTATION:
- Status: Implemented (matches the tick's prescribed solution verbatim)
- Location: internal/tui/burst_progress.go:250-253
- Notes: burstAllConfirmed is now a single expression: `_, failed := spawn.PartitionResults(msg.Results); return msg.Err == nil && len(msg.Results) == len(m.burstExternal) && len(failed) == 0`. The hand-rolled `for … if !r.Confirmed()` loop is gone (grep confirms only doc-comment mentions of "!r.Confirmed()" remain in burst_progress.go:242 and the test file header; no live loop in tui or cmd/spawn.go). Both msg.Err and dual-length guards preserved. The gate is identical to the CLI's runSpawn (cmd/spawn.go:179-181: `_, failed := spawn.PartitionResults(results); if len(failed) > 0`), so cross-path parity holds by construction. Doc comment (240-249) accurately explains the derivation and why the N=1 length guard stays. Consumption site (model.go:2519) unchanged: `if m.burstAllConfirmed(msg) && !m.burstCancelled` — behaviour preserved.

TESTS:
- Status: Adequate
- Coverage: internal/tui/burst_all_confirmed_test.go. TestBurstAllConfirmed_TruthTable (8 cases) pins the full gate contract: all-confirmed→true; one timeout→false; one failed→false; permission→false; msg.Err set with otherwise-all-confirmed→false; length mismatch too-few→false; too-many→false; empty-results-vs-non-empty-external→false. TestBurstAllConfirmed_ClassificationParityWithChokepoint (7 cases) derives the terminal 3-way class from the shared spawn.PartitionResults/FirstPermission primitives (canonicalBurstClass) and asserts (a) the fixture reaches its declared class, (b) burstAllConfirmed equals `class==all-confirmed`, and (c) burstAllConfirmed equals the raw `len(failed)==0` from spawn.PartitionResults — the exact expression runSpawn gates on.
- Notes: The permissionResult fixture correctly models a permission-walled window as Ack=AckFailed (never opened), so len(failed)==0 stays equivalent to all-Confirmed even when a permission result is present — a genuine edge case, not redundant. The parity test anchors to the shared spawn primitives rather than executing cmd/spawn.go's runSpawn directly; this is exactly what the tick's acceptance specified ("asserted against spawn.PartitionResults / spawn.FirstPermission"), and CLI parity is guaranteed by both callers resting on the same primitive — a heavier end-to-end runSpawn drive is out of scope for this chokepoint-derivation task. Truth-table and parity tests overlap on a few fixtures (timeout/failed/permission) but test distinct aspects (full gate contract incl. err/length guards vs. chokepoint derivation), so this is not over-testing. Would fail if the loop were reintroduced with different semantics or a guard dropped. Regression suites not run (per instructions) but the consumption arm is logically unchanged.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md tui convention); white-box package tui test drives a bare Model literal with only burstExternal set — no seams/goroutines, matching the "small interface / minimal setup" ethos. Declarative fixture helpers (confirmedResult/timeoutResult/failedResult/permissionResult/sessionsOf) keep tables readable.
- SOLID: Good. Single responsibility; the gate now delegates the count semantics to the one chokepoint (removes duplicated logic — the whole point of the task).
- Complexity: Low. One PartitionResults call + a three-clause boolean.
- Modern idioms: Yes. Idiomatic Go, table-driven subtests.
- Readability: Good. Doc comment explains the derivation, the CLI parity, and why the length guard covers the vacuous N=1 case.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
