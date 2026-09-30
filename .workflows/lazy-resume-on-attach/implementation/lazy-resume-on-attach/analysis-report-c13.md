# Analysis Report: Lazy Resume On Attach (Cycle 13)

## Stats

- Total findings: 1
- Deduplicated findings: 1
- Proposed tasks: 0

## Summary

The standards and architecture passes found nothing that clears the floor. The architecture pass raised two comment corrections to the capture's live-token claims. Both are confirmed against `internal/state/capture.go`: `liveTokenSet` is built from the fresh index, which holds only sessions the capture reached, so a live pane in a missed session carries a token the set does not contain. The duplication pass raised one low-severity finding, the EINTR-retrying select wait written once in `cmd` and once in `internal/tui`. It clears the floor, but it stands alone and nothing clusters with it, so it is discarded under the filter. No proposal is staged, no Decision, and there are no spec defects.

## Comment Corrections

- internal/state/capture.go:342-343 — since the missed-session carry, the capture distinguishes panes the enumeration lists from panes in sessions it reached, and this set holds only the second; a live pane in a missed session carries a token it does not contain
  OLD: // liveTokenSet holds the token of every record in fresh: before any merge
// replaces a pane with a previous record, that is every live pane's token.
  NEW: // liveTokenSet holds the token of every record in fresh: before any merge
// replaces a pane with a previous record, that is the token of every live pane
// in a session the capture reached.
- internal/state/capture.go:39-42 — the address-match exclusion is built from liveTokenSet, so it covers only tokens carried by panes in reached sessions; a record whose token a live pane in a missed session carries can still be taken by address
  OLD: // address has become. A skipped pane carrying no token keeps the whole previous
// record at its own address, a pending one those three fields of it, unless a
// live pane carries that record's token. A stale marker never resurrects a
// killed pane.
  NEW: // address has become. A skipped pane carrying no token keeps the whole previous
// record at its own address, a pending one those three fields of it, unless a
// live pane in a session the capture reached carries that record's token. A
// stale marker never resurrects a killed pane.

## Discarded Findings

- **The bounded select wait on the pane's tty is written twice, once per package** (duplication, low). Discarded because it is low severity and does not cluster.
  - **It clears the floor.** The two copies are `awaitTTYInput` (`cmd/tty_drain.go:72-89`) and `selectBoundedReader.awaitReadable` (`internal/tui/pane_appearance.go:174-192`). They agree today on select over poll, the EINTR retry with the window recomputed, and ending when the window is spent. No test in either package drives the EINTR path: no test file names `EINTR`, `awaitTTYInput`, `selectBoundedReader` or `armStdinRead`. A fix to one copy's retry that missed the other would therefore pass every test, and its symptom would be a rare panel that ends on a resize with nothing pointing to the cause. That corrects the ground on which consolidation pass 17 and cycle 12 set the pair below the floor, which was that drift would show.
  - **Why it stays low.** The failure needs a future edit to one copy alone. The copies do not diverge in behaviour now. The FD_SETSIZE sub-claim is no current defect: the waiter's `awaitStdinInput` (`cmd/state_resume_wait.go:424-426`) skips the check, but the descriptor it passes is stdin, fd 0, which `FdSet.Set` indexes safely.
  - **Why it stands alone.** This cycle has no other finding. The comment corrections concern the capture's token set and share nothing with the tty wait.
  - **Not a reversal.** No approved proposal settled where the wait lives. It was dropped for severity and for lacking a cluster, not on its merit, so the leaf-package consolidation it recommends would come back as its own proposal if a later finding clusters with it.
