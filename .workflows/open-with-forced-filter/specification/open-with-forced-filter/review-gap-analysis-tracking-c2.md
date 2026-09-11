# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. A malformed search line boots the machine before refusing it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §5.1 (Argv Composition — the rule), §7.1–§7.2 (Cold-Path Classification), §3.7 (the zero-match failure)

**Problem**:
A line that pairs a session search with anything else — a second target, a trailing command, a domain pin — is refused. Nothing says *when* it is refused on a cold machine. The invocation is classified as picker-bound from the shape of its arguments alone, before anything is resolved, so a mistyped `x /port -f port` can start the tmux server, restore every saved session and hold the loading page for several seconds before printing a usage error into the frame the TUI has just vacated. The specification answers exactly this timing question for a search that matches nothing — the miss arrives after the loading page, because the count cannot be taken any earlier — and leaves the malformed line unanswered, so two builders will make opposite calls and one of them turns a typo into a multi-second cold boot.

**Proposal**:
A refused line needs nothing from tmux in order to be refused, so it is refused before anything starts: no server start, no restore, no loading page, the message on stderr and the ordinary usage exit status. The picker classification applies to a complete search invocation only. What determines it is the distinction the document already draws — the match count is the one thing on this path that must wait for a live session list, and an argument-shape refusal is not.

**Current**:
| Line | Outcome |
|---|---|
| `portal open /term` | the sigil, per §3 |
| `portal open /term -e <cmd>` | usage error |
| `portal open /term -- <cmd>` | usage error |
| `portal open /term <other-target>` | usage error, at every arity |
| `portal open <other-target> /term` | usage error — recognition is positional-independent (§2.3) |
| `portal open /term -s|-p|-a|-z <value>` | usage error |
| `portal open /term -f <text>` | usage error |
| `portal open /term --ack <batch>:<token>` | usage error |

**Proposed Text**:
| Line | Outcome |
|---|---|
| `portal open /term` | the sigil, per §3 |
| `portal open /term -e <cmd>` | usage error |
| `portal open /term -- <cmd>` | usage error |
| `portal open /term <other-target>` | usage error, at every arity |
| `portal open <other-target> /term` | usage error — recognition is positional-independent (§2.3) |
| `portal open /term /other` | usage error — a second sigil is another target |
| `portal open /term -s\|-p\|-a\|-z <value>` | usage error |
| `portal open /term -f <text>` | usage error |
| `portal open /term --ack <batch>:<token>` | usage error |

**A refused line starts nothing.** The refusal is decided from the arguments alone, so it needs no tmux server and takes no loading page: on a cold machine as on a warm one, a refused line prints its usage error and exits without starting the server, restoring a session or painting a frame. The picker classification of §7.1 applies to a complete sigil invocation only. That is the one respect in which a usage error differs from the zero-match failure (§3.7), which can only be discovered once a live session list exists.

**Resolution**: Pending
**Notes**: The `/term /other` row is also finding 6; if that finding is declined, drop that row and keep the paragraph.

---

### 2. A directory the picker guesses for grouping can leak into the search and the column

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §4.1 (The matched fields), §6.1 (Search result display — the requirement), §4.4 (the rule by entry point)

**Problem**:
A session created before the directory stamp shipped carries no recorded directory: it is findable by name alone and its row shows no path. But the picker fills in a missing directory itself whenever it groups the list — it asks the session's pane where it is and keeps the answer for the rest of that picker. So a user who lands in a grouped view can watch the same session acquire a path partway through: findable by a directory it was not findable by a moment earlier, and showing a path beside a name that showed none. That is precisely the view-dependent behaviour the feature rules out, reached from inside one picker rather than across launches. The document forbids the derived value without saying what happens once the picker has already produced one for its own purposes, and the field a builder reaches for is the same field the derivation writes into.

**Proposal**:
State that only the stamped value is ever matched or displayed, and that a directory the picker derived in order to group the list never becomes either. What determines it is the requirement already carried: the count taken in the shell and the list shown in the picker must answer identically, which a mid-picker derivation breaks.

**Current**:
**Known limitation, unconditional and self-correcting:** a session created before the directory stamp shipped carries no recorded directory and matches on name alone, in every view. Such sessions age out as they are killed and replaced. Deriving the missing value everywhere was rejected for its cost — one pane read per unrecorded session on every `/term`.

**Proposed Text**:
**Known limitation, unconditional and self-correcting:** a session created before the directory stamp shipped carries no recorded directory and matches on name alone, in every view. Such sessions age out as they are killed and replaced. Deriving the missing value everywhere was rejected for its cost — one pane read per unrecorded session on every `/term`.

This holds *within* a single picker as well as across launches. The grouped views derive a missing directory while the sigil's own narrowed list is on screen, and retain what they derive; that value belongs to grouping and to nothing else. Neither the match (§4.4) nor the directory column (§6.1) ever reads it, so a regroup can never make a session findable by a path it was not findable by a moment earlier, and can never put a path beside a name that showed none.

**Resolution**: Pending
**Notes**:

---

### 3. Two sections say opposite things about what Tab does with a slash-leading word

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §2.2 (The recognition rule), §8.1 (Completion looks past the sigil)

**Problem**:
The recognition rule tells the reader that Portal returns no candidates for a word beginning with a slash and that `x /tm<TAB>` therefore completes to nothing. The completion requirement says the opposite is to be built: a word beginning with a slash completes against live session names, and `/po<TAB><Enter>` is meant to become the whole interaction. A builder who follows the first returns no candidates and the headline interaction of the feature never arrives; a builder who follows the second leaves a passage of the document asserting the form is inert. The durable point the recognition rule actually needs — that completion never turns `/tm` into `/tmp/`, so a trailing slash is always something the user typed — survives the change and is worth keeping; the claim of no candidates does not.

**Current**:
Completion offers no help telling the two apart. Portal returns no candidates for a slash-leading word and switches the shell's filename fallback off (`portal __complete open /tm` → no candidates, `ShellCompDirectiveNoFileComp`; `portal completion bash | grep -n 'compopt +o default'`), so `x /tm<TAB>` completes to nothing rather than to `/tmp/`. The trailing slash that keeps an argument a path is therefore one the user types, and a single-segment absolute directory typed without it is a sigil however it was reached.

**Proposed Text**:
Completion cannot turn one shape into the other. The words it offers for a slash-leading argument are live session names carrying the sigil, never directories (§8.1), and Portal switches the shell's filename fallback off (`portal completion bash | grep -n 'compopt +o default'`) — so `x /tm<TAB>` never becomes `/tmp/`. The trailing slash that keeps an argument a path is therefore always one the user types, and a single-segment absolute directory typed without it is a sigil however it was reached.

**Resolution**: Pending
**Notes**:

---

### 4. Glob characters in a search term have no stated reading

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §4.3 (The matching rule), §2.2 (The recognition rule)

**Problem**:
The neighbouring way to reach live sessions takes globs — a quoted `port*` opens a window per match — so a user who has that in their fingers will type `/port*`. Nothing says whether the star is a wildcard or a character to find. One reading makes the form a glob that narrows instead of bursting; the other makes `/port*` match only a session whose name or path literally contains `port*`, which is to say nothing at all. The two differ on every term a user copies across from the glob form.

**Proposal**:
The term is literal text: containment tests the characters as typed, so `*`, `?` and `[` carry no special meaning on this path. What determines it is the stated rule — the typed characters appearing as a run in the matched text — together with the glob form's answer to ambiguity being a window per match, which is the ceremony this form exists to replace.

**Proposed Text**:
**The term is literal text.** `*`, `?` and `[` are characters to find rather than wildcards: `/port*` matches a session whose name or recorded directory contains `port*`, and nothing else does. The glob forms answer ambiguity by opening a window per match; the sigil answers it by narrowing, and the two rules are not mixed.

**Resolution**: Pending
**Notes**: Lands as a paragraph in §4.3, after the containment rule.

---

### 5. Two searches on one line have no stated outcome

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §5.1 (Argv Composition — the rule)

**Problem**:
`portal open /api /port` is missing from a composition table that otherwise enumerates line shapes down to arity and argument order. Two searches on one line is the shape a user reaches for when they want two windows, and the natural wrong guess — run them as a two-window burst — is exactly what the form refuses for every other pair of targets.

**Proposal**:
A second sigil is another target and the line is the same usage error. What determines it is the rule that the form composes with nothing, plus the refusal of the multi-window burst.

**Current**:
| `portal open <other-target> /term` | usage error — recognition is positional-independent (§2.3) |

**Proposed Text**:
| `portal open <other-target> /term` | usage error — recognition is positional-independent (§2.3) |
| `portal open /term /other` | usage error — a second sigil is another target |

**Resolution**: Pending
**Notes**: Same row as the table in finding 1; apply once.

---

### 6. Tab on a bare slash is pulled two ways

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §8.1 (Completion looks past the sigil), §2.5 (The degenerate form)

**Problem**:
`x /<TAB>` is the first thing a user tries once they know the form exists, and the document points both ways: a bare slash with no term is a usage error, while completion offers the live session names the typed term prefixes — which, for an empty term, is every one of them. A builder reading the first suppresses the candidates and Tab is dead at the moment it is most useful; a builder reading the second offers the whole session list. Nothing settles which.

**Proposal**:
Offer every live session name, each carrying the sigil. What determines it is the completion rule as stated — every name is prefixed by the empty term — and the fact that the bare slash is an error only when it is run, which is what Tab exists to save the user from.

**Proposed Text**:
`/<TAB>` — the sigil with nothing after it — offers every live session name, since every name is prefixed by the empty term. The bare slash is a usage error only when it is run (§2.5), which is precisely what makes completing it worth doing.

**Resolution**: Pending
**Notes**: Lands in §8.1, after the paragraph on which words are offered.

---

### 7. A cold-boot miss says nothing about warnings that were accumulated on the way

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §3.7 (the zero-match failure), §7.5 (a sigil that resolves to a direct attach)

**Problem**:
On a cold machine the bootstrap can finish with soft warnings — the saver failed to come up, saved state was corrupt — and the notice band that normally carries them never appears when the search finds nothing. The document says where those warnings go when the search resolves to an attach and is silent when it resolves to a miss, so a user whose reboot left the daemon down may get only "no session matched" and never learn the rest, or may get both, depending on who builds it. The warning is the more important of the two messages.

**Proposal**:
Same route, same answer: the accumulated warnings are written to the terminal after teardown, ahead of the miss message. What determines it is the reason already given for the attach case — the notice band never surfaces, so the terminal is the only place left.

**Current**:
**On a cold boot the miss arrives after the loading page and reads identically.** A sigil invocation takes the loading page before K can be taken (§7), so a K = 0 on a cold server is discovered with the TUI already on screen. The TUI closes, the same message is written to the terminal, and the exit status is the same non-zero one the warm path returns. This is not a bootstrap fatal and does not take the in-TUI error frame; the picker never appears, exactly as §3.2 requires.

**Proposed Text**:
**On a cold boot the miss arrives after the loading page and reads identically.** A sigil invocation takes the loading page before K can be taken (§7), so a K = 0 on a cold server is discovered with the TUI already on screen. The TUI closes, the same message is written to the terminal, and the exit status is the same non-zero one the warm path returns. Any soft bootstrap warnings accumulated on the way out take the same route as they do on a resolved attach (§7.5) — written to the terminal after teardown, ahead of the message. This is not a bootstrap fatal and does not take the in-TUI error frame; the picker never appears, exactly as §3.2 requires.

**Resolution**: Pending
**Notes**:

---

### 8. The documentation deliverable carries nothing about Tab

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §9.2 (What the documentation must carry), §8.3 (The correction), §8.5 (Rollout consequence)

**Problem**:
The list of what the documentation must carry is a closed one and says nothing about completion. Two user-visible things go undocumented as a result: that the form completes against live session names after the slash, and that an existing install keeps completing the way it did until the user re-evaluates `portal init` or opens a new shell. An upgrading user whose `x <TAB>` still offers subcommand names has nothing to read that explains it and no reason to suspect re-running init would fix it — which is the whole deliverable, the form becoming muscle memory, failing quietly for every install already out there.

**Proposal**:
Add both to the documentation list. What determines it is the feature's own standard for the completion work: a completion decision that does not reach the user's fingers has not been delivered.

**Current**:
- The sigil form and its recognition rule, including the single-segment absolute-directory cost and the `-p` escape (§2.2, §2.4).
- The three outcomes by match count (§3.2).
- What the search matches, and that it matches by containment while the picker's own filter stays fuzzy (§4).
- That the form composes with nothing (§5.1).
- `-f` and `/term` distinguished by outcome rather than by input shape, and which of the two to reach for (§9.1).

**Proposed Text**:
- The sigil form and its recognition rule, including the single-segment absolute-directory cost and the `-p` escape (§2.2, §2.4).
- The three outcomes by match count (§3.2).
- What the search matches, and that it matches by containment while the picker's own filter stays fuzzy (§4).
- That the form composes with nothing (§5.1).
- `-f` and `/term` distinguished by outcome rather than by input shape, and which of the two to reach for (§9.1).
- That the form completes against live session names after the slash (§8.1).
- That the corrected completion reaches an existing install only once the output of `portal init` is re-evaluated — a new shell, or re-running `portal init` (§8.3, §8.5).

**Resolution**: Pending
**Notes**:

---

### 9. The rejection of project-prefix session matching is stated twice in full

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §10.1 (The bare-positional resolution chain), §3.1 (The sigil is session-domain by declaration)

**Problem**:
Why a bare `api` must not resolve to the sole live `api-*` session — the attach-versus-create guess and its cliff at the second match — is written out twice, with the same example and the same reasoning, in the section that argues the search is safe and again in the out-of-scope section. Two copies of one rule drift apart under later edits and come back as a contradiction, and a reader who meets the second copy has to work out whether it is saying something new.

**Proposal**:
Keep the reasoning where it does work — the section that uses it to show why declaring the domain first makes eager resolution safe — and reduce the out-of-scope entry to a pointer.

**Current**:
**Unchanged.** Its ordering, its domains, and the sibling `cli-verb-surface-redesign` specification's Axiom 2 (no find-or-create) stand exactly as that specification set them. Project-prefix session matching stays rejected, on that specification's own grounds — `api` resolving to the sole live `api-*` session reintroduces attach-versus-create guessing with an ambiguity cliff the moment a second `api-*` session exists.

**Proposed Text**:
**Unchanged.** Its ordering, its domains, and the sibling `cli-verb-surface-redesign` specification's Axiom 2 (no find-or-create) stand exactly as that specification set them. Project-prefix session matching stays rejected, on that specification's own grounds (§3.1).

**Resolution**: Pending
**Notes**:

---

### 10. The one-match glob attach is restated where a pointer would do

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §3.2 (Outcomes by match count), §1.1 (The gap this fills)

**Problem**:
That a session glob matching exactly one session attaches it outright is stated where the surface is described and again where the match-count outcomes are set out. The second copy is doing rhetorical work — establishing that eager attach is not new — which a reference carries just as well, and the two copies are a pair that can disagree after a later edit to either.

**Current**:
Eager attach on a single session-search match is already shipped behaviour rather than a new one: `-s <glob>` matching exactly one session attaches it outright (§1.1).

**Proposed Text**:
Eager attach on a single session-search match is already shipped behaviour rather than a new one (§1.1).

**Resolution**: Pending
**Notes**:

---

## Working Notes

Full fresh pass over the whole specification for cycle 2; no prior cycle's tracking file was read, so findings here are independent of what earlier cycles raised or resolved.

Checked and found sound, recorded so a later cycle need not re-derive them: the recognition rule against every argument shape it admits (`/`, `//x`, `/.`, a term that can never itself contain a slash); the count-versus-list agreement across grouping modes, previews, regroups and refreshes; the committed-filter landing and the character-identical test that governs when the picker's own rule returns; the cold-path classification against the K = 0 and K = 1 outcomes and where warnings land in each; the burst's non-participation; the truncation ladder's floor and the home abbreviation; and the corrigendum owed to the sibling specification.

Not raised, deliberately: the exact wording of the zero-match message and of the empty-term refusal, both explicitly delegated to an existing house convention with their required content stated; the per-shell mechanism for re-pointing the emitted function's completion, which is a build decision rather than a behaviour the specification owes; and the absence of a directory column on lists reached by `-f` or a hand-typed filter, which the scope section decides rather than leaves open.
