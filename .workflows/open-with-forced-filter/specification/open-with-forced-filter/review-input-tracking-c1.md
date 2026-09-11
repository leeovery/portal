# Review Tracking: Open With Forced Filter - Input Review

## Findings

### 1. A zero-match search on a cold boot has nowhere to report itself

**Source**: `discussion/open-with-forced-filter.md` — `cold-path-classification` Decision ("the sigil is classified as a picker invocation … it gets the concurrent bootstrap and the loading page") together with `unambiguous-direct-attach` Decision ("zero fails honestly")
**Category**: Gap/Ambiguity
**Move**: settled
**Affects**: §3.2 (outcomes by match count), §3.7 (the zero-match failure), §7.4 (cold-path accepted cost)

**Problem**:
On a cold boot `x /port` shows the loading page while the server starts and every saved session is restored — seconds of it. If nothing matches when that finishes, the user is sitting in a full-screen alternate-screen TUI holding a failure it has no stated way to deliver: the search must fail hard with nothing opened and no picker, but the screen the failure has to travel through is already up. Left unstated, the same miss can land three different ways — the user dropped into an unfiltered picker anyway, the message printed into the alternate screen and wiped as it tears down, or a silent exit 0 that a shell function treats as success.

**Proposal**:
The cold path delivers the miss exactly as the warm path does: the TUI closes, the same message goes to the terminal, the same non-zero exit. The loading page is the only difference between the two paths, and a search that matched nothing is an ordinary `open` failure rather than a bootstrap fatal, so it does not take the in-TUI error frame reserved for those.

**Proposed Text**:
Append to §3.7, after the paragraph on the K = 0 message:

**On a cold boot the miss arrives after the loading page and reads identically.** A sigil invocation takes the loading page before K can be taken (§7), so a K = 0 on a cold server is discovered with the TUI already on screen. The TUI closes, the same message is written to the terminal, and the exit status is the same non-zero one the warm path returns. This is not a bootstrap fatal and does not take the in-TUI error frame; the picker never appears, exactly as §3.2 requires.

**Resolution**: Approved
**Notes**: Applied to §3.7 verbatim.

---

### 2. Nothing says which rule governs the narrowed list after it is re-rendered

**Source**: `discussion/open-with-forced-filter.md` — `search-match-domain`, "The matching rule": "If the user then edits the filter by hand inside that picker, the picker's own rule applies and the row set can widen. The divergence is therefore visible only by rows appearing, never by rows the user expected going missing" — read against the seed flow in the discussion Context ("`Space` through the two or three survivors to see which is which, `Enter` to attach")
**Category**: Gap/Ambiguity
**Move**: choice
**Affects**: §4.4 (the rule diverges by entry point), §3.3 (the committed-filter landing)

**Problem**:
The narrowed list the search hands over does not sit still. The user presses `Space` to preview a candidate and comes back, cycles the grouping with `s`, or the list refreshes because a session was killed in another window. The record only ever considered the row set widening when the user edits the filter text by hand. If the loose rule takes over on any of those re-renders instead, rows nobody asked for appear underneath a cursor the user is in the middle of moving — in precisely the flow this feature was built for: narrow, `Space` through the two or three survivors, `Enter`. The user's protection ("never rows going missing") survives either way; what does not survive is knowing whether the three rows they were choosing between are still the three rows in front of them.

**Options**:
- The containment set stays in force for as long as its filter text stands untouched — every re-render reproduces it, and only a hand edit hands the list back to the picker's own rule (recommended)
- Containment narrows the first displayed set only; every later re-render filters through the picker's own rule, so the set can widen with no keystroke from the user

**Resolution**: Approved
**Notes**: Option 1 chosen — the containment set stays in force for as long as its filter text stands untouched; every re-render reproduces it, and only a hand edit returns the list to the picker's own rule. Applied to §4.4.

---

### 3. A session with no recorded directory has an unstated row

**Source**: `discussion/open-with-forced-filter.md` — `search-match-domain` Decision, "Which directory value is matched" ("Only the directory tmux records against the session … Never a derived one"; "a session created before the directory stamp shipped carries no recorded directory and so matches on name alone, in every view") and `search-result-display` Decision ("shows the directory it matched on, beside the session name")
**Category**: Gap/Ambiguity
**Move**: settled
**Affects**: §6.1 (the display requirement), §4.1 (the matched fields)

**Problem**:
A session created before the directory stamp shipped carries no recorded directory, and a search can still return it on a name match. What its row shows in the directory slot is unstated, which leaves the obvious-looking repair open: fill the slot by asking the session's pane where it is. That is the per-session pane read the feature refused to pay for when taking the match count, smuggled back in at render time — one tmux round-trip per unstamped session on a path whose whole point is to feel instant, and a directory on the row that the search never matched against, so the row now explains itself with something that is not the reason it is there.

**Proposal**:
The displayed directory is the recorded one and nothing else; where a session has none, the slot is empty. Nothing is derived at render time, for the same reason nothing is derived for the count.

**Proposed Text**:
Append to §6.1, after the "In every grouping mode" paragraph:

**Only the recorded directory is displayed.** A session carrying no recorded directory (§4.1) shows none beside its name — the slot is simply empty. The displayed value is never derived from a pane read, for the same reason the match is not: the row must show what the search actually matched against, and the sigil path pays for no per-session pane read.

**Resolution**: Approved
**Notes**: Applied to §6.1 verbatim.

---

### 4. The column treatment the source handed to this specification is passed on instead

**Source**: `discussion/open-with-forced-filter.md` — `search-result-display` Decision (Initial): "The exact column treatment is presentation detail the specification settles" and "a long path in a narrow terminal needs truncating — the delegate already truncates, so this is presentation detail for the specification rather than an open question"
**Category**: Enhancement to existing topic
**Move**: choice
**Affects**: §6.2 (placement and weight)

**Problem**:
In a narrow terminal a row cannot carry both a session name and a long absolute path. Which one gives way, and how the path is shortened, is unsettled — so one build renders `api-work  /Users/leeovery/Cod…` and another `api-work  …/Code/portal`, and only the second still answers the question the row exists to answer: which of these two sessions is the Portal one. A path shortened from the wrong end loses exactly the segment a human recognises it by, and the feature's whole premise is that the directory is what the user recognises and the name is not.

**Options**:
- Truncate the directory from the left so its tail survives (`…/Code/portal`), home-abbreviated as `~/…`, with the session name never truncated (recommended)
- Truncate the directory from the right with the row's existing truncation, home-abbreviated, name never truncated
- Show the directory in full and let the session name yield when the row is tight
- Leave the treatment to implementation, as the section reads today

**Current**:
Exact column treatment is presentation detail for implementation. The row already flexes the name against a fixed count slot, a fixed attached slot and a right margin, and already truncates (`grep -n 'ansi.Truncate' internal/tui/session_item.go`), so a long path in a narrow terminal is handled by the mechanism that is already there.

**Resolution**: Pending
**Notes**:

---

### 5. The documentation distinguishes the two filtered forms but never says which to reach for

**Source**: `discussion/open-with-forced-filter.md` — `surface-reconciliation` Journey: "`-f` is the explicit form that always lands in the picker, which is what a script or a keybinding wants; `/term` is the interactive form that finishes the job when it can"
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §9.1 (why documentation is a deliverable), §9.2 (what the documentation must carry)

**Problem**:
The help text and README will tell a reader how the two filtered forms differ and still leave them working out which one belongs in their shell alias. The difference has a use behind it — a script or a keybinding needs the form that lands in the same place every time, while a person at a prompt wants the one that finishes the job when there is only one place to go — and that is the half a reader actually acts on.

**Current**:
`-f` documented as "open the picker pre-filtered" invites a reader to assume `/term` is shorthand for it. They differ exactly where it matters: with one match, `/term` attaches and `-f` shows a list of one. **The pair must be described by outcome — one always shows the list, one takes you there when there is only one place to go.**

(and in §9.2)

- `-f` and `/term` distinguished by outcome rather than by input shape (§9.1).

**Proposed Text**:
`-f` documented as "open the picker pre-filtered" invites a reader to assume `/term` is shorthand for it. They differ exactly where it matters: with one match, `/term` attaches and `-f` shows a list of one. **The pair must be described by outcome — one always shows the list, one takes you there when there is only one place to go.** The outcome carries a use with it, and the documentation says which to reach for: `-f` is the form for a script or a keybinding, which needs to land in the same place every time; `/term` is the interactive form.

(and in §9.2)

- `-f` and `/term` distinguished by outcome rather than by input shape, and which of the two to reach for (§9.1).

**Resolution**: Pending
**Notes**:

---

### 6. The picker's filter is case-folded, which is what the sigil's rule keeps

**Source**: `discussion/open-with-forced-filter.md` — `search-match-domain`, "The matching rule": "the sessions list filters through `charm.land/bubbles/v2/list.DefaultFilter`, which is `fuzzy.Find` from `sahilm/fuzzy` — subsequence matching, case-folded and rank-sorted"
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §4.3 (the matching rule), §4.4 (the divergence table)

**Problem**:
Whether `/port` finds a session living in `~/Code/Portal` reads as undecided, and whether it agrees with the same four letters typed by hand inside the picker reads as undecided too. Case is stated for one of the two rules and omitted from the other, so a reader comparing them cannot tell whether case is part of what diverges — and the sigil's own case-folding looks like a new rule invented for it rather than the single property it keeps from the filter it is defined against.

**Proposal**:
Carry case-folding in the description of the picker's own filter, where it is a measured property of the matcher in use, and say plainly that it is the property the sigil retains — so the divergence is subsequence against containment and nothing else.

**Current**:
The picker's own filter does not. The sessions list filters through `charm.land/bubbles/v2/list.DefaultFilter`, which is `fuzzy.Find` from `sahilm/fuzzy` — subsequence matching, anywhere, rank-sorted — and `internal/tui` installs no filter of its own, so that default stands (`grep -rn 'SetFilterFunc\|DefaultFilter' internal/tui/*.go | grep -v _test` → no matches).

**Proposed Text**:
The picker's own filter does not. The sessions list filters through `charm.land/bubbles/v2/list.DefaultFilter`, which is `fuzzy.Find` from `sahilm/fuzzy` — subsequence matching, anywhere, case-folded and rank-sorted — and `internal/tui` installs no filter of its own, so that default stands (`grep -rn 'SetFilterFunc\|DefaultFilter' internal/tui/*.go | grep -v _test` → no matches). Case-folding is the one property the sigil keeps from it: the two rules diverge on subsequence against containment and on nothing else.

**Resolution**: Pending
**Notes**:

---
