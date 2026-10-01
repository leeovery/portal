# Review Report: Lazy Resume On Attach (Cycle 3)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 1

## Summary

Prep routed four findings. Two were do-now corrections, already applied and committed in `3eba79607`: A1, a subtest renamed to the contract it asserts, and A2, a carried waiting record re-filed onto its token transcript. One, A4, predates this feature and is held out of scope. The single replan action, A3 (source 17-2-1), becomes the one proposal. While a waiting pane is handed from its draw to its waiter, the tty is still echoing, so on the discard drop route a stream of input scrolls the panel away and overprints it. The direction is settled with no decision staged: the draw turns echo off at the source, and the two answer paths that hand the pane to a hook or shell turn it back on.

## Spec Defects

None. The specification makes no claim about terminal echo. Its input rules in §4.3 are honoured by the code: nothing the user did not send answers the panel, and input in flight when the confirmation opens is dropped. The defect is in the code, which leaves a cooked window between the paint and the waiter's raw mode.

## Discarded Findings

None. A1 and A2 were applied as do-now corrections and A4 is out of scope; none of them becomes a task.
