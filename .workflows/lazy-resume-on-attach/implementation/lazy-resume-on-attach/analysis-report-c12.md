# Analysis Report: Lazy Resume On Attach (Cycle 12)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 2

## Summary

The duplication pass found nothing that clears the floor. The standards and architecture passes each raised one low-severity finding, and both were confirmed against the tree. The first is a capture that skips a waiting pane's session, for example because a rename landed mid-capture: its housekeeping pass then deletes the pane's token-named transcript. The second is a failed restore token re-stamp: the stale-hook sweep then deletes the registration of a pane that is still waiting on it.

Both findings follow one pattern. A waiting pane's protection rests on a step that is allowed to miss once, and the indefinite wait turns a miss that used to heal itself into permanent loss of the transcript or registration the wait exists to keep. Earlier passes closed other instances of the same pattern (analysis cycles 4 and 6, consolidation pass 10, ad-hoc task 1). Because the two findings cluster on that pattern, both are staged rather than filtered as isolated lows.

Each proposal carries a settled direction. The transcript fix carries the previous record forward rather than having the housekeeping pass spare token-named files, which cycle 6 rejected. No Decision is staged, and there are no spec defects or comment corrections.

## Discarded Findings

- None.
