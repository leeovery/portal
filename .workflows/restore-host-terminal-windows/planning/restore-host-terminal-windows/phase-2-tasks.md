---
phase: 2
phase_name: Spawn Execution Core
total: 7
---

## restore-host-terminal-windows-2-1 | approved

### Task 2.1: Adapter interface, typed result taxonomy & fake adapter seam

**Problem**: The spawn layer must open host-terminal windows through pluggable per-terminal drivers, but every OS/terminal specific detail (AppleScript, `osascript`, AppleEvent codes, TCC) must stay quarantined inside the driver — Portal's general spawn/report/UI code must switch on a small generic category and *never* see an AppleScript string or an AppleEvent number. Nothing exists yet: there is no adapter contract, no typed result taxonomy, and no fake seam for unit-testing the pipeline without a real terminal.

**Solution**: Define the `Adapter` interface (single capability: open a window running a given command) and the generic typed `Result` taxonomy (four distinguishable outcomes plus an opaque `detail`/`guidance` payload that general code never inspects) in `internal/spawn`, and a test-only `FakeAdapter` (in `internal/spawntest`) that records the exact composed argv handed to it and returns scripted results — the primary seam that makes the whole spawn pipeline unit-testable.

**Outcome**: `internal/spawn` exposes `Adapter.OpenWindow(command []string) Result` and a `Result` type whose `Outcome` distinguishes `success` / `unsupported` / `spawn-failed` / `permission-required`, carrying OS-specific text only in an opaque `Detail` (and `Guidance` for the permission case) that general code passes through verbatim. `internal/spawntest.FakeAdapter` satisfies `spawn.Adapter`, records each `OpenWindow` argv in call order, and returns a scripted `Result` per call. All logic is unit-tested with fabricated inputs; no real terminal is touched.

**Do**:
- Create `internal/spawn/adapter.go`:
  - Define the single-capability contract: `type Adapter interface { OpenWindow(command []string) Result }`. `command` is the composed env-self-sufficient attach argv built by Task 2.3 (`/usr/bin/env … <exe> attach <session>`); the adapter runs it verbatim as a real argv — it is **not** session-aware and never bakes in `portal attach` (spec rejects a session-aware `OpenAttached(session)`).
  - Define the outcome enum: `type Outcome int` with `OutcomeSuccess`, `OutcomeUnsupported`, `OutcomeSpawnFailed`, `OutcomePermissionRequired` — all four must be distinct values (the taxonomy the spec's Permissions & Error Quarantine fixes).
  - Define `type Result struct { Outcome Outcome; Detail string; Guidance string }` where `Detail` is the **opaque** OS-specific string (e.g. an `osascript` error body) that rides up only as a log `detail` attr, and `Guidance` is the opaque permission-guidance text (target terminal + Automation-settings hint) composed *in the driver* — populated only by the permission path (mapping deferred to Phase 3, but the field exists so later phases surface it without a taxonomy change).
  - Add constructors that keep call sites terse and self-documenting: `func Success(detail string) Result`, `func Unsupported(detail string) Result`, `func SpawnFailed(detail string) Result`, `func PermissionRequired(detail, guidance string) Result`.
  - Add `func (r Result) OK() bool { return r.Outcome == OutcomeSuccess }` — the single predicate general code uses to gate self-attach; it never reads `Detail`/`Guidance`.
- Create `internal/spawntest/adapter.go` (test-only package; production code must not import it, mirroring `transienttest`/`restoretest`):
  - `type FakeAdapter struct { Calls [][]string; Results []spawn.Result }` (guard the recording with a `sync.Mutex` for safety even though spawn is sequential).
  - `func (f *FakeAdapter) OpenWindow(command []string) spawn.Result` — append a copy of `command` to `Calls`, then return the next scripted entry from `Results` (consumed in order); when `Results` is exhausted or empty, default to `spawn.Success("")`. This lets a test script "second window fails" by setting `Results = []spawn.Result{Success, SpawnFailed}`.
  - Add a package doc noting it is the primary DI seam (spec *Testing Strategy → Primary seam: the `Adapter` interface*).
- Add `internal/spawn/adapter_test.go` (unit lane) asserting the four constructors produce four distinct `Outcome`s, `OK()` is true only for `Success`, and `Detail`/`Guidance` round-trip. Add a `FakeAdapter` self-test (`package spawntest_test` to avoid an import cycle) asserting it records argv in order and returns scripted results.

**Acceptance Criteria**:
- [ ] `internal/spawn` and `internal/spawntest` compile; `go test ./internal/spawn/... ./internal/spawntest/...` passes.
- [ ] `Success`, `Unsupported`, `SpawnFailed`, `PermissionRequired` yield four distinct `Outcome` values; only `Success(...).OK()` is `true`.
- [ ] `PermissionRequired("evt -1743", "grant Automation for Ghostty")` round-trips both `Detail` and `Guidance` unchanged.
- [ ] `FakeAdapter.OpenWindow` records the exact `command` argv passed (a defensive copy) into `Calls` in call order, and returns the scripted `Results[i]` for call `i`, defaulting to `Success` once exhausted.
- [ ] No field or method on `Result` requires general code to parse `Detail`/`Guidance` to classify an outcome — `Outcome` alone is sufficient.

**Tests**:
- `"it distinguishes all four outcomes with distinct enum values"`
- `"it reports OK only for the success outcome"`
- `"it round-trips opaque detail and guidance without interpretation"`
- `"the fake adapter records the exact composed argv per call in order"`
- `"the fake adapter returns scripted results per call and defaults to success when exhausted"`

**Edge Cases**:
- OS-specific detail is carried only in the opaque `Detail` (and permission `Guidance`) and is never leaked to / parsed by general code — classification is on `Outcome` alone.
- All four outcomes (success + unsupported / spawn-failed / permission-required) are distinguishable at the type level.
- The fake records the exact composed argv (defensive copy so a later mutation of the caller's slice cannot corrupt the record).

**Context**:
> Spec *Adapter Contract & Extensibility → Generic contract*: "The adapter's single job is **open a new host window running a given command** — `OpenWindow(command)` — **not** 'attach to a session.' … Rejected: a session-aware `OpenAttached(session)`." *Two implementations, same contract*: "quarantining all OS/terminal specifics behind a typed result."
> Spec *Permissions & Error Quarantine → Architectural boundary*: "All terminal/OS-specific concerns … live **inside the terminal driver and nowhere else**. The driver translates them into a **generic typed result** — a small taxonomy: `permission-required` (with guidance text), `unsupported`, `spawn-failed`. Portal's general spawn/report/UI code switches on the category and **never sees an AppleScript string or AppleEvent number**."
> Spec *Testing Strategy → Primary seam*: "A **fake adapter** records 'would open a window running command X' without touching a real terminal → the entire spawn pipeline is unit-testable."
> Scope: the `-1712`/`-1743` → `permission-required` *mapping* is deferred to Phase 3 (this task only defines the `permission-required` member + `Guidance` field so the taxonomy is complete). The composed-argv shape is finalised in Task 2.3; here it is just `[]string`.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Adapter Contract & Extensibility → Generic contract / Two implementations*; *Permissions & Error Quarantine → Architectural boundary*; *Testing Strategy & DI Seams → Primary seam*; *Observability & State Footprint → Attr keys (`detail`)*.

---

## restore-host-terminal-windows-2-2 | approved

### Task 2.2: Adapter resolver — identity → native Ghostty adapter / unsupported

**Problem**: Detection (Phase 1) yields a terminal `Identity`, but nothing yet maps that identity to an `Adapter`. The spawn pipeline needs a resolver that turns an identity into "here is the driver that opens windows for this terminal" or "unsupported — no driver," and it needs to classify the resolution (`native` vs `unsupported`) so the pipeline can both log it and gate the N≥2 no-op. In Phase 2 only the built-in Ghostty driver exists; the `terminals.json` config tier is Phase 4.

**Solution**: Add `spawn.ResolveAdapter(id Identity) (Adapter, Resolution)` — a precedence resolver that (for now) matches the identity's bundle id against a native-adapter registry keyed by bundle-id *family* (Phase 1's `MatchesFamily`), returns the native Ghostty adapter on a family hit, and returns a nil adapter + `ResolutionUnsupported` for a NULL identity, a known-but-undriven identity, or any passthrough/unknown identity. A commented placeholder marks where the Phase-4 config override slots in ahead of native.

**Outcome**: `ResolveAdapter` returns `(*ghosttyAdapter, ResolutionNative)` for a Ghostty-family bundle id (including a channel-suffixed variant), and `(nil, ResolutionUnsupported)` for NULL / `com.apple.Terminal` / any unknown identity — never a non-nil adapter with `unsupported`. The mapping is fully unit-tested with fabricated identities; no adapter is executed.

**Do**:
- Create `internal/spawn/resolver.go`:
  - Define `type Resolution string` with `ResolutionNative Resolution = "native"`, `ResolutionConfig Resolution = "config"`, and `ResolutionUnsupported Resolution = "unsupported"` — the string values are exactly the `resolution` log-attr vocabulary from the spec's closed attr set (`config | native | unsupported`), so the resolution maps straight onto the log attr.
  - Define a native-adapter registry as an ordered slice of `{ family string; build func() Adapter }` entries. Phase 2 has exactly one entry: family `"com.mitchellh.ghostty*"` (glob per `MatchesFamily`, so both `com.mitchellh.ghostty` and any future channel-suffixed variant resolve), `build` = `func() Adapter { return newGhosttyAdapter() }` (the constructor from Tasks 2.4/2.5, wired with the real `osascript` runner).
  - Implement `func ResolveAdapter(id Identity) (Adapter, Resolution)`:
    1. `if id.IsNull() { return nil, ResolutionUnsupported }` (remote/mosh / no host-local terminal).
    2. **Phase-4 placeholder**: a comment marking where a `terminals.json` config-override lookup will run *first* (config → native → unsupported precedence). No config read in Phase 2.
    3. For each native registry entry, `if MatchesFamily(id.BundleID, entry.family) { return entry.build(), ResolutionNative }`.
    4. No match ⇒ `return nil, ResolutionUnsupported` (known-but-undriven like `com.apple.Terminal`, or any passthrough/unknown identity).
  - Keep the resolver pure (no logging, no I/O) — the pipeline (Task 2.6) logs the `resolution` attr. Constructing the Ghostty adapter must not touch tmux/`osascript`; only `OpenWindow` does.
- Add `internal/spawn/resolver_test.go` (unit lane, `package spawn`): fabricate identities via `NewIdentity`, assert the returned `Resolution` and whether the adapter is nil / a `*ghosttyAdapter` (type-assert). No `OpenWindow` call.

**Acceptance Criteria**:
- [ ] `ResolveAdapter(NewIdentity("com.mitchellh.ghostty", "Ghostty"))` returns a non-nil `*ghosttyAdapter` and `ResolutionNative`.
- [ ] A channel-suffixed Ghostty bundle id (e.g. `NewIdentity("com.mitchellh.ghostty.debug", "")`) resolves to `(*ghosttyAdapter, ResolutionNative)` via the family glob.
- [ ] A NULL identity (`NewIdentity("", "")`) returns `(nil, ResolutionUnsupported)`.
- [ ] A known-but-undriven identity (`NewIdentity("com.apple.Terminal", "Apple Terminal")`) returns `(nil, ResolutionUnsupported)`.
- [ ] A passthrough/unknown identity (`NewIdentity("com.example.MyTerm", "")`) returns `(nil, ResolutionUnsupported)`.
- [ ] The resolver never returns a non-nil adapter together with `ResolutionUnsupported`, and never returns `ResolutionConfig` in Phase 2.

**Tests**:
- `"it resolves a Ghostty bundle id to the native adapter"`
- `"it resolves a channel-suffixed Ghostty bundle id via family match"`
- `"it returns unsupported for a NULL identity"`
- `"it returns unsupported for a known terminal with no native adapter"`
- `"it returns unsupported for a passthrough unknown identity"`

**Edge Cases**:
- Channel-suffixed Ghostty bundle id → native via `MatchesFamily`.
- NULL identity → unsupported.
- Known-but-no-native-adapter (e.g. `com.apple.Terminal`) → unsupported.
- Passthrough/unknown identity → unsupported.

**Context**:
> Spec *Adapter Contract & Extensibility → Detection is separate from the adapter / Resolution precedence*: "Detect-self resolves *identity*; a **resolver** maps identity → adapter via the precedence chain … **config override → native adapter → unsupported.** … A NULL/unmatched identity → unsupported." Detection is **not** an adapter method.
> Spec *Config Schema → Precedence*: "config override → native adapter → unsupported. Config can override a built-in." Phase 4 inserts the `terminals.json` tier ahead of native; Phase 2 ships the native→unsupported stub, with the config step marked as a placeholder.
> Spec *Observability → Attr keys*: `resolution` is a closed attr with values `config | native | unsupported`; `Resolution`'s string values must match so the pipeline logs it directly.
> Ghostty's real bundle id is `com.mitchellh.ghostty` (validated live via the `com.mitchellh.ghostty` TCC reset, Phase 1 `ghosttyBundleID`). The family glob (`com.mitchellh.ghostty*`) is used for channel-awareness uniformity with the spec's `dev.warp.Warp-*` example even though Ghostty stable carries no channel suffix.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Adapter Contract & Extensibility → Detection is separate from the adapter / Resolution precedence / Two implementations*; *Config Schema → Precedence*; *Observability & State Footprint → Attr keys (`resolution`)*.

---

## restore-host-terminal-windows-2-3 | approved

### Task 2.3: Env-self-sufficient attach command composition

**Problem**: A host terminal launches the spawned command in a **bare environment** — Ghostty execs an argv (not a shell) in a bare `PATH` with no Homebrew/login `PATH`, so a spawned `portal attach` would fail to find `tmux`. Worse, a picker triggered from *inside* tmux must not leak `TMUX`/`TMUX_PANE` into the spawned window, or the fresh window's `portal attach` would take the `switch-client` path instead of a clean out-of-tmux exec-attach. The spawn layer therefore needs one uniform, env-self-sufficient command composition that every adapter (native or config) runs verbatim.

**Solution**: Add `internal/spawn/command.go` composing the attach command as an explicit `env`-prefixed argv — `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<picker's full PATH> <os.Executable()> attach <session>` — a real argv (never shell syntax) with the session name as a discrete element, injecting only `PATH` (no whole-env snapshot) and explicitly unsetting `TMUX`/`TMUX_PANE`. The executable path is resolved via an injectable seam so an `os.Executable()` failure is surfaced.

**Outcome**: `spawn.AttachCommand(session, exe, getenv)` returns the composed `[]string` argv or a wrapped error when the executable cannot be resolved. The argv strips `TMUX`/`TMUX_PANE`, injects only `PATH`, uses the picker's own binary path (not a bare `portal` PATH lookup), and keeps a space-containing session name as one unquoted element. `--spawn-ack` is **not** included (Phase 3). Fully unit-tested with fabricated `exe`/`getenv` seams.

**Do**:
- Create `internal/spawn/command.go`:
  - `type ExecutableResolver func() (string, error)` — the seam for `os.Executable`; production callers pass `os.Executable`.
  - Unexported pure builder `func composeAttachArgv(exePath, path, session string) []string` returning exactly:
    `[]string{"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE", "PATH=" + path, exePath, "attach", session}`.
    Rationale to encode in a comment: `-u TMUX -u TMUX_PANE` is the explicit strip (load-bearing — the spawned N−1 must run out of tmux); `PATH=<path>` is the *only* injected var (no whole-env snapshot); `exePath` is the picker's own absolute binary so the version-gated warm-command latch stays satisfied and each spawned attach takes the abridged fast-path; the session is a discrete argv element so a space never needs shell quoting.
  - `func AttachCommand(session string, exe ExecutableResolver, getenv func(string) string) ([]string, error)`:
    1. `p, err := exe()`; on error ⇒ `return nil, fmt.Errorf("spawn: resolve executable path: %w", err)` (surface, do not swallow).
    2. `return composeAttachArgv(p, getenv("PATH"), session), nil`.
  - Do **not** append `--spawn-ack <batch>:<token>` — the ack carrier and the `portal attach` `--spawn-ack` flag are Phase 3. Leave a one-line comment marking where the ack token will be appended.
- Add `internal/spawn/command_test.go` (unit lane): drive `AttachCommand` with a fabricated `exe` (returning a fixed absolute path, and separately an error) and a `getenv` backed by a `map[string]string` (including a `TMUX` and `TMUX_PANE` entry to prove they are never propagated as assignments).

**Acceptance Criteria**:
- [ ] `AttachCommand("proj-abc123", func()(string,error){return "/abs/portal",nil}, mapGetenv{"PATH":"/opt/homebrew/bin:/usr/bin"})` returns `["/usr/bin/env","-u","TMUX","-u","TMUX_PANE","PATH=/opt/homebrew/bin:/usr/bin","/abs/portal","attach","proj-abc123"]`.
- [ ] The argv contains exactly one `PATH=` assignment and **no** `TMUX=` / `TMUX_PANE=` assignment (only the two `-u` unsets), even when `getenv` reports a live `TMUX`/`TMUX_PANE` (composed-from-inside-tmux case).
- [ ] A session name containing a space (`"my session"`) is a single, unquoted argv element at the tail — no shell quoting is added anywhere.
- [ ] The argv uses the resolved absolute executable path, not the literal string `portal`.
- [ ] When `exe()` returns an error, `AttachCommand` returns `nil` and a non-nil error wrapping it (`errors.Is` reaches the injected sentinel).
- [ ] The argv does **not** contain `--spawn-ack` (Phase-2 scope).

**Tests**:
- `"it composes env -u TMUX -u TMUX_PANE PATH=<full> <exe> attach <session>"`
- `"it injects only PATH and strips TMUX/TMUX_PANE even when composed inside tmux"`
- `"it keeps a session name with spaces as a single unquoted argv element"`
- `"it uses the resolved executable path rather than a bare portal lookup"`
- `"it surfaces an os.Executable resolution error"`
- `"it omits --spawn-ack in phase 2"`

**Edge Cases**:
- `TMUX`/`TMUX_PANE` stripped even when composed from inside tmux (via `-u`, and by never snapshotting the whole env).
- Only `PATH` injected — no whole-env snapshot.
- Session name with spaces stays a discrete argv element (no shell quoting).
- `os.Executable()` error surfaced (not swallowed, not defaulted to `"portal"`).
- `--spawn-ack` deferred to Phase 3.

**Context**:
> Spec *Spawn Architecture → Spawned-window environment (PATH injection)*: "the picker builds the spawned command as an **env-self-sufficient argv** — it prefixes a **minimal, explicit env** (`/usr/bin/env PATH=<picker's full PATH>`) ahead of `<os.Executable()> attach <session> --spawn-ack <batch>:<token>` … **Inject the minimal set, not a snapshot of the picker's whole environment** — `PATH` is the only required var. … **Load-bearing invariant: `TMUX` (and `TMUX_PANE`) MUST NOT be propagated** … a picker triggered from *inside* tmux therefore explicitly strips `TMUX`/`TMUX_PANE`. … the composed command carries its own env … a real **argv**, never shell syntax."
> Spec *Spawn Architecture → Command composition*: "The N−1 windows spawn running **`<os.Executable()> attach <session>`** — the picker's own absolute binary path, **not** a bare `portal` PATH lookup. … Using the picker's own binary guarantees version parity → latch satisfied → each attach takes the abridged fast-path."
> Scope: the `--spawn-ack <batch>:<token>` suffix and the `portal attach` `--spawn-ack` contract are Phase 3; Phase 2 composes only `… <exe> attach <session>`. `os.Executable()` returns the absolute picker binary path (macOS resolves it via the process image). BSD `/usr/bin/env` on macOS supports `-u name` to unset a variable, so the explicit strip is a real, testable argv fragment robust to whatever ambient env the host terminal provides.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Spawn Architecture → Command composition / Spawned-window environment (PATH injection)*; *Adapter Contract & Extensibility → Two implementations (env not an adapter concern)*.

---

## restore-host-terminal-windows-2-4 | approved

### Task 2.4: Ghostty driver — pure osascript command construction

**Problem**: The native Ghostty driver must open a new Ghostty window that runs the composed attach argv, via `osascript`. Building the AppleScript is fiddly (a `surface configuration` record with a `command` and a `wait after command` property, then `new window`) and the composed argv must be embedded with correct AppleScript-string escaping. This construction must be a pure, unit-testable function so it can be asserted without ever launching `osascript` or opening a real window.

**Solution**: Add the pure command-construction half of `internal/spawn/ghostty.go`: a function that renders the validated (Ghostty 1.3.1) AppleScript embedding the composed argv with proper escaping and a present `wait after command` property, and a function that wraps it into the `osascript -e <script>` argv. No execution.

**Outcome**: `ghosttyOpenArgv(command []string)` returns `["osascript", "-e", <script>]` where `<script>` builds a `surface configuration` whose `command` is the AppleScript-escaped composed argv, includes the `wait after command` property (normal-detach window lifecycle), and opens a `new window` with it. Asserted purely (no `osascript` run).

**Do**:
- In `internal/spawn/ghostty.go` add the construction layer (the driver struct + exec boundary are Task 2.5):
  - `func ghosttyEmbed(command []string) string` — render the composed argv into the single string Ghostty's `command` property expects: join the argv elements with single spaces, then AppleScript-string-escape the result (`\` → `\\`, then `"` → `\"`) so it embeds safely inside the double-quoted AppleScript string literal. (The composed argv from Task 2.3 is the real thing to run; Ghostty execs it as an argv in a bare `PATH`, which the `env PATH=…` prefix makes self-sufficient.)
  - `func ghosttyOpenScript(command []string) string` — build the AppleScript (validated shape, Ghostty 1.3.1): `tell application "Ghostty"` → make a `surface configuration` record with properties `{command:"<ghosttyEmbed(command)>", wait after command:true}` → `new window` using that configuration → `end tell`. The `wait after command` property MUST be present (it governs whether the window persists after its command exits — the normal-detach window lifecycle for a spawned session).
  - `func ghosttyOpenArgv(command []string) []string` — `return []string{"osascript", "-e", ghosttyOpenScript(command)}`.
- Add `internal/spawn/ghostty_command_test.go` (unit lane): assert on the built script/argv without executing.

**Acceptance Criteria**:
- [ ] `ghosttyOpenArgv(cmd)[0] == "osascript"` and `[1] == "-e"`; `[2]` is the script.
- [ ] The script contains a `surface configuration` reference, embeds the composed command inside the `command:` property, contains `wait after command`, and issues `new window`.
- [ ] The embedded command is AppleScript-escaped: an input element containing a `"` appears as `\"` in the script, and a `\` appears as `\\`; no unescaped `"` from the payload prematurely closes the AppleScript string literal.
- [ ] `ghosttyOpenScript` is pure — calling it twice with the same input yields identical output and performs no I/O / no process exec.
- [ ] The composed argv (`/usr/bin/env … attach <session>`) is embedded verbatim (post-escape) so the window runs exactly that command.

**Tests**:
- `"it wraps the script as osascript -e <script>"`
- `"it builds a surface configuration with a command property and new window"`
- `"it includes the wait after command property"`
- `"it AppleScript-escapes embedded double quotes and backslashes in the composed command"`
- `"it embeds the composed attach argv verbatim after escaping"`

**Edge Cases**:
- `wait after command` present (normal-detach window lifecycle).
- Composed argv embedded with correct AppleScript-string escaping (quotes/backslashes).
- Asserts the built command without running `osascript`.

**Context**:
> Spec *Dependencies … → Build-time residuals*: "**Ghostty AppleScript API** … Real shape (validated on 1.3.1): make a `surface configuration` record with a `command` property (and a `wait after command` property governing whether the window persists after its command exits — the normal-detach window lifecycle for a spawned session), then `new window` with it."
> Spec *Testing Strategy → Driver split for testability*: "**Pure command-construction** (building the `osascript`/argv) — unit-tested (assert the built command)."
> Spec *Config Schema → Recipe execution contract*: "`{command}` substitutes as a single, already-resolved command string, dropped literally into the recipe. Escaping it for an *embedding* context (e.g. inside an AppleScript string) is the recipe author's responsibility" — here the Ghostty driver is that author, so it owns the AppleScript escaping.
> Build-time residuals to carry (not blockers): the exact `wait after command` boolean and whether Ghostty word-splits the `command` string are confirmed against real Ghostty in Task 2.5's manual gate; Portal-generated session names (`{project}-{nanoid}`) contain no spaces, so the space-join embedding is safe in practice. The Ghostty AppleScript is a preview API (may churn in 1.4) — pin/watch per the spec residual.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Dependencies, Deferred Scope & Build-Time Residuals → Build-time residuals (Ghostty AppleScript API)*; *Testing Strategy & DI Seams → Driver split for testability (pure command-construction)*; *Config Schema → Recipe execution contract (embedding escape responsibility)*.

---

## restore-host-terminal-windows-2-5 | approved

### Task 2.5: Ghostty driver — thin exec boundary + outcome mapping (manual live-Mac)

**Problem**: The Ghostty driver's construction (Task 2.4) must be joined to a thin `osascript` execution boundary that runs the built argv and maps the result into the generic typed `Result` — success on a clean exit, `spawn-failed` on a non-zero exit — keeping the OS-specific text quarantined in the opaque `detail`. The real `osascript` + real window is only verifiable on a live Mac, so the outcome mapping must be split out and unit-tested with a fabricated `osascript` outcome, while the real exec is a manual gate.

**Solution**: Add the `ghosttyAdapter` struct implementing `Adapter.OpenWindow` over an injectable `osascript` runner seam, plus a pure outcome-mapping function (clean exit → `Success(detail)`, non-zero exit → `SpawnFailed(detail)`), unit-tested with a fake runner; the real `osascript` boundary is exercised only by a manual live-Mac verification (not automated CI). Permission-code mapping (`-1712`/`-1743`) is deferred to Phase 3.

**Outcome**: `newGhosttyAdapter()` returns a `*ghosttyAdapter` whose `OpenWindow(command)` builds the argv (Task 2.4), runs it through the runner seam, and returns `Success` with an opaque `detail` on a clean exit or `SpawnFailed` with the opaque combined output on a non-zero exit. The mapping is unit-tested with fabricated runner outcomes; a documented manual step confirms a real Ghostty window actually opens.

**Do**:
- In `internal/spawn/ghostty.go`:
  - Define the runner seam: `type osascriptRunner interface { Run(argv []string) (out string, exitCode int, err error) }` (small, 1-method, Portal DI style). `out` is the combined stdout+stderr; `exitCode` is the process exit status; `err` is a non-exit execution error (e.g. `osascript` not found).
  - Real impl `type execOsascriptRunner struct{}` using `exec.Command(argv[0], argv[1:]...)` through `log.CombinedOutputWithContext` (the stderr-preserving boundary helper), deriving `exitCode` from an `*exec.ExitError` (0 on success).
  - `type ghosttyAdapter struct { runner osascriptRunner }` and `func newGhosttyAdapter() *ghosttyAdapter { return &ghosttyAdapter{runner: &execOsascriptRunner{}} }` (the constructor the Task 2.2 registry calls).
  - Pure mapping `func mapGhosttyResult(out string, exitCode int, err error) Result`:
    - `err == nil && exitCode == 0` ⇒ `Success(detail)` where `detail` is an opaque string (e.g. `"ghostty osascript exit 0"` or the trimmed `out`).
    - otherwise ⇒ `SpawnFailed(detail)` where `detail` is the opaque combined output / error text. **No** `-1712`/`-1743` → `permission-required` branch here — that mapping is Phase 3; in Phase 2 every non-clean exit is `spawn-failed`.
  - `func (g *ghosttyAdapter) OpenWindow(command []string) Result` ⇒ `out, code, err := g.runner.Run(ghosttyOpenArgv(command)); return mapGhosttyResult(out, code, err)`.
- Add `internal/spawn/ghostty_openwindow_test.go` (unit lane) with a fake `osascriptRunner` that records the argv it was handed and returns fabricated `(out, exitCode, err)` — assert the mapped `Result.Outcome` and that `Detail` carries the opaque text; assert the runner received `ghosttyOpenArgv(command)`.
- Add a **manual verification note** (a `//go:build manual`-guarded test or a documented `MANUAL:` comment + steps in the test file) that, on a live Mac inside Ghostty, `newGhosttyAdapter().OpenWindow(<a real attach argv>)` actually opens a new Ghostty window running the command — the irreducible live-terminal inch. This is **not** part of the unit lane and must not run in `go test ./...`.

**Acceptance Criteria**:
- [ ] `OpenWindow` passes exactly `ghosttyOpenArgv(command)` to the runner (asserted via the fake runner's recorded argv).
- [ ] A fabricated clean exit (`err=nil`, `exitCode=0`) maps to `OutcomeSuccess` with a non-empty opaque `Detail`.
- [ ] A fabricated non-zero exit (e.g. `exitCode=1`, `out="…AppleScript error…"`) maps to `OutcomeSpawnFailed` with the opaque `Detail` carrying that output.
- [ ] A fabricated execution error (`osascript` not found) maps to `OutcomeSpawnFailed` (never a panic, never `Success`).
- [ ] No unit test executes real `osascript`; the real-window verification is manual/live-Mac only and excluded from the default lanes.
- [ ] The mapping never returns `OutcomePermissionRequired` in Phase 2 (permission-code mapping deferred).

**Tests**:
- `"it hands the osascript argv to the runner"`
- `"it maps a clean osascript exit to success with an opaque detail"`
- `"it maps a non-zero osascript exit to spawn-failed with the opaque output"`
- `"it maps an osascript execution error to spawn-failed"`
- `"MANUAL: it opens a real Ghostty window on a live Mac"` (manual-gated, not in the unit lane)

**Edge Cases**:
- Non-zero `osascript` exit → `spawn-failed`.
- Clean exit → `success` with opaque `detail`.
- Real window actually opens — live-Mac manual, not automated.
- Permission-code mapping (`-1712`/`-1743` → `permission-required`) deferred to Phase 3.

**Context**:
> Spec *Testing Strategy → Driver split for testability*: each terminal driver splits into "**Pure command-construction** … **Error-mapping** (`-1712`/`-1743` → typed `permission-required`) — unit-tested (fabricated `osascript` outcome; assert the mapped typed result) — **Thin exec boundary** (real `osascript` + TCC modal) — manual/integration-gated only." *Irreducible manual/integration residue*: "The real window actually opening + the TCC modal need a live Mac — covered by manual verification … not automated CI."
> Spec *Permissions & Error Quarantine → Defensive net*: the `-1743`/`-1712` recognition returning `permission-required` is a defensive net — but the *code mapping* is deferred to Phase 3 per this phase's scope; Phase 2's mapping distinguishes only success vs spawn-failed. TCC is self-exempt in the normal flow (Ghostty-scripting-Ghostty), so the defensive path is rare.
> Spec *Observability → `detail`*: "The driver's OS-specific detail rides up as an opaque `detail` attr so the closed vocabulary stays intact (honours the driver-quarantine rule)." The `osascript` output stays inside `Result.Detail`; general code never parses it.
> `log.CombinedOutputWithContext` is the codebase's stderr-preserving `exec.Cmd` boundary helper (per CLAUDE.md's `internal/log` role). Build-time residual: iTerm2 / Terminal.app self-scripting exemption is assumed same-exempt (per-adapter check) — not built here (Ghostty only).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Testing Strategy & DI Seams → Driver split for testability / Irreducible manual/integration residue*; *Permissions & Error Quarantine → Defensive net / TCC is self-exempt*; *Observability & State Footprint → `detail`*.

---

## restore-host-terminal-windows-2-6 | approved

### Task 2.6: `portal spawn <sessions…>` pipeline — sequential spawn N−1 + self-attach Nth

**Problem**: Phase 1 wired `portal spawn --detect` and the usage gate but left the session-args branch a placeholder. The core of the feature is now needed: `portal spawn <sessions…>` must detect the host terminal, resolve its adapter, spawn the N−1 external windows sequentially (each running the composed attach command through the adapter), and — only if every external spawn returns success — self-attach its own calling terminal window to the Nth session (net-N windows, never N+1). This is also the faithful CLI test seam the picker (Phase 5/6) reuses.

**Solution**: Add `spawn.SpawnWindows` (sequential, one adapter call per session, stop on first non-success) and replace the Phase-1 placeholder branch in `cmd/spawn.go` with the pipeline: detect → resolve → split N vs N−1 → `SpawnWindows` the N−1 → gate self-attach on all-success via the existing `AttachConnector`/`SwitchConnector` (`buildSessionConnector`). Extend `SpawnDeps` with `Resolve`, `Connector`, `ExePath`, and `Getenv` seams so the whole pipeline is unit-testable through the fake adapter and a fake connector. Self-attach is gated on adapter-returned-success only (token ack is Phase 3); on any non-success the pipeline skips self-attach and exits 1.

**Outcome**: `portal spawn s1 s2 s3` (supported terminal) opens two host windows for `s1`,`s2` in arg order via the adapter and then self-attaches `s3` (self-execs away). `portal spawn s1` (N=1) opens zero windows and self-attaches `s1` directly regardless of terminal. Any adapter `OpenWindow` non-success skips the self-attach and returns a plain error → exit 1 with a one-line stderr message naming the failed session. Fully unit-tested via `spawntest.FakeAdapter` + a fake connector (no real `osascript`, no real tmux).

**Do**:
- Add `internal/spawn/burst.go`:
  - `type SpawnOutcome struct { Session string; Result Result }`.
  - `func SpawnWindows(adapter Adapter, sessions []string, exe ExecutableResolver, getenv func(string) string) ([]SpawnOutcome, error)`:
    1. Resolve the executable **once**: build the first command via `AttachCommand` machinery, or call `exe()` up front and surface its error (`return nil, err`) — an unresolvable executable aborts the whole burst before any window opens.
    2. Iterate `sessions` **in list order**, sequentially (one `OpenWindow` completes before the next fires): compose the argv (`composeAttachArgv`/`AttachCommand`), call `adapter.OpenWindow(argv)`, append `SpawnOutcome{session, result}`.
    3. **Stop on the first non-success** result and return the outcomes collected so far (the failed one last). (Phase 2 keeps failure handling minimal — the detailed leave-what-opened / continue-and-retry-the-missing-set behaviour is Phase 3.)
    4. Return `(outcomes, nil)` when all succeed or `sessions` is empty (N=1 external set).
- Replace the Phase-1 placeholder in `cmd/spawn.go` (the `len(args) > 0 && !detect` branch) with the pipeline, and extend the deps:
  - Extend `SpawnDeps`: add `Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` (default `spawn.ResolveAdapter`), `Connector SessionConnector` (default `buildSessionConnector(tmuxClient(cmd))`), `ExePath spawn.ExecutableResolver` (default `os.Executable`), `Getenv func(string) string` (default `os.Getenv`). `Detector` stays from Phase 1.
  - Pipeline (`runSpawn(cmd, args, deps)`):
    1. `sessions := args`; `n := len(sessions)`. Split by the **list-order** convention (which one the trigger becomes is spec-unspecified implementation-convenience): `external := sessions[:n-1]`, `trigger := sessions[n-1]`.
    2. Detect: `id := deps.Detector.Detect()` (order is load-bearing: detect first).
    3. Resolve: `adapter, resolution := deps.Resolve(id)`.
    4. **(N≥2 unsupported gate is added in Task 2.7 — insert its hook here.)**
    5. `outcomes, err := spawn.SpawnWindows(adapter, external, deps.ExePath, deps.Getenv)`; on `err` (executable resolution) ⇒ return it (exit 1). (When `external` is empty this is a no-op — the N=1 path.)
    6. If any `outcome` is not `OK()` ⇒ log the batch summary as a partial, **skip self-attach**, and `return fmt.Errorf("spawn: failed to open window for %q", failedSession)` (plain error → exit 1, one-line stderr message; the opaque `Result.Detail` goes to the log, never the user message).
    7. All external succeeded (or none) ⇒ log the batch summary, then `return deps.Connector.Connect(trigger)` — outside tmux this exec-replaces the process (self-execs away), inside tmux it `switch-client`s. This is the single self-attach, gated on all-N−1-success.
  - Logging (bind `var spawnLogger = log.For("spawn")` once in `cmd/spawn.go`; the `spawn` component was introduced in Phase 1): one **INFO** cycle-summary `spawn: opened <opened>/<total>` with attrs `resolution`, `terminal` (`id.Name`), `bundle_id` (`id.BundleID`), `opened`, `total`; one **DEBUG** per external window with `session` + opaque `detail` (from `Result.Detail`). **Count semantics:** `total` = N (all sessions incl. the trigger's self-attach target); `opened` = acked/successful external spawns + the trigger's self-attach when it occurs (full success ⇒ `opened N/N`; failure path skips the trigger and does not count it). Emit only the closed attr keys `resolution`, `terminal`, `bundle_id`, `session`, `opened`, `total`, `detail` — **not** `ack` or `batch` (Phase 3).
- Extend `cmd/spawn_test.go` (unit lane, `package cmd`, no `t.Parallel()`): inject `spawnDeps` with a fake `TerminalDetector` (Ghostty identity), a `Resolve` returning `(*spawntest.FakeAdapter, spawn.ResolutionNative)`, a fake `SessionConnector` recording the self-attach target, a fixed `ExePath`, and a `Getenv` returning a known `PATH`; set `bootstrapDeps` so `PersistentPreRunE` short-circuits (per Phase 1). Drive via `rootCmd` Execute.

**Acceptance Criteria**:
- [ ] `portal spawn s1 s2 s3` (fake adapter, all success) records exactly two `FakeAdapter.OpenWindow` calls, for `s1` then `s2` (arg order), and the fake connector's `Connect` is called once with `s3`.
- [ ] Each recorded `OpenWindow` argv is the composed env-self-sufficient attach command for that session (`/usr/bin/env -u TMUX -u TMUX_PANE PATH=<fixed> <exe> attach <session>`).
- [ ] `portal spawn s1` (N=1) records zero `OpenWindow` calls and self-attaches `s1` via the connector — regardless of the detected terminal (supported or unsupported).
- [ ] With the fake adapter scripted so the second window returns `SpawnFailed`, the connector's `Connect` is **never** called and the command returns a plain (non-`UsageError`) error whose message names the failed session (main maps to exit 1, printed on stderr).
- [ ] The inside-tmux vs outside-tmux self-attach uses `SwitchConnector` vs `AttachConnector` respectively (verified: production `buildSessionConnector` branches on `tmux.InsideTmux()`; the pipeline routes self-attach through it / the injected `Connector`).
- [ ] One INFO `spawn: opened N/N` summary is emitted with `resolution`/`terminal`/`bundle_id`/`opened`/`total`; no `ack` or `batch` attr appears in Phase 2.
- [ ] No unit test dials a real tmux server or runs real `osascript` (deps fully injected; cmd `TestMain` `TMUX` poison would fail a missed injection loudly).

**Tests**:
- `"it spawns N-1 windows in arg order and self-attaches the Nth"`
- `"it composes the env-self-sufficient attach command for each spawned window"`
- `"it self-attaches directly with zero spawns for N=1 regardless of terminal"`
- `"it skips self-attach and exits 1 when any window fails to open"`
- `"it routes self-attach through the inside/outside-tmux connector"`
- `"it emits a spawn: opened N/N summary without ack or batch attrs"`

**Edge Cases**:
- N=1 → zero spawns, direct self-attach regardless of terminal.
- Inside-tmux self-attach via `SwitchConnector`; outside via `AttachConnector`.
- Sessions opened in list/arg order, sequentially (one `OpenWindow` completes before the next).
- Any adapter `OpenWindow` non-success → skip self-attach + exit 1 (detailed leave-what-opened deferred to Phase 3).
- Full success → the trigger self-execs away (never returns on the outside-tmux exec path).

**Context**:
> Spec *Spawn Architecture → The N vs N−1 split / Order is load-bearing*: "the picker **always self-attaches to exactly one** of the N; only the **N−1 others** are externally spawned. Each spawned window runs the **existing `portal attach <session>`** command. … 1. Detect the host terminal. 2. Spawn the N−1 windows (one adapter call per window — for failure isolation) … 3. **Only after all N−1 confirm**, exec self into the Nth session." Step 3 is a point of no return (exec replaces the picker), so the N−1 spawns complete first.
> Spec *`portal spawn` CLI behaviour*: "`portal spawn <sessions…>` mirrors the picker's commit exactly — same net-N invariant: it reuses its **calling terminal window** as one of the N (self-attach-last via `AttachConnector`/`SwitchConnector`, in or out of tmux) and spawns the **N−1** others."
> Spec *Reporting & exit codes*: **Success** → the process self-execs away (no success exit code). **Partial spawn failure** → **exit `1`** with the one-line message on **stderr**; nothing self-execs.
> Spec *Trigger-Context Matrix → Open order*: "Open in **list order** (top-to-bottom as shown), not pick order. … **Which marked session the trigger window becomes: unspecified (implementation-convenience).**" *N=0/N=1 boundary*: "**N=1** … the picker self-attaches to that one session … a plain single attach … No special-casing."
> Spec *Observability → Count semantics*: `total` = N (incl. the trigger's self-attach target); `opened` = each acked/successful spawn plus the trigger's self-attach when it occurs; on the failure path the trigger self-attach is skipped and not counted.
> Scope boundaries for Phase 2: self-attach is gated on the **adapter-returned success only** — the explicit token-ack (`@portal-spawn-<batch>-<token>` marker + `--spawn-ack`) is Phase 3, so no `batch`/`ack` here. **Pre-flight** `has-session` validation and the **detailed leave-what-opened** (continue-and-retry-the-missing-set, unmark-opened) are Phase 3 — Phase 2 stops on first failure and exits 1. The **N≥2 unsupported/NULL atomic no-op** gate is Task 2.7. The connectors (`AttachConnector`/`SwitchConnector`/`buildSessionConnector`) already exist in `cmd/open.go`; the pipeline reuses them. CLAUDE.md — cmd tests inject every tmux-touching `*Deps` seam; `TestMain` poisons `TMUX` package-wide.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Spawn Architecture → Model: one service, two callers / `portal spawn` CLI behaviour / The N vs N−1 split / Order is load-bearing*; *Trigger-Context Matrix & Open Order → N=0/N=1 boundary / Open order*; *Observability & State Footprint → Count semantics*; *Testing Strategy & DI Seams → Primary seam*.

---

## restore-host-terminal-windows-2-7 | approved

### Task 2.7: N≥2 unsupported/NULL atomic no-op — exit 1, nothing spawned

**Problem**: On an unsupported or NULL (remote/mosh, or a recognised-but-undriven terminal like Apple Terminal) host terminal, spawning the N−1 external windows is impossible — they need an adapter that isn't available. The pipeline must therefore refuse an N≥2 batch **atomically** (before touching any adapter): nothing opens, no self-attach, exit 1 with a one-line message naming the detected identity. The asymmetry is intentional — an N=1 batch still self-attaches (single attach needs no adapter).

**Solution**: Add the pre-adapter gate to the `cmd/spawn.go` pipeline: after detect + resolve, if the resolution is `unsupported` and there is at least one external window to open (N≥2), return a plain error (exit 1) with a one-line stderr message naming the detected terminal — before any `SpawnWindows`/`OpenWindow` call — and never self-attach. N=1 (empty external set) bypasses the gate and self-attaches as before.

**Outcome**: `portal spawn s1 s2` on an unsupported/NULL terminal opens nothing, calls no adapter method, does not self-attach, prints one line to stderr naming the detected identity, and exits 1. `portal spawn s1` on the same terminal still self-attaches `s1`. Unit-tested via a fake unsupported `Resolve` + fake connector asserting zero `OpenWindow` calls, zero `Connect` calls for N≥2, and one `Connect` for N=1.

**Do**:
- In `cmd/spawn.go` `runSpawn`, insert the gate at step 4 (immediately after `adapter, resolution := deps.Resolve(id)`, before the `SpawnWindows` call — the atomic point):
  - `if len(external) >= 1 && resolution == spawn.ResolutionUnsupported {` → return the unsupported no-op:
    - Compose the one-line message from the detected identity: if `id.IsNull()` ⇒ `"spawn: no host-local terminal — nothing opened"`; else ⇒ `fmt.Sprintf("spawn: unsupported terminal — %s · %s — nothing opened", id.Name, id.BundleID)` (mirrors the design banner copy `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal`; the exact CLI wording isn't pinned by the spec beyond naming the detected identity + conveying nothing opened).
    - Emit one `spawn` INFO/WARN outcome line with `resolution=unsupported`, `terminal`=`id.Name`, `bundle_id`=`id.BundleID` (no per-window records — nothing was attempted).
    - `return errors.New(<message>)` — a plain, non-`UsageError`, non-silent error so `main.classify` prints it to **stderr** and exits **1**. Do **not** call the connector (no self-attach) and do **not** call `SpawnWindows` (no adapter touched — the check precedes any adapter call, making the no-op atomic).
  - The gate condition `len(external) >= 1` is exactly N≥2 (for N=1 `external` is empty, so the gate is skipped and the pipeline self-attaches — single attach needs no adapter).
- Extend `cmd/spawn_test.go` (unit lane): inject a `Resolve` returning `(nil, spawn.ResolutionUnsupported)` and a `Detector` returning both a NULL identity and a recognised-but-undriven identity (e.g. `NewIdentity("com.apple.Terminal","Apple Terminal")`) across cases; use a `FakeAdapter` and assert `len(FakeAdapter.Calls) == 0`; use a fake connector and assert `Connect` was not called for N≥2 and was called once for N=1.

**Acceptance Criteria**:
- [ ] `portal spawn s1 s2` with `resolution == unsupported` records **zero** `FakeAdapter.OpenWindow` calls (the check precedes any adapter call — atomic).
- [ ] The same invocation calls the connector's `Connect` **zero** times (no self-attach) and returns a plain error → exit 1, with a one-line stderr message naming the detected terminal (friendly name + bundle id for a resolved-but-undriven identity, or the "no host-local terminal" line for NULL).
- [ ] `portal spawn s1` (N=1) on the same unsupported/NULL terminal opens nothing but **does** self-attach `s1` via the connector (single attach needs no adapter).
- [ ] A recognised-but-undriven identity (`com.apple.Terminal`) and a NULL identity both trigger the N≥2 atomic no-op; the message names the identity for the former and the honest no-host-local line for the latter.
- [ ] The returned error is not a `*cmd.UsageError` (exit 1, not 2) and is not silenced (it prints to stderr).

**Tests**:
- `"it refuses an N>=2 batch on an unsupported terminal atomically with no adapter call"`
- `"it does not self-attach on an N>=2 unsupported batch and exits 1"`
- `"it names the detected terminal (friendly name + bundle id) in the one-line message"`
- `"it prints the honest no-host-local-terminal line for a NULL identity N>=2 batch"`
- `"it still self-attaches for N=1 on an unsupported terminal"`

**Edge Cases**:
- N≥2 unsupported → nothing spawns, exit 1, one-line message on stderr, no self-attach.
- The check happens before any adapter call (atomic — no partial windows).
- N=1 on unsupported still self-attaches (single-attach needs no adapter).
- NULL identity (remote/mosh / no host-local terminal) and recognised-but-undriven identity both fold to the same no-op path.

**Context**:
> Spec *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter)*: "**`Enter` with N=1** proceeds regardless of detection: it is a plain self-attach … opens no host window, needs no adapter. **`Enter` with N≥2** on an unsupported/NULL terminal is an **atomic no-op** — nothing opens (the N−1 external windows need an adapter that isn't available) … Same 'honest no-op' as remote/mosh. (The N=1-works vs N≥2-blocked asymmetry is intentional: only external-window spawning needs the adapter.)"
> Spec *Reporting & exit codes*: "**unsupported/NULL terminal with N≥2** → **exit `1`** with the same one-line message the picker would show, on **stderr**; nothing self-execs."
> Spec *User-facing display: both / Design References*: the banner names the detected identity as friendly name **and** bundle id (design copy `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal`); the CLI one-line message mirrors it. NULL (remote/mosh / no local client) is the honest "no host-local terminal" no-op (*Detection model → Host-local principle*).
> Scope: the picker-side banner precedence, in-mode re-assertion, and the TUI notice band are Phase 5; Phase 2 implements only the CLI's atomic gate + exit-1 message. The permission-required burst-stop (a distinct, guidance-carrying path) is Phase 3 and is separate from this unsupported no-op.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter) / User-facing display: both / Host-local principle*; *Spawn Architecture → Reporting & exit codes*; *Design References*.
