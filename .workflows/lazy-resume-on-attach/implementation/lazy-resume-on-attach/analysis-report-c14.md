# Analysis Report: Lazy Resume On Attach (Cycle 14)

## Stats

- Total findings: 1
- Deduplicated findings: 1
- Proposed tasks: 0

## Summary

The standards and architecture passes found nothing that clears the floor. The architecture pass raised one comment correction, to the `CaptureCycle.Pending` field doc. It is confirmed against the tree. `Pending` is `paneKeySet(live)`, and `captureStructure` adds to `live` only for sessions whose environment read succeeded (`internal/state/capture.go:111-128`, `:158`). A live waiting pane in a session the capture missed carries the marker but is not in the set; the re-file reaches it through `carriedWaiting` instead (`internal/state/scrollback.go:305-308`). `CaptureStructure`'s own doc (`internal/state/capture.go:56-58`) already states that limit, and the field doc does not. The duplication pass raised the same low-severity finding cycle 13 discarded: the EINTR-retrying select wait, written once in `cmd` and once in `internal/tui`. It still stands alone with nothing clustering with it, so it is discarded under the filter again. No proposal is staged, there is no Decision, and there are no spec defects.

## Comment Corrections

- internal/state/scrollback.go:261 — Pending is paneKeySet(live), and live is built only for sessions whose environment read succeeded (captureStructure's per-session loop), so a live waiting pane whose session the capture missed carries the marker but is not in this set; its carried record goes to Carried and the re-file reaches it through carriedWaiting, which this field's doc hides
  OLD: // Pending holds every live pane carrying the resume pending marker.
  NEW: // Pending holds every live pane carrying the resume pending marker in a
	// session the capture reached.

## Discarded Findings

- **The bounded select wait on the pane's stdin is written twice, once in cmd and once in internal/tui** (duplication, low). Discarded because it is low severity and does not cluster.
  - **It clears the floor, as it did in cycle 13.** The copies are `awaitTTYInput` (`cmd/tty_drain.go:72-89`), which backs both the drain and the waiter's quiet window through `awaitStdinInput` (`cmd/state_resume_wait.go:430-432`), and `selectBoundedReader.awaitReadable` (`internal/tui/pane_appearance.go:174-193`). Both use select over poll, retry on EINTR with the remaining window recomputed, and stop once the window is spent. No test file under `cmd` or `internal` names `EINTR`, `awaitTTYInput`, `awaitStdinInput`, `selectBoundedReader`, `awaitReadable` or `armStdinRead`. If an edit changed one copy's retry and missed the other, every test would still pass.
  - **Why it stays low.** The failure needs a future edit to one copy alone. The two copies behave the same today. This cycle's finding gives a more detailed SIGWINCH path, but every symptom it names comes from a regression that has not happened. The FD_SETSIZE difference is no current defect: `awaitStdinInput` skips the check, but the descriptor it passes is stdin, fd 0, which `FdSet.Set` indexes safely. The two `makeStdinRaw` copies and the two `term.IsTerminal` checks encode no rule of their own; the finding itself lists them only because they would move with the wait. So they add no cluster.
  - **Why it stands alone.** This cycle has no other finding. The only other item is the `Pending` doc correction, which concerns the capture's pane sets and has nothing in common with the tty wait.
  - **Not a reversal.** No approved proposal settled where the wait lives. Consolidation pass 17 and review cycle 3 touch the drain and the echo hold, not the wait's placement. The finding is dropped for severity and for lacking a cluster, not on its merits. If a later finding clusters with it, the leaf-package consolidation it recommends comes back as a proposal of its own.
