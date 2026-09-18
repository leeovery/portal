# Review Tracking: Lazy Resume On Attach - Input Review

## Findings

### 1. A colourless install gets a theme-coloured panel in every restored pane

**Source**: `discussion/lazy-resume-on-attach.md` — Colourless Indicator Rendering (Context: "Portal has a `NO_COLOR` carve-out where the picker renders colourless on the terminal's native foreground and background, and the rule for that mode is that state stays glyph-backed and never colour-only — enforced at each site it matters")
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §5.2 (the panel's canvas and theme resolution), with the same reading carried to the discard confirmation in §5.4

**Problem**:
A user who runs their terminal colourless reboots and meets forty-odd panes each filled edge to edge with a theme colour, and a terminal-wide background query fired once per pane on the way there. The source names the colourless mode as a Portal-wide rule enforced at every site it matters, and the feature applies it to the two new session-row indicators only. The one genuinely new full-pane painted surface in the product — the thing that covers every restored pane after a reboot — is left with an unqualified instruction to fill the pane with the active theme and to race a terminal query before doing it, so the mode the user chose is honoured on a two-cell indicator and broken across their whole screen.

**Proposal**:
Carry the existing carve-out onto the panel: no canvas fill, no appearance detection, colourless render on the terminal's native foreground and background, with the panel's state carried on glyphs and words as the source's rule requires. The source states the carve-out as Portal's rule at every rendering site and the specification already binds the panel's theme to "as it does everywhere else in Portal", so nothing here is a new call. The one thing worth saying out loud is that coverage does not depend on the fill: the panel is on the pane's alternate screen (§5.1), so the transcript stays hidden whether or not a colour is painted over it.

**Current**:
> **The theme resolves as it does everywhere else in Portal.** A named theme paints from the first frame with no gate at all. A light/dark pair runs the same detect-or-timeout appearance gate the picker runs — a query to the terminal raced against the same short timeout (`appearanceDetectTimeout`, `internal/tui/appearance_gate.go:12`), resolving dark when there is no answer — in the process that draws. That process hands off before it waits (§4.2), so the gate is paid once per draw and nothing of it stays resident while the pane waits. A pane drawn with no client attached to it gets no answer and resolves dark — that fallback reached by the ordinary route rather than a second rule. Most panes are drawn at restore with nobody watching them, so under a pair the dark half is what most first draws land on; a redraw with a client present resolves against the terminal in front of it.

**Proposed Text**:
(insert as a new paragraph immediately after the paragraph quoted in Current; nothing existing changes)

**`NO_COLOR` is the same carve-out here as everywhere else in Portal.** No canvas is painted, no appearance detection runs at all, and the panel renders colourless on the terminal's native foreground and background. Coverage does not depend on the fill: the panel sits on the pane's alternate screen (§5.1), so the transcript stays hidden underneath it whether or not a colour is painted over the pane. What the panel loses is hue alone — the card frame, the `● PAUSED` badge, the `ON RESUME` label and the key hints carry it on glyphs and words, and the discard confirmation keeps its `▲ Discard resume?` title and its `y discard   esc cancel` footer where the command loses `state.destructive`. That is the same rule the session row's indicators take (§8.3): state stays glyph-backed and never colour-only.

**Resolution**: Approved
**Notes**: Applied verbatim. The source states the carve-out as a Portal-wide rule at every rendering site, and §5.2 already binds the panel's theme to Portal's own resolution — the record determines it.

---

### 2. A mistyped pin is stored as a pin that pins nothing

**Source**: `discussion/lazy-resume-on-attach.md` — Eager Lazy Preference ("The override is set where the registration is made: `portal hook set`. A flag on the command that writes the entry"); Unreadable Stored Registrations (the tolerant read is derived for a hand-edited store: "The store is hand-editable by design, and the object form invites exactly the mistakes a hand edit makes")
**Category**: Gap/Ambiguity
**Move**: decide
**Affects**: §3.3 (setting the override), read against §3.2 (a stored value the reader cannot make sense of)

**Problem**:
A user pins the one registration that has to come back automatically — a dev server, a tunnel, a watcher — and mistypes the mode. Nothing refuses it, so the pin is written, the command reports success, and the registration quietly follows the install default instead: at the shipped default that thing does not come up at the next reboot, it waits behind a panel nobody is watching, and the only trace is an empty cell in `portal hook list` the user has no reason to go and read. The same mistake made by a script that writes registrations on every session start is never seen at all. The specification says what a value the reader cannot make sense of does once it is on disk, and says nothing about whether the command that writes it accepts one.

**Proposal**:
Refuse an unrecognised mode at the command: `--resume-mode` takes `eager` and `lazy` and nothing else, and anything else exits non-zero and writes nothing — the same disposition the section already gives a mode passed with no command. What leans it is who is on each side of the two rules: the tolerant read exists for a file the user hand-edits, where failing a pane over a typo costs more than ignoring it, while the flag is an explicit assertion made by a writer who is still at the keyboard (or a script whose exit code is checked), where refusing is free and silence is not. The alternatives that also fit the record: store the value verbatim and let the tolerant read drop it, which keeps one rule for both sides at the cost of a pin that silently does not exist; or accept the flag and drop it without writing a mode, which is the same silence with less on disk.

**Proposed Text**:
(insert as a new paragraph immediately after "**A mode is always passed with the command it belongs to.** …" in §3.3)

**A mode the command cannot recognise is refused with it.** `--resume-mode` takes `eager` or `lazy` and nothing else; any other value exits non-zero and writes nothing, so a mistyped pin fails where it was typed rather than landing on disk as a mode nothing reads. That is the writer's side and it does not soften the reader's: a value that reaches the file by hand edit still carries no mode and still never fails a pane (§3.2).

**Resolution**: Routed
**Notes**: The call landed in the discussion, extending the Unreadable Stored Registrations decision with the writer's side; §3.3 aligned to it.

## Observations

- §6.4 mints two members of the closed log vocabulary — `op=discard` and `via=panel`; the discussion decides only that the removed command is logged in the stale sweep's recoverable form, not what names the route takes.
- §8.3 calls the attached indicator "green" while every other colour on the new surfaces is named as a theme token.
- A theme committed in the picker while panes are waiting leaves already-drawn panels in the old palette until their next redraw; neither source addresses it, and a reboot or a resize corrects it.
