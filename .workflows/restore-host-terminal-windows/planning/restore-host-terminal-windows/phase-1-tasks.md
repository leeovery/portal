---
phase: 1
phase_name: Terminal Detection & `portal spawn --detect` dry-run
total: 6
---

## restore-host-terminal-windows-1-1 | approved

### Task 1.1: Package scaffold + Identity model + bundle-id family matching

**Problem**: The feature needs a home package (`internal/spawn`) and a value type that represents a detected host terminal's identity. Every downstream detection path (walk, env fast-path, list-clients, the orchestrator, the `--detect` CLI) produces or consumes this identity, and later phases resolve it to an adapter by matching its bundle id against a family glob. Nothing exists yet — there is no `internal/spawn` package.

**Solution**: Stand up the `internal/spawn` Go package with a package doc, an `Identity` value type (the terminal's macOS bundle id + a human-readable display name, plus a NULL state), a passthrough constructor, and a standalone bundle-id **family-matching** glob primitive that Phase 2's adapter resolver will consume.

**Outcome**: `internal/spawn` compiles as a new package; `spawn.Identity` can be constructed from a bundle id (+ optional app name) and reports whether it is NULL; `spawn.MatchesFamily` correctly matches channel-suffixed bundle ids against a family glob. All logic is unit-tested with fabricated inputs (no OS calls).

**Do**:
- Create `internal/spawn/doc.go` with a package comment describing the package's role (terminal detection, adapter resolution, and window spawning — the shared service reached in-process by the picker and by `portal spawn`), per spec *Spawn Architecture → Model: one service, two callers*.
- Create `internal/spawn/identity.go`:
  - Define `type Identity struct { BundleID string; Name string }` — `BundleID` is the exact macOS bundle id read from the terminal's `.app` (e.g. `dev.warp.Warp-Stable`, `com.mitchellh.ghostty`, `com.apple.Terminal`); `Name` is the friendly display name for reading (e.g. `Warp`, `Ghostty`, `Apple Terminal`).
  - Add `func (i Identity) IsNull() bool` returning `i.BundleID == ""` — the NULL state is defined solely by an empty bundle id (spec: remote/mosh → NULL → unsupported → honest no-op).
  - Add a constructor `func NewIdentity(bundleID, appName string) Identity` that:
    - trims `bundleID`; if empty ⇒ return the zero `Identity{}` (NULL) regardless of `appName`.
    - for a non-empty `bundleID`, always produces a **passthrough** identity (never NULL) even when the bundle id is unknown to Portal — this is the "unknown bundle id → passthrough" rule.
    - sets `Name` to `appName` when non-empty; otherwise **derives** a friendly name from the bundle id: take the last dot-delimited segment, then trim any channel suffix at the first `-` (e.g. `dev.warp.Warp-Stable` → `Warp`, `com.apple.Terminal` → `Terminal`). Never leave `Name` empty for a non-empty bundle id.
  - Define `func MatchesFamily(bundleID, pattern string) bool` — the family-glob primitive: a `*` in `pattern` matches any run of characters (including a channel suffix). `dev.warp.Warp-Stable` matches `dev.warp.Warp-*`; an exact (no-`*`) pattern matches only its literal; a bare `*` matches anything; a non-matching pattern returns false. Use `path.Match`-style semantics or a small hand-rolled matcher (document which). This is a pure primitive with no consumer inside Phase 1 — it is built and tested here for Phase 2's resolver.
- Add `internal/spawn/identity_test.go` (package `spawn`, unit lane).

**Acceptance Criteria**:
- [ ] `internal/spawn` compiles and `go test ./internal/spawn/...` passes.
- [ ] `NewIdentity("dev.warp.Warp-Stable", "")` returns `IsNull()==false`, `BundleID=="dev.warp.Warp-Stable"`, `Name=="Warp"`.
- [ ] `NewIdentity("", "Ghostty").IsNull()` is `true` (empty bundle id ⇒ NULL even with an app name).
- [ ] `NewIdentity("com.example.MyTerm", "")` (an unknown bundle id) returns a non-NULL passthrough identity carrying the raw bundle id and a derived name — never NULL.
- [ ] `MatchesFamily("dev.warp.Warp-Stable", "dev.warp.Warp-*")` is `true`; `MatchesFamily("com.apple.Terminal", "com.apple.Terminal")` is `true`; `MatchesFamily("com.apple.Terminal", "dev.warp.Warp-*")` is `false`; `MatchesFamily("anything", "*")` is `true`.

**Tests**:
- `"it builds a passthrough identity from a channel-suffixed bundle id with a derived name"`
- `"it returns a NULL identity for an empty or whitespace-only bundle id"`
- `"it keeps an unknown bundle id as a passthrough identity, never NULL"`
- `"it prefers a supplied app name over the derived name"`
- `"it matches a channel-suffixed bundle id against its family glob"`
- `"it matches an exact bundle id only against its literal pattern"`
- `"it matches any bundle id against a bare * catch-all"`
- `"it rejects a bundle id that belongs to a different family"`

**Edge Cases**:
- Channel-suffixed bundle id matches its family glob (`dev.warp.Warp-Stable` → `dev.warp.Warp-*`).
- Unknown bundle id → passthrough identity (raw id + derived name), never NULL.
- Empty/absent bundle id → NULL identity.

**Context**:
> Spec *Terminal Identity & Detection → Identity resolution*: "The system-blessed identity is the terminal's macOS **bundle id**. … Matching is by **bundle-id family** (e.g. `dev.warp.Warp-*`), channel-aware. Remote/mosh clients resolve to a **NULL bundle id** → unsupported → honest no-op."
> Spec *User-facing display: both*: the banner and `--detect` show both the friendly `.app` name and the exact bundle id (design copy example: `Apple Terminal · com.apple.Terminal`), so `Identity` must carry both.
> Scope boundary: the friendly-alias table (`ghostty`/`warp` → family) and the resolver that maps identity → adapter are **Phase 2**. Phase 1 builds only the value type and the standalone matching primitive. The exact friendly-name-derivation algorithm (last dotted segment, channel suffix trimmed) is an implementation choice the spec does not pin beyond "non-empty, human-readable"; the walk (Task 1.2) supplies the true `.app` display name when available.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Identity resolution: macOS bundle id, matched as a family*; *User-facing display: both*; *Naming*.

---

## restore-host-terminal-windows-1-2 | approved

### Task 1.2: Process-tree walk to bundle id

**Problem**: The primary detection mechanism walks from a starting process (the picker, or a tmux client) up its ancestry until it reaches a macOS `.app` bundle, then reads that bundle's id. This must cleanly separate a local terminal (reaches `/Applications/Ghostty.app/…/ghostty`) from a remote/mosh client (reaches `mosh-server` at ppid 1 with no `.app`), and must distinguish a genuine OS failure from a clean "no host-local terminal" outcome.

**Solution**: Implement a process-tree walk behind two small seam interfaces — a process-info reader (`ps`) and a bundle-info reader (`defaults read`) — plus a pure `walkToBundle` function with a three-shape return contract (resolved `Identity` / clean NULL / typed transient error), and real `ps`/`defaults` implementations.

**Outcome**: `walkToBundle(startPID, walker, reader)` returns a resolved non-NULL `Identity` when ancestry reaches a `.app`; a NULL `Identity` with nil error when ancestry exhausts at ppid 1 / a `.app`-less boundary; and a typed transient error (distinct from clean NULL) when `ps` or `defaults read` fails. The resolution logic is fully unit-tested with fabricated seam data; the real `ps`/`defaults` route is manual/integration.

**Do**:
- Create `internal/spawn/walk.go`:
  - Define the seams (small, Portal DI style):
    - `type ProcessWalker interface { ProcessInfo(pid int) (ppid int, command string, err error) }` — returns the parent pid and the executable path/comm for `pid`. Real impl runs `ps -o ppid=,comm= -p <pid>` (validated: `ps -o comm=` returns full paths on this Mac), parses the two fields.
    - `type BundleReader interface { Read(appPath string) (bundleID, name string, err error) }` — given an `.app` bundle directory, returns its `CFBundleIdentifier` and a display name. Real impl runs `defaults read <appPath>/Contents/Info.plist CFBundleIdentifier` (required) and best-effort `CFBundleName` (fall back to the `.app` basename with `.app` stripped when absent) — the clean `lsappinfo`-free route the spec chose.
  - Define a typed transient error distinct from clean NULL, e.g. `var ErrDetectTransient = errors.New("terminal detection transient failure")`, wrapped around the underlying `ps`/`defaults` failure so callers `errors.Is(err, ErrDetectTransient)`.
  - Implement `func walkToBundle(startPID int, walker ProcessWalker, reader BundleReader) (Identity, error)`:
    1. From `startPID`, loop: call `walker.ProcessInfo(pid)`.
    2. On a `ps` error ⇒ return `Identity{}, ErrDetectTransient`-wrapped error.
    3. Inspect `command` for a `.app` bundle: find `.app/` in the path and take the prefix up to and including `.app` (e.g. `/Applications/Ghostty.app/Contents/MacOS/ghostty` → `/Applications/Ghostty.app`). If found, call `reader.Read(appPath)`; on read error ⇒ return transient error; on success ⇒ `return NewIdentity(bundleID, name), nil`.
    4. No `.app` in this hop ⇒ ascend to `ppid`. Stop when `ppid <= 1` (or the same pid reappears): the ancestry is exhausted with no `.app` ⇒ `return Identity{}, nil` (clean NULL).
    5. Guard against runaway/cyclic ancestry with a bounded max-hop count (e.g. 32); hitting the bound ⇒ clean NULL, not an error.
  - Add the real `ProcessWalker`/`BundleReader` implementations (e.g. `realProcessWalker`, `realBundleReader`) in the same file, using `log.CombinedOutputWithContext` or `exec.Command` for the `ps`/`defaults` boundary.
- Add `internal/spawn/walk_test.go` (unit lane) with map-backed fake seams (`map[int]struct{ppid int; command string}`, `map[string]struct{bundleID, name string}`).

**Acceptance Criteria**:
- [ ] A multi-hop chain `picker(pid=100,ppid=200) → zsh(200,ppid=300) → ghostty(300,ppid=1, command=/Applications/Ghostty.app/Contents/MacOS/ghostty)` resolves to a non-NULL `Identity{BundleID:"com.mitchellh.ghostty", Name:"Ghostty"}`.
- [ ] An ancestry that reaches `mosh-server`/ppid 1 with no `.app` in any hop returns `Identity{}` (NULL) and `nil` error.
- [ ] A `ps` failure mid-walk returns a NULL-ish `Identity` and an error satisfying `errors.Is(err, ErrDetectTransient)` — distinct from the clean-NULL nil-error case.
- [ ] A `defaults read` failure on a found `.app` returns `ErrDetectTransient`, not a clean NULL.
- [ ] A cyclic/over-long ancestry terminates via the hop bound and returns clean NULL (no hang).

**Tests**:
- `"it resolves a multi-hop local chain to the app bundle id"`
- `"it returns clean NULL when ancestry reaches ppid 1 with no app bundle"`
- `"it returns clean NULL for a mosh-server ancestry"`
- `"it returns a transient error when ps fails, distinct from clean NULL"`
- `"it returns a transient error when defaults read fails on a found app"`
- `"it terminates on a cyclic ancestry via the hop bound"`

**Edge Cases**:
- Ancestry reaches ppid-1 / mosh-server with no `.app` → clean NULL.
- Multi-hop walk (picker → zsh → ghostty).
- `ps` or `defaults read` failure → typed transient error distinct from clean NULL.

**Context**:
> Spec *Terminal Identity & Detection → Detection model*: "Outside tmux … the picker **self-walks its own process tree** to the terminal (`picker → zsh → ghostty`)"; *Identity resolution*: "The walk resolves `client_pid → process-tree → .app bundle` via an Info.plist read (`defaults read` of the bundle's Info.plist — a clean `lsappinfo`-free route)." Validated: "`ps -o comm=` returns full paths; the walk cleanly separates local Ghostty (`→ login → /Applications/Ghostty.app/…/ghostty` → bundle id) from remote mosh (`→ mosh-server` at ppid 1 → NULL). Read-only, no `osascript`/Apple-event needed."
> Spec *Detection lifecycle → Error vs clean NULL*: "A clean NULL (remote/mosh → unsupported) and a *transient detection error* (a `ps` / `defaults read` failure) both resolve to the unsupported/no-op path; a transient error additionally emits a `spawn`-component WARN breadcrumb." The transient error must therefore be programmatically distinguishable from clean NULL (Task 1.5 branches on it).
> Spec *Testing Strategy → Detection behind small seams*: "Detection reads behind small (1–3-method) interfaces … detect-self *resolution* … is unit-testable with fabricated data. The real walk is integration (real-tmux `tmuxtest` fixture) / manual." The real `ps`/`defaults` boundary is manual/integration only; no automated test executes real `ps`.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Detection model / Identity resolution*; *Detection lifecycle → Error vs clean NULL*; *Testing Strategy & DI Seams → Detection behind small seams*.

---

## restore-host-terminal-windows-1-3 | approved

### Task 1.3: Outside-tmux detection — env fast-path + walk fallback

**Problem**: The primary trigger flow is a fresh terminal → picker running **outside** tmux. Detection there is direct: prefer a cheap environment-variable fast-path (`__CFBundleIdentifier` / `GHOSTTY_*`, accurate outside tmux), and fall back to the process-tree walk only when the env gives nothing usable. No client list, no tiebreak.

**Solution**: Implement `detectOutsideTmux` composing an injectable env reader with Task 1.2's walk: `__CFBundleIdentifier` → bundle id directly (no walk); `GHOSTTY_*` (without `__CFBundleIdentifier`) → Ghostty's known bundle id (no walk); otherwise (both absent, or an empty/malformed env value) → walk from the picker's own pid.

**Outcome**: `detectOutsideTmux` returns an `Identity` (or the walk's transient error) chosen by the fast-path-then-walk precedence, unit-tested with a fabricated env getter and fabricated walk seams.

**Do**:
- Create `internal/spawn/detect_outside.go`:
  - Add a const for Ghostty's bundle id, `const ghosttyBundleID = "com.mitchellh.ghostty"` (validated live via the `com.mitchellh.ghostty` TCC reset; this is detection-layer knowledge — mapping it to a driver is Phase 2).
  - Implement `func detectOutsideTmux(getenv func(string) string, selfPID int, walker ProcessWalker, reader BundleReader) (Identity, error)`:
    1. Read `__CFBundleIdentifier` via `getenv`. If it is non-empty after trimming **and** passes a minimal plausibility check (contains a `.`, no internal whitespace) ⇒ `return NewIdentity(<value>, ""), nil` — direct, no walk.
    2. Else if any `GHOSTTY_*` env var is present (e.g. `GHOSTTY_RESOURCES_DIR`, `GHOSTTY_BIN_DIR`) ⇒ `return NewIdentity(ghosttyBundleID, "Ghostty"), nil` — direct, no walk.
    3. Else (both absent, or `__CFBundleIdentifier` empty/malformed) ⇒ `return walkToBundle(selfPID, walker, reader)` (may return a resolved identity, clean NULL, or a transient error — propagate verbatim).
  - Use an injectable `getenv func(string) string` seam (production passes `os.Getenv`) so unit tests fabricate env without `t.Setenv`. Check the specific `GHOSTTY_*` keys explicitly (a named, finite set), not a full-environ scan.
- Add `internal/spawn/detect_outside_test.go` (unit lane) with a `getenv` backed by a `map[string]string` and map-backed walk seams (reuse the Task 1.2 fakes). Assert the walk seam is **not** invoked on the two fast-path branches.

**Acceptance Criteria**:
- [ ] `__CFBundleIdentifier=com.apple.Terminal` (no other setup) ⇒ `Identity{BundleID:"com.apple.Terminal", Name:"Terminal"}` and the walk seam is never called.
- [ ] `GHOSTTY_RESOURCES_DIR` set with `__CFBundleIdentifier` absent ⇒ `Identity{BundleID:"com.mitchellh.ghostty", Name:"Ghostty"}` and the walk seam is never called.
- [ ] Both env vars absent ⇒ the function delegates to `walkToBundle(selfPID, …)` and returns exactly its result (resolved / NULL / transient error).
- [ ] `__CFBundleIdentifier` set to `""` or a malformed value (e.g. `"  "`) ⇒ falls back to the walk (does not return a bogus identity).

**Tests**:
- `"it resolves directly from __CFBundleIdentifier without walking"`
- `"it resolves to Ghostty from a GHOSTTY_* var when __CFBundleIdentifier is absent"`
- `"it falls back to the walk when both env vars are absent"`
- `"it falls back to the walk for an empty or malformed __CFBundleIdentifier"`
- `"it propagates the walk's clean NULL and transient error unchanged"`

**Edge Cases**:
- `__CFBundleIdentifier` present → bundle id direct, no walk.
- `GHOSTTY_*` present without `__CFBundleIdentifier` → Ghostty bundle id direct, no walk.
- Both env vars absent → walk fallback.
- Empty/malformed env value → walk fallback.

**Context**:
> Spec *Terminal Identity & Detection → Detection model*: "**Outside tmux (primary flow — fresh terminal → picker):** the picker **self-walks its own process tree** to the terminal (`picker → zsh → ghostty`), or uses the env fast-path (`GHOSTTY_*` / `__CFBundleIdentifier`, accurate outside tmux). Direct — no client list, no tiebreak."
> The exact set of `GHOSTTY_*` keys and the "malformed" plausibility rule are not enumerated by the spec beyond "accurate outside tmux"; pin the empty/whitespace case as the clear fallback trigger and treat a value with no `.` or internal whitespace as malformed. Flag any real-world `GHOSTTY_*` key discovery for the build-time residual walk-confirmation.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Detection model (item 1)*.

---

## restore-host-terminal-windows-1-4 | approved

### Task 1.4: Inside-tmux detection — list-clients NULL-filter + local-only activity tiebreak

**Problem**: When the picker is triggered **inside** tmux, the picker's own ancestry leads to the tmux server, not the terminal — so detection must instead enumerate the current session's tmux clients and walk each client's process tree, filtering out remote/mosh clients (which walk to NULL). Among 2+ local clients, `client_activity` breaks the tie.

**Solution**: Add a `list-clients` seam to the tmux client, and implement `detectInsideTmux`: enumerate clients for the current session, walk each client's pid (reusing Task 1.2), drop clients that walk to NULL (remote/mosh), and return the sole local client's identity — or, among 2+ local clients, the one with the highest `client_activity`. A `list-clients` failure is a typed transient error.

**Outcome**: `detectInsideTmux` returns the local terminal's `Identity`, a NULL `Identity` when no client is host-local, or a transient error when `list-clients` fails — unit-tested with a fake client lister and fake walk seams; the real `list-clients` parse is covered by a fake-Commander unit test on the new tmux method.

**Do**:
- In `internal/tmux/` (new file, e.g. `clients.go`):
  - Define `type ClientInfo struct { PID int; Activity int64 }`.
  - Add `func (c *Client) ListClients(session string) ([]ClientInfo, error)` running `list-clients -t <session> -F "#{client_pid} #{client_activity}"` via the `Commander` seam, parsing each line into `ClientInfo`. Collapse a "no server / no clients" error to an empty slice + nil (mirror `ListSessions`' no-server tolerance); a genuine parse failure returns an error.
  - Unit-test the parse with `MockCommander` (fake Commander), same package-test pattern as `tmux_test.go` — this is the unit lane. The real `list-clients` against a live server is manual/integration (`tmuxtest` fixture).
- In `internal/spawn/detect_inside.go`:
  - Define a spawn-local seam `type clientLister interface { ListClients(session string) ([]ClientActivity, error) }` with `type ClientActivity struct { PID int; Activity int64 }` (spawn-local so the resolution function is unit-testable without tmux).
  - Add a thin production adapter (e.g. `type tmuxClientLister struct{ c *tmux.Client }`) that calls `c.ListClients(session)` and maps `tmux.ClientInfo` → `spawn.ClientActivity`.
  - Implement `func detectInsideTmux(session string, lister clientLister, walker ProcessWalker, reader BundleReader) (Identity, error)`:
    1. `clients, err := lister.ListClients(session)`; on error ⇒ return `Identity{}, ErrDetectTransient`-wrapped.
    2. For each client, `id, werr := walkToBundle(client.PID, walker, reader)`. A resolved non-NULL `id` ⇒ this is a **local** client; keep `(id, client.Activity)`. A clean NULL ⇒ remote/mosh, drop it. A transient walk error ⇒ record that a transient failure occurred but continue scanning other clients.
    3. Zero local clients: if a transient failure was seen and nothing resolved ⇒ return `ErrDetectTransient`; otherwise ⇒ clean NULL (no host-local terminal).
    4. Exactly one local client ⇒ return its `Identity` (no tiebreak).
    5. 2+ local clients ⇒ return the identity with the highest `Activity` (first-wins on an exact tie).
- Add `internal/spawn/detect_inside_test.go` (unit lane) with a fake `clientLister` returning fabricated `ClientActivity` slices and map-backed walk seams.

**Acceptance Criteria**:
- [ ] A session whose only clients walk to `mosh-server`/NULL ⇒ `detectInsideTmux` returns clean NULL (nil error) — no host-local terminal.
- [ ] A single local client (walks to a `.app`) ⇒ that client's `Identity`, with no reliance on `client_activity`.
- [ ] Two local clients with different `client_activity` ⇒ the higher-activity client's `Identity` wins.
- [ ] `ListClients` returning an error ⇒ `detectInsideTmux` returns an error satisfying `errors.Is(err, ErrDetectTransient)`.
- [ ] `tmux.Client.ListClients` parses `"<pid> <activity>"` lines into `ClientInfo` correctly and tolerates the no-clients case as an empty slice.

**Tests**:
- `"it returns NULL when every client is remote or mosh"`
- `"it returns the single local client's identity without a tiebreak"`
- `"it picks the highest-client_activity local client among 2+ locals"`
- `"it returns a transient error when list-clients fails"`
- `"it drops remote clients but still resolves a mixed local+remote client set"`
- `"tmux.ListClients parses client_pid and client_activity lines"` (tmux package, fake Commander)

**Edge Cases**:
- Only remote/mosh clients → NULL (no host-local terminal).
- Single local client → no tiebreak.
- 2+ local clients → highest `client_activity` wins.
- `list-clients` failure → typed transient error.

**Context**:
> Spec *Terminal Identity & Detection → Detection model (items 2–4)*: "**Inside tmux:** take the current session's clients, **NULL-filter to local host clients** (drop mosh/remote/other-machine). The local client's app = the terminal. … **`client_activity` demoted to a local-only tiebreak** — used *only* to choose among 2+ local clients on the same session. … Never the primary cross-client signal. **Host-local principle** … A purely-remote trigger (no local client) → the honest 'no host-local terminal' no-op."
> Live-probed against ~33 clients (many lingering mosh/Blink): "`focused` and raw highest-`client_activity` proved **unreliable across clients**" — hence the walk-based NULL-filter first, activity only as the narrow local tiebreak.
> The per-client transient-walk-error policy (continue scanning; surface a transient error only when nothing local resolved) is the reasonable reading of "transient error distinct from clean NULL" applied to the multi-client loop — the spec names only the `list-clients` failure explicitly; this keeps a single flaky `ps` from masking a resolvable local client while still emitting the WARN when detection genuinely could not complete.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Detection model (items 2–4)*; *Testing Strategy & DI Seams → Detection behind small seams*.

---

## restore-host-terminal-windows-1-5 | approved

### Task 1.5: Detect orchestrator + spawn log component

**Problem**: The inside/outside detection paths must be composed into a single detect-self entry point that the `--detect` CLI (and, in later phases, the picker banner and the N≥2 gate) call. Detection also needs Portal's observability: a new spec-governed `spawn` log component that records the detection outcome and, on a transient failure, a WARN breadcrumb — folding transient errors into the same unsupported/NULL path as clean NULL.

**Solution**: Implement the `Detector` orchestrator that branches on inside/outside tmux, wires the real seams (real `ps`/`defaults`, real `list-clients`, `os.Getenv`, `os.Getpid`), returns the resolved-or-NULL `Identity`, and emits `spawn`-component logs: an INFO detection-outcome line for resolved and clean-NULL, plus a WARN on a transient error (which is then folded to NULL). Introduce the `spawn` log component bound via `log.For("spawn")`.

**Outcome**: `spawn.Detect()` returns an `Identity` (NULL when unsupported/remote/mosh/transient-error), having logged exactly the right `spawn`-component records: resolved ⇒ INFO with `terminal` + `bundle_id` (+ opaque `detail`); clean NULL ⇒ INFO NULL-bundle outcome, no WARN; transient error ⇒ WARN (with opaque `detail`) then a NULL identity. Logging behaviour is unit-tested via an injected `logtest.Sink`.

**Do**:
- Create `internal/spawn/detect.go`:
  - Bind the component logger: `var detectLogger = log.For("spawn")` (package-level, cached at init — safe before `log.Init`, following the `internal/state` `signal.go` / `internal/alias` `store.go` idiom). This introduces the new **`spawn`** component into Portal's closed logging taxonomy (spec-governed amendment — see Context; do not invent further `spawn` attr keys at call sites).
  - Define a `Detector` with unexported seam fields for testability: `insideTmux func() bool`, `getenv func(string) string`, `selfPID int`, `walker ProcessWalker`, `reader BundleReader`, `lister clientLister`, `currentSession func() (string, error)`, and `logger *slog.Logger`.
  - Add the production constructor `func NewDetector(client *tmux.Client) *Detector` wiring: `tmux.InsideTmux`, `os.Getenv`, `os.Getpid()`, the real `ProcessWalker`/`BundleReader` from Task 1.2, the `tmuxClientLister` adapter from Task 1.4, `client.CurrentSessionName`, and `detectLogger`.
  - Implement `func (d *Detector) Detect() Identity`:
    1. If `d.insideTmux()` ⇒ resolve the session name via `d.currentSession()` (on error ⇒ treat as a transient detection error), then `detectInsideTmux(session, d.lister, d.walker, d.reader)`. Else ⇒ `detectOutsideTmux(d.getenv, d.selfPID, d.walker, d.reader)`.
    2. If the result error satisfies `errors.Is(err, ErrDetectTransient)` (or the session read failed) ⇒ emit a `spawn` **WARN** carrying an opaque `detail` (the underlying error string), then `return Identity{}` (fold to NULL — same unsupported path).
    3. Else on a resolved non-NULL identity ⇒ emit a `spawn` **INFO** detection-outcome line with `terminal` = `id.Name`, `bundle_id` = `id.BundleID`, and an opaque `detail` (e.g. the detection route / resolved `.app` path — value opaque, presence asserted). Return the identity.
    4. Else (clean NULL) ⇒ emit a `spawn` **INFO** NULL-bundle detection-outcome line (no `terminal`/`bundle_id`, or empty), **no WARN**. Return `Identity{}`.
  - **Attr-key scope (Phase 1):** emit **only** the closed keys `terminal`, `bundle_id`, and the opaque `detail`. Do **not** emit `resolution`, `session`, `ack`, `opened`, `total`, or `batch` — those belong to later phases (adapter resolution / burst). The `pid`/`version`/`process_role` baseline attrs are injected by the handler; do not add them.
- Add `internal/spawn/detect_test.go` (unit lane): inject a `Detector` with fabricated seams and a `logtest.Sink`-backed `*slog.Logger` (`logtest.NewCaptureLogger`), then assert the emitted records' level, message, and attr-key set for each outcome.

**Acceptance Criteria**:
- [ ] Outside-tmux resolved path returns the resolved `Identity` and emits exactly one INFO `spawn` record carrying `terminal` and `bundle_id` (and a `detail`), no WARN.
- [ ] Inside-tmux clean-NULL path (only remote clients) returns `Identity{}` and emits an INFO NULL-bundle outcome with **no** WARN record.
- [ ] A transient detection error (fabricated `ps`/`list-clients` failure) returns `Identity{}` **and** emits one WARN `spawn` record carrying an opaque `detail`; the returned identity is indistinguishable (NULL) from the clean-NULL case.
- [ ] No emitted `spawn` record in Phase 1 carries `resolution`, `session`, `ack`, `opened`, `total`, or `batch` (attr-key scope guard).
- [ ] `NewDetector` compiles against a real `*tmux.Client` and branches on `tmux.InsideTmux()`.

**Tests**:
- `"it emits a resolved INFO with terminal and bundle_id and returns the identity"`
- `"it emits a NULL-bundle INFO with no WARN for a clean remote-only detection"`
- `"it folds a transient detection error to NULL and emits a spawn WARN"`
- `"it emits only the terminal, bundle_id and detail attr keys in phase 1"`
- `"it branches to inside-tmux detection when TMUX is set"`

**Edge Cases**:
- Transient error folds to unsupported and emits a `spawn` WARN.
- Clean NULL emits a NULL-bundle outcome with no WARN.
- Resolved identity emits `terminal` + `bundle_id` (+ opaque `detail`).

**Context**:
> Spec *Observability & State Footprint → Observability (`spawn` log component)*: "The spawn flow gets its **own `spawn` log component** — a deliberate amendment to Portal's closed logging taxonomy (a new spec-governed component, not a call-site invention). … **Attr keys (closed set).** The `spawn` component introduces these spec-governed attr keys … `batch`, `terminal` (friendly app name), `bundle_id` (matched bundle-id family), `resolution` … `session`, `ack` … `opened` / `total` … and the opaque `detail` (driver OS-specific string)." Phase 1 emits only `terminal`, `bundle_id`, `detail`; the rest arrive with adapter resolution and the burst (later phases).
> Spec *Detection lifecycle → Error vs clean NULL*: both clean NULL and a transient error "resolve to the unsupported/no-op path; a transient error additionally emits a `spawn`-component WARN breadcrumb."
> Spec *Detection is a standalone operation*: detection is separately callable (not buried in the spawn path) because the banner and `--detect` need identity without spawning; it is **not** an adapter method. CLAUDE.md — bind one component logger per package via `var logger = log.For("<component>")`; the `spawn` component is the new name added by this feature.
> Emission shape follows bootstrap/restore/daemon (INFO outcome breadcrumb, WARN reserved for the transient-error case); the exact message strings are added to the `spawn` closed event catalog (detection outcome: identity / unsupported / NULL-bundle).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Observability & State Footprint → Observability (`spawn` log component)*; *Detection lifecycle → Error vs clean NULL, In-flight*; *Detection is a standalone operation*.

---

## restore-host-terminal-windows-1-6 | approved

### Task 1.6: portal spawn command — --detect dry-run + usage-error gate

**Problem**: The feature needs its thin CLI (`portal spawn`) — the test seam and, in later phases, the entry point that mirrors the picker's commit. In Phase 1 the command exposes the `--detect` dry-run (print the detected terminal identity, open nothing) and the usage-error gate (no sessions + no `--detect`, and unknown flags, exit 2).

**Solution**: Add `cmd/spawn.go` registering a `portal spawn` Cobra command with a `--detect` bool flag, an injectable detector seam (`spawnDeps`), friendly/NULL print branches, and flag-error → `UsageError` wiring so unknown flags and the empty invocation exit 2.

**Outcome**: `portal spawn --detect` prints the friendly `.app` name + exact bundle id (or the honest "no host-local terminal" line) and exits 0; `portal spawn` with no args and no `--detect` returns a `*cmd.UsageError` (exit 2); an unknown flag returns a `*cmd.UsageError` (exit 2). Behaviour is unit-tested by Executing the command body with injected deps (no built binary, no real tmux).

**Do**:
- Create `cmd/spawn.go`:
  - Define the detector seam and deps: `type TerminalDetector interface { Detect() spawn.Identity }`, `type SpawnDeps struct { Detector TerminalDetector }`, and `var spawnDeps *SpawnDeps` (nil ⇒ production). Add a `buildSpawnDeps(cmd)` helper that returns `spawnDeps.Detector` when set, else `spawn.NewDetector(tmuxClient(cmd))`.
  - Define `spawnCmd` with `Use: "spawn [sessions...]"`, `SilenceUsage: true`, `SilenceErrors: true`, a `--detect` bool flag (`detect`), and `SetFlagErrorFunc(func(c *cobra.Command, err error) error { return NewUsageError(err.Error()) })` so unknown-flag parse errors map to exit 2 (main's `classify` maps `*cmd.UsageError` → 2).
  - `RunE`:
    1. If `--detect` is set ⇒ `id := buildSpawnDeps(cmd).Detector.Detect()`; if `id.IsNull()` ⇒ print the honest `no host-local terminal` line to `cmd.OutOrStdout()`; else ⇒ print the friendly name + exact bundle id (format `"%s · %s\n"`, `id.Name`, `id.BundleID`, echoing the design `Apple Terminal · com.apple.Terminal` separator). Return nil (exit 0).
    2. Else if `len(args) == 0` ⇒ `return NewUsageError("spawn: provide one or more sessions, or use --detect")` (exit 2).
    3. Else (session args, no `--detect`) ⇒ this is the spawn-burst path built in Phase 2; return a plain non-usage error (e.g. `errors.New("spawn: opening sessions is not yet available")`) as an explicit Phase-1 placeholder that Phase 2 replaces. (Not exercised by Phase 1 tests; keeps the increment honest.)
  - `func init() { rootCmd.AddCommand(spawnCmd) }`.
  - Note: `portal spawn` is intentionally **not** added to `skipTmuxCheck` in `root.go` — it runs its own bootstrap first, consistent with spec (*Concurrency & Post-Reboot Safety*: "A direct `portal spawn` CLI as the first post-reboot command runs its own bootstrap synchronously first"). Unknown-flag parse errors fire before `PersistentPreRunE`, so that gate needs no server.
- Add `cmd/spawn_test.go` (unit lane, package `cmd`, no `t.Parallel()`): inject `spawnDeps` with a fake `TerminalDetector` (restore via `t.Cleanup`), and set `bootstrapDeps` so `PersistentPreRunE` short-circuits (Orchestrator nil ⇒ no real tmux; the cmd `TestMain` poisons `TMUX`, so any missed injection fails loudly). Drive via `rootCmd` Execute with args, capturing stdout and asserting the returned error type.

**Acceptance Criteria**:
- [ ] `portal spawn --detect` with a fake detector returning `Identity{Name:"Ghostty", BundleID:"com.mitchellh.ghostty"}` prints a line containing both `Ghostty` and `com.mitchellh.ghostty` to stdout and returns nil (exit 0).
- [ ] `portal spawn --detect` with a fake detector returning `Identity{}` (NULL) prints a line containing `no host-local terminal` and returns nil.
- [ ] `portal spawn` (no args, no `--detect`) returns an error that `errors.As` matches `*cmd.UsageError` (main maps to exit 2).
- [ ] `portal spawn --bogus` returns an error that `errors.As` matches `*cmd.UsageError` (exit 2), via `SetFlagErrorFunc`.
- [ ] The command Executes with `bootstrapDeps` injected and never dials a real tmux server (no `TMUX`-poison connect failure).

**Tests**:
- `"it prints the friendly name and exact bundle id on --detect for a resolved terminal"`
- `"it prints the honest no-host-local-terminal line on --detect for a NULL identity"`
- `"it returns a UsageError when no sessions and no --detect are given"`
- `"it returns a UsageError for an unknown flag"`

**Edge Cases**:
- Resolved terminal → prints friendly `.app` name + exact bundle id.
- NULL (remote/mosh / no local client) → prints the honest "no host-local terminal" line.
- No sessions and no `--detect` → `UsageError` (exit 2).
- Unknown flag → exit 2.

**Context**:
> Spec *Spawn Architecture → `portal spawn` CLI behaviour*: "`portal spawn --detect` is a dry-run that only prints the detected terminal identity (friendly name + bundle id) and opens nothing. `portal spawn` with no session args and no `--detect` is a usage error." *Reporting & exit codes*: "**Usage error** (no sessions, no `--detect`; unknown flag) → **exit `2`**."
> Spec *User-facing display: both*: `--detect` shows both the friendly `.app` name and the exact bundle id (design copy `Apple Terminal · com.apple.Terminal`); the NULL case is the honest "no host-local terminal" no-op (*Detection model → Host-local principle*). The exact CLI wording is not pinned by the spec beyond containing both fields (resolved) / naming the no-host-local outcome (NULL).
> Scope boundary: the actual spawn burst (session args → pre-flight → sequential spawn → self-attach) is **Phase 2+**; Phase 1 wires only `--detect` and the usage gate. The command retrieves the tmux client via `tmuxClient(cmd)` (populated by bootstrap into `cmd.Context()`) for inside-tmux detection. `UsageError` and main's exit-code `classify` already exist (`cmd/errors.go`, `main.go`); this task reuses them, adding the `SetFlagErrorFunc` bridge so cobra flag errors reach the exit-2 path. CLAUDE.md — cmd tests inject every tmux-touching `*Deps` seam; `TestMain` `TMUX` poison catches a missed injection.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Spawn Architecture → `portal spawn` CLI behaviour / Reporting & exit codes*; *Terminal Identity & Detection → User-facing display: both / Host-local principle*; *Concurrency & Post-Reboot Safety*.
