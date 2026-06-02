TASK: Best-effort write path with stderr fallback and unbuffered-writer guarantee (portal-observability-layer-2-7)

ACCEPTANCE CRITERIA:
- Open failure (unwritable state dir) does not propagate to logger.Info caller; record attempted on stderr, process continues.
- Write failure mid-record drops the record, Handle returns nil; next record (after reopen) writes normally.
- Disk-full/EACCES open or write never returns error to caller, never panics.
- Writer unbuffered: after logger.Info(x) returns, line readable from file with no flush/Sync.
- Failed symlink swing leaves writes flowing to the open fd.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism → Resolved operational edges (best-effort: single stderr fallback then drop; chmod/unlink WARN-and-skip; failed swing leaves prior symlink) + § Defensive invariants → Flush (no bufio; marker in kernel before Info returns; unbuffered locked constraint).

IMPLEMENTATION:
- Status: Implemented
- Location: handler.go:131-183 (Handle always returns nil; level-drop returns nil; delegates to bestEffortWrite); :185-196 (bestEffortWrite: one io.WriteString, on error single Fprint to stderrFallback, error discarded, never returns/panics); :14-20 (stderrFallback package var = os.Stderr, swappable seam); sink.go:105-129 (Write surfaces open/write errors; unbuffered s.file.Write with "Do NOT introduce a bufio.Writer here" comment); sink.go reopen/rotateSameDay swing failure swallowed (_=); rotate.go/retention.go chmod/unlink WARN-and-continue.
- Notes: Open-failure and write-failure unified into one error path (sink.Write returns open error via ensureCurrent → bestEffortWrite routes to stderr). Unbuffered structurally enforced (no bufio in package; only the prohibition comment). Init-time open-failure also surfaces advisorily (openLogWriter returns os.Stderr + error) — complementary belt-and-braces.

TESTS:
- Status: Adequate
- Location: internal/log/write_path_test.go
- Coverage: AC1 (errWriter no-propagate + stderr fallback); AC2 (drop + nil + next record writes); AC3 (table ENOSPC/EACCES/closed-file, no panic/error); AC1 end-to-end against real sink with 0500 dir forcing EACCES; AC4 unbuffered (read without Sync after Info); AC5 symlinkFunc seam forcing swing failure, record still lands in open fd.
- Notes: Behaviour (caller-observable error, file/stderr contents, no panic). Genuine-EACCES + seam injection where deterministic forcing is non-portable. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (stderrFallback seam mirrors symlinkFunc/chmodFunc; t.Cleanup; no t.Parallel).
- SOLID: Good — sink surfaces errors, best-effort policy lives in textHandler.bestEffortWrite (documented seam boundary sink.go:36-44).
- Complexity: Low (bestEffortWrite 4 lines).
- Modern idioms: Yes; explicit _,_= discard justified.
- Readability: Good — locked-contract comments accurate.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] AC5 verified at rotatingSink.Write layer not through Handle (correct — swing is sink-internal, Handle can't observe); a one-line test comment tying it to the AC would aid future readers.
- [idea] Open-failure record-loss differs subtly between Init probe path (openLogWriter returns os.Stderr as h.w) and steady-state (bestEffortWrite routes to stderrFallback seam); two distinct stderr vars — harmless, worth a doc sentence.
