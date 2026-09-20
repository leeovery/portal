# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. Turning the feature off install-wide is never proved to work

**Type**: Incomplete coverage
**Spec Reference**: §2 (the mode comes from an install-wide default with a three-state per-registration override; "everything else is governed centrally"), §2.1, §3.1 (no UI — the file is hand-edited)
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-5`
**Move**: settled
**Change Type**: add-to-task

**Problem**:
`resume_mode` in `prefs.json` is the only way a user can stop every restored pane coming back holding a panel, and it has no UI at all — they hand-edit the file and reboot. Nothing in the plan checks that setting it to `eager` actually suppresses the wait. The store's resolution function is proved in isolation and the helper is proved to call it, but no criterion or test joins the two: the plan's eager cases all pin the mode on the registration, which is the route a user who wants one exception takes, not the route a user who wants the feature off takes. If the install default never reaches the decision, an install that set it to `eager` comes back with forty-one panels anyway and the user has nothing to turn the handle on. The reverse pin — a registration set to `lazy` under an eager install — is equally unexercised at the helper, so the override's second direction is unproved where it is applied.

**Proposal**:
Name both routes to eager in task 4.5's first acceptance criterion and add the two missing cases to its tests. The specification decides the behaviour (a registration that names a mode wins outright; otherwise the install's; otherwise lazy) — this carries that decision into the phase that applies it rather than leaving it proved only in the vocabulary package.

**Current**:

Task 4.5, **Acceptance Criteria**, first bullet:

```
- [ ] A pane with no registration, and one whose registration resolves eager, produce byte-identical behaviour to today on all three tails: no `set-option -p`, no pending marker, the same skeleton-marker clear, the same `exec` argv and the same log records.
```

Task 4.5, **Tests**, third entry:

```
- `"it restores an eager registration exactly as today"` (table over the three tails)
```

**Proposed Text**:

Task 4.5, **Acceptance Criteria**, first bullet — replaced by two bullets:

```
- [ ] A pane with no registration, and one whose mode resolves eager, produce byte-identical behaviour to today on all three tails: no `set-option -p`, no pending marker, the same skeleton-marker clear, the same `exec` argv and the same log records — covering both routes to eager, a registration pinned `eager` under the shipped lazy install and a registration naming no mode under an install whose `resume_mode` is `eager`.
- [ ] A registration pinned `lazy` under an install whose `resume_mode` is `eager` still waits, so the override beats the install in both directions where the decision is taken and not only inside the resolution function.
```

Task 4.5, **Tests**, third entry — replaced by three entries:

```
- `"it restores an eager registration exactly as today"` (table over the three tails)
- `"it restores a registration naming no mode eagerly under an eager install"` (table over the three tails)
- `"it waits for a registration pinned lazy under an eager install"`
```

**Resolution**: Pending
**Notes**:

---

### 2. The end-to-end resume suite never checks that the panel shows the right command

**Type**: Incomplete coverage
**Spec Reference**: §5.3 (an `ON RESUME` label with the registered command beneath it), §5.2 (the command is the only thing on the panel that says which piece of work the pane is holding)
**Plan Reference**: Phase 4, task `lazy-resume-on-attach-4-7`
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The panel exists to tell the user which piece of work the pane is holding, and the only thing on it that does that is the registered command. The one suite that drives a real restore into a real pane reads the pane back and checks the title and the two key hints — not the command. So the feature could come back with a panel whose command line is empty, truncated at a space, or mangled by the shell the helper composed, and the suite would pass. That is the specific failure this suite exists to catch: the command is arbitrary user-authored text that crosses one `sh -c` boundary on its way to the pane, and the task's own problem statement names that shell parse as one of the three things only a real run can prove. The seeded command is also unconstrained, so a fixture using a single bare word would exercise none of the quoting.

**Proposal**:
Assert the registered command on the panel alongside the title and the key hints, and author the subject's command so it carries the shapes the quoting exists for — a space and an embedded single quote. The specification fixes what the panel renders; this reads it back off the pane. The discard suite already reads the command off the confirmation, so this closes the same check on the resume side.

**Current**:

Task 4.7, **Do**, second bullet:

```
- Seed two sessions on that socket: a **lazy subject** whose `hooks.json` entry is the string form (so it inherits the shipped lazy default with no `prefs.json` key set at all) and an **eager control** whose entry carries `resume: eager`. Give the subject's window a second pane holding a plain shell and carrying no registration — the specification's worked example, a left pane that waits beside a right pane the user works in. Stamp each registered pane's own token with `ts.StampPaneToken`, print a distinct recognisable line into each pane before the capture, and give each hook command its own sentinel file so the two cannot be confused.
```

Task 4.7, **Do**, the `(b)` clause of the ordered-subtests bullet:

```
(b) `capture-pane -p` on the subject returns the panel's title and both key hints while `capture-pane -a -p` returns the pre-reboot line underneath it;
```

Task 4.7, **Acceptance Criteria**, second bullet:

```
- [ ] `capture-pane -p` on the subject returns the panel — its title and both key hints — and `capture-pane -a -p` returns the line the pane held before the reboot, intact underneath it.
```

Task 4.7, **Tests**, second entry:

```
- `"it shows the panel over the pane's own transcript"`
```

**Proposed Text**:

Task 4.7, **Do**, second bullet:

```
- Seed two sessions on that socket: a **lazy subject** whose `hooks.json` entry is the string form (so it inherits the shipped lazy default with no `prefs.json` key set at all) and an **eager control** whose entry carries `resume: eager`. Give the subject's window a second pane holding a plain shell and carrying no registration — the specification's worked example, a left pane that waits beside a right pane the user works in. Stamp each registered pane's own token with `ts.StampPaneToken`, print a distinct recognisable line into each pane before the capture, and give each hook command its own sentinel file so the two cannot be confused. Author the subject's command so it carries a space and an embedded single quote, and keep it short enough to render inside the card's wrapped rows — it is both the string the panel has to show back and the string that crosses the `sh -c` the helper composed.
```

Task 4.7, **Do**, the `(b)` clause of the ordered-subtests bullet:

```
(b) `capture-pane -p` on the subject returns the panel's title, the registered command as it is stored, and both key hints, while `capture-pane -a -p` returns the pre-reboot line underneath it;
```

Task 4.7, **Acceptance Criteria**, second bullet — replaced by two bullets:

```
- [ ] `capture-pane -p` on the subject returns the panel — its title, the registered command and both key hints — and `capture-pane -a -p` returns the line the pane held before the reboot, intact underneath it.
- [ ] The command the panel shows is the stored command as stored, its space and its embedded single quote included, so the quoting that carried it across the helper's `sh -c` is proved at the pane and not only at the argv.
```

Task 4.7, **Tests**, second entry — replaced by two entries:

```
- `"it shows the panel over the pane's own transcript"`
- `"it shows the stored command on the panel through the shell the helper composed"`
```

**Resolution**: Pending
**Notes**:

---

### 3. Phase 3 signs the discard confirmation off against a paraphrase of its consequence line

**Type**: Incomplete coverage
**Spec Reference**: §5.4 and Corrigendum 2026-09-19 (the consequence line is stated verbatim: `Removes this pane's resume command permanently. The session and its scrollback are untouched.`)
**Plan Reference**: Phase 3, **Acceptance**, fourth bullet (`planning.md`)
**Move**: settled
**Change Type**: update-task

**Problem**:
The sentence on the discard confirmation is the only user-facing copy on the screen that destroys a user-authored command with no other copy anywhere, and the specification fixes its wording word for word — a corrigendum was raised precisely because leaving it as "a plain-language consequence line" left that one string unstated. Phase 3's acceptance criterion still describes it that way, so the phase can be signed off against any sentence an implementer finds plausible. The task detail carries the verbatim string, but the phase contract is what the gate reads, and the two now say different things about the same line.

**Proposal**:
Restate the criterion with the specification's own sentence, as the task detail already does. The wording is decided; the criterion only has to carry it.

**Current**:

`planning.md`, Phase 3, **Acceptance**, fourth bullet:

```
- [ ] The discard confirmation is built through the same shared destructive-confirm builder as the picker's kill modal: `▲ Discard resume?`, the command in the destructive token, a plain-language consequence line, and `y discard   esc cancel`.
```

**Proposed Text**:

```
- [ ] The discard confirmation is built through the same shared destructive-confirm builder as the picker's kill modal: `▲ Discard resume?`, the command in the destructive token, the consequence line `Removes this pane's resume command permanently. The session and its scrollback are untouched.` rendered verbatim, and `y discard   esc cancel`.
```

**Resolution**: Pending
**Notes**:

---

### 4. Phase 4's contract for the chain's tail omits the pane that was already answered

**Type**: Incomplete coverage
**Spec Reference**: §4.2 and Corrigendum 2026-09-19 (a pane that has been answered is handed no second shell; the recovery step reads the pending marker, does nothing for a pane no longer carrying one, and treats a failed read as still pending)
**Plan Reference**: Phase 4, **Acceptance**, ninth bullet (`planning.md`)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The recovery step that rescues an abandoned pane runs on every waiting pane's chain, answered or not, so what it does for a pane the user already resumed is half of its contract — and the half that a corrigendum had to add, because getting it wrong means a resumed pane falls into a second shell and the user has to type `exit` twice to close it, a regression against how restored panes behave today. Phase 4's acceptance states only what the step does for a pane that was never answered. A phase signed off against those criteria has verified the rescue and not the discrimination that makes it safe to run.

**Proposal**:
Add the answered-pane half beside the abandoned-pane half in the phase's acceptance, in the terms the corrigendum states it. The behaviour is decided and task 4.4 already builds it; this puts it where the phase is signed off.

**Current**:

`planning.md`, Phase 4, **Acceptance**, ninth bullet:

```
- [ ] A waiter torn down with its pane exits; one that exits without handing the pane over leaves the pane on its own transcript, clears the marker, records a WARN if that clear failed, and execs the user's shell either way.
```

**Proposed Text**:

```
- [ ] A waiter torn down with its pane exits; one that exits without handing the pane over leaves the pane on its own transcript, clears the marker, records a WARN if that clear failed, and execs the user's shell either way.
- [ ] A pane that was answered is handed no second shell: the chain's tail reads the pending marker, does nothing at all for a pane that no longer carries one, and treats a read it could not take as still pending — so a resumed pane still closes on the first `exit`.
```

**Resolution**: Pending
**Notes**:
