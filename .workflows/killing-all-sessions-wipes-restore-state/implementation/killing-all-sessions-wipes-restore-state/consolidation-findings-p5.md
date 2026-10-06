# Consolidation Findings: Killing All Sessions Wipes Restore State (Phase 5)

## Comment Corrections

- internal/state/scrollback.go:314-315 — still puts the skip on the caller's dump as an obligation, but the phase moved it into the cycle's writer (`ScrollbackWriter.Write` refuses a skipped key), and `CommitCycle.Dump`'s doc was reworded to say so. The two docs now disagree about who enforces the rule.
  OLD: // CaptureCycle is what one capture cycle hands its caller: the index to commit
// and the sets of pane keys the caller's own scrollback dump must skip.
  NEW: // CaptureCycle is what one capture cycle hands its caller: the index to commit
// and the sets of pane keys the cycle's scrollback writer refuses.

- internal/state/scrollback.go:330-331 — same issue: it says a dump "must skip" paneKey, but the writer now refuses that key whatever the dump does. The daemon's dump checks the key only to save the `capture-pane` read.
  OLD: // SkipsScrollback reports whether a scrollback dump over this capture must skip
// paneKey: a key in Skeleton, Pending or Carried.
  NEW: // SkipsScrollback reports whether paneKey's scrollback is never written over
// this capture: a key in Skeleton, Pending or Carried.
