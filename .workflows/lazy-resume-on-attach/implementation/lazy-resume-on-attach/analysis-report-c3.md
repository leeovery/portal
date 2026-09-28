# Analysis Report: lazy-resume-on-attach (Cycle 3)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 0

## Summary
Duplication found nothing that clears the floor. Standards found the implementation conforms to the specification and its corrigenda, with one low-severity divergence (a resize landing inside a screen hand-off is never redrawn) and two comment corrections. Architecture raised three findings, one medium and two low. None of the four becomes a proposal: the three low ones stand alone with no shared pattern, and the medium one rests on an omission that the existing lazy suite already catches. The two comment corrections are collected for direct application.

## Comment Corrections

- cmd/state_resume_recover.go:25-28 — the waiter was never the pane's process. The parked shell is the pane's process and the waiter is its child, which is exactly the premise the spec's 2026-09-22 and 2026-09-28 corrigenda corrected.
  OLD: // runResumeRecover is the tail of the chain a waiting pane parks: it runs when
// the waiter above it has stopped being the pane's process, and gives the pane a
// shell so tmux does not close it — and with it the session and the whole
// transcript of a pane that is the only one in it.
  NEW: // runResumeRecover is the tail of the chain a waiting pane parks: it runs once
// the waiter above it has ended, and gives the pane a shell so tmux does not
// close it — and with it the session and the whole transcript of a pane that is
// the only one in it.
- cmd/state_resume_chain.go:112-113 — the hand-off replaces the draw's or the waiter's own image, not the pane's process. The parked shell stays the pane's process through every hand-off.
  OLD: // resumeHandOff replaces the pane's process image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the
  NEW: // resumeHandOff replaces this process's image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the

## Discarded Findings
- A resize that lands during a screen hand-off is lost, and the panel stays drawn at the old size (standards, low) — discarded: low severity, and it does not cluster with anything. The divergence is real. The waiter compares sizes only in its settle arm, and only a SIGWINCH reaches that arm. So a change that lands between the draw's size read (cmd/state_resume_draw.go:43) and the waiter arming its notify (`winchSignals`, called from the RunE at cmd/state_resume_wait.go:479) leaves no trace. The window is only a draw's appearance probe plus two process starts long, though, and it only matters when it holds the last size change of a stream. Enter and `d` still act on the misplaced card, and the next resize redraws it at the right size.
- The hydrate helper parks a pane on the resolved intent and leaves each tail to confirm the marker landed (architecture, medium) — discarded: the failure it names is not silent. `TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker` (cmd/state_hydrate_lazy_test.go:184) runs all three tails through `lazyTails()` (:37-65), which wires in the production `handleHydrateTimeout` and `handleHydrateFileMissing`. It fails the test whenever a tail reaches the exec without writing the pending marker. A tail that went back to unsetting the skeleton marker directly would turn it red, whether or not the executable had been moved up to the top-of-run resolution. The nil-decision branch has no production caller: `runHydrate` sets the decision (cmd/state_hydrate.go:112) before each of the four exec sites (:121, :151, :166, :181). Both outcomes therefore need a future edit, and the existing suite already catches the first. Cycles 1 and 2 discarded the same substance at low. The move to medium rests on the "every test still passes" claim, and the per-tail lazy suite disproves it.
- The pane draw logs `theme: loaded` on every keypress and redraw (architecture, low) — discarded: low severity, and it does not cluster with anything. The cost is INFO-level log volume, not wrong behaviour. Each line records a draw that really did paint in that theme, and §5.2 has a pane's theme "resolve as it does everywhere else in Portal". Nothing reads the component programmatically, so the extra lines cannot change what any pane or reader does.
- CaptureAndRefile builds in the re-file but leaves skipping the frozen panes to the caller (architecture, low) — discarded: low severity, and it does not cluster with anything. The daemon is the only production scrollback writer, and it applies both halves through `paneSkipsScrollback` (cmd/state_daemon.go:322-328). Commit-now dumps nothing. The `cmd/bootstrap` tick copy (cmd/bootstrap/daemon_tick_test_helpers_test.go:45) is a test fixture, and it has no failure today because its suites pin eager registrations. The failure needs a future dumping caller. The settled phase-4 direction (task 4) deliberately left the dump and its skip with the caller, as "the part that genuinely differs". Cycle 1 discarded the same finding.
