# Review Tracking: Killing All Sessions Wipes Restore State - Input Review

## Findings

### 1. The panel's draw and the waiter must catch SIGTERM, never ignore it

**Source**: investigation `killing-all-sessions-wipes-restore-state.md` — Fix Direction → Chosen Approach, part 2 ("The handling must be a caught trap, never an ignored disposition — the parked chain's existing rule, since an ignored signal survives exec and the hook program and the user's shell would inherit it"; "So the waiter, like the parked shell, outlasts SIGTERM with the panel up and the marker set"); Testing Recommendations ("Restored eager and lazy resume-hook panes survive a SIGTERM to the pane's top process, while the hook program and the user's shell still receive SIGTERM with default handling (the trap is not inherited as an ignore)")
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §3.2 A waiting pane stays waiting through the shutdown signal; §6.2 Panes

**Problem**:
The specification requires the panel's draw and the waiter to outlast SIGTERM, but it states the caught-never-ignored rule only for the two shell chains. The draw and the waiter are Portal's own binary, and the obvious way to make a program outlast SIGTERM is to ignore the signal. When the user answers the panel, the pane goes on by exec to run its hook command and then the user's shell. An ignored SIGTERM survives that exec, and a non-interactive `sh` cannot trap a signal it was started with ignored. So the user's resume program and the shell after it would run with SIGTERM ignored for their whole life. The user would notice it as a resume program that `kill` no longer stops. The TERM trap this fix adds to the hook chain would also do nothing on every lazy pane answered from its panel. Lazy is the shipped default, so that is most restored panes.

**Proposal**:
Apply the source's rule to the draw and the waiter too: they outlast SIGTERM by catching it, never by ignoring it, so an answered pane's hook program and shell start with default SIGTERM handling. The source settles this. It gives the rule as the reason the parked chain traps rather than ignores. It requires the waiter to outlast SIGTERM "like the parked shell". And its testing recommendation requires default handling for lazy panes' hook program and shell as well as eager ones. Add a matching test for a lazy pane answered after its waiter caught a SIGTERM.

**Current**:
§3.2:
> So every process the waiting pane runs while it waits outlasts SIGTERM: the panel's draw, and the waiter it becomes. The panel stays up and the marker stays set. The pane is saved still waiting, with its transcript. After the reboot it comes back still asking, which is what lazy resume already does for an unanswered pane.

§6.2 (first bullet, unchanged; the new bullet follows it):
> - Restored eager and lazy resume-hook panes survive a SIGTERM to the pane's top process. The hook program and the user's shell still receive SIGTERM with default handling, so the trap is not inherited as an ignore.

**Proposed Text**:
§3.2:
> So every process the waiting pane runs while it waits outlasts SIGTERM: the panel's draw, and the waiter it becomes. The panel stays up and the marker stays set. The pane is saved still waiting, with its transcript. After the reboot it comes back still asking, which is what lazy resume already does for an unanswered pane.
>
> The draw and the waiter outlast SIGTERM by catching it, never by ignoring it, under the same rule as the parked chain (§3.1). An ignored signal stays ignored across `exec`, and an answered pane goes on to run its hook program and then the user's shell, which keep default SIGTERM handling.

§6.2, new bullet after the first:
> - A lazy pane whose waiter caught a SIGTERM, and which the user then answers on its panel, runs its hook program and the user's shell with default SIGTERM handling.

**Resolution**: Pending
**Notes**:

---

### 2. The ban on new log components and attribute keys is a decision no source made

**Source**: No source decides this. Checked: investigation `killing-all-sessions-wipes-restore-state.md`, all sections. Fix Direction part 3 and Testing Recommendations set the dropped-session line at INFO, by name, and say the back-off and refused-write lines exist. They say nothing about which log component or attribute keys those lines use.
**Category**: Unsourced decision
**Move**: route
**Affects**: §4.2 Backed-off saves and refused empty writes are logged

**Problem**:
The specification says: "Each uses the existing log component and attribute vocabulary, with no new component or attribute key." Portal's log taxonomy is closed. A new component or attribute key can only come from a specification amendment. So this sentence is the governing decision, and it rules that amendment out before anyone has checked whether the existing vocabulary can carry what the new lines need. The main need is each dropped session's name, on the component the commit logs under. If no existing key fits, the implementer must either break the specification or put the names into free message text. The user then gets a weaker reading of the post-reboot log, because finding which sessions were dropped during shutdown depends on those lines. That reading is the evidence the fix relies on to decide whether the deferred hold is needed.

**Proposed Text**:

**Resolution**: Pending
**Notes**:

---

## Observations

- The hydrate helper is a pane's top process while it replays scrollback. It is not a shell and dies on SIGTERM, so a reboot landing within seconds of a restore could close a session while tmux still answers. The sources do not raise it, and the window is very narrow.
- The investigation records that the waiting-pane transcript carry is skipped on the empty-listing path. Under the confirmation rule an empty listing from a dying server no longer reaches a commit, so the specification needs no separate rule for it.
- E5's 20ms trials that kept only one session came from per-session reads refused mid-capture. The confirmation rule already covers this generically, but the specification's list of what the confirmation catches names only empty listings.
