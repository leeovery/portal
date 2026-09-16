# Consolidation Findings: Open With Forced Filter (Phase 14)

No finding cleared the floor. The phase is one task over two files; the accumulate
change is monotonically safer than what it replaced (adding pairs under a value can
only make `everyEntryMatches` stricter, so it can never surface a row the term does
not match), the slice clone under the write lock is load-bearing rather than
defensive, and the growth it introduces is bounded by the distinct `(name, dir)`
pairs one picker run observes, deduped on write. What the phase does owe is one
comment correction and two specification claims it has outrun.

## Comment Corrections

- `internal/tui/search_filter.go:21-23` — the sentence claims a filter pass can only
  be handed targets the source has already held. `bubbles/list`'s `filterItems`
  builds `targets` from **every** item's `FilterValue()`
  (`charm.land/bubbles/v2@v2.1.0/list/list.go:1258-1262`), headers included, and a
  `HeaderItem`'s empty value is precisely the one target the source never holds —
  `set` skips it, as the next doc comment down (`:38-40`) says in as many words.
  Within this file's own vocabulary `targets` is the `FilterFunc`'s parameter, so
  the claim contradicts its own neighbour. The fix is one word; the invariant the
  sentence carries (why accumulation is sound) is worth keeping.
  OLD:
  ```
  // One source is built per picker run and every generation is recorded before its
  // items reach the list, so a pass can only be handed targets the source has
  // already held. A filter value is not injective — a session named for the text
  ```
  NEW:
  ```
  // One source is built per picker run and every generation is recorded before its
  // items reach the list, so a pass can only be handed session targets the source
  // has already held. A filter value is not injective — a session named for the text
  ```

## Spec Defects

### S1: §4.4 states the withholding rule over live sessions; the code holds it over every pair the source has ever recorded

- **Claim**: §4.4 (specification.md:180) — "Where the sessions behind one target
  disagree about the term, every row behind that target is withheld, so the narrowed
  list is a subset of the K sessions §3.2 counts".
- **Observed**: `set` (`internal/tui/search_filter.go:41-62`) folds each generation's
  pairs into what the source already holds and evicts nothing for the life of the
  source, and `containmentFilter` (`:94`) ranks a target only when *every* recorded
  pair matches. A session killed, renamed or otherwise gone from the list therefore
  keeps withholding: the sessions behind the target are now one, it agrees with the
  term, and its row still never appears. The phase pins exactly this — "it ranks
  nothing when the session the target came from matches and a since-departed
  generation's colliding entry does not"
  (`internal/tui/search_filter_test.go:251-263`).
- **Read**: spec stale. The code is right and the retention is what makes §4.4's own
  next clause true — "withholds the same rows on every one of those re-renders, so
  the list is narrower than the containment set rather than unstable". Evicting a
  departed pair would make the withholding blink off mid-picker (the instability
  §4.4 rules out) and would reopen the stale-pass hazard this phase closed. §4.4 has
  no text for the pairs the source retains, and the user-visible residual — a row
  the user cannot recover by killing or renaming the collider, only by relaunching
  the picker — is an accepted cost the specification does not currently record.

### S2: §4.4 still describes the stale-pass omission the phase removed

- **Claim**: §4.4 (specification.md:180) — "A filter pass answering for a list
  generation the source has since rebuilt omits in the same direction."
- **Observed**: that omission is what this phase deleted. The source now holds every
  generation's pairs (`internal/tui/search_filter.go:18-20`, `:41-49`), so a pass
  belonging to an older generation resolves its own targets by containment and ranks
  them rather than finding the source empty of them —
  `internal/tui/search_filter_test.go:265-277` sets two generations in turn and then
  ranks both generations' targets. After the change such a pass omits only when a
  later generation recorded a *disagreeing* pair under the same filter value, which
  is the collision case S1 covers, not the rebuild.
- **Read**: spec stale. The sentence understates the tree: it tells a reader rows can
  go missing purely because a rebuild landed under them, which no longer happens.
  The surrounding conclusion — "both fail toward showing less, and neither widens the
  set" — still holds.
