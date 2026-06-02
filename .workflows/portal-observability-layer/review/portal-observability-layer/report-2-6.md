TASK: Same-day size-cap overflow rotation to portal.log.<today>.N (portal-observability-layer-2-6)

ACCEPTANCE CRITERIA:
- currentSize + len(record) >= cap → opens portal.log.<today>.1 (first overflow) and writes there.
- Max-N discovery: .1 and .3 present (gap) → next opens .4 (max+1), not .2.
- No existing .N → first overflow opens .1.
- EEXIST on chosen .N retries N+1.
- Previous segment NOT chmod'd (stays 0600) after same-day rotation.
- Steady state (records far below cap) → no overflow.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism step 3 (419-423). After fd current, check current_size + len(serialized) >= cap; roll to portal.log.<today>.N (max+1 or 1), O_CREAT|O_EXCL|O_APPEND|O_WRONLY, retry N+1 on EEXIST, swing symlink via pid-scoped procedure. Prior same-day segment NOT chmod'd (peer may hold O_APPEND fd). Cap resolved once at init.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/sink.go — Write (105-129) → rotateIfOverCap (137-146) after ensureCurrent; rotateSameDay (164-191); claimNextSegment (197-208); nextSegmentN (215-237). daySegmentFile names.go (47-49). swingSymlink symlink.go. Cap wired once at init.go:122-123 → newRotatingSink stores rotateSize.
- Notes: Boundary exact (rotate fires on >= cap). nextSegmentN globs portal.log.<today>.*, parses trailing with Atoi, max+1 (gaps preserved, non-numeric siblings skipped, base not matched). EEXIST retry via errors.Is(os.ErrExist). Prior segment closed in-process, never chmod'd; sealPastDayFiles only seals date != today (documented comment block sink.go:154-163). Cap read from s.rotateSize, never env per Handle. Stat-failure → don't rotate this Write (defensive).

TESTS:
- Status: Adequate
- Location: internal/log/size_cap_test.go (six tests 1:1 with ACs)
- Coverage: first overflow→.1 (+ base untouched); max+1 across gap (.2 not filled); .1 when none; EEXIST retry via openSegmentFunc seam → .3; prior segment stays 0600; 100 small writes → one file, symlink never swung.
- Notes: Behaviour (symlink target, contents, perms, count). openSegmentFunc seam justified (genuine EEXIST race non-deterministic). Not over-tested. Minor gap: prior-fd-close not directly asserted (hard to test portably, not an AC).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; package-var seams + t.Cleanup; no internal/state import).
- SOLID: Good — rotateIfOverCap/rotateSameDay/claimNextSegment/nextSegmentN decomposition.
- Complexity: Low.
- Modern idioms: Yes (errors.Is, CutPrefix, Glob).
- Readability: Good — DELIBERATELY-NOT-chmod'd + disk-fill-valve rationale documented.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] claimNextSegment is an unbounded loop; a pathological adversary winning every O_EXCL race would spin. A sanity ceiling (fall back to stderr) would harden it. Not required by spec.
- [idea] nextSegmentN and pastDayLogDate/sealPastDayFiles independently re-derive the portal.log.<date>[.N] parse with different parsers (Atoi vs isAllDigits+time.Parse); consolidating segment-name parsing into one helper would prevent drift between seal and overflow-discovery paths.
