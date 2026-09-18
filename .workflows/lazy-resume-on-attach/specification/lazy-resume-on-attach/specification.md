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
---

## Working Notes
