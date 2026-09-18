# Specification: Lazy Resume On Attach

## Specification

### 1. What This Feature Changes

Portal restores every saved session on each reboot, and every restored pane fires its registered resume hook as part of coming back — whether or not the user ever looks at that pane. The firing is unconditional once scrollback replay finishes: the hydrate helper looks the pane's baked hook key up in the store and, on a hit, execs `sh -c '<HOOK>; exec $SHELL'` (`cmd/state_hydrate.go:172-196`). A restored pane is either a running process or a plain shell — there is no third state.

The cost is memory, and it scales with the saved-session count rather than with what the user is working on. Measured on a 64 GB M1 MacBook Pro on 2026-08-21: 42 live Claude processes, 13.1 GB resident between them, averaging 318 MB each and peaking at 702 MB, with the pageout counter over a million. Every new piece of work adds a session that is resumed on every subsequent reboot, indefinitely, regardless of whether it is still active work.

**The feature introduces the third state.** A restored pane whose resume is lazy comes back holding a panel that states what is about to be run and waits for the user to answer it. The live process behind the pane starts when the user says so, not at boot.

**Everything else about restore is unchanged.** Skeleton, geometry and scrollback replay exactly as today, so the pane still reads as restored — the transcript the user left there sits in the pane's primary buffer the whole time the panel waits, and is revealed the moment the panel goes.

Figures about the install are point-in-time, not constants. Measured across three sittings: 44 live sessions against 41 registered resume hooks (2026-09-17), 42 against 41, then 43 against 40 (2026-09-18). No figure supersedes another. The shape they agree on is one session, one pane, one registration — 43 of the 44 live sessions held a single pane (`tmux list-panes -a -F '#{session_name}' | sort | uniq -c | awk '{print $1}' | sort -n | uniq -c` → `43 × 1 pane, 1 × 2 panes`, 2026-09-17) — and a set that grows over months.


### 2. Resume Modes

**Every registration resolves to one of two modes at restore time: eager or lazy.** Eager is today's behaviour — the hook fires as soon as scrollback replay finishes, with no panel and no wait. Lazy holds the hook behind the waiting panel (§5), and the pane's process starts only when the user answers.

**The mode comes from an install-wide default with a three-state per-registration override.** A registration carries eager, lazy, or nothing. Nothing means inherit — the entry follows the install and changes with it. An entry that sets either mode holds that choice regardless of what the install says. A user who wants one particular resume to always come back automatically sets it eager; one who wants a particular resume to always ask sets it lazy; everything else is governed centrally.

The finer model was taken rather than bet against. The store holds arbitrary user-authored commands, and a thing the user walks up to behaves very differently under a panel that waits forever than a thing that needs to be *up* whether or not anyone looks at it — a dev server, a tunnel, a watcher. Every registration on the measured install is the first kind, so the install does not settle whether the second case exists; the override covers it at a cost small enough that betting was the worse trade.

#### 2.1 The install-wide default is lazy

The feature ships on rather than waiting to be discovered. An install that upgrades and reboots meets panels rather than processes.

Defaulting eager was weighed and rejected. It is the conservative choice — an upgrade changes nothing, and the new behaviour is opted into — but it leaves the feature switched off behind a setting with no UI (§3.1), which is a poor place to leave the thing the work exists to deliver. What makes lazy safe is that it is not a silent change: a pane holding a panel says what it is and what key answers it, so the worst first-boot outcome is a few extra keypresses landing the user exactly where eager would have put them.

#### 2.2 A registration is written whole

**Nothing survives a re-registration it was not given.** `portal hook set` writes exactly what it is handed. A mode the caller does not pass is not a mode — no attribute from the entry being replaced is carried forward, and the store's existing wholesale overwrite of an event's value is the correct behaviour rather than something to work around.

The alternative — that a `hook set` carrying no mode flag should leave an existing mode alone — was rejected on the model rather than the mechanics. A registration replaces its predecessor completely; inheriting configuration from a dead one means a writer's output depends on state it never saw, which is the behaviour nobody can account for when it surprises them later. Policing that is not Portal's job.

The consequence is stated rather than mitigated: **a pin lives on the registration, not on the pane.** Anything that re-registers without the flag drops it, and that is the contract rather than a defect. The practical exposure is small: the external Claude Code `SessionStart` hook writes only for panes running a session, its `SessionEnd` counterpart usually removes the entry first, and the one collision — a pinned pane whose hook is re-registered — resolves by re-passing the flag.


### 3. Storing, Setting and Reading the Mode

#### 3.1 The install-wide default

`prefs.json` holds the install's UI preferences — the theme and the session-list grouping mode — and is where this one lives, as the key **`resume_mode`**, holding `eager` or `lazy`. It decodes tolerantly and independently like every other field there: missing, empty, corrupt or unrecognised gives the shipped default (§2.1), and the key is `omitempty` on write, so an install that never set it carries no key.

**There is no surface for changing it.** The theme picker is the only preference with a UI, so until a settings screen exists this one is changed by hand-editing the file. That raises the stakes on the default rather than changing where it lives. A settings screen is parked on the roadmap (§10).

#### 3.2 The stored registration

**An event's value is either the command as a string or an object carrying the command alongside its settings.** The object's mode attribute is `resume`, holding `eager` or `lazy`:

```json
{
  "0gT5gC": { "on-resume": "cd \"/Users/leeovery/Code/flowx\" && claude --resume 45604077-…" },
  "2zyjmp": { "on-resume": { "command": "cd \"/Users/leeovery/Code/nod\" && claude --resume d89f89ab-…",
                             "resume": "eager" } }
}
```

**Neither shape is legacy and neither deprecates the other.** The string form is the primary shape for a registration that is only a command; the object form is the primary shape for one that carries configuration. Both are permanently valid, there is no migration, and there is no future pass that converts one into the other. The writer picks by whether there is anything to carry.

That rule is load-bearing rather than cosmetic. The external Claude Code `SessionStart` hook fires `portal hook set` on every session start, and each call rewrites the whole file — so a writer that always emitted the object form would convert every entry on the install to the verbose shape on the next session start. Emitting the string form when there is nothing to carry keeps a hand-edited file looking as it does today, with the occasional expanded entry where something has been pinned. The object is open-ended, so it is room for whatever comes later rather than a slot cut for this one setting.

This is a genuine change to the on-disk shape, not an additive field. `hooks.json` is `map[hook_key]map[event]command` — strings all the way down, with no slot for an attribute that is not a command (`internal/hooks/store.go:28-31`).

**The alternative — a second entry beside `on-resume` in the same inner map — was rejected.** It keeps a `jq` reader working, but it puts a non-event in the event namespace and adds a row to `hook list`, which is a machine interface an external script parses.

**The out-of-repo consumers were checked before the shape was chosen.** `~/.claude/hooks/portal-resume-hook.sh:125` reads `portal hook list` and filters on the event column rather than parsing the file, and that output does not change shape — the command lands in the same column out of either form. `~/.claude/hooks/portal-resume-backfill.sh:71` does parse the file with `jq` and would read an object where it expects a string — but it looks entries up by the pre-token `session:window.pane` key and so already matches nothing on the current install; the user confirms it is no longer used, having been a backstop for a defect since fixed. Measured 2026-09-18: all 41 keys in the live file are token-shaped, none old-format, and `portal doctor` reports no stale hooks across 7 passing checks.

#### 3.3 Setting the override

**The override is set where the registration is made: `portal hook set`, as the flag `--resume-mode eager|lazy`, alongside `--on-resume`.** This follows from who writes registrations — the external `SessionStart` hook is a shell script, not a person at a screen, so the route has to be something a script can pass.

The waiting panel is not the place for it. A panel offering "always resume this one without asking" would be setting a durable preference from a surface whose whole job is answering one instance of a question.

#### 3.4 Reading it back

**A pinned registration is readable from `portal hook list`, as a fifth column.** A mode that could only be seen by opening `hooks.json` would be configuration you can set and cannot check.

Today the output is four tab-separated columns — key, event, command, location (`cmd/hooks.go:151`). The mode is **appended** as a fifth rather than inserted, so anything reading the first four positionally is untouched; this is how the location column itself arrived. The cell holds `eager` or `lazy` when the registration carries one and is **empty when it does not**, matching how the location column already reads when it has nothing to say. So the column reports what is stored, and an empty cell means the entry follows the install.

**The install-wide default is deliberately not in that listing.** It is one value for the whole install rather than a property of any row, and the only place to put it is a header or footer line — which breaks naive parsers of a machine interface for a fact that does not vary between rows.

**It is reported by `portal doctor` instead, as a passing informational line** carrying the install's resume mode. Doctor already reports on this machinery, and this shape already exists there (`checkInfo`, `cmd/doctor.go:345-350`). Like the pending count (§8.1), it never fails the check and never changes the exit code.

### 4. The Waiting Pane

#### 4.1 A waiting program in the pane

**A pane whose resume is lazy holds a live Portal process that draws the panel and blocks until the user answers.** The helper restore already puts in each pane does not finish: it lays down the scrollback as today, then — instead of handing the pane to the hook — draws the panel and waits.

The alternative was a **dead pane**: the helper paints the panel and exits, tmux holds the pane open with no process in it, and Enter and the discard key are tmux key bindings that restart the pane with the right payload. It costs nothing at rest, which is the shape this feature's own thesis argues for — a cost proportional to a set that only grows is the problem the work exists to remove.

**It was ruled out because tmux cannot capture a key at pane granularity.** `key-table` is a session option, not a pane option. tmux accepts `set-option -p -t <pane> key-table <name>` without complaint and resolves it upward: set on one pane of a two-pane session, it reads back set on the session *and on the sibling pane that was never named*. Verified on tmux 3.7c — before the write all three scopes report the option unset; after it all three report the custom table. And a custom key table swallows every key, not only its bound ones: with the table active, Enter fired the binding and the word typed after it never reached the pane's process at all.

So arming one waiting pane's keys arms every pane in its session, and locks them. In the worked example — a left pane waiting and a right pane holding a shell the user wants to work in — the right pane's keyboard would be dead. **A waiting pane must never block the panes beside it**, which is the same constraint that eliminated every floating overlay (§5.1).

One route survived for the dead pane and is worse: bind Enter and Escape in tmux's **root** table globally, each wrapped in a conditional that fires Portal's command when the focused pane is waiting and passes the key through otherwise. That is Portal permanently rebinding two keys across the user's entire tmux server, with a conditional evaluated on every press — Escape most of all, which every modal program on the machine depends on. Not a trade worth making for a memory curve.

**A process in a pane reads the keys sent to it and nothing else.** The scoping the design needs is a property of processes, not something tmux has to provide. The choice was forced rather than preferred, and the reasoning that favoured the dead pane still stands on its own terms; it rests on a capability tmux does not have at pane granularity.

#### 4.2 The waiter is the Portal binary, and it hands off before it waits

**The resident cost is the runtime floor, not the binary.** Go pages in lazily, so linking a library costs disk rather than memory. Measured on the target machine: a Go binary linking the rendering library and never touching it, blocked on a read, is **1680 KB** resident — indistinguishable from one importing nothing at all at **1696 KB** — and 4.8 MB on disk. The 22 MB the daemon carries is the cost of doing work, not of existing (`ps -o rss= -p $(pgrep -f '^portal state daemon')` → `17744` KB on 2026-09-17, `22496` KB on 2026-09-18).

**So no separate binary is warranted: the waiter is the same Portal binary entered on a path that does almost nothing.**

**The process that draws must hand off to a fresh one before waiting.** Drawing touches the theme and the rendering path, and those pages stay resident for that process's life. Drawing and then blocking in the same process would carry all of it for as long as the pane waits. Drawing and then replacing the process image with a minimal wait puts the resting state back at the floor. The helper already ends in exactly that kind of handover (`ExecShell`, `cmd/state_hydrate.go:40`), so this is the shape the code is already built around, not a new one.

**A resize is the same handover run backwards**: the waiter replaces itself with a fresh draw, which draws at the new width and hands back to a fresh wait. The resting state stays at the floor and the cost is paid only at the moment of the resize.

Two figures the memory case rests on are estimates rather than measurements, because they cannot be taken until the code exists: the waiter's actual resident size on the settled path (~2 MB is the measured Go floor plus a guess at what Portal's startup touches), and whether the draw-then-hand-off split holds it at that floor in practice. Both are bounded — the floor is measured above and the ceiling is the daemon's 22 MB — and neither changes a decision. At the estimate, a full waiting set of 41 costs some 80 MB against the 13.1 GB the eager path was measured at (§1).

#### 4.3 Enter and `d` act; everything else is swallowed

**The waiter is the pane's only process, so anything that kills it takes the pane with it** — Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit. The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot answer the panel, because nothing but Enter and `d` means anything to it.

**That refusal covers what a person at the keyboard can send, and stops there.** When tmux tears the pane down — the user kills the session, closes the window, or the server shuts down — the waiter exits. It does not decline the hangup, and a closed terminal ends it.

An unbounded refusal would outlive the destruction of its own pane: culling fifteen finished sessions from the picker would leave fifteen Portal processes running with nothing to attach to, reinstating the resident cost this work exists to remove, on the cleanup path. Neither bootstrap's marker sweep nor the tmux server's own exit reaps a process that has refused the hangup. The swallow rule's purpose is that nothing accidental can *answer* the panel, and pane teardown is not an answer.

#### 4.4 Nothing triggers the panel

**The panel is drawn once, at restore, by the machinery that already runs for every pane.** No focus hooks, no attach hooks, no per-event rendering. `client-attached` and `client-session-changed` are not touched by this feature.

There is nothing to trigger because the panel does not have to be produced when the user arrives — it is already there, held on the pane by the process waiting in it. Arriving at a pane is not an event Portal needs to observe.

**Stickiness is the absence of a dismissal path rather than a feature.** Ignoring the panel changes no state, so it is still there next time. Detaching, closing the window, and rebooting are not dismissals, so none of them clear anything — and a reboot restores the pane and draws the panel again from the still-unfired registration. Only Enter and a confirmed discard change anything (§6).

### 5. The Resume Panel

#### 5.1 Painted into the pane's alternate screen, not floated over it

**Nothing floats above the pane and nothing tmux draws is involved.** The panel is Portal drawing into a surface it owns, exactly as the picker's own modals are.

**tmux's overlay primitives were measured and rejected.** `display-popup` produced exactly the described panel on a disposable socket — explicit dimensions, a border style, a title, a body of Portal's own drawing — and then failed the one hard constraint. **A tmux overlay captures the whole client's keyboard.** Measured on tmux 3.7c: with a two-pane window, the *right* pane selected as the active pane, and a popup opened over the *left* pane, keystrokes sent from the attached client were delivered to the popup and never reached the focused right pane at all. A popup is not a pane; tmux's own description is "a rectangular box drawn over the top of any panes", and it intercepts the client's input ahead of pane routing. `display-menu` is the same kind of object, and additionally renders a list sized to its contents with no room for an arbitrary body.

**There is no pane-scoped overlay facility in tmux.** Both overlay primitives are client-scoped by construction, and nothing else in the command set draws over a pane — the remaining candidates put text in the pane's own content (`remain-on-exit-format`, pane borders) or in the status line. An overlay that floats above one pane while the others stay live is not something tmux offers. This is the same constraint that forced the waiting program over the dead pane (§4.1).

**The panel is painted into the pane's alternate screen, so it never enters the scrollback.** The panel is not user content — it is a hold placed on the session, never asked for — and once it is gone it should leave no trace in the history. The alternate screen is exactly that facility: the buffer `vim` and `less` draw on, which is not added to a pane's scrollback ring.

Measured on tmux 3.7c: a pane printed two lines of real content, entered the alternate screen, and painted a card. `capture-pane -p` returned the card; `capture-pane -a -p` returned the two original lines, intact underneath. The isolation is a property of the alternate screen and does not depend on whether a process remains — re-measured with a live process holding it open, the same isolation holds (§7.1 records what the saver's own invocation returns, which is a different reading of the same pane).

**Nothing blocks.** The panel is the pane's own content and the program holding it reads only the keys sent to that pane. Panes beside it are fully live throughout.

**Panes with no pending resume are untouched.** In the worked example — one window, a left pane that held a resumable session and a right pane that held a bare shell — the right pane restores exactly as it does today, its scrollback in place, and goes on capturing normally. Work done in it during one attachment shows up in its scrollback on the next, unchanged by this feature.

#### 5.2 A full-pane canvas with a card centred on it

**The overlay fills the pane, painted in the active Portal theme so nothing behind it shows through, and the decision sits in a compact bordered card in the middle.** A box stretched to near-full size with a few lines in the middle would look lost on a large terminal; the canvas-plus-card shape is what Portal already uses everywhere else for exactly this reason.

The card is the shape of Portal's existing rename modal: a header row carrying the title and a state badge, a body, and a footer row of key hints, assembled through the same joined-panel frame the picker's modals use (`renderJoinedPanel`, `internal/tui/panel.go`).

**The reuse is at the presentation layer, not the code path.** The picker's modals are Bubble Tea components rendering into its own model, while this panel is drawn straight into a pane by the program that then waits there. What carries across is the theme tokens and the modal's visual grammar, which is what makes it read as Portal rather than as a tmux dialog.

Every colour is a theme token, as everywhere else in Portal — the panel holds no raw hex.

#### 5.3 The waiting panel

It carries this, and carries nothing else:

- **Header** — `Resume session` on the left; a `● PAUSED` badge on the right in `accent.attention`, occupying the slot the rename modal gives `◉ EDIT MODE`.
- **Body** — an `ON RESUME` label in `accent.primary`, the token the rename modal gives `NEW NAME`, with the registered command beneath it.
- **Footer** — `⏎ resume` and `d discard`.

A meta line carrying the directory and how long the pane had been paused was drafted and cut. It was invented rather than decided, and on the page it added nothing the command and the badge did not already say. The pending marker can carry metadata (§8.2) — this panel does not need it to.

#### 5.4 The discard confirmation

**The discard confirmation is the kill modal, retitled.** `▲ Discard resume?`, the command rendered in `state.destructive` where the kill modal puts the session name, a plain-language consequence line, and `y discard   esc cancel`. Nothing structural differs, which is the point: it is the same act the picker's kill confirm performs, so it is the same object, built through the same shared destructive-confirm builder (`internal/tui/destructive_confirm.go`).

#### 5.5 Design references

Three frames were built in the Paper file `Portal`, against the Nord artboards the user runs, so the new work sits beside the existing designs in the same palette:

- **Resume panel — waiting (Nord)**
- **Resume panel — discard confirm (Nord)**
- **Sessions — pending resume dot (Nord)** (§8.3)

The waiting panel's frame was built by duplicating the Nord kill modal, so its card geometry — width, header and footer rules, padding — is identical to the existing modals' rather than approximate. The frames are the design reference for implementation; the panel is built from Portal's own shared panel machinery, not from the frames' pixel dimensions.

### 6. Answering the Panel

Two keys change anything. Everything else is swallowed (§4.3).

#### 6.1 Enter resumes

**Enter hands the pane over in place; nothing is wiped and nothing is re-laid.** The replayed transcript is already in the pane's primary buffer, underneath the alternate screen the panel is drawn on, so leaving the alternate screen reveals it and the resume command starts over it. The pending marker is cleared before the hook runs (§7.2).

**The command is read at the moment the user answers, not carried from when the panel was drawn.** The waiting program looks the registration up before it draws, to know whether to draw at all — and it reads it again when Enter is pressed, and runs what the store holds then.

A pane can wait for days, and in that time the entry can be removed by `portal hook rm --pane-key`, rewritten by a re-registration, or hand-edited. Acting on a value read days earlier would resume something the user had already deregistered — the one case where the two readings differ, and the one where the stale reading is plainly wrong. Re-reading costs a single file read at a moment already doing far more.

**An entry that has gone by then drops the pane through to a plain shell**, exactly as an unregistered pane does — a path the helper already has. So does an unreadable store: the existing degradation is that a lookup failure gives a bare shell so the pane stays usable, and that is unchanged. The marker clears either way; the pane is no longer waiting whatever the read returned.

#### 6.2 `d` discards, behind a confirmation

**`d` opens a second confirmation over the panel** — *this is permanent* — where **`y`** agrees and **Escape** backs out to the resume panel (§5.4).

**A confirmed discard removes the pane's resume registration, permanently, and nothing else.** The entry is cleaned out of the store rather than suppressed for the boot, so the panel does not return on this boot, on the next attach, or after any future reboot. The pending marker is cleared and the pane falls through to a plain shell with its replayed scrollback still above it — indistinguishable from a pane that never had a hook.

**The tmux session is untouched.** It stays live, stays saved, and restores on the next reboot as an ordinary hookless pane — bare shell, scrollback intact, no panel. Killing it is a separate act the user takes when they want it.

Retiring the whole session on discard was considered and rejected. It would answer the other half of the problem — that the saved population only ever grows — and the multi-pane hazard that argues against it is rare (43 of 44 live sessions held a single pane, §1). It is rejected on intent rather than on that hazard: declining a resume and disposing of a session are two different decisions, and binding them to one keystroke removes the user's ability to make only the first.

Per-boot decline and re-offer-on-next-attach were both rejected for the opposite reason. Pressing the discard key is an act of disposal, not deferral — "I've decided, actually, I don't need that session" — and an offer that comes back after you have declined it is treating a decision as a hesitation.

**Discarding neither unstamps the pane's durable token nor touches any other entry.** It is a third removal route alongside `portal hook rm` and a hand edit of the store, reached from where the user already is instead of by remembering a CLI verb.

#### 6.3 Escape on the waiting panel does nothing at all

There is nowhere to back out to, so it is inert. Escape is live only inside the confirmation `d` opens, where it backs out.

**That is the point rather than a side effect.** Everywhere else in Portal, Escape means *back out* — it reverses, it never acts. Binding it here to an irreversible deletion would make this the one place in the product where the reflex key destroys something. Assuming the positive instead — Enter resumes, a named key discards behind a confirmation — makes Escape mean exactly one thing everywhere, with no site where it also destroys. A key the user's hands press without consulting them can then never be the key that loses work. An inert Escape reads as cleaner than a dangerous one.

**The discard key is `d`, derived from the picker's existing split** rather than chosen fresh: `k` kills a live thing (a session), `d` deletes a persisted record (a project). Nothing is killed here — the session and the pane both survive — and what goes is a stored registration, which is `d`'s side of that line.

**The confirm key is `y`, matching Portal's two existing destructive confirmations.** Killing a session and deleting a project both take `y` with `esc` to cancel, through one shared builder (`internal/tui/kill_modal.go:13`, `delete_modal.go:12`, `destructive_confirm.go:16`). Enter was rejected for it: Enter resumes on the panel one keystroke earlier, so confirming the discard with it would make the same key mean "bring it back" and "delete it forever" on consecutive screens.

#### 6.4 What the discard destroys is recorded

**The removed command is written to the log as it goes.** The confirmation makes the act deliberate; the log line makes it recoverable anyway, and costs nothing.

Portal already destroys these entries two ways and treats them differently. The typed removal command records only which entry went (`op=rm`, `internal/hooks/store.go:221`), while the automatic stale sweep deliberately records the command itself, its own source comment calling that the recoverable form an operator copies back out of the log (`op=clean-stale` carrying `value`, `internal/hooks/store.go:369-373`). This path is the sweep's situation rather than the typed command's — it is reached by a keystroke on a panel the user is walking through, not by naming a key on a command line — so it takes the sweep's treatment.

The emission is one INFO line under the `hooks` component carrying the hook key and the removed command as `value`, at the production default level. It adds **two members to closed vocabularies**, amended here because a specification is the sanctioned route and a call site is not:

- a new `op`, **`discard`**, so the three removal routes stay greppable apart;
- a new `via`, **`panel`**, because the existing four (`cli`, `internal`, `hydrate`, `doctor`) name none of them — the waiter is neither a typed command, nor Portal acting on its own behalf, nor a hydration lookup, nor a diagnosis.

*Sibling check: `resume-hooks-silently-lost` owns hook removal — the `hook rm` CLI, the rule that removing nothing always exits non-zero, and that a removal never unstamps the pane's durable token. This section adds a route beside that CLI and contradicts none of those rules; because the key it removes is always a token baked from saved state, it also never touches the old-format entries that specification retains permanently.*

### 7. Protecting the Waiting Pane's Saved Transcript

#### 7.1 An unfrozen waiting pane accretes dead panels into its saved history

Portal's saver re-reads every live pane on each tick and rewrites that pane's saved scrollback whenever the capture hashes differently from the last write (`cmd/state_daemon.go:263-290` — `CaptureAndHashPane` then `WriteScrollbackIfChanged`, which is a dedup, not a protection).

The saver reads a pane with `capture-pane -e -p -S -` (`internal/tmux/tmux.go:752`). Measured against that exact invocation on tmux 3.7c, with a live process holding the alternate screen open over a pane carrying 40 lines of prior output: the capture returns the **primary buffer's history with the alternate screen appended at the tail** — the real transcript, then the card's lines.

So an unfrozen waiting pane hashes differently from its last write and the saver rewrites its saved file as *transcript plus a picture of the card*. Once, not per tick — identical captures dedup. The transcript is not lost, which is milder than a replacement, but it is not recoverable either: the next reboot replays that file, so the dead card image returns as part of the pane's history and a live card is drawn over it. A pane waited on across three reboots accretes three dead cards into a transcript that can never shed them.

An earlier reading of this used a bare `capture-pane -p`, which returns the visible screen alone; that is why the card first looked like a wholesale replacement rather than an addition. **Measuring with the caller's exact invocation is what changes the answer**, and the decision below rests on the second reading.

#### 7.2 The pane stays frozen for as long as its resume is unanswered

**The marker that already tells the saver to leave a pane alone is held through the waiting state** instead of being cleared at the end of scrollback replay. Today the hydrate helper clears it the moment replay finishes and before it hands off — replay, settle sleep, unset, exec (`cmd/state_hydrate.go:139-148`) — which under lazy resume would land at exactly the moment the panel goes up.

**It is cleared when the user answers** — on Enter before the hook runs, and on a confirmed discard before the pane falls through to a shell (§6).

The cost is nil in practice: a pane waiting on a resume has no new content worth saving, so freezing it at its last live state is exactly the desired end state.

**A waiting pane keeps its place in the saved set throughout.** The freeze suppresses that pane's scrollback write alone; the structural capture still enumerates the pane into `sessions.json` and merges its *previous* record back into the fresh index — guarded so a stale marker cannot resurrect a pane whose session, window or pane is gone (`internal/state/capture.go:96-127`). So a pane can wait indefinitely and still be restored on every subsequent reboot, with its original content and a fresh panel, however many reboots it waits through.

**The freeze is load-bearing, not insurance.** Without it a waiting pane's saved history silently accretes junk for as long as it waits.

#### 7.3 The freeze is held by the pane, not by the pane's position

The marker that suppresses capture today is addressed positionally — session name plus window and pane index. The saver recomputes that address for every live pane each tick and skips only on an exact match (`cmd/state_daemon.go:263-273`), and bootstrap's stale-marker sweep unsets any marker no live positional address answers to, enumerating through the same positional format (`cmd/bootstrap/stale_marker_cleanup.go`). All three components of that address move: closing an earlier window renumbers, `break-pane` and `move-pane` relocate, and a rename changes the session half.

Today that exposure is the few seconds between skeleton restore and handover, which is why it has never mattered. This feature stretches it to the whole waiting life. A pane waited on for a week, whose session is renamed or whose sibling window is closed, loses its protection twice over: the saver's skip stops matching and the capture lands, and the next `portal open` — which runs bootstrap, and which the user runs constantly — sweeps the marker away as stale. This is the failure `resume-hooks-silently-lost` already fixed once for hook keys, reappearing on a different marker.

**The saver's skip gains a second condition rather than changing its first.** A pane is left alone when it is mid-restore — the existing positional marker, whose other jobs are unchanged — **or** when it is waiting, read from a pane-scoped pending marker. That marker travels with the pane through every rearrangement tmux can perform, measured in `resume-hooks-silently-lost` against `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename.

**The marker is the pane user-option `@portal-resume-pending`, set to `1`** — the value convention the skeleton markers already use (`internal/state/markers.go:78`). Presence is what is read; any richer payload is a later change the shape already admits (§8.2).

Two consequences follow and are part of the decision:

- **The pending marker is set before the mid-restore marker is cleared** — and therefore before the panel is painted. A gap where neither is set is a one-tick window in which the saver writes the card into the pane's saved history: the whole failure, in the space between two steps.
- **The saver reads it for free.** It already enumerates every pane on the server each tick with a per-pane format to build its structural index, and that format already carries the pane's durable token as a column (`captureFormat`, `internal/state/capture.go:26`). The pending marker joins it as another column rather than costing a second tmux call. The arity of that read changes — `captureFieldCount` 11 → 12 — which is the same contained move the pane token made.

**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — is structurally hard to reach.** The marker lives on the pane, the waiting program is the pane's only process, and it dies with the pane. There is no state in which the pane survives while the marker is wrong.

*Sibling check: `built-in-session-resurrection` records the marker lifecycle as "Helper unsets marker after dump + 100ms sleep", which is true of the code as it stands and is what this feature changes. No corrigendum is owed: that text is not a claim that has gone wrong, it is current behaviour this work supersedes — and the same reading applies to that specification's rejection of a Zellij-style confirmation prompt, a decision superseded by new product intent rather than a factual error.*
---

## Working Notes
