---
phase: 3
phase_name: Token-Ack Confirmation, Pre-flight & Partial-Failure Handling
total: 7
---

## restore-host-terminal-windows-3-1 | approved

### Task 3.1: Option-safe batch/token ids + `@portal-spawn` marker-name derivation

**Problem**: The token-ack channel keys each spawned window's confirmation on a namespaced tmux **server option** named `@portal-spawn-<batch>-<token>`. The `<batch>` and `<token>` must be **option-name-safe** ids — deliberately *not* the renameable session name, which can carry characters (`.`, `:`, spaces, and `-` introduced by an external `rename-session` or the in-TUI `r` modal) that either make `set-option` fail (→ no marker → a false ack-timeout) or make the marker name ambiguous to parse. Nothing in `internal/spawn` yet produces these ids or the marker-name string, and the whole ack channel (Task 3.2), the `portal attach --spawn-ack` write (Task 3.3), and the burst's flag composition (Task 3.5) all depend on **every site deriving the marker name by one identical rule**.

**Solution**: Add `internal/spawn/ackid.go` — the single home for the ack-id vocabulary: an option-safe id generator that reuses Portal's existing hyphen-free nanoid alphabet, the `@portal-spawn-` prefix constant, a marker-name formatter and its round-trip parser (unambiguous because the ids never contain the `-` delimiter), and the `<batch>:<token>` flag-value formatter/parser used by the `--spawn-ack` carrier. Every downstream site (attach write, burst compose, ack collect/clean) routes through these functions so the produce-and-consume rule can never drift.

**Outcome**: `internal/spawn` exposes `NewSpawnID(gen)` (independent, collision-resistant, option-safe ids; a generator error propagates and never yields an empty/malformed id), `SpawnMarkerName(batch, token)` → `@portal-spawn-<batch>-<token>`, `ParseSpawnMarkerName(name)` round-tripping back to `(batch, token, true)` and rejecting foreign names, and `FormatSpawnAckFlag`/`ParseSpawnAckFlag` for the `<batch>:<token>` flag value. All logic is unit-tested with fabricated generators and fixed strings; no OS, tmux, or crypto assertions on the random output beyond charset/non-empty.

**Do**:
- Create `internal/spawn/ackid.go`:
  - `const SpawnMarkerPrefix = "@portal-spawn-"` — the namespace prefix (distinct from `internal/state`'s `SkeletonMarkerPrefix = "@portal-skeleton-"`; Task 3.2 proves the two enumerators are blind to each other's prefix).
  - `const spawnIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"` — the option-safe charset (identical to `session`'s nanoid alphabet: no `.`, no `:`, no space, **no `-`**). The absence of `-` is load-bearing: it makes `<batch>-<token>` split on the single delimiter unambiguously.
  - `func NewSpawnID(gen func() (string, error)) (string, error)` — call `gen()`; on error `return "", fmt.Errorf("spawn: generate ack id: %w", err)` (propagate — never swallow to an empty id). On success, defensively verify the result is non-empty **and** every rune is in `spawnIDAlphabet` via `isOptionSafeID`; if not, `return "", fmt.Errorf("spawn: generated ack id %q is not option-safe", id)`. Production callers pass `session.NewNanoIDGenerator()` (6-char crypto/rand alphanumerics). Batch and per-window tokens are each an **independent** `NewSpawnID` call (independent → collision-resistant across windows and batches).
  - `func isOptionSafeID(s string) bool` — `s != "" && strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune(spawnIDAlphabet, r) }) < 0`.
  - `func SpawnMarkerName(batch, token string) string { return SpawnMarkerPrefix + batch + "-" + token }`.
  - `func ParseSpawnMarkerName(name string) (batch, token string, ok bool)` — `rest, found := strings.CutPrefix(name, SpawnMarkerPrefix)`; if `!found` ⇒ `("", "", false)`. Then `b, t, ok2 := strings.Cut(rest, "-")`; if `!ok2 || b == "" || t == ""` ⇒ `("", "", false)`; else `(b, t, true)`. (Because ids are hyphen-free, the first `-` after the prefix is the sole, unambiguous delimiter.)
  - `func FormatSpawnAckFlag(batch, token string) string { return batch + ":" + token }` — the `--spawn-ack` value form (colon-delimited; ids are colon-free so this is unambiguous too).
  - `func ParseSpawnAckFlag(value string) (batch, token string, ok bool)` — `b, t, found := strings.Cut(value, ":")`; `ok = found && b != "" && t != ""`.
- Add `internal/spawn/ackid_test.go` (unit lane, `package spawn`): use a deterministic fake generator (e.g. a closure returning a queued slice of ids, and one returning an error) plus fixed-string round-trip assertions. Do **not** assert on the random content of the real nanoid — only that `NewSpawnID` with the real generator yields a non-empty option-safe id.

**Acceptance Criteria**:
- [ ] `NewSpawnID` with a generator returning `"b1abcd"` yields `("b1abcd", nil)`; with a generator returning an error, yields `("", err)` where the error wraps the generator error and the id is empty (never a partial/blank id).
- [ ] `NewSpawnID` with a generator returning a value containing `-`, `.`, `:`, or a space returns a non-nil "not option-safe" error and an empty id.
- [ ] `SpawnMarkerName("b1abcd", "t2wxyz") == "@portal-spawn-b1abcd-t2wxyz"`.
- [ ] `ParseSpawnMarkerName("@portal-spawn-b1abcd-t2wxyz")` returns `("b1abcd", "t2wxyz", true)`.
- [ ] `ParseSpawnMarkerName("@portal-skeleton-foo")` returns `ok == false` (foreign prefix), and `ParseSpawnMarkerName("@portal-spawn-onlyonepart")` returns `ok == false` (no delimiter).
- [ ] `ParseSpawnAckFlag("b1abcd:t2wxyz")` returns `("b1abcd", "t2wxyz", true)`; `ParseSpawnAckFlag("nocolon")`, `ParseSpawnAckFlag(":t")`, and `ParseSpawnAckFlag("b:")` all return `ok == false`.
- [ ] Two independent `NewSpawnID` calls with the real `session.NewNanoIDGenerator()` produce two non-empty, option-safe strings (independence: the function does not reuse a cached id).

**Tests**:
- `"it generates a non-empty option-safe id and propagates a generator error to an empty id"`
- `"it rejects a generated id containing a hyphen, dot, colon or space"`
- `"it formats and round-trips a marker name to (batch, token)"`
- `"it rejects a foreign-prefixed or delimiter-less marker name on parse"`
- `"it formats and round-trips the <batch>:<token> ack flag value"`
- `"it rejects a flag value with a missing colon or an empty batch or token"`

**Edge Cases**:
- Ids restricted to the tmux-option-name-safe charset (hyphen-free) — this is *why* the renameable session name is rejected: it can carry `set-option`-invalid characters and the hyphen delimiter would become ambiguous.
- Batch and token are independent generator calls (collision-resistant across windows and across batches).
- The marker name round-trips to `(batch, token)` with an unambiguous single-`-` delimiter (guaranteed by the hyphen-free id charset).
- A generator error propagates; the function never returns an empty or malformed id as if it were valid.

**Context**:
> Spec *Burst & Partial-Failure Contract → Ack channel*: "A namespaced **`@portal-spawn-<batch>-<token>` tmux server option** — where `<batch>` and `<token>` are picker-generated **option-name-safe ids** (nanoid-style), deliberately **not** the renameable session name (a session name can contain characters invalid in a tmux option name, which would make `set-option` fail → no marker → a false ack-timeout)."
> Spec *Ack delivery & `portal attach` contract*: "The `<batch>` and `<token>` are picker-generated option-name-safe ids; the flag puts `attach` in spawn-ack mode and tells it exactly which marker to write (no derivation from the session name)."
> The nanoid generator to reuse is `session.NewNanoIDGenerator()` (`internal/session/naming.go`) — a `crypto/rand` 6-char alphanumeric generator over exactly the hyphen-free alphabet used here; reusing its alphabet (rather than its generator directly at every site) keeps the id vocabulary in one place while still letting production pass the real generator into `NewSpawnID`.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Confirmation mechanism: explicit token ack / Ack channel*; *Ack delivery & `portal attach` contract*.

---

## restore-host-terminal-windows-3-2 | approved

### Task 3.2: `@portal-spawn` ack channel seam — write / collect / clean over tmux server options

**Problem**: The picker/CLI must (a) let each spawned `portal attach` **write** its `@portal-spawn-<batch>-<token>` marker, (b) **collect** exactly the markers of *its* batch to decide which windows confirmed, and (c) **clean** its batch's markers on every terminal path — all over tmux server options, and all behind a small seam so the burst pipeline is unit-testable without a real tmux server. The collection must return only the given batch's tokens and must ignore both foreign batches and every `@portal-skeleton-` marker; symmetrically, the existing skeleton enumerator (`state.ListSkeletonMarkers`) must remain blind to `@portal-spawn-` markers. Nothing implements this channel yet.

**Solution**: Add `internal/spawn/ack.go` with `ServerOptionAckChannel` over two spawn-local server-option seams (implicitly satisfied by `*tmux.Client`): `Write` sets the marker, `Collect` enumerates all server options once and returns the target batch's token set (filtered by `SpawnMarkerPrefix` + batch, parsed via Task 3.1), and `Clean` enumerates and unsets every marker for the batch (idempotent — tmux user-option unset is exit-0 even when absent). Add `internal/spawntest/ack.go` with an in-memory `FakeAckChannel` for the pipeline tests, and prove the two-way prefix isolation against `state.ListSkeletonMarkers` in a unit test.

**Outcome**: `spawn.NewServerOptionAckChannel(w, l)` returns a channel whose `Collect(batch)` yields only that batch's tokens (foreign-batch and `@portal-skeleton-` markers excluded), whose `Write(batch, token)` sets `@portal-spawn-<batch>-<token>=1`, and whose `Clean(batch)` unsets every one of that batch's markers idempotently. An enumeration/read failure surfaces as an error from `Collect` (never a false-empty set). `spawntest.FakeAckChannel` satisfies the consumer interfaces for the pipeline tests. Unit tests cover the isolation both directions with a crafted `show-options` string; an integration-tagged real-tmux test round-trips real markers.

**Do**:
- Create `internal/spawn/ack.go`:
  - Define spawn-local seams (mirroring Phase 1's spawn-local `clientLister`, so `internal/spawn` need not import `internal/state` for these): `type serverOptionWriter interface { SetServerOption(name, value string) error; UnsetServerOption(name string) error }` and `type serverOptionLister interface { ShowAllServerOptions() (string, error) }`. `*tmux.Client` satisfies both implicitly.
  - `type ServerOptionAckChannel struct { w serverOptionWriter; l serverOptionLister }` and `func NewServerOptionAckChannel(w serverOptionWriter, l serverOptionLister) *ServerOptionAckChannel`.
  - `func (c *ServerOptionAckChannel) Write(batch, token string) error` — `return c.w.SetServerOption(SpawnMarkerName(batch, token), "1")` (value opaque; *presence* is the signal, per spec).
  - `func (c *ServerOptionAckChannel) Collect(batch string) (map[string]struct{}, error)` — call `c.l.ShowAllServerOptions()`; **on error return `(nil, err)`** (never a partial/empty-as-success set). Parse each line as tmux's `@name value` form (same shape `state.ListSkeletonMarkers` parses: split on the first space/tab, take the name); for each name call `ParseSpawnMarkerName(name)`; keep the token only when `ok && parsedBatch == batch`. Names that fail `ParseSpawnMarkerName` (including every `@portal-skeleton-…` name — wrong prefix) are silently skipped. Return the token set (empty non-nil map when the batch has none).
  - `func (c *ServerOptionAckChannel) Clean(batch string) error` — enumerate via `ShowAllServerOptions`; for every line whose name parses to this `batch`, call `c.w.UnsetServerOption(name)`. Idempotent by construction (only present markers are enumerated) and robust to a concurrent unset because tmux `set-option -su @<absent-user-option>` returns exit 0 (verified against tmux 3.6b). Collect per-marker unset errors but continue the loop; return the first non-nil unset error (or the enumeration error) so a genuine failure is observable, while a `Clean` on a batch with zero markers is a nil-return no-op. Callers treat `Clean` as **best-effort** (the CLI swallows its error — bounded, harmless leaks self-expire with the server).
  - Define the narrow **consumer** interfaces where they are used (Go idiom), or export them here for reuse: `type AckCollector interface { Collect(batch string) (map[string]struct{}, error) }`, `type AckCleaner interface { Clean(batch string) error }`, `type AckWriter interface { Write(batch, token string) error }`, and the combined `type AckChannelFull interface { AckCollector; AckCleaner }` — the `Collect`+`Clean` seam the burst orchestrators depend on (`SpawnDeps.Ack` in task 3-5 and `tui.Deps.AckChannel` in task 6-3 both reference `spawn.AckChannelFull`). `*ServerOptionAckChannel` satisfies all four; `spawntest.FakeAckChannel` satisfies `AckChannelFull`.
- Create `internal/spawntest/ack.go` (test-only package, mirroring `spawntest.FakeAdapter`):
  - `type FakeAckChannel struct { mu sync.Mutex; store map[string]map[string]struct{}; Cleaned []string }` — an in-memory batch→token-set store.
  - `Write(batch, token string) error` records the token under the batch. `Collect(batch string) (map[string]struct{}, error)` returns a copy of the batch's set (empty non-nil when none); expose a `FailCollect error` field to script the enumeration-failure case. `Clean(batch string) error` appends `batch` to `Cleaned` and deletes the batch's set. Add a test helper `Ack(batch, token string)` (alias of `Write`) so a test can seed "this token arrived."
- Add `internal/spawn/ack_test.go` (unit lane, `package spawn`):
  - Drive `Collect`/`Clean` with a fake `serverOptionLister`/`serverOptionWriter` whose `ShowAllServerOptions` returns a crafted multi-line string mixing: two markers of batch `b1`, one marker of batch `b2`, and two `@portal-skeleton-…` markers. Assert `Collect("b1")` returns exactly the two `b1` tokens; assert `Clean("b1")` unset exactly the two `b1` marker names (recorded by the fake writer) and touched neither `b2` nor the skeleton markers.
  - **Two-way isolation proof (pure, no tmux):** feed the *same* crafted string to `state.ListSkeletonMarkers` (via a fake `state.ServerOptionLister`) and assert it returns only the two skeleton paneKeys and **none** of the `@portal-spawn-…` names — proving the skeleton enumerator is blind to the spawn prefix, complementing `Collect`'s blindness to the skeleton prefix.
  - Assert `Collect` returns `(nil, err)` when the lister errors (false-empty guard).
- Add `internal/spawn/ack_realtmux_test.go` (**`//go:build integration`**, real-tmux, per-test `-L` socket via `tmuxtest` — no daemon, no built binary, consistent with the unit lane's real-tmux *client* tests): set real `@portal-spawn-b1-t1` and a real `@portal-skeleton-foo` option; assert `Collect("b1")` sees `t1` only, `state.ListSkeletonMarkers` sees `foo` only, `Clean("b1")` removes the spawn marker (and a second `Clean("b1")` is a nil-return no-op — idempotency), and the skeleton marker is untouched throughout.

**Acceptance Criteria**:
- [ ] `Collect(batch)` returns **only** the given batch's tokens — foreign-batch `@portal-spawn-` markers and all `@portal-skeleton-` markers are excluded (both directions of isolation verified in one crafted-string test).
- [ ] `state.ListSkeletonMarkers` over the same crafted option dump returns only skeleton paneKeys and no `@portal-spawn-` names (proving skeleton enumeration is blind to the spawn prefix).
- [ ] `Write(batch, token)` sets the server option `@portal-spawn-<batch>-<token>` to `"1"`.
- [ ] `Clean(batch)` unsets every one of that batch's markers and leaves foreign-batch and skeleton markers intact; a `Clean` on a batch with zero present markers returns nil (idempotent — already-absent is not an error).
- [ ] A `ShowAllServerOptions` failure makes `Collect` return `(nil, err)` — never a false-empty success.
- [ ] The integration test round-trips a real marker on a real tmux server: set → `Collect` sees it → `Clean` removes it → second `Clean` is a nil no-op, with a co-resident `@portal-skeleton-` marker untouched.

**Tests**:
- `"it collects only the target batch's tokens and ignores foreign batches and skeleton markers"`
- `"it proves ListSkeletonMarkers ignores @portal-spawn markers on the same option dump"`
- `"it writes the @portal-spawn-<batch>-<token> marker set to 1"`
- `"it cleans every batch marker idempotently and leaves other markers intact"`
- `"it returns an error (not a false-empty set) when enumeration fails"`
- `"integration: it round-trips and idempotently cleans a real tmux marker alongside a skeleton marker"` (`//go:build integration`)

**Edge Cases**:
- Collect returns only the given batch's tokens, ignoring other batches **and** all `@portal-skeleton-` markers (both directions).
- `state.ListSkeletonMarkers` is proven blind to the `@portal-spawn-` prefix (the complementary isolation direction).
- Clean unsets every batch marker idempotently (already-absent is not an error — tmux user-option unset is exit-0).
- Enumerate/read failure surfaces as an error, not a false-empty Collect (which would silently mis-classify every window as failed, or every window as needing no cleanup).

**Context**:
> Spec *Burst & Partial-Failure Contract → Ack channel*: "Behind a small ack seam (write-token / collect-tokens interface). Code-verified safe: the only all-server-options enumerator, `ListSkeletonMarkers`, skips any name not prefixed `@portal-skeleton-` (`internal/state/markers.go`), so a distinct `@portal-spawn-` prefix is invisible to it; namespacing isolates sweeps in both directions; server options die with the server."
> Spec *Burst & Partial-Failure Contract → Cleanup*: "The picker self-cleans its batch markers before self-exec (and on a pre-flight abort or a reported spawn failure). Bounded, harmless leaks (a late-laggard ack, a crashed picker) self-expire with the server and never collide (unique batch ids)."
> Spec *Observability & State Footprint → State/daemon footprint*: "**Writes** only transient `@portal-spawn-*` tmux server options (self-cleaned per the ack contract — not files, not captured)." The channel is deliberately **daemon-readable** for a deferred follow-on, but this task builds only write/collect/clean.
> `state.ListSkeletonMarkers` and the server-option methods (`ShowAllServerOptions` → `@name value` lines; `SetServerOption`; `UnsetServerOption` via `set-option -su`) already exist (`internal/state/markers.go`, `internal/tmux/tmux.go`). tmux user-option unset was verified exit-0 when absent, so `Clean` is idempotent without special-casing. Real-tmux marker tests belong to the integration lane per CLAUDE.md's lane rule; a per-`-L`-socket client test carries no daemon and no built binary.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Confirmation mechanism: explicit token ack / Ack channel / Cleanup*; *Observability & State Footprint → State/daemon footprint*.

---

## restore-host-terminal-windows-3-3 | approved

### Task 3.3: `--spawn-ack <batch>:<token>` flag on the existing `portal attach`

**Problem**: A spawned window runs `portal attach <session>`; to confirm the window actually came up, that attach must write the picker's `@portal-spawn-<batch>-<token>` marker **as its last action before the exec handoff** into tmux (once `portal` execs into tmux it is replaced and can never ack afterwards). The carrier is a `--spawn-ack <batch>:<token>` flag on the *existing* `portal attach` command (part of the composed argv, so it flows uniformly through the native adapter and, later, config recipes). `attach` has no such flag yet.

**Solution**: Add a `--spawn-ack` string flag to `cmd/attach.go`. When set, `attach` parses the `<batch>:<token>` value (malformed → usage error, exit 2), performs the existing session-exists check first, and — only when the session exists — writes the marker via the ack channel (best-effort) immediately before `Connect`. A failed write still execs; a missing session writes nothing and takes the existing no-session path. When the flag is absent, `attach` behaves exactly as today.

**Outcome**: `portal attach <s> --spawn-ack <batch>:<token>` writes `@portal-spawn-<batch>-<token>` right before it hands off to `Connect`, but only after confirming `<s>` exists; a marker-write failure does not abort the attach; a malformed flag value exits 2; a gone session writes no marker and returns the existing "No session found" error; and omitting the flag leaves attach byte-for-byte unchanged. Unit-tested by Executing the attach body with injected deps (fake connector, fake validator, fake ack writer) — no real tmux.

**Do**:
- In `cmd/attach.go`:
  - Register the flag on `attachCmd`: `attachCmd.Flags().String("spawn-ack", "", "internal: write the @portal-spawn-<batch>:<token> ack marker before attaching")` (in `init`). Keep `Args: cobra.ExactArgs(1)`.
  - Extend `AttachDeps` with `AckWriter spawn.AckWriter` (the `Write(batch, token string) error` seam from Task 3.2). Update `buildAttachDeps` so production wires `spawn.NewServerOptionAckChannel(client, client)` (the `*tmux.Client` satisfies the writer seam); when `attachDeps` is injected, use `attachDeps.AckWriter`.
  - In `RunE`, before the `HasSession` check, read and validate the flag:
    - `ackVal, _ := cmd.Flags().GetString("spawn-ack")`.
    - If `ackVal != ""`: `batch, token, ok := spawn.ParseSpawnAckFlag(ackVal)`; if `!ok` ⇒ `return NewUsageError("attach: --spawn-ack must be <batch>:<token>")` (a `*cmd.UsageError` → exit 2). (Fail fast, before touching tmux.)
  - Keep the existing order: `if !validator.HasSession(name) { return fmt.Errorf("No session found: %s", name) }` — a gone session returns here, having written **no** marker (the write is strictly after this check).
  - After the session-exists check and immediately before `return connector.Connect(name)`, when a valid `ackVal` was parsed: `if err := ackWriter.Write(batch, token); err != nil { /* best-effort: log at DEBUG under the spawn component, do NOT return */ }`. Then fall through to `Connect`. (The write is the last action before the exec handoff; a failed write just means the picker times out and classifies the window failed — safe.)
  - `buildAttachDeps` now returns the ack writer alongside the connector/validator (extend its return tuple, or return the full `*AttachDeps`), keeping the nil-injected production path intact.
- Add cases to `cmd/attach_test.go` (unit lane, `package cmd`, no `t.Parallel()`): inject `attachDeps` with a fake connector, a fake validator, and a fake `AckWriter` that records `Write` calls; set `bootstrapDeps` so `PersistentPreRunE` short-circuits (cmd `TestMain` poisons `TMUX`, so a missed tmux-seam injection fails loudly). Drive via `rootCmd` Execute with `attach <s> --spawn-ack …`.

**Acceptance Criteria**:
- [ ] `attach s1 --spawn-ack b1:t1` with an existing `s1` calls `AckWriter.Write("b1","t1")` exactly once and *then* `Connector.Connect("s1")` — write strictly before connect.
- [ ] With the ack writer scripted to return an error, `attach` still calls `Connector.Connect("s1")` (best-effort: the write failure does not abort the attach) and returns no error to the caller.
- [ ] `attach s1 --spawn-ack bogus` (no colon) and `attach s1 --spawn-ack b1:` / `--spawn-ack :t1` (empty part) return a `*cmd.UsageError` (exit 2) and never call `Write` or `Connect`.
- [ ] `attach ghost --spawn-ack b1:t1` where `HasSession("ghost")` is false returns the existing `No session found: ghost` error, calls `Write` **zero** times, and does not `Connect`.
- [ ] `attach s1` (no `--spawn-ack`) calls `Connect("s1")` and never touches the ack writer — behaviour identical to today.

**Tests**:
- `"it writes the ack marker after the session-exists check and before connect"`
- `"it still execs the attach when the marker write fails (best-effort)"`
- `"it returns a usage error (exit 2) for a malformed --spawn-ack value"`
- `"it writes no marker and takes the no-session path when the session is gone"`
- `"it leaves plain attach unchanged when --spawn-ack is absent"`

**Edge Cases**:
- Marker-write failure still execs the attach (best-effort — a failed write folds to a picker ack-timeout, which is the safe classification).
- Malformed flag value (missing colon / empty batch or token) → usage error, exit 2, no tmux touched.
- Session-not-found writes no marker and takes the existing no-session path (the write is strictly after the `HasSession` check).
- Flag absent → normal attach unchanged.

**Context**:
> Spec *Ack delivery & `portal attach` contract*: "**Carrier: a flag** `--spawn-ack <batch>:<token>` on `portal attach`, part of the composed argv … **Write point & ordering:** abridged bootstrap → confirm the session exists → **write `@portal-spawn-<batch>-<token>`** (value opaque; *presence* is the signal) → exec into tmux. The write is the last action before the exec handoff. **Best-effort:** `attach` still execs if the marker write fails … A session that fails to resolve at attach time produces **no** marker → picker timeout → failed classification."
> Spec *Reporting & exit codes*: "**Usage error** (no sessions, no `--detect`; unknown flag) → **exit `2`**." A malformed `--spawn-ack` value is the same class (a `*cmd.UsageError`, which `main.classify` maps to exit 2).
> `cmd/attach.go` today: `AttachDeps{Connector, Validator}`, `buildAttachDeps` returns real `SessionConnector` + `*tmux.Client` (which is the validator). The "abridged bootstrap" is `PersistentPreRunE`, already run before `RunE`; this task adds only the marker write between the existing `HasSession` check and `Connect`. `spawn.AckWriter`/`ParseSpawnAckFlag`/`SpawnMarkerName` come from Tasks 3.1–3.2 — attach derives the marker through the *same* vocabulary as the picker, honouring the "every site derives the key identically" invariant.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Ack delivery & `portal attach` contract*; *Spawn Architecture → Reporting & exit codes*.

---

## restore-host-terminal-windows-3-4 | approved

### Task 3.4: Pre-flight `has-session` gate in the spawn orchestrator

**Problem**: The dominant failure cause is a selected session killed between picker-load and Enter. Before opening a single window, the orchestrator must verify **every** selected session (including the trigger's self-attach target) still exists, and if any is gone, abort atomically — nothing spawns, no window opens, no self-attach — naming the gone session(s) in one line. Phase 2's pipeline spawns/self-attaches with no such gate.

**Solution**: Add a pure `spawn.PreflightMissing(sessions, exists)` helper (reusable by the Phase 6 picker) that returns the gone sessions in list order, and wire it as the first gate in `cmd/spawn.go`'s `runSpawn`: probe all sessions via an injectable `Exists` seam (production `*tmux.Client.HasSession`, which conservatively returns false on any probe error), and on any missing session return a plain error (exit 1) naming all of them, before detect/resolve/spawn/self-attach.

**Outcome**: `portal spawn s1 s2 s3` where `s2` is gone opens nothing, calls no adapter and no connector, and returns a one-line error naming `s2` (exit 1 on stderr). With multiple gone sessions, all are named in the single message. `portal spawn s1` where `s1` is gone still aborts with no self-attach. When all sessions are present, the gate is transparent and the pipeline proceeds unchanged. A probe error folds to "gone" (conservative abort). Unit-tested via a fake `Exists` and the existing fake adapter/connector, asserting zero adapter and zero connector calls on abort.

**Do**:
- Create `internal/spawn/preflight.go`:
  - `func PreflightMissing(sessions []string, exists func(name string) bool) []string` — iterate `sessions` in order; collect every `s` for which `exists(s)` is false; return the collected slice (nil/empty when all present). Pure and side-effect free.
- In `cmd/spawn.go`:
  - Extend `SpawnDeps` with `Exists func(name string) bool` (default `tmuxClient(cmd).HasSession`). `HasSession` returns `err == nil`, so a transient tmux probe fault yields false → treated as gone → conservative abort (never risks opening a window for a possibly-absent session).
  - In `runSpawn`, as the **first** step (before `Detect`, before the N≥2 unsupported gate — a gone session must abort even on an unsupported terminal): `gone := spawn.PreflightMissing(sessions, deps.Exists)` over **all** `sessions` (external + trigger). If `len(gone) > 0`:
    - Emit one `spawn` INFO/WARN outcome line naming the gone session(s) (no per-window records — nothing was attempted).
    - **Define the two shared `internal/spawn` message helpers here** (this task is their first consumer; Task 3.6's leave-what-opened message and the Phase-6 picker in Tasks 6.6/6.7 reuse them verbatim, so they live in `internal/spawn` — a new `internal/spawn/message.go` — for CLI/picker lockstep). They MUST be **exported** (capitalised), because their consumers live in other packages — `runSpawn` here and in Task 3.6 is package `cmd` (`cmd/spawn.go`), and the Phase-6 picker in Tasks 6.6/6.7 is package `tui` (`internal/tui`). An unexported `quoteJoin`/`goneVerb` in `internal/spawn` is unreachable from `cmd`/`tui` and will not compile. Follow the identical exported-cross-package pattern already used by `spawn.PreflightMissing` (this task) and `spawn.ParseSpawnAckFlag`/`spawn.SpawnMarkerName` (Tasks 3.1/3.3):
      - `func QuoteJoin(names []string) string` — single-quote each name and join with `, `: renders `'s2'` for one name and `'s2', 's4'` for several.
      - `func GoneVerb(n int) string { if n == 1 { return "is" }; return "are" }` — the count-aware verb (`"is"` for one / `"are"` for several).
    - `return fmt.Errorf("spawn: %s %s gone — nothing opened", spawn.QuoteJoin(gone), spawn.GoneVerb(len(gone)))` — so the one-line message is `spawn: 's2' is gone — nothing opened` (singular) / `spawn: 's2', 's4' are gone — nothing opened` (plural), the **same one-line message the picker shows** (spec *Reporting & exit codes*), matching the delivered design copy `⚠ '<session>' is gone — nothing opened` in the singular case. Both helpers are defined **once, here (this task)** as exported `internal/spawn` functions and reused verbatim (called as `spawn.QuoteJoin` / `spawn.GoneVerb`) by Tasks 3.6/6.6/6.7 (which must **not** re-declare them), so the CLI and picker stay in lockstep. A plain, non-`UsageError`, non-silenced error → exit 1 on stderr.
    - Do **not** call `Detect`, `Resolve`, `SpawnWindows`/the burster, or the connector.
  - When `gone` is empty, fall through to the existing pipeline (N=1 direct self-attach; N≥2 detect → resolve → unsupported gate → burst → gate). The pre-flight runs for **all** N — an N=1 batch whose sole session is gone aborts here with no self-attach.
- Extend `cmd/spawn_test.go` (unit lane): inject `Exists` returning false for named-gone sessions, a `FakeAdapter`, and a fake connector; assert `len(FakeAdapter.Calls) == 0` and the connector's `Connect` count is 0 on any abort; assert the returned error names every gone session and is not a `*cmd.UsageError`. Add a case where `Exists` returns false to model a probe fault (conservative abort).

**Acceptance Criteria**:
- [ ] `spawn s1 s2 s3` with `s2` gone returns a plain error naming `s2`, calls zero `FakeAdapter.OpenWindow` and zero `Connect`, and never reaches detect/resolve.
- [ ] Multiple gone sessions (`s2`, `s3` gone) are **all** named in the single one-line message.
- [ ] `spawn s1` (N=1) with `s1` gone aborts with the gone-session message and **no** self-attach (pre-flight is not skipped for N=1).
- [ ] All-present (`spawn s1 s2 s3`, all exist) proceeds unchanged — the gate returns empty and the pipeline runs to its normal outcome.
- [ ] A probe that returns false for a session (modelling a transient tmux fault) aborts conservatively (no window opened, no self-attach) rather than risking a false open.
- [ ] `PreflightMissing` is pure — it performs no I/O beyond calling the injected `exists`, and preserves list order in its result.

**Tests**:
- `"it aborts atomically naming the single gone session with no spawn and no self-attach"`
- `"it names every gone session in one line when several are missing"`
- `"it aborts an N=1 batch whose sole session is gone with no self-attach"`
- `"it proceeds unchanged when all sessions are present"`
- `"it aborts conservatively when a session probe fails (treats unprobeable as gone)"`
- `"PreflightMissing returns the gone sessions in list order"`

**Edge Cases**:
- Multiple gone sessions are all named in the one-line message.
- All-present proceeds unchanged (transparent gate).
- A gone session with N=1 still aborts with no self-attach (pre-flight runs for all N).
- `has-session` probe error handled conservatively — abort rather than risk a false open (`HasSession` folds any tmux error to false → gone → abort).

**Context**:
> Spec *Burst & Partial-Failure Contract → Stance: pre-flight + all-or-nothing*: "**Pre-flight validate on Enter.** Before opening a single window, verify every selected session still exists (quick `has-session` checks). … If any selected session is gone: **Abort atomically** — nothing spawns, no window opens, no self-attach. Show a clean one-line error … naming the gone session(s) (design copy: `⚠ '<session>' is gone — nothing opened`). … Zero windows opened → nothing to undo, no flash."
> Spec *Trigger-Context Matrix*: "**Selected session vanished** between picker-load and Enter: caught by the pre-flight check → atomic abort, nothing opens."
> The **selection-pruning** half of the spec's stance ("Prune the gone session(s) from the selection … keep the surviving marks … a second `Enter` proceeds with the survivors") is a **picker/TUI selection-state mutation — Phase 6**, not this CLI task. The CLI has no persistent selection to prune; it simply aborts and exits 1. `PreflightMissing` is authored here as the shared pure helper the Phase 6 picker will also call to compute what to prune.
> **Ordering choice (not pinned by the spec):** pre-flight is placed *before* detect and the N≥2 unsupported gate, so a gone session aborts with the more-actionable gone-session message even on an unsupported terminal; both paths exit 1, so the choice is cosmetic. `*tmux.Client.HasSession` (`internal/tmux/tmux.go`) returns `err == nil`, giving the conservative "probe fault → gone → abort" behaviour for free.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Stance: pre-flight + all-or-nothing*; *Trigger-Context Matrix & Open Order → Behaviour across trigger contexts (Selected session vanished)*.

---

## restore-host-terminal-windows-3-5 | approved

### Task 3.5: Token-ack self-attach gate + per-window `spawnAckTimeout`

**Problem**: Phase 2 gated the trigger's self-attach on the **adapter** returning success — but `osascript` success only confirms "the terminal accepted the request," not that the window rendered, `portal` ran, the session existed, and attach began. The gate must be upgraded to the **explicit token ack**: each spawned window's `portal attach --spawn-ack` writes its `@portal-spawn-<batch>-<token>` marker just before exec, and the orchestrator confirms each window by watching for that marker with a **per-window timeout**. Only when every external window's token is confirmed does the trigger self-attach (and clean its batch markers first). The composed argv also still lacks the `--spawn-ack` suffix (Phase 2 left a placeholder).

**Solution**: Finish the ack-flag composition in `internal/spawn/command.go` (`AttachCommand` gains `batch`/`token`, appending `--spawn-ack <batch>:<token>`), and evolve Phase 2's `SpawnWindows` into a `Burster` that generates a batch id + per-window tokens up front (Task 3.1), spawns each external window sequentially, and after each spawn **awaits that window's token** via the ack channel's `Collect` (polling until the token appears or a per-window `spawnAckTimeout` elapses — each window's timer starting at *its own* spawn). The `cmd/spawn.go` gate then self-attaches only when all windows are `confirmed`, cleaning batch markers before the exec handoff; N=1 self-attaches immediately with no ack wait.

**Outcome**: `AttachCommand` appends `--spawn-ack <batch>:<token>` to the env-self-sufficient argv. `Burster.Run(external)` returns a batch id and a per-window `WindowResult` tagged `confirmed` / `timeout` / `failed`; a token arriving late but within `spawnAckTimeout` counts as confirmed; each window's timeout budget is independent of the cumulative sequential delay of earlier windows. `portal spawn s1 s2 s3` self-attaches `s3` only after `s1` and `s2` both confirm, cleaning markers first; `portal spawn s1` self-attaches immediately without any ack wait. `spawnAckTimeout` is a named, documented, tunable ~8s constant. Fully unit-tested with the fake ack channel, a fake clock, and a fake adapter that simulates marker writes — no real time, tmux, or `osascript`.

**Do**:
- In `internal/spawn/command.go` (finishing the Phase 2 placeholder):
  - Change `composeAttachArgv` / `AttachCommand` to accept `batch, token string` and append the ack flag as two discrete argv elements: `"--spawn-ack", FormatSpawnAckFlag(batch, token)` (i.e. `<batch>:<token>`), yielding `[]string{"/usr/bin/env","-u","TMUX","-u","TMUX_PANE","PATH="+path, exePath, "attach", session, "--spawn-ack", batch+":"+token}`. Keep the env strip / PATH-only / own-binary invariants intact.
- In `internal/spawn/burst.go` (evolving Phase 2's `SpawnWindows`):
  - Declare the timeout with an in-source justification block (mirroring the daemon self-supervision hysteresis constant): `// spawnAckTimeout is the per-window budget for a spawned window's token ack … ~260ms osascript open + the spawned window's own abridged portal attach (fast-path, no full bootstrap) before it writes its token; ~8s gives generous headroom; tunable. const spawnAckTimeout = 8 * time.Second`.
  - `type AckOutcome string` with `AckConfirmed = "confirmed"`, `AckTimeout = "timeout"`, `AckFailed = "failed"` (exactly the spec's closed `ack` attr vocabulary).
  - `type WindowResult struct { Session string; Token string; Result Result; Ack AckOutcome }`.
  - `type Burster struct { Adapter Adapter; Ack AckCollector; Exe ExecutableResolver; Getenv func(string) string; NewID func() (string, error); Timeout time.Duration; Poll time.Duration; Now func() time.Time; Sleep func(time.Duration) }` with a constructor applying defaults (`NewID` = a `session.NewNanoIDGenerator()` closure wrapped by `NewSpawnID`; `Timeout` = `spawnAckTimeout`; `Poll` ≈ 75ms; `Now` = `time.Now`; `Sleep` = `time.Sleep`).
  - `func (b *Burster) Run(external []string) (batch string, results []WindowResult, err error)`:
    1. Resolve the executable **once** up front (`b.Exe()`); on error `return "", nil, err` (abort, zero windows).
    2. Generate **all** ids up front: `batch, err := NewSpawnID(b.NewID)`; then one token per external session. On any generation error `return "", nil, err` (abort before any window opens — Task 3.1's "never an empty/malformed id" propagates here).
    3. For each `(session, token)` in list order, sequentially: compose the argv via `AttachCommand(session, b.Exe, b.Getenv, batch, token)`, call `b.Adapter.OpenWindow(argv)`, then classify:
       - `result.OK()` ⇒ **await this window's token**: `awaitToken(b, batch, token)` — poll `b.Ack.Collect(batch)` for `token`; if present ⇒ `AckConfirmed`; else `b.Sleep(b.Poll)` and loop while `b.Now()-start < b.Timeout`; on expiry ⇒ `AckTimeout`. The per-window timer starts at *this* iteration (right after this window's `OpenWindow`), so earlier windows' cumulative delay never eats this window's budget.
       - `!result.OK()` ⇒ `AckFailed` (no ack wait — the adapter itself reported no window; the detailed failed/permission handling is Tasks 3.6/3.7).
       - Append the `WindowResult`. (Continuation vs early-stop on failure is refined in 3.6/3.7; 3.5 spawns and awaits each external window.)
    4. Return `(batch, results, nil)`.
  - Keep a small `awaitToken` helper taking the `Burster` (for the injected `Ack`/`Now`/`Sleep`/`Poll`/`Timeout`) so the poll loop is fully deterministic under a fake clock.
- In `cmd/spawn.go` `runSpawn` (upgrading the Phase 2 self-attach gate):
  - Extend `SpawnDeps` with `Ack spawn.AckChannelFull` (a channel exposing `Collect` + `Clean`; default `spawn.NewServerOptionAckChannel(client, client)`), and a `Burst` construction seam (or the `NewID`/`Timeout`/`Poll`/`Now`/`Sleep` fields) so tests inject a fake ack channel + fake clock.
  - N=1 (empty external set) — after pre-flight passes — self-attach **immediately**: `return deps.Connector.Connect(trigger)`; no burster, no ack wait.
  - N≥2 (after detect/resolve/unsupported gate): `batch, results, err := burster.Run(external)`; on `err` ⇒ return it (exit 1). Then `_ = deps.Ack.Clean(batch)` (best-effort, on every post-burst path — success or failure). If **every** `WindowResult.Ack == AckConfirmed` ⇒ emit the batch summary and `return deps.Connector.Connect(trigger)` (self-attach; outside tmux this exec-replaces the process). The not-all-confirmed branch is owned by Tasks 3.6/3.7 (this task ensures those windows do **not** self-attach).
  - Marker cleanup ordering: `Clean(batch)` runs **before** `Connect(trigger)` on the success path (before the point-of-no-return exec).
  - Logging (extending Phase 2's existing `spawn: opened N/N` emission — **not** a new logging task): the per-window DEBUG line now carries the `ack` attr (`confirmed`/`timeout`/`failed`) and the summary carries the `batch` attr; these attrs become meaningful only with this task's machinery. Keep to the closed attr set.
- Extend `internal/spawn/command_test.go`, `internal/spawn/burst_test.go`, and `cmd/spawn_test.go` (unit lane): use `spawntest.FakeAckChannel` + a manual clock (`Now` reads a `*time.Time`, `Sleep` advances it) + a `FakeAdapter` extended (this task) with an `Ack *spawntest.FakeAckChannel` reference and a parallel `Confirm []bool` (nil ⇒ all true): on a success result with `Confirm[i]` true, the fake parses `--spawn-ack <batch>:<token>` out of the argv it was handed and calls `Ack.Write(batch, token)` (simulating the spawned `portal attach`'s marker write); with `Confirm[i]` false it writes nothing (→ the burster times out that window). This makes the pipeline confirm/timeout end-to-end without real time or tmux.

**Acceptance Criteria**:
- [ ] `AttachCommand("s","…","…","b1","t1")` appends `"--spawn-ack","b1:t1"` as the final two argv elements, preserving the env-strip/PATH-only/own-binary prefix.
- [ ] `Burster.Run(["s1","s2"])` with the fake adapter confirming both returns two `WindowResult`s both `AckConfirmed`, and `cmd/spawn.go` then calls `Connect("s3")` (for `spawn s1 s2 s3`) exactly once, after `Ack.Clean(batch)`.
- [ ] Each window's ack timer is independent: with `Poll` and a manual clock, window 2's confirmation is judged against window 2's own `spawnAckTimeout` budget (starting at its spawn), not a single clock from Enter — verified by advancing the fake clock past `Timeout` *between* windows and still confirming a later window whose token arrives within its own budget.
- [ ] A token that appears after several polls but before `spawnAckTimeout` elapses is classified `AckConfirmed` (late-but-in-time).
- [ ] `portal spawn s1` (N=1) records zero `OpenWindow` calls, performs **no** ack wait, and self-attaches `s1` immediately.
- [ ] On all-confirm, `Ack.Clean(batch)` is invoked before `Connect(trigger)` (markers cleaned before the exec handoff).
- [ ] `spawnAckTimeout` is a single named package constant (~8s) with an in-source justification comment; changing it changes the per-window budget (tunable).

**Tests**:
- `"it appends --spawn-ack <batch>:<token> as the final two argv elements"`
- `"it self-attaches only after every external window's token is confirmed"`
- `"it starts each window's ack timer at its own spawn (per-window, not one global clock)"`
- `"it confirms a token that arrives late but within the timeout"`
- `"it self-attaches immediately for N=1 with no ack wait"`
- `"it cleans the batch markers before the self-attach exec handoff"`

**Edge Cases**:
- Per-window timer starts at its own spawn — the cumulative sequential delay never eats a later window's budget (not one global clock from Enter).
- A token arriving late but within `spawnAckTimeout` counts as confirmed.
- All-confirm self-attaches and cleans markers before the exec handoff.
- N=1 self-attaches immediately, no ack wait.
- `spawnAckTimeout` is a named/documented/tunable constant (~8s default).

**Context**:
> Spec *Burst & Partial-Failure Contract → Spawn, then self-attach LAST — gated on ALL N−1 confirming*: "**All confirm** → the trigger window self-attaches silently (no '14/14 ✓' nag)."
> Spec *Confirmation mechanism → Timeout is per-window, not global*: "Under sequential spawn the Nth window's `osascript` fires seconds after Enter and then runs its own abridged attach before writing its token; a single global clock from Enter would over-report late windows as failed. Each window's ack timer starts when *its* spawn fires — the cumulative sequential delay never eats the budget." *Timeout value*: "A named `spawnAckTimeout` constant, **default ~8s per window** … Each window's timer starts when *its* spawn fires; expiry classifies that window as a failed spawn."
> Spec *Order is load-bearing*: "3. **Only after all N−1 confirm**, exec self into the Nth session." Step 3 is a point of no return, so cleanup precedes it. *Cleanup*: "The picker self-cleans its batch markers before self-exec."
> Spec *N=0/N=1 boundary*: "**N=1** … the picker self-attaches to that one session … a plain single attach … No special-casing." (No ack wait — there are zero external windows.)
> Scope: this task builds the ack **gate** + timing + happy path + N=1; the not-all-confirmed **reporting** (leave-what-opened, unified timeout/spawn-failed classification, the failed-window message, clean-on-failure) is Task 3.6, and the `permission-required` burst-stop is Task 3.7. The batch-summary emission already exists from Phase 2 (`cmd/spawn.go`); this task only enriches it with the now-meaningful `ack`/`batch` attrs. `AttachCommand`/`composeAttachArgv` and `SpawnWindows` are the Phase 2 seams being evolved (`internal/spawn/command.go`, `internal/spawn/burst.go`).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Spawn, then self-attach LAST / Confirmation mechanism: explicit token ack (Timeout is per-window / Timeout value) / Cleanup*; *Spawn Architecture → Order is load-bearing*; *Trigger-Context Matrix & Open Order → N=0/N=1 boundary*.

---

## restore-host-terminal-windows-3-6 | approved

### Task 3.6: Leave-what-opened partial-failure handling

**Problem**: Once past pre-flight, a rare per-window spawn hiccup (a transient `osascript`/terminal failure → adapter `spawn-failed`, or a token that never arrives → ack timeout) can still occur. Portal must **not** tear down the windows that already opened (it does not own those host windows and won't rely on untested teardown): it leaves them in place, **skips** the trigger's self-attach (so the trigger stays in its calling context), cleans its batch markers, and reports a one-line error naming the window(s) that failed. Phase 2's placeholder stopped the burst on the first non-success and reported a bare error; this task implements the real leave-what-opened contract, unifying ack-timeout and adapter `spawn-failed` into a single "failed" classification.

**Solution**: Have `Burster.Run` (Task 3.5) spawn **all** external windows (no early stop on `spawn-failed` or `timeout` — superseding Phase 2's stop-on-first placeholder; the only early stop is `permission-required`, added in Task 3.7), so every window that can open does open. In `cmd/spawn.go`, on any not-all-confirmed result: leave opened windows untouched (no teardown call exists anywhere), skip `Connect(trigger)`, `Clean(batch)` the markers, log the partial batch summary, and return a plain error naming every failed window — where "failed" is any `WindowResult` whose `Ack != AckConfirmed` (both `AckTimeout` and adapter `AckFailed`/`spawn-failed`). Exit 1 with that one-line message on stderr.

**Outcome**: `portal spawn s1 s2 s3 s4` where `s2`'s window times out and the rest confirm: `s1`,`s3` windows stay open (never closed), `s3` continues to be spawned despite `s2`'s earlier failure, the trigger does **not** self-attach (stays in its calling context), the batch markers are cleaned, and the command returns `spawn: failed to open window(s) for 's2' — others left open` (exit 1, stderr). An adapter `spawn-failed` on a window classifies identically to a timeout. Unit-tested with the fake adapter (scripted timeouts/failures) + fake ack channel + fake connector, asserting no teardown, the correct set of opened windows, a skipped self-attach, cleaned markers, and the named message.

**Do**:
- In `internal/spawn/burst.go` `Run` (Task 3.5's loop): confirm the loop **continues** through all external windows on a `spawn-failed` or `timeout` outcome — do not `break`. (Remove/override Phase 2's "stop on the first non-success"; the sole early stop, `permission-required`, is added in Task 3.7.) Every window that the adapter accepts opens a real host window; the burst records each window's `WindowResult` regardless of the others' fates.
- In `cmd/spawn.go` `runSpawn` — the not-all-confirmed branch after `burster.Run` + `Ack.Clean(batch)`:
  - Compute `failed := [ r.Session for r in results if r.Ack != AckConfirmed ]` (unifying `AckTimeout` and adapter `AckFailed`). (The `permission-required` sub-case routes to Task 3.7's guidance path *before* this generic branch; here handle the non-permission failures.)
  - **Do not** attempt to close or undo any opened window — there is deliberately no teardown code path (Portal does not own the host windows). The opened windows are left as working attached sessions.
  - **Skip** `Connect(trigger)` — the trigger stays in its current context (the picker/calling terminal), never self-execs.
  - Log the partial batch summary (INFO `spawn: opened <opened>/<total>` where `opened` counts confirmed windows and excludes the skipped trigger self-attach; per-window DEBUG carries the `ack` attr). This extends Phase 2's existing emission — no new logging task.
  - `return fmt.Errorf("spawn: failed to open window(s) for %s — others left open", spawn.QuoteJoin(failed))` — a plain, non-`UsageError`, non-silenced error → exit 1 on stderr. The opaque `Result.Detail` for each failure goes only to the DEBUG log, never the user-facing message.
- Extend `cmd/spawn_test.go` and `internal/spawn/burst_test.go` (unit lane): script the fake adapter so one window among several times out (`Confirm[i]=false`) and, separately, so one window returns `spawn.SpawnFailed(...)`; drive `spawn s1 s2 s3 s4`. Assert: the burst spawned windows for all four (or all up to N−1 — i.e. no early stop); the connector's `Connect` was called **zero** times; `Ack.Clean(batch)` was called; the returned error names the failed session(s) and is not a `*cmd.UsageError`; there is no code path that calls any "close window" operation (assert by construction — no teardown seam exists).

**Acceptance Criteria**:
- [ ] With one window among many timing out, the burst still spawns the remaining windows (no early stop on timeout/`spawn-failed`) and the opened windows are left in place (no teardown invoked).
- [ ] The trigger's self-attach (`Connect`) is **skipped** on any not-all-confirmed batch, so the trigger stays in its calling context.
- [ ] An adapter `spawn-failed` result and an ack `timeout` are classified identically as "failed" and both appear (by session name) in the one-line message.
- [ ] The batch markers are `Clean`ed on the failure path (best-effort), just as on the success path.
- [ ] The command returns a plain error (exit 1) whose one-line stderr message names the failed window(s) and does not leak the opaque `Result.Detail`.
- [ ] Multiple failed windows are all named in the single message.

**Tests**:
- `"it leaves already-opened windows in place when one window times out among many"`
- `"it continues spawning the remaining windows after a spawn-failed window (no early stop)"`
- `"it classifies an ack timeout and an adapter spawn-failed identically as failed"`
- `"it skips the trigger self-attach on a partial failure and stays in the calling context"`
- `"it cleans the batch markers on the failure path"`
- `"it returns exit 1 with a one-line message naming the failed window(s) on stderr"`

**Edge Cases**:
- One window times out among many → it is named while the others stay open (no teardown).
- Ack-timeout and adapter `spawn-failed` both map to the "failed" classification.
- Self-attach skipped so the trigger stays in its calling context.
- Markers self-cleaned on the failure path.
- Exit 1 with the one-line failed-window message on stderr.

**Context**:
> Spec *Burst & Partial-Failure Contract → Spawn, then self-attach LAST*: "**Any fails** (a transient `osascript`/terminal hiccup *after* pre-flight passed — genuinely rare) → Portal does **not** try to close or undo the windows that already opened; it doesn't own those host windows and won't rely on untested teardown. It **leaves them in place** (they're working attached sessions), **skips the trigger window's self-attach** so you stay in the picker, and shows a clean one-line error naming the window that failed to come up."
> Spec *Confirmation mechanism*: "A missing marker at timeout = a failed spawn → that window is treated as failed (per the Stance above: skip the trigger self-attach, leave the other opened windows in place, report the failed window)."
> The **selection mutation** half — "Portal **unmarks the sessions whose windows opened and keeps the failed/un-acked ones marked**, so a second `Enter` retries exactly the missing set" — is a **picker/TUI selection-state concern (Phase 6)**, not this CLI task. The CLI has no persistent selection; it reports and exits 1. This task delivers the CLI's leave-what-opened + report + clean + skip-self-attach behaviour that Phase 6 wraps its selection mutation around.
> **Reconciliation with Phase 2:** Phase 2's Task 2.6 "stop on the first non-success" was explicitly a placeholder ("the detailed leave-what-opened / continue-and-retry-the-missing-set behaviour is Phase 3"). This task replaces it with continue-through-all so every window that can open does, and the report names exactly the ones that didn't. The sole early-stop exception (`permission-required`) is Task 3.7.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Burst & Partial-Failure Contract → Stance / Spawn, then self-attach LAST — gated on ALL N−1 confirming / Confirmation mechanism: explicit token ack*; *Spawn Architecture → Reporting & exit codes (partial spawn failure → exit 1)*.

---

## restore-host-terminal-windows-3-7 | approved

### Task 3.7: `permission-required` burst-stop

**Problem**: The native adapter's defensive net can surface `permission-required` (a `-1743` denied / `-1712` timeout AppleEvent result). Because spawns are sequential and the macOS Automation grant is per-`(source, target)` pair, if window *k* hits the permission wall, every later window (same source terminal → same target terminal) would hit the identical wall — so the burst must **stop** at *k*, not grind through k+1…N−1. It must surface the permission **guidance once for the batch** (naming the target terminal + the Automation-settings hint the driver composed), not the generic spawn-failed one-liner, while leaving the windows opened before *k* in place and cleaning markers. Two pieces are missing: the Ghostty driver never yet *produces* `permission-required` (Phase 2 deferred the `-1712`/`-1743` mapping here), and the orchestrator has no burst-stop/guidance path.

**Solution**: (1) Extend the Ghostty driver's pure `mapGhosttyResult` (`internal/spawn/ghostty.go`) to recognise `-1743`/`-1712` in the `osascript` output and return `PermissionRequired(detail, guidance)` with a driver-composed, opaque guidance string (target terminal + Automation-settings deep-link) — before the `spawn-failed` catch-all. (2) In `Burster.Run`, make a `permission-required` result the **sole** early-stop: record it and break, so windows k+1…N−1 are never spawned. (3) In `cmd/spawn.go`, when any window result is `permission-required`: clean markers, skip self-attach, leave earlier windows in place, and return the driver's `Guidance` verbatim as the one-line error (shown once), exit 1.

**Outcome**: A fabricated `osascript` outcome carrying `-1743` maps to `OutcomePermissionRequired` with a non-empty opaque `Guidance` (naming Ghostty + the Automation-settings hint) and never to `spawn-failed`. In a burst, a `permission-required` on window *k* stops the adapter from being called for windows k+1…N−1; the windows opened before *k* stay open; the batch markers are cleaned; and the command prints the driver's guidance **once** (not the generic `failed to open window(s)` line) and exits 1. Unit-tested via the fake adapter (scripted `PermissionRequired`) for the burst-stop and a fabricated `osascript` outcome for the driver mapping — no real `osascript`, TCC modal, or tmux.

**Do**:
- In `internal/spawn/ghostty.go` `mapGhosttyResult(out string, exitCode int, err error) Result` (from Phase 2 Task 2.5): **before** the `spawn-failed` catch-all, detect a permission outcome — `if strings.Contains(out, "-1743") || strings.Contains(out, "-1712") { return PermissionRequired(out, ghosttyPermissionGuidance()) }` (`-1743` = AppleEvent not-permitted / denied; `-1712` = AppleEvent timeout). Keep `out` as the opaque `Detail`; the `Guidance` is composed **in the driver**:
  - `func ghosttyPermissionGuidance() string` — an opaque, user-readable string naming the **target terminal** (Ghostty) and the Automation-settings hint, e.g. `"Ghostty needs permission to open new windows. Grant it under System Settings → Privacy & Security → Automation (x-apple.systempreferences:com.apple.preference.security?Privacy_Automation), then try again."` (The deep-link `x-apple.systempreferences:com.apple.preference.security?Privacy_Automation` is composed here and handed up opaque; general code never parses it. Exact copy is not pinned by the spec beyond "names the target terminal + the Automation-settings hint.")
  - Clean exit (`err == nil && exitCode == 0`) still → `Success`; any non-clean, non-permission exit still → `SpawnFailed` (Phase 2 behaviour preserved for the catch-all).
- In `internal/spawn/burst.go` `Run` loop: after appending each `WindowResult`, `if r.Result.Outcome == OutcomePermissionRequired { break }` — the **only** early stop (timeout/`spawn-failed` continue, per Task 3.6). Windows after *k* are never composed or handed to the adapter.
- In `cmd/spawn.go` `runSpawn` — **before** Task 3.6's generic failed branch, after `Ack.Clean(batch)`:
  - `if perm, ok := firstPermission(results); ok {` (the first `WindowResult` with `Outcome == OutcomePermissionRequired`):
    - Leave earlier opened windows in place (no teardown); skip `Connect(trigger)`.
    - Emit one `spawn` outcome line with the `permission-required` category and the opaque `detail` (driver `Result.Detail`) — honouring the driver-quarantine rule (general code logs the opaque detail, never an AppleEvent number it interpreted).
    - `return errors.New(perm.Result.Guidance)` — the driver-composed guidance verbatim, **once** for the batch (a plain, non-`UsageError`, non-silenced error → exit 1 on stderr). Do **not** also emit the generic `failed to open window(s)` message.
  - Because `firstPermission` is checked before the generic failed branch, the permission case never double-reports.
- Extend `internal/spawn/ghostty_openwindow_test.go` / a driver-mapping test and `cmd/spawn_test.go` / `internal/spawn/burst_test.go` (unit lane):
  - Driver: feed `mapGhosttyResult` a fabricated `(out="…AppleScript error: … (-1743)", exitCode=1, err=nil)` → assert `OutcomePermissionRequired`, a non-empty `Guidance` containing "Ghostty" and the Automation hint, and `Detail` carrying the opaque output; feed `-1712` similarly; feed a plain `exitCode=1` with no permission code → still `OutcomeSpawnFailed` (regression guard).
  - Burst-stop: script the `FakeAdapter` `Results` so window 2 of 5 returns `spawn.PermissionRequired("evt -1743","grant Automation for Ghostty")`; drive `spawn s1 s2 s3 s4 s5`; assert `len(FakeAdapter.Calls) == 2` (windows 3,4,5 never spawned), `Connect` count 0, `Ack.Clean(batch)` called, the returned error equals the guidance string (shown once, not the generic failed line), and the two earlier `OpenWindow` calls (windows 1,2) were made (earlier windows attempted/left in place).

**Acceptance Criteria**:
- [ ] `mapGhosttyResult` maps an output containing `-1743` **or** `-1712` to `OutcomePermissionRequired` with a non-empty opaque `Guidance` (naming Ghostty + the Automation-settings hint) and an opaque `Detail`; a non-permission non-zero exit still maps to `OutcomeSpawnFailed`.
- [ ] A `permission-required` on window *k* in a burst stops the adapter from being called for windows k+1…N−1 (asserted via `FakeAdapter.Calls` length == *k*).
- [ ] The permission guidance is surfaced **once** for the batch — the returned error is the driver's `Guidance` string, not the generic `failed to open window(s)` one-liner.
- [ ] Windows opened before *k* are left in place (no teardown) and the trigger self-attach is skipped (exit 1, no self-exec).
- [ ] The batch markers are `Clean`ed on the permission path.
- [ ] General code never inspects an AppleEvent number — the `-1743`/`-1712` recognition lives only in the driver; the orchestrator switches on `Outcome`/`Guidance` alone.

**Tests**:
- `"it maps a -1743 or -1712 osascript outcome to permission-required with driver-composed guidance"`
- `"it still maps a non-permission non-zero exit to spawn-failed (regression)"`
- `"it stops the burst on permission-required so later windows are never spawned"`
- `"it surfaces the permission guidance once, not the generic spawn-failed message"`
- `"it leaves earlier-opened windows in place and skips the self-attach on permission-required"`
- `"it cleans the batch markers on the permission-required path"`

**Edge Cases**:
- `permission-required` on window *k* stops windows k+1…N−1 (each would hit the same per-`(source, target)` wall).
- Guidance shown once for the batch (target terminal + Automation-settings hint), not the generic spawn-failed one-liner.
- Windows opened before *k* left in place (no teardown).
- Markers self-cleaned on the permission path.

**Context**:
> Spec *Permissions & Error Quarantine → Defensive net*: "**Within a burst:** a `permission-required` result is accounted like a failed window (skip self-attach, leave opened windows in place, keep the affected session marked) **and stops the burst** — since spawns are sequential and the grant is per-`(source, target)`, every later window would hit the same wall. It surfaces the permission **guidance once for the batch** (naming the target terminal + the Automation-settings deep-link), not the generic one-line spawn error. The grant persists, so a retry after granting proceeds."
> Spec *Permissions & Error Quarantine → Architectural boundary*: "All terminal/OS-specific concerns — the AppleScript, `osascript`, the `-1712`/`-1743` AppleEvent codes, TCC, any macOS deep-link — live **inside the terminal driver and nowhere else**. The driver translates them into a **generic typed result** … Portal's general spawn/report/UI code switches on the category and **never sees an AppleScript string or AppleEvent number**."
> Spec *Reporting & exit codes*: "**`permission-required`** (rare; native-adapter defensive path) → **exit `1`** with the permission guidance (target terminal + Automation-settings hint) on stderr; nothing self-execs."
> **Consolidation note (why this task touches two files):** Phase 2 Task 2.5 explicitly deferred the Ghostty `-1712`/`-1743` → `permission-required` *mapping* to Phase 3, and the Phase 3 task list has no separate driver-mapping task, so the mapping is authored here alongside its sole consumer (the burst-stop). Without the driver mapping, production would never produce `permission-required` and the burst-stop would be dead code; without the burst-stop, the driver's `permission-required` would be mishandled. They are unit-tested independently (fabricated `osascript` outcome for the mapping; `FakeAdapter`-scripted `PermissionRequired` for the burst-stop). The `Guidance` field on `Result` already exists from Phase 2 Task 2.1; this task populates it in the Ghostty driver. TCC is self-exempt in the normal Ghostty-scripting-Ghostty flow (validated live), so this path is rare — it is a defensive net, not a first-run gate.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Permissions & Error Quarantine → Architectural boundary / Defensive net (within a burst)*; *Spawn Architecture → Reporting & exit codes (`permission-required` → exit 1)*; *Testing Strategy & DI Seams → Driver split for testability (error-mapping)*.
