---
phase: 2
phase_name: Save daemon, triggers, and on-disk state format
total: 12
---

## built-in-session-resurrection-2-1 | approved

### Task 2-1: Add state-directory path helpers and paneKey sanitizer

**Problem**: Every subsequent Phase 2 task (and Phase 3's hydrate helper, Phase 6's status / cleanup commands) reads and writes files beneath `~/.config/portal/state/`. The spec fixes a specific directory layout (`sessions.json`, `save.requested`, `daemon.pid`, `daemon.version`, `portal.log`, `hydrate-<paneKey>.fifo`, `scrollback/<paneKey>.bin`) and a specific path-resolution order (per-file env var → `XDG_CONFIG_HOME/portal/state/` → `~/.config/portal/state/`). It also pins down the `paneKey` sanitizer used identically in scrollback filenames, FIFO filenames, and `@portal-skeleton-<paneKey>` marker names — one canonical function, applied everywhere, so the daemon's write path and the hydrate helper's read path agree byte-for-byte. Getting these two primitives wrong produces subtle bugs (wrong file paths on a user's XDG setup, marker-name mismatches that silently stop the daemon's skip-skeleton logic from working). They belong in one place, landed before anything that depends on them.

**Solution**: Add a small set of path helpers layered on top of Portal's existing `configFilePath` mechanism that resolve the state directory and every file within it, create the directory with mode `0700` on demand, and a `SanitizePaneKey(session string, window, pane int) string` that implements the spec's sanitizer (replace `/`, null bytes, leading `.`, and other filesystem-unsafe characters; hash-suffix on collision). All files under the state directory use the same single-source-of-truth resolver so the `PORTAL_STATE_DIR` env var / `XDG_CONFIG_HOME` override applies uniformly.

**Outcome**: Callers throughout later Phase 2 and Phase 3 tasks ask for `state.Dir()`, `state.SessionsJSON()`, `state.SaveRequested()`, `state.DaemonPID()`, `state.DaemonVersion()`, `state.PortalLog()`, `state.ScrollbackFile(paneKey)`, `state.FIFOPath(paneKey)`. A call to `state.EnsureDir()` creates the state tree (`state/` and `state/scrollback/`) with mode `0700`. `state.SanitizePaneKey("my/session", 0, 1)` returns a deterministic filesystem-safe string that matches what was written at save time regardless of which subsystem computes it.

**Do**:
- Create `internal/state/paths.go` with:
  - Private constants for filenames: `sessionsJSONName = "sessions.json"`, `saveRequestedName = "save.requested"`, `daemonPIDName = "daemon.pid"`, `daemonVersionName = "daemon.version"`, `portalLogName = "portal.log"`, `portalLogOldName = "portal.log.old"`, `scrollbackSubdir = "scrollback"`.
  - `Dir() (string, error)` — resolves the state directory path. Honours `PORTAL_STATE_DIR` first, then `XDG_CONFIG_HOME/portal/state`, then `~/.config/portal/state`. Because `internal/state` cannot import `cmd`, implement the resolution directly in `internal/state/paths.go` mirroring `configFilePath`'s logic (env var → XDG → home). Do not trigger any macOS migration — state is Phase 2 new, there is nothing to migrate.
  - `EnsureDir() (string, error)` — returns the resolved path after `os.MkdirAll(dir, 0o700)` and `os.MkdirAll(filepath.Join(dir, scrollbackSubdir), 0o700)`. Treats "already exists with different mode" as non-fatal — do not chmod existing directories.
  - Thin accessors that each return `filepath.Join(dir, filename)` given a resolved `dir`: `SessionsJSON(dir)`, `SaveRequested(dir)`, `DaemonPID(dir)`, `DaemonVersion(dir)`, `PortalLog(dir)`, `PortalLogOld(dir)`, `ScrollbackDir(dir)`, `ScrollbackFile(dir, paneKey)` (returns `scrollback/<paneKey>.bin` joined with dir), `FIFOPath(dir, paneKey)` (returns `hydrate-<paneKey>.fifo`).
- Create `internal/state/panekey.go` with:
  - `func SanitizePaneKey(session string, window, pane int) string`:
    1. Start with the session name.
    2. Replace every byte in the disallow set (`/`, `\x00`, and any other `os.IsPathSeparator` byte on the platform) with `_`.
    3. If the first rune is `.`, replace it with `_` (prevents hidden-file leading-dot collisions).
    4. If the original un-sanitized name differs from the result, append a short hash suffix of the *original* name so two different session names that sanitize identically produce distinct keys. Use `fmt.Sprintf("%s-%08x", sanitized, xxhash.Sum64String(session)&0xffffffff)` — truncate the hex digest to 8 characters, placed *between* the sanitized session name and the `__<window>.<pane>` suffix, i.e., `sanitized + "-" + hash8 + "__" + window + "." + pane`. Only append the hash when sanitization changed the string; if the session name was already filesystem-safe, return `sanitized + "__" + window + "." + pane` with no hash.
    5. Append `"__" + strconv.Itoa(window) + "." + strconv.Itoa(pane)`.
  - The sanitizer is the *only* paneKey builder used by the daemon, the restore flow, and the hydrate helper. All three agree by construction.
- Add `xxhash` as a dependency (`github.com/cespare/xxhash/v2` — fast, stdlib-style, no cgo). Same hash is reused in task 2-9 for scrollback content dedup; land the dependency here so the scrollback task has no setup work.
- Tests in `internal/state/paths_test.go` and `internal/state/panekey_test.go` using `t.Setenv` and `t.TempDir` for the env-var / XDG overrides:
  - Path resolver respects `PORTAL_STATE_DIR` when set.
  - Path resolver falls back to `XDG_CONFIG_HOME/portal/state` when `PORTAL_STATE_DIR` is unset.
  - Path resolver falls back to `~/.config/portal/state` when neither is set.
  - `EnsureDir` creates `state/` and `state/scrollback/` both with mode `0700`.
  - `EnsureDir` on an existing directory is a no-op (does not change mode, does not error).
  - `SanitizePaneKey("work", 0, 0)` → `"work__0.0"` (no hash — already safe).
  - `SanitizePaneKey("a/b", 0, 0)` → `"a_b-<hash>__0.0"` (slash replaced + hash appended because sanitization changed the string).
  - `SanitizePaneKey(".hidden", 0, 0)` → `"_hidden-<hash>__0.0"` (leading dot replaced + hash appended).
  - `SanitizePaneKey("a\x00b", 0, 1)` → null byte replaced with `_`, hash appended.
  - Two different sanitized-to-same session names produce distinct outputs via the hash suffix — call the function with `"a/b"` and `"a_b"` and assert the results differ.
  - Window and pane indices are appended as decimal integers — `SanitizePaneKey("work", 10, 5)` → `"work__10.5"`.
- Do NOT read or write `sessions.json` in this task — schema types land in 2-3. This task is paths + sanitizer only.

**Acceptance Criteria**:
- [ ] `state.Dir()` returns `$PORTAL_STATE_DIR` when set, `$XDG_CONFIG_HOME/portal/state` when set, else `~/.config/portal/state`.
- [ ] `state.EnsureDir()` creates `state/` and `state/scrollback/` with mode `0700` when they do not exist.
- [ ] `state.EnsureDir()` is a no-op on an existing state directory and does not alter its mode.
- [ ] Accessors (`SessionsJSON`, `SaveRequested`, `DaemonPID`, `DaemonVersion`, `PortalLog`, `PortalLogOld`, `ScrollbackFile`, `FIFOPath`) return the documented absolute paths given a resolved state dir.
- [ ] `SanitizePaneKey` replaces `/`, null bytes, and a leading `.` with `_`, appends an 8-character hex hash suffix of the *original* session name whenever sanitization changes the string, and always appends `"__<window>.<pane>"` as a decimal suffix.
- [ ] Two differently-spelled session names that sanitize to the same value produce distinct paneKeys via the hash suffix.
- [ ] Session names that are already filesystem-safe produce no hash suffix — `"work"` round-trips as `"work__0.0"` without extra characters.
- [ ] Sanitization is deterministic: the same inputs always produce the same output within a process and across processes (no randomness).

**Tests**:
- `"it resolves the state directory to PORTAL_STATE_DIR when set"`
- `"it falls back to XDG_CONFIG_HOME/portal/state when PORTAL_STATE_DIR is unset"`
- `"it falls back to ~/.config/portal/state when neither env var is set"`
- `"it creates state/ and state/scrollback/ with mode 0700"`
- `"it is a no-op when the state directory already exists"`
- `"it returns the documented paths for sessions.json, save.requested, daemon.pid, daemon.version, portal.log"`
- `"it builds scrollback/<paneKey>.bin and hydrate-<paneKey>.fifo paths"`
- `"it leaves filesystem-safe session names unchanged"`
- `"it replaces forward slashes in session names"`
- `"it replaces a leading dot in session names"`
- `"it replaces null bytes in session names"`
- `"it appends an 8-char hash suffix only when sanitization changed the name"`
- `"it distinguishes two sessions that sanitize to the same stem via the hash"`
- `"it appends window and pane indices as decimal integers"`

**Edge Cases**:
- `/` in session name → replaced with `_`; hash appended so a future `a_b` session does not collide.
- Null byte (`\x00`) in session name → replaced with `_`; hash appended. (Session names with null bytes are pathological but tmux has no parser stopping them — defensive handling.)
- Leading `.` → replaced with `_`; hash appended. Prevents hidden-file creation on macOS/Linux.
- Collision hash — two different real session names `"a/b"` and `"a_b"` both sanitize to `"a_b"` for the stem; the hash suffix differentiates them.
- Directories created with `0700` — metadata privacy, per spec "to prevent other users on a multi-user system from listing filenames."
- XDG override — honoured via the standard Portal mechanism, matching `configFilePath` behaviour in `cmd/config.go`. No macOS-migration path for state (state is new in this feature).
- Per-file env var — the spec only names `PORTAL_STATE_DIR` as the per-file override (no per-file env vars for every artifact); implement `PORTAL_STATE_DIR` and let all accessors derive from it.

**Context**:
> Spec "Save Format & Schema → Storage Location": "Saved state lives at `~/.config/portal/state/`, resolved via Portal's existing `configFilePath` mechanism (per-file env var → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/`). Same location as other Portal config (`hooks.json`, `projects.json`, `aliases`) — no separate XDG state directory. All files written with mode `0600` (owner read/write only). New directories (`state/`, `state/scrollback/`) created with mode `0700` to prevent other users on a multi-user system from listing filenames — keeping scrollback content private even at the metadata level."
>
> Spec "Save Format & Schema → Canonical paneKey (sanitization reference)":
> ```
> paneKey = sanitize(session_name) + "__" + window_index + "." + pane_index
> ```
> "where `sanitize()` replaces `/`, null bytes, leading `.`, and other filesystem-unsafe characters, with hash-suffix fallback on collision. The same sanitization is applied everywhere `paneKey` is used."
>
> Spec "Save Format & Schema → Scrollback Files → Filename scheme": "`<session>__<window>.<pane>.bin` — `session` is the session name, passed through a filesystem-safe sanitizer (replace characters that conflict with filesystem conventions: `/`, null bytes, leading `.`, etc.). On collision (two sanitized session names map to the same file key), append a hash suffix."
>
> Spec "Content-Hash Dedup": "Hash the bytes (xxhash or equivalent fast non-cryptographic hash)". Landing xxhash here as a shared dependency avoids duplicate go-mod churn in task 2-9.
>
> Existing pattern: `cmd/config.go` — `configFilePath` / `xdgConfigBase`. This task mirrors the pattern inside `internal/state` because `internal/` cannot import `cmd/`. The env-var / XDG-first / home-fallback chain is the same.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save Format & Schema → Storage Location", "Save Format & Schema → Directory Layout", "Save Format & Schema → Canonical paneKey (sanitization reference)", "Content-Hash Dedup".

## built-in-session-resurrection-2-2 | approved

### Task 2-2: Implement `portal state notify` subcommand

**Problem**: `portal state notify` is the hot path of every save-trigger tmux hook — seven events fire it (`session-created`, `session-closed`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`). The spec pins this command to a deliberately minimal behaviour: touch `~/.config/portal/state/save.requested` (create or bump mtime) and exit. **No tmux calls. No state-file reads. No conditional logic.** Any additional work — including checking `@portal-restoring` or reading `sessions.json` — belongs in the daemon's tick loop, not here. Keeping `notify` dumb is load-bearing: tmux's `run-shell` is synchronous; every microsecond spent in `notify` blocks the tmux server during the event fire. The spec is explicit that restoration suppression is the daemon's problem, not `notify`'s. The Phase 1 stub from task 1-1 currently returns `nil` without side effects; this task replaces the stub with the real behaviour.

**Solution**: Replace the `RunE` body in `cmd/state_notify.go` with three lines: resolve the state directory via `state.EnsureDir()` (task 2-1), compute the `save.requested` path, and touch the file. Touching creates the file if absent (empty contents, mode `0600`) or bumps mtime if present. Use `os.OpenFile(path, O_WRONLY|O_CREATE|O_TRUNC, 0o600)` for create-or-truncate and then `os.Chtimes(path, now, now)` to defensively bump mtime on the existing-file path. Stray positional args are harmless — `cobra.NoArgs` from task 1-1 already rejects them at parse time.

**Outcome**: Running `portal state notify` creates or updates `save.requested` beneath the resolved state directory, exits 0 on success, and exits non-zero with the OS error only when filesystem access fails (permission denied, read-only disk). The command makes zero tmux calls, reads zero state files, and has no conditional behaviour branching on state. Test coverage verifies call-count zero on any tmux interface mock, file presence + mtime bump after invocation, and that the file is empty after `notify` runs.

**Do**:
- Edit `cmd/state_notify.go` (stub from task 1-1):
  - Set `RunE` body:
    1. `dir, err := state.EnsureDir()` — creates `state/` with mode `0700` if absent. Return error on failure.
    2. `path := state.SaveRequested(dir)`.
    3. `f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)` — creates empty file or truncates existing to zero. Return error on failure.
    4. `f.Close()` — close before `os.Chtimes`.
    5. `now := time.Now(); _ = os.Chtimes(path, now, now)` — defensively bump mtime. Ignore `Chtimes` errors (best-effort; the file exists, the daemon can still observe it).
    6. Return `nil`.
- Keep `Args: cobra.NoArgs` from task 1-1 (already rejects stray args).
- Keep `Hidden: true` from task 1-1.
- Tests in `cmd/state_notify_test.go` using `t.TempDir` + `t.Setenv("PORTAL_STATE_DIR", dir)`:
  - First invocation: file did not exist → file exists post-invocation with zero bytes and mode `0600`.
  - Second invocation: file already exists with some non-zero mtime → new mtime is ≥ old mtime (allowing clock-granularity equality on systems where `time.Now()` resolves to the same tick; insert `time.Sleep(10 * time.Millisecond)` between invocations to guarantee strict-greater for the test).
  - State directory does not exist → created by `state.EnsureDir()` with mode `0700`; file created successfully.
  - Permission error (test with a directory set to `0500` read-only): `RunE` returns non-zero; Cobra propagates the error.
  - Stray args (e.g., `portal state notify ignored-arg`): Cobra's `NoArgs` returns validation error before `RunE` runs — test asserts the error message matches Cobra's standard form.
  - Confirm no tmux calls: inject a mock `tmux.Commander` that panics on any `Run` invocation and invoke `notify`; the panic must not fire.
- Do NOT invoke `PersistentPreRunE` / bootstrap from this command (already exempt via `state` in `skipTmuxCheck` from task 1-3). Explicit test: inject a panicking `bootstrapDeps` and confirm `notify` runs without triggering the panic.
- Do NOT read `@portal-restoring` or any tmux option. Do NOT open `sessions.json`. Do NOT log. The command is a single syscall pair on the success path.

**Acceptance Criteria**:
- [ ] `portal state notify` creates `save.requested` with mode `0600` if absent.
- [ ] `portal state notify` bumps mtime on `save.requested` if present.
- [ ] `portal state notify` creates the state directory (with mode `0700`) if missing.
- [ ] `portal state notify` makes zero tmux calls (verified via a panicking Commander mock).
- [ ] `portal state notify` makes zero state-file reads.
- [ ] `portal state notify` exits 0 on success.
- [ ] `portal state notify` exits non-zero on filesystem permission error, propagating the underlying OS error.
- [ ] Stray positional args are rejected by Cobra's `NoArgs` validator.
- [ ] The command does not invoke bootstrap (exempt via `state` allowlist from task 1-3).
- [ ] The command has no conditional behaviour branching on `@portal-restoring` — the daemon is the sole suppression point.

**Tests**:
- `"it creates save.requested when absent"`
- `"it bumps mtime on save.requested when present"`
- `"it creates the state directory with mode 0700 when missing"`
- `"it writes save.requested with mode 0600"`
- `"it truncates existing save.requested content to zero bytes"`
- `"it exits 0 on success"`
- `"it exits non-zero when the state directory is not writable"`
- `"it makes zero tmux calls"`
- `"it reads no state files"`
- `"it rejects stray positional arguments via Cobra NoArgs"`
- `"it does not invoke PersistentPreRunE bootstrap"`
- `"it does not branch on @portal-restoring"`

**Edge Cases**:
- State directory missing: `state.EnsureDir()` (task 2-1) creates both `state/` and `state/scrollback/` with `0700` on first call. First-ever `notify` invocation handles this path.
- Existing file mtime bump: `O_TRUNC` on many filesystems advances mtime, but `Chtimes(now, now)` is a defensive belt-and-braces step so the daemon's staleness-detection never misses a notify because of filesystem quirks.
- Permission error: if `~/.config/portal/state/` was created with wrong permissions (e.g., user manually `chmod 000`'d it), the error propagates via the RunE return value. No retry, no recovery — user intervention required. Spec: "permission error surfaces via exit code only."
- Stray args: Cobra `NoArgs` rejects at argv parse time with a standard error message. No `RunE` branch needed.
- Concurrent `notify` invocations (seven hooks firing from a burst of structural events): `O_CREATE|O_TRUNC` on a zero-byte file is essentially idempotent; concurrent writers produce the same end state. No locking, no coordination.
- The daemon observing a stale `save.requested` from a prior crashed process is covered by the daemon's startup `os.Remove(saveRequested)` in task 2-7 — not this task's concern.

**Context**:
> Spec "Save-Side Architecture: Triggers & Serialization → Single-Writer Serialization via Dirty Flag → Mechanism → 3.": "`portal state notify` is a small binary: touch (create or bump mtime of) `~/.config/portal/state/save.requested`, exit. **No tmux calls. No state-file reads. No conditional logic.** The binary is deliberately dumb: it always sets the dirty flag, even during restoration. The daemon (not `notify`) is responsible for honouring `@portal-restoring` and suppressing captures. The file's **contents are irrelevant** — `notify` writes an empty file, and the daemon only checks for presence (not content)."
>
> Spec "Save-Side Architecture: Triggers & Serialization → Single-Writer Serialization via Dirty Flag → Mechanism → 3. (continued)": "Any behavioural augmentation (session-rename migration, diagnostic fan-out, etc.) lives in a **separate internal subcommand** invoked by a dedicated tmux hook — it does not accrue into `notify`."
>
> Spec "CLI Surface → Internal Subcommands → `portal state notify`": "A small binary (~20 lines of Go) invoked by tmux save-trigger hooks. Responsibilities: Touch `~/.config/portal/state/save.requested` (create if absent, bump mtime otherwise). Exit 0. That is the entire behavior. No tmux calls, no state file reads, no logging beyond critical errors. Designed for minimum latency on the hot path of every structural event."
>
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-restoring`": "`portal state notify` itself is unaware of the marker — it always touches the dirty flag, including during restore; the daemon's entry-check is the single suppression point."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save-Side Architecture: Triggers & Serialization → Single-Writer Serialization via Dirty Flag", "CLI Surface → Internal Subcommands → `portal state notify`", "Restore-Side Architecture → Marker Coordination → `@portal-restoring`".

## built-in-session-resurrection-2-3 | approved

### Task 2-3: Define `sessions.json` v1 schema types and encoder/decoder

**Problem**: `sessions.json` is the structural index — the single atomic-commit artifact that every downstream consumer reads (Phase 3 restore flow, Phase 6 `portal state status`, future diagnostic tooling). The spec fixes the v1 schema exactly: `version`, `saved_at`, and `sessions[]` with nested `windows[]` and `panes[]`, each carrying specific field names and types. Getting the schema wrong — a missing field, a type mismatch, serializing an empty slice as `null` instead of `[]` — produces corrupt state files, silent restore failures, or brittle unmarshalling. Task 2-10 writes `sessions.json` via `AtomicWrite`, but the serialization itself must already be correct and stable. Future schema changes (v1 → v2) are explicitly deferred per spec; v1's shape is frozen, which makes "write exactly this" the correct primitive to isolate in its own task.

**Solution**: Add `internal/state/schema.go` with Go struct types matching the spec exactly (`Index`, `Session`, `Window`, `Pane`), a `SchemaVersion = 1` constant, `SavedAt time.Time` serialized as RFC 3339 UTC, and encoder/decoder helpers that use `json.MarshalIndent` (2-space indent for grep-friendliness) and `json.Unmarshal`. Unknown fields on decode are silently ignored (no `DisallowUnknownFields`) so a v2 writer rolling back to a v1 reader does not crash. Empty slices serialize as `[]` (not `null`) by initialising them with `make(...)` on the write path — achieved by writing a helper `Index.Canonicalize()` that ensures nil slices become zero-length non-nil before marshalling. Missing optional fields on decode round-trip as zero values.

**Outcome**: `internal/state` exports `Index`, `Session`, `Window`, `Pane` structs and `EncodeIndex(idx Index) ([]byte, error)` / `DecodeIndex(data []byte) (Index, error)` helpers. Round-tripping any `Index` through encode → decode produces an equal `Index`. Empty `sessions` slice, empty `environment` map, empty `panes` slice all serialize as `[]` / `{}` (not `null`). Unknown fields on decode are silently ignored. Task 2-10 calls `EncodeIndex` before `AtomicWrite`; Phase 3's Restore calls `DecodeIndex` after reading the file.

**Do**:
- Create `internal/state/schema.go`:
  ```go
  package state

  import (
      "encoding/json"
      "time"
  )

  const SchemaVersion = 1

  type Index struct {
      Version  int       `json:"version"`
      SavedAt  time.Time `json:"saved_at"`
      Sessions []Session `json:"sessions"`
  }

  type Session struct {
      Name        string            `json:"name"`
      Environment map[string]string `json:"environment"`
      Windows     []Window          `json:"windows"`
  }

  type Window struct {
      Index  int    `json:"index"`
      Name   string `json:"name"`
      Layout string `json:"layout"`
      Zoomed bool   `json:"zoomed"`
      Active bool   `json:"active"`
      Panes  []Pane `json:"panes"`
  }

  type Pane struct {
      Index          int    `json:"index"`
      CWD            string `json:"cwd"`
      Active         bool   `json:"active"`
      CurrentCommand string `json:"current_command"`
      ScrollbackFile string `json:"scrollback_file"`
  }
  ```
- Add `func (idx *Index) Canonicalize()`:
  - If `idx.Sessions == nil`, assign `idx.Sessions = []Session{}`.
  - For each `Session`: if `Windows == nil`, assign `[]Window{}`; if `Environment == nil`, assign `map[string]string{}`.
  - For each `Window`: if `Panes == nil`, assign `[]Pane{}`.
  - Always sets `idx.Version = SchemaVersion`.
- Add `func EncodeIndex(idx Index) ([]byte, error)`:
  1. Call `idx.Canonicalize()` on a mutable local copy (receiver semantics — take the value, copy, mutate).
  2. `return json.MarshalIndent(idx, "", "  ")`.
- Add `func DecodeIndex(data []byte) (Index, error)`:
  1. `var idx Index`.
  2. `err := json.Unmarshal(data, &idx)` — `Unmarshal` ignores unknown fields by default; do NOT use a decoder with `DisallowUnknownFields`.
  3. On success, if `idx.Version == 0`, return a wrapped error `errors.New("sessions.json missing version field")`. If `idx.Version != SchemaVersion`, return a wrapped error `fmt.Errorf("unsupported sessions.json version %d (expected %d)", idx.Version, SchemaVersion)`.
  4. Otherwise return `(idx, nil)`.
- `saved_at` serialization: Go's default `time.Time` JSON marshalling produces RFC 3339; this matches the spec's example (`"2026-04-17T10:30:00Z"`). Ensure the daemon writes UTC times (tasks 2-8 / 2-10 call `time.Now().UTC()` before setting `SavedAt`).
- Tests in `internal/state/schema_test.go`:
  - Round-trip: build an `Index` with two sessions, each with one window and two panes, one window zoomed, environment populated → marshal → unmarshal → `reflect.DeepEqual` on the result.
  - Empty sessions: `Index{Version: 1, SavedAt: time.Now().UTC()}` → marshal → JSON contains `"sessions": []` (not `"sessions": null`).
  - Empty environment: `Session{Name: "x", Environment: nil}` after `Canonicalize` → marshal → JSON contains `"environment": {}` (not `null`).
  - Empty panes: `Window{Panes: nil}` after `Canonicalize` → `"panes": []`.
  - Unknown fields on decode: prepare a JSON payload with an extra `"experimental_field": 42` at the top level and inside a session → `DecodeIndex` succeeds and `Index` contains the documented fields only (the unknown field is silently dropped).
  - Missing optional fields on decode: JSON without `zoomed` on a window → `Window.Zoomed == false`; without `environment` on a session → `Session.Environment == nil` (caller normalises via `Canonicalize` if needed).
  - Version 0 on decode → error.
  - Version 99 on decode → error naming the unsupported version and the expected version.
  - `saved_at` round-trip: use a known time with non-zero nanoseconds → marshalled output is RFC 3339 UTC → decoded time equals the input via `time.Equal` (not `==`, to tolerate timezone object differences).
  - Verify exact spec-matching JSON output: construct a minimal Index and assert the marshalled output contains the key order `version`, `saved_at`, `sessions` at the top level (Go's `encoding/json` preserves struct field order, which matches the spec example).
- Do NOT add schema migration logic — v1 is the only version; future migrations are explicit non-scope per spec.

**Acceptance Criteria**:
- [ ] `Index`, `Session`, `Window`, `Pane` structs exist with exact field names and types from the spec.
- [ ] `SchemaVersion` constant equals `1`.
- [ ] `EncodeIndex` produces 2-space-indented JSON with `version` = 1 always set and `saved_at` as RFC 3339 UTC.
- [ ] `Canonicalize` converts nil slices and nil maps to zero-length non-nil so marshalled JSON has `[]` and `{}`, never `null`.
- [ ] `DecodeIndex` silently ignores unknown fields.
- [ ] `DecodeIndex` returns an error for `version == 0` or `version != SchemaVersion`.
- [ ] Round-trip equality: encode then decode produces a deeply-equal `Index` for every documented field.
- [ ] Missing optional fields on decode round-trip as zero values (`Zoomed == false`, `Environment == nil`, `CurrentCommand == ""`).
- [ ] The encoder never writes `null` for any slice or map in the schema.

**Tests**:
- `"it round-trips a fully-populated Index"`
- `"it serialises an empty sessions slice as [] not null"`
- `"it serialises an empty environment map as {} not null"`
- `"it serialises an empty panes slice as [] not null"`
- `"it always sets version to 1 on encode"`
- `"it serialises saved_at as RFC 3339 UTC"`
- `"it silently ignores unknown fields on decode"`
- `"it decodes missing optional fields as zero values"`
- `"it returns an error when version is 0"`
- `"it returns an error when version is unsupported"`
- `"it preserves the spec-documented JSON field order at the top level"`
- `"it preserves saved_at nanosecond precision across round-trip"`

**Edge Cases**:
- Unknown fields ignored on decode — forward-compat with future schema additions. A v2 writer's extra fields do not break a v1 reader.
- Missing optional fields — decoded as zero values. The consumer (Phase 3 Restore) uses these zero values meaningfully (e.g., `Environment == nil` → no environment to restore; `Zoomed == false` → do not apply zoom).
- Empty slices serialize as `[]` — important for human-readability (JSON `null` is ambiguous) and for any downstream JSON tooling that assumes `sessions` is always a list.
- Empty maps serialize as `{}` — same rationale.
- Version mismatch on decode — returned as an error rather than a silent zero-value fallback. Phase 3 surfaces this as the "corrupt sessions.json" soft-failure path.
- `saved_at` time zone — Go's default `time.Time` marshal uses the value's timezone. Callers (task 2-8, 2-10) must write `time.Now().UTC()`; this task does not enforce UTC-only because the decoder tolerates any offset the JSON carries (RFC 3339 is fully specified).
- `environment` with removed-form variables: spec says `environment -r` entries are not captured; this task does not need to model them. Empty values (key with empty string) round-trip as `"KEY": ""`.

**Context**:
> Spec "Save Format & Schema → Structural Index: `sessions.json`": "Single JSON file at the root of the state directory. Contains the complete structural topology plus references to scrollback files. Schema (version 1):"
> ```json
> {
>   "version": 1,
>   "saved_at": "2026-04-17T10:30:00Z",
>   "sessions": [
>     {
>       "name": "work",
>       "environment": { "LANG": "en_US.UTF-8", "TERM": "xterm-256color" },
>       "windows": [ ... ]
>     }
>   ]
> }
> ```
>
> Spec "Save Format & Schema → Structural Index → Field semantics": names every field and its source: `version` integer starting at 1, `saved_at` RFC 3339 UTC, `sessions[].name`, `sessions[].environment`, `sessions[].windows[].{index,name,layout,zoomed,active}`, `sessions[].windows[].panes[].{index,cwd,active,current_command,scrollback_file}`.
>
> Spec "Save Format & Schema → Structural Index → Omitted fields": "`options` (generic per-session/per-window/per-pane option capture): dropped per Scope. `marks`: dropped per Scope. `last_pane`: dropped per Scope." Do not add these.
>
> Spec "Scope & Constraints → Deferred → Schema migration (v1 → v2)": "Standard practice when the time comes; not a v1 design decision." Therefore the only version the encoder ever writes is 1; the decoder errors on anything else.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — section "Save Format & Schema → Structural Index: `sessions.json`".

## built-in-session-resurrection-2-4 | approved

### Task 2-4: Add daemon.pid + signal(0) liveness check and version-marker read/write helpers

**Problem**: Two small on-disk artifacts — `daemon.pid` and `daemon.version` — together are the single source of truth for "is the hosted save daemon actually alive, and does it match the currently-invoking binary's version?" The spec explicitly rejects `#{pane_current_command}` as a liveness predicate (it returns a short process name that any `portal <subcommand>` would match), and the version-marker-driven restart flow (task 2-6) depends on reading the stored version. Bootstrap's `_portal-saver` idempotency (task 2-5) decides whether to `kill-session` + recreate based on these helpers. Packaging the read / write / signal-0 check as a cohesive primitive now keeps later tasks focused on orchestration instead of filesystem / syscall plumbing.

**Solution**: Add `internal/state/daemon_state.go` exposing `WritePIDFile(dir string, pid int) error`, `ReadPIDFile(dir string) (int, error)`, `IsProcessAlive(pid int) bool` (wraps `syscall.Kill(pid, 0)`), `WriteVersionFile(dir, version string) error`, `ReadVersionFile(dir string) (string, error)`, and a composite `DaemonAlive(dir string) bool` that reads the PID file and checks liveness in one call. PID files are one line containing the decimal PID; version files are one line containing the raw version string. Both use `fileutil.AtomicWrite` for writes (mode `0600`) and plain `os.ReadFile` + `strings.TrimSpace` for reads. Read errors (missing file, unparseable content) return a sentinel or a specific error so callers can distinguish "absent" from "I/O error."

**Outcome**: Task 2-5's bootstrap logic composes `state.DaemonAlive(dir)` with `state.ReadVersionFile(dir)` + comparison against `cmd.version` to decide whether `_portal-saver` needs a kill + recreate. Task 2-7's daemon startup calls `state.WritePIDFile(dir, os.Getpid())` and `state.WriteVersionFile(dir, cmd.version)` atomically. Task 2-6's version-marker restart flow reads `state.ReadVersionFile` and compares. `signal(0)` via `syscall.Kill(pid, 0)` returns `nil` for alive, `ESRCH` for dead, `EPERM` for "alive but not ours" — the helper treats `nil` and `EPERM` as alive (the process exists) and `ESRCH` as dead, matching POSIX semantics.

**Do**:
- Create `internal/state/daemon_state.go`:
  - `var ErrPIDFileAbsent = errors.New("daemon.pid absent")`.
  - `var ErrVersionFileAbsent = errors.New("daemon.version absent")`.
  - `func WritePIDFile(dir string, pid int) error`:
    - Path via `filepath.Join(dir, daemonPIDName)`.
    - Content: `strconv.Itoa(pid) + "\n"` (trailing newline is cosmetic; matches shell-friendly convention).
    - Write via `fileutil.AtomicWrite(path, []byte(content))`. Atomic write already ensures parent dir exists.
  - `func ReadPIDFile(dir string) (int, error)`:
    - `data, err := os.ReadFile(path)`.
    - On `os.IsNotExist`: return `0, ErrPIDFileAbsent`.
    - On other read errors: wrap and return.
    - `pid, err := strconv.Atoi(strings.TrimSpace(string(data)))`. On parse error: return `0, fmt.Errorf("daemon.pid unparseable: %w", err)`.
    - Return `pid, nil`.
  - `func IsProcessAlive(pid int) bool`:
    - If `pid <= 0`: return `false`.
    - `err := syscall.Kill(pid, 0)`.
    - `nil` → alive. `errors.Is(err, syscall.EPERM)` → alive (exists but we cannot signal it). `errors.Is(err, syscall.ESRCH)` → dead. Any other error → `false` (treat as dead for safety — we would rather over-kill-then-recreate than leave a zombie).
  - `func DaemonAlive(dir string) bool`:
    - `pid, err := ReadPIDFile(dir)`.
    - On any error (absent, unparseable): return `false`.
    - Return `IsProcessAlive(pid)`.
  - `func WriteVersionFile(dir, version string) error`:
    - Path via `filepath.Join(dir, daemonVersionName)`.
    - Content: `version + "\n"` (trailing newline). Empty version writes an empty-plus-newline file — the string `"\n"` — so it is distinguishable from "file absent" at read time.
    - Write via `fileutil.AtomicWrite`.
  - `func ReadVersionFile(dir string) (string, error)`:
    - `data, err := os.ReadFile(path)`.
    - On `os.IsNotExist`: return `"", ErrVersionFileAbsent`.
    - On other read errors: wrap and return.
    - Return `strings.TrimSpace(string(data)), nil` — strips the trailing newline. Empty contents return `""` with `nil` error (distinct from `ErrVersionFileAbsent`).
- Tests in `internal/state/daemon_state_test.go`:
  - `WritePIDFile(dir, 12345)` then `ReadPIDFile(dir)` → returns `12345, nil`.
  - `ReadPIDFile` on missing file → `0, ErrPIDFileAbsent`.
  - `ReadPIDFile` on file containing `"not-a-number"` → error (wrapped).
  - `ReadPIDFile` on file containing `"  12345  \n"` (leading/trailing whitespace) → `12345, nil`.
  - `IsProcessAlive(os.Getpid())` → `true` (the test process is obviously alive).
  - `IsProcessAlive(-1)` → `false` (invalid PID).
  - `IsProcessAlive(99999)` — a PID unlikely to be in use; handle as probabilistic: spawn a `sleep 30` subprocess, record its PID, kill it, `syscall.Kill(pid, 0)` returns `ESRCH` → `false`. (Use `exec.Command("sleep", "30").Start()` and `cmd.Process.Kill()` for determinism.)
  - `DaemonAlive` composite: write a PID file with the current process's PID → `true`; write one with a definitely-dead PID → `false`; no PID file → `false`.
  - `WriteVersionFile(dir, "v0.4.2")` then `ReadVersionFile(dir)` → `"v0.4.2", nil`.
  - `ReadVersionFile` on missing file → `"", ErrVersionFileAbsent`.
  - `WriteVersionFile(dir, "")` then `ReadVersionFile(dir)` → `"", nil` (distinguishes "empty version" from "file absent" via the error).
  - `WriteVersionFile(dir, "dev")` then `ReadVersionFile(dir)` → `"dev", nil` (dev version round-trips verbatim).
  - Both PID and version files are written with mode `0600` — stat the file, assert `Mode().Perm() == 0o600`.
- Do NOT implement version comparison logic here — that belongs in task 2-6's bootstrap composition. This task is pure read/write + liveness.

**Acceptance Criteria**:
- [ ] `WritePIDFile` writes the PID atomically (temp + rename) with mode `0600`.
- [ ] `ReadPIDFile` returns the integer PID for a well-formed file; `ErrPIDFileAbsent` for a missing file; wrapped error for an unparseable file.
- [ ] `ReadPIDFile` tolerates leading/trailing whitespace (trims before parsing).
- [ ] `IsProcessAlive` returns true for the current process's PID.
- [ ] `IsProcessAlive` returns false for a definitely-dead PID (e.g., a child process that has been `Wait()`-ed).
- [ ] `IsProcessAlive` returns false for `pid <= 0`.
- [ ] `DaemonAlive` returns true only when the PID file exists, parses cleanly, AND the process is alive.
- [ ] `WriteVersionFile` writes the version atomically with mode `0600`.
- [ ] `ReadVersionFile` returns the version string stripped of trailing newlines; distinguishes "file absent" (via `ErrVersionFileAbsent`) from "file present but empty" (via empty string + nil error).
- [ ] Empty version and `"dev"` round-trip verbatim through the read/write pair.
- [ ] PID reused by a different process is accepted as alive — this task does not attempt to verify the process identity beyond `signal(0)`; task 2-5 documents that limitation as an accepted trade-off.

**Tests**:
- `"it writes and reads a PID file"`
- `"it returns ErrPIDFileAbsent when the PID file is missing"`
- `"it returns an error when the PID file is unparseable"`
- `"it trims whitespace when reading the PID file"`
- `"it reports the current process as alive via IsProcessAlive"`
- `"it reports a freshly-reaped child process as dead via IsProcessAlive"`
- `"it reports an invalid PID (0 or negative) as dead"`
- `"it returns true from DaemonAlive only when both PID file and process exist"`
- `"it returns false from DaemonAlive when PID file is absent"`
- `"it returns false from DaemonAlive when the PID file points to a dead process"`
- `"it writes and reads a version file"`
- `"it returns ErrVersionFileAbsent when the version file is missing"`
- `"it distinguishes empty-contents from absent version file"`
- `"it round-trips the literal dev version marker"`
- `"it writes PID and version files with mode 0600"`

**Edge Cases**:
- PID file missing — `ErrPIDFileAbsent`. Bootstrap (task 2-5) treats this as "daemon is not running" and proceeds to create.
- PID file unparseable — wrapped error. Bootstrap treats this the same as absent (recreate the daemon) because the liveness predicate cannot be trusted.
- PID file has whitespace — trimmed before `Atoi`. Covers user-editing via `echo 12345 > daemon.pid`.
- PID reused by a different process — `syscall.Kill(pid, 0)` returns `nil` (the PID exists), so `DaemonAlive` returns `true` and bootstrap keeps the existing session. This is an accepted race per spec: "PID reused by a different process (accepted)."
- Version file missing — `ErrVersionFileAbsent`. Task 2-6 treats this as a mismatch and restarts the daemon.
- Version file empty — returned as `("", nil)`. Task 2-6 treats empty version the same as "dev" and restarts.
- `"dev"` version — returned as `"dev"`. Task 2-6 always restarts on `"dev"`.
- `syscall.Kill(pid, 0)` returning `EPERM` — the process exists but belongs to a different user (shouldn't happen on a single-user machine but possible on shared systems). Treated as alive to avoid unnecessary recreate attempts that would fail anyway.
- Write atomicity — `fileutil.AtomicWrite` temp-file + rename ensures concurrent readers never see partial contents. The daemon writes on startup; bootstrap reads on every invocation. No locking required.

**Context**:
> Spec "Save-Side Architecture → Lifecycle Summary": "**Liveness verification.** `has-session` returning true is not sufficient proof the daemon is running — the session could have been left behind with a dead process inside. The daemon writes its OS PID to `~/.config/portal/state/daemon.pid` on startup (alongside `daemon.version`). Bootstrap reads `daemon.pid` and tests the process via `kill(pid, 0)` (Go: `syscall.Kill`; signal 0 tests existence without signalling). If the PID file is missing, unparseable, or the process check fails, Portal treats the daemon as absent: `kill-session -t _portal-saver` (tolerant of already-dead) then recreate. `#{pane_current_command}` is not used as a liveness predicate — it returns only a short process name, which is too imprecise (any `portal <subcommand>` invocation would match). The PID-file + signal-0 check is definitive."
>
> Spec "tmux Hook Registration Lifecycle → Scenario 3: Portal upgrade with running server → Version-marker-based restart": "On daemon startup, `portal state daemon` writes its version (`cmd.version`) to `~/.config/portal/state/daemon.version`. On every `EnsureServer()` call, Portal reads `daemon.version` and compares to the currently-invoking binary's `cmd.version`."
>
> Spec "tmux Hook Registration Lifecycle → Scenario 3 → Dev-build handling": "If `cmd.version` is empty or literally `"dev"`, treat every bootstrap as a mismatch and restart the daemon." The comparison lives in task 2-6; this task just guarantees the stored value round-trips faithfully.
>
> Existing pattern: `internal/fileutil/atomic.go` `AtomicWrite`. Both writes in this task route through it.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save-Side Architecture → Lifecycle Summary", "tmux Hook Registration Lifecycle → Scenario 3: Portal upgrade with running server".

## built-in-session-resurrection-2-5 | approved

### Task 2-5: Implement idempotent `_portal-saver` bootstrap with defensive `destroy-unattached off -t`

**Problem**: Bootstrap needs to ensure `_portal-saver` exists as a detached tmux session hosting `portal state daemon`, and it must do so idempotently — re-running bootstrap a second time in the same server lifetime produces zero new sessions. The spec pins a specific creation flow: (1) `has-session -t _portal-saver` → if absent, `new-session -d -s _portal-saver "portal state daemon"`; (2) `has-session` returning true is not sufficient — PID + signal(0) liveness check (task 2-4) determines whether to `kill-session` + recreate; (3) `set-option -t _portal-saver destroy-unattached off` runs *unconditionally* on every bootstrap as defence against users with `destroy-unattached on` globally in `.tmux.conf`. The `-t <session>` scoping is load-bearing — `-g` would stomp on the user's global setting, explicitly out of scope. Version-marker restart (task 2-6) layers on top of this primitive; this task is pure idempotent presence + defensive option.

**Solution**: Add `BootstrapPortalSaver(c *tmux.Client, dir string) error` in a new file `internal/tmux/portal_saver.go`. The function implements the spec's flow: `has-session` + PID/liveness check → kill + recreate if the session exists but the daemon is dead → otherwise leave running. After the presence logic, it *always* runs `set-option -t _portal-saver destroy-unattached off`. Retry transient new-session failures with a small bounded retry (up to 3 attempts, 100ms backoff) — if the daemon session keeps failing to spawn, return the error so bootstrap can surface the "failed to start after retries" stderr warning (Phase 6). Concurrent bootstraps from two Portal processes are handled naturally: tmux's `new-session` fails with "duplicate session" if another process created it first; the function retries has-session and succeeds on the second pass.

**Outcome**: `BootstrapPortalSaver` on a fresh tmux server creates `_portal-saver`, writes `destroy-unattached off` on it, and returns nil. On a server where `_portal-saver` already exists with a live daemon (task 2-4's `DaemonAlive` returns true), the function is a no-op except for the unconditional `destroy-unattached off`. On a server where the session exists but the daemon is dead (orphan session), the function kills it, recreates it, and sets the option. Transient `new-session` failures retry up to 3 times with 100ms backoff; persistent failure surfaces as an error. Zero calls to `-g` (global) `set-option` are ever made.

**Do**:
- Create `internal/tmux/portal_saver.go`:
  ```go
  package tmux

  const PortalSaverName = "_portal-saver"
  const portalSaverCommand = "portal state daemon"
  ```
- Extend `tmux.Client` or add free functions. Free functions are fine here since they compose existing client methods.
- `func BootstrapPortalSaver(c *Client, stateDir string) error`:
  1. `sessionPresent := c.HasSession(PortalSaverName)`.
  2. `daemonAlive := state.DaemonAlive(stateDir)` — imported from `internal/state` (task 2-4).
  3. If `sessionPresent && !daemonAlive`: call `c.KillSession(PortalSaverName)` (tolerant of already-dead). Log `warn` to `portal.log` via the logger scaffolding that lands in task 2-7 — for this task, just `fmt.Fprintf(os.Stderr, ...)` is acceptable because the log-file writer is not yet available. Actually — defer logging to task 2-7's scaffolding. For this task, do not log; task 2-7's daemon-startup logger will be the right seam. Return the kill error wrapped only if kill itself failed with a non-"session-absent" error (tmux's `kill-session` on an absent session returns a non-zero exit, but the error message contains "session not found" / similar — treat any kill error as non-fatal here since the goal is "ensure the session does not exist afterwards"; fall through to the creation step regardless).
  4. If `!sessionPresent || (sessionPresent && !daemonAlive)`: attempt creation via `c.NewSession(PortalSaverName, "", portalSaverCommand)`. Retry up to 3 times on error with 100ms backoff between attempts. If all 3 attempts fail, return `fmt.Errorf("_portal-saver creation failed after retries: %w", err)`.
  5. Unconditionally run `c.SetSessionOption(PortalSaverName, "destroy-unattached", "off")` — a new `tmux.Client` method to add (see next bullet). On error, wrap and return.
  6. Return `nil`.
- Add `func (c *Client) SetSessionOption(session, name, value string) error` on `tmux.Client`:
  - Calls `c.cmd.Run("set-option", "-t", session, name, value)`. Note: no `-s`, no `-g` — the default scope is session-local. The `-t` flag targets the specific session. Spec: "The `-t <session>` scoping is load-bearing: it targets the saver session only, as a session-local override. **Do not** use `-g` (global)."
  - Returns `fmt.Errorf("failed to set session option %q on %q: %w", name, session, err)` on failure.
- Add `NewSession` overload handling — existing `NewSession(name, dir, shellCommand)` requires a dir. Spec says `new-session -d -s _portal-saver "portal state daemon"` without `-c`. For `_portal-saver`, pass `dir=""` and modify `NewSession` to omit `-c` when dir is empty — OR add a thin `NewDetachedSession(name, shellCommand string)` helper that sends `new-session -d -s name command` without `-c`. Preferred: new helper so the existing `NewSession` signature stays unchanged and callers are explicit. Name it `NewDetachedSessionNoCwd`.
- Tests in `internal/tmux/portal_saver_test.go` using `MockCommander.RunFunc` to dispatch on argv patterns:
  - Fresh server (`has-session` returns error, `daemon.pid` absent): 1 `has-session` + 1 `new-session` + 1 `set-option -t _portal-saver destroy-unattached off`. Zero `kill-session` calls.
  - Session present, daemon alive (`DaemonAlive` → true): 1 `has-session` + 0 `new-session` + 1 `set-option`. Zero `kill-session` calls.
  - Session present, daemon dead (PID file exists, process dead): 1 `has-session` + 1 `kill-session` + 1 `new-session` + 1 `set-option`.
  - `new-session` fails twice then succeeds: total `new-session` calls = 3 (two failures + one success). Final return is nil.
  - `new-session` fails 3 times: function returns the wrapped error after 3 attempts. Total `new-session` calls = 3.
  - `set-option` fails: error propagated, wrapped with session and option name.
  - `kill-session` failure (non-session-absent error) does NOT abort — the function still attempts `new-session`. Verified by asserting `Calls` contains `new-session` after a failing `kill-session`.
  - Concurrent bootstrap simulation — not feasible as a unit test, but assert that when `has-session` returns true on the second `BootstrapPortalSaver` call (simulating another process won the race), the function observes it and does not attempt a redundant create.
  - `set-option` call uses `-t` (session scope), never `-g` — assert the argv.
- Do NOT register any tmux hooks in this task (Phase 1 already did). Do NOT touch `@portal-restoring` (that is task 2-12's bootstrap integration).
- Do NOT call the version-marker restart logic here (task 2-6 composes that on top of this primitive).

**Acceptance Criteria**:
- [ ] `BootstrapPortalSaver` creates `_portal-saver` via `new-session -d -s _portal-saver "portal state daemon"` when absent.
- [ ] `BootstrapPortalSaver` detects session-present-but-daemon-dead via `state.DaemonAlive(dir)` and recreates.
- [ ] `BootstrapPortalSaver` always calls `set-option -t _portal-saver destroy-unattached off` at the end of the successful path.
- [ ] The `set-option` call uses `-t` (session scope), never `-g` (global).
- [ ] Transient `new-session` failures retry up to 3 times with 100ms backoff before returning an error.
- [ ] Persistent `new-session` failure returns a wrapped error naming the retry exhaustion.
- [ ] `kill-session` failures in the "session present but daemon dead" branch do not abort the function — it falls through to the creation attempt anyway.
- [ ] Idempotent: running `BootstrapPortalSaver` twice in a row on a fresh server produces exactly one `new-session` call across both runs.
- [ ] Concurrent bootstrap race (simulated via a `HasSession` that returns true on retry) does not produce a duplicate `new-session` attempt.

**Tests**:
- `"it creates _portal-saver on a fresh server"`
- `"it is a no-op when _portal-saver exists and the daemon is alive"`
- `"it kills and recreates _portal-saver when the session exists but the daemon is dead"`
- `"it always calls set-option -t _portal-saver destroy-unattached off"`
- `"it uses -t session scope and never -g global scope for destroy-unattached"`
- `"it retries new-session up to 3 times on transient failure"`
- `"it returns a wrapped error after retry exhaustion"`
- `"it tolerates kill-session failure when transitioning from orphan to fresh"`
- `"it propagates set-option failure with the session and option name"`
- `"it does not create _portal-saver redundantly on a concurrent-bootstrap race"`

**Edge Cases**:
- `has-session` returns true but `DaemonAlive` returns false → orphan session with dead process → kill + recreate.
- `kill-session` on an already-dead session: tmux returns a non-zero exit; treat as non-fatal and proceed to create.
- `new-session` transient failure (e.g., fd exhaustion, transient tmux lock): retry up to 3 times with 100ms backoff. Empirical; spec does not pin the retry count but mentions "Portal retries a small number of times" for `_portal-saver` creation.
- Concurrent bootstrap from two Portal processes: the loser's `new-session` returns "duplicate session" error; retry logic re-checks `has-session` via the next BootstrapPortalSaver call and observes the winner's session as already-present. For robustness inside a single BootstrapPortalSaver call, on `new-session` error, also re-check `HasSession` and treat a now-present session as success.
- `destroy-unattached off` is idempotent — repeated applications are no-ops on tmux's side. Always apply.
- `set-option -t _portal-saver` targets the session; `-g` would overwrite the user's global setting. The spec is emphatic: do not use `-g`.
- The `_portal-saver` session name is reserved: `_*` session names are excluded from capture, TUI picker, etc., per Phase 1 and the Save-Side Execution Model.

**Context**:
> Spec "Save-Side Architecture → Defensive Session Setup": "On every `EnsureServer()` call, Portal runs `tmux set-option -t _portal-saver destroy-unattached off` unconditionally (idempotent). The `-t <session>` scoping is load-bearing: it targets the saver session only, as a session-local override. **Do not** use `-g` (global) — that would overwrite the user's global `destroy-unattached` setting, which is out of scope."
>
> Spec "Save-Side Architecture → Lifecycle Summary": "**Creation:** `EnsureServer()` calls `has-session -t _portal-saver`. If absent, `new-session -d -s _portal-saver \"portal state daemon\"`. **Liveness verification.** `has-session` returning true is not sufficient proof the daemon is running — the session could have been left behind with a dead process inside. ... If the PID file is missing, unparseable, or the process check fails, Portal treats the daemon as absent: `kill-session -t _portal-saver` (tolerant of already-dead) then recreate."
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 4. `_portal-saver` session setup": "`has-session -t _portal-saver` — if present, skip creation. If absent: `new-session -d -s _portal-saver \"portal state daemon\"`. ... Always run `set-option -t _portal-saver destroy-unattached off` (defensive, idempotent)."
>
> Spec "Failure Modes & Recovery → `_portal-saver` creation fails at bootstrap": "Portal retries a small number of times. On persistent failure: log, emit stderr warning (see Observability), continue bootstrap without the save daemon." 3 attempts × 100ms backoff is a reasonable interpretation of "small number."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save-Side Architecture → Defensive Session Setup", "Save-Side Architecture → Lifecycle Summary", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 4", "Failure Modes & Recovery → `_portal-saver` creation fails at bootstrap".

## built-in-session-resurrection-2-6 | approved

### Task 2-6: Wire version-marker-driven restart into bootstrap

**Problem**: When Portal is upgraded (`brew upgrade portal`, `go install`, binary replaced in-place), the new binary needs to ensure the hosted daemon is running the new code — not the old binary that was `exec`'d into `_portal-saver` when the previous Portal version bootstrapped. The spec's mechanism: on every bootstrap, read `daemon.version` and compare it to the currently-invoking binary's `cmd.version`. Mismatch (including absent file, empty string, literal `"dev"`) → `kill-session -t _portal-saver` and recreate so the new binary takes over. The SIGHUP-on-kill path flushes the current save via `AtomicWrite` before exit (task 2-7 wires the handler), so worst-case data loss is ≤1s of drift. Task 2-5 owns the "is the session present with an alive daemon" primitive; this task layers the version comparison on top and decides when to force a restart even if the existing daemon is perfectly alive.

**Solution**: Add `EnsurePortalSaverVersion(c *tmux.Client, dir, currentVersion string) error` in `internal/tmux/portal_saver.go`. The function reads `state.ReadVersionFile(dir)`; determines whether the stored version is stale (absent, empty, `"dev"`, or not equal to `currentVersion`); if stale, calls `c.KillSession(_portal-saver)` (tolerant of absent) to trigger the SIGHUP flush + tmux auto-destroy; then delegates to `BootstrapPortalSaver` (task 2-5) for the create-and-configure flow. If the version matches, it still delegates to `BootstrapPortalSaver` — which is already idempotent — so the defensive `destroy-unattached off` always runs. The currently-invoking binary's version comes from `cmd.version` (set via ldflags); the function takes it as an argument so tests can inject.

**Outcome**: `EnsurePortalSaverVersion(client, dir, cmd.version)` on a fresh install (no version file, no session) creates `_portal-saver` and lets the daemon write `daemon.version` on its own startup (task 2-7). On an upgrade (stored version `v0.4.1`, invoking `v0.4.2`), it kills `_portal-saver` then recreates; the new daemon overwrites the version file on startup. On a dev build (`currentVersion == ""` or `"dev"`), it always kills + recreates. On a matching release version, it is a no-op beyond the idempotent `destroy-unattached off`. A tmux `kill-session` on an already-dead session returns an error but is tolerated — treated the same as "already absent."

**Do**:
- In `internal/tmux/portal_saver.go` add:
  - `func EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error`:
    1. `stored, readErr := state.ReadVersionFile(stateDir)`. Any error (including `ErrVersionFileAbsent`) → treat as mismatch.
    2. Determine `mismatch`:
       - `readErr != nil` → `mismatch = true`.
       - `currentVersion == "" || currentVersion == "dev"` → `mismatch = true` (always restart on dev / empty builds).
       - `stored == ""` → `mismatch = true` (empty stored version — possible if daemon crashed before writing).
       - `stored == "dev"` → `mismatch = true` (previous dev build — always restart).
       - Otherwise: `mismatch = (stored != currentVersion)`.
    3. If `mismatch && c.HasSession(PortalSaverName)`:
       - Call `c.KillSession(PortalSaverName)`. Ignore errors — tmux returns an error for "session already gone" and we want to proceed anyway.
       - Do NOT write a new version file here — the new daemon writes its own version on startup (task 2-7).
    4. Return `BootstrapPortalSaver(c, stateDir)` — idempotent; creates if absent, no-op if present, always applies `destroy-unattached off`.
- Expose `PortalSaverName` as a package-exported constant (from task 2-5) so callers can reference it without duplicating the string.
- Tests in `internal/tmux/portal_saver_test.go` (extending task 2-5's test file) using `MockCommander` + `t.Setenv("PORTAL_STATE_DIR", dir)` to redirect the version-file location:
  - Fresh server, no version file, current version `v0.4.2`: mismatch=true but `has-session` returns false → no kill call; creation via `BootstrapPortalSaver` still happens. Zero `kill-session` calls.
  - Release version match (stored `v0.4.2`, current `v0.4.2`, session alive): mismatch=false → no kill call → idempotent `BootstrapPortalSaver` no-op except for `destroy-unattached off`.
  - Release version mismatch (stored `v0.4.1`, current `v0.4.2`, session alive): mismatch=true → one `kill-session` + one `new-session` + one `set-option`.
  - Dev build (current `""`): stored version does not matter → always kill + recreate. Test two sub-cases: empty stored and non-empty stored; both fire a kill.
  - Dev build (current `"dev"`): same — always restart.
  - Empty stored version (`""`, file exists but contents are empty): mismatch=true → kill + recreate even if current is a real release version.
  - Stored `"dev"`, current `v0.4.2`: mismatch=true (stored-side check).
  - Absent version file, `_portal-saver` session live, current release version: mismatch=true → kill + recreate. (Spec: "version file absent while `_portal-saver` live (accepted unnecessary restart)".)
  - `kill-session` on non-existent session (tmux error) is tolerated — function proceeds to `BootstrapPortalSaver`, which creates the session, returns nil.
  - Version-file read error other than absent (e.g., permission denied simulated via unreadable file) → treated as mismatch → restart.
- Do NOT write `daemon.version` from this task. The daemon writes it on startup (task 2-7). A small race exists where two bootstraps could both restart the daemon before the first instance has written the new version file — this is benign (each restart is ~50ms; eventually the file settles).

**Acceptance Criteria**:
- [ ] `EnsurePortalSaverVersion` reads the stored version via `state.ReadVersionFile`.
- [ ] Mismatch is detected for: version file absent, stored version empty, stored `"dev"`, current `""` or `"dev"`, or any non-equal string pair.
- [ ] On mismatch with live `_portal-saver`: `kill-session -t _portal-saver` is called exactly once.
- [ ] On match: no `kill-session` call is made.
- [ ] After the (conditional) kill, `BootstrapPortalSaver` is always invoked (idempotent recreate + defensive option).
- [ ] `kill-session` error for "session already absent" does not abort the function.
- [ ] Dev build (`""` or `"dev"` as current version) always triggers a restart regardless of stored version.
- [ ] The function does not write `daemon.version` — the daemon owns that.

**Tests**:
- `"it does not kill when stored version matches current version"`
- `"it kills and recreates when stored version differs from current"`
- `"it always restarts when current version is empty string"`
- `"it always restarts when current version is literal dev"`
- `"it treats stored dev as a mismatch and restarts"`
- `"it treats empty stored version as a mismatch and restarts"`
- `"it treats absent version file as a mismatch"`
- `"it skips the kill step when no _portal-saver session exists"`
- `"it tolerates kill-session errors for an already-absent session"`
- `"it always invokes BootstrapPortalSaver after the version check"`
- `"it does not write daemon.version itself"`

**Edge Cases**:
- Version file absent while `_portal-saver` is live: accepted unnecessary restart — one ~50ms recreate, self-corrects on the next bootstrap after the new daemon writes its version. Spec explicitly accepts this trade-off.
- Empty-string version: same as absent — the file existing with empty contents is indistinguishable in intent from the file being missing.
- Literal `"dev"`: always restart. Covers the dev workflow where every `go build` produces a fresh binary that should immediately take over.
- Release semver equality: raw string comparison, no semver parsing. `v0.4.2` == `v0.4.2` is a match; any differing bytes trigger restart.
- `kill-session` tolerant of already-dead: the session might have auto-destroyed between the `has-session` check and the `kill-session` call. Error is ignored.
- Mass-kill races (two Portal invocations both trying to restart): benign. Each call is idempotent; worst-case is two recreates back-to-back, which is still correct end-state.
- `@portal-restoring` state: this task does not interact with the restoring marker. Bootstrap step 3 (task 2-12's integration) sets `@portal-restoring` *before* this task's invocation, so any SIGHUP delivered here causes the daemon's signal handler to skip the final flush — desired behaviour during upgrade-triggered restart to avoid capturing mid-transition state.

**Context**:
> Spec "tmux Hook Registration Lifecycle → Scenario 3: Portal upgrade with running server → Version-marker-based restart": "On daemon startup, `portal state daemon` writes its version (`cmd.version`) to `~/.config/portal/state/daemon.version`. On every `EnsureServer()` call, Portal reads `daemon.version` and compares to the currently-invoking binary's `cmd.version`. If they differ → `kill-session -t _portal-saver`, then recreate with the new binary. New daemon overwrites the version file on startup. If the version file is absent (first-ever bootstrap, or user-initiated state-dir cleanup) → treat as mismatch; recreate."
>
> Spec "tmux Hook Registration Lifecycle → Scenario 3 → Dev-build handling": "If `cmd.version` is empty or literally `\"dev\"`, treat every bootstrap as a mismatch and restart the daemon. Covers the common workflow of rebuilding Portal during development and expecting each rebuild's daemon code to take effect. Otherwise (release build, real semver), raw-string comparison determines mismatch."
>
> Spec "tmux Hook Registration Lifecycle → Scenario 3 → Data safety during restart": "tmux `kill-session -t _portal-saver` closes the PTY; kernel delivers SIGHUP to the daemon; signal handler flushes the final save via `AtomicWrite` before exit. New daemon takes over on recreation. Worst-case data loss: whatever accrued since the last dirty-flag check (≤1 second of scrollback drift)."
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 4. `_portal-saver` session setup": "Read `~/.config/portal/state/daemon.version`; compare to `cmd.version`: If `cmd.version` is empty or `\"dev\"` → always restart (kill + recreate) on bootstrap. If version file is absent → treat as mismatch (first-ever bootstrap). If stored version differs from `cmd.version` → `kill-session -t _portal-saver`, then recreate with the new binary. Else → leave running. Always run `set-option -t _portal-saver destroy-unattached off` (defensive, idempotent)."
>
> Existing pattern: `cmd.version` is a package-level var set via ldflags (`-X github.com/leeovery/portal/cmd.version`). The caller passes it into this function rather than importing `cmd` from `internal/`.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "tmux Hook Registration Lifecycle → Scenario 3: Portal upgrade with running server", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 4".

## built-in-session-resurrection-2-7 | approved

### Task 2-7: Scaffold `portal state daemon` entrypoint with startup side-effects and signal wiring

**Problem**: Tasks 2-5 and 2-6 create `_portal-saver` with `portal state daemon` as its command. Until this task lands, `portal state daemon` is the Phase 1 stub that exits 0 immediately — which means tmux auto-destroys `_portal-saver` right after creation, making all of Phase 2 non-functional. This task brings up the daemon's process-lifetime scaffolding: the startup side-effects (rotate log, clear `save.requested` defensively, write `daemon.pid` + `daemon.version`, seed scrollback hash map — seeding happens in task 2-9 but the hook is here), the blocking run loop that keeps the process alive (actual tick logic lands in task 2-12), and the SIGHUP / SIGTERM signal handlers that trigger a final flush. The capture + write path lands in subsequent tasks (2-8 through 2-11) and hangs off this scaffold; this task is the skeleton that keeps the daemon process alive and wires context cancellation so Ctrl-C / SIGHUP properly unblocks the run loop.

**Solution**: Replace the `cmd/state_daemon.go` stub with a `RunE` that: (1) resolves the state directory via `state.EnsureDir()`; (2) performs log-rotation check on `portal.log` (if size ≥ 1MB, rename to `portal.log.old`); (3) opens `portal.log` in `O_APPEND|O_CREATE|O_WRONLY` mode with `0600` and installs a minimal logger; (4) clears `save.requested` defensively via `os.Remove` (ignore ENOENT); (5) writes `daemon.pid` + `daemon.version`; (6) installs a signal handler for SIGHUP and SIGTERM that cancels a `context.Context`; (7) invokes a placeholder `daemonRun(ctx, deps)` that task 2-12 replaces with the real tick loop. The placeholder simply blocks on `<-ctx.Done()` and returns. On shutdown, run the final-flush path via the code landing in task 2-12; for this task, the final-flush call is a no-op stub (the capture code isn't wired yet) that checks `@portal-restoring` and would call `captureAndWrite` — stub the call so tests can verify it is invoked. Use a small logger component tag (`daemon`) matching the spec's log format.

**Outcome**: A correctly-compiled `portal state daemon` process writes `daemon.pid` and `daemon.version` within milliseconds of startup, clears any stale `save.requested`, rotates `portal.log` if needed, installs SIGHUP and SIGTERM handlers, and blocks on a cancellable context. SIGHUP or SIGTERM cancels the context, the placeholder run-loop returns, the final-flush stub runs (skipping if `@portal-restoring` is set), and the process exits 0. Task 2-12 replaces the placeholder body with the real tick loop without changing the startup / shutdown scaffolding. Tests exercise the signal path by spawning the daemon as a subprocess and verifying PID file presence, log rotation, and clean shutdown.

**Do**:
- Create `internal/state/logger.go`:
  - Minimal logger that writes `timestamp | level | component | message` lines to `portal.log` via `O_APPEND|O_CREATE|O_WRONLY`, mode `0600`.
  - API: `func OpenLogger(path string, rotate bool) (*Logger, error)`, `Logger.Warn(component, format, args...)`, `Logger.Error(...)`, `Logger.Info(...)`, `Logger.Debug(...)` (respects `PORTAL_LOG_LEVEL=debug`).
  - If `rotate` is true and `Stat(path).Size() >= 1 * 1024 * 1024`, rename `portal.log` → `portal.log.old` (replacing any existing `.old`) before opening the new log. Rotation happens at `OpenLogger` time, not lazily per-write.
  - Writes are `O_APPEND`-atomic on POSIX for sizes < `PIPE_BUF` (one line is trivially small). No locking.
  - Only the daemon passes `rotate: true`; every other writer passes `false`. Per spec: "Only the daemon rotates logs."
  - Non-daemon writers (the hydrate helper, `notify`, `signal-hydrate`, bootstrap) will use this same `OpenLogger` API with `rotate: false` in their respective tasks. This task only needs the API to exist and be used by the daemon.
- Create `internal/state/logger_test.go`:
  - Log-line format matches `timestamp | level | component | message`.
  - RFC 3339 UTC timestamp.
  - Rotation fires when file size ≥ 1MB; `portal.log.old` is overwritten if present; a new `portal.log` starts.
  - No rotation when size < 1MB.
  - No rotation when file does not exist (first-ever invocation).
  - `PORTAL_LOG_LEVEL=debug` enables DEBUG lines; default suppresses them.
- Replace `cmd/state_daemon.go`:
  - Import `internal/state`, `context`, `os`, `os/signal`, `syscall`, `time`.
  - `RunE` body:
    1. `dir, err := state.EnsureDir()`. Return on error.
    2. `logPath := state.PortalLog(dir)`.
    3. `logger, err := state.OpenLogger(logPath, true)`. Return on error (the daemon needs logging to be useful).
    4. `defer logger.Close()`.
    5. `logger.Info("daemon", "starting, version=%s, pid=%d", cmd.version, os.Getpid())`.
    6. `_ = os.Remove(state.SaveRequested(dir))` — defensive clear of stale dirty flag. Ignore `ENOENT`. Per spec "Defensive Dirty-Flag Clear on Daemon Startup."
    7. `if err := state.WritePIDFile(dir, os.Getpid()); err != nil { logger.Error("daemon", "failed to write daemon.pid: %v", err); return err }`.
    8. `if err := state.WriteVersionFile(dir, cmd.version); err != nil { logger.Error("daemon", "failed to write daemon.version: %v", err); return err }`.
    9. Create dependency struct (for task-2-12 replacement):
       ```go
       deps := &daemonDeps{
           StateDir: dir,
           Logger:   logger,
           Client:   tmux.NewClient(&tmux.RealCommander{}),
           // HashSeed, Ticker, etc. added in tasks 2-9 / 2-12.
       }
       ```
       Package-private `daemonDeps` struct in `cmd/state_daemon.go`; tests can swap fields.
    10. `ctx, cancel := context.WithCancel(context.Background())`; `defer cancel()`.
    11. Install signal handler:
       ```go
       sigCh := make(chan os.Signal, 2)
       signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
       go func() {
           sig := <-sigCh
           logger.Info("daemon", "received signal %s, shutting down", sig)
           cancel()
       }()
       ```
    12. Call `daemonRun(ctx, deps)` — the run loop. For this task, implement `daemonRun` as:
       ```go
       func daemonRun(ctx context.Context, deps *daemonDeps) error {
           <-ctx.Done()
           return shutdownFlush(deps)
       }
       ```
    13. `shutdownFlush(deps)` — stub for this task that: checks `@portal-restoring` via `deps.Client.GetServerOption("@portal-restoring")`; if set (value non-empty), log "skipping final flush (@portal-restoring set)" and return nil; else log "final flush" and return nil. Task 2-12 replaces the body with the real `captureAndWrite` call.
    14. Return `daemonRun`'s result. Cobra's `RunE` propagation surfaces any error as a non-zero exit.
- Keep `Hidden: true`, `Args: cobra.NoArgs` from task 1-1.
- The daemon is exempt from `PersistentPreRunE` bootstrap (task 1-3 via `state` in `skipTmuxCheck`). The daemon creates its own `tmux.Client` locally; no bootstrap is needed because the daemon does not register hooks or create sessions on its own — it only reads tmux state via `list-sessions`, `list-panes`, `show-options`, `capture-pane`, etc., which work against any running server.
- Tests in `cmd/state_daemon_test.go`:
  - Subprocess-based tests using `go test -run` + `exec.Command` on the built binary: start `portal state daemon`, read `daemon.pid` once it appears, send SIGHUP, verify process exits 0 within a 2s timeout. Integration test, not unit.
  - Unit-ish test by factoring `runDaemon` into a testable function that takes a `deps` struct and a ctx: call it with a pre-cancelled context; verify PID file was written, version file was written, `save.requested` was removed, and the shutdown-flush stub was invoked.
  - Repeated startup overwrites pid/version defensively: first call writes pid=X, version=v1; second call (same process) writes pid=X, version=v1 again via `AtomicWrite` — no duplicate-file error.
  - Log rotation: pre-populate `portal.log` with 2MB of content; invoke the daemon startup; verify `portal.log.old` exists with the old content and `portal.log` is fresh (small or empty, containing only the daemon's startup line).
  - Log rotation when `portal.log.old` already exists: rotation overwrites the old `.old`. Verify contents match the previous `portal.log`.
  - No rotation when `portal.log` < 1MB.
  - State directory missing → `EnsureDir` creates it → daemon starts normally.
  - SIGHUP cancels context: verify via a shutdown-flush spy that the stub ran exactly once on signal delivery.
  - SIGTERM cancels context: same verification.
  - `@portal-restoring` set → shutdown flush is skipped. Test by setting `@portal-restoring` via a mock tmux client and verifying the flush stub observes it.
- Do NOT wire the tick loop here — that lands in task 2-12. The placeholder `daemonRun` is a block-on-ctx-done stub sufficient for signal wiring tests.
- Do NOT implement capture / write here — tasks 2-8, 2-9, 2-10, 2-11 own those.

**Acceptance Criteria**:
- [ ] `portal state daemon` startup writes `daemon.pid` and `daemon.version` via atomic writes.
- [ ] Startup clears any existing `save.requested` (`os.Remove`, ignoring `ENOENT`).
- [ ] Startup rotates `portal.log` when its size is ≥ 1MB, replacing any existing `portal.log.old`.
- [ ] Startup does not rotate when `portal.log` is smaller than 1MB.
- [ ] Startup does not rotate when `portal.log` does not exist.
- [ ] `OpenLogger(path, true)` writes `timestamp | level | component | message` lines with mode `0600` via `O_APPEND`.
- [ ] `PORTAL_LOG_LEVEL=debug` enables DEBUG entries; default suppresses them.
- [ ] Daemon installs SIGHUP and SIGTERM signal handlers.
- [ ] SIGHUP or SIGTERM cancels the run-loop context and triggers a final-flush call.
- [ ] Final flush is skipped when `@portal-restoring` is set (value non-empty).
- [ ] Repeated startup overwrites pid/version files without error.
- [ ] Only the daemon performs log rotation; other Portal writers call `OpenLogger(path, false)` and skip rotation.

**Tests**:
- `"it writes daemon.pid and daemon.version on startup"`
- `"it clears stale save.requested on startup"`
- `"it rotates portal.log when size >= 1MB on startup"`
- `"it replaces an existing portal.log.old during rotation"`
- `"it does not rotate when portal.log is under 1MB"`
- `"it does not rotate when portal.log is absent"`
- `"it writes log lines in the documented timestamp | level | component | message format"`
- `"it suppresses DEBUG lines unless PORTAL_LOG_LEVEL=debug is set"`
- `"it cancels the run loop on SIGHUP"`
- `"it cancels the run loop on SIGTERM"`
- `"it calls the final-flush path on shutdown"`
- `"it skips the final flush when @portal-restoring is set"`
- `"it overwrites pid and version atomically on repeated startup"`
- `"it creates the state directory if missing"`

**Edge Cases**:
- State directory missing → `state.EnsureDir()` creates it with `0700`. Daemon proceeds normally.
- Existing `portal.log.old` overwritten on rotation: spec "replacing any existing old file." Prevents `.old` accumulation; `.old.old` never exists.
- Log file absent → no rotation (there is nothing to rotate). `OpenLogger` simply creates a fresh file on `O_CREATE`.
- `os.Remove(saveRequested)` returns `ENOENT` — expected and ignored. Any other error (permission denied) is logged at warn level but does not abort startup.
- Repeated startup — `fileutil.AtomicWrite` via `WritePIDFile` / `WriteVersionFile` handles the overwrite atomically. No lockfile.
- SIGHUP and SIGTERM both cancel context via the same handler — second signal after the first is a no-op (channel drain in the goroutine; `cancel()` is idempotent).
- `@portal-restoring` set at shutdown — `shutdownFlush` stub checks via `GetServerOption`; task 2-12 makes this the real final-capture path.
- Signal handler goroutine leak on clean exit — acceptable for this command (the process is about to exit anyway).
- Concurrent-writer logging — all Portal processes can log to `portal.log` via `O_APPEND`. POSIX guarantees atomic appends for sub-PIPE_BUF writes. Only the daemon rotates.

**Context**:
> Spec "CLI Surface → Internal Subcommands → `portal state daemon`": "The long-running process invoked as the `command` of the `_portal-saver` session. Responsibilities: Write `~/.config/portal/state/daemon.version` on startup with `cmd.version`. Write `~/.config/portal/state/daemon.pid` on startup with the daemon's OS PID. Clear `save.requested` on startup (defensive). Perform log-rotation check on startup (rotate `portal.log` → `portal.log.old` if the current log is ≥1 MB). Seed the in-memory `paneKey → scrollback-hash` map from existing `scrollback/*.bin` files (avoids full-rewrite on every startup). Hold the in-memory `paneKey → scrollback-hash` map for content-hash dedup. Run the 1-second ticker loop. Honor `@portal-restoring` (skip ticks while set). Trap SIGHUP and SIGTERM; flush final state (unless `@portal-restoring` is set)."
>
> Spec "Save-Side Architecture → Signal Handling": "The daemon traps two signals: **SIGHUP** — delivered by the kernel when tmux closes the PTY master fd. This is the dominant shutdown path (tmux `kill-server`, server crash, reboot). Discussion verified the kernel sends SIGHUP, not SIGTERM, in this case — Portal must trap SIGHUP explicitly. **SIGTERM** — delivered by direct `kill <pid>` from outside tmux. Less common but handled for completeness."
>
> Spec "Save-Side Architecture → Signal Handling → Handler behavior": "1. If the `@portal-restoring` marker is set, skip the final flush (an in-progress restore is underway; capturing now would capture mid-transition state). 2. Otherwise, flush the current state atomically via `AtomicWrite` and exit."
>
> Spec "Save-Side Architecture → Triggers & Serialization → Defensive Dirty-Flag Clear on Daemon Startup": "On daemon startup, the first action is to clear `save.requested` if present. This prevents a stale dirty flag from a prior (crashed or version-mismatch-restarted) daemon from triggering an immediate save of a mid-restore state."
>
> Spec "Observability & Diagnostics → Log File → Format": `timestamp | level | component | message` single-line format. Level: `DEBUG`/`INFO`/`WARN`/`ERROR`. Component: `daemon`, `restore`, `hydrate`, `notify`, `hooks`, `bootstrap`. Timestamp RFC 3339 UTC.
>
> Spec "Observability & Diagnostics → Log Rotation": "Simple 2-file cap at **1 MB per file**. On reaching 1 MB during a write, Portal renames `portal.log` → `portal.log.old` (replacing any existing old file), then starts a fresh `portal.log`. Total disk usage bounded at ~2 MB. Portal performs rotation itself in-process. **Concurrent-writer discipline.** Multiple Portal processes can log concurrently (daemon + CLI commands + hydrate helpers + signal-hydrate subprocesses). To avoid rotation races (two processes both observing ≥1 MB and both renaming, clobbering `portal.log.old`), **only the daemon rotates.** Every other Portal writer appends to `portal.log` with `O_APPEND`."
>
> Spec "Save-Side Architecture → Daemon Tick Loop (Pseudocode)": shows the `for { select { case <-ticker.C: ... case <-ctx.Done(): ... } }` structure. This task implements the shell of the function; task 2-12 fills in the ticker case body.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "CLI Surface → Internal Subcommands → `portal state daemon`", "Save-Side Architecture → Signal Handling", "Save-Side Architecture → Triggers & Serialization → Defensive Dirty-Flag Clear on Daemon Startup", "Observability & Diagnostics → Log File", "Observability & Diagnostics → Log Rotation", "Save-Side Architecture → Daemon Tick Loop".

## built-in-session-resurrection-2-8 | approved

### Task 2-8: Implement structural capture: enumerate sessions, panes, and per-session environment

**Problem**: The daemon's capture cycle has two halves: structural capture (sessions, windows, panes, layouts, environment) and content capture (per-pane scrollback). This task owns the structural half — producing a fully-populated `state.Index` (task 2-3) from live tmux state via a handful of tmux commands and some straightforward filtering. The spec pins the tmux commands (`list-sessions -F ...`, `list-panes -a -F ...`, `show-environment -t <session>`), the format variables to use (`#{window_layout}` not `#{window_visible_layout}`; `#{pane_current_path}`, `#{pane_active}`, `#{pane_current_command}`, `#{window_zoomed_flag}`), the filter (skip `_*` session names), and the edge cases (environment `-r` removed-form entries ignored, multi-byte UTF-8 session names, empty environment map round-trips as `{}`). Structural capture is deterministic and has no I/O beyond tmux calls — a good unit test target with `MockCommander`.

**Solution**: Add `internal/state/capture.go` with `func CaptureStructure(c *tmux.Client) (Index, error)`. The function enumerates sessions, filters `_*`, then per session queries windows + panes via a batched `list-panes -a -F '#{session_name}|#{window_index}|#{window_name}|#{window_layout}|#{window_zoomed_flag}|#{window_active}|#{pane_index}|#{pane_current_path}|#{pane_active}|#{pane_current_command}'` and parses the pipe-separated output. Per-session environment comes from `show-environment -t <session>`, parsed into `map[string]string` (ignoring `-<NAME>` removed-form lines). `SavedAt` is set to `time.Now().UTC()`. Scrollback file paths are filled in as `scrollback/<paneKey>.bin` using the task-2-1 sanitizer; content is NOT captured here (task 2-9 owns scrollback). The function returns a fully-populated `Index` ready for task 2-10's atomic commit step.

**Outcome**: `CaptureStructure(client)` against a mocked tmux server with two sessions (`work` and `_portal-saver`) produces an `Index` with `version=1`, `saved_at` ≈ now, and `sessions=[{name: "work", environment: {...}, windows: [...]}]` — the internal `_portal-saver` session is filtered. All `scrollback_file` paths are set to `scrollback/<paneKey>.bin` (content capture happens in task 2-9). Zero live sessions produces an `Index` with `sessions: []` (not `null`, thanks to `Index.Canonicalize()` from task 2-3). Multi-byte session names round-trip without corruption. Environment `-r` lines are ignored. Empty environment maps round-trip as `{}`.

**Do**:
- Create `internal/state/capture.go`:
  - `func CaptureStructure(c *tmux.Client) (Index, error)`:
    1. `idx := Index{Version: SchemaVersion, SavedAt: time.Now().UTC(), Sessions: []Session{}}`.
    2. Enumerate session names via `c.ListSessions()` (existing method) — returns `[]Session{Name, Windows, Attached}`. For each, skip if `strings.HasPrefix(name, "_")`.
    3. If zero non-underscore sessions: return `idx` with empty `Sessions` slice (canonicalized — `[]Session{}`).
    4. For the remaining sessions, call a single `list-panes -a -F '<pipe-separated format>'` to get all panes across all sessions in one call. Filter out panes whose session name starts with `_`. This is more efficient than N per-session calls.
       - Format string: `#{session_name}|#{window_index}|#{window_name}|#{window_layout}|#{window_zoomed_flag}|#{window_active}|#{pane_index}|#{pane_current_path}|#{pane_active}|#{pane_current_command}`.
       - Parse each line: 10 pipe-separated fields. Handle session names containing `|` by using a sufficiently uncommon delimiter — the spec does not anticipate `|` in session names, but defensive: use `\x00` (null) as the separator instead, since the paneKey sanitizer replaces nulls. Actually: pipe is fine; tmux's session name validation disallows `:` and `.`, and `|` is technically permitted but never used in practice. If a session name contains `|`, the capture silently truncates — documented limitation. Prefer a separator unlikely to appear: `"|||"` (triple pipe). Tmux's format output supports literal multi-character separators.
       - Use `"|||"` (triple pipe) as the separator throughout.
    5. Group parsed panes by session, then by window within each session. For each session:
       - Query `show-environment -t <session>` via a new `tmux.Client` helper `ShowEnvironment(session)` (add it — single-purpose, small). Returns raw output.
       - Parse environment output: each line is `KEY=VALUE` (set) or `-KEY` (removed). Skip lines starting with `-` (removed-form — per spec "removed-form variables (`-r` in tmux's on-wire syntax) were not captured; only plain set values round-trip"). For set lines, split on the first `=` (VALUE may contain `=`). Build `map[string]string`. If the output is empty, use `map[string]string{}` (not nil — for JSON round-trip).
       - Build `[]Window` from the grouped pane data. Within each window, `Panes` is ordered by `pane_index`. Window-level fields (`Index`, `Name`, `Layout`, `Zoomed`, `Active`) come from any pane's shared fields (they are identical within a window — tmux format expansion flattens them per pane).
       - For each `Pane`, set `ScrollbackFile` to `"scrollback/" + state.SanitizePaneKey(sessionName, windowIndex, paneIndex) + ".bin"` — relative to the state directory.
    6. Append populated `Session` to `idx.Sessions`, ordered by session name for stable test output.
    7. Call `idx.Canonicalize()` (task 2-3) and return.
- Add `func (c *Client) ShowEnvironment(session string) (string, error)`:
  - Calls `c.cmd.Run("show-environment", "-t", session)`. Returns raw output (parser's job to split lines). On error (session absent), returns `fmt.Errorf("failed to show environment for %q: %w", session, err)`.
- Tests in `internal/state/capture_test.go` using `MockCommander.RunFunc` with canned outputs:
  - Two sessions `work` and `_portal-saver`: `list-sessions` returns both, capture produces only `work` in the index. Verify the internal session is skipped.
  - Zero sessions: `ListSessions` returns `[]Session{}`; capture returns `Index{Version: 1, Sessions: []Session{}, ...}` — empty slice, not nil.
  - Session with one window + two panes: verify pane count, pane indices, `cwd`, `current_command`, `active`, and scrollback path.
  - Session with two windows: verify window indices, window names, layouts, zoomed flag, active flag all round-trip.
  - Multi-byte UTF-8 session name (e.g., `"プロジェクト"`): session name preserved in the Index without mojibake.
  - Environment with two set variables and one removed: removed-form line ignored, set variables appear in `Environment` map.
  - Empty environment (`show-environment` returns empty): `Environment` is `{}` (not nil) in the Index.
  - Environment value containing `=`: split on first `=` only, so `KEY=a=b=c` → `map["KEY"] = "a=b=c"`.
  - `list-panes -a` returns sessions in a specific order; capture groups + sorts so the resulting `Index.Sessions` is ordered alphabetically by session name for stable tests.
  - `list-panes -a` returns both `work` and `_portal-saver` panes; capture filters `_portal-saver`'s panes.
  - `ShowEnvironment` tmux error for a session → capture returns the error for that session. Document whether this is fatal or per-session: prefer fatal for this task — if environment capture fails, the whole cycle aborts and the daemon logs + retries next tick. The index structure depends on per-session environment, and partial environment could confuse Phase 3's restore.
  - `list-panes -a` error → capture returns the error; no partial index.
  - `SavedAt` within 1 second of `time.Now().UTC()` at test time.
- Do NOT implement scrollback content capture in this task — task 2-9 does that. `scrollback_file` paths are set; `.bin` files are written separately.
- Do NOT implement atomic commit / GC / write — task 2-10 does that.
- Do NOT handle the `@portal-skeleton-<paneKey>` filter here — task 2-11 layers the marker check on top of `CaptureStructure`.

**Acceptance Criteria**:
- [ ] `CaptureStructure` enumerates sessions via `list-sessions` and filters names beginning with `_`.
- [ ] `CaptureStructure` uses a single `list-panes -a -F '<pipe-separated format>'` to collect pane data across all sessions.
- [ ] The format string uses `#{window_layout}` (pre-zoom), NOT `#{window_visible_layout}`.
- [ ] Per-session `show-environment -t <session>` populates `Session.Environment`; removed-form (`-KEY`) lines are ignored.
- [ ] `Session.Environment` is an empty map (not nil) when no variables are defined.
- [ ] `Pane.ScrollbackFile` is set to `"scrollback/" + SanitizePaneKey(session, window, pane) + ".bin"` for every pane.
- [ ] `Index.Version == SchemaVersion` and `Index.SavedAt` is set to UTC.
- [ ] Zero live sessions produces `Index.Sessions == []Session{}` (empty non-nil).
- [ ] Multi-byte UTF-8 session names are preserved byte-exactly.
- [ ] Environment values containing `=` split on the first `=` only.
- [ ] `Window.Zoomed` reflects `#{window_zoomed_flag}`; `Window.Active` reflects `#{window_active}`.
- [ ] `Pane.Active`, `Pane.CurrentCommand`, `Pane.CWD` reflect the respective format variables.

**Tests**:
- `"it captures a single session with one window and one pane"`
- `"it filters sessions whose names begin with underscore"`
- `"it returns an empty Sessions slice when zero non-internal sessions exist"`
- `"it captures per-session environment from show-environment"`
- `"it ignores removed-form environment entries starting with a dash"`
- `"it returns an empty Environment map when show-environment output is empty"`
- `"it preserves multi-byte UTF-8 characters in session names"`
- `"it splits environment lines on the first = only"`
- `"it captures window layout from #{window_layout} not #{window_visible_layout}"`
- `"it captures zoomed and active flags per window"`
- `"it captures CWD, active, and current_command per pane"`
- `"it sets scrollback_file to scrollback/<paneKey>.bin via the canonical sanitizer"`
- `"it sorts sessions alphabetically by name for stable output"`
- `"it sets Index.Version to the schema constant"`
- `"it sets Index.SavedAt to UTC within the call"`
- `"it returns an error and no partial index when list-panes -a fails"`
- `"it returns an error when show-environment fails for a session"`

**Edge Cases**:
- `_*` sessions filtered — including the internal `_portal-saver` and any other `_`-prefixed internal sessions Portal may add in the future.
- Zero live sessions — `Sessions: []Session{}` empty slice, never nil. Critical for JSON round-trip.
- Environment `-r` removed-form lines — skipped silently; not captured.
- Multi-byte session names — preserved through pipe-separator parsing (tmux's format output is UTF-8; pipe separators are ASCII so they cannot appear inside multi-byte runes).
- Environment value with `=` — `split("KEY=a=b", "=", 2)` preserves the value correctly.
- Empty environment — `show-environment` for a session with no custom vars may return empty output; map is `{}` not nil.
- Multi-word environment values with spaces — preserved verbatim; tmux does no escaping in `show-environment` output.
- The chosen separator `"|||"` — a session name containing literal `"|||"` would confuse parsing. This is pathological; document as a known limitation. Real session names never contain triple-pipes.
- `list-panes -a` output ordering — tmux orders by session insertion then by window + pane. Capture groups + sorts for stable test output.
- Session appears in `list-sessions` but not in `list-panes -a` — race condition (session closed between calls). Safe behaviour: the empty-panes session simply has `[]Window{}`; Phase 3 Restore logs and skips empty-pane sessions. Prefer to filter such sessions out in capture to avoid emitting structurally-invalid entries. Actually — per spec "If a saved session's `panes` array is empty (corrupt or invalid `sessions.json`) → log a warning, skip that window/session entirely" — that logic is in Phase 3; capture should not pre-filter. Emit the session with empty windows; restore handles.
- Pane `current_command` field is internal-diagnostic only per spec — captured but not surfaced in `portal state status`. This task captures it faithfully.

**Context**:
> Spec "Save Format & Schema → Structural Index → Field semantics": names every field and its source format variable: `sessions[].windows[].layout` comes from `#{window_layout}` (not `window_visible_layout`), `windows[].zoomed` from `#{window_zoomed_flag}`, `windows[].active` from `#{window_active}`, `panes[].cwd` from `#{pane_current_path}`, `panes[].active` from `#{pane_active}`, `panes[].current_command` from `#{pane_current_command}`.
>
> Spec "Save Format & Schema → Atomic Commit Discipline → 1. In-memory capture": "Enumerate live sessions (skipping `_*` names), call `list-panes -a -F ...`, `show-environment -t <session>` per session, and `capture-pane -e -p -S - -t <pane>` per eligible pane. All reads run to completion before any writes."
>
> Spec "Bootstrap Flow → PersistentPreRunE Sequence → 5. Restore()": Per-session environment is applied via `set-environment -t <session>` on restore; task 2-8 captures it so task 2-10 can commit it. Spec notes: "Removed-form variables (`-r` in tmux's on-wire syntax) were not captured; only plain set values round-trip."
>
> Spec "Layout Restoration → Layout String Source": "Portal captures `#{window_layout}` (pre-zoom form), not `#{window_visible_layout}`. The pre-zoom form is the correct input for `select-layout` replay. Storing the zoomed form would cause `select-layout` to collapse panes incorrectly on re-application."
>
> Spec "Save-Side Architecture → Session Visibility and Filtering": "Portal filters sessions whose names begin with `_` (underscore prefix is reserved for Portal internals) from: The TUI session picker. `sessions.json` capture (the save process skips `_*` sessions when enumerating live state). Any future internal-only sessions."
>
> Existing pattern: `tmux.Client.ListSessions` already exists. `show-environment` does not; add `ShowEnvironment(session)` here.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save Format & Schema → Structural Index", "Save Format & Schema → Atomic Commit Discipline", "Save-Side Architecture → Session Visibility and Filtering", "Layout Restoration → Layout String Source".

## built-in-session-resurrection-2-9 | approved

### Task 2-9: Implement per-pane scrollback capture with xxhash content dedup and startup seed

**Problem**: Raw per-pane scrollback can grow to a meaningful size (`history-limit 50000 × 10 panes × avg-line-bytes`). Writing every file on every tick wastes disk (the spec estimates ~86 GB/day in heavy configs) and burns SSD cycles. The spec's answer is content-hash dedup: hash each pane's captured bytes, compare to an in-memory `paneKey → hash` map, and only write the `.bin` file when the hash has changed. To avoid the first-tick-after-startup rewriting every file (because the in-memory map starts empty), the daemon seeds the map from disk on startup — read each existing `scrollback/*.bin`, hash it, populate the map. After seeding, the first tick only writes genuinely-changed scrollback. xxhash (non-cryptographic, ~several GB/s) is the spec's chosen hash.

**Solution**: Add `internal/state/scrollback.go` with (a) `type HashMap map[string]uint64` — paneKey → xxhash of last-written content; (b) `func SeedHashMap(dir string, logger *Logger) HashMap` — reads `scrollback/*.bin`, hashes each, returns a populated map (logs and continues on unreadable files); (c) `func CaptureAndHashPane(c *tmux.Client, target string) ([]byte, uint64, error)` — runs `capture-pane -e -p -S - -t <target>` and returns the bytes + xxhash; (d) `func WriteScrollbackIfChanged(dir, paneKey string, bytes []byte, hash uint64, hm HashMap) (written bool, err error)` — atomic-writes the file and updates `hm` only if the hash differs from the stored value. The function composes with task 2-8's structural capture and task 2-10's commit to produce a coherent tick cycle.

**Outcome**: Seeding on daemon startup returns a `HashMap` populated from every `.bin` file under `scrollback/`. A capture cycle calls `CaptureAndHashPane` for each live eligible pane, then `WriteScrollbackIfChanged` — returning `written = false` if the hash matches. Identical scrollback across two consecutive ticks produces zero writes. Empty scrollback (fresh pane) writes one empty `.bin` file on first tick and skips on subsequent ticks. An unreadable `.bin` during seed is logged at warn level and the seed continues with the remaining files. Hash collisions (xxhash produces ~1-in-2^64 chance — astronomically rare) are accepted.

**Do**:
- Create `internal/state/scrollback.go`:
  - `type HashMap map[string]uint64`.
  - `func SeedHashMap(dir string, logger *Logger) HashMap`:
    1. `hm := HashMap{}`.
    2. `scrollbackDir := ScrollbackDir(dir)`.
    3. `entries, err := os.ReadDir(scrollbackDir)`. On `os.IsNotExist`: return empty map (directory not yet created). On other error: `logger.Warn("daemon", "seed: readdir %s: %v", scrollbackDir, err)` and return empty map.
    4. For each entry: skip if not a file, or if the name does not end in `.bin`. Compute `paneKey := strings.TrimSuffix(name, ".bin")`.
    5. `data, err := os.ReadFile(filepath.Join(scrollbackDir, name))`. On error: `logger.Warn("daemon", "seed: read %s: %v", name, err)` and continue to the next entry.
    6. `hm[paneKey] = xxhash.Sum64(data)`.
    7. Return `hm`.
  - `func CaptureAndHashPane(c *tmux.Client, target string) ([]byte, uint64, error)`:
    1. `out, err := c.CapturePane(target)`.
    2. On error: return `nil, 0, err`.
    3. `hash := xxhash.Sum64([]byte(out))`.
    4. Return `[]byte(out), hash, nil`.
  - `func WriteScrollbackIfChanged(dir, paneKey string, data []byte, newHash uint64, hm HashMap) (bool, error)`:
    1. `prev, ok := hm[paneKey]`.
    2. If `ok && prev == newHash`: return `false, nil` (no write).
    3. `path := ScrollbackFile(dir, paneKey)`.
    4. `err := fileutil.AtomicWrite(path, data)`. On error: return `false, err`.
    5. Set mode `0600` on the written file (`fileutil.AtomicWrite` creates with temp-file default; explicitly `os.Chmod(path, 0o600)` after rename). Ignore chmod errors.
    6. `hm[paneKey] = newHash`.
    7. Return `true, nil`.
- Add `func (c *Client) CapturePane(target string) (string, error)` to `internal/tmux/tmux.go`:
  - Calls `c.cmd.Run("capture-pane", "-e", "-p", "-S", "-", "-t", target)`.
  - Returns raw output (no trimming — ANSI escapes include trailing bytes that matter).
  - On error: wrapped `fmt.Errorf("failed to capture pane %q: %w", target, err)`.
  - Note: the existing `RealCommander.Run` trims whitespace. This is wrong for scrollback — ANSI scrollback can legitimately end in whitespace or escape sequences. Fix: add a parallel `RunRaw(args ...string) (string, error)` method on `Commander` that does NOT trim. Implement on `RealCommander`. Update `MockCommander` in tests to add `RunRaw` too. `CapturePane` uses `RunRaw`; existing methods continue to use `Run`.
- Tests in `internal/state/scrollback_test.go` and `internal/tmux/tmux_test.go` (for `CapturePane` / `RunRaw`):
  - `SeedHashMap` on empty directory → `HashMap{}` (empty map).
  - `SeedHashMap` on directory with three `.bin` files → three entries with correct xxhash values.
  - `SeedHashMap` skips non-`.bin` files (e.g., a stray `README` in the scrollback dir).
  - `SeedHashMap` with an unreadable file (permissions `000`) logs a warning and continues; other files still seeded.
  - `SeedHashMap` on a missing scrollback directory → empty map, no error.
  - `CaptureAndHashPane` runs `capture-pane -e -p -S - -t <target>` via `RunRaw` and returns bytes + hash.
  - `CaptureAndHashPane` error propagates from the Commander.
  - `WriteScrollbackIfChanged` with no prior hash → writes file, returns `(true, nil)`, map updated.
  - `WriteScrollbackIfChanged` with matching prior hash → no write, returns `(false, nil)`, file on disk unchanged (verify via stat).
  - `WriteScrollbackIfChanged` with different prior hash → writes file, updates map.
  - Written file has mode `0600`.
  - Atomic write via temp-file + rename — verify by asserting the file never has intermediate sizes during concurrent reads (use a goroutine that polls `os.Stat` and fails if it sees a non-final size).
  - Empty scrollback (bytes = `[]byte{}`): first capture writes zero-byte file with hash of empty string; second capture with same empty state skips the write.
  - Two panes with identical scrollback produce identical hashes but are stored under different paneKeys — dedup is per-pane, not global.
  - `RunRaw` returns output verbatim without trimming.
- Ensure `xxhash` import is `github.com/cespare/xxhash/v2` — added in task 2-1. Confirm `go.mod` is correct.
- Do NOT integrate with capture cycle timing here — task 2-12 owns ticker logic. This task's functions are pure primitives.
- Do NOT apply the `@portal-skeleton-<paneKey>` skip here — task 2-11 layers marker awareness on top.

**Acceptance Criteria**:
- [ ] `SeedHashMap` returns `HashMap{}` for an empty or missing scrollback directory.
- [ ] `SeedHashMap` hashes every `.bin` file and populates the map by paneKey (filename minus `.bin` extension).
- [ ] `SeedHashMap` logs warnings and continues on unreadable `.bin` files.
- [ ] `SeedHashMap` skips non-`.bin` files silently.
- [ ] `CapturePane` uses `RunRaw` (no output trimming) and passes `-e -p -S -` flags verbatim.
- [ ] `CaptureAndHashPane` returns the captured bytes plus their xxhash.
- [ ] `WriteScrollbackIfChanged` is a no-op (no write, `written=false`) when the new hash matches the stored hash.
- [ ] `WriteScrollbackIfChanged` writes via `AtomicWrite` and updates the map when the hash differs or is absent.
- [ ] Written `.bin` files have mode `0600`.
- [ ] Empty scrollback writes a zero-byte `.bin` file on first capture and skips on subsequent captures with same (empty) content.
- [ ] The `RunRaw` method preserves trailing whitespace and ANSI escape sequences byte-exactly.
- [ ] Hash dedup is per paneKey — two panes with identical content are stored under distinct files.

**Tests**:
- `"it returns an empty HashMap for a missing scrollback directory"`
- `"it returns an empty HashMap for an empty scrollback directory"`
- `"it hashes every .bin file during seed"`
- `"it skips non-bin files during seed"`
- `"it logs a warning and continues when a .bin file is unreadable"`
- `"it uses capture-pane -e -p -S - -t <target> verbatim"`
- `"it returns both bytes and hash from CaptureAndHashPane"`
- `"it skips the write when the new hash matches the stored hash"`
- `"it writes and updates the hash when content has changed"`
- `"it writes and inserts the hash when the paneKey was absent from the map"`
- `"it writes scrollback files with mode 0600"`
- `"it writes a zero-byte file for empty scrollback on first capture"`
- `"it skips zero-byte writes on subsequent captures of identical empty scrollback"`
- `"it preserves trailing whitespace and ANSI escapes via RunRaw"`
- `"it maintains independent hash entries per paneKey"`

**Edge Cases**:
- First-ever daemon startup — no `scrollback/` directory yet. `SeedHashMap` returns empty map; first tick writes every pane's scrollback.
- Existing `.bin` file unreadable (permissions 000) during seed — log and continue. The in-memory hash for that paneKey stays absent, so the first tick overwrites the unreadable file (and likely fails with the same permission error, which `WriteScrollbackIfChanged` surfaces).
- Hash collision — xxhash is non-cryptographic, ~1 in 2^64 chance. Accepted per spec — "hash collision accepted per xxhash guarantees." Worst case: one stale scrollback file. User re-attaching fires a fresh capture; the stale version is eventually overwritten on next content change.
- Empty scrollback — hash of `[]byte{}` is a specific xxhash value; round-trips consistently. First empty-capture writes zero-byte file; subsequent empty captures skip.
- Very large scrollback (> 50 MB per pane — unusual but possible with wider history-limit or heavy ANSI colour output) — `ReadFile` loads into memory; hash is computed over the whole slice; `AtomicWrite` writes to temp + renames. Memory cost is bounded by `history-limit × avg-line-bytes` per pane; the seed step is the bottleneck (all files loaded sequentially). Acceptable per spec: "Seed cost scales with total on-disk scrollback (~30 panes × 500 KB × few ms/MB = sub-second)."
- Daemon restart — `SeedHashMap` repopulates the map from disk so the first tick after restart does not rewrite unchanged panes. This is load-bearing for dev builds (every `go build` triggers a version-mismatch restart).
- Concurrent daemon invocations — should never happen (single-writer architecture). If it does, both writers race on `AtomicWrite` — both succeed but one rename wins; the losing write's temp file is removed. No corruption risk, but the `HashMap` in each daemon is out of sync with disk after the race.

**Context**:
> Spec "Save Format & Schema → Content-Hash Dedup": "To avoid rewriting unchanged scrollback on every tick — which would generate on the order of 86 GB/day of writes in a heavy-history configuration (`history-limit 50000` × 10 panes) and cause significant SSD wear — the daemon holds an in-memory map `paneKey → hash-of-last-written-scrollback`. Content-hash dedup reduces worst-case write volume to single-digit MB/day for realistic workloads."
>
> Spec "Save Format & Schema → Content-Hash Dedup → Per pane per capture cycle": "1. Capture scrollback bytes (cheap — tmux internal buffer). 2. Hash the bytes (xxhash or equivalent fast non-cryptographic hash). 3. Compare to the stored hash for this pane. 4. If identical → skip the disk write; no change. 5. If different → `AtomicWrite` the scrollback file, update the stored hash."
>
> Spec "Save Format & Schema → Content-Hash Dedup → Daemon-startup seed": "On startup the in-memory hash map is empty; without a seed step, the first tick after every daemon start (including the version-mismatch restart that fires on every `portal open` during `dev`/empty-version builds) would rewrite every scrollback file. The daemon avoids this by **seeding the hash map from disk on startup**: read each existing `scrollback/*.bin`, hash the bytes, populate the `paneKey → hash` map. Seed cost scales with total on-disk scrollback (~30 panes × 500 KB × few ms/MB = sub-second). After seeding, the first tick only writes panes whose live scrollback genuinely differs from what is on disk — typical case is a near-no-op."
>
> Spec "In Scope — Captured and Restored → Content": "Full pane main-screen scrollback with ANSI escape sequences — colors, attributes, formatting preserved via `tmux capture-pane -e -p -S - -t <pane>`". The `-e` flag preserves escape sequences; `-p` prints to stdout; `-S -` captures from the start of history.
>
> Spec "Save Format & Schema → Storage Location": "All files written with mode `0600` (owner read/write only)."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save Format & Schema → Content-Hash Dedup", "In Scope → Content", "Save Format & Schema → Storage Location".

## built-in-session-resurrection-2-10 | approved

### Task 2-10: Atomic commit of `sessions.json` plus post-commit orphan GC

**Problem**: A capture cycle must commit its work atomically so restore never reads a half-written state. The spec pins the commit order: in-memory capture → per-pane scrollback writes (only for changed panes) → `sessions.json` written last via `AtomicWrite`, and **the `sessions.json` rename is the atomic commit**. After the rename succeeds, a GC step sweeps the scrollback directory, removing any `.bin` file not referenced by the freshly-written index. The GC is synchronous (runs on the same goroutine immediately after the commit), idempotent, and tolerant of `ENOENT`. Cycles that produce no changes (no pane hash differed, no structural delta) skip both the `sessions.json` write and the GC — preserving the "zero disk activity when nothing changed" property.

**Solution**: Add `internal/state/commit.go` with `func Commit(dir string, idx Index, anyScrollbackChanged bool, logger *Logger) error`. The function decides whether to write `sessions.json` based on structural delta (compare with on-disk `sessions.json` via a content-hash check) OR `anyScrollbackChanged`. If neither, skip both write and GC. Otherwise `AtomicWrite` the JSON, then run GC: list `scrollback/*.bin`, collect the set of referenced paths from the fresh Index, `os.Remove` any file not in the set. GC per-file failures are logged and continue; `ENOENT` during the remove is ignored (expected race). A companion `ComputeReferencedSet(idx Index) map[string]struct{}` utility extracts the referenced scrollback filenames for the GC step and for tests.

**Outcome**: `Commit` on a cycle with structural changes (e.g., window renamed) writes the new `sessions.json` and GC removes any `.bin` files no longer referenced. On a zero-change cycle — same structure, all pane hashes matched — neither the JSON write nor the GC runs. GC tolerates `ENOENT` (another process deleted the file first). Per-file GC failures are logged but do not abort the GC loop. A `sessions.json` write failure leaves the previous state intact (temp file cleaned up by `AtomicWrite`).

**Do**:
- Create `internal/state/commit.go`:
  - `func Commit(dir string, idx Index, anyScrollbackChanged bool, logger *Logger) error`:
    1. Canonicalize idx (task 2-3's helper ensures nil slices → `[]`).
    2. Encode the new Index via `EncodeIndex(idx)`.
    3. Determine if the structural state changed: read `sessions.json` from disk, compare content byte-exact ignoring `saved_at` (since that always changes). Approach: decode the old file, zero out `SavedAt` on both old and new, compare via `reflect.DeepEqual`. If equal AND `!anyScrollbackChanged`: return `nil` (skip commit + GC).
    4. If old file absent or decode fails: treat as "structural changed" and proceed.
    5. `sessionsPath := SessionsJSON(dir)`.
    6. `if err := fileutil.AtomicWrite(sessionsPath, data); err != nil { return fmt.Errorf("commit sessions.json: %w", err) }`.
    7. `os.Chmod(sessionsPath, 0o600)` — ignore errors.
    8. Run GC via `gcOrphanScrollback(dir, idx, logger)`. GC errors are logged but not fatal — the commit has already succeeded; a partial GC is better than surfacing an error that callers might interpret as "commit failed."
    9. Return `nil`.
  - `func ComputeReferencedSet(idx Index) map[string]struct{}`:
    1. `ref := map[string]struct{}{}`.
    2. For each session / window / pane: if `pane.ScrollbackFile != ""`, `ref[pane.ScrollbackFile] = struct{}{}`.
    3. Return.
  - `func gcOrphanScrollback(dir string, idx Index, logger *Logger) error`:
    1. `ref := ComputeReferencedSet(idx)` — keys are relative paths like `"scrollback/<paneKey>.bin"`.
    2. `scrollbackDir := ScrollbackDir(dir)`.
    3. `entries, err := os.ReadDir(scrollbackDir)`. On `os.IsNotExist`: return nil (directory not yet created). Other errors: log warn, return nil (GC is best-effort).
    4. For each entry: skip non-files and non-`.bin`. Compute `relPath := "scrollback/" + entry.Name()`.
    5. If `_, ok := ref[relPath]; !ok`: `err := os.Remove(filepath.Join(scrollbackDir, entry.Name()))`. On `os.IsNotExist`: silent (expected race). Other errors: `logger.Warn("daemon", "gc: remove %s: %v", entry.Name(), err)` and continue.
    6. Return nil.
- Tests in `internal/state/commit_test.go` using `t.TempDir`:
  - Fresh state (no on-disk `sessions.json`): first `Commit` writes the file and runs GC. Verify file presence + mode `0600`.
  - Zero-change cycle: write a `sessions.json`, run `Commit` with a structurally-identical idx and `anyScrollbackChanged=false`: verify no file rewrite (use file mtime — mtime does not advance) and verify no GC ran (add a `.bin` file not referenced by the idx and assert it is still present).
  - Structural change: initial idx had one session; new idx has two. `Commit` writes new file + runs GC. Pre-populate an unreferenced `.bin` file; assert it is removed after Commit.
  - Scrollback changed only (structure unchanged): `anyScrollbackChanged=true`. `Commit` writes `sessions.json` (the saved_at will be newer, which is desired) and runs GC.
  - GC with `ENOENT` during remove: simulate by creating an unreferenced file, then deleting it between `ReadDir` and `Remove` (tricky — use a spy `os.Remove` or just test the `errors.Is(err, os.ErrNotExist)` branch by deleting manually in a test after `ReadDir` is called). Acceptable alternative: verify `os.Remove` on an absent path is tolerated via direct call (standard Go behaviour).
  - GC with per-file remove failure (file on read-only filesystem — simulate via chmod-0500 on the parent dir then restore): log warn, continue. Verify other removals still succeed.
  - GC preserves files referenced by the new idx (skeleton-marked panes' files — those entries should still be in the idx per task 2-11, so GC won't remove them; this task's test just checks the referenced-set filter, not marker awareness).
  - `AtomicWrite` failure (disk full simulated by making the dir read-only temporarily) → `Commit` returns the wrapped error; prior `sessions.json` on disk is intact.
  - GC failure does not cause `Commit` to return error — it logs and returns nil.
  - Referenced set correctly collects paths from all panes across multiple sessions.
- Do NOT tie this task to marker-awareness — task 2-11 layers `@portal-skeleton-<paneKey>` handling on top of the capture path, not the commit path. A skeleton-marked pane's scrollback file is already in the idx (preserved from the prior cycle) and therefore referenced — GC preserves it.
- Do NOT integrate with the tick loop here — task 2-12 owns the composition of capture + commit into a tick cycle.

**Acceptance Criteria**:
- [ ] `Commit` writes `sessions.json` via `AtomicWrite` and chmod-`0600`s the result.
- [ ] `Commit` skips both the write and the GC on a zero-change cycle (structurally identical new idx AND no scrollback changes).
- [ ] `Commit` writes on any structural change.
- [ ] `Commit` writes when scrollback changed even if structure did not.
- [ ] GC removes `.bin` files not referenced by the fresh idx.
- [ ] GC tolerates `ENOENT` on remove (another process beat us).
- [ ] GC logs per-file remove failures and continues with remaining files.
- [ ] GC tolerates a missing `scrollback/` directory (returns nil).
- [ ] `AtomicWrite` failure returns a wrapped error; prior `sessions.json` remains intact on disk.
- [ ] GC failure does not cause `Commit` to return error (best-effort; logs only).
- [ ] `ComputeReferencedSet` collects scrollback paths across all sessions and panes in the idx.

**Tests**:
- `"it writes sessions.json on the first commit"`
- `"it skips the write and GC on a zero-change cycle"`
- `"it writes when the structure has changed"`
- `"it writes when scrollback has changed but structure has not"`
- `"it removes orphan .bin files not referenced by the new idx"`
- `"it preserves .bin files referenced by the new idx"`
- `"it tolerates ENOENT during GC remove"`
- `"it logs and continues when a single file remove fails"`
- `"it tolerates a missing scrollback directory"`
- `"it returns a wrapped error when AtomicWrite fails and leaves prior sessions.json intact"`
- `"it does not return an error when GC fails after a successful commit"`
- `"it writes sessions.json with mode 0600"`
- `"it collects scrollback paths from all sessions and panes via ComputeReferencedSet"`

**Edge Cases**:
- Zero-change cycle — no write, no GC. Preserves the spec's "zero disk activity when nothing changed" property.
- `ENOENT` during GC remove — expected race when the file was removed by another cleanup path. Silent.
- Per-file GC failure — logged at warn level, loop continues. No aggregate error — GC is purely best-effort.
- Missing scrollback directory — happens on first-ever daemon run before any pane has been captured. Returns nil.
- `AtomicWrite` failure (disk full, permission denied) — wrapped error propagated. Temp file cleaned up by `AtomicWrite` itself. Prior `sessions.json` intact. Task 2-12's tick loop logs the error and retries on the next tick.
- GC preserves skeleton-marked panes' files — the marker-aware capture in task 2-11 preserves those panes' entries in the idx, so their scrollback paths end up in the referenced set. GC sees them as referenced and skips them.
- `saved_at` difference only — spec: "zero-change cycle skips both write and GC." Comparing idx byte-exactly would trigger a write every tick because `saved_at` always advances. Solution: normalise `saved_at` to zero before comparison.
- The `reflect.DeepEqual` comparison over large Index structures is O(n) in total size; acceptable at Portal's scale (~100s of panes × small per-pane data).
- Temp file cleanup — `fileutil.AtomicWrite` handles it on error. No additional temp-file sweeping needed.

**Context**:
> Spec "Save Format & Schema → Atomic Commit Discipline": "Multi-file state with per-file atomicity. The commit order: 1. **In-memory capture.** ... 2. **Per-pane scrollback writes.** For each pane whose scrollback hash changed (see Content-Hash Dedup below), write its `.bin` file via `AtomicWrite` (temp file + rename). Unchanged panes are skipped. 3. **Structural index write.** `sessions.json` written last, via `AtomicWrite`. **This rename is the atomic commit.** If the rename succeeds, all referenced scrollback files are present on disk."
>
> Spec "Save Format & Schema → Atomic Commit Discipline → Failure modes": "Crash before step 3 → old `sessions.json` still valid, still references old scrollback files. Restore works as of the previous save. Crash mid-step 3 → `AtomicWrite` guarantees either the old or new `sessions.json`, never a partial. Still consistent. Orphan new scrollback files from a partial save → cleaned by GC on the next successful save."
>
> Spec "Save Format & Schema → GC / Orphan Cleanup": "After every successful save (after `sessions.json` is atomically committed), run GC synchronously: 1. Read the freshly-written `sessions.json` and collect every `scrollback_file` path it references. 2. List everything under `scrollback/`. 3. Any file present on disk but NOT referenced by the new index → `os.Remove`. Handles every stale-file scenario: Pane closed → file no longer referenced → deleted. Session renamed → old-name files deleted, new-name files written. Window or pane renumbered → same. Orphan files from a previous mid-save crash → cleaned on next successful save. Idempotent. Runs once per save. Self-healing by construction."
>
> Spec "Save Format & Schema → Content-Hash Dedup": "`sessions.json` is written at the end of the cycle only if *anything* changed (structural delta or at least one pane's hash differed). If a full 30-second cycle produces zero changes, zero disk activity occurs."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save Format & Schema → Atomic Commit Discipline", "Save Format & Schema → GC / Orphan Cleanup", "Save Format & Schema → Content-Hash Dedup".

## built-in-session-resurrection-2-11 | approved

### Task 2-11: Marker-aware capture via single `show-options -sv` per cycle

**Problem**: Phase 3's skeleton restore sets `@portal-skeleton-<paneKey>` on each created pane to tell the daemon "this pane is awaiting hydration — its saved scrollback on disk is authoritative; do not overwrite it with a capture of the blank pane that has not yet been hydrated." The daemon must honour this marker in its capture loop by skipping any pane whose marker is set. The spec pins the mechanism: *one* `show-options -sv` call per tick (dumps every server-option the tmux server knows about), filter the output in memory for keys prefixed with `@portal-skeleton-`, and skip any pane whose computed paneKey is in that set. Avoids N per-pane `show-option` calls (which would dominate tick cost at scale). Skeleton-marked panes also retain their existing `sessions.json` entry — the daemon should not drop them from the index just because the current capture skipped them.

**Solution**: Add `internal/state/markers.go` with `func ListSkeletonMarkers(c *tmux.Client) (map[string]struct{}, error)` — runs `show-options -sv`, scans lines for `@portal-skeleton-<suffix>` keys, returns the set of `paneKey`s. Extend task 2-8's `CaptureStructure` (rename to take a `skipSet map[string]struct{}` parameter, or add a sibling function `CaptureStructureWithSkip`) so panes in the skip set are not re-captured but their previous `sessions.json` entries + `.bin` files are preserved. The preservation is achieved by merging: read the prior `sessions.json`; for each pane whose paneKey is in the skip set, copy the old entry into the new idx. Panes not in the skip set are captured fresh. Scrollback write loop (task 2-9) also skips skeleton-marked panes — the daemon does not call `CaptureAndHashPane` on them.

**Outcome**: With `@portal-skeleton-work__0.0 = 1` set on tmux, a capture cycle: (1) runs one `show-options -sv`; (2) parses out `{"work__0.0"}`; (3) captures structural data for all non-skipped panes; (4) merges prior `sessions.json` entries for the skipped pane so `work__0.0` still appears in the new index pointing to its (preserved) `.bin` file; (5) does not run `capture-pane` or `WriteScrollbackIfChanged` on `work__0.0`. After the user attaches (Phase 3 flow), the helper clears the marker via `set-option -su`; the next tick observes the marker gone and captures normally, producing a fresh `.bin` file under the live paneKey.

**Do**:
- Create `internal/state/markers.go`:
  - `const SkeletonMarkerPrefix = "@portal-skeleton-"`.
  - `const RestoringMarkerName = "@portal-restoring"`.
  - `func ListSkeletonMarkers(c *tmux.Client) (map[string]struct{}, error)`:
    1. `out, err := c.ShowAllServerOptions()` — a new method on `tmux.Client` below.
    2. On error: return `nil, err`.
    3. Parse each line as `<name><sep><value>`. tmux's `show-options -sv` output format: `name value` separated by a space (or `name "quoted value"` if the value contains spaces). Safe approach: split on first whitespace. Value does not matter for this filter — presence of the key is sufficient.
    4. For each line where the name starts with `SkeletonMarkerPrefix`: extract the paneKey = `strings.TrimPrefix(name, SkeletonMarkerPrefix)`.
    5. Return the set.
  - `func IsRestoringSet(c *tmux.Client) (bool, error)`:
    1. Uses `GetServerOption(RestoringMarkerName)`; returns `true` if the value is non-empty, `false` if `ErrOptionNotFound`, error otherwise.
    2. Spec: "marker values other than `1` still treated as present" — any non-empty value means set.
- Add `func (c *Client) ShowAllServerOptions() (string, error)` to `internal/tmux/tmux.go`:
  - Calls `c.cmd.Run("show-options", "-sv")`.
  - Returns the raw output verbatim (parser handles whitespace).
  - Wraps error with context.
- Extend `CaptureStructure` (task 2-8) to accept markers + prior index:
  - New signature: `func CaptureStructure(c *tmux.Client, skipSet map[string]struct{}, prev *Index) (Index, error)`.
  - After building the fresh idx from live tmux: for each pane in `prev` (if non-nil) whose `SanitizePaneKey(session, window, pane)` is in `skipSet`, copy the entry into the fresh idx. Copying semantics:
    - If the session is missing from the fresh idx but present in `prev`: add the whole session entry from prev.
    - If the session is present in both: find the window in prev by index; if missing in fresh, add the whole window entry. If present, find the pane by index; if missing, add the pane entry.
    - This preserves structural data (layouts, environment, active flags) for skeleton-marked panes until hydration completes.
  - Tests document this merging behaviour end-to-end.
- Extend task 2-12's ticker to filter `capture-pane` and `WriteScrollbackIfChanged` by `skipSet` — this task's responsibility is the skip set + structural merge; the actual scrollback loop integration is in task 2-12.
- Tests in `internal/state/markers_test.go` using `MockCommander.RunFunc`:
  - `show-options -sv` empty output → empty set.
  - `show-options -sv` with unrelated `@` options (e.g., `@my-option 1`, `@user-prefs foo`) → empty set (no skeleton-prefixed keys).
  - `show-options -sv` with `@portal-skeleton-work__0.0 1` → set containing `"work__0.0"`.
  - `show-options -sv` with marker values other than `1` (e.g., `@portal-skeleton-work__0.1 anything`) → still included.
  - `show-options -sv` with multiple skeleton markers → all included.
  - `show-options -sv` with quoted-value lines (e.g., `@user-opt "quoted value"`) → parsed correctly (split on first whitespace captures the name).
  - `IsRestoringSet` with `@portal-restoring` absent → `false, nil`.
  - `IsRestoringSet` with `@portal-restoring = 1` → `true, nil`.
  - `IsRestoringSet` with `@portal-restoring = ""` (empty value) → `false, nil` (empty value is not "present").
  - `IsRestoringSet` with `@portal-restoring = anything` → `true, nil`.
- Tests for `CaptureStructure` extension in `internal/state/capture_test.go`:
  - Skip set empty and prev nil → identical behaviour to current task 2-8.
  - Skip set contains `"work__0.0"` and prev contains that pane's old entry → fresh idx contains the old entry merged in (verify scrollback_file path preserved).
  - Skip set contains a pane whose session is missing from the fresh capture (the user closed the session but the marker stuck) → merge skips that entry. Actually: per spec, markers are volatile (server-option scope). If the server restarted, markers are gone. If the session was killed without server restart, the marker remains but the pane is gone — defensive behaviour: only merge if the saved session/window/pane still makes sense contextually. Simpler rule: merge regardless — if a pane marked as skeleton disappears from live tmux, its old entry in prev is copied into the new idx, and the next tick (with marker still present, but the pane still absent) keeps copying. GC (task 2-10) sees the entry as referenced and preserves the `.bin` file. When the server restarts, the marker clears and the next capture drops the entry naturally. Adopt this simpler rule.
  - Marker cleared mid-cycle: if the marker is cleared between `ListSkeletonMarkers` (start of cycle) and `CaptureStructure` (during cycle), the cycle still treats it as skipped (uses the start-of-cycle snapshot). Next cycle picks up the cleared state and captures normally. Spec: "marker cleared mid-cycle → next cycle captures."
- Do NOT integrate with the tick loop here — task 2-12 composes marker listing + capture + scrollback skip + commit into the full tick.

**Acceptance Criteria**:
- [ ] `ListSkeletonMarkers` uses a single `show-options -sv` call per invocation.
- [ ] `ListSkeletonMarkers` filters for keys prefixed with `@portal-skeleton-` and extracts the paneKey suffix.
- [ ] `ListSkeletonMarkers` handles empty output, unrelated `@` options, and non-`1` marker values.
- [ ] `IsRestoringSet` returns `true` only when `@portal-restoring` is set to a non-empty value.
- [ ] `IsRestoringSet` returns `false` when the option is absent or empty-valued.
- [ ] `CaptureStructure` with a skip set preserves prior `sessions.json` entries for skipped panes.
- [ ] Skip-set-marked panes' scrollback `.bin` files are not overwritten (verified end-to-end in task 2-12; this task asserts the skip-set propagation).
- [ ] Marker cleared mid-cycle → current cycle uses the start-of-cycle snapshot; next cycle captures normally.

**Tests**:
- `"it returns an empty set for empty show-options -sv output"`
- `"it ignores unrelated @ options"`
- `"it extracts paneKeys from @portal-skeleton-<key> entries"`
- `"it treats any non-empty marker value as present"`
- `"it returns multiple paneKeys when multiple skeleton markers are set"`
- `"it parses show-options lines with quoted values correctly"`
- `"it returns false from IsRestoringSet when @portal-restoring is absent"`
- `"it returns true from IsRestoringSet when @portal-restoring is set"`
- `"it returns false from IsRestoringSet when @portal-restoring has an empty value"`
- `"it preserves prior Index entries for panes in the skip set"`
- `"it merges a skipped pane's session and window data from prev when missing from fresh capture"`
- `"it leaves the fresh capture unchanged when the skip set is empty"`
- `"it leaves the fresh capture unchanged when prev is nil"`
- `"it uses the start-of-cycle marker snapshot for the whole cycle"`

**Edge Cases**:
- `show-options -sv` empty output — empty set. Handles the first-ever capture before any skeleton has been restored.
- Unrelated `@` options — user-defined options on the server (e.g., plugin markers) do not match the `@portal-skeleton-` prefix and are ignored.
- Marker values other than `1` — treated as present. tmux stores server options as strings; Portal sets `1` but defensive parsing accepts any non-empty value.
- Skeleton-marked pane whose `.bin` file is missing — merge copies the prior entry anyway; the next attach's hydrate helper will see the missing file and go down the "file missing" path (Phase 3). GC (task 2-10) preserves the referenced path; if nothing ever writes the file, it stays missing indefinitely, but nothing tries to read it except a hydrate helper that tolerates absence.
- Skeleton-marked pane's session disappeared (user ran `kill-session` on the saved session before hydrating) — merge still copies the entry. The entry is orphan structural data; the next server restart clears the marker and a clean capture drops the entry. Benign.
- Marker cleared mid-cycle — the cycle's snapshot (taken at `ListSkeletonMarkers`) is authoritative for the rest of the cycle. On the next cycle, the cleared marker means the pane is captured fresh; the old `.bin` file (under the saved paneKey, possibly different from live paneKey if base-index drifted) is GC'd because the live paneKey entry is the only one referenced.
- tmux lock during `show-options -sv` — the call returns an error, which propagates from `ListSkeletonMarkers`. Task 2-12's tick loop logs + skips the cycle on this rare transient.
- `@portal-restoring` and `@portal-skeleton-*` coexist — the two predicates are independent. Task 2-12 honours both: `@portal-restoring` skips the entire tick; `@portal-skeleton-*` skips per-pane within a tick.

**Context**:
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>`": "**Enumeration mechanism:** per capture cycle, the daemon runs a single `tmux show-options -sv` to dump all server-scope options, and filters in memory for keys prefixed with `@portal-skeleton-`. This produces the set of marker-bearing paneKeys in O(1) tmux invocations per cycle, regardless of pane count. During `list-panes` enumeration, the daemon checks each pane's computed paneKey against the filtered set; marker-present panes are skipped. Avoids N per-pane `show-option` calls."
>
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>` → Effect on save": "the daemon's capture loop **skips** panes whose marker is set. Neither the scrollback file nor the pane's `sessions.json` entry is updated. Disk file preserved."
>
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-restoring`": "**Effect on save:** the hosted daemon's tick loop honours this marker (skip the entire tick if set). `portal state notify` itself is unaware of the marker — it always touches the dirty flag, including during restore; the daemon's entry-check is the single suppression point."
>
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>` → User-created panes never receive this marker": "Brand-new post-boot panes are captured normally from the start." Good cross-check: our skip set must never contain a paneKey for a user-created pane.
>
> Spec — Phase 2 task 2-11 edge cases: "`show-options -sv` empty output, unrelated `@` options present, marker values other than `1` still treated as present, skeleton-marked pane's `.bin` and `sessions.json` entry preserved, marker cleared mid-cycle → next cycle captures."
>
> Existing pattern: `GetServerOption` / `SetServerOption` in `internal/tmux/tmux.go` use `-sv`. `ShowAllServerOptions` is the plural form (`show-options -sv` with no name argument dumps every option).

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Marker Coordination".

## built-in-session-resurrection-2-12 | approved

### Task 2-12: Ticker trigger logic, defensive startup clear, and shutdown final flush

**Problem**: With capture, commit, marker-awareness, and daemon scaffolding all in place (tasks 2-7 through 2-11), the final step is to compose them into the actual tick cycle the spec pins: 1-second ticker; per tick, check `@portal-restoring` (skip entire tick if set), check `save.requested` presence OR 30-second max-gap, and run a capture-and-commit cycle if either is true. Clear `save.requested` after a successful capture so subsequent ticks do not re-fire. SIGHUP / SIGTERM final flush runs the same capture-and-commit (skipped if `@portal-restoring` is set). Defensive startup clear of `save.requested` (from task 2-7) prevents a stale flag from firing an immediate capture during a mid-restore window. Race: a `notify` fires between the daemon's clear-save.requested step and its next tick — the tick picks it up on the following iteration. This task wires the whole tick state machine and its shutdown counterpart, replacing task 2-7's placeholder `daemonRun`.

**Solution**: Replace the `daemonRun(ctx, deps)` stub from task 2-7 with the real tick loop. State: `lastSaveAt time.Time` (zero-valued initially), `hashMap state.HashMap` (seeded via `SeedHashMap` at startup), `prevIndex *state.Index` (nil initially; decoded from `sessions.json` on first successful read). Per tick: (1) check `IsRestoringSet(c)` → skip; (2) check dirty (`save.requested` exists) OR `time.Since(lastSaveAt) >= 30s`; (3) if either → run `captureAndCommit()`. Per `captureAndCommit`: (a) list skeleton markers; (b) run `CaptureStructure` with skip set and prev index; (c) for each non-skipped pane, `CaptureAndHashPane` + `WriteScrollbackIfChanged`; aggregate `anyScrollbackChanged`; (d) `Commit(dir, idx, anyScrollbackChanged, logger)`; (e) update `lastSaveAt = time.Now()`, `prevIndex = &idx`, remove `save.requested`. On shutdown (`ctx.Done()`): re-check `IsRestoringSet`; if false, run `captureAndCommit()` as a final flush; else skip. Every per-pane error is logged + continues (per "degrade locally, log, continue"). A per-tick error (rare — show-options failure, say) logs + skips the cycle; lastSaveAt is NOT advanced, so next tick retries.

**Outcome**: The daemon runs indefinitely until SIGHUP/SIGTERM. Structural events set `save.requested` via tmux hooks → daemon's next tick (≤ 1s) captures and commits. 30-second idle periods fire a capture (to catch scrollback drift). `@portal-restoring` suppresses captures during bootstrap's skeleton-restore window. Skeleton-marked panes' files are preserved. SIGHUP at shutdown flushes a final state unless restore is in progress. Dirty flag races (notify arrives between `os.Remove(save.requested)` and the next tick) are picked up on the following tick.

**Do**:
- Edit `cmd/state_daemon.go` to replace `daemonRun` and `shutdownFlush` stubs from task 2-7:
  - Extend `daemonDeps` struct with:
    ```go
    type daemonDeps struct {
        StateDir    string
        Logger      *state.Logger
        Client      *tmux.Client
        HashMap     state.HashMap
        PrevIndex   *state.Index
        LastSaveAt  time.Time
        TickerPeriod time.Duration   // defaults 1 * time.Second; overridable for tests
        MaxGap       time.Duration   // defaults 30 * time.Second
    }
    ```
  - At startup (after writing pid/version and clearing save.requested):
    1. `deps.HashMap = state.SeedHashMap(deps.StateDir, deps.Logger)`.
    2. Attempt to load prior `sessions.json`: `data, err := os.ReadFile(state.SessionsJSON(deps.StateDir))`. If success, `idx, derr := state.DecodeIndex(data)`. If `derr == nil`, `deps.PrevIndex = &idx`. Otherwise `deps.PrevIndex = nil` (first run or corrupt).
    3. If `deps.TickerPeriod == 0`: default `1 * time.Second`. If `deps.MaxGap == 0`: default `30 * time.Second`.
  - Replace `daemonRun(ctx, deps)`:
    ```go
    func daemonRun(ctx context.Context, deps *daemonDeps) error {
        ticker := time.NewTicker(deps.TickerPeriod)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                if restoring, err := state.IsRestoringSet(deps.Client); err != nil {
                    deps.Logger.Warn("daemon", "tick: read @portal-restoring: %v", err)
                    continue
                } else if restoring {
                    continue
                }
                dirty := fileExists(state.SaveRequested(deps.StateDir))
                gap := time.Since(deps.LastSaveAt) >= deps.MaxGap
                if !dirty && !gap {
                    continue
                }
                if err := captureAndCommit(deps); err != nil {
                    deps.Logger.Warn("daemon", "tick: capture-and-commit: %v", err)
                    // lastSaveAt not advanced; next tick retries.
                    continue
                }
                deps.LastSaveAt = time.Now()
                _ = os.Remove(state.SaveRequested(deps.StateDir))  // tolerate ENOENT
            case <-ctx.Done():
                return shutdownFlush(deps)
            }
        }
    }
    ```
  - Replace `shutdownFlush(deps)`:
    ```go
    func shutdownFlush(deps *daemonDeps) error {
        restoring, err := state.IsRestoringSet(deps.Client)
        if err != nil {
            deps.Logger.Warn("daemon", "shutdown: read @portal-restoring: %v", err)
            // conservative: do NOT flush; spec wants flush skipped if restore in progress and a check error is ambiguous.
            return nil
        }
        if restoring {
            deps.Logger.Info("daemon", "shutdown: skipping final flush (@portal-restoring set)")
            return nil
        }
        if err := captureAndCommit(deps); err != nil {
            deps.Logger.Warn("daemon", "shutdown: final flush failed: %v", err)
        }
        return nil
    }
    ```
  - New helper `func captureAndCommit(deps *daemonDeps) error`:
    1. `skipSet, err := state.ListSkeletonMarkers(deps.Client)`. Propagate error.
    2. `idx, err := state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex)`. Propagate error.
    3. `anyScrollbackChanged := false`.
    4. For each session / window / pane in `idx.Sessions`: compute paneKey; if in `skipSet`, continue (scrollback write skipped, file preserved, hash map untouched). Otherwise `tmuxTarget := fmt.Sprintf("%s:%d.%d", session.Name, window.Index, pane.Index)` — build the tmux `-t` argument from raw session + indices.
    5. `data, hash, err := state.CaptureAndHashPane(deps.Client, tmuxTarget)`. On error: log per-pane warning, continue (do not fail the whole cycle for one pane).
    6. `written, err := state.WriteScrollbackIfChanged(deps.StateDir, paneKey, data, hash, deps.HashMap)`. On error: log per-pane warning, continue.
    7. If `written`: `anyScrollbackChanged = true`.
    8. `if err := state.Commit(deps.StateDir, idx, anyScrollbackChanged, deps.Logger); err != nil { return err }`.
    9. `deps.PrevIndex = &idx`.
    10. Return nil.
  - `fileExists(path string) bool`: `_, err := os.Stat(path); return err == nil`.
- Tests in `cmd/state_daemon_run_test.go` (or extend `cmd/state_daemon_test.go`) using dependency injection on `daemonDeps`:
  - Dirty-flag race: start with `dirty=false`, `lastSaveAt=now`. Tick: no capture. Inject `save.requested` creation then tick again: capture fires, save.requested removed. Simulate another notify arriving immediately after remove: on the *following* tick the file is present again and fires another capture. Picks up the race.
  - 30s max-gap: set `lastSaveAt = now - 31s`, no dirty flag: next tick captures and advances `lastSaveAt`.
  - `@portal-restoring` set mid-tick: inject a Commander that returns `@portal-restoring=1` on `show-option -sv @portal-restoring`. Tick skips entirely — verify no `list-panes`, no `capture-pane` calls.
  - Dirty flag set during restore (a notify fires while `@portal-restoring` is set): tick skips, `save.requested` persists. After `@portal-restoring` clears, next tick picks it up.
  - `@portal-restoring` set at shutdown: `shutdownFlush` skips capture. Verify no commit call.
  - `@portal-restoring` clear at shutdown: `shutdownFlush` runs `captureAndCommit`. Verify commit fires.
  - `show-option -sv @portal-restoring` errors at shutdown: conservative — skip flush, log warn. Verify no commit, one warn line.
  - In-flight capture started before `@portal-restoring` set: simulate a capture cycle entering `CaptureStructure`; mid-execution (in a test, this is hard to simulate; approximate by running `captureAndCommit` to completion against a pre-restore mock state and verifying the commit goes through atomically). Spec: "a capture that started before the flag was set was capturing pre-restore (steady-state) tmux, which is a valid snapshot. The next tick will see `@portal-restoring=1` at entry and skip."
  - Per-pane capture error: one pane's `capture-pane` returns an error; other panes still captured + written; commit fires; log contains one per-pane warning.
  - `ListSkeletonMarkers` error: tick logs warn, skips cycle, lastSaveAt not advanced.
  - `Commit` error (disk full): tick logs warn, skips advancement of lastSaveAt, save.requested not removed — next tick retries.
  - Zero live sessions: `captureAndCommit` runs, `Commit` with `anyScrollbackChanged=false` and structurally-identical idx (empty) is a no-op on a zero-change cycle.
  - Seed hash map is populated from disk at startup — use a pre-existing `.bin` file and verify it is in `deps.HashMap` before the first tick.
- Wire the `PersistentPreRunE` bootstrap composition:
  - Bootstrap order (per spec step 3): `SetServerOption("@portal-restoring", "1")` → task 2-6 (`EnsurePortalSaverVersion`) → Phase 3's `Restore()` (stub OK for now) → `DeleteServerOption("@portal-restoring")` → task 2-12's daemon is already running via task 2-7 / 2-5.
  - For Phase 2, the `@portal-restoring` wiring is only half-implemented (full flow lands in Phase 3 alongside the Restore step). This task should verify the daemon's *reading* of `@portal-restoring` is correct; the *setting* of it is Phase 3's concern.
- Do NOT land the `Restore()` body here — Phase 3 owns that. But do verify that when `@portal-restoring` is set (however it got set), the tick skips correctly.

**Acceptance Criteria**:
- [ ] The tick loop runs every `TickerPeriod` (default 1s).
- [ ] On each tick, `IsRestoringSet` is checked; if true, the tick is skipped entirely (no capture, no scrollback read, no commit).
- [ ] On each tick, `save.requested` presence OR 30-second max-gap triggers a capture.
- [ ] Successful capture updates `lastSaveAt` and removes `save.requested`.
- [ ] `save.requested` race between clear and next notify is picked up on the following tick.
- [ ] Skeleton-marked panes are skipped in the scrollback write loop; their prior entries are merged into the new idx.
- [ ] Per-pane capture errors log and continue; the cycle still commits.
- [ ] Per-tick errors (show-options fail, capture-structure fail, commit fail) log and skip the cycle; `lastSaveAt` is not advanced; `save.requested` is not removed.
- [ ] SIGHUP or SIGTERM cancels the context; `shutdownFlush` runs.
- [ ] `shutdownFlush` skips the final capture when `@portal-restoring` is set.
- [ ] `shutdownFlush` runs a final capture otherwise.
- [ ] A read error on `@portal-restoring` at shutdown conservatively skips the flush.
- [ ] In-flight capture started before `@portal-restoring` is set completes normally and commits its result.
- [ ] Hash map is seeded from disk at startup before the first tick.
- [ ] `prevIndex` is loaded from `sessions.json` at startup; absent / corrupt file → `nil` prev.

**Tests**:
- `"it fires a capture when save.requested is present"`
- `"it fires a capture after the 30-second max-gap even without dirty flag"`
- `"it does not fire a capture when neither dirty nor gap"`
- `"it skips the entire tick when @portal-restoring is set"`
- `"it preserves save.requested when the tick is suppressed by @portal-restoring"`
- `"it removes save.requested after a successful capture"`
- `"it does not remove save.requested when the cycle errors"`
- `"it picks up a notify arriving between the clear and the next tick"`
- `"it skips skeleton-marked panes in the scrollback write loop"`
- `"it merges prior index entries for skeleton-marked panes into the new index"`
- `"it continues the cycle after a per-pane capture error"`
- `"it logs and skips the cycle on show-options or capture-structure error"`
- `"it logs and skips the cycle on commit error without advancing lastSaveAt"`
- `"it flushes a final capture on SIGHUP when @portal-restoring is not set"`
- `"it skips the final flush on SIGHUP when @portal-restoring is set"`
- `"it skips the final flush on SIGTERM when @portal-restoring is set"`
- `"it flushes a final capture on SIGTERM when @portal-restoring is not set"`
- `"it conservatively skips the final flush when @portal-restoring read errors"`
- `"it completes an in-flight capture normally when @portal-restoring flips mid-capture"`
- `"it seeds the hash map from disk before the first tick"`
- `"it loads prevIndex from sessions.json at startup"`
- `"it handles a missing sessions.json at startup as prevIndex=nil"`

**Edge Cases**:
- Dirty flag set during restore — `notify` writes `save.requested` unconditionally, including during bootstrap. The daemon's tick sees `@portal-restoring=1` at entry → skip. `save.requested` persists on disk. When `@portal-restoring` clears (bootstrap step 6), the next tick picks up the file and captures.
- 30-second max-gap with zero dirty signals — happens when a user leaves tmux idle (no structural events, slow scrollback drift). Every 30 seconds, a capture runs. `lastSaveAt` advances.
- `@portal-restoring` set at shutdown — skip flush. Saves are bounded by the last successful tick; worst-case data loss is ≤ the capture interval.
- `@portal-restoring` set mid-capture — the in-flight capture runs to completion with pre-restore state and commits via atomic rename. The next tick sees the flag set and skips. No corruption; no mid-transition capture.
- `save.requested` race — between the tick's remove step and the next tick, another `notify` can fire. The file is present on the next tick and triggers another capture. No coalescing required — spec says "Natural coalescing. Five events firing in 100ms all just set the flag; the next tick does exactly one save."
- Per-pane capture error — `capture-pane` can fail if the pane was closed between `list-panes` (in `CaptureStructure`) and `capture-pane`. Log warn, continue; the structural data is already captured, the scrollback for that pane stays at the prior version (prev hash still in map, no write fires).
- Commit error (disk full) — the daemon does not crash. Logs warn, retries on next tick. Spec: "Disk full during save: `AtomicWrite` fails at write or rename step. Daemon logs the error, continues ticking, and retries on the next tick."
- Shutdown during mid-tick capture — the tick's `select` re-evaluates `case <-ctx.Done()` only between select iterations; mid-`captureAndCommit`, the shutdown waits for the capture to complete. Acceptable — a tick's cost is bounded (~100ms–1s for heavy configs) and shutdown wait is similarly bounded.
- `fileExists(save.requested)` race between the stat and the next syscall — irrelevant; the presence check is a hint, not a lock. Either outcome is correct.
- Hash-map seed with unreadable file — task 2-9's `SeedHashMap` logs warn and skips; the tick eventually overwrites the unreadable file on first capture (and likely fails again with the same permission error; logged per-pane).

**Context**:
> Spec "Save-Side Architecture → Daemon Tick Loop (Pseudocode)":
> ```
> for {
>     select {
>     case <-ticker.C:  // 1 second
>         if isRestoringFlagSet() {
>             continue  // skip entire tick during restore
>         }
>         if isDirty() || timeSinceLastSave() >= 30*time.Second {
>             captureAndWrite()
>             clearDirty()
>         }
>     case <-ctx.Done():  // SIGHUP or SIGTERM
>         if !isRestoringFlagSet() {
>             captureAndWrite()  // flush final state on shutdown
>         }
>         return
>     }
> }
> ```
>
> Spec "Save-Side Architecture → Triggers & Serialization → Properties": "**Single writer by construction.** Only the hosted daemon writes scrollback files and `sessions.json`. No filesystem coordination beyond the dirty flag. **Natural coalescing.** Five events firing in 100ms all just set the flag; the next tick does exactly one save. **Max-gap guarantee.** 30 seconds is the ceiling on save staleness, even during idle periods. **Event latency.** ≤1 second from tmux event to save completion (bounded by the ticker interval). **Restoration guard.** The daemon's tick checks `@portal-restoring` at the top of the cycle. While set (during the skeleton-restore window), no capture runs, regardless of dirty-flag state."
>
> Spec "Save-Side Architecture → In-Flight Capture Atomicity": "A capture cycle is a single synchronous Go function. It checks `@portal-restoring` at entry only and runs to completion without re-checking. If bootstrap flips `@portal-restoring` from `0` to `1` while a capture is mid-execution, the in-flight capture completes normally and may commit its write after the flag is set. This is safe because: (a) A capture that started before the flag was set was capturing pre-restore (steady-state) tmux, which is a valid snapshot. (b) Writes are atomic (per-file `AtomicWrite`) and commit via the `sessions.json` rename — so the committed state is a coherent pre-restore snapshot. (c) The next tick will see `@portal-restoring=1` at entry and skip, so no subsequent capture interferes with the skeleton-build. No per-tick locking is required."
>
> Spec "Save-Side Architecture → Triggers & Serialization → Defensive Dirty-Flag Clear on Daemon Startup": "On daemon startup, the first action is to clear `save.requested` if present." — already wired in task 2-7.
>
> Spec "Save-Side Architecture → Signal Handling → Handler behavior": "1. If the `@portal-restoring` marker is set, skip the final flush (an in-progress restore is underway; capturing now would capture mid-transition state). 2. Otherwise, flush the current state atomically via `AtomicWrite` and exit."
>
> Spec "Failure Modes & Recovery → Disk full during save": "Daemon logs the error, continues ticking, and retries on the next tick (or on the next dirty-flag set). Previous save state remains intact on disk. When disk space frees, save resumes normally. Daemon never crashes from disk-full alone."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Save-Side Architecture → Daemon Tick Loop", "Save-Side Architecture → Triggers & Serialization", "Save-Side Architecture → In-Flight Capture Atomicity", "Save-Side Architecture → Signal Handling", "Failure Modes & Recovery → Disk full during save".

