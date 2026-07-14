TASK: restore-host-terminal-windows-6-1 — Async terminal-detection lifecycle + caching

ACCEPTANCE CRITERIA:
- Reaching PageSessions dispatches exactly one detection command; DetectDispatched() becomes true and a subsequent terminalDetectedMsg sets DetectResolved() true with the cached DetectedIdentity().
- The terminalDetectedMsg arm caches the adapter resolution (DetectedResolution()) alongside the identity, computed once via the injected config-aware m.resolve; DetectUnsupported() true for a resolved-ResolutionUnsupported identity (NULL remote/mosh OR a non-NULL undriven identity like com.apple.Terminal), false for native/config; a rebuild never re-resolves.
- Before terminalDetectedMsg: DetectDispatched() && !DetectResolved() (in-flight); after a NULL identity: DetectResolved() && DetectedIdentity().IsNull() (resolved NULL) — the two states distinguishable.
- A rebuild (s-toggle, SessionsMsg refresh, filter apply/clear, projects-edit->Sessions return) does not dispatch a second detection command (Detect call count stays 1).
- A transient detection error (fake returns spawn.Identity{}) caches as unsupported (IsNull() true); the model emits no additional WARN.
- A direct warm Sessions entry (no loading page) dispatches detection exactly once.
- The detection command is never part of the first-paint appearance gate.

STATUS: Complete

SPEC CONTEXT: Spec "Terminal Identity & Detection -> Detection lifecycle" mandates: detect once per picker session on Sessions-page entry, cached (rebuild must not re-walk); off the ~50ms first-paint appearance-gate path (async, tens-of-ms walk); a transient detection error and a clean NULL both resolve to the unsupported/no-op path (the transient WARN is emitted inside Phase-1 spawn.Detector.Detect, not the TUI); the gate distinguishes in-flight (not yet set) from a resolved NULL. The detection identity is consumed by the proactive unsupported banner (6-2) and the N>=2 Enter gate (6-3/6-9), which key on the cached Resolution via DetectUnsupported(), not on IsNull().

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria; forward-compatible supersets from later tasks noted).
- Location:
  - internal/tui/spawn_detect.go — TerminalDetector seam (Detect() spawn.Identity), terminalDetectedMsg, WithTerminalDetector / WithResolve (both nil-tolerant), maybeDispatchDetectionCmd (spawn_detect.go:82-91), and all five test accessors incl. DetectUnsupported() (spawn_detect.go:109-118).
  - internal/tui/model.go — Model fields detector/resolve/detectIdentity/detectResolution/detectAdapter/detectResolved/detectDispatched (model.go:464-478); terminalDetectedMsg Update arm caching identity + adapter + resolution from ONE resolve (model.go:2459-2483); dispatch wired at three guarded sites — warm SessionsMsg after evaluateDefaultPage (model.go:2258, with the cold PageLoading early-return at 2245-2247), cold LoadingMinElapsedMsg (model.go:2275), cold BootstrapCompleteMsg (model.go:2318).
  - internal/tui/build.go — Deps.Detector + Deps.Resolve (build.go:53-54), always-injected nil-tolerant WithTerminalDetector/WithResolve (build.go:207-208), plus WithInitialDetection harness seed (build.go:240).
  - cmd/open.go — production wiring from the shared buildProductionSpawnSeams: detector = spawnSeams.Detector, resolve = spawnSeams.Resolve (the config-aware NewResolver(terminals.json).Resolve), the single injection site reused by the later burst (open.go:598-599, 426-427).
- Notes:
  - Guard chain in maybeDispatchDetectionCmd (nil detector | detectDispatched | activePage != PageSessions) is correct; the detectDispatched latch makes every re-entry a no-op. Pointer receiver mutates the addressable Update-local value; the mutation persists on the returned model. Verified.
  - The async closure captures a local `detector := m.detector` and returns terminalDetectedMsg only — the walk runs on Bubble Tea's command goroutine, never inside Update (proven by the calls==0-before-drain assertion). Correct off-first-paint behaviour.
  - DRIFT (expected, non-blocking): the terminalDetectedMsg arm also caches detectAdapter (documented as the §10-1 addition — retaining the Adapter half of the single resolve so the burst gate and the adapter can never disagree) and handles pendingBurstEnter/decideBurst (6-3/6-9). These are supersets layered onto the same arm by later phase-6 tasks; 6-1's specified behaviour (cache identity + resolution, flip detectResolved) is fully intact. Not a 6-1 defect — all phase-6 tasks are done.
  - Empty-sessions landing (evaluateDefaultPage -> PageProjects) does not dispatch detection, and the projects-edit->Sessions return deliberately does not either; but any subsequent SessionsMsg while on PageSessions re-enters the guarded dispatch, so detection is not permanently lost and the zero-session case (nothing to burst) is moot. By design.

TESTS:
- Status: Adequate
- Coverage (internal/tui/spawn_detect_test.go + build_test.go):
  - Warm dispatch-once + async (calls==0 pre-drain, ==1 post-drain) + cached identity — TestDetection_WarmSessionsEntry_DispatchesOnce.
  - In-flight vs resolved-NULL distinction — TestDetection_InFlightVsResolvedNull.
  - DetectUnsupported truth table over the real EMPTY-config resolver: NULL -> unsupported, com.apple.Terminal (non-NULL undriven) -> unsupported, ghostty -> native — TestDetection_Unsupported_Predicate. Using the production resolver (not a fake resolution map) keeps the non-NULL-yet-unsupported distinction honest.
  - Transient error caches unsupported (NULL shape) — TestDetection_TransientError_CachesUnsupported.
  - No re-dispatch / no re-resolve on every rebuild path, asserting BOTH Detect and resolve call counts stay 1 and the cache is untouched — s-toggle, SessionsMsg refresh, filter apply/clear, projects-edit->Sessions return.
  - Cold loading->Sessions dispatch-once with the post-restore refetch NOT re-dispatching — TestDetection_ColdLoadingToSessions_DispatchesOnce.
  - Independent of the appearance gate (gate resolves with detection undispatched; loading-page dispatch guarded off) — TestDetection_IndependentOfAppearanceGate.
  - Nil detector never dispatches — TestDetection_NilDetector_NeverDispatches.
  - Build wires + is nil-tolerant for Detector/Resolve — TestBuild_WiresDetectorAndResolve; WithInitialDetection seeds resolved-unsupported — build_test.go "initial detection seeds the resolved unsupported cache".
  - All six plan-named tests are present (mapped) plus justified extras (cold route, appearance-gate independence, nil detector, build wiring). Every acceptance criterion has a corresponding assertion.
- Notes:
  - Not over-tested: the fakeDetector/countingResolve are minimal call-counters; shared assertDetectionResolvedOnce/assertNoReDispatch helpers dedupe the rebuild-path setup rather than copy-paste. Focused on behaviour (dispatch/resolve/cache), not internals.
  - Minor coverage nuance: the "model emits no additional WARN" criterion is not asserted with a logtest.Sink. In practice the detection lifecycle has NO logger call at all (the model's only spawn logger is the §6-10 burst-summary one, untouched here), so there is no WARN path to fire — the assertion would only guard against a future regression that adds logging into the arm. Low value; see NON-BLOCKING NOTES.

CODE QUALITY:
- Project conventions: Followed. 1-method DI seam behind an interface (TerminalDetector), nil-tolerant With* options, config-shape Deps, test accessors mirroring the existing convention — all match the codebase's DI/testing pattern. No t.Parallel (consistent with the tui test surface). Async command mirrors the tea.RequestBackgroundColor / bootstrap-progress off-first-paint pattern.
- SOLID principles: Good. Single-responsibility seam; the single resolve injection site avoids duplicate resolver construction; DetectUnsupported centralises the "cannot spawn host windows" predicate so the banner and gate cannot drift.
- Complexity: Low. maybeDispatchDetectionCmd is a three-clause guard + a closure; the Update arm is three assignments plus the (later-task) deferred-Enter branch.
- Modern idioms: Yes. Value-vs-pointer receiver split is correct and documented; closure captures a local copy of the detector.
- Readability: Good. Comments are dense but load-bearing (they record why the resolution and adapter are both cached, and why the pointer receiver is needed).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/spawn_detect_test.go:188 (TestDetection_TransientError_CachesUnsupported) — optionally add a logtest.Sink assertion that the transient/NULL detection path emits zero records, to guard the "no additional WARN" acceptance criterion against a future regression that introduces logging into the terminalDetectedMsg arm. Low value today (the detection lifecycle wires no logger), so this is a regression-guard nicety, not a gap.
