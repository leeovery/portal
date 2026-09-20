# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. The page that tells users what a reboot does still says their commands run by themselves

**Type**: Incomplete coverage
**Spec Reference**: §1 (the third state), §2.1 (the install-wide default is lazy — "an install that upgrades and reboots meets panels rather than processes"), §4.1
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5` (The helper decides, marks and hands the pane to the panel)
**Move**: settled
**Change Type**: update-task

**Problem**:
Under the mode this feature ships on, a rebooted install comes back holding panels and nothing runs until the user presses Enter. Two places in the README go on telling the reader the opposite, and neither is corrected by any task. The **Automatic Server Bootstrap & Restoration** section — the section a user reads to learn what a reboot does to their sessions — still ends its description of restore with "and resume hooks run on the recreated panes", and repeats it two paragraphs later ("Pair restoration with resume hooks to re-run pane commands such as dev servers and editors after a reboot"). Inside the `xctl hook` section, the paragraph about renaming still says "A renamed session still re-runs its command after the next reboot". The plan already applies the rule that a doc edit rides with the task that falsifies it — task 4.5 carries the hook section's opening paragraph and its **When hooks fire** paragraph for exactly this reason — and these sentences are falsified by the same change and left standing. A user who upgrades, reboots and meets forty-one panels then reads their own documentation telling them the commands were supposed to have run, with nothing on the page naming the panel, its two keys, or the mode that restores the old behaviour.

**Proposal**:
Widen the README edit task 4.5 already carries to cover every sentence the same change falsifies, rather than the two paragraphs it names today. The spec decides the behaviour (§2.1: the install ships lazy and an upgraded install meets panels; §4.1: the pane holds a panel until the user answers) and §3.3 decides where the old behaviour is re-selected (`--resume-mode eager`, documented by task 1.6), so nothing new is decided here — the wording stays the executor's under the constraint the rest of the feature holds to: state what the pane comes back holding, name `eager` as the mode that keeps today's behaviour, name no particular tool.

**Current**:

Task 4.5 — **Do**, final bullet:

```
- Edit the README's `xctl hook` section, which still tells the reader a registered command re-executes automatically after a reboot, that a renamed session still re-runs its command, and — under its **When hooks fire** heading — that resume hooks run when Portal recreates a pane. Under the shipped default the pane comes back holding the resume panel showing that command, with `⏎ resume` and `d discard`, and the command runs when the user answers; `eager` is the mode that keeps the fire-on-restore behaviour those sentences describe. One edit to the section's opening paragraph and one to the **When hooks fire** paragraph, wording the executor's, naming no particular tool.
```

Task 4.5 — **Acceptance Criteria**, final criterion:

```
- [ ] The README's `xctl hook` section no longer states that a registered command re-executes by itself after a reboot: its opening paragraph and its **When hooks fire** paragraph both describe the panel a lazy registration comes back holding and name `eager` as the mode that keeps today's behaviour.
```

**Proposed Text**:

Task 4.5 — **Do**, final bullet, replaced by:

```
- Edit the README everywhere it tells the reader a registered command runs by itself on restore. In the `xctl hook` section: the opening paragraph ("re-executes automatically when a session is attached after a reboot"), the rename paragraph ("A renamed session still re-runs its command after the next reboot" — the hook still survives the rename, but what comes back is the panel), and the **When hooks fire** paragraph ("resume hooks run only when Portal recreates a pane from saved state"). In the **Automatic Server Bootstrap & Restoration** section: the sentence ending "and resume hooks run on the recreated panes", and the "Pair restoration with resume hooks to re-run pane commands such as dev servers and editors after a reboot" line that reads the same way. Under the shipped default the pane comes back holding the resume panel showing that command, with `⏎ resume` and `d discard`, and the command runs when the user answers; `eager` is the mode that keeps the fire-on-restore behaviour those sentences describe. Wording the executor's, naming no particular tool.
```

Task 4.5 — **Acceptance Criteria**, final criterion, replaced by:

```
- [ ] No README sentence still states that a registered command re-executes by itself after a reboot: the `xctl hook` section's opening, rename and **When hooks fire** paragraphs, and the **Automatic Server Bootstrap & Restoration** section's two resume-hook sentences, all describe the panel a lazy registration comes back holding and name `eager` as the mode that keeps today's behaviour.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Task 4-5's README edit now covers every sentence the change falsifies — the hook section's opening, rename and When-hooks-fire paragraphs, and the bootstrap-and-restoration section's two resume-hook sentences — with the criterion widened to match.

---
