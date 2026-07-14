TASK: restore-host-terminal-windows-1-5 — Detect orchestrator + spawn log component

ACCEPTANCE CRITERIA:
- Outside-tmux resolved path returns the resolved Identity and emits exactly one INFO spawn record carrying terminal + bundle_id (and a detail), no WARN.
- Inside-tmux clean-NULL path (only remote clients) returns Identity{} and emits an INFO NULL-bundle outcome with no WARN.
- A transient detection error (fabricated ps/list-clients failure) returns Identity{} AND emits one WARN spawn record carrying an opaque detail; the returned identity is indistinguishable (NULL) from the clean-NULL case.
- No emitted spawn record in Phase 1 carries resolution, session, ack, opened, total, or batch (attr-key scope guard).
- NewDetector compiles against a real *tmux.Client and branches on tmux.InsideTmux().

STATUS: Complete

SPEC CONTEXT:
Spec "Observability & State Footprint" grants the spawn flow its own `spawn` log component — a spec-governed amendment to Portal's closed logging taxonomy — with a closed attr-key set. Phase 1 emits only `terminal` (friendly app name), `bundle_id` (matched family), and the opaque `detail`; `resolution`/`session`/`ack`/`opened`/`total`/`batch` arrive with adapter resolution and the burst in later phases. Spec "Detection lifecycle → Error vs clean NULL": both a clean NULL (remote/mosh → unsupported) and a transient detection error resolve to the unsupported/no-op path, but the transient error additionally emits a `spawn`-component WARN breadcrumb. Spec "Detection is a standalone operation": detection is separately callable (banner / --detect need identity without spawning), not an adapter method.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/detect.go (Detector struct :49, NewDetector :64, Detect :87, resolve :108, message/route constants :26-38, detectLogger :21)
- Notes:
  - detectLogger bound via `log.For("spawn")` at package init — introduces the new component correctly (log.For is convention-only, no runtime guard; internal/log→internal/spawn cycle-free). Deviation from CLAUDE.md's `var logger = log.For(...)` naming to `detectLogger` is deliberate and documented (avoids shadowing a function-param `logger`).
  - Detect's switch (err != nil → WARN+NULL / !IsNull → INFO resolved / default → INFO NULL-bundle) matches the plan's four-step contract. Emits exactly the closed keys: WARN carries only `detail`; resolved INFO carries `terminal`+`bundle_id`+`detail`; NULL-bundle INFO carries only `detail`. No forbidden keys, no baseline attrs added.
  - Minor drift from plan wording (not a defect): plan step 2 says branch on `errors.Is(err, ErrDetectTransient)`; Detect branches on `err != nil`. resolve() establishes and documents the invariant that every non-nil error it returns is ErrDetectTransient-wrapped (currentSession error, detectInsideTmux, detectOutsideTmux all wrap via `transient`), so the two are equivalent here — and `err != nil` is actually the safer default (any future error path still folds to WARN+NULL). Correct as written.
  - NewDetector wires all real seams (tmux.InsideTmux, os.Getenv, os.Getpid, realProcessWalker, realBundleReader, tmuxClientLister, client.CurrentSessionName, detectLogger). Message strings declared as a Phase-1 slice of the spawn closed event catalog.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/detect_test.go covers all five acceptance criteria 1:1 — resolved INFO (terminal+bundle_id+detail, no WARN), clean-NULL INFO (no terminal/bundle_id, no WARN), transient→WARN+NULL (detail carries underlying error), the attr-key scope guard across all three outcomes, the inside-tmux branch (getenv fatal-guarded, currentSession consulted, lister called with the session), and TestNewDetectorWiresProductionSeams for construction/wiring. Expected catalog messages declared independently of production constants (locks wording, no tautology). Uses logtest.NewCaptureLogger + OnlyRecord/HasAttr/AttrString and a reusable assertNoWarn helper. Not over-tested — each subtest maps to a distinct criterion; no redundant assertions.
- Notes:
  - Untested real branch: the currentSession-error → transient path in resolve() (plan step 1: "on error treat as transient"). The transient test fabricates a list-clients failure only; every other subtest returns a nil-error currentSession. A regression that dropped the currentSession error check would not be caught. See NON-BLOCKING.
  - The attr-key scope-guard subtest ("it emits only the terminal, bundle_id and detail attr keys in phase 1") is a blacklist (asserts the six forbidden keys are absent) — which is exactly acceptance criterion #4 — but the test name reads as a whitelist ("only ... attr keys"). It does not positively bound the emitted key set, so a future stray key outside the forbidden list would slip through. See NON-BLOCKING.

CODE QUALITY:
- Project conventions: Followed. One component logger per package via log.For; slog attrs passed positionally; no *slog.Logger constructed outside internal/log (uses log.For); closed attr-key discipline honoured; DI via small injectable seams; test package `spawn`, unit lane, no t.Parallel.
- SOLID principles: Good. Detector has a single orchestration responsibility; resolve() cleanly extracts the branch selection from Detect's emission; every collaborator is a 1-method (or func) seam injected at construction.
- Complexity: Low. Detect is a three-arm switch; resolve is a single if/else. No nesting concerns.
- Modern idioms: Yes. Clean error wrapping (double-%w transient), constants for catalog strings/routes, typed-nil client tolerated for construction test.
- Readability: Good. Doc comments explain the NULL-fold rationale, the detectLogger naming choice, and the resolve() every-error-is-transient invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/detect_test.go — add a subtest exercising the currentSession-error branch (insideTmux()==true, currentSession returns a non-nil error): assert Detect folds to Identity{} and emits exactly one WARN whose detail carries the underlying error. This is a real resolve() branch (plan step 1) that no current test covers.
- [quickfix] internal/spawn/detect_test.go:137-197 — the scope-guard subtest checks only that forbidden keys are absent (a blacklist). Strengthen it to positively bound the emitted key set (allowed ⊆ {component, terminal, bundle_id, detail}) so a future stray attr key outside the forbidden six is also caught, matching the subtest's "only ... attr keys" name.
