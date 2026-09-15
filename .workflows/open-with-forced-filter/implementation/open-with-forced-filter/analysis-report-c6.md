# Analysis Report: Open With Forced Filter (Cycle 6)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 3

## Summary

Duplication found nothing: every rule this feature introduces has one home and every consumer reaches it there. The four findings from standards and architecture are all seam-quality and non-overlapping — a bootstrap-warning message field carrying two provenances with no merge rule that fits both, a containment filter whose stale-source fallback is reachable through events the specification says reproduce the containment set, a blocking tmux read on the render goroutine at the loading-page transition, and a completer that offers a candidate on a line the Args validator refuses. The first three are staged; the fourth is recorded as a spec defect, since the code implements the completion rule as written and only the specification can say how far it reaches.

## Spec Defects

### S1: The completion rule's separator bound is the only one the specification writes, though the reason it gives for it covers every other non-composing case

- **Claim**: The specification's completion section states the offered set unconditionally — "The words offered are the searched set's session names the typed term prefixes" — and bounds the sigil arm in exactly one place: "**Completion is bounded by the `--` separator, as recognition is.** A `/word` the separator leaves among the trailing command's own arguments is that command's argument, never a sigil, so it is completed against session names like any other word rather than offered sigil candidates — offering one there would complete to a term the parser will not honour, and would suppress the filenames the user actually wanted in the same breath." Elsewhere the specification makes the sigil a whole-invocation mode: "`portal open /term` is a complete invocation, and nothing else belongs on the line", with a second target on the line a usage error.
- **Observed**: `completeOpenPositional` (`cmd/completion.go:93-98`) gates the sigil arm on the word's shape and on the separator bound, and on nothing else, so `x api /po<TAB>` is offered `/portal-a1b2` — a line `validateSearchFormCollisions` (`cmd/open_search.go:77-107`) refuses with `cannot use a /term search with another target`. The user accepts a suggestion, presses Enter, and is refused, which reads as Portal contradicting itself. The divergence is deliberate and pinned twice: `cmd/completion_test.go:621-629` asserts it as intended behaviour, and cycle 5's approved Task 3 carried it as an acceptance criterion ("Pre-dash completion is unchanged … `portal __complete open api /po` still takes the sigil arm"). The corrigendum dated 2026-09-15 that added the separator bound to the specification did not extend it to any other case.
- **Read**: genuinely open. The code is faithful to the rule as written, so it cannot be called wrong against a rule that was never written — but the reason the specification gives for the one bound it does write applies verbatim to every other case the composition rule refuses (another pre-dash positional, `-f`, a domain pin, `-e`, `--ack`), and the specification never says whether it stops at the separator on purpose. Both sides are defensible and the choice is the user's: a completer completes the word being typed, and the user may yet delete the other target before pressing Enter — or a completer never assembles a line the validator will refuse, which is the principle the separator bound was added under. Note the asymmetry that cuts toward the current code: past the separator the word is genuinely not a sigil and offering session names there also suppresses the filenames the user wanted, while pre-dash the word *is* a sigil and only the surrounding line is illegal. Two consequences follow the answer: the composition rule would have one home shared by the validator and the completer rather than one full statement and one fragment, and the second-positional session-name arm must stay whichever way it goes, since multi-target bursts are legal.

## Discarded Findings

None. Every finding either became a proposal or is recorded above.
