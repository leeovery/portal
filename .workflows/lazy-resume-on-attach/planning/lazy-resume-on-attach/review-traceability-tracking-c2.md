# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. A panel the user ignores is never proved to survive the next reboot

**Type**: Incomplete coverage
**Spec Reference**: §4.4 ("Stickiness is the absence of a dismissal path… a reboot restores the pane and draws the panel again from the still-unfired registration"); §7.2 ("a pane can wait indefinitely and still be restored on every subsequent reboot, with its original content and a fresh panel, however many reboots it waits through"); §7.1 (the screenful-plus-dead-card loss the freeze exists to prevent)
**Plan Reference**: Phase 4 Acceptance; task `lazy-resume-on-attach-4-7`. Nearest existing coverage: `lazy-resume-on-attach-2-3` (merge + housekeeping, unit), `lazy-resume-on-attach-2-5` (marker durability, real tmux, no restore), `lazy-resume-on-attach-5-6` (second reboot, but after the discard)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The feature's central promise to a user who does nothing is that doing nothing is safe: leave the panel up, reboot next week, and the pane comes back with the same transcript and the same offer. Nothing the plan builds ever demonstrates that. Every verification of a pane's survival while it waits stops short of a restore — the token-matched merge is checked against an in-memory index, the housekeeping pass against a computed reference set, and the real-tmux durability suite deliberately writes no scrollback at all, so it can only assert the *path* a record names and never that the bytes behind it come back. Both real-pane suites reboot from a state captured while nothing was waiting, and the only second reboot in the plan happens after a discard, when there is no panel left to offer. So the one path where a waiting pane's record is itself the input to the next restore — the path every unanswered pane on the install takes on its second reboot — is never exercised. If the frozen record, its scrollback file, or the fresh decision to draw fails there, the whole plan is green and the user loses a week-old transcript and the offer with it, silently, on the reboot after the one they were shown.

**Proposal**:
The rig already exists: task 4.7 boots a real subject that is holding the panel, so the missing step is to capture that live server a second time while the subject is still waiting, encode it, reboot again, and read the pane. That is the only point in the plan where the frozen record is produced by a real capture and consumed by a real restore. It lands as a step in the middle of 4.7's existing ordered subtests rather than as a new task, because a new one would restage the whole fixture — a built binary, an isolated state dir, a disposable socket, two seeded sessions and a reboot — to assert one property of the pane 4.7 already has waiting in front of it; and it lands in Phase 4 rather than Phase 2 because Phase 2's suites run in the unit lane, where no restore orchestrator and no built binary are available. The assertions are the specification's own words: a fresh panel, the original content underneath it, and the marker still set.

**Current**:

*(A) planning.md — Phase 4, Acceptance, the sibling-liveness criterion*
```
- [ ] A pane beside a waiting one is fully live throughout: keys sent to it reach its own process, and its content and its capture are untouched by the neighbour holding the panel.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, the ordered-subtests bullet*
```
- Assert in ordered subtests: (a) `state.CaptureStructure`'s pending set holds the subject pane's live key and not the control's; (b) `capture-pane -p` on the subject returns the panel's title and both key hints while `capture-pane -a -p` returns the pre-reboot line underneath it; (c) the control's sentinel appears within `restoretest.PaneReactionBudget` and its `capture-pane -p` shows its transcript and no panel; (d) the subject's process tree is exactly one `sh -c` parent over one `portal state resume-wait`, with no `resume-draw` still resident and no second shell; (e) after `send-keys Enter`, the marker clears (poll `tmux.ReadPaneOption`), the subject's sentinel appears, and `capture-pane -p` shows the pre-reboot line with the command's output over it; (f) after `send-keys exit`, the pane is gone within the existing budget on the first press.
```

*(C) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Acceptance Criteria, the sibling-capture and Enter criteria*
```
- [ ] The sibling pane carries no pending marker, is absent from the capture's pending set, shows no panel, and its scrollback file is written while the subject's is not.
- [ ] `send-keys Enter` clears `@portal-resume-pending` on the subject within a bounded poll, produces the subject's sentinel, and leaves the pre-reboot line visible with the resumed command's output over it.
```

*(D) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests*
```
- `"it goes on capturing the pane beside it"`
- `"it clears the marker and runs the command on Enter"`
```

*(E) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Edge Cases, the eager-control entry*
```
- An eager registration on the same server restores exactly as today in the same run: the two modes ship live together, and a regression that only shows when both are present is exactly the one a single-mode fixture would miss.
```

**Proposed Text**:

*(A) planning.md — Phase 4, Acceptance (the criterion above, followed by one more)*
```
- [ ] A pane beside a waiting one is fully live throughout: keys sent to it reach its own process, and its content and its capture are untouched by the neighbour holding the panel.
- [ ] A pane still holding the panel when the state is captured comes back on the next reboot holding a fresh panel, over the transcript it paused on, with its pending marker set — an offer the user ignored is never spent, and a wait that spans reboots costs no content.
```

*(B) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Do, the ordered-subtests bullet*
```
- Assert in ordered subtests: (a) `state.CaptureStructure`'s pending set holds the subject pane's live key and not the control's; (b) `capture-pane -p` on the subject returns the panel's title and both key hints while `capture-pane -a -p` returns the pre-reboot line underneath it; (c) the control's sentinel appears within `restoretest.PaneReactionBudget` and its `capture-pane -p` shows its transcript and no panel; (d) the subject's process tree is exactly one `sh -c` parent over one `portal state resume-wait`, with no `resume-draw` still resident and no second shell; (e) with the subject still waiting and unanswered, take a second capture by the same route the first took — `state.CaptureStructure`, the per-pane scrollback dump for every pane it did not report pending, then `state.EncodeIndex` — and assert the subject's record still names the scrollback file its pre-reboot line was filed under and that the file's bytes are unchanged; then `restoretest.RebootServer`, restore and `restoretest.DriveSignalHydrate` a second time and assert the subject comes back with the panel on `capture-pane -p`, the pre-reboot line intact under it on `capture-pane -a -p`, `@portal-resume-pending` set again, and its live pane key back in the fresh capture's pending set; (f) after `send-keys Enter`, the marker clears (poll `tmux.ReadPaneOption`), the subject's sentinel appears, and `capture-pane -p` shows the pre-reboot line with the command's output over it; (g) after `send-keys exit`, the pane is gone within the existing budget on the first press.
```

*(C) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Acceptance Criteria (the sibling-capture criterion, then two new ones, then the Enter criterion)*
```
- [ ] The sibling pane carries no pending marker, is absent from the capture's pending set, shows no panel, and its scrollback file is written while the subject's is not.
- [ ] A second capture taken while the subject is still waiting leaves its scrollback file byte-unchanged and carries its previous record forward, so the file `sessions.json` names after that capture is the one written before the pane ever paused.
- [ ] Restored a second time from that capture, the subject comes back holding the panel with the pre-reboot line intact underneath it, `@portal-resume-pending` set, and its key in the fresh capture's pending set — an unanswered offer returns rather than being spent, and nothing of the transcript is lost across the second reboot.
- [ ] `send-keys Enter` clears `@portal-resume-pending` on the subject within a bounded poll, produces the subject's sentinel, and leaves the pre-reboot line visible with the resumed command's output over it.
```

*(D) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Tests (the two above, with two inserted between them)*
```
- `"it goes on capturing the pane beside it"`
- `"it keeps the waiting pane's transcript through a capture it was never answered through"`
- `"it offers the panel again on the reboot after the one that drew it"`
- `"it clears the marker and runs the command on Enter"`
```

*(E) phase-4-tasks.md — task `lazy-resume-on-attach-4-7`, Edge Cases (the eager-control entry, followed by one more)*
```
- An eager registration on the same server restores exactly as today in the same run: the two modes ship live together, and a regression that only shows when both are present is exactly the one a single-mode fixture would miss.
- A pane captured while it is still waiting is the only case in which the freeze's whole purpose is observable: its saved record is the token-matched merge's output, its scrollback file is one nothing has rewritten since it paused, and the panel that comes back is drawn afresh from a registration that was never fired. Every other check of those three stops at an in-memory index, a computed reference set, or the path a record names — none of them reaches the bytes a second restore replays, and the loss they are guarding against is silent when it happens.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 4-7 gains the second capture-and-reboot step in the middle of its ordered subtests (later letters shifted), two acceptance criteria, two test names and the edge case; Phase 4 gains the acceptance criterion. Task file and tick body both updated.

---

### 2. The unreadable preferences file is decided by corrigendum and pinned by nothing

**Type**: Incomplete coverage
**Spec Reference**: §3.1 ("**A file that cannot be read at all resolves the same way**, so an install whose `prefs.json` is unreadable meets panels rather than processes"); Corrigendum 2026-09-19 (unreadable preferences file)
**Plan Reference**: task `lazy-resume-on-attach-1-5`
**Move**: settled
**Change Type**: add-to-task

**Problem**:
Whether every restored pane on an install waits or fires is decided by one value, and the specification was amended mid-planning precisely to say what happens when the file holding it cannot be read at all: the install meets panels, because a panel costs a keystroke and a resume nobody asked for cannot be taken back. The task that owns that read directs the behaviour in its steps and then never checks it — its criteria and tests cover an absent file, an absent key, an empty value, an unrecognised value, a wrong-typed value and a wholly corrupt file, and stop there. An unreadable file is the one case of the six that is not a decode at all: it is the branch where the accessor hands its caller an error alongside the value, so it is also the one where a later reader could treat the failure as a reason to do something other than default, and nothing in the task would notice. The cost of getting it wrong is the inverse of what the amendment chose — an install that silently resumes forty-one processes on the boot after its preferences file went unreadable.

**Proposal**:
The behaviour is already directed in the task's steps ("a non-`ErrNotExist` read error propagates alongside `resumemode.Default`"), so this is the criterion and the test that hold it there, plus the corrigendum's own words in the task's Context so an implementer reads the reason beside the rule. The fixture is the one the repo already uses for a denied read — a staged file at mode 0000, as `themetest.DenyRead` stages one — and the assertion is the specification's: the value answered is the shipped default, whatever the error says.

**Current**:

*(A) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Acceptance Criteria, the tolerant-decode criterion*
```
- [ ] It returns `Lazy` for an absent file, an absent key, an empty value, an unrecognised value (`"LAZY"`, `" lazy"`, `"off"`), a non-string value, and a wholly corrupt file.
```

*(B) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Tests*
```
- `"it answers lazy for an absent prefs.json and for a corrupt one"`
```

*(C) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Edge Cases, the first entry*
```
- Missing, empty, corrupt or unrecognised all give lazy — the shipped default — with no error and no repair of the file.
```

*(D) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Context, the first paragraph*
```
> `prefs.json` holds the install's UI preferences — the theme and the session-list grouping mode — and is where this one lives, as the key `resume_mode`, holding `eager` or `lazy`. It decodes tolerantly and independently like every other field there, and the key is `omitempty` on write, so an install that never set it carries no key.
```

**Proposed Text**:

*(A) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Acceptance Criteria (the criterion above, followed by one more)*
```
- [ ] It returns `Lazy` for an absent file, an absent key, an empty value, an unrecognised value (`"LAZY"`, `" lazy"`, `"off"`), a non-string value, and a wholly corrupt file.
- [ ] A `prefs.json` that exists but cannot be read at all answers `Lazy` too — the value is the shipped default alongside whatever error is propagated, so a caller that takes the value meets panels rather than processes.
```

*(B) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Tests (the test above, followed by one more)*
```
- `"it answers lazy for an absent prefs.json and for a corrupt one"`
- `"it answers lazy for a prefs.json that cannot be read at all"` (file staged unreadable, as `themetest.DenyRead` stages one)
```

*(C) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Edge Cases (the entry above, followed by one more)*
```
- Missing, empty, corrupt or unrecognised all give lazy — the shipped default — with no error and no repair of the file.
- A file that cannot be read at all gives lazy as well, and by the same route: the accessor answers the default beside the error rather than leaving the mode undecided, so no caller has to invent a rule for the case. An install whose preferences file has gone unreadable meets panels rather than processes, which is the safe direction — a panel is answered in a keystroke and a resume the user did not want cannot be taken back.
```

*(D) phase-1-tasks.md — task `lazy-resume-on-attach-1-5`, Context (the paragraph above, followed by one more)*
```
> `prefs.json` holds the install's UI preferences — the theme and the session-list grouping mode — and is where this one lives, as the key `resume_mode`, holding `eager` or `lazy`. It decodes tolerantly and independently like every other field there, and the key is `omitempty` on write, so an install that never set it carries no key.
>
> **Corrigendum 2026-09-19**: a file that cannot be read at all resolves the same way — the shipped default arriving by the ordinary route rather than a second rule. The reason is stated with it: a panel can be answered in a keystroke, while a resume the user did not want cannot be taken back, so an unreadable preferences file lands the install on the safe side of the setting rather than the convenient one.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 1-5 gains the unreadable-file criterion, its test over a denied read, the edge case and the corrigendum's own words in Context. Task file and tick body both updated.

---
