AGENT: standards
FINDINGS:
- FINDING: commit-now subtest of the failed-listing stand-down guard passes whatever the listing does
  SEVERITY: low
  FAILURE: If the commit path ever reads a failed list-sessions as zero sessions again, the "commit-now" subtest of TestCommittersStandDownOnAFailedSessionListing still passes. It runs with the package-wide poisoned TMUX ("…,0,0"), so ownTmuxServer() returns 0 and confirmOwnServer refuses every cycle with "own server unknown" before any confirmation is sent. A swallowed listing therefore still stands down, writes nothing and returns errCommitNowFailed. Those are the only three things the subtest asserts. A developer relying on this test for the "failed listing stands commit-now down" contract gets a pass that proves nothing. Today the contract is still caught, but only by the "a failed session listing" case of TestCommitNowReportsAStandDownAsABackOff, which sets the own server and checks tmux's stderr.
  FILES: cmd/state_commit_listing_failure_test.go:125-140
  DESCRIPTION: The spec's testing section requires the failed-listing stand-down to hold for the daemon's tick, its shutdown flush and commit-now. The tick and flush subtests go through makeDeps, which sets OwnServer to fakeOwnServerPID, so a swallowed listing would reach a confirmation the fake answers and the commit would write; those two subtests do test the rule. The commit-now subtest never calls withOwnTmuxServer, unlike every other commit-now test added in this change (state_commit_confirmation_test.go, state_commit_backoff_test.go, state_commit_drop_log_test.go). So its stand-down comes from the unknown own server, not from the listing.
  RECOMMENDATION: Call withOwnTmuxServer(t, fakeOwnServerPID) at the top of the commit-now subtest. A swallowed listing then reaches a confirmation the fake answers as the own server and commits, which fails assertUnchanged.
COMMENT_CORRECTIONS:
- cmd/state_hydrate.go:322 — the claim that only a gone pane reads as clear is false for a read already in flight when tmux begins exiting (exit 0, no output), which is the shutdown case the draw loop now runs in
  OLD: // marker reads back clear; a read that fails counts as still pending. A plain
// format read serves: the one pane it misreads as clear is a gone one, which
// wants nothing more from the chain.
  NEW: // marker reads back clear; a read that fails counts as still pending. A plain
// format read serves: a pane it misreads as clear is gone or on a server
// already exiting, and wants nothing more from the chain.
SUMMARY: The production code matches the specification throughout: the probe listing on the commit path only, own-server confirmation before the re-file, link-based re-file, confirmed empty-capture writes, SIGTERM traps in the eager and parked shells, the draw and waiter catching SIGTERM (with the parked shell closing the gap before each catch is installed), and drop and back-off logging on existing attribute keys. The only findings are one test subtest that passes whatever the listing does, and one shell-helper comment whose reasoning skips the shutdown in-flight answer.
