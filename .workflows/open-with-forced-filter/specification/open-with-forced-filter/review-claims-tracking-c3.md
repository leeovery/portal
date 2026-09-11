# Review Tracking: Open With Forced Filter - Claims Verification

## Findings

### 1. The `/` `.` `~` path test is attributed to a document that never stated it

**Source**: Tree measurement — `grep -n 'leading `/`\|`~`\|`\.`' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` and `git show 35c47ed19 -- .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`
**Category**: Source defect
**Move**: route
**Affects**: Section 11 (the correction owed to `cli-verb-surface-redesign`); the scope sentence in §1.3 that names it

**Problem**:
The record states that the `cli-verb-surface-redesign` specification defines a path argument by the leading `/` / `.` / `~` test, and that this feature owes that document a correction narrowing the test. That document has never carried such a test. Its target-resolution section defines the path domain only as "**path** (existing directory)" inside the chain "exact session name → path → alias → zoxide query" — the three-character shape test lives in the code (`internal/resolver/path.go`), not in that specification. Anyone auditing the owed correction goes to the named document expecting to find that test, finds nothing matching, and cannot tell what was supposed to be corrected. Two further measured facts about that document are unchanged and sound: it does place the path domain second in the bare-positional chain, and its pinned-domain contract does hard-fail every pin without falling back to the picker.

**Evidence**:

Claim (specification §11, verbatim):
> "That specification's target-resolution section defines a path argument by the leading `/` / `.` / `~` test and places the path domain second in the bare-positional chain."

Source carrying the same claim — `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, the `filter-shortcut-form` sibling check (lines 446–449) and the Open Threads entry (lines 1110–1116):
> "Sibling check: `cli-verb-surface-redesign` specification — its resolution precedence puts the path domain second in the chain and defines a path argument by the leading `/` / `.` / `~` test."
> "**A correction is owed to the `cli-verb-surface-redesign` specification.** Its target-resolution section defines a path argument by the leading `/` / `.` / `~` test, which this feature narrows"

Measurement 1 — the named document holds no such test:
```
$ grep -n '`~`\|`\.`\|leading `/`\|`/` / ' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
478:> **Corrigendum 2026-09-11** (from `open-with-forced-filter`): "**Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**", with the path domain reached by any target carrying a leading `/` — corrected: a positional beginning with `/` and containing no further `/` is a session-search sigil that never enters the chain, so the path test is narrowed for that one shape. Multi-segment and trailing-slash paths are unaffected.
```
The only hit is the corrigendum written by this work unit. Its own target-resolution text reads:
```
$ sed -n '55,58p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
Each domain maps to an outcome per Axiom 2:
- **exact session name** → attach existing session
- **path** (existing directory) → mint new session there
```

Measurement 2 — the pre-corrigendum text of that section held no test either (so the phrasing was not edited away):
```
$ git show 35c47ed19 -- .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
-1. **Glob pre-check.** If the target contains glob metacharacters (`*`, `?`, `[…]`), it is **session-domain by construction**: …
-2. **Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**.
```

Measurement 3 — the test the claim describes is in the tree, not in that document:
```
$ sed -n '14p' internal/resolver/path.go
	return strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'
```

Measurement 4 — what that document does say, both claims measured sound:
```
$ sed -n '53,57p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
3. **Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**.
$ sed -n '114,116p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md
### Pinned-domain contract — never falls back to the picker
**Every domain pin (`-s`, `-p`, `-z`, `-a`) hard-fails on unresolvable and never falls back to the TUI picker** …
```

**Proposed Text**:

**Resolution**: Routed
**Notes**: Routed to `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md` (the `filter-shortcut-form` sibling check and the Summary's Open Threads entry) and repaired in place: the sibling names its path domain semantically and the character test lives in `internal/resolver/path.go`; what this feature changes in the sibling's own terms is the precedence chain. The correction itself is still owed and the landed corrigendum already stated it against the chain sentence. Specification §11 re-aligned, and the corrigendum's own gloss in `cli-verb-surface-redesign` tightened (commit e6f388d88) because it repeated the same misattribution.
