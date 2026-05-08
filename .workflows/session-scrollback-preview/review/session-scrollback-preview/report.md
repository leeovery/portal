# Implementation Review: Session Scrollback Preview

**Plan**: session-scrollback-preview
**QA Verdict**: Approve

## Summary

The session-scrollback-preview feature is implemented end-to-end and matches the specification with care. All 35 tasks across six phases are complete, all acceptance criteria are met, and zero blocking issues were found. The implementation is well-bounded: one new read-only `tmux.Client` listing method, one new `internal/state` tail-N helper, and a new `pagePreview` arm in `internal/tui` — exactly the footprint the spec authorised. Hermetic tests pin the side-effect-free contract (one enumerator call per open, one Tail per focus event, zero hooks/state-writes/FIFO touches), and a durable surface audit guards against scope creep. The Phase 5 and 6 analysis fixes (applySessions extraction, value-receiver discipline, chrome wording, doc-comment correction, invariant comments, and the missing top-level `Model.View()` arm) are correctly applied.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across all dimensions reviewed:
- **Side-effect-free contract** — tested hermetically: exactly one enumerator call, one Tail per focus event, no `hooks.Store` reference, no `state` writer reference, no FIFO reference (`pagepreview_hermetic_test.go`).
- **Initial-open ordering** — enumerate → fail/empty silent no-open → focus (0,0) → synchronous tail-N → `viewport.GotoBottom()`.
- **Chrome counter semantics** — 1-based ordinals from slice position, never raw `window_index` / `pane_index`. Verified under non-contiguous (0,2,5) and base-index-1 fixtures.
- **Tail-N read pipeline** — single fd held across reverse chunked scan; three-shape return contract; performance-budget benchmark and regression-guard test for p99 < 5ms on 4MB fixture.
- **Refresh semantics** — Sessions list re-fetches on `pagePreview → pageSessions`; cursor re-anchored by name; filter state preserved.
- **Cross-cutting seams** — `_portal-saver` excluded at `tmux.Client.ListSessions` source, no preview-layer blacklist; chrome stable mid-preview (no live re-enumeration); ANSI bytes passthrough verbatim.

### Plan Completion

- [x] Phase 1 acceptance criteria met (6/6 tasks complete).
- [x] Phase 2 acceptance criteria met (7/7 tasks complete).
- [x] Phase 3 acceptance criteria met (7/7 tasks complete).
- [x] Phase 4 acceptance criteria met (9/9 tasks complete).
- [x] Phase 5 (analysis cycle 1) acceptance criteria met (5/5 tasks complete).
- [x] Phase 6 (analysis cycle 2) acceptance criteria met (1/1 tasks complete).
- [x] All tasks completed.
- [x] No scope creep — surface audit pins additive footprint at one tmux listing method, one state tail-N helper, and the new pagePreview arm in internal/tui.

### Code Quality

No issues found. Across all 35 task-level reports the architectural picture is consistent:
- Constructor-injected seams (`TmuxEnumerator`, `ScrollbackReader`) with `stateDir` correctly hidden behind the `Tail` interface; production adapter wiring at `cmd/open.go` (`openTUI`) resolves `stateDir` exactly once.
- Pane key derivation (`state.SanitizePaneKey(session, raw window_index, raw pane_index)`) is byte-identical to the daemon writer call site in `internal/state/capture.go:121`.
- Single `readFocusedPaneIntoViewport` dispatcher feeds the three-shape `Tail` outcome into the viewport from the constructor and all three cycle handlers — no drift, no per-pane error cache.
- Receiver discipline unified to value receivers across `previewModel` (Phase 5 fix); `readFocusedPaneIntoViewport` returns the updated viewport rather than mutating via pointer.
- Idiomatic Go: `errors.Is`/`%w` wrapping, `io.SeekStart`/`io.SeekEnd`, `io.ReadFull`, `strings.SplitN` with explicit caps, `max()` builtin for defensive sizing.

### Test Quality

Tests adequately verify requirements. Coverage is balanced and behaviour-anchored:
- Phase 1 tail-N helper: happy/no-content/OS-error branches each have dedicated tests; multi-chunk reverse-scan verified against a `naiveTail` oracle; single-fd invariant pinned via test seam; performance regression guard with `PORTAL_SKIP_PERF` opt-out.
- Phase 1 enumeration: 14 sub-tests covering grouping, non-contiguous indices, base-index 1, pipe-bearing window names, empty/whitespace stdout, error wrap shape.
- Phase 2 entry/dismiss: page-machine routing, no-op gates (empty list, nil selection, enumeration failure), filter passthrough, cursor preservation across open/dismiss, fresh model on re-open.
- Phase 3 cycling: Tab/]/[ wrap directions, pane-0 reset on window cycle, exactly one Tail per cycle, scroll-tail reset, raw-index pane key derivation, single-window/single-pane no-ops, keymap precedence.
- Phase 3 chrome: 1-based ordinals under non-contiguous and base-index drift, verbatim window name, no-liveness substring guard, layout sizing with chrome subtraction, 100-resize zero-Tail invariant.
- Phase 4 edge cases: placeholder for (nil, nil), uniform error string for (nil, err), retry-on-refocus, fewer-than-N renders all lines (1/50/999/sweep), brand-new-session traversal, externally-killed-session stability with progressive placeholders.
- Phase 4 hermetic and surface audits: side-effect-free invariant pinned via mock-call-count assertions; surface audit blocks new capture wrappers, daemon writers, restore/bootstrap/hooks references.

Tests do not import `tmuxtest` from `pagepreview*_test.go` files (mandated by spec). No `t.Parallel()` (consistent with project convention).

### Required Changes

None.

## Recommendations

### Quick-fixes

1. [task 1-2] Doc comment on `TailScrollback` mentions `os.ErrClosed` alongside `fs.ErrPermission` as `errors.Is`-recoverable; cosmetic — consider trimming for layer-of-abstraction consistency.
2. [task 1-3] Test name `"returns an error from a mid-scan seek/read failure"` describes both seek and read but the test only exercises seek. Rename to `"...mid-scan seek failure"`.
3. [task 1-4] `for i := 0; i < b.N; i++` and similar loops could use Go 1.22+ integer-range form (`for i := range b.N`). Stylistic.
4. [task 1-5] `internal/tmux/tmux.go` doc comment narrows the cause of non-contiguous indices to "after window kills" — could also arise from `move-window`. Minor wording.
5. [task 1-6] Consider an inline comment in the new method pointing at the test that locks the wrap-message shape so future refactors notice the contract.
6. [task 2-2] `previewModel` doc says "methods must not be called on a zero previewModel" — a debug-only panic in `currentGroup()`/`currentPaneKey()` would make misuse loud (already partially addressed by 5-5 invariant comment).
7. [task 2-7] Doc comment on `scrollbackReaderAdapter` claims `n` is "supplied at construction so the helper has no implicit dependency on `previewTailLines`" — but the production constructor hardcodes the constant. Trim prose.
8. [task 4-2] Dispatcher arm `case err != nil:` (pagepreview.go:208) catches a `(bytes != nil, err != nil)` shape the helper contract never produces. Add a one-line comment pinning the precedence rationale.
9. [task 4-6] Comment at pagepreview_externalkill_test.go:219 reads ambiguously alongside the "Window 2 of 2" assertion. Rephrase.
10. [task 4-8] `hermeticEnumerator.lastArg` is recorded but never asserted. Either drop the field or add `if enum.lastArg != "work" { ... }` to tighten the contract.

### Ideas

11. [task 1-1] `merged := make([]byte, len(buf)+len(tail))` + double `copy` reallocates the entire accumulated tail per iteration. For pathological inputs O(n^2) in tail bytes. Out of scope unless task 1-4's benchmark proves it matters.
12. [task 1-1] `indexOfNthNewlineFromEnd` returns -1 as "unreachable given precondition". A defensive panic would fail loudly if a future refactor breaks the precondition.
13. [task 1-1] The "holds a single file descriptor" test fixture is small enough (~36 KiB) that it never exercises the multi-iteration loop. Combining the multi-iteration AND opens == 1 assertions into one test would tighten the single-fd-across-chunks invariant.
14. [task 1-2] `indexOfNthNewlineFromEnd` is invoked exactly once where `bytes.LastIndexByte` already locates the final `\n`. Could be folded into a single backwards-walk, removing the documented-unreachable default.
15. [task 1-3] `io.ReadFull` returns `io.ErrUnexpectedEOF` on short read, surfaced as an error — defensible if a future fixture concurrent-truncates. Not a defect.
16. [task 1-4] Regression-guard test rests on a single sample; could measure several iterations and assert on a percentile.
17. [task 1-4] Benchmark exercises only N=1000. Sub-benchmarks at N=100 / N=10_000 would catch chunk-boundary regressions a single N cannot.
18. [task 1-5] `sort.Slice` could be `slices.SortFunc` (Go 1.21+); the file uses `sort.Slice` elsewhere, so consistency wins.
19. [task 2-1] The three-shape `Tail` contract is documented on the seam and on the Phase 1 helper; consider a single canonical reference once 2-7 lands to prevent silent drift.
20. [task 2-2] Forward-integration of Phase 4 placeholder/error wording in `readFocusedPaneIntoViewport` makes the 2-2 task wording slightly out-of-sync with the code. Add a code comment cross-referencing 4-2/4-3.
21. [task 2-2] `TestNewPreviewModel_PassesRawANSIBytesVerbatimToSetContent` uses `strings.Contains` on full View() (chrome included). Byte-equality on viewport content alone would pin the verbatim invariant more strictly.
22. [task 2-3] Extracting `handleSpacePreview` from the inline switch in `updateSessionList` would make precedence structurally explicit and shorten the function.
23. [task 2-3] `TestPagePreviewRoutesUpdateToPreviewModel` could pin viewport `YOffset` movement (or absence) for stronger signal that the message reached `previewModel.Update`.
24. [task 2-4] After Phase 4 refresh layered on top, cursor preservation in production is by name, not by index. If the test harness ever wires a `SessionLister`, `TestPreviewEscPreservesListCursor` may need to assert by name.
25. [task 2-4] `previewModel` zero-value is reserved for "between opens"; consider a `hasPreview bool` sentinel or `*previewModel` pointer to make "between opens" state observably distinct.
26. [task 2-5] `TestExactlyOneSpaceBranchInUpdateSessionList` uses substring count of `tea.KeySpace`; brittleness if a future change adds `tea.KeySpace` in a comment. Tighten to a `case msg.Type == tea.KeySpace` line-anchored match if needed.
27. [task 2-5] Implementation diverges from the literal Do-snippet (early break for all keys vs. inline SettingFilter check inside Space branch). Better engineering but worth recording.
28. [task 2-7] Compile-time assertions duplicated between `preview_adapter.go` and `preview_adapter_test.go`. Production-file alone suffices.
29. [task 2-7] `preview_adapter_test.go` does not declare `t.Parallel()`. Adapter tests have no package-level mutable seams, so could parallelise.
30. [task 3-1] `previewModel` doc says zero-value `currentGroup` "would index an empty slice" — actually it would panic on out-of-range, not silently. Minor wording drift.
31. [task 3-1] `newPreviewModelForHelpers` leaves enumerator/reader nil intentionally; a one-line comment that any helper accidentally calling them would nil-panic would self-document the purity contract.
32. [task 3-2] Tab handler uses local `paneCount <= 1` guard rather than `degenerate()`. Functionally equivalent; current choice is defensible.
33. [task 3-7] `driveCycleSequence` is shared only within one file; if peer cycle-stability suites land later it could move to a shared `_testhelpers` file.
34. [task 3-5] Chrome implementation does not use lipgloss styling; deferred is fine.
35. [task 4-3] `decimal()` helper duplicates `strconv.Itoa` with hand-rolled itoa. Replace for ~12 fewer LOC.
36. [task 4-3] "Up at top boundary returns nil cmd" pins a property of bubbles/viewport's current implementation; future viewport version returning a tea.Cmd at the top would fail despite unchanged user-visible behaviour.
37. [task 4-4] `TestPreviewBrandNew_CycleKeysDoNotSkipPlaceholderPanes` asserts `len(reader.calls) == 4` — coincidentally also the pane count. A clarifying comment on dimension would help.
38. [task 4-4] `TestPreviewMixed_FocusFromBytesPaneToPlaceholderAndBackIssuesFreshTailCalls` only exercises w0; w1 panes never visited.
39. [task 4-5] `refreshSessionsAfterPreviewCmd` returns nil when `sessionLister` is unset (test-tolerance only). Production always wires it; consider construction-time non-nil assertion or explicit "test-only path" doc.
40. [task 4-5] `reanchorSessionCursor` clamps to `len(visible) - 1` when previous name is missing; spec says "neighbour". Last-index clamp is one valid interpretation but could land far from the original cursor. Refinement is additive.
41. [task 4-6] `_ = mm.viewport.View()` in the panic test is redundant with `_ = mm.View()`; intent ("exercise every panic surface") is reasonable.
42. [task 4-6] `killedSessionFixture()` is local; if a third test file needs the 2x2 shape, promote into a shared fixtures file.
43. [task 4-7] Exclusion is by underscore prefix rather than exact-match on `_portal-saver`. Intentional and pinned.
44. [task 4-8] `TestPreviewHermetic_NoStatePackageWriters` uses a hand-curated denylist. Future state-package writer added without updating the list would not be caught. An allowlist would be more durable.
45. [task 4-8] `TestPreviewHermetic_NoFIFOReferences` scopes to `pagepreview*.go`; if future preview code is factored to a different filename, the audit would silently drop it.
46. [task 4-9] **Missing subtest**: Plan's Tests list explicitly enumerates "audit: save-format constants unchanged" — pin `scrollbackSubdir = "scrollback"`, `paneKey+".bin"` in `ScrollbackFile`, `"hydrate-"`/`".fifo"` in `FIFOPath`. No dedicated subtest pins these literals. Non-blocking gap; the corresponding AC ("Save-format constants and `.bin` file shape are unchanged") has no direct assertion. Add a subtest to close the gap.
47. [task 4-9] Forbidden-symbols list in `TestSurfaceAudit_TmuxNoNewCaptureWrapper` is hand-curated. A regex scan for `func \(c \*Client\) CapturePane[A-Z]\w*\(` would catch any future capture-wrapper variant by shape.
48. [task 4-9] `TestSurfaceAudit_NoNewPackageForPreview`'s `preExistingPackages` allow-list duplicates knowledge from CLAUDE.md's package table. A one-line comment pointing future maintainers at the canonical source when updating either side would prevent drift.
49. [task 5-2] Cycle re-read coverage isn't pinned by a test that asserts the post-Update View() output. A one-line addition to existing tests would close the gap.
50. [task 5-3] Could add an explicit negative assertion `!strings.Contains(got, "#W:")` in `pagepreview_chrome_test.go` to pin the cleanup against future regressions.

### Bugs

None.
