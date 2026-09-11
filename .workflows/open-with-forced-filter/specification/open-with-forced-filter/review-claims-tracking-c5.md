# Review Tracking: Open With Forced Filter - Claims Verification

## Findings

### 1. The correction owed to the sibling specification has already landed

**Source**: Tree measurement — `grep -n 'open-with-forced-filter' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`
**Category**: Source defect
**Move**: route
**Affects**: §11 (Correction Owed to `cli-verb-surface-redesign`); §1.3's closing sentence, which names that correction as something this feature still owes

**Problem**:
The specification records an outstanding debt to the sibling `cli-verb-surface-redesign` specification: it describes that document as running *every* bare positional through `exact session name → path → alias → zoxide query`, and instructs that its live body be edited so the chain carries the leading-slash exception as a pre-check, with one corrigendum entry naming this work unit as the source. Both halves are already done. The sibling's target-resolution section now opens with a numbered **Search-sigil pre-check** step that states the no-second-slash rule, keeps `/tmp/` and multi-segment paths as path targets, and points at this specification as the owner of the form; its `## Corrigenda` section carries a dated entry attributed to `open-with-forced-filter` quoting the old chain sentence and stating the correction. Read as written, the section sends whoever picks it up to make an edit the sibling already has — a second corrigendum saying the same thing and a redundant pre-check — and it tells a reader comparing the two documents that the sibling reads a way it no longer reads.

**Evidence**:
Claim verbatim (§11, first and third paragraphs):

> That specification's target-resolution section runs every bare positional through the precedence chain `exact session name → path → alias → zoxide query`, naming the path domain semantically as an existing directory (`sed -n '55,58p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`). … What §2.2 changes in the sibling's own terms is the chain: a single-segment leading-`/` argument is session-search text and never enters it at all.
>
> … The correction lands in the sibling's `## Corrigenda` section — its live body edited so the precedence chain carries the exception as a pre-check, with one corrigendum entry naming this work unit as the source.

Measurement — the sibling's live precedence section:

```
$ sed -n '51,53p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
1. **Search-sigil pre-check.** If the target begins with `/` and contains no further `/`, it is a **session-search sigil**: the text after the `/` is forced into a session search and the target never enters the chain below. The path test in step 3 is narrowed by exactly this shape — `/tmp` is a sigil, while `/tmp/` and `/Users/leeovery/Code/portal` remain path targets. See the `open-with-forced-filter` specification, which owns the form, its outcomes, and the `-p` escape for minting at a single-segment absolute directory.
2. **Glob pre-check.** If the target contains glob metacharacters (`*`, `?`, `[…]`), it is **session-domain by construction**: expand it against live session names and skip the chain below entirely (see Glob Targets). Zero matches ⇒ unresolvable ⇒ hard fail.
3. **Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**.
```

The preceding line in that document reads `A bare positional target is resolved in three steps:` (`sed -n '49p'`), so the chain the claim quotes is now step 3 of three rather than the rule for every bare positional.

Measurement — the corrigendum entry:

```
$ grep -n 'open-with-forced-filter' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
51:1. **Search-sigil pre-check.** … See the `open-with-forced-filter` specification, which owns the form, its outcomes, and the `-p` escape for minting at a single-segment absolute directory.
478:> **Corrigendum 2026-09-11** (from `open-with-forced-filter`): "**Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**", applied to every bare positional — corrected: a positional beginning with `/` and containing no further `/` is a session-search sigil and never enters the chain, so the chain gains a pre-check ahead of the glob one. Multi-segment and trailing-slash paths are unaffected.
```

Both are committed, not working-tree edits:

```
$ git status --porcelain
(no output)
$ git log --oneline -2 -- .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
e6f388d88 specification(cli-verb-surface-redesign): tighten the corrigendum gloss to what the document stated
35c47ed19 specification(cli-verb-surface-redesign): corrigendum from open-with-forced-filter
```

The sed citation the claim carries still resolves as quoted — it is the surrounding framing, not the cited lines, that no longer holds:

```
$ sed -n '55,58p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
Each domain maps to an outcome per Axiom 2:
- **exact session name** → attach existing session
- **path** (existing directory) → mint new session there
- **alias** (known alias key) → mint at aliased dir
```

Source carrying the same assertion — `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, the `filter-shortcut-form` sibling check (lines 485–495) and the `Open Threads` entry (lines 1205–1211):

```
$ sed -n '485,489p' .workflows/open-with-forced-filter/discussion/open-with-forced-filter.md
Sibling check: `cli-verb-surface-redesign` specification — its resolution
precedence runs every bare positional through `exact session name → path → alias
→ zoxide query`, the path domain named semantically as an existing directory
(`sed -n '55,58p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`);
the leading `/` / `.` / `~` test that decides which arguments reach that domain

$ sed -n '1205,1211p' .workflows/open-with-forced-filter/discussion/open-with-forced-filter.md
- **A correction is owed to the `cli-verb-surface-redesign` specification.** Its
  target-resolution section runs every bare positional through the precedence
  chain, which this feature narrows: a single-segment leading-`/` argument becomes
  session-search text and never enters the chain at all. The correction is owed
  once this feature has a specification of its own to name as the superseding
  source; it is recorded in the `filter-shortcut-form` sibling check as well as
  here.
```

**Proposed Text**:

**Resolution**: Approved
**Notes**: Reclassified from `route` to `settled` before applying. The discussion is not defective: it recorded the correction as *owed*, which was true when written, and whether that debt has since been discharged is pipeline state rather than substance the discussion carries. What was wrong is the specification's tense — the correction was applied earlier in this session (commits 35c47ed19 and e6f388d88) while §11 still read as future work, which would have had a reader apply it twice. §11 retitled "Relationship to `cli-verb-surface-redesign`" and rewritten to record the correction as applied, with the grep that shows both halves in place; §1.3's pointer follows. No source document edited.
