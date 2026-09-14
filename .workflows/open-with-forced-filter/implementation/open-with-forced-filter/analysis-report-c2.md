# Analysis Report: Open With Forced Filter (Cycle 2)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 1

## Summary

The standards agent found nothing: every decision point in the specification maps to an implementation site that matches it. The duplication and architecture agents landed on two halves of one pattern — "which session the search excludes" is read in three places (`openTUI`, `searchCandidates`, and, by omission, `completeSearchTerm`), so the sigil's tab completion offers the one session the sigil's own count deliberately drops, and the count and the picker's list derive their exclusion from two independently-authored reads. The two findings are grouped into a single proposal, since the architecture agent's recommendation ("derive the offered set from the counted set") is the duplication agent's helper. Three comments in the change set's own files make claims the change set falsified.

## Comment Corrections

- cmd/open.go:734 — claims the model emits staged warnings only after the loading page dismisses, which this change set contradicted: a search attach and a command-pending picker have no loading page and get them at teardown, as `stageBootstrapWarningsOnModel`'s own (corrected) doc now says
  OLD: // Staged rather than written: the model emits them only after the loading page
       // is dismissed, because a direct write during loading corrupts the rendered UI.
  NEW: // Staged rather than written: a direct write while the TUI holds the screen
       // corrupts the rendered frame.

- cmd/root.go:141 — the widened `isTUIPath` in this same file now admits members with no loading page, so "until the loading page dismisses" names a gate those invocations never reach
  OLD: // the TUI path leaves them in the sink until the loading page dismisses.
  NEW: // the TUI path leaves them in the sink for the model to carry, so nothing is
       // written into a frame the picker is about to claim.

- internal/tui/session_dir_column.go:20 — claims purity and then names the environment
  read that contradicts it, which invites treating the helper as safe to render or
  assert against without pinning $HOME (the capture suite has to pin it)
  OLD: // empty directory or a non-positive width. Pure: the only environment it reads
       // is the one AbbreviateHome consults.
  NEW: // empty directory or a non-positive width. Not environment-independent: the
       // abbreviation resolves the home directory, so one value renders differently
       // under a different $HOME.

## Discarded Findings

None — both findings name a failure, a user and a way it is noticed, and both were grouped into Task 1 rather than dropped.

## Notes on the settled directions

Cycle 1's approved Task 1 ("Single-source the session set the sigil counts and the picker lists") scoped itself with the clause "`searchCandidates` keeps its own `tmux.InsideTmux()` / `CurrentSessionName()` read and its error policy, passing the resolved name in, so only the set rule becomes single-sourced and the seam stays where it is." Task 1 below extends that rather than reversing it: `tui.PickerSessions` stays the single declaration of the set rule and the `SearchSessionSource` seam is untouched — only the read that produces its argument is collapsed, and a third surface (tab completion) that was never routed through the rule at all is brought onto it.
