---
status: complete
created: 2026-05-19
cycle: 2
phase: Input Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: saver-kill-respawn-loop-leaks-daemons - Input Review

## Findings

### 1. Defect 3 origin conclusion contradicts investigation's user-confirmation

**Source**: investigation.md "Initial Hypotheses" sub-question: *"User confirmed (2026-05-18): the disappearance was unprompted — no `portal clean`, no manual `rm`, nothing user-initiated touched the state dir. **The deleter is therefore somewhere in portal's own runtime path.** Candidates to investigate: an atomic-write race in `state.WriteVersionFile`, an over-eager cleanup pass in the daemon's tick loop, the bootstrap's CleanStale step (#10), or shutdown-flush behaviour in `defaultShutdownFlush`."*
**Category**: Enhancement to existing topic
**Affects**: Root Cause → Defect 3

**Details**:
The spec's current characterisation of Defect 3 says the disappearance "originates from outside portal's production code (manual `--purge`, dev-build escape, or external process)." This directly contradicts the investigation's user-confirmed conclusion, which is the opposite: the user denied any of the user-initiated paths (no `portal clean`, no manual `rm`, no state-dir touch), and the investigation explicitly concluded the deleter lives *inside* portal's runtime path — just not in any production code path that was code-traced. The candidates the investigation lists (atomic-write race in `WriteVersionFile`, over-eager cleanup in the daemon's tick loop, `CleanStale`, shutdown-flush) are all portal code paths, not external.

This matters because:
1. The spec's framing makes the open question seem less actionable than it is. "External process / dev-build / forgotten purge" suggests the next step is user-side hygiene; the investigation's conclusion correctly points to portal code as the suspect.
2. Cycle 1's finding #4 already preserved the candidate list under Change 3 ("atomic-write race in `WriteVersionFile`, over-eager cleanup in the daemon tick loop, `CleanStale`, shutdown-flush"). The Defect 3 root-cause paragraph still describes the origin as external — internally inconsistent with the candidate list that was carried forward.
3. The breadcrumb (Change 3) is more obviously load-bearing once the deleter is acknowledged to be inside portal's own code path.

**Current**:
> Code-trace exhaustively enumerated every production file-removal path; **no production code path removes `daemon.version` individually**. The disappearance therefore originates from outside portal's production code (manual `--purge`, dev-build escape, or external process). Fixing Defect 1 makes the disappearance non-load-bearing for the user-visible symptom — Defect 3 becomes a follow-up question, not a blocker.

**Proposed Addition**:
Reword Defect 3's origin sentence to match the investigation's conclusion: production code paths were ruled out by code trace, the user confirmed no user-initiated cleanup occurred, so the deleter is likely inside portal's own runtime path but in a code path not surfaced by the linear code trace (atomic-write race, over-eager cleanup pass, bootstrap CleanStale, or shutdown-flush behaviour — already enumerated under Change 3's carry-forward). Keep the closing point that fixing Defect 1 makes the disappearance non-load-bearing.

**Resolution**: Approved
**Notes**: Auto-applied. Defect 3 origin paragraph reworded to align with investigation's user-confirmed conclusion (suspect lives inside portal's runtime path, not external) and the candidate-list carry-forward under Change 3.

---
