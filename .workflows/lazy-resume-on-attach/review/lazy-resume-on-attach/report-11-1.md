TASK: A Pane Without an Id Never Takes Another Live Pane's Record (lazy-resume-on-attach-11-1, tick-d8cf5d)

ACCEPTANCE CRITERIA:
- Saved windows 1, 3 and 4 of `work` restore as 1, 2 and 3: Y, carrying token `T` and saved at window 3, sits skeleton-marked at window 2; X, carrying no token and saved at window 4, sits skeleton-marked at window 3. A capture commits exactly one record carrying `T` — Y's, with Y's saved working directory and command — and X's record carries no token and neither Y's working directory nor Y's command
- The same layout with X waiting (pending, no token) and Y pending by token commits exactly one record carrying `T`, on Y
- A skeleton-marked pane carrying no token whose address's saved record carries a token no live pane carries (restore's re-stamp failed) still takes that record whole, token included, as today
- A skeleton-marked pane carrying no token whose address's saved record carries no token still takes that record whole, as today
- Every existing capture, merge, re-file and link test stays green unchanged, among them `TestCaptureStructureMergeSkippedPanesByToken`, `TestCaptureStructureFrozenMerge` and `TestCaptureAndRefileLinksAMovedSkeletonPaneOntoItsToken`

STATUS: complete

SPEC CONTEXT: §7.2 (specification.md:304-306, corrigendum at :474). The token match covers the whole hand-over from restore to wait. A mid-restore pane carrying a token takes its previous record by that token. One carrying none takes it by address, unless a live pane carries that record's token, because the record belongs to the pane answering to it. The tokenless pane then gets a fresh record. A record whose token no live pane carries is still taken by address. That is the case where restore's re-stamp failed and the pane at the address is the record's own. The spec rests this on its statement that one live pane answers to a token. At :310 the spec leaves the positional scrollback exposure of a tokenless pane in place ("keeps today's exposure, which is the pre-feature behaviour"). Moving off positional names is deferred to the durable-pane-identity work.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/capture.go:106 (`liveTokenSet(idx)` is taken from the fresh index before either merge runs), internal/state/capture.go:109 and :113 (the set is handed to both merges), internal/state/capture.go:216-228 (`liveTokenSet`), internal/state/capture.go:235-250 (`indexPrevPanes` leaves a record out of `byAddress` when a live pane carries its token, at :243-245, while still indexing it in `byToken`), internal/state/capture.go:288-301 (`takePrevRecord` is unchanged), doc comments at internal/state/capture.go:37-40 and :230-234
- Notes:
  - The rule lives only in the shared lookup. `mergeSkippedPanes` (:151) and `mergeFrozenPanes` (:183) pass `liveTokens` through and add no rule of their own, as the Do section asks.
  - Criterion 1 traced through the code. Y is at work:2.0 and carries `T`, so it takes `byToken[T]` through `carryPrevContent`. X is at work:3.0 with no token; `byAddress[work__3.0]` no longer holds Y's record, so X keeps its fresh record. The committed index holds one record carrying `T`.
  - Criterion 3 traced: a record whose token is absent from `liveTokens` stays in `byAddress`, so `*p = record` (:168) still copies it whole.
  - Criterion 4 traced: a record with no token is always indexed by address.
  - Residual exposure (not reported as a finding): in the renumbered layout, X's fresh record names its live-key positional path, `scrollback/work__3.0.bin` (buildPanes, capture.go:434). Until X is first dumped, that file holds Y's saved bytes. So a reboot inside the mid-restore window would still replay Y's history into X. This is the positional exposure for tokenless panes that the spec deliberately keeps (specification.md:310). The phase-11 comment correction (c86fcc453) removed the "hand one pane another's history" claim from the `indexPrevPanes` comment for this reason. The code matches the spec. It does not match the "or history" wording in the task's Outcome, but the spec governs here.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: internal/state/capture_test.go:2062, the subtest "it leaves a tokenless pane off the record of the live pane carrying that record's token". It asserts exactly one record carries `T`, that Y has `T` + /y + vim, and that X has no token and neither /y nor vim, with X's fresh /live-x + zsh. Before the fix, X copied Y's record whole, giving two records carrying `T`, so this test would fail on the old code.
  - Criterion 2: internal/state/capture_frozen_merge_test.go:316 `TestCaptureStructureFrozenMergeAddressMatchNeverTakesALiveTokensRecord`. Before the fix, the frozen merge reset X's token to "", so the count assertion alone would have passed on the old code. The test also asserts X does not carry /y or vim, and that assertion does fail on the old code.
  - Criterion 3: internal/state/capture_test.go:2090, the unstamped `gone12` token is taken whole by address.
  - Criterion 4: internal/state/capture_test.go:2108, a tokenless record is taken whole.
  - Criterion 5: commit 8e9ea1519 only appends to the two test files, and no later commit touches capture_test.go, capture_frozen_merge_test.go or capture_refile_test.go. I read the subtests of `TestCaptureStructureMergeSkippedPanesByToken`, `TestCaptureStructureFrozenMerge` (including the duplicated-token subtest at capture_frozen_merge_test.go:250) and the layout of the link test. None of them puts a tokenless pane at an address whose record carries a live pane's token, so none of their expectations moves.
- Notes: The tests are focused, with no redundant assertions. The criterion-3 and criterion-4 subtests guard the boundary: they would catch an implementation that dropped every tokened record from `byAddress`.

CODE QUALITY:
- Project conventions: Followed. Sets use the `map[string]struct{}` idiom used elsewhere in the package, subtest names match the file's "it …" style, and comments carry no references to process artifacts.
- SOLID principles: Good. There is one lookup rule in one place, and both merges inherit it.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments at capture.go:37-40, :214-215 and :230-234 are accurate against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Every existing capture, merge, re-file and link test stays green unchanged, among them `TestCaptureStructureMergeSkippedPanesByToken`, `TestCaptureStructureFrozenMerge` and `TestCaptureAndRefileLinksAMovedSkeletonPaneOntoItsToken`" — the "unchanged" half is settled from the commit history. The "green" half needs a run of `go test ./internal/state` (the unit lane) to observe the named tests and the rest of the package's capture, merge, re-file and link tests passing.
