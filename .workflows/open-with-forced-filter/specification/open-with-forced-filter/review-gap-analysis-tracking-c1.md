# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. Does `-f` now find sessions by their directory, or not?

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Critical
**Affects**: §4.2, §4.4, §5.5, §10.5

**Problem**:
The specification supports two incompatible builds of the existing `-f/--filter` flag and of the filter the user types by hand in the picker. One part states, deliberately and with a reason, that all three ways of narrowing the sessions list search the same fields — session name plus the session's recorded directory — so widening for the new form widens for all of them. Two other parts promise that `-f` keeps its existing behaviour "in full" and that nothing on the existing surface changes except how `-f` is described. A builder can therefore ship `-f api` returning only sessions whose *names* contain `api`, or returning sessions whose *directory* contains `api` with nothing on the row to explain why they are there. Both pass a reading of the document, and users of a shipped flag get whichever one the builder picked.

**Proposal**:
The widening governs and the two blanket "unchanged" statements are what is wrong. The widening is a specific, reasoned decision — one list that narrows on different fields depending on how the user arrived at it is harder to predict than one wider rule — while the "unchanged" statements are written about the flag's argument contract and were not drafted against it. Correct both so they say what actually stays put (the flag's argument contract, its Projects redirect under a pending command, its fuzzy matching rule, its prominence) and record the one thing that moves (the fields it searches).

**Current**:
§5.5:
> `-f/--filter` keeps its existing behaviour in full, including its Projects redirect under a pending command. The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9) and no behaviour change to a shipped flag.

§10.5:
> **Nothing is retired, renamed or deprecated.** `-f`, all four domain pins, and the bare positional chain keep their current behaviour and their current prominence. The only change owed to the existing surface is how the `-f` / `/term` pair is described (§9.1).

**Proposed Text**:
§5.5:
> `-f/--filter` keeps its argument contract, its fuzzy matching rule and its Projects redirect under a pending command. Its matched fields widen with everything else's (§4.2): after this feature `-f` narrows on session name and recorded directory, exactly as the sigil and the hand-typed filter do. The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9).

§10.5:
> **Nothing is retired, renamed or deprecated.** `-f`, all four domain pins, and the bare positional chain keep their current prominence, and the domain pins and the bare positional chain keep their current behaviour exactly. Two changes are owed to the existing surface: `-f`'s widened match domain (§4.2), and how the `-f` / `/term` pair is described (§9.1).

**Resolution**: Approved
**Notes**: Applied to §5.5 and §10.5 verbatim; §5.5's heading retitled from "`-f` is unchanged" to "What `-f` keeps", which the old title contradicted.

---

### 2. The search shape is recognised inside a trailing command's own arguments

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Critical
**Affects**: §2.2, §2.3, §5.1

**Problem**:
The rule that a `/word` argument is a session search *wherever it sits on the command line* takes the trailing command's own arguments with it. `portal open ~/Code/api -- ls /tmp` would be refused as a usage error because `/tmp` appears in the command the user asked to run — not because they asked for a session search at all. A shipped way of minting a session with a command in it stops working for every command that takes a single-segment absolute path (`/tmp`, `/opt`, `/etc`), and the error message can only talk about a form the user never used.

**Proposal**:
Recognition applies to the arguments `open` parses as its own targets, and stops at the `--` separator: everything after it is the command's payload, passed through untouched and never tested for the shape. This costs the rule nothing, because a line carrying the search form carries no command at all — a line holding both is already a usage error on the target it names. A command supplied as a flag value is not at risk either way, being a value rather than a positional.

**Proposed Text**:
Add to §2.3, after the existing paragraphs:

> **The command payload is not searched for the shape.** Recognition applies to the arguments `open` parses as targets, and stops at a `--` separator: the words after it are the trailing command's own, passed to that command untouched, so a `/word` among them is that command's argument and never a sigil. `portal open ~/Code/api -- ls /tmp` is unaffected by this feature. The rule loses nothing by stopping there, because a sigil line carries no command at all (§5.1) — a line holding both is a usage error on the target it names. A command carried as a flag value is a value rather than a positional and was never in reach of the rule.

**Resolution**: Approved
**Notes**: Applied to §2.3 verbatim.

---

### 3. "Any other argument is a usage error" also refuses `--help`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §5.1

**Problem**:
The composition rule is written as an absolute: any other argument on the line is a usage error. Read as written it refuses `portal open /port --help` — so the one user who half-remembers the form and reaches for its help gets a refusal instead of the help — and it refuses any root-level flag that applies to every command. That is not what the rule is for, and a builder implementing it literally ships a command that cannot explain itself.

**Proposal**:
The rule governs targets, the trailing command, and `open`'s own flags. Flags that answer before the command body runs — help, version, and root-level persistent flags — are outside it and behave as they do on any other invocation.

**Current**:
> **`/term` composes with nothing. Any other argument on the line is a usage error.**

**Proposed Text**:
> **`/term` composes with nothing. Any other target, any trailing command, and any of `open`'s own flags on the same line is a usage error.**
>
> Flags that answer before the command body runs are outside the rule: `portal open /term --help` prints help, and root-level persistent flags apply as they do to any other invocation.

**Resolution**: Approved
**Notes**: Applied to §5.1 verbatim.

---

### 4. What returns the narrowed list to the picker's looser rule is not pinned down

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §4.4

**Problem**:
The strict search rule is said to hold "for as long as the filter text stands untouched", and a hand edit returns the list to the picker's looser one. A builder cannot tell which act counts. If the user opens the filter input and closes it without changing a character, or types a character and deletes it again, does the list stay the set they were handed, or silently widen with rows they never searched for? Both builds are defensible from the text, and the difference is rows appearing under a moving cursor.

**Proposal**:
Key the rule on the text, not on the act: while the committed filter value is character-identical to the term the search supplied, the strict rule stands; any other value — including a cleared one — uses the picker's own rule. That is what "stands untouched" names, and it is the only version a user can predict, since the input's contents are what they can see.

**Current**:
> **The containment set holds for as long as the sigil's filter text stands untouched.** The narrowed list does not sit still — a `Space` preview and back, an `s` regroup, a refresh after a session is killed elsewhere all re-render it — and every one of those reproduces the containment set. The list the user is choosing from is the list they were handed. Only a hand edit of the filter text returns the list to the picker's own rule.

**Proposed Text**:
> **The containment set holds for as long as the sigil's filter text stands untouched.** The narrowed list does not sit still — a `Space` preview and back, an `s` regroup, a refresh after a session is killed elsewhere all re-render it — and every one of those reproduces the containment set. The list the user is choosing from is the list they were handed. Only a hand edit of the filter text returns the list to the picker's own rule, and the test is the text rather than the act: while the committed filter value is character-identical to the term the sigil supplied, containment stands — opening the filter input and leaving it as it was, or editing back to the same characters, keeps it. Any other value, a cleared filter included, is the picker's own rule.

**Resolution**: Approved
**Notes**: Applied to §4.4 verbatim.

---

### 5. Nothing says what order the narrowed list is in

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §3.2, §4.3, §4.4

**Problem**:
The cursor is required to land on the first matching row, but the order those rows are in is never stated. The picker's own filter sorts by match quality; the stricter rule this feature uses produces no such score, so a builder either hands the list back in its usual order or invents a ranking of their own. The row the cursor lands on — the one a user hitting Enter straight away gets — differs between the two, and an invented ranking would also reorder rows across the group boundaries the grouped views are built on.

**Proposal**:
Narrowing removes rows and reorders nothing: rows keep the order the sessions list gives them in whatever grouping mode is current, and the first matching row is the first survivor of that order. Determined by the requirement that every re-render (regroup, refresh, preview and back) reproduces the same list, and by grouping surviving a committed filter — a match-quality sort would cut across both.

**Proposed Text**:
Add to §4.4, after the paragraph beginning "**The containment set holds…**":

> **Containment narrows the list; it does not reorder it.** Rows keep the order the sessions list gives them in whatever grouping mode is current, with non-matching rows removed and nothing re-ranked — the picker's rank-sorting is a property of its fuzzy rule, which the sigil does not use. The first matching row (§3.2) is the first surviving row of that existing order.

**Resolution**: Approved
**Notes**: Applied to §4.4 verbatim.

---

### 6. Whether every row carries a directory, or only the rows found by one

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §6.1

**Problem**:
"A row shows the directory it matched on" reads two ways. Either every row in a search-narrowed list carries its directory, giving a uniform column, or only the rows that were found *by* their directory do — in which case a bare row silently means "this one matched by name", a signal no user will read that way, and the list comes out ragged. The two builds look substantially different on the same search.

**Proposal**:
Every row shows its recorded directory, whether it was found by name or by directory. The rest of the section already describes a column — a slot that is simply empty for a session with no recorded directory, taking its width from what the name and the fixed slots leave — which only makes sense as a column present on every row; and the premise of the feature is that the user cannot recognise sessions by name, so a name-matched row needs accounting for too.

**Current**:
> **A row in a sigil-opened list shows the directory it matched on, beside the session name.**

**Proposed Text**:
> **Every row in a sigil-opened list shows its recorded directory beside the session name** — the rows found by their directory and the rows found by their name alike, so the column is uniform rather than a signal in itself.

**Resolution**: Approved
**Notes**: Applied to §6.1 verbatim.

---

### 7. Whether the directory column survives the user editing the filter

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §6.3, §4.4

**Problem**:
The matching rule reverts to the picker's own the moment the user hand-edits the filter text. Whether the directory column goes with it is not said. One build keeps the column for as long as that picker is open; the other makes a column vanish from every row the instant the user types a character — while the rows it was explaining are still on screen, still present because of directories that are now invisible.

**Proposal**:
The column belongs to the invocation, not to the filter text: it stands for the whole picker session the search opened, after a hand edit and after a cleared filter alike. The fields stay widened after the edit, so rows can still be present on the strength of their directory, and the accounting the column provides is still owed.

**Current**:
> Scoped to the sigil's own list, across every grouping mode that list can be in (§6.1). How Sessions rows render when the picker is reached any other way is untouched, consistent with the matching rule's own divergence (§4.4).

**Proposed Text**:
> Scoped to the picker session a sigil opened — across every grouping mode that list can be in (§6.1), and for as long as that picker is open. A hand edit of the filter text returns the matching rule to the picker's own (§4.4) but does not take the column with it: the rows can still be present on the strength of their directory, so the accounting is still owed. How Sessions rows render when the picker is reached any other way is untouched.

**Resolution**: Approved
**Notes**: Applied to §6.3 verbatim.

---

### 8. What the directory does on a row too narrow to hold it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: choice
**Priority**: Important
**Affects**: §6.2

**Problem**:
The directory is said to give way when the row is too narrow for both it and the session name, and separately to take its width from whatever remains and be shortened from the left. On a narrow terminal those describe different rows: one ends in an ellipsis and a character or two of a path, which says nothing and reads as damage; the other drops the directory and shows the name alone. A builder must pick, and the narrow-terminal row — the case the sentence exists for — is what differs.

**Options**:
- Drop the directory whenever the remaining width cannot hold an ellipsis plus one whole path segment, showing the name alone below that point (recommended)
- Always render the directory into whatever width remains, left-truncated, however little that is

**Resolution**: Approved
**Notes**: Option 1 chosen — the directory is dropped below a floor of an ellipsis plus one whole path segment, rather than truncated to noise. Applied to §6.2.

---

### 9. A cold-boot single match loses its bootstrap warnings entirely

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §7.1, §7.2, §3.7

**Problem**:
On a cold boot the search takes the loading page, and its soft bootstrap warnings are routed into the TUI's notice band rather than the terminal. When exactly one session matches, the TUI is torn down and replaced by the attach — so the band never appears long enough to be read, and a warning that the saver is down is lost on precisely the path the classification was chosen to protect. A builder has to decide where those warnings go, and "nowhere" is one of the answers the text permits.

**Proposal**:
When a search resolves to a direct attach, its accumulated soft warnings are written to the terminal after the TUI has torn down and before the connector runs. That is where the same invocation already puts them on a warm server, so the two paths agree; and the alternate screen is gone by that point, so the corruption the picker classification exists to prevent cannot occur.

**Proposed Text**:
Add as a new subsection after §7.4:

> #### 7.5 A sigil that resolves to a direct attach still delivers its warnings
>
> On K = 1 the TUI tears down before the connector runs, so the notice band never surfaces. The accumulated soft warnings are written to the terminal at that point instead — after teardown, before the attach — which is where a warm-server sigil attach already puts them. The alternate screen is gone by then, so the corruption this classification prevents cannot occur.

(The existing §7.5 becomes §7.6.)

**Resolution**: Approved
**Notes**: Applied as §7.5 verbatim; the former §7.5 renumbered to §7.6 (no §-references pointed at it).

---

### 10. Whether a home-directory path is always abbreviated, or only when width runs out

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §6.2

**Problem**:
A path under the user's home is "abbreviated to `~/` first". That reads either as how such a path is always displayed, or as the first rung of a ladder that only runs when the row is too narrow. Most sessions live under the user's home, so the two builds differ on nearly every row of a wide terminal: `~/Code/portal` on one, `/Users/leeovery/Code/portal` on the other.

**Proposal**:
Always abbreviate. The clause says the abbreviation reclaims width *before any truncation is needed*, which places it outside the truncation path, and the abbreviated form is the one a human reads a checkout by.

**Current**:
> A path under the user's home is abbreviated to `~/` first, which reclaims the width before any truncation is needed.

**Proposed Text**:
> A path under the user's home is always displayed abbreviated to `~/`, at any width — the abbreviation is how the value is rendered rather than a rung of the truncation ladder, and it commonly reclaims enough width that no truncation is needed at all.

**Resolution**: Approved
**Notes**: Applied to §6.2 verbatim.

---

### 11. The three outcomes are written out twice

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §1.2, §3.2

**Problem**:
What happens on zero, one and two-or-more matches is stated in full in the opening description and again in the table that owns it. The two agree today; the next correction to either leaves the document stating both the old behaviour and the new one, and a builder reading only the opening gets the stale version.

**Proposal**:
The outcomes table is the home. The opening description keeps its job — naming the form and what the term does — and points at the table for the outcomes.

**Current**:
> A positional beginning with `/` — `x /port` — declares session-search intent. The term after the slash is forced into the sessions list as filter text and never enters the resolution chain. Exactly one matching session attaches outright; two or more open the picker with the list already narrowed and the cursor on the first row; zero is a hard failure.

**Proposed Text**:
> A positional beginning with `/` — `x /port` — declares session-search intent. The term after the slash is forced into the sessions list as filter text and never enters the resolution chain, and the number of live sessions it matches decides what happens (§3.2).

**Resolution**: Approved
**Notes**: Applied to §1.2 verbatim.

---

### 12. Tab completion: what the offered words look like, and when Tab comes back empty

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §8.1

**Problem**:
Completion is required to leave the leading slash in place while completing the term after it, which leaves a builder two unsettled points. Whether the offered words carry the slash — a bare session name replaces the whole word and deletes the slash the user typed, producing the opposite of the stated outcome. And whether the words offered are the sessions the term *starts*, or the sessions the search would *find*: pressing Tab on a term the form itself resolves and getting nothing back reads as a broken feature unless it is stated as the deal.

**Proposal**:
Offered words carry the sigil, so the completed word is a sigil again. They are the live session names the typed term prefixes — the shell discards any offered word that is not an extension of the word being completed, so completion is prefix-shaped even though the form itself matches by containment. Saying so keeps the help text from promising more.

**Proposed Text**:
Add to §8.1, after the first paragraph:

> Offered words carry the sigil — `/po` completes to `/portal-a1b2`, never to `portal-a1b2`, which would replace the whole word and drop the slash the user typed. The words offered are the live session names the typed term prefixes: the shell discards any candidate that is not an extension of the word being completed, so completion is prefix-shaped even though the form itself matches by containment (§4.3). `/ort<TAB>` therefore offers nothing, while `/ort` still finds `portal-a1b2` on Enter.

**Resolution**: Approved
**Notes**: Applied to §8.1 verbatim.

---

### 13. How the two searched fields combine

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §4.1, §4.3

**Problem**:
A session is searched on its name and on its recorded directory, but not whether those are two tests or one joined string. Joined, a term can match across the seam between the end of a name and the start of a path and return a session that neither field contains — a row the user cannot account for even with the directory shown beside it, which is exactly the failure the display requirement exists to prevent.

**Proposal**:
Two separate tests: a session matches when the term appears as a run in its name, or as a run in its recorded directory. That is what "matches on its name and on its recorded directory" says, and it is the only version whose results a user can reconstruct from what the row shows them.

**Proposed Text**:
Add to §4.3, after the paragraph beginning "**The sigil matches by containment**":

> The two fields are tested separately: a session matches when the term appears as a run in its name, or as a run in its recorded directory. They are never joined into a single string to be searched — a term matching across the join would return a session that neither field contains, and no row could account for it.

**Resolution**: Approved
**Notes**: Applied to §4.3 verbatim.

---

### 14. The directory's colour is described by role but never named

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §6.2

**Problem**:
The directory is to be rendered in "the muted rung of the text ramp", the role paths, counts and subtitles already take. The picker's closed colour vocabulary holds more than one plausible muted role, so a builder picks, and the directory can land a shade off the window count sitting beside it on the same row.

**Proposal**:
Name it by the neighbour rather than by the ramp: the directory takes the same colour token as the row's window count. That is one of the roles the section already points at, it sits on the same row so any mismatch would be visible, and it adds nothing to the token vocabulary.

**Current**:
> Alongside the name rather than on its own line, rendered in the muted rung of the text ramp — the role paths, counts and subtitles already take elsewhere in the picker.

**Proposed Text**:
> Alongside the name rather than on its own line, taking the same colour token as the row's window count — the muted rung of the text ramp, the role paths, counts and subtitles already take elsewhere in the picker.

**Resolution**: Pending
**Notes**:

---
