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

**A stored value the reader cannot make sense of never fails a pane.** An object whose `resume` attribute is absent, empty, or holds anything other than `eager` or `lazy` carries no mode — the registration inherits the install-wide default exactly as a string-form entry does, and the mode column (§3.4) reads empty for it. An object carrying no command, or an empty one, is not a registration: the pane falls through to a plain shell as an unregistered pane does (§6.1). Nothing is rewritten to correct either case; the file stays as the user left it.

This is a genuine change to the on-disk shape, not an additive field. `hooks.json` is `map[hook_key]map[event]command` — strings all the way down, with no slot for an attribute that is not a command (`internal/hooks/store.go:28-31`).

**The alternative — a second entry beside `on-resume` in the same inner map — was rejected.** It keeps a `jq` reader working, but it puts a non-event in the event namespace and adds a row to `hook list`, which is a machine interface an external script parses.

**The out-of-repo consumers were checked before the shape was chosen.** `~/.claude/hooks/portal-resume-hook.sh:125` reads `portal hook list` and filters on the event column rather than parsing the file, and that output does not change shape — the command lands in the same column out of either form. `~/.claude/hooks/portal-resume-backfill.sh:71` does parse the file with `jq` and would read an object where it expects a string — but it looks entries up by the pre-token `session:window.pane` key and so already matches nothing on the current install; the user confirms it is no longer used, having been a backstop for a defect since fixed. Measured 2026-09-18: all 41 keys in the live file are token-shaped, none old-format, and `portal doctor` reports no stale hooks across 7 passing checks.

#### 3.3 Setting the override

**The override is set where the registration is made: `portal hook set`, as the flag `--resume-mode eager|lazy`, alongside `--on-resume`.** This follows from who writes registrations — the external `SessionStart` hook is a shell script, not a person at a screen, so the route has to be something a script can pass.

The waiting panel is not the place for it. A panel offering "always resume this one without asking" would be setting a durable preference from a surface whose whole job is answering one instance of a question.

**A mode is always passed with the command it belongs to.** `--resume-mode` on its own, with no `--on-resume`, is refused — the command exits non-zero and writes nothing. A registration is written whole (§2.2) and both stored shapes carry a command (§3.2), so there is no entry a mode could attach to by itself; pinning an existing registration means re-passing its command alongside the flag.

**A mode the command cannot recognise is refused with it.** `--resume-mode` takes `eager` or `lazy` and nothing else; any other value exits non-zero and writes nothing, so a mistyped pin fails where it was typed rather than landing on disk as a mode nothing reads. That is the writer's side and it does not soften the reader's: a value that reaches the file by hand edit still carries no mode and still never fails a pane (§3.2).

#### 3.4 Reading it back

**A pinned registration is readable from `portal hook list`, as a fifth column.** A mode that could only be seen by opening `hooks.json` would be configuration you can set and cannot check.

Today the output is four tab-separated columns — key, event, command, location (`cmd/hooks.go:151`). The mode is **appended** as a fifth rather than inserted, so anything reading the first four positionally is untouched; this is how the location column itself arrived. The cell holds `eager` or `lazy` when the registration carries one and is **empty when it does not**, matching how the location column already reads when it has nothing to say. So the column reports what is stored, and an empty cell means the entry follows the install.

**The install-wide default is deliberately not in that listing.** It is one value for the whole install rather than a property of any row, and the only place to put it is a header or footer line — which breaks naive parsers of a machine interface for a fact that does not vary between rows.

**It is reported by `portal doctor` instead, as a passing line** carrying the install's resume mode. Doctor already reports on this machinery, and this is the shape the pending count takes (§8.1) — a check that always passes, so it carries the `✓` and counts toward the total the summary line reports (`checkPass`, `cmd/doctor.go:400`). Like the pending count, it never fails the check and never changes the exit code.

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

**The redraw is taken once the size has settled, not once per size change.** A terminal dragged to a new size delivers a stream of size changes to every pane in the window, and a full waiting set answering each of them with a fresh draw would be hundreds of process launches a second for the length of the drag. The panel holds nothing that moves, so one draw at the end of the stream is the whole of what it owes, and the cost of a resize stays a single handover per pane.

**Every screen the pane shows takes that same handover.** A wait is not one draw: `d` puts up the confirmation, Escape brings the card back, and an answer that cannot be carried out redraws the card with its report row (§5.3, §6.2). Each is a fresh draw that hands back to a fresh wait, exactly as a resize does, so the process holding a pane between screens carries the wait and nothing else — and the resting cost of a waiting set does not depend on how many of its panes the user has stopped to look at.

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

**A pane too small for the card still says what it is.** Below the size the card needs, the panel degrades instead of disappearing: the canvas is painted as always, and the title, the command and the key hints stack plainly without the card frame, down to the smallest pane a restore can produce. Enter and `d` act at every size. A waiting pane swallows every other key (§4.3), so one that drew nothing would read as an ordinary restored pane with a dead keyboard.

The card is the shape of Portal's existing rename modal: a header row carrying the title and a state badge, a body, and a footer row of key hints, assembled through the same joined-panel frame the picker's modals use (`renderJoinedPanel`, `internal/tui/panel.go`).

**The reuse is at the presentation layer, not the code path.** The picker's modals are Bubble Tea components rendering into its own model, while this panel is drawn straight into a pane by the program that then waits there. What carries across is the theme tokens and the modal's visual grammar, which is what makes it read as Portal rather than as a tmux dialog.

Every colour is a theme token, as everywhere else in Portal — the panel holds no raw hex.

**The theme resolves as it does everywhere else in Portal.** A named theme paints from the first frame with no gate at all. A light/dark pair runs the same detect-or-timeout appearance gate the picker runs — a query to the terminal raced against the same short timeout (`appearanceDetectTimeout`, `internal/tui/appearance_gate.go:12`), resolving dark when there is no answer — in the process that draws. That process hands off before it waits (§4.2), so the gate is paid once per draw and nothing of it stays resident while the pane waits. A pane drawn with no client attached to it gets no answer and resolves dark — that fallback reached by the ordinary route rather than a second rule. Most panes are drawn at restore with nobody watching them, so under a pair the dark half is what most first draws land on; a redraw with a client present resolves against the terminal in front of it.

**`NO_COLOR` is the same carve-out here as everywhere else in Portal.** No canvas is painted, no appearance detection runs at all, and the panel renders colourless on the terminal's native foreground and background. Coverage does not depend on the fill: the panel sits on the pane's alternate screen (§5.1), so the transcript stays hidden underneath it whether or not a colour is painted over the pane. What the panel loses is hue alone — the card frame, the `● PAUSED` badge, the `ON RESUME` label and the key hints carry it on glyphs and words, and the discard confirmation keeps its `▲ Discard resume?` title and its `y discard   esc cancel` footer where the command loses `state.destructive`. That is the same rule the session row's indicators take (§8.3): state stays glyph-backed and never colour-only.

**Every string this surface renders is tool-agnostic.** Portal's resume machinery runs whatever command a registration holds, so nothing the panel shows names a particular tool — it states the command and says nothing about what the command is. That covers the discard confirmation's consequence line (§5.4) and the indicator legend that ships with the picker's pending dot (§8.3) as much as the panel itself.

#### 5.3 The waiting panel

It carries this, and carries nothing else:

- **Header** — `Resume session` on the left; a `● PAUSED` badge on the right in `accent.attention`, occupying the slot the rename modal gives `◉ EDIT MODE`.
- **Body** — an `ON RESUME` label in `accent.primary`, the token the rename modal gives `NEW NAME`, with the registered command beneath it.
- **Footer** — `⏎ resume` and `d discard`.

**A command longer than the card wraps rather than being cut.** The registered command is the only thing on the panel that says which piece of work the pane is holding, and a realistic one carries a directory and an identifier. It wraps within the card's inner width over at most three lines, with anything beyond marked `…`; the card's width is unchanged. The discard confirmation renders the command the same way (§5.4).

**A card with something to report carries one more row.** When an answer cannot be carried out, the reason is stated on a single line between the command and the key hints, and it stays there until the next key is pressed rather than timing out: a report the user can miss leaves them believing the thing they asked for happened. The row is present only when there is something to say, and a panel with nothing to report carries exactly the three parts above. The card carries the report for a freeze that will not lift (§7.2), on either answer. A discard the store will not accept never reaches the card: it is answered from the confirmation and reported there, on that screen's own row (§5.4), because a confirmation that closed on a failed write would be indistinguishable from one the user backed out of.

A meta line carrying the directory and how long the pane had been paused was drafted and cut. It was invented rather than decided, and on the page it added nothing the command and the badge did not already say. The pending marker can carry metadata (§8.2) — this panel does not need it to.

#### 5.4 The discard confirmation

**The discard confirmation is the kill modal, retitled.** `▲ Discard resume?`, the command rendered in `state.destructive` where the kill modal puts the session name, a plain-language consequence line, and `y discard   esc cancel`. Nothing structural differs, which is the point: it is the same act the picker's kill confirm performs, so it is the same object, built through the same shared destructive-confirm builder (`internal/tui/destructive_confirm.go`).

**The confirmation carries a report row too.** A discard the store will not accept (§6.2) is reported on the confirmation itself, on the same single line the waiting panel's card gives a report (§5.3), and `y` retries from there. The screen the user answered on stays in front of them: a confirmation that closed on a failed write would be indistinguishable from one that was backed out of, and the user would read a discard that did nothing as a discard they cancelled.

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

**While the confirmation is up, `y` and Escape are the only keys that act.** Enter, `d` and everything else are swallowed there exactly as they are on the waiting panel (§4.3) — the pane never acts on a key the screen in front of the user does not offer, and the reflex of confirming with Enter costs nothing but a second press of the key the footer names.

**A confirmed discard removes the pane's resume registration, permanently, and nothing else.** The entry is cleaned out of the store rather than suppressed for the boot, so the panel does not return on this boot, on the next attach, or after any future reboot. The pending marker is cleared and the pane falls through to a plain shell with its replayed scrollback still above it — indistinguishable from a pane that never had a hook.

**A discard that finds nothing to remove is still a discard.** If the entry has already gone — removed by `portal hook rm`, replaced by a re-registration, or hand-edited away while the pane waited — the marker clears and the pane falls through to a shell as it would after a removal, because the end state the user asked for is the state the store is already in. **A discard that cannot be written leaves the pane waiting and says so on the panel**: an unreadable store or an unavailable lock is reported in place, the registration and the marker both stand, and the key can be pressed again. The one outcome ruled out is a pane that drops its panel while the registration it named survives.

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

The saver reads a pane with `capture-pane -e -p -S -` (`internal/tmux/tmux.go:752`). Measured against that exact invocation on tmux 3.7c, with a live process holding the alternate screen open over a pane carrying 40 lines of prior output: the capture returns **only the lines that had already scrolled out of view, then the card** — the most recent screenful of real output is absent. Those lines are still in the buffer `capture-pane -a -p` reads, which the saver never calls. (`tmux capture-pane -e -p -S -` over a pane holding `T-01…T-40` with the alternate screen on → `T-01…T-20` plus the card; `tmux capture-pane -a -p` on the same pane → `T-21…T-40`.)

So an unfrozen waiting pane hashes differently from its last write and the saver rewrites its saved file as *transcript-minus-its-last-screenful, plus a picture of the card*. Once, not per tick — identical captures dedup. The lost screenful is the part the user was last reading and the part a resumed session continues from, and there is no copy of it anywhere: the next reboot replays the truncated file, so that work is gone and a dead card image sits in the history where it was, with a live card drawn over that. A pane waited on across three reboots loses a screenful each time and accretes three dead cards it can never shed.

Measuring with the caller's exact invocation is what changes the answer, and it took three readings to settle. A bare `capture-pane -p` returns the visible screen alone, which made the card look like a wholesale replacement. Reading the saver's own invocation as history-plus-card made it look like pure accretion. Neither held: the failure is loss of the most recent screenful *and* accretion of a dead card.

#### 7.2 The pane stays frozen for as long as its resume is unanswered

**The saver is kept off a waiting pane for the whole of the wait.** Today the only thing that keeps it off is the mid-restore marker, which the hydrate helper clears the moment replay finishes and before it hands off — replay, settle sleep, unset, exec (`cmd/state_hydrate.go:139-148`) — which under lazy resume would land at exactly the moment the panel goes up. That marker keeps its lifecycle and its other jobs unchanged; what holds the freeze through the wait is a second marker carried by the pane itself, set before the mid-restore one is cleared (§7.3).

**It is cleared when the user answers** — on Enter before the hook runs, and on a confirmed discard before the pane falls through to a shell (§6) — and on both paths only once the pane has left the panel's screen and is showing its own transcript again. Clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus the card (§7.1) — the whole failure, in the space between two steps, on the path every resume takes. This is the mirror of the rule that sets the pending marker before the mid-restore one is cleared (§7.3).

**A freeze that cannot be lifted holds the answer.** The pane leaves the panel's screen first, as every answer does; if the marker cannot then be cleared, the panel is drawn again carrying the reason on its report row (§5.3), and the key can be pressed again from it. Neither the hook nor the fall-through to a shell runs while the marker stands. Nothing is at risk in between: the freeze is still in force and the pane is showing its own transcript, so a tick landing there captures what is really in the pane. Handing the pane over with the marker still set would freeze that pane's saved scrollback for the rest of the pane's life — the pane goes on being used and every reboot restores the transcript it held when it paused — and nothing reports that state or reclaims it, since no sweep reaches a pane option (§8.2) and the waiter that would have died with the pane has just exec'd away. This is the shape a discard that cannot be written already takes (§6.2): what the screen claims and what the pane holds never disagree.

**The panel that comes back after a discard names a registration that is already gone.** On the discard path the entry is removed while the confirmation is still up, so a store that refuses the write can be reported there (§5.4); a marker that then refuses to clear brings the card back with nothing in the store behind it. It comes back as it was drawn — the removed command under its label, both key hints live — with the reason on its report row (§5.3). Enter reads the store again, finds nothing, and drops the pane through to a plain shell once the marker clears (§6.1), which is where the discard was going. The redraw is not a fresh decision about whether to draw at all: one that re-read the store would find no registration, paint nothing, and leave a pane that looks restored, swallows every key and has its saved transcript frozen for the rest of the pane's life.

The cost is nil in practice: a pane waiting on a resume has no new content worth saving, so freezing it at its last live state is exactly the desired end state.

**A waiting pane keeps its place in the saved set throughout.** The freeze suppresses that pane's scrollback write alone; the structural capture still enumerates the pane into `sessions.json` and merges its *previous* record back into the fresh index — guarded so a stale marker cannot resurrect a pane whose session, window or pane is gone (`internal/state/capture.go:96-127`). So a pane can wait indefinitely and still be restored on every subsequent reboot, with its original content and a fresh panel, however many reboots it waits through.

**The freeze is load-bearing, and what it prevents is lost work rather than clutter.** Without it, the one-tick gap between the two markers (§7.3) costs the user a screenful of their own transcript every time it is hit.

#### 7.3 The freeze is held by the pane, not by the pane's position

The marker that suppresses capture today is addressed positionally — session name plus window and pane index. The saver recomputes that address for every live pane each tick and skips only on an exact match (`cmd/state_daemon.go:263-273`), and bootstrap's stale-marker sweep unsets any marker no live positional address answers to, enumerating through the same positional format (`cmd/bootstrap/stale_marker_cleanup.go`). All three components of that address move: closing an earlier window renumbers, `break-pane` and `move-pane` relocate, and a rename changes the session half.

Today that exposure is the few seconds between skeleton restore and handover, which is why it has never mattered. This feature stretches it to the whole waiting life. A pane waited on for a week, whose session is renamed or whose sibling window is closed, loses its protection the moment the address stops matching: the saver recomputes it every tick and the capture lands. Nothing announces it, and the marker itself is then swept away as stale on the next bootstrap that actually runs — a cold server or a version change, rather than every `portal open`, which takes the warm latch's short-circuit and runs no orchestrator at all (`cmd/root.go:108-118`). This is the failure `resume-hooks-silently-lost` already fixed once for hook keys, reappearing on a different marker.

**The saver's skip gains a second condition rather than changing its first.** A pane is left alone when it is mid-restore — the existing positional marker, whose other jobs are unchanged — **or** when it is waiting, read from a pane-scoped pending marker. That marker travels with the pane through every rearrangement tmux can perform, measured in `resume-hooks-silently-lost` against `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename.

**The marker is the pane user-option `@portal-resume-pending`, set to `1`** — the value convention the skeleton markers already use (`internal/state/markers.go:78`). Presence is what is read; any richer payload is a later change the shape already admits (§8.2).

Two consequences follow and are part of the decision:

- **The pending marker is set before the mid-restore marker is cleared** — and therefore before the panel is painted. A gap where neither is set is a one-tick window in which the saver truncates the pane's saved transcript and writes the card over the end of it (§7.1): the whole failure, in the space between two steps.
- **The saver reads it for free.** It already enumerates every pane on the server each tick with a per-pane format to build its structural index, and that format already carries the pane's durable token as a column (`captureFormat`, `internal/state/capture.go:26`). The pending marker joins it as another column rather than costing a second tmux call. The arity of that read changes — `captureFieldCount` 11 → 12 — which is the same contained move the pane token made.

**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — has no route this feature leaves open.** The marker lives on the pane and the pane's only process is the waiter, so the marker goes when the pane goes. Two routes reach a live pane whose marker is wrong, and both are closed: an answer whose clear failed is held rather than carried out, so the pane goes on waiting and the marker is still the truth (§7.2), and a restore never respawns a waiting pane, because it skips any session that is already live (§9.1). What is left is a pane respawned out from under its waiter by hand — the user destroying the process that held that pane's state, in the same class as a hand edit of the store.

*Sibling check: `built-in-session-resurrection` records the marker lifecycle as "Helper unsets marker after dump + 100ms sleep", which is true of the code as it stands and is what this feature changes. No corrigendum is owed: that text is not a claim that has gone wrong, it is current behaviour this work supersedes — and the same reading applies to that specification's rejection of a Zellij-style confirmation prompt, a decision superseded by new product intent rather than a factual error.*

### 8. Seeing What Is Waiting

A reboot leaves roughly forty-one panes each holding a pending decision, and the panel exists only inside its own pane. The feature creates a state that previously did not exist and puts it somewhere invisible, so it owes an answer to "what is waiting".

**The panel in the pane is the whole interaction surface.** No list, no bulk answer path, no way to resume or discard from outside the pane it belongs to.

A dashboard was rejected: a pending-resume list is a second feature wearing this one's clothes, and the pane is the right place to decide, because the pane is where the context is. The decision needs the transcript above it, which no list can carry.

Bulk culling is already the picker's job and already exists as a route: killing a session takes its pending resume with it. It is not a *bulk* route — multi-select deliberately ignores the kill key, with the source stating why: none of the row actions compose with a marked set (`internal/tui/model.go:2588-2594`). So culling fifteen finished sessions today is fifteen rounds of select, confirm, repeat. That is the picker's gap rather than this feature's, and this feature neither widens nor narrows it.

#### 8.1 `portal doctor` reports a count

**Pending panes are visible as a passing count in `portal doctor`**, which already reports on this machinery.

**A non-zero pending count never fails the check and never changes doctor's exit code.** The number is detail on a line that passes.

That follows from what doctor's exit code means against what this feature produces. A check passes or fails, the exit code is zero only if all pass, and the catalog's nearest neighbours — the stale-hook and stale-project counts — fail the moment their count is non-zero (`cmd/doctor.go:398`, `:420`). Pending resumes are not a fault. A correctly functioning install presents roughly forty-one of them after a reboot, which is precisely the state this feature is built to produce, so wiring the count like its neighbours would make `portal doctor` report failure on success and break the scriptable exit code its whole design rests on.

#### 8.2 A pending pane is marked explicitly

The marker is the pane user-option `@portal-resume-pending` (§7.3). **The helper sets it before it clears the mid-restore marker, and therefore before it paints** — so the pane is never unprotected. Resume and a confirmed discard each clear it, both in the waiting program, on the ordering §7.2 sets.

**Only a pane that is going to wait is marked.** The helper resolves the pane's mode (§2) before it clears the mid-restore marker, so a pane with no registration — and one whose registration resolves eager — is never marked and goes on being captured exactly as it is today. That condition is also what holds the unreachability below: the marker only ever lands on a pane whose sole process is the waiter, which dies with the pane. A marker set on a pane that then execs its hook would have nothing left to clear it, and the saver would refuse that pane's scrollback write for the rest of the pane's life.

**A pane that cannot be marked does not wait.** If the pending marker cannot be written, the helper does not paint: it fires the hook as an eager registration does, and the pane comes back as today's restore leaves it. A wait with no marker on the pane is the one state the design refuses — the saver rewrites that pane's saved transcript as history-minus-its-last-screenful plus the card (§7.1) at the first tick that lands, for as long as the user takes to answer — and landing the user where eager would have put them is the degradation this feature already accepts (§2.1).

**The fall-through is recorded.** The helper emits one WARN as it fires the hook, naming the pane and the error that refused the marker — one more event on its existing hydrate catalog, not a new component. This is the only degradation the feature introduces that the user cannot read off the pane in front of them: a discard the store will not accept and a freeze that will not lift both report on the panel (§5.3). Without the line, an install that came back eager because a write failed is indistinguishable from one that is configured eager.

**Deriving the state instead was argued for and rejected.** tmux reports what is actually running in every pane in one read, so a pane running the waiter is a waiting pane by definition — nothing to set, nothing to clear, nothing that can go stale. It fails on two counts. It does not compose: every consumer — the picker, doctor, and whatever an agent-aware Portal wants later — has to re-derive it and re-handle its ambiguity, since tmux reports a process's name without its arguments, so any pane briefly running another Portal command reads as pending. And it carries nothing: a marker set at the moment a pane starts waiting can hold metadata about the pause, which a process name cannot. What that metadata should be is open — the point is only that the facility exists, and that a derivation forecloses it.

**The pending marker has no staleness case and owes no sweep.** A pane option is destroyed with its pane, so a pane closed mid-wait leaves nothing behind and there is no address by which a sweep could reach one. The clears are the two explicit actions above, and the unreachability of the inverse failure (§7.3) carries the rest.

Bootstrap's existing stale-marker sweep is not a backstop here and could not be: it enumerates `@portal-skeleton-*` **server** options and unsets them as server options (`cmd/bootstrap/stale_marker_cleanup.go:50-58`, `internal/state/markers.go:12,45,82`), so it is structurally blind to a pane option — which is exactly why the saver's skip had to gain a second condition rather than reuse the existing marker.

**A pane option rather than a server option keyed by position.** `resume-hooks-silently-lost` established this directly: positional keys go stale when tmux renumbers panes, and its measurements confirmed a pane user-option survives `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename. Portal models every other pane and session condition as a tmux option — the restore markers, the restoring flag, the spawn acks, the directory stamp, the pane token — and this is that vocabulary rather than an addition to it. The whole-server enumeration for pane options already exists, so the picker and doctor each cost one read.

#### 8.3 The picker's session row gains a second indicator

The picker is where the user would notice a new state. The row already carries an attached indicator; attached and pending-resume are independent, so it is a second indicator rather than a second meaning for the first.

**The session row drops the word `attached` and gains a second dot.** The green attached indicator loses its label and stands alone; a pending resume shows as a second dot in `accent.attention`. **A row carries that dot when any pane in the session is waiting** — the row answers whether the session holds a decision, not how many it holds. Doctor counts panes (§8.1), so a session holding two waiting panes contributes two to that count and one dot to the list.

**The dots pack to the right in a fixed order — attached, then pending — rather than holding reserved lanes.** A row with one dot puts it hard right whichever it is; a row with both shows the attached dot pushed left to make room. This was built the other way first, with a reserved lane per indicator so the columns aligned down the list, and it was rejected: an indicator should not claim space it is not using.

**Under `NO_COLOR` each indicator renders as a letter in the same cell** — `A` for attached, `P` for pending — so the packing order is untouched and there is no second row geometry: `A` alone, `P` alone, or `AP` when both. Portal's rule for that mode is that state stays glyph-backed and never colour-only, and two identically-shaped circles separated only by hue would not survive it. Uppercase rather than lowercase: a lone `a` in a status column reads as a typo, `A` reads as a status code.

**A gone row is unaffected.** The transient `session gone` badge already replaces the whole trailing region, so it continues to, and a session flagged gone has no live pane to be pending.

**Dropping `attached` is pulled into this feature; the rest of the row rework stays parked.** Removing the word is what makes room for a second indicator, so it cannot wait for the roadmap item — the window count, the session paths and the wider right-hand rework stay on `picker-row-redesign` (§10). One consequence rides with it: a bare indicator carries no meaning on its own, so **the help modal gains the legend in the same change** rather than after it.

### 9. Restore Pipeline Integration

The feature inserts an indefinite pause into the middle of a pipeline built on the assumption that hydration completes in seconds. Every edge was measured against the tree rather than reasoned about, and most of them need nothing.

**Nothing in the restore pipeline changes except the helper's own tail.** No new bootstrap step, no change to step ordering, no change to the eager signal pass, and no change to the global hooks. The insertion is contained to the one process that was already the last thing to run in a restored pane.

The one exception to that containment is the saver's capture skip, which gains a second condition (§7.3). That is the daemon, not the restore path.

#### 9.1 What was measured and needs nothing

**A second bootstrap does not disturb a waiting pane.** Most `portal open` runs never reach the orchestrator at all: a version-satisfied `@portal-bootstrapped` latch short-circuits the pre-run after a saver liveness check and returns (`cmd/root.go:108-118`), so restore only runs on a cold server or after a version change. When it does run, it skips any saved session whose name is already live (`internal/restore/restore.go:118`). A session holding a waiting pane is live, so it is never re-restored and its pane is never respawned out from under the user.

**A frozen pane is not dropped from the saved set.** The freeze suppresses that pane's scrollback write and nothing else, so a pane can wait indefinitely and still be restored on the next boot with the transcript it had when it paused (§7.2).

**Bootstrap's two sweeps leave it alone.** The stale-marker sweep only unsets markers whose pane is no longer live, and a waiting pane is live. The orphan-FIFO sweep has nothing to reclaim: the helper unlinks its FIFO as soon as the hydrate signal arrives (`cmd/state_hydrate.go:107`), long before the panel is drawn.

**The eager signal pass is unchanged.** Bootstrap step 7 still writes the hydrate byte to every freshly-armed pane, and the helper still replays on receiving it. What changes is only what the helper does afterwards.

**The global attach hooks are untouched.** `client-attached` and `client-session-changed` were named as likely surfaces and are not reached — nothing triggers the panel (§4.4).

**Scrollback replays exactly as it does today, for every restored pane, whether or not a resume is pending.** Nothing in the restore path branches on whether a pane has an unfired hook.

Suppressing replay behind a waiting panel was considered and rejected. The case for it was that a replayed transcript ending in an exited session looks live and is not. It is not a lie — it is an accurate record of what happened, and exactly what the pane would show if the session had been quit by hand; the visible end of the transcript says plainly that the thing is gone. The decisive argument was scope: restoring a pane's content and resuming its process are two separate concerns, and making the first conditional on the second buys a small presentational gain at the cost of a conditional in the middle of the restore path. The presentational worry is answered by the surface instead — the panel's canvas covers the pane while the decision is pending, so the old transcript is hidden until the user has chosen, and revealed only once they have declined and know what they are looking at.

#### 9.2 The pending state is never persisted

It is recomputed on each boot from a registration that has not fired, so nothing about it reaches `sessions.json` and **no schema version moves**. The pane's waiting-ness lives on the pane as a tmux option for as long as the pane does (§8.2), and nowhere else.

### 10. Not In This Feature

Two capabilities this work touched are on the product roadmap rather than left as loose ends. Neither is a prerequisite: the feature ships complete without them.

**Preferences UI** (`preferences-ui`, horizon `next`). A settings screen in the picker for install preferences, so `prefs.json` is not hand-edited. The theme picker is currently the only preference with any UI, and this feature adds a second setting that needs one. Until it exists, the install-wide resume mode is changed by hand-editing the file (§3.1) — which is what raised the stakes on the default (§2.1).

**Picker row redesign** (`picker-row-redesign`, horizon `next`). Dropping the window count, which reads "1 window" on every row of the measured install (42 of 42 live sessions held a single window, `tmux list-windows -a -F '#{session_name}' | sort | uniq -c`, 2026-09-18), showing each session's directory path beside its name, and finishing the right-hand status strip. It changes every row for every session and is driven by its own rationale rather than by this feature.

**Two pieces of the row rework are pulled forward into this feature** and are not on the roadmap item: dropping the `attached` word, and adding the pending-resume indicator beside it (§8.3). The word had to go to make room for a second indicator, so it could not wait — and the help-modal legend rides with it, because a bare indicator carries no meaning on its own.

Also explicitly not built:

- **A pending-resume dashboard or list.** Rejected on its merits, not deferred (§8).
- **A bulk answer path.** Multi-select still ignores the row actions; culling finished sessions stays one-at-a-time in the picker. That gap predates this feature and is untouched by it (§8).
- **A migration of `hooks.json`.** Both stored shapes are permanently valid and neither converts to the other (§3.2).
- **Any change to the pane's durable token.** Discarding removes a registration and never unstamps a pane (§6.2).
- **Any expiry or tidy pass over old-format hook keys.** The retention rule `resume-hooks-silently-lost` established is untouched; this feature only ever removes token-shaped keys.
---

## Working Notes
