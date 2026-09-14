# Consolidation Findings: open-with-forced-filter (Phase 8)

## Findings

### F1: The README promises the sigil completes against the same names `x <TAB>` offers
- **Class**: drift
- **Failure**: A user inside tmux presses `x /<TAB>` and the session they are attached to is absent from the candidates, while `x <TAB>` on the same line offers it. The README tells them the two completers offer the same set, so the omission reads as broken completion rather than the deliberate exclusion it is — and there is no message on either surface to account for it, because the sigil never fails. Noticed at the prompt by anyone who tab-completes a search from inside a Portal session.
- **Evidence**:
  - `README.md:175` — "`x <TAB>` offers your live session names, and `x /po<TAB>` completes the term after the slash against those same names, leaving the slash in place." The two sets are no longer the same set.
  - `cmd/completion.go:70-87` — `completeSearchTerm` holds the attached session back (`current := completionCurrentSession()` at :72, the skip at :79-81), landed by this phase's commit.
  - `cmd/completion.go:31-39` — `completeSessionNames`, which answers `x <TAB>`, `-s` and the bare positional, is unchanged and still offers every live name including the attached one.
  - `cmd/completion_test.go:52-60` — "it still offers the attached session on the plain session-name completer" pins that the asymmetry is deliberate, so the docs are the side that is wrong.
  - `cmd/open_search.go:113-121` + `internal/tui/picker_sessions.go:13-24` — the reachability rule the completer was aligned to: the searched set is the picker's set, which omits the attached session.
- **Proposed shape**: Amend the README's Tab-completion sentence so the offered sets are stated separately rather than as "those same names" — `x <TAB>` offers your live session names; `x /po<TAB>` completes the term after the slash against the sessions a search can reach, which is every live session but the one you are attached to (a search never lands you back where you already are). Documentation text only; no behaviour change. `TestReadmeDocumentsSearchForm` (`cmd/open_docs_test.go:68-92`) reads only the tokens `` `/<term>` ``, `-p /tmp`, `portal init` and `-f, --filter` from that section plus the commented `x /port` / `x /` example lines — none is the phrase being edited — so the guard stays green with no test change.
- **Bank**: reviewer entry "README tab-completion sentence now overstates the offered set for the search form" — confirmed against the final state; its specification half is reported separately as S1 rather than folded in here, since the two artifacts are corrected by different owners.

## Spec Defects

### S1: §8.1 states the offered completion set as every live session name
- **Claim**: §8.1, line 324: "The words offered are the live session names the typed term prefixes: the shell discards any candidate that is not an extension of the word being completed, so completion is prefix-shaped even though the form itself matches by containment (§4.3)." And line 326: "`/<TAB>` — the sigil with nothing after it — offers every live session name, since every name is prefixed by the empty term."
- **Observed**: `cmd/completion.go:70-87` offers the live session names the term prefixes **less the session the caller is attached to** (`:72`, `:79-81`) and less any name carrying a slash (`:76-78`). Inside tmux, `/<TAB>` therefore offers every live session name but one. `cmd/completion_test.go:394-429` pins all three cases (a prefixed term, the bare slash, and a current-session read that answers nothing, which drops nothing).
- **Read**: spec stale. The narrowing is required by the specification's own §86 — "The searched set is the set the picker lists" — which excludes the attached session (`internal/tui/picker_sessions.go:13-24`, reached from `cmd/open_search.go:120`): offering a name the search cannot reach completes to a term that lands on a picker filtered to zero rows. §8.1 was written before the offered set was tied to the searched set and never took the exclusion up; the code is right and the section owes the qualification. (The slash-bearing hold-back predates this phase — §328 covers the adjacent directory case only — and is noted for completeness rather than as this phase's defect.)
