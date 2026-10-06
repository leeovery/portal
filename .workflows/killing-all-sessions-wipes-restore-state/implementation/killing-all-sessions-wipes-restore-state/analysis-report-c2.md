# Analysis Report: Killing All Sessions Wipes Restore State (Cycle 2)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 2

## Summary

The duplication and standards agents found nothing that clears the floor. The architecture agent reported two low-severity weak points in the shared commit cycle, and both are proposed as tasks.

- **Task 1.** Phase 3 Task 1 added a hold for answered lazy panes. That hold is decided from the caller's previous index, and the daemon's copy can predate a `commit-now`. When that happens, the pane's token-named transcript can still be deleted. §2.4 says the measure is the last committed record.
- **Task 2.** This is the part of the second finding that is a measured defect. The session listing can come back answered by tmux but fail to parse. That parse failure is labelled a back-off before the confirmation gets to classify it.

The rest of the second finding reverses settled directions without grounds, so it is discarded below.

## Discarded Findings

- **Part of "Listing failures are labelled 'tmux stopped answering' before the cycle's own classifier sees them".** This part recommended two things: that `captureStructure` return refused listing reads without the sentinel, and that the skeleton-marker read failure go through `classifyFailedCapture`. Both reverse settled directions:
  - phase 1 Task 1, which wraps the skeleton-marker read and the pane listing in the sentinel;
  - cycle 1 Task 2, under which a refused listing ends the cycle at that read and sends no confirmation after it.

  Nothing grounds the reversal. `ListSkeletonMarkers` and `ListAllPanesWithFormat` return only their command's error. Nothing shows either read failing while the server is healthy. The one measured mislabel, the session listing's parse failure, is kept as Task 2.
- **Both proposals are low-severity and were kept anyway.**
  - Task 1: the code misses §2.4's measure and breaks a guarantee the user chose at the phase 3 walk. The loss is irreversible and unseen until the next restore.
  - Task 2: the label contradicts phase 1 Task 1 and §4.2. A session name containing `|` also puts false back-off lines into the reboot evidence that §4.3 and the deferred hold (§5.3) rely on.
- **Not written as findings.** The duplication agent reported none. The standards agent weighed one divergence and placed it below the floor itself: an ordinary pane moved by a session rename has its empty capture judged at its new positional path.
