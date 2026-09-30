# Review Report: Lazy Resume On Attach (Cycle 2)

## Stats

- Total findings: 7
- Deduplicated findings: 5
- Proposed tasks: 1

## Summary
Prep produced five actions from seven findings. A1–A4 were routed do-now and applied in this session (`d0cad6a07`), so none of them becomes a task. The one `replan` action, A5, becomes one proposal. The resume panel assumes the pane has an alternate screen: under `alternate-screen off` the card is painted over the replayed transcript and later saved into its history. The same proposal carries the coupled refusal-table gap, where a mark-before-resolve reorder ships green. The fix had two forms: pin the option on the pane, or fall back to eager. I settled that choice on the specification instead of staging it. The specification promises that the panel never enters the history, and it ships lazy on so that an upgraded install meets panels. Its eager fallback is for a pane that cannot be protected, and a pane Portal can pin can be protected. Pinning is therefore the direction, and eager remains the fallback for a pane tmux will not pin.

## Spec Defects

None. The specification's claim that the panel is painted into the pane's alternate screen and never enters the scrollback is sound. It says nothing about tmux's `alternate-screen` option, but that is not a stale claim. The measurement shows the code failing to deliver the claim on a pane with the option off, and it also shows that pinning the option pane-scoped over a global `off` delivers it. The code is what is wrong.

## Discarded Findings
- None. Prep discarded nothing. A1 (`awaitWindow` doc), A2 (`armStdinRead` kqueue comment), A3 (the dump-forces-write subtest) and A4 (the back-to-back commit-now wait) were routed do-now and applied in this session, so none is proposed as a task.
