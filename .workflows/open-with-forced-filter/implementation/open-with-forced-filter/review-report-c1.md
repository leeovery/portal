# Review Report: Open With Forced Filter (Cycle 1)

## Stats

- Total findings: 7
- Deduplicated findings: 7
- Proposed tasks: 2

## Summary

The review's prep pipeline routed seven findings into six actions plus one discard; two of them (A4, A5) carry `replan` and are the whole of this synthesis, neither blocking. A4 is a live correctness defect in the containment filter's item source — the map key is the joined filter value, which is not injective, so two distinct sessions can collapse onto one entry and one of them is ranked by the other's fields. A5 is an evidence defect: task 5-5's `cmd/init.go` bash-shim edit shipped without the measurement its own void condition was gated on, and nothing in the tree can detect the derivation behind it being wrong.

## Spec Defects

None. Neither action indicts the specification. A4's evidence indicts the code against a rule the specification states plainly (§4.4: the sigil's fields are tested separately and never joined, which `resolver.MatchesSearchTerm` implements correctly — the collapse happens in `internal/tui`'s item source, not in the rule). A5 concerns a measurement the *plan task* required; the specification does not legislate cursor handling in the shim (its §2.2/§8.3 and the 2026-09-14 corrigendum fix only the configurations in which Portal answers a bash Tab at all), so there is no specification claim for the finding to contradict. The change-set verification's five sections likewise returned zero findings.

## Discarded Findings

- Sibling `Model` accessor read only by tests (`4-3-1`) — discarded by prep: the sibling accessors are test-read too, and report-10-3 settled the residue on the record.

Not discards, recorded here so the routing is legible: `1-5-1`, `4-8-1` and `5-4-1` were routed `do-now` and were applied, verified and committed this session as `a43f29f43`; `7-1-1` (the `progressReceiver` field comment, which belongs to the cold-path startup flip) was routed `out-of-scope` and is banked in the manifest. None of the four is re-raised here.
