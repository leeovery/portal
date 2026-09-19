# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. The designs drawn for this feature are never seen by the work that builds them

**Type**: Missing from plan
**Spec Reference**: §5.5 Design references (the three Paper frames as "the design reference for implementation"); §8.3 (the sessions row frame)
**Plan Reference**: Phase 3 (Acceptance, Planner's calls, tasks `lazy-resume-on-attach-3-3` and `lazy-resume-on-attach-3-6`); Phase 6 (Acceptance, Planner's calls, task `lazy-resume-on-attach-6-8`)
**Move**: settled
**Change Type**: update-task

**Problem**:
Three screens were designed for this feature — the waiting panel, the discard confirmation, and the session row carrying its new pending dot — and the plan signs all three off against something else. Phase 3 checks the two panel screens against Portal's existing modals, and Phase 6 checks the reworked session row against "its already-committed sessions reference frames", which are the frames of the row this feature is changing: the row that still carries the word `attached` and has one indicator. The row's new trailing region — a word removed, two indicators packed right, a colourless `AP` form — has no design reference in the plan at all, so its visual gate can pass a row that looks nothing like the one that was drawn and approved. Four statements in the plan ("not committed to the repository and are not an input", "No design frame is exported, committed or read anywhere in this phase") put this beyond an omission: the plan rules the designs out.

**Proposal**:
The specification names the frames as the design reference for implementation, so carrying them into the plan is not a new decision. The route is the one the repo already has: `testdata/vhs/reference/*.png` is CLAUDE.md's kept carve-out for "committed design exports — the frames the code was built *against*", so the three frames are exported from the Paper file `Portal` and committed there ahead of each phase's visual gate, and the gate reads them beside Portal's existing grammar. The specification's own limit stands unchanged — the screens are built from Portal's shared panel machinery, not from the frames' pixel dimensions — which is why this lands on the gates and the Context that frames them rather than on any renderer's acceptance criteria.

**Current**:

*(A) planning.md — Phase 3, Acceptance, final criterion*
```
- [ ] Both screens can be rendered on demand at a chosen theme and width for visual check against Portal's existing modal grammar, which both screens are built from.
```

*(B) planning.md — Phase 3, Planner's calls, final bullet*
```
- Phase 3 produces strings and a resolved theme and touches no terminal except the appearance query and the capture tool. The alternate-screen entry, the write into the pane, the hand-off and the key dispatch are Phase 4.
```

*(C) phase-3-tasks.md — task `lazy-resume-on-attach-3-3`, Context, fifth paragraph*
```
> The design frames named in the specification are not committed to the repository and are not an input to this task. Both screens are built from Portal's own existing modal grammar — the kill modal, the rename modal's badge slot, the shared destructive-confirm builder and the joined-panel frame — which is what those frames were themselves built by duplicating.
```

*(D) phase-3-tasks.md — task `lazy-resume-on-attach-3-6`, Context, fourth paragraph*
```
> The design frames named in the specification are not committed to the repository and are not an input here. These surfaces are checked against Portal's own existing modal grammar — the kill modal, the rename modal's badge slot and the shared panel frame — which are reachable through the picker fixtures the same tool already renders.
```

*(E) planning.md — Phase 6, Acceptance, final criterion*
```
- [ ] The picker resolves pending state from a single whole-server read, and the row renders on demand for visual check against Portal's existing row grammar and its already-committed sessions reference frames.
```

*(F) planning.md — Phase 6, Planner's calls, final bullet*
```
- No design frame is exported, committed or read anywhere in this phase, and no acceptance criterion names one. The capture surfaces exist so the row can be viewed live and checked against Portal's existing row grammar and its already-committed sessions reference frames; the check is the gate, not a criterion.
```

*(G) phase-6-tasks.md — task `lazy-resume-on-attach-6-8`, Context, fourth paragraph*
```
> No design frame is exported, committed or read anywhere in this phase, and no acceptance criterion names one. These fixtures exist so the row can be viewed live and checked against Portal's own existing row grammar and its already-committed sessions reference frames; that check is the phase's gate, not a criterion of this task.
```

**Proposed Text**:

*(A) planning.md — Phase 3, Acceptance, final criterion*
```
- [ ] Both screens can be rendered on demand at a chosen theme and width for visual check against the committed reference frames `resume-panel-waiting-nord.png` and `resume-panel-discard-confirm-nord.png`, and against Portal's existing modal grammar, which both screens are built from.
```

*(B) planning.md — Phase 3, Planner's calls, final bullet (replaced by two bullets)*
```
- The two frames the specification names for these screens — **Resume panel — waiting (Nord)** and **Resume panel — discard confirm (Nord)** — are exported from the Paper file `Portal` and committed to `testdata/vhs/reference/` before the phase's visual gate, as `resume-panel-waiting-nord.png` and `resume-panel-discard-confirm-nord.png`. That directory is the repo's kept carve-out for committed design exports — the frames the code is built against rather than renders of it — so they are a permanent artifact and not capture scaffolding. They are the design reference the gate reads beside the live `capturetool` render; the screens are still built from Portal's own shared panel machinery rather than from the frames' pixel dimensions, which is what the frames were themselves built by duplicating.
- Phase 3 produces strings and a resolved theme and touches no terminal except the appearance query and the capture tool. The alternate-screen entry, the write into the pane, the hand-off and the key dispatch are Phase 4.
```

*(C) phase-3-tasks.md — task `lazy-resume-on-attach-3-3`, Context, fifth paragraph*
```
> The design frame the specification names for this screen — **Resume panel — waiting (Nord)** — is committed at `testdata/vhs/reference/resume-panel-waiting-nord.png` and is the design reference for what this screen looks like. It is read at the phase's visual gate, not copied from: the panel is built from Portal's own existing modal grammar — the kill modal, the rename modal's badge slot, the shared destructive-confirm builder and the joined-panel frame — which is what the frame was itself built by duplicating, so its card geometry is identical to the existing modals' rather than approximate.
```

*(D) phase-3-tasks.md — task `lazy-resume-on-attach-3-6`, Context, fourth paragraph*
```
> The two design frames the specification names are committed at `testdata/vhs/reference/resume-panel-waiting-nord.png` and `testdata/vhs/reference/resume-panel-discard-confirm-nord.png`. These surfaces exist so each screen can be rendered live and held against its frame at the phase's visual gate, and against Portal's own existing modal grammar — the kill modal, the rename modal's badge slot and the shared panel frame — which is reachable through the picker fixtures the same tool already renders.
```

*(E) planning.md — Phase 6, Acceptance, final criterion*
```
- [ ] The picker resolves pending state from a single whole-server read, and the row renders on demand for visual check against the committed reference frame `sessions-pending-resume-dot-nord.png` and against Portal's existing row grammar.
```

*(F) planning.md — Phase 6, Planner's calls, final bullet*
```
- The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is exported from the Paper file `Portal` and committed to `testdata/vhs/reference/sessions-pending-resume-dot-nord.png` before the phase's visual gate, beside the sessions frames already there. It is the only design reference that exists for the reworked trailing region: the already-committed sessions frames show the row this phase changes, with the word `attached` and a single indicator, so they cannot settle the packing, the spacing or the colourless form. The capture fixtures exist so the row can be rendered live and held against that frame; the check is the gate, not a criterion.
```

*(G) phase-6-tasks.md — task `lazy-resume-on-attach-6-8`, Context, fourth paragraph*
```
> The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, and it is the design reference the phase's visual gate reads: the sessions frames already in that directory show the row before the word was dropped, so they say nothing about where the indicators sit. These fixtures exist so the row can be viewed live and held against that frame and against Portal's own existing row grammar; that check is the phase's gate, not a criterion of this task.
```

**Resolution**: Pending
**Notes**:

---

### 2. Nothing proves a waiting pane leaves the panes beside it alive

**Type**: Incomplete coverage
**Spec Reference**: §4.1 ("**A waiting pane must never block the panes beside it**"), §5.1 ("**Nothing blocks.** … Panes beside it are fully live throughout", and the worked example of a left pane waiting beside a right pane holding a shell)
**Plan Reference**: Phase 4 Acceptance; task `lazy-resume-on-attach-4-7`
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The constraint that decided this feature's entire architecture is never checked. A waiting pane must not take the keyboard of the panes beside it — that single property eliminated the dead-pane design (tmux `key-table` is a session option and arms every pane in the session) and every floating overlay (`display-popup` captures the whole client's keyboard), and it is the reason the waiter is a process in the pane at all. The plan's one real-pane suite reboots two **single-pane** sessions, so no test in the feature ever has a pane waiting beside a live one. If the waiter or the chain regresses into taking the client's input, the whole plan is green and the user finds out by losing the keyboard in the pane they were working in.

**Proposal**:
The specification's own worked example is the fixture: one window, a left pane waiting and a right pane holding a bare shell. Task 4.7 already reboots a real server with a real waiter, so this is a second pane on the subject's session plus two assertions — a key sent to the sibling reaches the sibling's shell while the neighbour holds the panel, and the sibling's own content is untouched. The capture half of the same worked example is already covered by task 2.4 against a fake commander; this is the keyboard half, which only a real pane can answer.

**Current**:

*(A) planning.md — Phase 4, Acceptance, after the "Scrollback replays for every restored pane…" criterion*
```
- [ ] Scrollback replays for every restored pane exactly as it does today, whether or not a resume is pending, and no bootstrap step, step ordering, eager signal pass or global hook changes.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, second bullet*
```
- Seed two single-pane sessions on that socket: a **lazy subject** whose `hooks.json` entry is the string form (so it inherits the shipped lazy default with no `prefs.json` key set at all) and an **eager control** whose entry carries `resume: eager`. Stamp each pane's own token with `ts.StampPaneToken`, print a distinct recognisable line into each pane before the capture, and give each hook command its own sentinel file so the two cannot be confused.
```

*(C) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Acceptance Criteria, after the process-tree criterion*
```
- [ ] The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`, and no `portal state resume-draw` survives the hand-off.
```

*(D) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests, after `"it carries one shell parent and one waiter"`*
```
- `"it carries one shell parent and one waiter"`
```

**Proposed Text**:

*(A) planning.md — Phase 4, Acceptance*
```
- [ ] Scrollback replays for every restored pane exactly as it does today, whether or not a resume is pending, and no bootstrap step, step ordering, eager signal pass or global hook changes.
- [ ] A pane beside a waiting one is fully live throughout: keys sent to it reach its own process, and its content and its capture are untouched by the neighbour holding the panel.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, second bullet*
```
- Seed two sessions on that socket: a **lazy subject** whose `hooks.json` entry is the string form (so it inherits the shipped lazy default with no `prefs.json` key set at all) and an **eager control** whose entry carries `resume: eager`. Give the subject's window a second pane holding a plain shell and carrying no registration — the specification's worked example, a left pane that waits beside a right pane the user works in. Stamp each registered pane's own token with `ts.StampPaneToken`, print a distinct recognisable line into each pane before the capture, and give each hook command its own sentinel file so the two cannot be confused.
```

*(C) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Acceptance Criteria*
```
- [ ] The subject's process tree under its `pane_pid` is one `sh -c` and one `portal state resume-wait`, and no `portal state resume-draw` survives the hand-off.
- [ ] While the subject holds the panel, a command sent to its sibling pane with `send-keys` runs in that pane and its output is readable from `capture-pane -p` on the sibling — the waiting pane takes no key that was not sent to it.
- [ ] The sibling pane carries no pending marker, is absent from `CaptureStructure`'s pending set, shows no panel, and its scrollback file is written while the subject's is not.
```

*(D) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests*
```
- `"it carries one shell parent and one waiter"`
- `"it leaves the pane beside it live"` (a command sent to the sibling runs there while the subject waits)
- `"it goes on capturing the pane beside it"`
```

Add to the same task's **Edge Cases**:
```
- Panes beside a waiting one are fully live throughout, and nothing short of a real two-pane window shows it: a waiting pane that captured the client's keyboard is exactly what ruled out the dead pane and every floating overlay, and it is invisible to every seam-driven test in the feature.
```

**Resolution**: Pending
**Notes**:

---

### 3. The memory figure the feature exists to deliver is never taken

**Type**: Incomplete coverage
**Spec Reference**: §4.2 ("Two figures the memory case rests on are estimates rather than measurements, because they cannot be taken until the code exists: the waiter's actual resident size on the settled path… and whether the draw-then-hand-off split holds it at that floor in practice. Both are bounded — the floor is measured above and the ceiling is the daemon's 22 MB"); §1 (13.1 GB across 42 resumed processes)
**Plan Reference**: task `lazy-resume-on-attach-4-7`
**Move**: settled
**Change Type**: add-to-task

**Problem**:
This work exists to turn 13.1 GB of resumed processes into a waiting set costing an estimated ~2 MB a pane, and the plan builds the whole thing without ever reading that number. The specification names the two figures it could not take — the waiter's resident size on the settled path, and whether the draw-then-hand-off split actually holds it at the floor — as figures that become takeable the moment the code exists. If the split leaks (a theme or a renderer page kept resident in the waiter, a chain step that does not exec away), every test in the plan still passes and the feature ships costing what it was built to save, on an install that produces forty-one waiting panes per reboot.

**Proposal**:
The one place the figure can be read is already in the plan: task 4.7 runs a real waiter in a real pane and already takes a scoped, read-only `ps` over the subject's own `pane_pid` and its descendants. Adding `rss` to that read takes the measurement, and the specification supplies the bound to assert against rather than one being invented — the daemon's measured 22 MB ceiling, against a floor the specification measured at ~1.7 MB. The assertion is deliberately the ceiling, not the estimate: it catches a split that did not hold without pinning the plan to a figure the specification itself calls a guess.

**Current**:

*(A) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, fourth bullet*
```
- Take the process-tree reading read-only and scoped: resolve the subject's `#{pane_pid}` from the fixture socket and run `ps -o pid,ppid,command` over that pid and its descendants alone. The suite signals nothing and enumerates no process it did not cause to exist.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests, after `"it carries one shell parent and one waiter"`*
```
- `"it carries one shell parent and one waiter"`
```

**Proposed Text**:

*(A) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, fourth bullet*
```
- Take the process-tree reading read-only and scoped: resolve the subject's `#{pane_pid}` from the fixture socket and run `ps -o pid,ppid,rss,command` over that pid and its descendants alone. The suite signals nothing and enumerates no process it did not cause to exist. Report the resting tree's combined `rss` in the test's own output whether it passes or fails, so the figure the memory case rests on is readable from a run rather than only from a failure.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests*
```
- `"it carries one shell parent and one waiter"`
- `"it holds the pane below the resident ceiling the daemon was measured at"`
```

Add to the same task's **Acceptance Criteria**, after the process-tree criterion:
```
- [ ] The subject's resting tree — the parked shell plus the waiter, measured once the pane is waiting and the draw is gone — carries a combined resident size below 22 MB, the ceiling the daemon was measured at, and the figure is reported by the test whether it passes or fails.
```

Add to the same task's **Edge Cases**:
```
- The resident size is asserted against the daemon's 22 MB ceiling rather than against the ~2 MB estimate: the estimate is a guess at what Portal's startup touches, and pinning a test to it would fail on a change that costs nothing, while the ceiling catches the failure that matters — a draw that did not hand off, or a wait path that kept the rendering pages resident, which would put a full waiting set back where the eager path was.
```

**Resolution**: Pending
**Notes**:

---
