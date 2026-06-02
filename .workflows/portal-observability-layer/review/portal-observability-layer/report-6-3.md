TASK: Emit the file-missing exit-path INFO hydrate: scrollback missing and resolve the fifo missing row (portal-observability-layer-6-3)

ACCEPTANCE CRITERIA:
- Scrollback os.Open ENOENT failure → one INFO hydrate: scrollback missing path=<cfg.File>.
- Permission-denied → same single INFO (one INFO regardless of cause).
- Generic-I/O os.Open failure → same single INFO.
- Mid-stream io.Copy failure → same single INFO (shared recovery).
- INFO uses reserved path attr = cfg.File (NOT target), precedes hydrate: exec INFO.
- Three existing per-cause WARNs, no-settle-sleep posture, marker-unset unchanged.
- fifo missing row resolved against live handler: wired at confirmed distinct FIFO-absence site OR documented as collapsed-under-timeout — NOT a synthetic branch.

STATUS: Complete

SPEC CONTEXT:
Spec § Hook-firing observability limit (924-972) Rule 3 catalogs four exit-path INFOs; this task owns scrollback missing + fifo missing rows. Rule 2 reserves path for genuine filesystem-path lines, distinct from target. fifo missing's "Silent ENOENT … then exec" framing assumes a fall-through-to-exec shape the live handler doesn't have.

IMPLEMENTATION:
- Status: Implemented (fifo missing spec/code divergence flagged per task instruction)
- Location: cmd/state_hydrate.go:403 Info("scrollback missing","path",cfg.File) inside handleHydrateFileMissing; :149 Info("fifo missing","path",cfg.FIFO) at non-timeout open-error branch before hard return fmt.Errorf("open fifo %s: %w"). Both file-missing call sites route through the handler: runHydrate:172-185 (os.Open) + :199-212 (mid-stream io.Copy) each invoke HandleFileMissing then execShellOrHookAndExit.
- Notes: Placement in handleHydrateFileMissing (single funnel for all four shapes ENOENT/permission/generic-I/O/mid-stream) guarantees one INFO per recovery regardless of cause/site, before exec INFO. path = cfg.File (distinct from target). [needs-info] resolved option (1) genuine: missing FIFO → os.OpenFile O_RDONLY returns ENOENT immediately in openFIFOWithTimeout goroutine → select surfaces verbatim as non-ErrHydrateTimeout → hard-returns (does NOT exec); divergence from spec's "then exec" documented in-code (:136-148) + test header. Phase-1 slog binding intact (hydrateLogger = log.For("hydrate")). Three per-cause WARNs (:385/387/389), no settle sleep, inline marker-unset (:411) preserved.

TESTS:
- Status: Adequate
- Location: cmd/state_hydrate_file_missing_log_test.go
- Coverage: ENOENT_EmitsScrollbackMissingPath; Permission_EmitsOneScrollbackMissingINFO (chmod 000, exactly ONE); GenericIO_EmitsOneScrollbackMissingINFO; MidStreamCopy_SharesScrollbackMissingINFO (partial bytes on stdout); PathAttrIsFileAndPrecedesExecINFO (path=cfg.File, no target=, ordering); PreservesPerCauseWARNsAndNoSettleSleep (WARN once, elapsed << 100ms, marker-unset call); FifoMissingLog_EmitsFifoMissingPathOnNonTimeoutOpenError (no FIFO created, real openFIFOWithTimeout, error wraps fs.ErrNotExist, fifo missing INFO, exec NOT called, no signal timeout — proves distinct from timeout).
- Notes: countLogLines/execLogLine prefix-anchored, exactly-one. Would fail if broken (remove INFO / swap target / drop WARN / collapse FIFO into timeout). Generic-I/O + mid-stream invoke handler directly (failing reader awkward to stage); routing verified structurally.

CODE QUALITY:
- Project conventions: Followed (slog log.For("hydrate"); no t.Parallel; path reserved-attr discipline).
- SOLID: Good — single-funnel placement; seam-based DI unchanged.
- Complexity: Low (two single-line Info calls).
- Modern idioms: Yes (slog attrs, errors.Is cause discrimination).
- Readability: Exemplary — divergence flags + placement rationale documented in-code + test header.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] fifo missing INFO (:149) is a genuine spec/code divergence (non-exec, hard-return) the spec Rule-3 table still frames as "then exec"; executor correctly chose option (1) and flagged it — task satisfied, but the spec table (965) remains stale. A one-line spec erratum noting the fifo missing row hard-returns would match shipped behaviour for future readers.
- [idea] Generic-I/O + mid-stream tests invoke handleHydrateFileMissing directly (only ENOENT/permission go through runHydrate end-to-end); pragmatic, but a single end-to-end mid-stream test would close the gap between "handler emits correctly" and "io.Copy call site reaches handler".
