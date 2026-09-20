# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. Nothing proves the discarded pane shows the user's transcript again

**Type**: Incomplete coverage
**Spec Reference**: §6.2 ("The pending marker is cleared and the pane falls through to a plain shell with its replayed scrollback still above it — indistinguishable from a pane that never had a hook"), §5.1 (the panel is painted into the alternate screen so it "never enters the scrollback" and "once it is gone it should leave no trace in the history"), §7.2 (both answers leave the panel's screen before the marker clears)
**Plan Reference**: Phase 5, task `lazy-resume-on-attach-5-6` (A real pane discards its resume)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
After a confirmed discard the pane is supposed to look exactly like a pane that never had a hook — the user's pre-reboot transcript back on screen, no card, no confirmation, nothing left behind. The only test that drives a discard in a real pane never looks at the pane afterwards. It checks the store, the token, the session, the marker and the next reboot, then sends `exit`. A build in which the confirmation stays painted over the transcript after `y` — an answer that removed the registration but never left the alternate screen — passes every criterion in that task, and the user would meet a discarded pane still showing a dead card with their work hidden under it. The resume answer has this covered (task 4.7 asserts Enter reveals the transcript); the discard answer, which is the one that destroys something, does not.

**Proposal**:
Task 5.6 already reads the pane through `capture-pane` at every other step — before the discard, on the confirmation, and after the second reboot. Add the same read immediately after `y`, where the spec states the strongest claim about what the pane shows: the replayed transcript above a plain shell, indistinguishable from a pane that never had a hook. The assertion is one `capture-pane -p` on the subject carrying the pre-reboot line and none of either screen's fixed copy, placed in the subtest that already interrogates the post-discard pane, with a matching acceptance criterion, test name and edge case. The planning file's task-table edge-case list carries the new edge case alongside, as every other entry in that row does.

**Current**:

In `.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/phase-5-tasks.md`, task 5.6's fourth **Do** bullet, sub-item (e):

> (e) the subject's `@portal-pane-id` still reads back through `tmux.ReadPaneOption`, its session is still live, a fresh `state.CaptureStructure` enumerates it with an empty pending set, and a scrollback write for that pane now lands;

Task 5.6's **Acceptance Criteria**, third and fourth entries:

```
- [ ] After `y`, the subject's key is absent from `hooks.json` and the bystander's entry is byte-identical to the bytes that were seeded, object form and `resume` attribute included.
- [ ] The subject's `@portal-pane-id` still reads back non-empty after the discard, and its session is still live and still enumerated by `state.CaptureStructure`.
```

Task 5.6's **Tests**, third and fourth entries:

```
- `"it removes only the subject's registration"`
- `"it leaves the pane's durable token stamped and its session live"`
```

Task 5.6's **Edge Cases**, third and fourth entries:

```
- After `y` the key is absent from `hooks.json` and every other entry is byte-unchanged: the store rewrites the whole file on every mutation, so "it removed one entry" is only true if the neighbours survive — including an object-form entry the reader models and could re-marshal into a different shape.
- The pane's durable token is still stamped and its session is still live and still enumerated into a fresh capture: discarding removes a registration and never touches the pane's identity or its session.
```

In `.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/planning.md`, the Phase 5 task-table row for `lazy-resume-on-attach-5-6`:

```
| lazy-resume-on-attach-5-6 | A real pane discards its resume | the confirmation is read out of the real pane through `capture-pane` with the pre-reboot transcript intact underneath it, Escape returns the waiting panel in the same run before the discard is driven, after `y` the key is absent from `hooks.json` and every other entry is byte-unchanged, the pane's durable token is still stamped and its session is still live and still enumerated into a fresh capture, the pending marker is cleared and the pane's saved scrollback resumes being written, the pane closes on the first `exit`, a second reboot of the same fixture restores that pane with no panel and no marker as an ordinary hookless pane, integration lane with isolated state a disposable socket and no daemon, the suite enumerates and signals no process it did not cause to exist |
```

**Proposed Text**:

In `phase-5-tasks.md`, task 5.6's fourth **Do** bullet, sub-item (e) becomes:

> (e) the subject's `capture-pane -p` returns the pre-reboot line with none of either screen's fixed copy on it — no `Resume session` title, no `ON RESUME` label, no `⏎ resume` / `d discard` hints and no `▲ Discard resume?` — so leaving the panel's screen revealed the transcript that was underneath it; its `@portal-pane-id` still reads back through `tmux.ReadPaneOption`, its session is still live, a fresh `state.CaptureStructure` enumerates it with an empty pending set, and a scrollback write for that pane now lands;

Task 5.6's **Acceptance Criteria** — one criterion inserted between the third and fourth entries:

```
- [ ] After `y`, the subject's key is absent from `hooks.json` and the bystander's entry is byte-identical to the bytes that were seeded, object form and `resume` attribute included.
- [ ] After `y` the subject's `capture-pane -p` returns the pre-reboot line with none of either screen's fixed copy on it — the pane falls through to a plain shell with its replayed transcript above it, indistinguishable from a pane that never had a hook, and neither the card nor the confirmation is left painted over it.
- [ ] The subject's `@portal-pane-id` still reads back non-empty after the discard, and its session is still live and still enumerated by `state.CaptureStructure`.
```

Task 5.6's **Tests** — one test inserted between the third and fourth entries:

```
- `"it removes only the subject's registration"`
- `"it reveals the transcript that was underneath the confirmation"`
- `"it leaves the pane's durable token stamped and its session live"`
```

Task 5.6's **Edge Cases** — one edge case inserted between the third and fourth entries:

```
- After `y` the key is absent from `hooks.json` and every other entry is byte-unchanged: the store rewrites the whole file on every mutation, so "it removed one entry" is only true if the neighbours survive — including an object-form entry the reader models and could re-marshal into a different shape.
- The pane shows its own transcript again the moment the discard lands, with no trace of either screen on it. The alternate screen is the whole mechanism by which the panel hides a transcript it never touches, so a discard that removed the registration without leaving that screen would leave the user's work hidden under a dead card on a pane that is otherwise an ordinary shell — and nothing else in the suite would notice. The resume answer's own end state is asserted the same way in task 4.7.
- The pane's durable token is still stamped and its session is still live and still enumerated into a fresh capture: discarding removes a registration and never touches the pane's identity or its session.
```

In `planning.md`, the Phase 5 task-table row for `lazy-resume-on-attach-5-6` becomes:

```
| lazy-resume-on-attach-5-6 | A real pane discards its resume | the confirmation is read out of the real pane through `capture-pane` with the pre-reboot transcript intact underneath it, Escape returns the waiting panel in the same run before the discard is driven, after `y` the key is absent from `hooks.json` and every other entry is byte-unchanged, the pane shows its own transcript again with no trace of either screen on it, the pane's durable token is still stamped and its session is still live and still enumerated into a fresh capture, the pending marker is cleared and the pane's saved scrollback resumes being written, the pane closes on the first `exit`, a second reboot of the same fixture restores that pane with no panel and no marker as an ordinary hookless pane, integration lane with isolated state a disposable socket and no daemon, the suite enumerates and signals no process it did not cause to exist |
```

**Resolution**: Pending
**Notes**:

---

### 2. The indicator legend's tool-agnostic rule is described but never checked

**Type**: Incomplete coverage
**Spec Reference**: §5.2 ("Every string this surface renders is tool-agnostic. Portal's resume machinery runs whatever command a registration holds, so nothing the panel shows names a particular tool … That covers the discard confirmation's consequence line (§5.4) and the indicator legend that ships with the picker's pending dot (§8.3) as much as the panel itself"), §8.3 (the help modal gains the legend in the same change)
**Plan Reference**: Phase 6, task `lazy-resume-on-attach-6-7` (The help modal's indicator legend)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The legend is new user-facing copy and its wording is left to the executor, so the one rule that constrains it — it names no particular tool — is the only thing standing between "Pending resume" and "Claude session waiting". That rule appears in the task's prose and in the phase's acceptance, but not in a single criterion or test the executor has to satisfy or a reviewer polices, and the task's own edge-case list has dropped it even though the plan's task table still carries it. Every other surface this feature builds pins the rule as a criterion — the waiting panel, the discard confirmation and the capture fixtures all do — so the legend is the one place a tool name could land and be signed off. Portal's resume machinery is generic, and a tool name in the picker's own help is the kind of copy that outlives the tool.

**Proposal**:
Restore the edge case the plan's task table already states for this task, and pin it the way the sibling surfaces pin it: an acceptance criterion and a test. Task 3.3 states the criterion as "the screen names no tool and carries no copy beyond what this task declares" and task 6.8 as "No fixture string names a tool"; the legend takes the same form against the labels this task declares.

**Current**:

In `.workflows/lazy-resume-on-attach/planning/lazy-resume-on-attach/phase-6-tasks.md`, task 6.7's **Acceptance Criteria**, first and second entries:

```
- [ ] The Sessions help modal renders a legend row for each indicator, below the key rows, separated by the panel's own divider.
- [ ] The Projects help body and the preview help body are byte-identical to their pre-change renders, in every built-in theme and in colourless mode.
```

Task 6.7's **Tests**, first and second entries:

```
- `"it renders a legend row for each indicator on the Sessions help"`
- `"it leaves the Projects help body byte-identical"`
```

Task 6.7's **Edge Cases**, second and third entries:

```
- It renders on the Sessions help alone and the Projects help body is byte-unchanged — the indicators exist only on the sessions row, and a legend on the projects page would explain something that page does not draw. The byte-identity is asserted rather than eyeballed, because the legend parameter passes through the shared renderer.
- Under `NO_COLOR` it names the letters rather than two dots the mode cannot tell apart — which is the whole reason the row renders letters there, and the legend inherits it by sharing the renderer rather than by a second rule.
```

**Proposed Text**:

Task 6.7's **Acceptance Criteria** — one criterion inserted between the first and second entries:

```
- [ ] The Sessions help modal renders a legend row for each indicator, below the key rows, separated by the panel's own divider.
- [ ] Each legend label says what its indicator means and names no particular tool: the rendered legend rows carry only the indicator forms the row's own renderer produces and the labels this task declares, and no other alphabetic text.
- [ ] The Projects help body and the preview help body are byte-identical to their pre-change renders, in every built-in theme and in colourless mode.
```

Task 6.7's **Tests** — one test inserted between the first and second entries:

```
- `"it renders a legend row for each indicator on the Sessions help"`
- `"it names no tool in either legend label"`
- `"it leaves the Projects help body byte-identical"`
```

Task 6.7's **Edge Cases** — one edge case inserted between the second and third entries:

```
- It renders on the Sessions help alone and the Projects help body is byte-unchanged — the indicators exist only on the sessions row, and a legend on the projects page would explain something that page does not draw. The byte-identity is asserted rather than eyeballed, because the legend parameter passes through the shared renderer.
- The wording states what the indicator means and names no tool. Portal's resume machinery runs whatever command a registration holds, so a pending resume is a pending resume whatever produced it — and this is the one string in the feature whose wording is the executor's and whose surface is the picker's own help, where a tool name would be read as a statement about what Portal is for.
- Under `NO_COLOR` it names the letters rather than two dots the mode cannot tell apart — which is the whole reason the row renders letters there, and the legend inherits it by sharing the renderer rather than by a second rule.
```

**Resolution**: Pending
**Notes**: The plan's Phase 6 task table already lists "the wording states what the indicator means and names no tool" in this task's edge-case column, so no edit to `planning.md` is required — this restores the task file to what the table already states.

---
