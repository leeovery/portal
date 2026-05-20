TASK: saver-kill-respawn-loop-leaks-daemons-1-3 — Gate kill decision on BootstrapAliveCheck in EnsurePortalSaverVersion before mismatch predicate.

STATUS: Complete

SPEC CONTEXT: Spec §Change 1 inverts the prior contract — the alive-check is now the authoritative gate. Matrix: not-alive → no kill (any version state); alive + dev/empty (either side) → kill; alive + absent → no kill (Task 1-4 layers defensive write); alive + non-absent read-error → kill (conservative); alive + match → no kill; alive + mismatch → kill.

IMPLEMENTATION:
- Status: Implemented
- internal/tmux/portal_saver.go:320-340 — EnsurePortalSaverVersion body.
- internal/tmux/portal_saver.go:358-382 — shouldKillSaverOnVersionDecision (alive-branch sub-matrix).

Key points verified:
- BootstrapAliveCheck is read once at line 322 before any decision branch.
- portalSaverReadVersionFile is read once up front (line 321) and the result reused — satisfies the spec's "read once" requirement.
- Kill branch is gated by `alive && shouldKillSaverOnVersionDecision(...)` (line 325). Implements alive-first invariant.
- The alive+absent branch (line 327-338) is clearly comment-marked and already has the Task 1-4 defensive write co-located.
- Not-alive path falls through directly to BootstrapPortalSaver with no kill.
- shouldKillSaverOnVersionDecision: dev short-circuit evaluated first; correctly gates `stored == ""` on `readErr == nil` so an unreadable file isn't misclassified as "stored is empty"; uses errors.Is(readErr, state.ErrVersionFileAbsent) per the edge-case requirement.
- Function signature unchanged.

TESTS:
- Status: Adequate
- All nine required tests present in internal/tmux/portal_saver_test.go:
  - TestEnsurePortalSaverVersion_NotAlive_AbsentVersion_DoesNotKill (line 1656)
  - TestEnsurePortalSaverVersion_NotAlive_VersionMismatch_DoesNotKill (line 1682)
  - TestEnsurePortalSaverVersion_Alive_StoredDev_Kills (line 1708)
  - TestEnsurePortalSaverVersion_Alive_CurrentDev_Kills (line 1728)
  - TestEnsurePortalSaverVersion_Alive_AbsentVersionNeitherDev_DoesNotKill (line 1748)
  - TestEnsurePortalSaverVersion_Alive_NonAbsentReadError_Kills (line 1773)
  - TestEnsurePortalSaverVersion_Alive_VersionsMatch_DoesNotKill (line 1796)
  - TestEnsurePortalSaverVersion_Alive_VersionsMismatch_Kills (line 1816)
  - TestEnsurePortalSaverVersion_ConsultsAliveCheckBeforeVersionMismatchDecision (line 1842) — pins ordering invariant via twin fixture run.
- Tests follow CLAUDE.md (no t.Parallel(), t.Cleanup() for seam restoration).

CODE QUALITY:
- Project conventions: followed — seam pattern matches established BootstrapAliveCheck idiom.
- SOLID: shouldKillSaverOnVersionDecision is single-responsibility; alive-check gating sits in the caller where it belongs.
- Complexity: low. Five-step linear evaluation maps 1:1 to spec matrix row order.
- Modern idioms: errors.Is (not string-matching) for the absent check.
- Readability: function-level comment at lines 282-319 documents the full matrix as a table; numbered comments in helper match spec ordering.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The plan and Task 1-1 instruct that portalSaverVersionMismatch be preserved ("Do not delete it"). The implementation renamed it to shouldKillSaverOnVersionDecision — the original name no longer appears in portal_saver.go. Spec-faithful, but a literal drift from the task wording.
- [idea] EnsurePortalSaverVersion uses switch { case alive && X: ... case alive && Y: ... } with two cases and an implicit fall-through to BootstrapPortalSaver. An if/else if chain would read more idiomatically since there's no scrutinee. Cosmetic only.
