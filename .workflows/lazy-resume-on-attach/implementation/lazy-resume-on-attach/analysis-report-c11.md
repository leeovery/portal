# Analysis Report: lazy-resume-on-attach (Cycle 11)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 1

## Summary

The duplication and architecture passes found nothing that clears the floor. The standards pass found two drifts on the discard route, and both let a confirmed discard destroy a user-authored registration the user never agreed to on screen. The first is medium and was reproduced on the real chain: the confirmation's input drop empties one kernel queue's worth of input (about 2 KB), so the rest of a larger multi-line paste reaches the confirmation and its `y` confirms. The second is low: the discard deletes whatever `on-resume` entry the key holds, including a replacement the confirmation never showed.

The low finding was discarded in cycle 10 for its severity and because it did not cluster. It now clusters with the medium finding, since both break the same guarantee: a discard removes only a registration the user confirmed on screen. The two are staged as one medium proposal. Neither reverses a settled direction:

- The drain keeps phase 3's single drop run after the appearance query, and changes only what that drop does.
- The bound's refusal takes the draw's existing refused-drop path, which cycle 10 settled.
- No approved proposal settled which entry the discard targets.

No Decision is staged, and there are no spec defects or comment corrections.

## Discarded Findings

None.
