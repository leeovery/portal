TASK: skip-bootstrap-when-warm-7-1 — Drop the non-vocabulary `marker` attr from the latch-write-failure WARN

ACCEPTANCE CRITERIA:
- The WARN line emits no `marker` attr; the `"error"` attr is retained.
- A whole-tree search for `"marker"` as an slog attr key in production (non-test) code returns zero results.
- The emitted message still identifies the specific latch (`@portal-bootstrapped`).
- No behaviour change on any path beyond the shape of this single log line; the latch write remains best-effort (failure swallowed, WARN under the bootstrap component).

STATUS: Complete

SPEC CONTEXT:
This is a Phase 7 (Analysis Cycle 4) log-vocabulary chore. CLAUDE.md's logging contract declares the attr-key vocabulary closed: "New components/attrs require amending the spec — never invent at call-site." The feature spec authorises the latch-write-failure path as "a pure log line (WARN under the bootstrap component)" — not a new attr key. The `marker` key was the sole call-site-minted attr outlier in the feature; its value was a compile-time constant (`@portal-bootstrapped`) carrying no runtime information beyond what the message + existing `error` attr already convey. Removing it eliminates a copy-template that could erode the closed vocabulary.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap/bootstrap.go:499
- Detail: Line 499 now reads `o.Logger.Warn("latch write failed for "+state.BootstrappedMarkerName, "error", err)`. The `"marker", state.BootstrappedMarkerName` attr pair is gone; the `"error", err` attr is retained; the marker name is folded into the message via constant concatenation (`state.BootstrappedMarkerName` = `"@portal-bootstrapped"`, confirmed at internal/state/markers.go:29). Surrounding best-effort control flow (nil-guard, swallow, no fatal, no warning append, no StepEvent) is unchanged.
- Whole-tree production search for `"marker"` as an slog attr key returns zero results (grep `'"marker"'` excluding `_test.go` → NO PRODUCTION MATCHES). The only remaining `"marker"` occurrences are: latch_test.go (the guard assertion), and internal/log/write_path_test.go:159 where `"marker"` is a log *message* argument, not an attr key. Neither is production, neither is an attr key.
- Notes: None.

TESTS:
- Status: Adequate
- Coverage: TestOrchestratorRun_swallowsLatchWriteFailureAsWarn (cmd/bootstrap/latch_test.go:128-197) drives a failing latch and asserts the full contract: (1) not fatal — Run returns nil err; (2) no warning appended (len(warnings)==0); (3) the latch write emits no StepEvent (event count == step-complete count); (4) a WARN under component=bootstrap whose message contains `state.BootstrappedMarkerName` exists; (5) that WARN carries NO `marker` attr (explicit `hasMarker` assertion at line 187-189). This directly guards every acceptance criterion of this task.
- Notes: Test asserts behaviour (message contains the marker name, absence of the `marker` attr key) rather than the exact message string, so it is robust to trivial wording changes while still pinning the contract. Message-name check uses `strings.Contains` against the constant — no brittle literal duplication. Well-scoped: not over-tested (each assertion covers a distinct guarantee), not under-tested (the negative `marker`-attr assertion is the crux of this chore and is present).

CODE QUALITY:
- Project conventions: Followed. Honours the closed attr-key vocabulary (internal/log). Message-embedded constant + `error` attr is the correct shape given a new attr key is prohibited. No golang-observability skill rule contradicts message-composition here.
- SOLID principles: N/A (single log-line shape change).
- Complexity: Low. One-line change; no new branches.
- Modern idioms: Yes. slog best practice generally favours fully-static message strings, but the concatenated segment is a compile-time constant (`@portal-bootstrapped`), so the emitted message is effectively invariant — log-aggregator groupability is preserved. Given the closed-vocabulary constraint bars the alternative (a new attr key), folding the constant into the message is the correct trade-off and matches the task's authorised solution.
- Readability: Good. Intent is clear; the adjacent comment block (lines 485-496) already documents the best-effort latch posture.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
