# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. A hand-respawned waiting pane stays frozen for life, and the plan says it is covered

**Severity**: Important
**Plan Reference**: Phase 4 (acceptance criteria) and Task 4.4 (`lazy-resume-on-attach-4-4`, "The chain's tail: a waiter that dies leaves a usable pane")
**Category**: Task Self-Containment / Cross-Phase Deferrals
**Move**: settled
**Change Type**: update-task

**Problem**:
A user who hand-respawns a waiting pane (`respawn-pane -k`, the reclaim route the design already names) gets back an ordinary shell with `@portal-resume-pending` still set on it. From that moment the pane's saved transcript stops being written and never resumes — every future reboot restores the content it held the second it paused — while the picker keeps showing its pending dot and `portal doctor` keeps counting it. Nothing reaches that state: a pane option has no address a sweep can find, and the waiter that would have reported it is gone.

Task 4.4 tells the implementer the case is handled. Its Problem lists "a hand `respawn-pane`" among the failures the chain's tail recovers, and the sentence immediately after it ("The pane's only process is then gone and tmux closes the pane") is the giveaway: a respawn does not close the pane, it runs a new command in it — and the command it kills is the parked shell the tail itself runs in, so the tail dies with the waiter. Every other item on that list works: `pkill portal` kills the waiter and leaves the `sh` parent standing, so the tail runs.

This is also a dropped hand-off between phases. Task 2.5 measures the marker's survival of `respawn-pane -k` and ends "what the system does about the state it leaves is Phase 4's concern" — and Phase 4's acceptance criteria, tasks and edge cases allocate nothing for it. A deferral recorded only in a sibling task's Context is one a later reader of the receiving phase never sees.

**Proposal**:
Accept the residual rather than build for it, and say so where it will be read. The plan's own stance settles it: Task 2.5 already places a hand respawn "in the same class as a hand edit of the store", and nothing in this feature repairs hand-edited state — a sweep would need an address the marker deliberately does not have. So the fix is to stop claiming coverage and to record the residual in the receiving phase's acceptance criteria, which is where a cross-phase deferral belongs. Three edits: drop the item from 4.4's Problem list, name it in 4.4's edge cases as the residual with its consequence stated, and add one line to Phase 4's acceptance criteria.

**Current**:

*planning.md — Phase 4, last acceptance criterion:*
```markdown
- [ ] The drawing process loads the same theme setting the picker loads and hands the resulting nomination to the panel's own appearance resolution.
```

*phase-4-tasks.md — Task 4.4, Problem:*
```markdown
**Problem**: A waiter can stop being the pane's process without having handed the pane over — a `pkill portal`, a reclaim, a hand `respawn-pane`, a read error, or a draw that could not exec the waiter at all. The pane's only process is then gone and tmux closes the pane; on an install where 43 of 44 live sessions hold a single pane, the session closes with it and the next capture drops it from the saved set with its whole transcript. The recovery cannot be unconditional, though: a pane that *was* answered has already exec'd into the user's shell, and a recovery step that ran anyway would hand it a second one — `exit` would have to be pressed twice to close a restored pane, a regression against behaviour the repo already has a test for.
```

*phase-4-tasks.md — Task 4.4, last Edge Cases bullet:*
```markdown
- Nothing in the tail reads the store or draws anything: it exists to make a pane usable, and a tail that consulted a registration could resurrect a decision the user has already had taken away from them.
```

**Proposed Text**:

*planning.md — Phase 4 acceptance criteria (the last bullet, plus one new bullet after it):*
```markdown
- [ ] The drawing process loads the same theme setting the picker loads and hands the resulting nomination to the panel's own appearance resolution.
- [ ] A waiting pane whose parked shell is destroyed along with its waiter — a hand `respawn-pane -k`, which takes the tail down with it — keeps its pending marker on a live pane, and nothing in this phase reaches that state. The residual is accepted, in the same class as a hand edit of the store: no sweep, no expiry and no repair is added for it.
```

*phase-4-tasks.md — Task 4.4, Problem:*
```markdown
**Problem**: A waiter can stop being the pane's process without having handed the pane over — a `pkill portal`, a reclaim, a read error, or a draw that could not exec the waiter at all. The pane's only process is then gone and tmux closes the pane; on an install where 43 of 44 live sessions hold a single pane, the session closes with it and the next capture drops it from the saved set with its whole transcript. The recovery cannot be unconditional, though: a pane that *was* answered has already exec'd into the user's shell, and a recovery step that ran anyway would hand it a second one — `exit` would have to be pressed twice to close a restored pane, a regression against behaviour the repo already has a test for.
```

*phase-4-tasks.md — Task 4.4, Edge Cases (the last bullet, plus one new bullet after it):*
```markdown
- Nothing in the tail reads the store or draws anything: it exists to make a pane usable, and a tail that consulted a registration could resurrect a decision the user has already had taken away from them.
- A hand `respawn-pane -k` is outside what the tail recovers, and nothing here attempts it. It kills the pane's command, which is the parked shell the tail runs in, so the tail dies with the waiter and the pending marker stays set on a pane that is now an ordinary shell — its saved scrollback frozen at the moment it paused, while the picker dot and the pending count go on claiming a decision is waiting there. The marker is destroyed only with its pane and there is no address by which a sweep could reach it, and the operation is the user destroying the process that held that pane's state — the same class as a hand edit of the store.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 4.2 cannot be finished: its source guard is asked to decide something no source scan can

**Severity**: Important
**Plan Reference**: Task 4.2 (`lazy-resume-on-attach-4-2`, "The waiting process: two keys, everything else swallowed") and Task 4.6 (`lazy-resume-on-attach-4-6`, "One redraw once the size has settled")
**Category**: Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 4.2 has an acceptance criterion the implementer cannot satisfy as written. It requires a *source* guard proving the wait path "arms no timer whose firing ends the wait or clears a report" — a claim about what a timer's firing leads to, which is dataflow, not syntax. Every source guard in this repo is a syntactic scan: a call name, an import, a declared type, a literal. There is no scan that distinguishes "a timer that ends the wait" from "a timer that triggers a redraw", and both are a channel in the same `select`.

Whoever picks this up invents a rule. If they write something loose it passes vacuously and guards nothing; if they write something tight it fails the moment task 4.6 adds the settle timer, and the guard is then deleted or exempted — taking with it the half that is genuinely checkable and genuinely load-bearing. The signal half is what keeps a killed session from leaving a Portal process behind per culled pane, and it is the part a scan can hold exactly.

The same unfalsifiable phrasing is then restated as a criterion on task 4.6, so the problem is inherited rather than resolved there.

**Proposal**:
Split the criterion where the repo's guards can actually hold. The guard covers the signal half alone and states its rule concretely: the wait path's `signal.Notify` calls name `syscall.SIGWINCH` and nothing else, so SIGHUP, SIGTERM and SIGINT keep their default disposition — which task 4.6's resize seam satisfies by construction and which a declined hangup fails. At task 4.2 the wait path arms no timer at all, so the timer clause has no subject there and is dropped; at task 4.6 it becomes real and is already held behaviourally by that task's own criteria ("A settled size equal to `--width`/`--height` produces no exec, no write and no terminal restore; the loop is still reading afterwards", "A non-empty `--report` survives the redraw unchanged"). Nothing is lost by taking it off the guard; what is gained is a guard that can be written and that fails for the right reason. Four edits, two per task.

**Current**:

*phase-4-tasks.md — Task 4.2, Do (the signal bullet):*
```markdown
- Install no handler for SIGHUP, SIGTERM or SIGINT, and arm no timer whose firing ends the wait or clears a report: the default disposition must continue to end the process when tmux tears the pane down, and a reported reason must stand until a key is pressed. Task 4.6 arms a settle timer for the resize redraw — it neither ends the wait nor expires a report — so write the guard below against those three signals and against the wait's own exit and report paths, not against the presence of a timer as such.
```

*phase-4-tasks.md — Task 4.2, Acceptance Criteria (the guard criterion):*
```markdown
- [ ] The command registers no handler for SIGHUP, SIGTERM or SIGINT, and arms no timer whose firing ends the wait or clears a report — asserted by a source guard over the wait path scoped to those three signals and to the wait's exit and report paths, so task 4.6's resize-settle timer leaves it green.
```

*phase-4-tasks.md — Task 4.2, Tests (the guard test):*
```markdown
- `"it installs no hangup, terminate or interrupt handler and arms no wait-ending or report-expiring timer"` (source guard over the wait path)
```

*phase-4-tasks.md — Task 4.6, Acceptance Criteria (the guard criterion):*
```markdown
- [ ] Task 4.2's signal-and-timer source guard passes unchanged with the resize and settle seams in place: the resize notify does not widen to SIGHUP, SIGTERM or SIGINT, and the settle timer neither ends the wait nor clears the report.
```

**Proposed Text**:

*phase-4-tasks.md — Task 4.2, Do (the signal bullet):*
```markdown
- Install no handler for SIGHUP, SIGTERM or SIGINT, and arm no timer at all: the default disposition must continue to end the process when tmux tears the pane down, and a reported reason must stand until a key is pressed. The guard below covers the signal half, which is the half a source scan can decide — it reads every `signal.Notify` call on the wait path and fails on any signal but `syscall.SIGWINCH`, so task 4.6's resize seam leaves it green while a declined hangup fails it. Task 4.6 arms the one timer the wait path ever holds, a settle window for the resize redraw; that it neither ends the wait nor expires a report is a behavioural property and is asserted there, not by this guard.
```

*phase-4-tasks.md — Task 4.2, Acceptance Criteria (the guard criterion):*
```markdown
- [ ] The command registers no handler for SIGHUP, SIGTERM or SIGINT — asserted by a source guard over the wait path that reads every `signal.Notify` call there and fails on any signal but `syscall.SIGWINCH`, so task 4.6's resize seam leaves it green.
- [ ] The wait path arms no timer: a waiter launched with a non-empty `--report` is still reading, still holding that report and still dispatching both keys however long it is left alone with no input.
```

*phase-4-tasks.md — Task 4.2, Tests (the guard test):*
```markdown
- `"it installs no hangup, terminate or interrupt handler"` (source guard over the wait path's signal.Notify calls)
- `"it holds its report and goes on waiting with no input at all"`
```

*phase-4-tasks.md — Task 4.6, Acceptance Criteria (the guard criterion):*
```markdown
- [ ] Task 4.2's signal guard passes unchanged with the resize and settle seams in place: the wait path's only `signal.Notify` names `syscall.SIGWINCH`, and SIGHUP, SIGTERM and SIGINT keep their default disposition.
- [ ] The settle timer's firing neither ends the wait nor clears the report, held by the two criteria above rather than by a guard: a matching settled size leaves the loop reading with a subsequent key still acting, and a differing one carries `--report` across unchanged.
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 1.2 changes what `hook set` writes and nothing in that task checks it

**Severity**: Minor
**Plan Reference**: Task 1.2 (`lazy-resume-on-attach-1-2`) and Task 1.3 (`lazy-resume-on-attach-1-3`)
**Category**: Acceptance Criteria Quality / Scope and Granularity
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 1.2 changes the behaviour of `portal hook set`: after it, re-registering the same command over an entry that carries a mode stops being a no-op and rewrites the entry, dropping the mode. That is the right behaviour and it needs to land in 1.2 — the alternative is an interim release in which `hook set` silently preserves a hand-pinned mode, which is exactly the model task 1.3's Context rules out. But the task has no acceptance criterion and no test for it, so the change ships in a commit whose reviewer has nothing to hold it against, and the external `SessionStart` hook exercises this path hourly.

Task 1.3 then directs the same change a second time ("Generalise `classifySet` to compare … on command and mode together"). Whoever picks up 1.3 finds the work already done and has to decide whether the plan means something more than what 1.2 left behind.

**Proposal**:
Cover the change where it lands and stop restating it. One acceptance criterion and one test in 1.2, pinned to the shape 1.2 can actually produce — a hand-written object-form entry rewritten by a bare `Set` — and a reworded Do bullet in 1.3 that says what 1.3 does to that classification rather than repeating it as new work. The two tasks then read as one change and its extension.

**Current**:

*phase-1-tasks.md — Task 1.2, Acceptance Criteria (the last criterion):*
```markdown
- [ ] `doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites.
```

*phase-1-tasks.md — Task 1.2, Tests (the last test):*
```markdown
- `"it renders every event=command pair for a key holding several events"` (existing behaviour, re-pinned over the new value type)
```

*phase-1-tasks.md — Task 1.3, Do (the classification bullet):*
```markdown
- Generalise `classifySet` to compare the registration to be written against the stored one on command and mode together: unchanged on both is `set-noop` (DEBUG, no write, file untouched), an absent key or event is `set`, anything else is `modify`.
```

**Proposed Text**:

*phase-1-tasks.md — Task 1.2, Acceptance Criteria (the last criterion, plus one new criterion after it):*
```markdown
- [ ] `doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites.
- [ ] Re-registering the same command over a hand-written object-form entry that carries a mode is a `modify` that writes, not a `set-noop`: the classification compares the registration to be written against the stored one on command and mode together, so the entry comes back as a plain string carrying no mode while every other entry keeps its bytes.
```

*phase-1-tasks.md — Task 1.2, Tests (the last test, plus one new test after it):*
```markdown
- `"it renders every event=command pair for a key holding several events"` (existing behaviour, re-pinned over the new value type)
- `"it treats a bare rewrite of a mode-carrying entry as a modify"` (object-form seed, `Set` with the same command, the file rewritten and the entry back in string form)
```

*phase-1-tasks.md — Task 1.3, Do (the classification bullet):*
```markdown
- Carry that classification across to the new signature: `classifySet` already compares command and mode together, and now reads both off the `Registration` it is handed rather than off a command it composes one from — unchanged on both is `set-noop` (DEBUG, no write, file untouched), an absent key or event is `set`, anything else is `modify`.
```

**Resolution**: Pending
**Notes**:

---

### 4. The two screens call the shared command block with the wrong arguments

**Severity**: Minor
**Plan Reference**: Task 3.3 (`lazy-resume-on-attach-3-3`) and Task 3.4 (`lazy-resume-on-attach-3-4`)
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
Both screens' Do steps call `resumeCommandRows` without the width it is declared to take, so the colour token lands in the width slot. The width is the one argument that decides whether a row is wrapped for the card's pinned width or for a narrower pane's — the distinction task 3.1 declares the parameter for, and the thing that keeps a degraded stack from being cut mid-word by the canvas. An implementer copying either call as written either loses that or stops to work out which reading was meant.

**Proposal**:
settled — restore the width argument at both call sites. Both are card builders, so both pass `resumeCardContentWidth`; the degraded stacks each task already describes pass the pane's width, which the surrounding prose in both tasks already states.

**Current**:

*phase-3-tasks.md — Task 3.3, Do (the card bullet):*
```markdown
- Build the card through `renderJoinedPanel` with three compartments: header `renderHeaderWithBadge(title, resumeCardContentWidth, true, resumePausedBadge, …)` with the title in `text.primary` bold; body `resumeCommandLabel` in `accent.primary`, then `resumeCommandRows(s.Command, th.TextPrimary, false, …)`, then the report row from `resumeReportRow` when it reports one; footer `renderConfirmCancelFooter(resumeKeyResume, resumeLabelResume, resumeKeyDiscard, resumeLabelDiscard, …)`.
```

*phase-3-tasks.md — Task 3.4, Do (the spec bullet):*
```markdown
- Build the card's spec with `targetRows: resumeCommandRows(s.Command, th.StateDestructive, true, …)`, `reportRows` from `resumeReportRow`, `title: discardConfirmTitle`, `consequence: discardConfirmConsequence`, `confirmKey: discardKeyConfirm`, `confirmLabel: discardLabelConfirm` — the `▲` glyph and the `esc cancel` half come from the builder unchanged.
```

**Proposed Text**:

*phase-3-tasks.md — Task 3.3, Do (the card bullet):*
```markdown
- Build the card through `renderJoinedPanel` with three compartments: header `renderHeaderWithBadge(title, resumeCardContentWidth, true, resumePausedBadge, …)` with the title in `text.primary` bold; body `resumeCommandLabel` in `accent.primary`, then `resumeCommandRows(s.Command, resumeCardContentWidth, th.TextPrimary, false, …)`, then the report row from `resumeReportRow` at the same width when it reports one; footer `renderConfirmCancelFooter(resumeKeyResume, resumeLabelResume, resumeKeyDiscard, resumeLabelDiscard, …)`.
```

*phase-3-tasks.md — Task 3.4, Do (the spec bullet):*
```markdown
- Build the card's spec with `targetRows: resumeCommandRows(s.Command, resumeCardContentWidth, th.StateDestructive, true, …)`, `reportRows` from `resumeReportRow` at the same width, `title: discardConfirmTitle`, `consequence: discardConfirmConsequence`, `confirmKey: discardKeyConfirm`, `confirmLabel: discardLabelConfirm` — the `▲` glyph and the `esc cancel` half come from the builder unchanged.
```

**Resolution**: Pending
**Notes**:

---
