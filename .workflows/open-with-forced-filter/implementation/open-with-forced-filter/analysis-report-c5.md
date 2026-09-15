# Analysis Report: Open With Forced Filter (Cycle 5)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 3

## Summary

Three agents found four things, with no overlap between them: one duplication finding (the search-form collision check restates `open`'s flag set as literals and is the only one of the package's three mirrors of that set with no protection behind it), two architecture findings (four staging methods whose correctness rests on how each caller spells the call rather than on the signature, and a positional completer that restates the sigil recognition rule without the `--` bound the parser applies), and one standards finding that is a specification defect rather than a code one. Three proposals are staged; nothing was discarded, and no Decision meets the bar — all three forks are internal and settled from the record. The feature's shared rules otherwise each have exactly one home and are reached by every consumer, and the implementation matches every decision the specification records except the one paragraph below.

## Spec Defects

### S1: The cold-path section still records the command-pending picker's missing bootstrap gate as a live defect

- **Claim**: `specification.md:314` (§7.6, "The concurrent path itself is unchanged"): "It is **not** unchanged for the command-pending picker: `WithCommand` forces `PageProjects`, so that member paints an actionable picker from frame one with no loading gate between the user and a half-bootstrapped server — for the `-- <command>` spelling admitted here as much as for the pre-existing `-e <command>` one." The dated corrigendum at `specification.md:480` restates it and adds: "That behaviour is a live defect rather than a documentation gap and is tracked as this phase's own consolidation finding."
- **Observed**: The tree does the opposite, and has since phase 7's approved consolidation task landed. `createSession` (`internal/tui/model.go:1821-1843`) refuses to mint while `bootstrapInFlight()` and stages the chosen directory instead, first stage winning; the `BootstrapCompleteMsg` arm (`internal/tui/model.go:1661-1670`) replays the staged directory through `mintSession` from the bootstrap's terminal event; `activeProjectNoticeBand` (`internal/tui/notice_band.go:35-37`, `:205-209`) displaces the pick-a-project banner with the wait message; and `BootstrapFatalMsg` (`internal/tui/model.go:1685`) quits a command-pending model outright. No corrigendum records the resolution, so the body text is now the opposite of the behaviour, and the staged-mint gate, its user-visible band copy and its first-stage-wins rule have no owning requirement anywhere in the specification.
- **Read**: Spec stale. The code is what the phase-7 task settled and approved, and it is the behaviour the classification was written to produce; the specification simply never recorded that its own tracked defect closed. The cost of leaving it is that the next person to touch the path reads the specification, finds the gate unaccounted for, and either deletes it as unspecified — reinstating the mint against a mid-restore server — or adds a second gate beside it; either way the failure shows up as a session minted while restore is still running. The remedy is a dated corrigendum recording that the gap named in §7.6 is closed and what closed it (stage rather than mint while in flight, band announces the wait in place of the banner, first stage wins, a fatal ends the run). No code changes.

## Discarded Findings

None. Every finding cleared the floor: each names a failure, the party it costs and how it would be noticed. The one finding not staged as a proposal (the specification divergence above) is routed as a spec defect rather than dropped, on its own author's reading that the code is right and the document is stale.
