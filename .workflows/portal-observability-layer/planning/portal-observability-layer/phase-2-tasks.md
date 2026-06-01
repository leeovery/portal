---
phase: 2
phase_name: Rotation, retention, and defensive invariants
total: 15
---

## portal-observability-layer-2-1 | approved

### Task 2-1: Parse `PORTAL_LOG_ROTATE_SIZE` and `PORTAL_LOG_RETENTION_DAYS` once at handler init

**Problem**: The date-aware rotating handler needs two tunables resolved once at construction: a size-cap safety valve (`PORTAL_LOG_ROTATE_SIZE`, default 500 MB) and a retention window (`PORTAL_LOG_RETENTION_DAYS`, default 30 days). Both must accept their respective valid forms, fall back to the default on invalid input, and (for retention) report enough to emit the prescribed startup WARN. Without these resolvers the size-cap rotation (Task 2-6) and the retention sweep (Task 2-8) have no thresholds to act on.

**Solution**: Add two pure resolver functions in `internal/log` (e.g. `internal/log/config.go`): `resolveRotateSize(raw string) (bytes int64, source string)` and `resolveRetentionDays(raw string) (days int, source string, normRaw string)`. The size resolver parses a number with an optional case-insensitive `K`/`M`/`G` suffix (bare number = bytes), falling back to `500M` (524288000 bytes) on any parse failure. The retention resolver parses an integer, falling back to 30 on non-integer / negative / `>365`. Both follow the same `(value, source, raw)` shape as the Phase 1 level resolver (Task 1-2) so the WARN/`log-level resolved`-style reporting is uniform. The functions perform resolution only; the retention WARN emission lands in Task 2-8 and the size-cap use lands in Task 2-6.

**Outcome**: Given any env value (or unset), each resolver returns the correct value plus a `source` of `default`/`env`/`fallback`: `PORTAL_LOG_ROTATE_SIZE` unset → `(524288000, "default")`; `"500M"`/`"1G"`/`"1048576"` → parsed bytes with `source="env"`; `"abc"`/`"5X"` → `(524288000, "fallback")`. `PORTAL_LOG_RETENTION_DAYS` unset → `(30, "default", "")`; `"7"` → `(7, "env", "7")`; `"-1"`/`"400"`/`"abc"` → `(30, "fallback", <verbatim>)`.

**Do**:
- In `internal/log/config.go`, add `func resolveRotateSize(raw string) (int64, string)`. Trim+uppercase the suffix region only; parse the numeric prefix with `strconv.ParseInt` (base 10). Recognise suffixes `K`=1024, `M`=1024*1024, `G`=1024*1024*1024 (binary multipliers — match the spec's "500M never fires even at ~20 MB/day DEBUG steady-state, catches a runaway within ~1 day" sizing; document the chosen 1024 base in a comment). A bare integer with no suffix = bytes. Empty/unset → default `500 * 1024 * 1024` with `source="default"`. Any parse failure (non-numeric prefix, unknown suffix, multiple suffixes, negative, zero) → default with `source="fallback"`.
- Define the default constant explicitly, e.g. `const defaultRotateSize int64 = 500 * 1024 * 1024`.
- In the same file, add `func resolveRetentionDays(raw string) (int, string, string)`. Trim, `strconv.Atoi`. Empty/unset → `(30, "default", "")`. A valid integer in `[0, 365]` → `(n, "env", raw)`. Non-integer, negative, or `>365` → `(30, "fallback", raw)` preserving `raw` verbatim. Note `0` is a valid retention value (delete everything older than today) and resolves `source="env"`; only negative is rejected. Confirm this interpretation against the spec's "non-integer, negative, > 365" rejection list (0 is not in the reject list).
- Define `const defaultRetentionDays = 30`.
- Keep both resolvers pure (take `raw`, return values) so they are unit-testable without env mutation; provide thin wrappers reading `os.Getenv("PORTAL_LOG_ROTATE_SIZE")` / `os.Getenv("PORTAL_LOG_RETENTION_DAYS")` if convenient for the handler-construction call site.
- Do NOT emit any WARN here — the invalid-retention WARN (`log-rotate: invalid PORTAL_LOG_RETENTION_DAYS raw="<v>" retention=30`) is Task 2-8; the invalid-size case has no spec-mandated WARN (only the retention invalid case does — confirm: the spec specifies a WARN only for invalid retention, not invalid size). This task computes and returns; emission is downstream.

**Acceptance Criteria**:
- [ ] `resolveRotateSize("")` → `(524288000, "default")`.
- [ ] `resolveRotateSize` accepts `"500M"`, `"500m"`, `"1G"`, `"1g"`, `"512K"`, and a bare `"1048576"` (bytes), each with `source="env"` and the correct byte count (binary multipliers).
- [ ] `resolveRotateSize` falls back to `(524288000, "fallback")` for `"abc"`, `"5X"`, `"-1"`, `"0"`, `"1.5M"`.
- [ ] `resolveRetentionDays("")` → `(30, "default", "")`.
- [ ] `resolveRetentionDays("7")` → `(7, "env", "7")`; `resolveRetentionDays("0")` → `(0, "env", "0")`; `resolveRetentionDays("365")` → `(365, "env", "365")`.
- [ ] `resolveRetentionDays` falls back to `(30, "fallback", <verbatim>)` for `"-1"`, `"366"`, `"400"`, `"abc"`, `"3.5"`.

**Tests**:
- `"it defaults rotate size to 500M when unset"`
- `"it parses K/M/G suffixes case-insensitively and bare bytes for rotate size"`
- `"it falls back to 500M with source=fallback for an invalid rotate size"`
- `"it defaults retention to 30 days when unset"`
- `"it accepts 0..365 retention with source=env"`
- `"it falls back to 30 with source=fallback for negative, >365, and non-integer retention"`

**Edge Cases**:
- Missing/empty → default (`source="default"`, distinct from `fallback`).
- K/M/G suffixes (both cases) + bare bytes.
- Invalid size string → 500M fallback.
- Retention non-integer / negative / `>365` → 30 fallback; `0` and `365` are valid (`source="env"`).
- `raw` preserved verbatim for retention reporting (the WARN in Task 2-8 renders it).

**Context**:
> "Size-cap safety valve: default **500 MB**, configurable via `PORTAL_LOG_ROTATE_SIZE` (K/M/G suffixes, e.g. `500M`, `1G`). Parsed once at handler init." (spec § Log rotation mechanism → Decision)
>
> "Default retention: **30 days**. Configurable via `PORTAL_LOG_RETENTION_DAYS`. Invalid values (non-integer, negative, > 365) fall back to default with a startup WARN." (spec § Retention policy and audit → Decision)
>
> "Invalid env value (non-integer, negative, > 365) → use default and emit one WARN: `log-rotate: invalid PORTAL_LOG_RETENTION_DAYS raw="<v>" retention=30`." (spec § Retention policy → Mechanical rule step 1)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Decision — size cap), § Retention policy and audit (Decision, Mechanical rule step 1)

---

## portal-observability-layer-2-2 | approved

### Task 2-2: Date-aware fd reuse with inode-identity reopen and first-of-day `O_CREAT|O_EXCL` open

**Problem**: The Phase 1 handler opens `${stateDir}/portal.log` once with a plain `O_APPEND` and writes forever — the exact churn-free-but-evidence-losing posture the feature replaces. Phase 2 must make every `Handle` date-aware: reuse the open fd only while it still points at *today's* file AND its inode still matches the live `portal.log` symlink target; otherwise reopen. A long-lived daemon whose today-file is unlinked out from under it must detect the inode mismatch and reopen onto the live file rather than silently writing into an orphaned inode (the 2026-05-28 unknown-zeroing scenario).

**Solution**: Replace the Phase 1 simple-text-handler's single plain-append open (the commented "Phase-2 rotating-handler insertion point" left by Task 1-4) with a per-`Handle` fd-management step inside the configured handler in `internal/log`. Add date-keyed filename helpers in `internal/log` (over the passed-in `stateDir`), compute `today := time.Now().Format("2006-01-02")`, and gate fd reuse on (a) date unchanged and (b) inode identity (`fstat` open fd vs `stat` symlink target, compare `st_dev`+`st_ino`). On any mismatch, reopen via the first-of-day path: `O_CREAT|O_EXCL|O_APPEND|O_WRONLY` mode `0600`, with `O_APPEND|O_WRONLY` fallback on `EEXIST`. This task delivers fd selection + the create/append-race; the symlink swing (Task 2-3), migration guard (Task 2-4), `chmod` sweep (Task 2-5), size-cap (Task 2-6), retention sweep (Task 2-8), and best-effort write (Task 2-7) are the sibling tasks the reopen path composes with.

**Outcome**: The handler reuses its fd across same-day writes with matching inode; on a date roll it reopens onto the new day's file (and triggers the new-day path); on a same-day inode mismatch or `ENOENT` of the target it reopens onto the live target (or recreates it) WITHOUT running the day-roll sweeps; the very first `Handle` (no file yet) creates `portal.log.<today>` via `O_CREAT|O_EXCL`.

**Do**:
- Add date-keyed filename helpers in `internal/log` (e.g. `internal/log/names.go`), keyed off the passed-in `stateDir` so `internal/log` never imports `internal/state`: `dayFile(stateDir, date) = ${stateDir}/portal.log.<date>`, `symlinkPath(stateDir) = ${stateDir}/portal.log`, plus the `.N` / `.swept.<date>` / `.<pid>.symlink.tmp` name builders used by Tasks 2-3/2-6/2-8. Keep them unexported; document the import-cycle guard in a comment.
- In the configured handler, store the open `*os.File`, the date string it was opened for, and its `st_dev`+`st_ino` (captured at open). Add a `mu sync.Mutex` (or reuse the existing handler write-serialisation lock) guarding the fd-management critical section so concurrent `Handle` calls within one process serialise the reopen + write.
- Implement the per-`Handle` fd-selection step (run before the size-cap check and the actual write):
  1. `today := time.Now().Format("2006-01-02")`.
  2. **Reuse** the open fd only if BOTH: (a) its stored date == `today`; AND (b) `fstat(fd).{Dev,Ino}` == `stat(symlinkPath).{Dev,Ino}` (the symlink target's inode). Use the symlink itself for the `stat` (following it) — if `stat` returns `ENOENT` (target gone), treat as a mismatch.
  3. **Date change** (stored date != today): take the new-day reopen path AND signal the caller to run the day-roll sweeps (the `chmod` past-day sweep of Task 2-5 and the retention sweep of Task 2-8). Return/record a `dateChanged=true` flag so the orchestrating `Handle` knows to invoke those sweeps after the file is open.
  4. **Inode mismatch / `ENOENT`, same day**: reopen by following the current symlink target if it exists (`O_APPEND|O_WRONLY`), else recreate via the first-of-day open (step below). Do NOT set `dateChanged` — the day-roll sweeps must NOT run.
- First-of-day / recreate open: open `dayFile(stateDir, today)` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`, mode `0600`. On `EEXIST` (lost the cross-process create race), retry with `O_APPEND|O_WRONLY`. After a successful open, capture and store the new fd's date + `st_dev`/`st_ino`. (The symlink swing is Task 2-3; the migration guard that precedes the swing is Task 2-4 — leave clearly-marked seams for both, ordered: migration guard → open → swing.)
- Keep the writer unbuffered (write directly to the `*os.File`, no `bufio`) — the locked constraint (Task 2-7 covers best-effort failure handling).
- Leave the size-cap check (Task 2-6) as a clearly-marked seam after the fd is current and before the write.

**Acceptance Criteria**:
- [ ] On the first `Handle` ever (no `portal.log.<today>` exists), the handler creates it via `O_CREAT|O_EXCL` and writes the record.
- [ ] Two same-day records reuse the same open fd (no reopen) when the inode still matches the symlink target.
- [ ] When `portal.log.<today>` is unlinked mid-day out from under the handler, the next `Handle` detects the inode/`ENOENT` mismatch and reopens onto the live (or recreated) target — and does NOT run the day-roll sweeps.
- [ ] When the date string advances, the next `Handle` opens the new day's file and signals the day-roll sweeps to run.
- [ ] An `EEXIST` on the `O_CREAT|O_EXCL` open is handled by an `O_APPEND|O_WRONLY` fallback (cross-process create-race loser appends).
- [ ] No data race under concurrent `Handle` calls (`go test -race`).

**Tests**:
- `"it creates portal.log.<today> via O_CREAT|O_EXCL on the first Handle"`
- `"it reuses the open fd across same-day writes with matching inode"`
- `"it reopens onto the live target on a same-day inode mismatch without running the sweeps"`
- `"it recreates the day file when the symlink target is ENOENT mid-day"`
- `"it opens the new day's file and flags the day-roll sweeps on a date change"`
- `"it falls back to O_APPEND on EEXIST when it loses the create race"`
- `"it is race-free under concurrent Handle"` (run with `-race`)

**Edge Cases**:
- EEXIST create-race → append fallback.
- Same-day inode mismatch (file unlinked mid-day) → reopen without sweeps.
- `ENOENT` on symlink target → recreate via first-of-day open.
- Date-change vs same-day reopen distinction (only date-change runs `chmod` + retention sweeps).
- First `Handle` ever (no file yet) → create.
- Import-cycle guard: date-keyed name helpers live in `internal/log`, never `internal/state`.

**Context**:
> "**Reuse the currently-open fd only while BOTH hold:** (a) no date change … AND (b) its inode still matches the current `portal.log` symlink target (`fstat` the open fd, `stat` the symlink target, compare `st_dev`+`st_ino`). Otherwise reopen. Two reopen triggers, handled differently: **Date change** → run the full new-day path … plus the Retention sweep … **Inode mismatch / `ENOENT` on the target, same day** → … Reopen by following the current symlink target if it exists (`O_APPEND|O_WRONLY`), else recreate via step a. Do **NOT** run the retention/`chmod` sweeps — the date did not change." (spec § Log rotation mechanism → Mechanical rule step 2)
>
> "a. Open `${stateDir}/portal.log.<today>` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`, mode `0600`. b. On `EEXIST`, retry with `O_APPEND|O_WRONLY`." (step 2a–2b)
>
> Import-cycle guard (planning): `internal/log` must NOT import `internal/state`; date-keyed filename helpers belong in `internal/log` over the passed-in `stateDir`.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Mechanical rule step 2, Resolved operational edges); § Defensive invariants (Invariant 2)

---

## portal-observability-layer-2-3 | approved

### Task 2-3: Pid-scoped atomic symlink swing with crash-leftover reclamation

**Problem**: `tail -f portal.log` must always follow today's file regardless of which process owns the boundary, so the `portal.log` symlink has to be swung atomically to point at `portal.log.<today>` (and at `.N` on size-cap rotation). Multiple portal processes can swing concurrently; a naïve shared temp-link name would collide across processes, and a crash between symlink-create and rename would leak a temp.

**Solution**: Add a pid-scoped atomic symlink-swing helper in `internal/log` invoked from the reopen path (Task 2-2) and the size-cap rotation path (Task 2-6). It uses a pid-scoped temp link name `portal.log.<pid>.symlink.tmp`, removes any stale same-pid temp first (crash leftover), `os.Symlink(target, pidTmp)` then `os.Rename(pidTmp, link)` (atomic, last-writer-wins). Because every concurrent swinger's target is identical (`portal.log.<today>` for the same day), a racing swing is benign. A failed swing leaves the prior symlink in place and writes continue to the open fd.

**Outcome**: After a swing, `portal.log` is a symlink to the intended target; a stale same-pid temp from a prior crash is reclaimed and the swing still succeeds; concurrent cross-process swings to the same target converge benignly (last writer wins, no error, no orphaned link); a swing failure does not abort the handler — the prior symlink stays and writes continue.

**Do**:
- In `internal/log` (e.g. `internal/log/symlink.go`), add `func swingSymlink(stateDir, target string) error` where `target` is the bare filename (e.g. `portal.log.<today>` or `portal.log.<today>.<N>`) the link should point at, and `link = symlinkPath(stateDir)`.
- Build the pid-scoped temp name via a helper `pidSymlinkTmp(stateDir, pid) = ${stateDir}/portal.log.<pid>.symlink.tmp` (the `<pid>` is `os.Getpid()`). A single process performs at most one swing at a time, so no counter is needed.
- Steps: `os.Remove(pidTmp)` (best-effort — reclaims a leftover from a prior crash of THIS pid between Symlink and Rename; ignore `ENOENT`); `os.Symlink(target, pidTmp)`; `os.Rename(pidTmp, link)`. `Rename` is atomic on Unix and last-writer-wins.
- Use a **relative** symlink target (the bare `portal.log.<today>` filename, not an absolute path) so the link is stable if the state dir is moved; document this choice. (The `stat`-the-symlink inode check in Task 2-2 follows the link regardless.)
- On any error from `Symlink`/`Rename`, return the wrapped error but do NOT close/clear the open fd — the caller (Task 2-7's best-effort write path) treats a swing failure as WARN-and-continue: "A failed symlink swing leaves the prior symlink in place; writes continue to the open fd."
- Document that a temp leaked by a crash between `Symlink` and `Rename` is reclaimed best-effort on the next swing (the `os.Remove(pidTmp)` first step) and by `portal clean` (the symlink-tmp is a `portal.log.*` sibling the clean sweeps may reclaim — out of scope here, just note it).

**Acceptance Criteria**:
- [ ] After `swingSymlink(dir, "portal.log.2026-05-30")`, `portal.log` is a symlink whose target reads back as `portal.log.2026-05-30`.
- [ ] A pre-existing `portal.log.<pid>.symlink.tmp` (simulating a prior-crash leftover for this pid) is removed and the swing still succeeds.
- [ ] Two concurrent swings to the same target both succeed (last-writer-wins), leaving exactly one valid symlink and no orphaned temp.
- [ ] The temp link name is pid-scoped — a swing by pid A never collides on the temp name with a swing by pid B (verify the temp filename embeds the pid).
- [ ] A swing failure (simulated `Symlink`/`Rename` error) returns an error and leaves the prior symlink in place (the caller continues writing to the open fd).

**Tests**:
- `"it swings the symlink to the target atomically"`
- `"it reclaims a stale same-pid tmp from a prior crash and still swings"`
- `"it converges two concurrent same-target swings to one valid link with no orphan"` (run with `-race`)
- `"it uses a pid-scoped tmp name that cannot collide across pids"`
- `"it leaves the prior symlink in place on a swing failure"`

**Edge Cases**:
- Stale same-pid tmp from a prior crash → remove + recreate.
- Concurrent cross-process swing, identical target, last-writer-wins → benign.
- Swing failure leaves the prior symlink in place (writes continue to the open fd).
- Temp never collides across pids (pid-scoped name).

**Context**:
> "The temp link is **pid-scoped** — `portal.log.<pid>.symlink.tmp` — so cross-process swings can never collide on the tmp name (a single process performs at most one swing at a time, so no counter is needed); if this pid's own tmp already exists from a prior crash, `os.Remove` it and recreate. Then `os.Symlink(target, pidTmp)` + `os.Rename(pidTmp, link)` — `Rename` is atomic and last-writer-wins, and every racer's target is identical … so a concurrent swing is benign. A tmp leaked by a crash between `Symlink` and `Rename` is reclaimed best-effort on the next swing and by `portal clean`." (spec § Log rotation mechanism → Mechanical rule step 2c)
>
> "A failed symlink swing leaves the prior symlink in place; writes continue to the open fd." (spec § Log rotation mechanism → Resolved operational edges)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Mechanical rule step 2c, Resolved operational edges)

---

## portal-observability-layer-2-4 | approved

### Task 2-4: First-run migration guard deleting legacy regular-file `portal.log` / `portal.log.old`

**Problem**: Before this feature, the old logger left a regular-file `portal.log` plus a single `portal.log.old` in the state dir. On the first run under the new symlink-based scheme the handler must clear that legacy slate so the `portal.log` name is free to become a symlink — otherwise the swing (Task 2-3) collides with a pre-existing regular file. After the first run `portal.log` is always a symlink, so the guard must never fire again.

**Solution**: Add a first-run migration guard in `internal/log` that runs inside the reopen path (Task 2-2) **before** the symlink swing: `lstat` `portal.log`; if it is a regular file (not a symlink), `os.Remove` it, and also `os.Remove` any `portal.log.old`. Because the very next swing makes `portal.log` a symlink, a subsequent `lstat` shows a symlink and the guard no-ops forever after.

**Outcome**: On the first reopen after migration, a legacy regular-file `portal.log` and any `portal.log.old` are deleted so the swing can create the symlink cleanly; if `portal.log` is already a symlink (any run after the first), the guard does nothing; absent files are tolerated.

**Do**:
- In `internal/log` (alongside the symlink helper or in the reopen path), add `func migrationGuard(stateDir string)` invoked from the reopen path in Task 2-2 **before** `swingSymlink`.
- Steps: `info, err := os.Lstat(symlinkPath(stateDir))`. If `err` is `ENOENT` → nothing to clear, return (the absent-entirely case). If `info.Mode()&os.ModeSymlink != 0` → it is already a symlink, return (the guard no-ops on every run after the first). Otherwise (a regular file): `os.Remove(symlinkPath(stateDir))` (best-effort) and `os.Remove(${stateDir}/portal.log.old)` (best-effort, `ENOENT`-tolerant).
- Use `Lstat` (not `Stat`) so a symlink is detected as a symlink rather than followed to its target.
- All removals are best-effort: a failure is WARN-and-continue (the swing may still succeed if the rename can replace; if not, Task 2-7's best-effort posture keeps the process alive). Do NOT abort the reopen on a guard failure.
- Ensure the guard runs at most once per file lifetime by construction (the next swing converts `portal.log` to a symlink) — no extra "already migrated" flag is needed; document this.

**Acceptance Criteria**:
- [ ] A pre-existing regular-file `portal.log` is removed by the guard, and the subsequent swing creates a symlink.
- [ ] A pre-existing `portal.log.old` is removed alongside the regular-file `portal.log`.
- [ ] When `portal.log` is already a symlink, the guard no-ops (leaves the symlink and its target untouched).
- [ ] When `portal.log.old` is absent, the guard tolerates it (no error).
- [ ] When `portal.log` is absent entirely, the guard no-ops.
- [ ] On a second reopen (after the first swing made `portal.log` a symlink), the guard does not delete anything.

**Tests**:
- `"it removes a legacy regular-file portal.log on first run"`
- `"it removes portal.log.old alongside the regular file"`
- `"it no-ops when portal.log is already a symlink"`
- `"it tolerates an absent portal.log.old"`
- `"it no-ops when portal.log is absent entirely"`
- `"it does not fire on the second run after the symlink exists"`

**Edge Cases**:
- `portal.log` is a regular file → removed.
- `portal.log` already a symlink → guard no-ops.
- `portal.log.old` absent → tolerated.
- `portal.log` absent entirely → no-op.
- Guard never fires on the second run (symlink present).

**Context**:
> "**(First-run migration guard — clean slate.)** Before swinging the symlink, if `portal.log` exists as a **regular file** (`lstat` shows it is not a symlink), `os.Remove` it; also `os.Remove` any `portal.log.old`. This deletes pre-migration legacy logs on the first run under the new system (the old logger left a regular-file `portal.log` + single `.old`). After the first run `portal.log` is always a symlink, so this guard never fires again." (spec § Log rotation mechanism → Mechanical rule step 2)
>
> "First-startup migration. Clean slate (see step 2 migration guard): legacy regular-file `portal.log` and `portal.log.old` are deleted on first run. Pre-migration history is not preserved." (spec § Log rotation mechanism → Resolved operational edges)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Mechanical rule step 2 migration guard, Resolved operational edges)

---

## portal-observability-layer-2-5 | approved

### Task 2-5: Past-day `chmod 0400` immutability sweep on the day-roll path

**Problem**: Rotated (past-day) files must become immutable so even a buggy library can't overwrite past evidence — narrowing the destruction surface to today's file only (Invariant 1). The sweep must `chmod 0400` only genuine past-day log files, strictly skipping the symlink temp, the `swept` sentinel, and any non-log sibling, and must NOT seal same-day segments (a peer may hold an open `O_APPEND` fd on them).

**Solution**: Add the immutability sweep in `internal/log` (step 2d of the rotation rule), invoked from the reopen path ONLY when `dateChanged==true` (Task 2-2's day-roll signal). It lists `${stateDir}/portal.log.*`, keeps only files whose date portion strict-parses as `portal.log.<YYYY-MM-DD>[.<N>]` and whose date is not today and whose mode is not already `0400`, and `chmod 0400`s each. `chmod` failures are WARN-and-skip; the sweep never aborts.

**Outcome**: On a day roll, every past-day log file (including all of yesterday's `.N` segments and any older days from a multi-day downtime) is `chmod 0400` in one sweep; the symlink temp, the `swept.<date>` sentinel, today's file, and already-`0400` files are skipped; a `chmod` failure logs a WARN and the sweep continues.

**Do**:
- In `internal/log` (e.g. `internal/log/rotate.go`), add `func sealPastDayFiles(stateDir, today string)` invoked from the reopen path after the new day's file is open and the symlink is swung, gated on `dateChanged==true`.
- List `${stateDir}/portal.log.*` via `filepath.Glob` or `os.ReadDir` + prefix filter.
- **Strict date-parse, skip otherwise**: for each sibling, parse the portion after `portal.log.`; a candidate is one matching `portal.log.<date>` or `portal.log.<date>.<N>` where `<date>` parses via `time.Parse("2006-01-02", date)`. Anything that does not strict-parse — `portal.log.<pid>.symlink.tmp`, `portal.log.swept.<date>`, any future non-log sibling — is skipped (never `chmod`'d). This is load-bearing: keeping the symlink temp writable means its best-effort reclamation isn't bricked by a `0400`.
- For each candidate whose date != `today`: `info, _ := os.Stat(file)`; if `info.Mode().Perm() == 0o400` skip (already sealed); else `os.Chmod(file, 0o400)`. On `chmod` error, emit one WARN under component `log-rotate` (`logger.Warn("chmod failed", "error", err, "path", file)`) and continue — never abort the sweep.
- Do NOT `chmod` today's file, and (critically) do NOT seal same-day `.N` segments — those are part of today's active write surface and a peer may hold an open `O_APPEND` fd; they are sealed only when the day rolls over (the next day's sweep seals all of yesterday's segments at once). Verify the candidate filter uses `date != today` so same-day `.N` files are excluded.
- A multi-day downtime catches up in one sweep: because the filter is "any candidate with date < today" (i.e. != today and strict-parsing), ALL past days are sealed in a single pass — no per-missed-day catchup logic.

**Acceptance Criteria**:
- [ ] On a day roll, yesterday's `portal.log.<yesterday>` and all its `.N` segments are `chmod 0400`.
- [ ] After a multi-day downtime, files from every past day are sealed in one sweep.
- [ ] `portal.log.<pid>.symlink.tmp` and `portal.log.swept.<date>` are never `chmod`'d (strict date-parse skip).
- [ ] A file already at mode `0400` is skipped (no redundant `chmod`).
- [ ] Today's file and today's `.N` same-day segments are NOT sealed.
- [ ] A `chmod` failure emits one WARN under `log-rotate` and the sweep continues to the remaining files.

**Tests**:
- `"it chmods all of yesterday's segments to 0400 on a day roll"`
- `"it seals every past day in one sweep after a multi-day downtime"`
- `"it skips the symlink-tmp, the swept sentinel, and non-log siblings"`
- `"it skips a file already at mode 0400"`
- `"it does not seal today's file or today's .N same-day segments"`
- `"it WARNs and continues when a chmod fails"`

**Edge Cases**:
- Strict date-parse skips symlink-tmp + swept sentinel + non-log siblings.
- Already-`0400` skipped.
- Multi-day downtime catches all past days at once.
- `chmod` failure → WARN-and-continue.
- Same-day segments NOT sealed (peer may hold an open fd).

**Context**:
> "d. `chmod 0400` any other `portal.log.<date>*` files in `${stateDir}` that are not `<today>` and not already mode 0400. **Strict date-parse, skip otherwise:** only files whose date portion parses as a valid `YYYY-MM-DD` … are candidates; any other `portal.log.*` sibling — the `portal.log.<pid>.symlink.tmp` swing temp, the `portal.log.swept.<date>` sentinel, any future non-log sibling — is **skipped** … This keeps a leaked symlink temp writable so its best-effort reclamation isn't bricked by a `0400`." (spec § Log rotation mechanism → Mechanical rule step 2d)
>
> "Same-day segments are sealed only when the day rolls over — the next day's step 2d sweep `chmod 0400`s all of yesterday's segments at once." (step 3) … "`chmod` / `unlink` failures during the day-roll-over and retention sweeps are WARN-and-skip (never abort the sweep)." (Resolved operational edges)
>
> "Missed-day catchup. Solved by construction — … step 2d `chmod 0400`s ALL past-day files at once, so a multi-day downtime gap is caught up in a single sweep." (Resolved operational edges)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Mechanical rule step 2d, step 3, Resolved operational edges); § Defensive invariants (Invariant 1)

---

## portal-observability-layer-2-6 | approved

### Task 2-6: Same-day size-cap overflow rotation to `portal.log.<today>.N`

**Problem**: The size cap is a disk-fill safety valve: when today's file plus the next record would reach `PORTAL_LOG_ROTATE_SIZE` (default 500 MB), the handler must roll to a fresh same-day segment `portal.log.<today>.N` so a runaway can't fill the disk. Unlike the day-roll, the previous segment must NOT be sealed (a peer may still be appending to it).

**Solution**: Add the size-cap check + same-day overflow rotation in `internal/log` (step 3 of the rotation rule), invoked after the fd is current (Task 2-2) and before the write (Task 2-7). It compares `current_size + len(serialized)` against the resolved cap (Task 2-1), discovers the next free `.N` via `O_CREAT|O_EXCL` retry against the highest existing `.N`, opens it, and swings the symlink to it (Task 2-3) — without `chmod`ing the prior segment.

**Outcome**: When today's file is at/over the cap, the next record opens `portal.log.<today>.N` (next free monotonic N), the symlink follows it, and writes continue there; in steady state (~20 MB/day at DEBUG) the cap never fires; a peer that didn't observe the rotation keeps appending to the prior segment (acceptable split — the symlink points at the newest).

**Do**:
- In `internal/log` (e.g. `internal/log/rotate.go`), add the size-cap step run on each `Handle` after the fd is current and before writing the serialized record: `if currentSize + int64(len(serialized)) >= rotateSize { rotateSameDay(...) }`. Get `currentSize` from `fstat(fd).Size()` (the open today-file's size).
- `rotateSameDay(stateDir, today string)`:
  1. Find the max existing `N` for today: list `portal.log.<today>.*`, parse the trailing `.N` integer of each, take the max (or 0 if none → next N = 1).
  2. Open `portal.log.<today>.<N>` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`, mode `0600`.
  3. On `EEXIST` (another writer / a stale gap), retry with `N+1`. Loop until a free N is claimed.
  4. Swing the symlink to the new segment via `swingSymlink` (Task 2-3, same pid-scoped-tmp + atomic-rename).
  5. Update the handler's stored fd + date + inode to the new segment, close (or let GC close) the prior segment's fd in THIS process.
- **Do NOT `chmod 0400` the previous segment** — it is a same-day file, a peer may still hold an open `O_APPEND` fd on it (`chmod` does not evict an already-open writer on Unix), and it is part of today's active write surface. Same-day segments are sealed only on the day roll (Task 2-5). Add a code comment stating this explicitly.
- Treat a peer that didn't observe this size-cap rotation continuing to append to the prior same-day segment as acceptable: today's writes split across two readable same-day files, the symlink points at the newest. Document this is acceptable — the cap is a disk-fill valve, not a correctness boundary.
- Use the cap resolved in Task 2-1 (`resolveRotateSize`), stored on the handler at construction; do NOT re-read the env per `Handle`.

**Acceptance Criteria**:
- [ ] When `currentSize + len(record) >= cap`, the handler opens `portal.log.<today>.1` (first overflow) and writes the record there.
- [ ] Max-N discovery: with `.1` and `.3` present (a gap), the next overflow opens `.4` (max+1), not `.2`.
- [ ] With no existing `.N`, the first overflow opens `.1`.
- [ ] An `EEXIST` on the chosen `.N` retries `N+1` until a free segment is claimed.
- [ ] The previous segment is NOT `chmod`'d (it remains mode `0600`) after a same-day rotation.
- [ ] In steady state (record sizes far below the cap), no overflow rotation occurs.

**Tests**:
- `"it rotates to portal.log.<today>.1 when the next record would reach the cap"`
- `"it discovers the next N as max+1 across gaps"`
- `"it opens .1 when no existing .N segments are present"`
- `"it retries N+1 on EEXIST until a free segment is claimed"`
- `"it does not chmod the prior same-day segment after a size-cap rotation"`
- `"it never rotates in steady state below the cap"`

**Edge Cases**:
- Max-N discovery (none → 1, gaps → max+1).
- `EEXIST` on N → retry N+1.
- Prior segment NOT `chmod`'d (peer may hold open fd).
- Peer keeps appending to prior segment (acceptable split).
- Cap never fires in steady state.

**Context**:
> "After fd is open, check `current_size + len(serialized) >= PORTAL_LOG_ROTATE_SIZE`. If true, rotate to `portal.log.<today>.N`: a. Find max existing `N` … next N = max + 1, or 1 if none. b. Open `portal.log.<today>.<N>` with `O_CREAT|O_EXCL|O_APPEND|O_WRONLY`. c. On `EEXIST`, retry with `N+1`. d. Swing the symlink to the new file … **Do NOT `chmod 0400` the previous segment**: it is a *same-day* file, a peer process may still hold an open `O_APPEND` fd on it … Same-day segments are sealed only when the day rolls over … A peer that didn't observe this size-cap rotation simply keeps appending to the prior same-day segment; that splits today's writes across two readable same-day files (the symlink points at the newest), which is acceptable — the size cap is a disk-fill valve, not a correctness boundary." (spec § Log rotation mechanism → Mechanical rule step 3)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Mechanical rule step 3)

---

## portal-observability-layer-2-7 | approved

### Task 2-7: Best-effort write path with stderr fallback and unbuffered-writer guarantee

**Problem**: Logging owns no control flow — a disk-full / `EACCES` / write failure must never crash portal or propagate to the caller. The writer must also be unbuffered so a marker is already in the kernel before `Info(...)` returns (so `os.Exit`/`syscall.Exec` don't discard it). And a failed symlink swing must not stop writes — they continue to the open fd.

**Solution**: Wrap the configured handler's `Handle` write path in best-effort error handling: on open failure attempt a single stderr fallback write of the serialized record then continue; on write failure drop the record and continue; never return an error to the caller. Keep the writer a plain `*os.File` `O_APPEND` write per record (no `bufio`). Treat a swing failure (Task 2-3) and `chmod`/`unlink` failures (Tasks 2-5/2-8) as WARN-and-continue.

**Outcome**: An open failure (or a write failure mid-record) does not propagate to the `logger.Info(...)` caller — the handler attempts a stderr fallback for the record then continues; the writer is unbuffered (the marker is in the kernel before `Info` returns); a failed symlink swing leaves writes flowing to the open fd; portal never crashes due to a logging failure.

**Do**:
- In the configured handler's `Handle`, structure the write path so EVERY failure mode is swallowed:
  - **Open/reopen failure** (Task 2-2's first-of-day or recreate open returns an error): attempt a single stderr fallback write of the serialized record (`fmt.Fprint(os.Stderr, serialized)`), then return `nil` from `Handle` (the record is best-effort dropped to stderr). Do NOT return the error.
  - **Write failure** (the `*os.File.Write`/`WriteString` returns an error mid-record): drop the record and return `nil`. Optionally attempt the stderr fallback once. Never return the error.
  - **Swing failure** (Task 2-3 returns an error): leave the prior symlink in place, keep writing to the currently-open fd, return `nil`. (A WARN under `log-rotate` is acceptable but secondary — the locked behaviour is "writes continue.")
  - **`chmod`/`unlink` failures** in the day-roll/retention sweeps (Tasks 2-5/2-8): WARN-and-skip within those sweeps; never abort `Handle`.
- Guarantee unbuffered writes: the handler writes the serialized bytes directly to the `*os.File` with no `bufio.Writer` wrapper. Add a code comment marking "unbuffered writer is a locked constraint" and verify via a test that after `logger.Info(...)` returns, the bytes are readable from the file without any flush call.
- Ensure `Handle` ALWAYS returns `nil` (or, if slog's contract requires propagating an error in a degenerate case, returns nil for all the swallow paths above). slog's `Logger.{Info,...}` ignore the handler's returned error in practice, but the spec's contract is "logging never crashes portal" — returning nil from `Handle` for these paths is the explicit guarantee.
- Confirm no `panic` path exists in `Handle` for any I/O failure (disk-full, EACCES, ENOSPC, broken symlink) — exercise these in tests by pointing the handler at an unwritable directory / read-only file.

**Acceptance Criteria**:
- [ ] An open failure (unwritable state dir) does not propagate to the `logger.Info(...)` caller; the record is attempted on stderr and the process continues.
- [ ] A write failure mid-record drops the record and `Handle` returns nil; the next record (after a successful reopen) writes normally.
- [ ] A disk-full / `EACCES` open or write never returns an error to the caller and never panics.
- [ ] The writer is unbuffered: after `logger.Info("x")` returns, the line is readable from the file with no explicit flush/`Sync`.
- [ ] A failed symlink swing leaves writes flowing to the open fd (the record still lands in the open file).

**Tests**:
- `"it does not propagate an open failure to the caller and writes to stderr fallback"`
- `"it drops a record on a write failure and continues"`
- `"it never panics or returns an error on disk-full/EACCES"`
- `"it writes unbuffered so the marker is readable before Info returns"`
- `"it keeps writing to the open fd when the symlink swing fails"`

**Edge Cases**:
- Open failure → stderr fallback + continue.
- Write failure mid-record → drop + continue.
- Disk-full / `EACCES` never propagates to caller.
- Writer is unbuffered (marker in kernel before `Info` returns).
- Failed symlink swing → writes continue to open fd.

**Context**:
> "Logging never crashes portal (the logger owns no control flow). On open or write failure the handler is best-effort: it attempts a single stderr fallback write for the record, otherwise drops it, and the process continues. `chmod` / `unlink` failures during the day-roll-over and retention sweeps are WARN-and-skip (never abort the sweep). A failed symlink swing leaves the prior symlink in place; writes continue to the open fd." (spec § Log rotation mechanism → Resolved operational edges)
>
> "Flush reduces to 'do not buffer the log writer.' The rotation handler writes directly to the `*os.File` (`O_APPEND`) with no `bufio` wrapper, so a marker is already in the kernel by the time `Info(...)` returns. … **Unbuffered writer is a locked constraint on the rotation handler.**" (spec § Defensive invariants → Flush)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log rotation mechanism (Resolved operational edges — disk-full/EACCES); § Defensive invariants (Flush — unbuffered writer constraint)

---

## portal-observability-layer-2-8 | approved

### Task 2-8: Single-winner retention sweep with per-deletion breadcrumbs and sentinel prune

**Problem**: Rotated history must be bounded (default 30 days) with an auditable, single-sourced breadcrumb per deletion. On a reboot morning ~32 processes each emit their `process: start` as their first log call of the day — without a gate, all 32 would re-run the deletion loop, emitting 32× duplicate INFO breadcrumbs and 31× "already gone" WARNs on exactly the forensic surface this feature exists to keep clean.

**Solution**: Add the retention sweep in `internal/log` (the Retention policy mechanical rule), invoked from the reopen path on the first `Handle` of each calendar date (the `dateChanged==true` signal from Task 2-2), **after** today's file is opened. It claims the day's sweep via an `O_CREAT|O_EXCL` `portal.log.swept.<today>` sentinel (single-winner gate); only the winner runs it. The winner computes the cutoff from the resolved retention (Task 2-1, emitting the invalid-value WARN here), strict-date-parses the `portal.log.*` siblings, emits one INFO `log-rotate: deleted` BEFORE each deletion, `os.Remove`s files older than the cutoff (WARN-and-continue on error), and prunes stale (`!= today`) `swept.*` sentinels.

**Outcome**: At most one process per host per day runs the sweep; the winner emits one INFO breadcrumb per deleted file (before deletion), falls back to 30 days with a WARN on invalid `PORTAL_LOG_RETENTION_DAYS`, skips the sentinel/tmp/non-log siblings (never deletes them), prunes non-today sentinels, and a partial sweep (winner SIGKILL'd mid-loop) self-heals next day; all losers return immediately, emit nothing, run nothing.

**Do**:
- In `internal/log` (e.g. `internal/log/retention.go`), add `func runRetentionSweep(stateDir, today string, retentionDays int, gated bool)` invoked from the reopen path on `dateChanged==true`, AFTER today's file is opened (so all deletion INFO lines land in today's file, never the file being aged out). The `gated` parameter lets `portal clean --logs` (Task 2-9) reuse this function with the gate bypassed.
  - **Step 0 — single-winner gate** (only when `gated==true`): create `${stateDir}/portal.log.swept.<today>` via `O_CREAT|O_EXCL`. On `EEXIST`, another process owns today's sweep → return immediately, run nothing, emit nothing. On success, this process owns the sweep, proceed. When `gated==false` (the `--logs` path), skip step 0 entirely (always run).
  - **Step 1 — cutoff**: `cutoff := today.AddDate(0, 0, -retentionDays)` (parse `today` to a `time.Time` for the arithmetic). The retention value is resolved in Task 2-1; emit the invalid-value WARN HERE if the resolution `source=="fallback"`: `logger.Warn("invalid PORTAL_LOG_RETENTION_DAYS", "raw", raw, "retention", 30)` under component `log-rotate`. (Render: `log-rotate: invalid PORTAL_LOG_RETENTION_DAYS raw="<v>" retention=30`.)
  - **Step 2 — delete past-cutoff files**: list `${stateDir}/portal.log.*`. **Strict date-parse, skip otherwise**: keep only filenames matching `portal.log.<YYYY-MM-DD>[.<N>]` (date strict-parses); skip the `portal.log.<pid>.symlink.tmp` temp, the `portal.log.swept.<date>` sentinel, any non-log sibling (never deleted). For each surviving file whose date `< cutoff`: (a) emit ONE INFO line BEFORE deletion: `logger.Info("deleted", "path", file, "retention", retentionDays)` under `log-rotate` (render: `log-rotate: deleted path=<file> retention=<N>`); (b) `os.Remove(file)`; on error emit one WARN with the `error` attr and continue (don't abort the sweep).
  - **Step 3 — prune stale sentinels**: unlink any `portal.log.swept.<date>` sentinel whose `<date>` != `today`. On error, WARN and continue. (This is why step 2 excludes `swept.*` from the cutoff walk — sentinels are pruned here by an exact not-today rule.)
- The sweep is best-effort and synchronous inside the winner's first-of-day `Handle` — which is the `process: start` line emitted during `Init` (Task 2-11), before the process does work or `syscall.Exec`s, so even a short-lived winner completes the sweep before exiting. Document the accepted partial-sweep risk: a winner SIGKILL'd mid-deletion-loop leaves a few extra rotated files until next day's fresh winner sweeps — self-heals, no resumable sentinel.
- Reuse the strict-date-parse helper from Task 2-5 (the candidate filter is identical: `portal.log.<date>[.<N>]` with date strict-parsing).

**Acceptance Criteria**:
- [ ] When `portal.log.swept.<today>` already exists, a gated sweep returns immediately — runs nothing, emits nothing (no INFO, no WARN).
- [ ] The winner emits exactly one INFO `log-rotate: deleted path=<file> retention=<N>` per deleted file, BEFORE the `os.Remove`.
- [ ] Files with date `< cutoff` are deleted; files within the retention window are kept.
- [ ] An invalid `PORTAL_LOG_RETENTION_DAYS` causes a fallback to 30 with one WARN `log-rotate: invalid PORTAL_LOG_RETENTION_DAYS raw="<v>" retention=30`.
- [ ] The `portal.log.<pid>.symlink.tmp` and `portal.log.swept.<date>` siblings are never deleted by the cutoff walk (strict date-parse skip).
- [ ] Stale `portal.log.swept.<date>` sentinels (date != today) are pruned; today's sentinel is kept.
- [ ] An `os.Remove` failure mid-loop emits one WARN and the sweep continues to the remaining files.
- [ ] A second process on the same day (sentinel already present) does not duplicate any breadcrumb.

**Tests**:
- `"it returns immediately when it loses the single-winner gate, emitting nothing"`
- `"it emits one INFO deleted breadcrumb before each os.Remove"`
- `"it deletes files older than the cutoff and keeps files within the window"`
- `"it falls back to 30 days with a WARN on invalid PORTAL_LOG_RETENTION_DAYS"`
- `"it never deletes the symlink-tmp or the swept sentinel"`
- `"it prunes stale non-today swept sentinels and keeps today's"`
- `"it WARNs and continues on an os.Remove failure"`
- `"it single-sources the deletion breadcrumbs across concurrent startups"`

**Edge Cases**:
- `EEXIST` gate loss → return immediately (no emit / no run).
- Invalid `PORTAL_LOG_RETENTION_DAYS` → WARN + default 30.
- INFO breadcrumb BEFORE delete (lands in today's file).
- `os.Remove` failure → WARN + continue.
- Strict date-parse excludes sentinel / tmp / non-log siblings.
- Stale non-today sentinels pruned.
- Partial-sweep crash self-heals next day (accepted risk).

**Context**:
> "0. **Single-winner gate.** Create `${stateDir}/portal.log.swept.<today>` via `O_CREAT|O_EXCL`. On `EEXIST`, another process already owns today's sweep — **return immediately, run nothing, emit nothing.** … 1. `cutoff := today.AddDate(0, 0, -PORTAL_LOG_RETENTION_DAYS)` … Invalid env value … → use default and emit one WARN … 2. List … **Strict date-parse, skip otherwise** … a. Emit one INFO line BEFORE deletion: `log-rotate: deleted path=<file> retention=<N>`. b. `os.Remove(file)`. On error, emit one WARN … 3. **Prune stale sentinels.** Unlink any `portal.log.swept.<date>` sentinel whose `<date>` ≠ `today`." (spec § Retention policy → Mechanical rule)
>
> "Winner-completion semantics. The sweep (steps 0–3) runs **synchronously inside the winner's first-of-day `Handle`** … even a short-lived winner … completes the whole sweep before it exits … The only failure of at-least-once-completed is a winner SIGKILL'd or crashing mid-deletion-loop … an **accepted risk** … self-heals." (spec § Retention policy → Winner-completion semantics)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Retention policy and audit (Mechanical rule steps 0–4, Winner-completion semantics, Resolved operational edges)

---

## portal-observability-layer-2-9 | approved

### Task 2-9: `portal clean --logs` gate-bypassing sweep with `cutoff=today`

**Problem**: `portal clean` today does project + hook cleanup and has no flags; it preserves rotated logs. The spec adds an opt-in `--logs` flag that triggers the retention sweep with `cutoff=today` (delete every rotated file, leaving only the current one), bypassing the single-winner gate because it is an explicit user invocation that must always run regardless of any `portal.log.swept.<today>` sentinel.

**Solution**: Add a `--logs` boolean flag to `cmd/clean.go`. When set, call into the same sweep function (Task 2-8's `runRetentionSweep`) with the gate bypassed (`gated=false`) and `retentionDays=0` (so `cutoff=today`), and also remove stale `portal.log.swept.*` sentinels. When the flag is absent, behaviour is unchanged — logs are preserved and the sweep is not triggered. Expose the sweep entry point from `internal/log` so `cmd/clean.go` can invoke it without duplicating the algorithm.

**Outcome**: `portal clean --logs` deletes every rotated `portal.log.<date>[.N]` file (leaving only the current symlink target — today's file is within the window when `retentionDays=0` because the cutoff is today and only files with date `< today` are deleted), bypasses the `swept.<today>` gate, removes stale `swept.*` sentinels, and reuses the same sweep code; `portal clean` (no flag) preserves logs exactly as today.

**Do**:
- Add an exported sweep entry point to `internal/log`, e.g. `func SweepLogs(stateDir string, retentionDays int, gated bool) error` wrapping Task 2-8's `runRetentionSweep`. (The `--logs` path passes `retentionDays=0, gated=false`; the per-process path passes the resolved retention with `gated=true`.) This is the single sweep function both call sites share — no duplication.
- In `cmd/clean.go`, register a `--logs` boolean flag on `cleanCmd` (`cleanCmd.Flags().Bool("logs", false, "also delete rotated portal.log history older than today")`). Default false.
- In `cleanCmd.RunE`, after the existing project + hook cleanup, read the flag (`logs, _ := cmd.Flags().GetBool("logs")`). When `logs` is true: resolve the state dir (the same resolution the rest of the code uses — `state.EnsureDir()` / `state.Dir()`), then call `log.SweepLogs(stateDir, 0, false)`. When false, do nothing new (logs preserved).
- Confirm `cutoff=today` semantics: with `retentionDays=0`, `cutoff := today.AddDate(0,0,0) == today`, and step 2 deletes files with date `< cutoff` (strictly less than today) — so today's file (and today's `.N` segments) survive, and every prior-day rotated file is deleted. This matches "delete every rotated file, leaving only the current one." Verify the strict `<` comparison so today's file is not deleted.
- The `--logs` path must remove stale `portal.log.swept.*` sentinels — step 3 of the sweep already prunes non-today sentinels; with the gate bypassed it also should remove the `swept.<today>` if present (an explicit clean should leave no sentinel). Confirm/extend step 3 (or the `gated=false` path) so all `swept.*` sentinels — including today's — are removed on the `--logs` path. Document this is the `--logs`-only behaviour (the per-process gated path keeps today's sentinel as its claim).
- Note `clean` is in the bootstrap-exclusion list (it does not run the 11-step bootstrap), so the sweep here is the deliberate user-triggered one, not the per-process automatic sweep. Do NOT wire the automatic sweep into `clean`.
- `[needs-info]`: the spec says `--logs` "also removes stale `portal.log.swept.*` sentinels" and "BYPASSES the step-0 single-winner gate." It does not explicitly say whether `--logs` removes today's sentinel too. Interpreting "leaving only the current [log] file" + "explicit deliberate user invocation that must always run" as: remove ALL `swept.*` sentinels (the user wants a clean slate; the next per-process startup will re-claim its own gate). Implement that; flag in Context that this is the chosen reading.

**Acceptance Criteria**:
- [ ] `portal clean` (no `--logs`) does not trigger the sweep — rotated logs are preserved.
- [ ] `portal clean --logs` deletes every `portal.log.<date>[.N]` file with date `< today`, leaving today's file (the current symlink target) intact.
- [ ] `portal clean --logs` bypasses the `portal.log.swept.<today>` gate (it runs even when that sentinel exists).
- [ ] `portal clean --logs` removes stale `portal.log.swept.*` sentinels.
- [ ] Both the `--logs` path and the per-process path call the same sweep function (no algorithm duplication).
- [ ] The `--logs` flag is registered on `cleanCmd` and defaults to false.

**Tests**:
- `"it preserves rotated logs when --logs is absent"`
- `"it deletes every prior-day rotated file with --logs, leaving today's"`
- `"it bypasses the swept.<today> gate with --logs"`
- `"it removes stale swept.* sentinels with --logs"`
- `"it reuses the same sweep function as the per-process path"`
- `"the --logs flag defaults to false"`

**Edge Cases**:
- No flag → logs preserved (sweep not triggered).
- `--logs` bypasses `swept.<today>` gate.
- `cutoff=today` leaves only the current file (date `< today` deleted, today kept).
- Removes stale `swept.*` sentinels.
- Reuses the same sweep function (no duplication).

**Context**:
> "`portal clean` (no flag): preserves rotated logs; does NOT trigger the sweep. `portal clean --logs`: triggers the sweep with `cutoff = today` (delete every rotated file, leaving only the current one). **BYPASSES the step-0 single-winner gate** — it is an explicit, deliberate user invocation and must always run regardless of any `portal.log.swept.<today>` sentinel … It also removes stale `portal.log.swept.*` sentinels." (spec § Retention policy → `portal clean` integration)
>
> "This applies to ONE seam: the `slog.Handler` in `internal/log` (and `portal clean --logs`, which calls into the same sweep function)." (spec § Retention policy)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Retention policy and audit (`portal clean` integration); current `cmd/clean.go` (no flags today)

---

## portal-observability-layer-2-10 | approved

### Task 2-10: Lifecycle-marker level-filter bypass in the handler

**Problem**: The `process`-component lifecycle markers (`start`, `exit`, `exec`, `panic`, and `log-level resolved`) are forensic tripwires (and a test anchor) that MUST appear even when `PORTAL_LOG_LEVEL=warn`/`error` would otherwise filter an INFO line. Without a bypass, the always-present-tripwire guarantee and the "line that proves the resolved level took effect" both fail at WARN/ERROR.

**Solution**: Special-case the closed `process`-lifecycle message set in the configured handler so those records write through the level gate unconditionally; every other INFO record stays subject to the configured level. Identify a bypass record by `component == "process"` AND `msg` in the closed lifecycle set. The markers remain semantically INFO for every other purpose (no ERROR pollution).

**Outcome**: At `PORTAL_LOG_LEVEL=warn` or `error`, a `process`-lifecycle INFO record (`start`/`exit`/`exec`/`log-level resolved`) is still emitted; a non-`process` INFO record is still filtered; an arbitrary `process`-component record whose msg is NOT in the closed set is filtered normally (only the closed set bypasses).

**Do**:
- In the configured handler (`internal/log`), define the closed lifecycle bypass set as a package-level set, e.g. `var lifecycleBypassMsgs = map[string]bool{"start": true, "exit": true, "exec": true, "panic": true, "log-level resolved": true}`. Document it is the closed `process`-lifecycle set from the spec and that adding to it requires a spec amendment.
- In `Handle` (NOT `Enabled` — `Enabled` is a coarse pre-filter slog may call without the record; the bypass needs the record's `component` + `msg`): before applying the level filter, check `component == "process" && lifecycleBypassMsgs[record.Message]`. If true, write the record unconditionally (skip the level-gate drop). Otherwise apply the normal level filter.
- Be careful with slog's `Enabled` pre-gate: slog's `Logger.Info` calls `Handler.Enabled(ctx, LevelInfo)` first and skips `Handle` entirely if it returns false. So `Enabled` must NOT return false for the bypass case. Two options: (a) make `Enabled` return true for `LevelInfo` always (then `Handle` does the real filtering for non-lifecycle INFO) — simplest and correct because the handler then owns all filtering; or (b) keep `Enabled` level-accurate but ensure the lifecycle emitters call `Handle` directly. Prefer (a): have `Enabled` admit `LevelInfo` and above regardless of configured level (so lifecycle INFO is never pre-gated out), and do the authoritative level filtering inside `Handle` (drop non-lifecycle records below the configured level). Document this design (the handler, not `Enabled`, is the filter authority) — note the minor cost that non-lifecycle INFO records reach `Handle` even when configured at WARN, where they are dropped; this is negligible and necessary for the bypass.
- `[needs-info]`: the spec does not pin whether bypass is keyed on `component=="process"` alone or `component+msg`. The four-way classification and "not arbitrary `process:` lines" (edge case) require `component+msg` — only the closed lifecycle msg set bypasses. Implement `component+msg`; flag this reading.
- Keep the markers semantically INFO (do not bump them to a higher level to force them through) — the bypass is the mechanism, not a level change.

**Acceptance Criteria**:
- [ ] At configured level WARN, a `process`-component record with msg `start`/`exit`/`exec`/`log-level resolved` is still emitted.
- [ ] At configured level ERROR, the same lifecycle records are still emitted.
- [ ] At configured level WARN, a non-`process` INFO record (e.g. `component=daemon`, msg `tick complete`) is filtered (not emitted).
- [ ] A `process`-component INFO record whose msg is NOT in the closed lifecycle set is filtered at WARN (only the closed set bypasses).
- [ ] At configured level INFO/DEBUG, all records behave normally (lifecycle and non-lifecycle both emitted per the level).

**Tests**:
- `"it emits process lifecycle markers at configured level WARN"`
- `"it emits process lifecycle markers at configured level ERROR"`
- `"it filters a non-process INFO record at WARN"`
- `"it filters an arbitrary process INFO whose msg is not in the lifecycle set at WARN"`
- `"it behaves normally at INFO and DEBUG levels"`

**Edge Cases**:
- Process lifecycle msgs emitted at WARN/ERROR level.
- Non-process INFO still filtered at WARN.
- Only the closed lifecycle msg set bypasses (not arbitrary `process:` lines).
- Identification by component + msg.

**Context**:
> "The `process`-component lifecycle set — `start`, `exit`, `exec`, `panic`, and `log-level resolved` … — is emitted **unconditionally by the custom handler, regardless of `PORTAL_LOG_LEVEL`.** … At `PORTAL_LOG_LEVEL=warn`/`error` a normal INFO line would be filtered — which would falsify the 'always-present tripwire' guarantee and hide the line that proves the resolved level took effect. The handler special-cases this `process` lifecycle set to write through the level gate. They remain semantically INFO for every other purpose (no ERROR pollution)." (spec § Defensive invariants → Lifecycle markers bypass the level filter)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants (Lifecycle markers bypass the level filter); § Log-level propagation verification (unconditional emission)

---

## portal-observability-layer-2-11 | approved

### Task 2-11: `process: start` and `log-level resolved` emission in `Init` (with invalid-level WARN)

**Problem**: Phase 1's `Init` builds and swaps the handler but leaves the lifecycle-marker emission as a Phase-2 seam. Every portal process must emit exactly one `process: start` INFO line as the final action of `Init`, then exactly one `log-level resolved` INFO line immediately after — both bypassing the level filter — and, when the level resolved via fallback, the prescribed invalid-level WARN. The `process: start` line is also the first-of-day `Handle` that drives the retention sweep (Task 2-8), so it must be emitted before the process does any work.

**Solution**: Fill the Phase-1 `Init` seam in `internal/log`: after constructing and swapping in the configured handler (and after Task 2-2's date-aware open path is in place), emit `process: start` (with `cmd`/`args` attrs), then `log-level resolved` (with `resolved`/`source`/`raw` attrs), and — when the level resolution source is `fallback` — the bootstrap invalid-level WARN. Baseline attrs (`pid`/`version`/`process_role`) are auto-injected by the handler (Phase 1 Task 1-3); the call site does NOT pass them.

**Outcome**: `Init` emits exactly one `process: start cmd=<base> args="<joined>"` line and exactly one `process: log-level resolved resolved=<lvl> source=<src> raw="<raw>"` line (in that order, both bypassing the level filter), plus one `bootstrap: invalid PORTAL_LOG_LEVEL raw="<v>" resolved=info` WARN iff the level resolved via fallback.

**Do**:
- In `internal/log.Init`, after the configured handler is swapped in (so these lines route to the real log file) and before returning, emit as the FINAL actions, in order:
  1. `log.For("process").Info("start", "cmd", filepath.Base(os.Args[0]), "args", strings.Join(os.Args[1:], " "))`. This is the very first record to the handler, which (via Task 2-2) triggers the first-of-day open and (for the winner) the retention sweep — so it must run before any other portal code logs.
  2. `log.For("process").Info("log-level resolved", "resolved", resolvedLevelStr, "source", levelSource, "raw", rawEnvValue)`. Use the Phase-1 level resolver (Task 1-2) results: `resolvedLevelStr` is the lowercase level string (`debug`/`info`/`warn`/`error`), `levelSource` is `env`/`default`/`fallback`, `rawEnvValue` is the verbatim observed env value (empty string if unset).
- When the level resolution `source == "fallback"`, ALSO emit one WARN under component `bootstrap`: `log.For("bootstrap").Warn("invalid PORTAL_LOG_LEVEL", "raw", rawEnvValue, "resolved", "info")` — render: `bootstrap: invalid PORTAL_LOG_LEVEL raw="<v>" resolved=info`. (Note: this WARN is under `bootstrap`, not `process` — per the spec's exact render string. It does NOT bypass the level filter, but at the production INFO default it is visible; document that at WARN/ERROR it is also visible because WARN ≥ those levels are at-or-above the filter.)
- Do NOT pass `pid`/`version`/`process_role` at these call sites — they are baseline attrs auto-injected per-record by the configured handler (Task 1-3).
- Ensure both `process` lines bypass the level filter (Task 2-10) — verify by emitting them through a handler configured at WARN and asserting they appear.
- Keep `Init` idempotent (Phase 1 contract): a second `Init` re-emits `process: start` + `log-level resolved` (the most recent `Init` defines the process's logical start; the spec's `startTime` reset in Phase 1 already covers `took`). Document that a second `Init` re-emits these markers (acceptable — `main` calls `Init` once in prod).
- Confirm the exact attr render order matches the spec examples: `process: start cmd=portal args="open ." pid=… version=… process_role=…` and `process: log-level resolved resolved=info source=default raw="" pid=… …`.

**Acceptance Criteria**:
- [ ] `Init` emits exactly one `process: start` INFO line with `cmd=<filepath.Base(os.Args[0])>` and `args="<strings.Join(os.Args[1:], " ")>"`, as its final pre-return action.
- [ ] `Init` emits exactly one `process: log-level resolved` INFO line immediately after `start`, with `resolved`/`source`/`raw` reflecting the resolution.
- [ ] `source=env` for a valid env value, `source=default` for unset (raw=""), `source=fallback` for an invalid value.
- [ ] When `source=fallback`, `Init` also emits one `bootstrap: invalid PORTAL_LOG_LEVEL raw="<v>" resolved=info` WARN.
- [ ] Both `process` lines are visible even when the configured level is WARN or ERROR (level-filter bypass).
- [ ] Baseline attrs (`pid`/`version`/`process_role`) are auto-injected — the call sites do not pass them.

**Tests**:
- `"it emits process: start exactly once as the final pre-return action with cmd and args"`
- `"it emits log-level resolved immediately after start"`
- `"it renders source=env/default/fallback correctly"`
- `"it emits the bootstrap invalid-level WARN only on the fallback path"`
- `"both process lines are visible when configured at WARN"`
- `"it does not pass baseline attrs at the call site (they are auto-injected)"`

**Edge Cases**:
- `start` emitted exactly once as the final pre-return action.
- `log-level resolved` immediately after `start`.
- `source=env`/`default`/`fallback` rendering.
- Fallback also emits the bootstrap WARN.
- Both lines bypass the level filter (visible at warn/error).
- Baseline attrs auto-injected, not passed.

**Context**:
> "`internal/log.Init(stateDir, version, processRole)`, after constructing the root logger and wiring the rotating handler, MUST emit exactly one INFO line as its final action before returning: `log.For("process").Info("start", "cmd", filepath.Base(os.Args[0]), "args", strings.Join(os.Args[1:], " "))`." (spec § Defensive invariants → Mechanical rule — `process: start`)
>
> "`internal/log.Init(...)` MUST emit one INFO line immediately AFTER the `process: start` line and BEFORE returning: `log.For("process").Info("log-level resolved", "resolved", resolvedLevelStr, "source", levelSource, "raw", rawEnvValue)`." (spec § Log-level propagation verification → Mechanical rule)
>
> "Invalid env value … → fall back to `info` and emit one WARN at process start: `bootstrap: invalid PORTAL_LOG_LEVEL raw="<v>" resolved=info`." (spec § Log-level discipline → Default and invalid-value handling)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants (Mechanical rule — `process: start`), § Log-level propagation verification (Mechanical rule), § Log-level discipline (Default and invalid-value handling)

---

## portal-observability-layer-2-12 | approved

### Task 2-12: `process: exit` emission in `Close`

**Problem**: Phase 1's `Close` computes `took` from `startTime` and owns no control flow, but leaves the actual `process: exit` INFO emission as a Phase-2 seam. The terminal marker must fire so the four-way classification (`exit`/`exec`/`panic`/nothing) of any `process: start` works.

**Solution**: Fill the Phase-1 `Close` seam in `internal/log`: emit exactly one `process: exit` INFO line with `code` (the passed `exitCode`) and `took` (computed from the package-private `startTime` captured at `Init`). `Close` still does NOT call `os.Exit` — it remains a pure marker-emitter. The line bypasses the level filter (Task 2-10).

**Outcome**: `Close(N)` emits exactly one `process: exit code=N took=<dur>` INFO line and returns normally (no `os.Exit`); called before any `Init`, it is safe (no panic; `took` is bounded — computed from a zero/sentinel `startTime`); the line is visible even at WARN/ERROR.

**Do**:
- In `internal/log.Close(exitCode int)`, fill the Phase-1 seam: `log.For("process").Info("exit", "code", exitCode, "took", time.Since(startTime))` (render: `process: exit code=0 took=2.1s pid=… version=… process_role=…`).
- Do NOT call `os.Exit` — preserve the Phase-1 no-control-flow contract. `main` owns the single `os.Exit`.
- Compute `took` from the package-private `startTime` captured at `Init` (Phase 1 Task 1-4 already captures/resets it). `took` must be non-negative for a normal `Init`→`Close` sequence.
- Make `Close` safe before any `Init`: guard so a zero-value `startTime` does not panic. `time.Since(zeroTime)` is a very large but valid duration — acceptable (the line still renders); ensure no nil-deref on the handler (the Phase-1 swap indirection always holds a valid handler, default or configured). Document the pre-`Init` behaviour: the line routes to the pre-`Init` default stderr-text handler, `took` is large-but-bounded, no panic.
- `code` attr reflects the passed `exitCode` verbatim (0 for clean, 1/2 for error/usage). Do NOT pass baseline attrs (`pid`/`version`/`process_role` are auto-injected).
- Ensure exactly one `exit` line per `Close` call (no double-emit). `main` (Task 2-13) skips `Close` on the panic path so `exit` and `panic` never both fire.

**Acceptance Criteria**:
- [ ] `Close(0)` emits exactly one `process: exit code=0 took=<dur>` INFO line and returns without calling `os.Exit`.
- [ ] `Close(1)` renders `code=1`; `Close(2)` renders `code=2`.
- [ ] `took` computed from `startTime` is non-negative for a normal `Init`→`Close` sequence.
- [ ] `Close` called before any `Init` does not panic and renders a bounded `took`.
- [ ] Exactly one `exit` line per `Close` call (no double-emit).
- [ ] The `exit` line is visible even when configured at WARN/ERROR (level-filter bypass).

**Tests**:
- `"it emits process: exit with the passed code and a took from startTime"`
- `"it does not call os.Exit"`
- `"it computes a non-negative took for a normal Init then Close"`
- `"it is safe to call Close before Init (no panic, bounded took)"`
- `"it emits exactly one exit line per Close call"`
- `"the exit line is visible at configured level WARN"`

**Edge Cases**:
- `code` attr reflects `exitCode`.
- `took` from `startTime` non-negative.
- `Close` still never calls `os.Exit`.
- `Close` before `Init` safe (no panic, `took` bounded).
- Exactly one exit line per `Close` call.

**Context**:
> "`internal/log` exposes `func Close(exitCode int)` — a marker-emitter that computes `took` from the package-private `startTime` (captured at `Init`) and emits one INFO line. **`Close` does NOT call `os.Exit`; the logger owns no control flow.** `log.For("process").Info("exit", "code", exitCode, "took", time.Since(startTime))`." (spec § Defensive invariants → Mechanical rule — `process: exit` and the `main` exit shape)
>
> Render: `2026-05-30T14:00:02Z INFO process: exit code=0 took=2.1s pid=12345 version=0.5.0 process_role=tui`.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants (Mechanical rule — `process: exit` and the `main` exit shape)

---

## portal-observability-layer-2-13 | approved

### Task 2-13: `process: panic` emission in the `main` recover block

**Problem**: Phase 1's `main` adopts the single-exit shape with a recover block that maps a recovered panic to `code=2`/`panicked=true`, leaving the `process: panic` marker emission as a Phase-2 seam. On a recovered panic, the sole terminal marker must be `process: panic` (ERROR, with `reason`); `Close` must stay skipped so `exit` and `panic` never both fire — keeping the four-way classification mutually exclusive.

**Solution**: Fill the Phase-1 seam in `main.go`'s recover block: emit `log.For("process").Error("panic", "reason", r)` when a panic is recovered, before setting `code=2`/`panicked=true`. The existing non-panic path is unchanged (it calls `Close(code)` per Task 2-12). The panic path does NOT call `Close`.

**Outcome**: A recovered panic emits exactly one ERROR `process: panic reason=<r>` line, sets `code=2`, skips `Close`, and `os.Exit(2)`; a clean/error run is unchanged (emits `process: exit` via `Close`); no run emits both `exit` and `panic`.

**Do**:
- In `main.go`'s recover block (the Phase-1 commented seam), add `log.For("process").Error("panic", "reason", r)` as the first action inside `if r := recover(); r != nil { ... }`, before `code = 2; panicked = true`.
- The `reason` attr carries the recovered value `r` directly (slog renders it; `r` is `any` — slog handles arbitrary values). Note `reason` is the cross-listed key (defined in the Lifecycle group, cross-listed in Process) — do NOT introduce a new key.
- Keep the non-panic path calling `log.Close(code)` (Task 2-12) only when `!panicked` — so the panic path skips `Close`. Verify the Phase-1 structure already guards `Close` behind `!panicked`; this task only adds the panic-line emission.
- `process: panic` is the SOLE terminal marker on the panic path — confirm `Close` (and thus `process: exit`) does not also fire. The four-way classification stays mutually exclusive: a `process: start` is followed by exactly one of `exit` / `exec` / `panic` / nothing.
- The panic line is ERROR (per the spec's example and the level-discipline "line immediately preceding panic/exit → Error"). It also bypasses the level filter (Task 2-10 includes `panic` in the closed bypass set), so it is visible regardless of `PORTAL_LOG_LEVEL` — but as ERROR it would pass any filter anyway; the bypass is belt-and-suspenders for completeness.
- Because `main` calls `os.Exit`, drive the test via the Phase-1 extracted `run() (code int, panicked bool)` helper (or the same testable seam Task 1-7 introduced) plus a `log.SetTestHandler` capture to assert the `process: panic` record fires on the recovered-panic path and `process: exit` does not.

**Acceptance Criteria**:
- [ ] A panic during `Execute()` is recovered, emits exactly one ERROR `process: panic reason=<r>` line, sets `code=2`, and skips `Close`.
- [ ] On the panic path, no `process: exit` line is emitted (mutually exclusive with `panic`).
- [ ] The non-panic path is unchanged: a clean run emits `process: exit code=0` via `Close`; an error run emits `process: exit code=N`.
- [ ] The `reason` attr carries the recovered panic value.
- [ ] `os.Exit(2)` on the panic path (preserving Phase 1's code mapping).

**Tests**:
- `"it emits ERROR process: panic with reason on a recovered panic"`
- `"it skips Close (no process: exit) on the panic path"`
- `"it emits process: exit (not panic) on a clean run"`
- `"the four-way classification stays mutually exclusive"`
- `"a recovered panic still exits with code 2"`

**Edge Cases**:
- Panic emits ERROR `process: panic` with `reason`.
- `Close` still skipped on the panic path (no double terminal marker).
- Non-panic path unchanged.
- Four-way classification stays mutually exclusive.

**Context**:
> The `main` exit shape: `defer func() { if r := recover(); r != nil { log.For("process").Error("panic", "reason", r); code = 2; panicked = true } }()` … "Exactly one terminal marker fires per run: `exit` on clean/error return, `panic` on a recovered panic. … On the panic path `process: panic` is the **sole** terminal marker — `Close` is skipped — so the four-way classification stays mutually exclusive." (spec § Defensive invariants → Mechanical rule — `process: exit` and the `main` exit shape)
>
> "The `process: panic` line additionally carries `reason` — that key is **defined in the Lifecycle group and cross-listed here** (not a separate key …)." (spec § Subsystem prefix taxonomy → Process attr group)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants (Mechanical rule — `process: exit` and the `main` exit shape, four-way classification); current `main.go` recover seam (Phase 1)

---

## portal-observability-layer-2-14 | approved

### Task 2-14: `process: exec` marker at the `AttachConnector` bare-shell handoff

**Problem**: `syscall.Exec` overwrites the process image, runs no deferred functions, and never returns — so `Close` never fires. The bare-shell `portal open` happy path (`AttachConnector` → `tmux attach-session`) is portal's most common termination; without a marker, a benign tmux handoff leaves an unpaired `process: start` indistinguishable from a destructive mid-flight kill.

**Solution**: Emit a plain `process: exec` INFO line immediately before the `syscall.Exec` in `AttachConnector.Connect` (`cmd/open.go`), under the `process` component, carrying `target="tmux"` and `args=<joined argv>`. The call site uses the ordinary logger then performs its own `syscall.Exec` (no logger-owned helper). Because the writer is unbuffered (Task 2-7), the marker is in the kernel before the image is replaced. `SwitchConnector` (in-tmux path) is unaffected — it returns normally and gets a proper `process: exit` via `Close`.

**Outcome**: `portal open` bare-shell attach emits `process: exec target=tmux args="attach-session -t =<name>"` immediately before `syscall.Exec`; `SwitchConnector` emits no exec marker (it returns and `main`'s `Close` emits `exit`); the args are logged verbatim.

**Do**:
- In `cmd/open.go`, in `AttachConnector.Connect`, immediately before `ex.Exec(tmuxPath, argv, os.Environ())`, add: `log.For("process").Info("exec", "target", "tmux", "args", strings.Join(argv[1:], " "))` where `argv` is `[]string{"tmux", "attach-session", "-t", "=" + name}`. Use `argv[1:]` so `args` renders the tmux subcommand + flags (`attach-session -t =<name>`) without the leading `tmux` program name, matching the spec example `args="attach-session -A …"` (the `target` already names tmux). Confirm the exact join slice against the spec's rendered example.
  - `[needs-info]`: the current argv is `{"tmux", "attach-session", "-t", "=" + name}` (exact-match `-t =<name>`), while the spec example shows `args="attach-session -A …"`. The `-A` variant is from the spec's illustrative example, not the live argv. Log the LIVE argv (`attach-session -t =<name>`), not the spec's example string — the marker must reflect what is actually exec'd. Flag this so a reviewer confirms the rendered `args` matches the real handoff.
- Bind/use the `process` component logger via `log.For("process")` (this is binary-level lifecycle, so `exec` joins `start`/`exit`/`panic` in the `process` component's event space — NOT under a per-subsystem component).
- The marker is INFO and bypasses the level filter (Task 2-10 includes `exec` in the closed bypass set), so it is visible regardless of `PORTAL_LOG_LEVEL`.
- Do NOT add an exec marker to `SwitchConnector.Connect` — it runs `tmux switch-client` as a subprocess and returns normally, so it gets a proper `process: exit` via `Close`. Only true `syscall.Exec` replace-process sites need the exec marker.
- `args` is logged verbatim (privacy posture: portal's single-user threat model accepts the full args string in `portal.log`).
- Because the marker must be in the kernel before the image is replaced, rely on the unbuffered-writer guarantee (Task 2-7) — no `Sync()`/flush call is needed; add a code comment noting the marker is pre-exec and the writer is unbuffered.
- Test via the existing `execer` injection seam: `AttachConnector` has an `execer` field (and `tmuxPath`) for test substitution — inject a fake execer that records the call instead of replacing the process, plus a `log.SetTestHandler` capture to assert the `process: exec` record was emitted BEFORE the (mocked) exec call.

**Acceptance Criteria**:
- [ ] `AttachConnector.Connect` emits one `process: exec` INFO line with `target=tmux` and `args` = the joined tmux argv, immediately before `syscall.Exec`.
- [ ] The marker is emitted BEFORE the exec call (verified via the injected `execer` ordering — the captured log record exists before the fake execer's recorded invocation).
- [ ] `SwitchConnector.Connect` emits NO `process: exec` marker.
- [ ] The marker is under the `process` component (not a per-subsystem component).
- [ ] `args` is logged verbatim (the exact tmux argv string).
- [ ] The marker is visible regardless of `PORTAL_LOG_LEVEL` (level-filter bypass).

**Tests**:
- `"it emits process: exec target=tmux before syscall.Exec in AttachConnector"`
- `"it emits the marker before the exec call (ordering via injected execer)"`
- `"SwitchConnector emits no exec marker"`
- `"it logs args verbatim as the joined tmux argv"`
- `"the exec marker is visible at configured level WARN"`

**Edge Cases**:
- Exec marker emitted before `syscall.Exec` (in kernel pre-image-replace).
- `target=tmux` + `args=joined argv`.
- `SwitchConnector` path unaffected (gets normal exit via `Close`).
- Args logged verbatim.

**Context**:
> "**Every `syscall.Exec` call site MUST emit a plain `exec`-terminal INFO line immediately before the exec, under its owning component.** … `log.For("process").Info("exec", "target", "tmux", "args", strings.Join(argv, " ")); syscall.Exec(tmuxPath, argv, env)`. `AttachConnector` (bare-shell `portal open` → tmux) emits `process: exec target=tmux args="attach-session -A …"`. This is binary-level lifecycle, so `exec` joins `start` / `exit` / `panic` in the **`process` component's event space**." (spec § Defensive invariants → Mechanical rule — exec-handoff markers)
>
> "`SwitchConnector` (in-tmux path) is unaffected — it runs `tmux switch-client` as a subprocess and returns normally, so it gets a proper `process: exit` via `Close`. Only true `syscall.Exec` replace-process sites need the exec marker." … "Privacy on `args` attr: verbatim." (spec § Defensive invariants → Notes)
>
> Per the planning scope boundary, task 2-14 covers ONLY the `AttachConnector` bare-shell handoff; the hydrate helper's `hydrate: exec` marker is Phase 6.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants (Mechanical rule — exec-handoff markers, Notes); current `cmd/open.go` `AttachConnector.Connect` (the `syscall.Exec` site)

---

## portal-observability-layer-2-15 | approved

### Task 2-15: `portaltest.AssertLogLevelResolved` integration-test assertion helper

**Problem**: If `PORTAL_LOG_LEVEL` fails to propagate (tmux clears it on `respawn-pane`, or a harness forgets to pass it), DEBUG coverage degrades silently — a test passes with less output than expected unless it asserts on a positive marker. The `log-level resolved` line (Task 2-11) is that marker; integration tests need a canonical helper that scans `portal.log` for it, matched by `pid`, and asserts `resolved=<expected> source=env`.

**Solution**: Add `AssertLogLevelResolved(t *testing.T, logPath string, pid int, expected string)` to `internal/portaltest`. It reads the log at `logPath` (following the `portal.log` symlink to today's file), finds the `process: log-level resolved` line whose `pid` attr matches the given pid, and fails the test if the line is absent, `source` is not `env`, or `resolved` is not `expected`. Tolerant of baseline-attr ordering.

**Outcome**: An integration test that sets `PORTAL_LOG_LEVEL` can call `portaltest.AssertLogLevelResolved(t, logPath, daemonPID, "debug")` and have it pass only when that pid's `log-level resolved` line shows `resolved=debug source=env`; it fails clearly when the line is absent (env did not propagate) or `source != env`.

**Do**:
- Add `internal/portaltest/log_level_resolved.go` with `func AssertLogLevelResolved(t *testing.T, logPath string, pid int, expected string)`. The `*testing.T`-first parameter follows the package convention and structurally marks it test-only (consistent with `IsolateStateForTest`). Mark it a test helper via `t.Helper()`.
- Read the log file at `logPath`. If `logPath` is the `portal.log` symlink, `os.ReadFile` follows it to today's file automatically — but a robust helper should handle both the symlink and a direct day-file path. Document that callers typically pass `state.PortalLog(stateDir)` (the symlink) and the read follows it to the current day's file. (Note `ReadPortalLogSafe` already reads `state.PortalLog(stateDir)`; this helper takes an explicit `logPath` for flexibility.)
- Parse the file line-by-line. Identify the target line by: contains the `process: log-level resolved` text-mode marker (component prefix `process:` + msg `log-level resolved`) AND its `pid=<pid>` attr matches the given pid. Multiple processes may have written to the same day-file (reboot-recovery day), so the `pid` match is load-bearing — scan ALL matching lines and select the one whose `pid` equals the argument.
- Tolerate baseline-attr ordering: parse `key=value` pairs from the line into a map rather than relying on positional order (the text-mode renderer appends `pid`/`version`/`process_role` last, but the helper must not be brittle to ordering changes). Extract `resolved`, `source`, `pid` from the parsed pairs; strip surrounding quotes from quoted values.
- Assertions (each a clear `t.Errorf`/`t.Fatalf` with the line context):
  - If no `log-level resolved` line with the matching `pid` exists → fail: "PORTAL_LOG_LEVEL did not propagate: no `process: log-level resolved` line for pid=<pid>".
  - If `source != "env"` → fail: the env var was not the resolution source (it was `default` or `fallback`, meaning the harness did not set it or set it invalidly).
  - If `resolved != expected` → fail: the resolved level differs from what the test expects.
- Do NOT depend on the log being in any particular handler mode — parse the text-mode line (the production tail/grep default). If JSON mode is ever wired, that is out of scope here; parse text-mode.
- Document the helper in the package doc / `doc.go` exceptions list (it takes `*testing.T`, so it is structurally test-only — it can stay in the `*testing.T`-first majority, not the discipline-only exceptions).
- Add a `_test.go` in `internal/portaltest` exercising the helper against fabricated `portal.log` content (the helper itself is pure file-parsing + assertions; test it by writing fixture lines and asserting it passes/fails via a `*testing.T` stub or by structuring the assertions to be table-testable). `[needs-info]`: testing an assertion helper that calls `t.Errorf` is awkward — either (a) extract the pure logic into an unexported `findLogLevelResolved(content string, pid int) (resolved, source string, found bool)` and unit-test THAT (preferred), with `AssertLogLevelResolved` a thin wrapper that calls it and asserts; or (b) use a recording `testing.T` shim. Prefer (a): make the parsing pure and table-testable, keep the assertion wrapper thin.

**Acceptance Criteria**:
- [ ] `AssertLogLevelResolved` selects the correct line by `pid` when multiple processes wrote to the same day-file.
- [ ] It fails when no `log-level resolved` line for the given pid exists (env did not propagate).
- [ ] It fails when the matched line's `source != env`.
- [ ] It fails when the matched line's `resolved != expected`.
- [ ] It passes when the matched line shows `resolved=<expected> source=env` for the given pid.
- [ ] It follows the `portal.log` symlink to today's file when `logPath` is the symlink.
- [ ] It tolerates baseline-attr ordering (parses `key=value` into a map, not positionally).

**Tests**:
- `"it matches the correct line by pid when multiple processes wrote"`
- `"it fails when no log-level resolved line exists for the pid"`
- `"it fails when source is not env"`
- `"it fails when resolved differs from expected"`
- `"it passes for resolved=expected source=env"`
- `"it parses the symlink-followed current log"`
- `"it tolerates baseline-attr ordering"`

**Edge Cases**:
- Matches the correct line by `pid` attr when multiple processes wrote.
- Fails on absent line (env did not propagate).
- Fails when `source != env`.
- Parses the symlink-followed current log.
- Tolerates baseline-attr ordering.

**Context**:
> "A canonical assertion helper lives in `internal/portaltest`: `func AssertLogLevelResolved(t *testing.T, logPath string, pid int, expected string)` … scans portal.log for the process: log-level resolved line matching the given pid and asserts the resolved level matches expected with source='env'. Used by integration tests that set PORTAL_LOG_LEVEL." (spec § Log-level propagation verification → Test assertion contract)
>
> "Any integration test that sets `PORTAL_LOG_LEVEL` MUST scan `portal.log` for the `process: log-level resolved resolved=<expected> source=env` line for the spawned process (matched by `pid` attr if multiple processes were involved). If the line is absent or `source` is not `env`, the test fails — the env var did not propagate." (spec § Log-level propagation verification → Test assertion contract)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log-level propagation verification (Test assertion contract); `internal/portaltest/portal_log.go` + `doc.go` (placement, `*testing.T`-first convention)
