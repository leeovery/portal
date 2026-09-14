# Analysis Report: Open With Forced Filter (Cycle 1)

## Stats

- Total findings: 6
- Deduplicated findings: 4
- Proposed tasks: 4

## Summary

Three agents ran; all six findings cleared the floor and two pairs deduplicated to one finding each — the session set the sigil counts K over versus the set the picker lists, and the field list `/term` matches against versus the one `-f` and the picker's own filter match against. Both are rules the specification requires to be identical across entry points and the code states twice by hand, with no compiler or test pinning the copies together. The remaining two are singletons: the containment filter's item source is an unsynchronised cell written on Bubble Tea's Update goroutine and read from a command goroutine (verified against `bubbles/v2@v2.1.0` `list.SetItems` → `filterItems`, whose closure `bubbletea/v2@v2.0.7` runs in its own goroutine at `tea.go:721`), and the in-TUI warning route the search form's new picker classification hands it to never fires on the warm latch-satisfied path, so a down state daemon goes unreported on an invocation that previously reported it.

No proposal reverses a settled direction: the p4 picker classification stands untouched (the standards finding names the classification as correct and asks only for a delivery path behind it), and the warning-delivery proposal builds on the `finishTUI` extraction p4 task 3 settled rather than undoing it.

One specification sentence is worth flagging without being a defect: §7.4's rationale says soft warnings "need somewhere safe to land, and the picker path already has one" (specification.md:302). That is false for the warm picker route today — there is no loading page there and so no surfacing call — but the requirement it supports (specification.md:288) is the one worth keeping, and Task 4 makes the sentence true rather than the sentence needing an edit. Recorded here rather than as a spec defect because the code, not the claim, is what is wrong.

## Comment Corrections

- cmd/open_search.go:183 — claims an opened picker surfaced the warnings "in its notice band", which holds only on the concurrent route; on the warm route the surfacing helper writes to stderr, and per Task 4 does not run at all.
  OLD: // failed. A picker that opened has already surfaced them in its notice band,
  // and a cancelled loading page is owed no report.
  NEW: // failed. The picker owns its own warning surfacing, and a cancelled loading
  // page is owed no report.

- cmd/open_search.go:165 — asserts a causal link that does not exist: no soft bootstrap warning can cause `list-sessions` to fail (a failed `EnsureServer` is a bootstrap fatal, and a down saver does not stop the server answering), and the true reason is already stated five lines below for the sibling branch.
  OLD: // The warning explains the error that follows it — a saver that is down is
  // why the list could not be read — so it is surrendered ahead of it.
  NEW: // This route paints no picker either, so the buffered warnings go to the
  // terminal.

## Discarded Findings

None. Every finding named a failure it prevents, and none reverses a settled direction.
