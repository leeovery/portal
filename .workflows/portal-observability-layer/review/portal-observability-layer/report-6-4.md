TASK: Emit the success exit-path INFO hydrate: scrollback replayed bytes=N took=T threading copied-byte count and replay duration (portal-observability-layer-6-4)

ACCEPTANCE CRITERIA:
- Successful rehydration → one INFO hydrate: scrollback replayed bytes=<n> took=<T>, n = io.Copy return.
- bytes = exact io.Copy byte count (0 for empty, file size for populated).
- took = measured replay (copy) duration, time.Duration (NOT settle-sleep, NOT quoted string).
- Zero-byte scrollback still emits bytes=0.
- scrollback replayed INFO emitted after postamble + 100ms settle sleep + marker-unset, before hydrate: exec INFO.
- Success path reaches execShellOrHookAndExit exactly once; INFO does not fire on timeout/file-missing paths.
- Reset preamble/postamble, verbatim streaming, settle sleep, marker-unset unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § Hook-firing observability limit Rule 3 success row: Info("scrollback replayed","bytes",n,"took",took) then exec. bytes is a Hydrate-group attr; time.Duration renders via String() (took=1.2s) not quoted. Success case = three lines (lookup DEBUG below INFO + this INFO + exec INFO).

IMPLEMENTATION:
- Status: Implemented (matches spec exactly)
- Location: cmd/state_hydrate.go:198-200 (start/n/took bracket io.Copy), :237 Info("scrollback replayed","bytes",n,"took",took), :243 single success-path execShellOrHookAndExit.
- Notes: n, err := io.Copy(...) (:199) captures previously-discarded int64; start (:198) + took := time.Since(start) (:200) bracket only the copy, excluding the 100ms settle sleep (documented :193-197). INFO at :237 on err==nil branch, after postamble write (:215), settle sleep (:220), marker-unset (:225), immediately before exec (:243). The three non-success execShellOrHookAndExit sites (timeout :131, file-missing :181, mid-stream :208) return nil before :237 → INFO fires once, only on success. bytes int64 (integer render); took Duration (String() render). Preamble/postamble/32K verbatim streaming/settle-sleep/marker-unset preserved. *slog.Logger via hydrateLogger/log.For.

TESTS:
- Status: Adequate
- Location: cmd/state_hydrate_replayed_log_test.go
- Coverage: EmitsScrollbackReplayedBytesTookOnSuccessPath; BytesEqualsCopyCountForPopulatedFile (NUL/non-UTF8/escape bytes → byte not rune count); ZeroByteScrollbackEmitsBytesZero; FiveMegabyteFileReportsExactByteCount; TookIsDurationAcrossReplayNotSettleSleep (took.Kind()==KindDuration via Record AND took.Duration() < hydrateSettleSleep); PrecedesExecINFOAndFiresOnce (countLogLines==1 + ordering); NotEmittedOnTimeoutPath + NotEmittedOnFileMissingPath.
- Notes: Drives real runHydrate (blocking FIFO open + signalFIFOAsync + real io.Copy). Duration test asserts KindDuration against Record (substring can't distinguish Duration from stringified). execLogLine prefix-anchored, exactly-one. Would fail if n reverted to discarded or INFO removed/misordered. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; *slog.Logger via log.For; terse message data-in-attrs; logtest.Sink).
- SOLID: Good — minimal additive change; seam DI untouched.
- Complexity: Low (two locals + one log call).
- Modern idioms: Yes (time.Now()/time.Since window; typed int64/Duration attrs).
- Readability: Good — step-5/step-9 comments document measurement window + single-emission/ordering contract.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Of four byte-count tests only TookIsDuration... asserts attr Kind via Record; bytes=N assertions are substring-only (int64 and stringified count render identically). A future hardening could assert rec.IntAttr(t,"bytes") (helper exists) in one byte-count test to pin bytes as KindInt64. Current coverage adequate — defence-in-depth, not a gap.
