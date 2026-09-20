# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. The README still tells the reader a hook entry is a command string

**Type**: Incomplete coverage
**Spec Reference**: §3.2 (the stored registration — "An event's value is either the command as a string or an object carrying the command alongside its settings"; "This is a genuine change to the on-disk shape, not an additive field")
**Plan Reference**: Phase 1, task `lazy-resume-on-attach-1-2` (hooks.json accepts the object form and preserves what it did not write)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
`hooks.json` is a file the user hand-edits, and the README's configuration table is the only place Portal tells them what is in it. That row says the file is `pane → event → command` — strings all the way down. After this feature an event's value is either a command string or an object carrying `command` alongside its settings, and both shapes are permanently valid with no migration between them. A user who opens `hooks.json` after pinning a registration finds an entry the documentation says cannot exist, with nothing telling them the expanded shape is correct and permanent rather than corruption to be tidied back into a string.

Every other place the new shape falsifies is corrected by the task that falsifies it — CLAUDE.md's `hooks` row in this same task, the `prefs.json` README row in task 1.5, the `hook` section and the `hook list` line in tasks 1.6 and 1.7. This one row is the gap.

**Proposal**:
Add the README edit to task 1.2's **Do** list, beside the CLAUDE.md edit already there — the task that changes the stored value's shape is the task that falsifies the row. The wording is taken from the specification's own statement of the rule (§3.2): both shapes permanently valid, neither deprecating the other, the string form written whenever there is nothing to carry so a hand-edited file keeps looking as it does today. The rest of the row — the `hooks.json.lock` sentence — is untouched, and no acceptance criterion is added, matching how tasks 1.5, 1.6 and 1.7 carry their own README edits.

**Current**:
```markdown
- Edit the `hooks` row of CLAUDE.md's package table where it says the store holds per-pane on-resume commands: an event's stored value is a `Registration` decoded from either a JSON string (the command alone) or a JSON object (`command` plus `resume`), both shapes permanently valid with no migration between them; an entry a mutation did not name is re-emitted from the bytes it was decoded from, so an attribute the reader does not model and a `resume` value it cannot make sense of both survive a sibling's rewrite; and a value that is neither string nor object never fails the load for its neighbours.
```

**Proposed Text**:
```markdown
- Edit the `hooks` row of CLAUDE.md's package table where it says the store holds per-pane on-resume commands: an event's stored value is a `Registration` decoded from either a JSON string (the command alone) or a JSON object (`command` plus `resume`), both shapes permanently valid with no migration between them; an entry a mutation did not name is re-emitted from the bytes it was decoded from, so an attribute the reader does not model and a `resume` value it cannot make sense of both survive a sibling's rewrite; and a value that is neither string nor object never fails the load for its neighbours.
- Edit the `hooks.json` row of the README's configuration table, where it describes the file as `Per-pane resume hooks (pane → event → command)`: an event's value is either the command as a string or an object carrying the command alongside its settings. Both shapes are permanently valid, neither deprecates the other, and there is no migration between them — the writer picks by whether there is anything to carry, so an entry that is only a command stays a plain string and a hand-edited file goes on looking as it does today. Leave the rest of that row — the `hooks.json.lock` sentence — exactly as it is, and leave every other row of the table alone.
```

**Resolution**: Pending
**Notes**:

---
