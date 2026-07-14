TASK: restore-host-terminal-windows-3-2 — `@portal-spawn` ack channel seam: write / collect / clean over tmux server options

ACCEPTANCE CRITERIA:
- Collect(batch) returns ONLY the given batch's tokens — foreign-batch @portal-spawn- markers AND all @portal-skeleton- markers excluded (both directions verified in one crafted-string test).
- state.ListSkeletonMarkers over the same crafted dump returns only skeleton paneKeys and no @portal-spawn- names (skeleton enumerator blind to the spawn prefix).
- Write(batch, token) sets @portal-spawn-<batch>-<token> to "1".
- Clean(batch) unsets every one of that batch's markers and leaves foreign-batch + skeleton markers intact; a Clean on a zero-marker batch returns nil (idempotent).
- A ShowAllServerOptions failure makes Collect return (nil, err) — never a false-empty success.
- Integration test round-trips a real marker: set → Collect sees it → Clean removes it → second Clean is a nil no-op, co-resident @portal-skeleton- untouched.

STATUS: Complete

SPEC CONTEXT: Spec (Burst & Partial-Failure Contract → Ack channel / Cleanup; Observability & State Footprint) mandates a small write-token/collect-tokens/clean seam over transient @portal-spawn-* tmux server options, self-cleaned per the ack contract, with a distinct prefix from @portal-skeleton- so the only all-server-options enumerator (state.ListSkeletonMarkers) is mutually blind — namespacing isolates sweeps in both directions and options die with the server. Collect must never false-empty on enumeration failure (that would mis-classify every window as failed). This task builds write/collect/clean only; daemon-readability is a deferred follow-on.

IMPLEMENTATION:
- Status: Implemented — faithful to plan, no drift.
- Location: internal/spawn/ack.go:1-159 (ServerOptionAckChannel + seams + consumer interfaces), internal/spawntest/ack.go:1-79 (FakeAckChannel).
- Notes:
  - Spawn-local seams serverOptionWriter / serverOptionLister (ack.go:10-21) mirror the Phase-1 clientLister idiom; *tmux.Client satisfies both implicitly (SetServerOption/UnsetServerOption at internal/tmux/tmux.go:435,939; ShowAllServerOptions at :531). internal/spawn does not import internal/state in production — seam rationale honoured.
  - Consumer interfaces AckCollector / AckCleaner / AckWriter / AckChannelFull exported (ack.go:23-50); compile-time guards (ack.go:70-75) prove *ServerOptionAckChannel satisfies all four; spawntest guards (ack.go:76-79) prove *FakeAckChannel satisfies AckChannelFull + AckWriter. Matches the 3-5 / 6-3 dependency contract.
  - Write sets "1" (ack.go:79-81); value opaque/presence-is-signal per spec. Collect returns (nil, err) on lister failure and a non-nil map otherwise (ack.go:92-102). Clean returns first non-nil unset error while continuing the sweep (ack.go:110-122).
  - forEachBatchMarker (ack.go:127-135) is a single parse chokepoint shared by Collect and Clean, filtering via ParseSpawnMarkerName (ackid.go:62-72) — the identical derive rule the plan requires. optionNames (ack.go:141-158) mirrors state.ListSkeletonMarkers' `@name value` split shape.
  - Prefix isolation is real: SpawnMarkerPrefix "@portal-spawn-" (ackid.go:14) vs SkeletonMarkerPrefix "@portal-skeleton-" (state/markers.go:14); each enumerator's prefix check rejects the other's names.

TESTS:
- Status: Adequate.
- Coverage: All six named tests present, one legitimate extra:
  - Collect isolation both directions + non-nil empty set for an absent batch (ack_test.go:74-98).
  - ListSkeletonMarkers blind to spawn prefix over the SAME crafted dump, with a defensive leak-check for spawn-derived keys (ack_test.go:100-115).
  - Write sets marker to "1" (ack_test.go:117-129).
  - Clean unsets exactly the two b1 names, touches neither b2 nor skeleton; zero-marker batch is a no-op (ack_test.go:131-154).
  - Collect (nil, err) on enumeration failure — false-empty guard (ack_test.go:180-192).
  - Extra: Clean continues past a per-marker unset error and returns the FIRST (ack_test.go:156-178) — covers the plan's "collect per-marker errors but continue, return first" behaviour; not redundant.
  - Integration: real-tmux round-trip on a per-test -L socket, no daemon/binary; set → Collect{t1} → ListSkeletonMarkers{foo} → Clean → empty → second Clean nil → skeleton untouched (ack_realtmux_test.go:44-111). `//go:build integration` matches both the plan and the golang-testing skill's build-tag rule.
- Notes: The single crafted optionDump (ack_test.go:18-25) mixes two b1 markers, one b2, two skeleton markers, and two ordinary noise options plus a blank line — exercises prefix rejection, delimiter parsing, and noise skipping in one fixture. Tests read behaviour (recorded set/unset calls, returned token sets), not implementation internals. No over-testing, no t.Parallel (correct per CLAUDE.md). Would fail if the feature broke (wrong batch collected, skeleton leak, false-empty on error, non-idempotent clean all caught).

CODE QUALITY:
- Project conventions: Followed. Spawn-local seams + exported consumer interfaces match the documented cross-package pattern; test-only spawntest package not imported by production; integration lane / build tag correct.
- SOLID principles: Good. Clear single-responsibility methods; consumer interfaces are narrow (1 method each) and segregated; AckChannelFull composes the two the orchestrators actually need.
- Complexity: Low. Straight-line Collect/Clean over one shared parse chokepoint.
- Modern idioms: Yes — strings.SplitSeq, strings.CutPrefix/Cut, maps.Copy.
- Readability: Good. Comments explain the false-empty guard, idempotency, and presence-is-signal rationale.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/ack_realtmux_test.go:36 — the `sortedSet` diagnostic helper does not sort (unlike ack_test.go's `sortedKeys`); rename to `setKeys` or add `sort.Strings(keys)` so failure output is deterministic and the name is honest. Test-diagnostic only, zero logic impact.
- [idea] internal/spawn/ack.go:141 — `optionNames` re-implements the `@name value` line-split shape already in state.ListSkeletonMarkers (state/markers.go:85-95). Consider homing a shared splitter in the leaf internal/tmuxout (already imported by internal/state) so the two enumerators cannot drift. Deliberately duplicated by the plan for import-cycle avoidance and the two have a minor semantic difference (Collect ignores the value; ListSkeletonMarkers requires non-empty), so this is optional and needs a design decision on where to home it.
