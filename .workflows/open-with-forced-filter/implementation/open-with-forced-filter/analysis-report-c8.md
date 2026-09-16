# Analysis Report: open-with-forced-filter (Cycle 8)

## Stats

- Total findings: 1
- Deduplicated findings: 1
- Proposed tasks: 0

## Summary

Duplication and standards both returned `FINDINGS: none` from a full fresh pass over the whole change set — every rule the feature restates resolves to one named home, and the specification (all eight corrigenda included) matches the implementation with `go build ./...`, `golangci-lint run` (0 issues) and the unit lane for the touched packages green. Architecture wrote one low-severity finding about `internal/resolver` holding a fragment of the sigil rule; it names a failure only for a caller that does not exist, so it does not clear the floor and is discarded below. One comment correction was collected and is the whole of this cycle's actionable output.

## Comment Corrections

- internal/tui/search_filter.go:39-40 — attributes the exclusion of header rows to their empty filter value, but the loop excludes them by the `item.(SessionItem)` type assertion, which never reads a filter value; the surviving half restates the dedup the `slices.Contains` line already shows in the unit the clause names.
  OLD: // Rows sharing a field pair collapse onto one entry, and a header's empty filter
  // value contributes none.
  NEW: // Rows sharing a field pair collapse onto one entry.

## Discarded Findings

- **The resolver declares a sigil "not a path" but still offers it to the two minting domains** (architecture, low) — discarded at the floor: the failure it names is reachable only by a call site that does not exist. Verified this cycle: `QueryResolver.Resolve` has exactly two non-test callers (`cmd/open.go:217` and `cmd/open_surfaces.go:36` via `ResolveBareAll`), and both sit behind `openCmd.RunE`'s search-form short-circuit and `validateSearchFormCollisions`, which together refuse or divert every line carrying a sigil before resolution runs. `IsPathArgument` itself is called from `query.go:105` and tests only. So no sigil reaches the alias or zoxide leg on any present path, nothing goes wrong, for nobody, and there is nothing to notice — the floor's test is not met by a consequence contingent on a future caller. The finding is also not new: cycle 7's architecture pass examined the identical candidate on the identical evidence and recorded it under "Examined and deliberately not written" with the same reason ("the risk is hypothetical for a caller that does not exist"), so staging it now would reverse a judgment already taken without any new measurement behind the reversal. The narrowing it objects to is deliberate and pinned — `internal/resolver/path_test.go` asserts `IsPathArgument` is false for `/port` and for a bare `/`, which is the shape rule the specification requires — and the low grade carries it out on the severity filter as well, having no pattern to cluster into.
