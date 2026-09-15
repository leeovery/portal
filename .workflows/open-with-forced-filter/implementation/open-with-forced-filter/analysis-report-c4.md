# Analysis Report: Open With Forced Filter (Cycle 4)

## Stats

- Total findings: 6
- Deduplicated findings: 5
- Proposed tasks: 5

## Summary

Three agents found six issues, two of which are the same function reached from different angles: `completeSearchTerm` re-authors both the sigil's shape test and the searched-set rule, each of which has a declared home elsewhere. The rest of the cycle is one theme — a rule with a single home that a second site restates or fails to enforce: the progress-channel subscription authored per `Init` branch (already missed once on the command-pending arm), the search outcome recorded in a field production never reads while the teardown re-derives it, and the landing union flattened into two independent `Deps` fields. The one non-consolidation finding is the search-results capture fixture, whose hardcoded `/home/user` directories mean the home-abbreviation rule the frame exists to show does not render on the live-view route the project documents.

## Comment Corrections

- cmd/open_search.go:190 — "either" points at the branch above it, which does paint a picker
  OLD: // This route paints no picker either, so the buffered warnings go to the
  NEW: // This route paints no picker, so the buffered warnings go to the

- cmd/open_search.go:172-174 — the stated reason for deferring the count is false for a warm-but-unlatched server, which the same branch takes (the concurrent path gates on `!latchSatisfied`, not on `has-server` — `cmd/root.go:189-193`) and which answers with its live sessions rather than zero
  OLD: // An invocation whose bootstrap runs in a goroutine behind the picker's loading
  // page has no server to count against yet — every term would answer zero — so it
  // hands the count to the picker instead of taking it here.
  NEW: // An invocation whose bootstrap runs in a goroutine behind the picker's loading
  // page has no settled session list to count against yet — restore has not run — so
  // it hands the count to the picker, which takes it once that bootstrap completes.

## Discarded Findings

None. Every finding named a failure it prevents, and none reverses a settled direction — Task 1 extends the ad-hoc pass's approved receiver wiring (the receiver still reaches the command-pending branch, from one site instead of three), Task 3 extends cycle 3's approved `WarningsOwedAtTeardown` (the answer stays in the model, now read from the state that records it), and Task 4 extends cycle 3's approved `pickerLanding` union to the boundary that union is flattened across.

## Notes on synthesis

- The duplication agent's sigil-shape finding (`cmd/completion.go:76`) and the architecture agent's searched-set finding (`cmd/completion.go:70-87`) are merged into Task 2: both are restatements inside the same fifteen-line function, the duplication agent's own description names the second beside the first, and two proposals editing that body could not be executed independently.
- One fork was settled rather than staged, and it is not a Decision: whether to make `searchAttached` load-bearing or delete it is a choice of internal representation with no product end state either side, and cycle 3's settled placement of the teardown answer "beside the state it reads" breaks the tie. Recorded in Task 3's Solution with its derivation.
- No spec defects. The standards pass reports the feature's spec-driven behaviour — recognition rule, match-count outcomes, the composes-with-nothing refusals and their pre-bootstrap timing, the directory column's placement and truncation, the cold-path warning routing and the three-shell completion correction — conforming throughout, with README and `open --help` carrying every documented obligation.
- Task 4 is the cycle's one low-severity proposal, kept rather than filtered because it sits in the same cluster as Task 3: both are facts about the search landing whose correctness rests on discipline outside the construct that owns them.
