# Review Tracking: Open With Forced Filter - Input Review

## Findings

### 1. Nothing records why shadowing `/tmp`, `/opt` and `/srv` is affordable

**Source**: `discussion/open-with-forced-filter.md` → `## filter-shortcut-form` → Decision, trade-off paragraph: "The user judges minting a session directly in a root-level directory as something they would never do."
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §2.4 (Accepted cost)

**Problem**:
Three command forms that work today — `x /tmp`, `x /opt`, `x /srv` — stop minting a session and start searching live ones, and the record of why that was judged affordable is missing. Whoever meets a user surprised by it has the cost and the escape hatch in front of them but nothing to weigh them against, and the obvious repair — checking whether the single-segment path exists on disk and minting when it does — quietly reintroduces a reading that differs between two machines with the same command typed on both. Every other cost this feature accepts states the ground it was accepted on; this one, the only one that overrides working behaviour, does not.

**Proposal**:
Carry the ground the source recorded alongside the cost: root-level directories are not places a session is opened in, so the population the rule shadows is one nobody mints in. This is the user's own judgement, recorded at the point the glyph was chosen.

**Current**:
Single-segment absolute directories typed *without* a trailing slash — `x /tmp`, `x /opt`, `x /srv` — stop minting and start filtering. The escape is `-p`, the pin that exists for exactly this: `portal open -p /tmp` mints there, and `portal open -p ~/Code/api -p /tmp` bursts two mints unchanged.

**Proposed Text**:
Single-segment absolute directories typed *without* a trailing slash — `x /tmp`, `x /opt`, `x /srv` — stop minting and start filtering. The escape is `-p`, the pin that exists for exactly this: `portal open -p /tmp` mints there, and `portal open -p ~/Code/api -p /tmp` bursts two mints unchanged.

The cost is affordable because of what it shadows: minting a session directly in a root-level directory is not something the user does, so the directories the rule takes out of the minting domain are ones nobody opens a session in. The recognition rule stays a test of the argument's shape and never consults the filesystem — a rule that minted when the single-segment path happened to exist would read the same command differently on two machines.

**Resolution**: Approved
**Notes**: Applied to §2.4 verbatim (the additive paragraph only; the quoted Current is unchanged).

---

### 2. Portal's own machinery sessions sit inside the search set

**Source**: No source addresses this — `discussion/open-with-forced-filter.md` speaks of "live sessions" throughout (`unambiguous-direct-attach`, `search-match-domain`) and never names the set the count is taken over.
**Category**: Gap/Ambiguity
**Move**: settled
**Affects**: §3.2 (Outcomes by match count), §4.1 (The matched fields)

**Problem**:
Portal keeps two sessions of its own on every running server — the one hosting the state daemon and the anchor that keeps a freshly-started server alive — and both carry `portal` in their names. With no statement of which sessions the search runs over, `/port` counts them among its matches, and a term like `/sav` or `/boot` matches nothing else at all: the user is attached straight into the daemon's own pane, where exiting the shell takes the daemon down with it. Short of that, the count and the list stop agreeing — a term counted as three matches opens a picker showing two rows, because those sessions are not shown in the picker at all, which is exactly the everywhere-identical answer the directory decision was taken to protect.

**Proposal**:
State that the searched set is the set the picker lists, so Portal's own internal sessions are never search candidates — neither counted toward the match total nor attachable by a term. This is determined by the specification's own requirement that the count and the list give the identical answer in the shell and in the picker.

**Proposed Text**:
(§3.2, immediately after "Let K be the number of live sessions matching the term under §4.")

The searched set is the set the picker lists. Portal's own internal sessions — the `_portal-saver` daemon host and the `_portal-bootstrap` server anchor — are absent from that list and are never search candidates: no term counts one toward K, and none can be attached by a sigil.

**Resolution**: Pending
**Notes**:
