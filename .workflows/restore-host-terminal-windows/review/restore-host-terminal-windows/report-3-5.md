TASK: 3.5 — Token-ack self-attach gate + per-window spawnAckTimeout (restore-host-terminal-windows-3-5)

ACCEPTANCE CRITERIA:
1. AttachCommand("s",…,…,"b1","t1") appends "--spawn-ack","b1:t1" as the final two argv elements, preserving the env-strip/PATH-only/own-binary prefix.
2. Burster.Run(["s1","s2"]) with the fake confirming both returns two AckConfirmed WindowResults, and cmd/spawn.go then calls Connect("s3") exactly once, after Ack.Clean(batch).
3. Each window's ack timer is independent (per-window budget starting at its own spawn, not one global clock from Enter).
4. A token that arrives late but within spawnAckTimeout is AckConfirmed.
5. portal spawn s1 (N=1) records zero OpenWindow calls, no ack wait, self-attaches immediately.
6. On all-confirm, Ack.Clean(batch) is invoked before Connect(trigger).
7. spawnAckTimeout is a single named package constant (~8s) with an in-source justification comment; tunable.

STATUS: Complete

SPEC CONTEXT:
Spec "Burst & Partial-Failure Contract" requires: spawn N−1 external windows, then self-attach LAST gated on ALL N−1 confirming via an explicit token ack (each spawned `portal attach --spawn-ack` writes @portal-spawn-<batch>-<token> right before exec; picker watches for the marker with a timeout). "Timeout is per-window, not global" — each window's timer starts when its own spawn fires so cumulative sequential delay never eats a later window's budget; "Timeout value" is a named spawnAckTimeout ~8s/window with build-time justification (same spirit as the daemon self-supervision hysteresis constant). "Order is load-bearing" — cleanup (Clean batch markers) precedes the point-of-no-return self-exec. "N=0/N=1 boundary" — N=1 degenerates to a plain single attach, no special-casing, no ack wait. This task is the gate + timing + happy path + N=1; leave-what-opened reporting is 3.6 and permission burst-stop is 3.7.

IMPLEMENTATION:
- Status: Implemented (correct, aligns with spec)
- Location:
  - internal/spawn/command.go:27 composeAttachArgv — appends "--spawn-ack", FormatSpawnAckFlag(batch,token) as the final two discrete argv elements after [env -u TMUX -u TMUX_PANE PATH=… exePath attach session]; env-strip/PATH-only/own-binary prefix intact.
  - internal/spawn/burst.go:30 spawnAckTimeout = 8*time.Second, with a substantial in-source justification block (lines 10-29) mirroring the hysteresis-constant convention. defaultAckPoll = 75ms (line 35).
  - internal/spawn/burst.go:39-62 AckOutcome vocabulary (confirmed/timeout/failed) + WindowResult{Session,Token,Result,Ack}.
  - internal/spawn/burst.go:74-102 Burster struct + NewBurster defaults (NewID = session.NewNanoIDGenerator wrapped by NewSpawnID at each call, Timeout = spawnAckTimeout, Poll = 75ms, Now = time.Now, Sleep = time.Sleep).
  - internal/spawn/burst.go:133-182 Run — resolves exe ONCE up front (abort on error), generates batch + one token per window up front (abort on gen error before any window opens), then sequentially composes → OpenWindow → awaitToken (only if result.OK()), classifying !OK as AckFailed with no wait.
  - internal/spawn/burst.go:199-215 awaitToken — per-window timer starts at start := b.Now() right after this window's OpenWindow; polls Collect for the token, AckConfirmed on presence, AckTimeout on b.Timeout expiry.
  - cmd/spawn.go:145-147 N=1 immediate self-attach (return Connector.Connect(trigger)); :161 burster.Run; :169 Ack.Clean(batch) on every post-burst path; :213-217 all-confirmed → summary + Connect(trigger).
- Notes: The Phase-2 seam named AttachCommand(session, exe, getenv, batch, token) was refactored to composeAttachArgv(exePath, path, session, batch, token) with the executable/PATH resolution hoisted once into Run (documented at burst.go:110-114) rather than resolved per window. The produced argv is byte-identical to the plan's spec and this is an improvement (avoids N redundant os.Executable reads); acceptance #1's intent is fully met. Run's signature has since gained ctx (Task 6.8 cancellation), a progress callback (Phase 6 picker), and an OutcomePermissionRequired early-stop (Task 3.7) — all legitimate later-task evolutions of the shared file, none of which regress Task 3.5's behaviour.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/command_test.go:99 "it appends --spawn-ack <batch>:<token> as the final two argv elements" (AC1) + env-strip/PATH-only/own-binary/spaces cases (:15-97).
  - internal/spawn/burst_test.go:125 "resolves the executable once and composes an ack-flagged attach argv per session in list order" (AC2 confirm side); :170 "starts each window's ack timer at its own spawn (per-window, not one global clock)" (AC3) — a genuinely discriminating test: window 1 times out advancing the shared clock past Timeout, window 2 spawns at ~800ms and its token reveals ~200ms later, so only a per-window timer confirms it (a global-timer regression fails); :219 "confirms a token that arrives late but within the timeout" (AC4); :351/:379 abort-before-any-window on exe-resolve / id-gen failure; :248/:280 no-early-stop-on-failure; :313 permission early-stop (3.7). TestNewBurster_Defaults:487 pins default Timeout == spawnAckTimeout (AC7).
  - cmd/spawn_test.go:285 "self-attaches only after every external window's token is confirmed" (AC2: Connect==[s3] once, Clean==1); :353 + :377 two N=1 tests (AC5: zero OpenWindow, Clean==0, immediate Connect); :407 "cleans the batch markers before the self-attach exec handoff" (AC6) using cleanOrderConnector to snapshot Clean-count at Connect time and assert Clean preceded Connect; :488 summary carries batch attr + per-window ack=confirmed DEBUG.
- Notes: Test doubles are deterministic (manualClock advances only via Sleep; delayingAck reveals tokens on the fake clock; FakeAdapter/writingAdapter parse --spawn-ack back out of the real composed argv via the real ParseSpawnAckFlag, keeping the fake honest to the wire format). No real time/tmux/osascript. Focused, no redundant assertions; the two N=1 tests are complementary (one proves N=1 bypasses detect/resolve, the other proves no ack machinery runs). Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Small injectable seams (function fields + narrow AckCollector interface), constructor-with-defaults DI, package-bound logger in the cmd layer, closed ack/attr vocabulary — all consistent with the codebase and the Go design-patterns/testing skills. Deterministic fake-clock testing matches the daemon-hysteresis testing precedent the constant's comment cites.
- SOLID principles: Good. Burster has a single responsibility (the N−1 external half); the Nth self-attach is explicitly the caller's concern. awaitToken is extracted for deterministic unit testing.
- Complexity: Low. Run is a linear loop with a clear three-way classification; awaitToken is a small bounded poll loop.
- Modern idioms: Yes — slices.Clone in the fake, ctx.Err() checks, wrapped error propagation via NewSpawnID.
- Readability: Good. The spawnAckTimeout justification block and the per-window-timer rationale (awaitToken doc + the discriminating test's inline reasoning) make the load-bearing intent self-documenting.
- Security: No shell involvement — argv is discrete elements; a session name with spaces stays one element (tested). No injection surface.
- Performance: Acceptable. Collect enumerates all server options per 75ms poll per window, bounded by spawnAckTimeout; the confirmation path exits after a few polls and only a genuinely-failed window polls to the ~8s cap (a rare path). The spec explicitly chose sequential spawn + polling.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/burst_test.go — awaitToken (burst.go:205) deliberately treats a Collect error as "token not present yet" (documented at burst.go:189-193), so a persistently-failing enumeration classifies as AckTimeout rather than a false confirm. This safe-direction behaviour is untested: the delayingAck double never errors and FakeAckChannel.FailCollect is not exercised through the burster. Add a burst_test case with an always-erroring ack double asserting the window resolves to AckTimeout (not AckConfirmed, no panic), pinning the documented contract.
