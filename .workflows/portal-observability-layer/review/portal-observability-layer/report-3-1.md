TASK: Expose AtomicWrite write-phase sentinels for error_class mapping (no logging in fileutil) (portal-observability-layer-3-1)

ACCEPTANCE CRITERIA:
- fileutil exposes exported sentinels per write phase; AtomicWrite wraps each failure via %w preserving *os.PathError (errors.Is + errors.As succeed).
- ClassifyWriteError returns correct error_class per sentinel + documented safe default.
- AtomicWrite/AtomicWrite0600 contain no internal/log import and no logger.* call.
- sessions.json write path behaviourally unchanged.
- write-failed-fsync [needs-info] resolved explicitly, covered by test or comment.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727). Store methods emit WARN with error_class from closed per-phase space (write-failed-temp-create/write/fsync/rename). AtomicWrite stays audit-unaware (shared with out-of-scope sessions.json). This task supplies the discriminable error shape.

IMPLEMENTATION:
- Status: Implemented
- Location: fileutil/atomic.go:17-34 (four exported sentinels); :44-57 (ClassifyWriteError pure switch, default→write-failed-write documented floor); :82-116 (AtomicWrite: MkdirAll/CreateTemp→ErrWriteTempCreate, tmp.Write→ErrWriteWrite, tmp.Close→ErrWriteFsync, os.Rename→ErrWriteRename, doubled %w preserving OS error); :71-77 (AtomicWrite0600 forwards verbatim).
- Notes: [needs-info] resolved option (a): tmp.Close→ErrWriteFsync documented (no explicit Sync). MkdirAll→ErrWriteTempCreate documented. No internal/log import (AST test). sessions.json caller (commit.go:48-50) wraps with %w, never errors.Is against sentinels — transparent.

TESTS:
- Status: Adequate
- Location: fileutil/atomic_classify_test.go
- Coverage: forced real temp-create failure → errors.Is + classify; *os.PathError via errors.As; forced rename failure → errors.Is + *os.LinkError (correctly identifies LinkError not PathError) + classify; table ClassifyWriteError all four + unrecognised→write-failed-write default; AtomicWrite0600 preserves sentinel; AST test fails on any internal/log import.
- Notes: tmp.Write/tmp.Close sentinels exercised synthetically (real failure hard to force portably); shared doubled-%w pattern transitively validated by live temp-create/rename tests. Behaviour-focused.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; documented sentinels; multi-%w).
- SOLID: Good — fileutil pure primitive; classification separate pure fn; logging pushed to store seam.
- Complexity: Low.
- Modern idioms: Yes (doubled %w, errors.Is/As, errors.New sentinels).
- Readability: Good — rationale comments (MkdirAll mapping, Close→fsync, default floor, LinkError vs PathError).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] tmp.Write/tmp.Close sentinels only validated via synthetic wrapped errors in the classify table, not a forced real failure inside AtomicWrite (unlike temp-create/rename which use live OS failures); shared %w pattern makes this low-risk. A closed-fd seam test would close the last gap. Optional.
