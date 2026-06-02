TASK: Date-aware fd reuse with inode-identity reopen and first-of-day O_CREAT|O_EXCL open (portal-observability-layer-2-2)

ACCEPTANCE CRITERIA:
- First Handle ever creates portal.log.<today> via O_CREAT|O_EXCL and writes.
- Two same-day records reuse the same fd when inode matches symlink target.
- portal.log.<today> unlinked mid-day → next Handle detects inode/ENOENT mismatch, reopens onto live/recreated target, does NOT run day-roll sweeps.
- Date string advances → next Handle opens new day's file + signals day-roll sweeps.
- EEXIST on O_CREAT|O_EXCL handled by O_APPEND|O_WRONLY fallback.
- No data race under concurrent Handle (-race).

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism step 2 a-d + § Defensive invariants Invariant 2. Reuse fd only while date unchanged AND fstat-fd inode == stat-symlink-target inode; date-change runs new-day path + retention; same-day inode/ENOENT mismatch reopens WITHOUT sweeps (2026-05-28 unknown-zeroing defence); O_CREAT|O_EXCL first-of-day with O_APPEND fallback.

IMPLEMENTATION:
- Status: Implemented (composes cleanly with sibling tasks)
- Location: internal/log/names.go (dayFile/daySegmentFile/symlinkPath/sweptSentinelFile, import-cycle guarded); internal/log/sink.go (rotatingSink file/date/dev/ino + mu; ensureCurrent :242; inodeMatchesSymlink :278 fstat fd + stat target compare dev+ino; openDayFile :350 O_CREATE|O_EXCL|O_APPEND|O_WRONLY 0600 + ErrExist fallback + devIno darwin/linux normalise; reopen :304 fires dayRoll only when dateChanged). Write :105 holds mu across ensureCurrent → size-cap → single unbuffered Write.
- Notes: Seam order correct (migration guard → open → swing). Deliberate documented refinement: first-ever write fires dayRoll(true) — safe via single-winner swept.<today> gate + idempotent seal, pinned by tests. Same-day mismatch passes false (no sweep). No drift.

TESTS:
- Status: Adequate
- Location: internal/log/sink_test.go, names_test.go
- Coverage: each AC has a behaviour-level test (fd-identity reuse via s.file.Fd() compare; inode-mismatch reopen w/o sweeps via counter delta; ENOENT recreate; date-change sweep count 1→2 + symlink swing; EEXIST append fallback; -race 8×50). Plus MkdirAll regression + legacy-migration composition.
- Notes: Asserts behaviour (fd identity, inode adoption, sweep-counter deltas, contents). dayRoll override is minimal seam. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (unexported package-var seams + t.Cleanup; no t.Parallel; import-cycle guard; devIno cross-platform).
- SOLID: Good — rotatingSink single responsibility; ensureCurrent/inodeMatchesSymlink/openDayFile/reopen factored; dayRoll callback decouples 2-2 from 2-5/2-8.
- Complexity: Low.
- Modern idioms: Yes (errors.Is os.ErrExist, functional seams).
- Readability: Excellent — comments justify unbuffered, no-chmod-same-day, first-write-sweep, inode-mismatch tied to incident.
- Security/Perf: 0600 files; per-Handle fstat+stat+write acceptable on 1 Hz tick.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] inodeMatchesSymlink collapses transient stat errors into "mismatch → reopen" (safe default, matches spec); a persistent transient error would reopen every Handle. Accepted trade-off; worth a doc note.
- [idea] First-ever-write-fires-dayRoll spans the 2-2/2-8 boundary; correctness depends on 2-8 single-winner gate + 2-5 idempotent seal (verified by TestRotatingSink_SecondFreshSinkSameDayDoesNotResweep). Informational.
