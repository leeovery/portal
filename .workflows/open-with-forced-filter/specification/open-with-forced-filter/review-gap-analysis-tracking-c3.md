# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. On a cold boot, the point at which the match count is taken is left open between "restore finished" and "bootstrap finished"

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Important
**Affects**: §3.4 (When the count is taken), §7.4, §7.6

**Problem**:
On a cold machine the search count can only be taken once the saved sessions exist, and the specification places that moment "after restore has reconstructed the saved sessions". Restore is one step of a ten-step startup sequence, and four steps follow it — among them the one that clears the restoring marker, which the startup contract treats as fatal if it does not happen. A builder who reads the count as takeable at the end of restore will, on a single match, hand the process over to tmux while those steps are still outstanding: the process is replaced, the marker is left set, and the orphan-FIFO and stale-marker cleanups never run. The marker outliving startup suppresses session capture and the stale-hook sweep until the next cold boot — a silent, durable regression triggered by the most ordinary use of the feature.

**Proposal**:
State that the count is taken only once the concurrent startup has run to completion, not at the end of restore, and that nothing the search decides fires earlier. The specification's own text determines it: the warnings delivered on a single-match attach are the *accumulated* soft warnings, which exist in full only once every step has run, and the concurrent startup path is declared unchanged by this feature.

**Current**:
K is evaluated against the live session set once the tmux server is ready to answer for it. On a warm server that is immediately. On a cold server the sigil takes the picker's concurrent-bootstrap path (§7), so the count is taken after restore has reconstructed the saved sessions — the loading page appears first and is replaced by the attach when K turns out to be 1.

**Proposed Text**:
K is evaluated against the live session set once the tmux server is ready to answer for it. On a warm server that is immediately. On a cold server the sigil takes the picker's concurrent-bootstrap path (§7), so the count is taken once that bootstrap has run to completion — every step of it, not merely the restore that reconstructs the saved sessions. Nothing the sigil decides fires earlier: the loading page stands until the count can be taken, and is then replaced by the attach when K turns out to be 1, or by the failure when it turns out to be 0. Acting at the end of restore would replace the process mid-bootstrap and abandon the steps that follow it — among them the clearing of the `@portal-restoring` marker, which must not outlive bootstrap.

**Resolution**: Approved
**Notes**: Applied to §3.4 verbatim.

---

### 2. Whether the directory is matched in its home-abbreviated form on the other two filter routes is left to the builder

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §4.1 (The searched form is the displayed form), §4.2, §4.4

**Problem**:
The directory a session was opened in becomes searchable on all three routes into the sessions list — the new slash form, `-f`, and the filter typed by hand in the picker. The specification pins the *form* that directory is searched in — abbreviated to `~/…` rather than the absolute path tmux recorded — inside the argument that justifies it for the slash form only, and the reason it gives (what was matched is what is shown) does not hold for the other two, which show no directory at all. A builder wiring the one shared filter value can therefore reasonably feed it the raw absolute path, and then every session on the machine matches `user`, `home` or the user's own account name the moment those letters are typed into the picker — the false-positive flood the whole matching section is written to prevent, arriving on the two routes that were never examined for it.

**Proposal**:
Say in the section that fixes the shared fields that the *form* of the directory is shared too: all three routes match the home-abbreviated value, and only the rule (containment against fuzzy) diverges. The specification's own decision determines it — the fields are shared by ruling, the abbreviation is a property of the field's value rather than of the slash form's rule, and the alternative reintroduces the false-positive set §4 exists to bound.

**Proposed Text**:
The form is shared with them too: all three match the recorded directory home-abbreviated, exactly as §4.1 fixes it, never the absolute path tmux recorded. The abbreviation belongs to the value, not to the sigil's rule — what diverges between the three entry points is the rule (§4.4) and nothing else.

**Resolution**: Approved
**Notes**: Applied to §4.2 verbatim.

---

### 3. A live session list that cannot be read is not distinguished from a search that matched nothing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §3.2 (Outcomes by match count), §3.7

**Problem**:
The feature introduces a read of the live session list on a path that previously took none, and the specification accounts for only one of its two failure modes: it says what happens when the read succeeds and matches nothing. If the read itself fails, a builder counting matches from whatever came back has zero of them, and the user is told no live session matched their term — while their sessions are all still running. The message is confidently wrong at exactly the moment the user needs to know something went wrong with tmux, and the specification is otherwise careful that this message names the search that found nothing.

**Proposal**:
Say that a failed read of the session list is reported as the failure it is and never as a zero-match miss. Determined by §3.7's own requirement that the zero-match message name the search that found nothing — a read that failed has not searched.

**Proposed Text**:
**A session list that could not be read is not a zero match.** K = 0 says the search ran and found nothing; a failed read has searched nothing, and reporting it as a miss tells the user their sessions are gone when they are running. Such a failure is reported in tmux's own terms rather than the zero-match wording, and exits non-zero. On a cold boot it reaches the user by the same route as the zero-match failure — the TUI closes and the message follows teardown — and, like it, is not a bootstrap fatal and takes no in-TUI error frame.

**Resolution**: Approved
**Notes**: Applied to §3.7, then re-worded when the user reversed the zero-match decision later in the same sitting: K = 0 now opens the picker, so the finding's distinction stands but no longer contrasts against a zero-match *message*. A failed tmux read remains the one failure path on this form.

---

### 4. The user who meant to create a session in a root-level directory is never pointed at the escape

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: choice
**Priority**: Important
**Affects**: §2.4 (Accepted cost), §3.7 (The zero-match failure)

**Problem**:
`x /tmp` stops creating a session there and starts searching — the one behaviour change this feature knowingly takes from the existing surface. The user who types it meaning the old thing is answered with "no live session matched tmp", and the specification decides only what that message must *not* say (it must not point at `-f`). Whether it names `-p`, the single route back to the outcome they wanted, is left open — so the escape may exist only in the README, reached by a user who does not yet know there is anything to look up. Pinned either way, the wording is one line; unpinned, half the builders will ship the version that strands the user.

**Options**:
- The zero-match message never names `-p`; the shadowed-directory cost is carried by the documentation alone (§9.2), and the miss reads purely as a search miss.
- The zero-match message always names `-p <dir>` beside the no-match statement — one extra line on every search miss, correct advice whether or not the term was meant as a directory.
- The zero-match message names `-p` only when the term, read as a single-segment absolute path, exists on disk — the recognition rule stays filesystem-free as §2.4 requires, while the message written after the failure is free to look (recommended).

**Resolution**: Declined
**Notes**: Moot. The user reversed the zero-match decision in this sitting — `/term` never errors; a term matching nothing opens the picker on an empty list, exactly as filtering to nothing inside the picker does, and `x /` opens the picker with an empty focused filter. There is no zero-match message, so there is nothing to point at `-p` from. The shadowed-directory cost is carried by §2.4 and the documentation (§9.2) as before. The reversal is recorded in the discussion's `unambiguous-direct-attach` and `filter-shortcut-form` Decisions (commit 33c8707a9).

---

### 5. `-f`'s widened match domain is restated where it is only referenced

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §5.5 (What `-f` keeps)

**Problem**:
Which fields `-f` narrows on after this feature is fixed once, in the section that rules the fields shared across all three entry points and in the table beside it. §5.5 states the same fields a second time in prose, immediately after referencing the section that holds them. Two statements of one rule drift apart under a later edit and come back as a contradiction between the table and the prose.

**Proposal**:
Keep §5.5's reference and drop the restatement of the fields behind it; the fields' home is §4.2 and its table in §4.4.

**Current**:
`-f/--filter` keeps its argument contract, its fuzzy matching rule and its Projects redirect under a pending command. Its matched fields widen with everything else's (§4.2): after this feature `-f` narrows on session name and recorded directory, exactly as the sigil and the hand-typed filter do. The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9).

**Proposed Text**:
`-f/--filter` keeps its argument contract, its fuzzy matching rule and its Projects redirect under a pending command. Its matched fields widen with everything else's (§4.2). The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9).

**Resolution**: Approved
**Notes**: Applied to §5.5 verbatim.

---

### 6. The out-of-scope entry for the picker copies two rules that are settled elsewhere

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §10.6 (The picker reached any other way)

**Problem**:
That the picker's own filter keeps its fuzzy, rank-sorted behaviour is settled where the matching rules are set out, and that rows reached by any other route render as they do today is settled where the directory column's scope is set. §10.6 restates both, adding nothing of its own — so a later change to either rule has two places to land and will only reach one.

**Proposal**:
Keep the scope boundary the entry exists to draw and let it point at the two rules rather than reproduce them.

**Current**:
#### 10.6 The picker reached any other way

**Unchanged.** The picker's own filter keeps its fuzzy, rank-sorted behaviour (§4.4), and Sessions rows reached by any route other than the sigil render as they do today (§6.3).

**Proposed Text**:
#### 10.6 The picker reached any other way

**Unchanged** — its own filter as §4.4 sets it, its rows as §6.3 scopes them.

**Resolution**: Pending
**Notes**:

---

### 7. The suppressed filename fallback is stated twice

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §8.1 (Completion looks past the sigil)

**Problem**:
That Portal turns the shell's filename fallback off is established once, with its measurement, where the recognition rule depends on it — it is why a trailing slash is always one the user typed. §8.1 states the same fact again without the measurement. Two copies of one behaviour, and only one of them is anchored to evidence.

**Proposal**:
Leave the fact and its measurement at §2.2 and have §8.1 point at it.

**Current**:
Today the sigil form completes to nothing — the slash is part of the word being completed, no session name begins with one, and Portal suppresses the shell's filename fallback, so Tab is silently inert rather than misleading (`portal __complete open /po` → no candidates, `ShellCompDirectiveNoFileComp`).

**Proposed Text**:
Today the sigil form completes to nothing — the slash is part of the word being completed, no session name begins with one, and the shell's filename fallback is off (§2.2), so Tab is silently inert rather than misleading (`portal __complete open /po` → no candidates, `ShellCompDirectiveNoFileComp`).

**Resolution**: Pending
**Notes**:

---
