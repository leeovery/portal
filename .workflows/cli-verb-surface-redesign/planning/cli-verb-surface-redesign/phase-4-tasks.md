---
phase: 4
phase_name: "doctor & uninstall — maintenance surface reshuffle"
total: 7
---

## cli-verb-surface-redesign-4-1 | approved

### Task 4.1: `doctor` command + read-only diagnosis framework (state-package checks)

**Problem**: Portal has no single read-only health verb. `state status` renders a fixed six-line report and `clean` bundles unrelated repairs; neither is the scriptable "is Portal healthy?" gate the redesign calls for. The redesign replaces both with `portal doctor`, and this task lays the foundation: the command, the check-result framework, the exit-code contract, and the checks that read only the state directory (no tmux).

**Solution**: Add a new bootstrap-exempt `portal doctor` command (`cmd/doctor.go`) that runs an ordered catalog of health checks, renders one line per check to stdout, and drives a scriptable exit code. This task builds the framework — a `checkResult`/`checkStatus` model, a render pass, the silent-exit sentinel wiring — plus the three state-directory checks that need no tmux server: **daemon alive**, **state dir sane**, and **sessions.json valid**. The tmux-runtime checks (task 4.2), stale-entry checks (4.3), host-terminal line (4.4), and `--fix` (4.5) layer onto this framework.

**Outcome**: `portal doctor` runs against raw state (never bootstrapping), prints a report that always carries at least the three state-dir checks, exits `0` when every check passes and `1` when any check reports a problem — a fresh install / down server reports honestly and exits non-zero without crashing.

**Do**:
- Create `cmd/doctor.go` with `doctorCmd` (`Use: "doctor"`, `Args: cobra.NoArgs`, `SilenceErrors: true`, `SilenceUsage: true`), registered on `rootCmd` in `init()`.
- Define the check model in the same file: `type checkStatus int` with values `checkPass`, `checkFail`, `checkInfo` (informational — never drives exit), `checkNotEvaluable` (neutral — never drives exit); and `type checkResult struct { name string; status checkStatus; detail string }`.
- Define `runDoctorDiagnosis(deps *DoctorDeps) ([]checkResult, error)` that resolves the state dir via `state.Dir()` (**read-only — never `EnsureDir`**, so a missing dir is observed, not created) and appends the three state-dir checks below, returning the ordered slice. Introduce the package-level injection point `var doctorDeps *DoctorDeps` and `type DoctorDeps struct { StateDir string; Now func() time.Time }` (nil in production → real values; `StateDir` overridable for hermetic tests), following the cmd-package `*Deps` idiom (mirrors `StateCleanupDeps` / `commitNowDeps`).
- **Daemon-alive check**: use `state.DaemonAlive(dir)` (or `state.CollectStatus(dir, now).DaemonRunning`). Alive → `checkPass` with detail `running (pid N, version V)`; dead/missing/unparseable pid → `checkFail` `not running`. (The distinct down-server message is layered in task 4.2; here a dead pid simply fails.)
- **State-dir-sane check**: `state.Dir()` error → `checkFail` `cannot resolve state dir`; `os.Stat(dir)` ErrNotExist → `checkPass` `not created yet` (fresh install is fine — bootstrap creates it); exists and `IsDir()` → `checkPass`; exists but not a directory → `checkFail`; stat permission/other error → `checkFail` `unreadable`.
- **sessions.json-valid check**: call `state.ReadIndex(dir)` directly (NOT the lossy `HasLastSave` boolean) and branch on its three-shape contract — `(_, true, nil)` absent → `checkPass` `no sessions saved yet`; `(_, true, err)` (wraps `state.ErrCorruptIndex`) → `checkFail` `sessions.json corrupt`; `(idx, false, nil)` → `checkPass` `N sessions, M panes`.
- Add `renderDoctorReport(w io.Writer, results []checkResult)` writing a header (`Portal doctor:`) then one line per result with a clear pass/fail/not-evaluable indicator and its detail. The informational host-terminal line (task 4.4) renders without a pass/fail marker.
- Add the exit-code wiring: `doctorUnhealthy(results)` returns true iff any result is `checkFail` (`checkInfo` and `checkNotEvaluable` never count). Introduce `var ErrDoctorUnhealthy = errors.New("doctor unhealthy")` and return it from `RunE` after rendering when unhealthy (report already on stdout; the sentinel drives a non-zero exit with no stderr). Extend `IsSilentExitError` (`cmd/state_commit_now.go`) to also recognise `ErrDoctorUnhealthy` (leave the existing `ErrStatusUnhealthy` in place — task 4.7 removes it when `state status` is deleted).
- Add `"doctor": true` to `skipTmuxCheck` (`cmd/root.go`) so `PersistentPreRunE` returns early and doctor observes raw state (no EnsureServer/RegisterHooks/EnsureSaver/Restore healing its own subject). Update the `skipTmuxCheck` doc comment to name `doctor` (renamed successor to `state status`).

**Acceptance Criteria**:
- [ ] `portal doctor` is a registered public command, `Args: cobra.NoArgs`, that renders a report and exits `0` iff every check passes, `1` if any check `checkFail`s.
- [ ] `doctor` is in `skipTmuxCheck` — `PersistentPreRunE` runs no bootstrap for it; it starts no server, registers no hooks, respawns no daemon.
- [ ] The report always carries at least the three state-dir checks (daemon alive, state dir sane, sessions.json valid), even on a fresh install with no state dir.
- [ ] A dead/missing `daemon.pid` fails the daemon-alive check → non-zero exit.
- [ ] A missing state dir does NOT crash and does NOT fail state-dir-sane (`not created yet` → pass); a corrupt `sessions.json` (`ReadIndex` err wrapping `ErrCorruptIndex`) fails sessions.json-valid; an absent `sessions.json` passes as `no sessions saved yet`.
- [ ] The command is strictly read-only — it uses `state.Dir()` not `state.EnsureDir()`, issues no tmux mutation, writes no files.
- [ ] Unhealthy exit routes through `ErrDoctorUnhealthy` recognised by `IsSilentExitError` — non-zero exit, report on stdout, nothing on stderr.

**Tests**:
- `"it renders a report and exits zero when all state-dir checks pass"` — hermetic: `doctorDeps.StateDir` = temp dir seeded with a live `daemon.pid` (pid = `os.Getpid()`) and a valid `sessions.json`.
- `"it fails and exits non-zero when daemon.pid is dead"` — seed `daemon.pid` with an unused pid; assert daemon-alive fails and the run returns `ErrDoctorUnhealthy`.
- `"it reports a fresh install honestly without crashing"` — `StateDir` = a non-existent path; assert state-dir-sane passes `not created yet`, sessions.json passes `no sessions saved yet`, daemon-alive fails, exit non-zero, no panic.
- `"it fails sessions.json-valid on corrupt content but passes on an absent file"` — two subcases: write malformed JSON (fail); no file (pass `no sessions saved yet`).
- `"it is registered in skipTmuxCheck"` — assert `skipTmuxCheck["doctor"]`.
- `"IsSilentExitError recognises ErrDoctorUnhealthy"` — unit assertion so main.go suppresses stderr on the unhealthy exit.
- Integration-tagged (`//go:build integration`, `portaltest.IsolateStateForTest`): `"it exits non-zero against a fresh isolated state dir with no running daemon"` — build via `portalbintest`, run the real `portal doctor`, assert exit 1 and a report on stdout.

**Edge Cases**:
- Fresh install / no state dir yet → reported honestly (state-dir-sane `not created yet`, sessions.json `no sessions saved yet`), no crash; overall still non-zero because daemon-alive fails.
- Missing or corrupt `sessions.json` distinguished via `ReadIndex`'s `(skip, err)` shapes — absent is a pass, corrupt is a fail; do NOT collapse both to a single `HasLastSave=false` failure.
- Dead `daemon.pid` (stale after a crash/self-eject) → daemon-alive fails → non-zero.
- Strictly read-only: `state.Dir()`, no tmux, no writes.
- Bootstrap-exempt so it observes raw state and heals nothing.
- Report always carries ≥1 check (the state-dir checks never depend on tmux).

**Context**:
> Spec § `doctor`: read-only health report; check catalog fixed but "planning implements the concrete probe per check". Exit-code contract: "exits 0 iff every check passes; non-zero (1) if any check reports a problem". Spec § Bootstrap Exemption: "`doctor` must be exempt — otherwise bootstrap re-registers hooks and respawns the daemon one step *before* `doctor` reads health, so a read-only check would heal its own subject and always report green".
>
> Ambiguity flagged: the catalog lists "sessions.json valid" and the task table notes both missing and corrupt yield `HasLastSave=false`. `StatusReport.HasLastSave` conflates absent and corrupt, but the exit-code example attributes a down-server's failures to "daemon / saver / hooks checks" — not sessions.json — and a healthy just-booted server can momentarily lack `sessions.json`. Planning resolves this by using `state.ReadIndex`'s richer `(skip, err)` discrimination: absent → pass (`no sessions saved yet`), corrupt → fail. This honours "fresh install reported honestly" without a false failure while still failing on genuine corruption.
>
> `state.CollectStatus` / `state.StatusReport` (`internal/state/status.go`) are REUSED — they survive `state status`'s deletion (task 4.7). `ErrDoctorUnhealthy` mirrors the retired `ErrStatusUnhealthy` silent-exit pattern (`cmd/state_status.go`).

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — Diagnostics & Repair; Exit-code contract; Bootstrap Exemption.

## cli-verb-surface-redesign-4-2 | approved

### Task 4.2: Runtime tmux checks (saver up, hooks no duplicates) + distinct down-server report

**Problem**: The doctor catalog includes three checks that require a live tmux server — daemon alive (in the runtime sense), `_portal-saver` up, and global hooks registered without duplicates. When the server is down, all three must fail with a single distinct "runtime not running" message rather than looking like corruption; and the hooks check must count Portal entries per managed event using the per-event read (the no-arg `show-hooks -g` is blind to `pane-*`/geometry `window-*` events on tmux 3.6b).

**Solution**: Extend `cmd/doctor.go` with a `ServerRunning()` front gate and two new runtime checks. When the server is down, the daemon / saver / hooks checks all report the distinct message `Portal runtime not running — run \`portal open\` to start`. When it is up, run the real probes: `_portal-saver` presence via `tmux.SaverPanePIDOrAbsent`, and a per-event Portal-hook count via a new read-only `internal/tmux` helper that reuses `managedEvents` and the per-event fingerprints. Add the tmux client + probe seams to `DoctorDeps`.

**Outcome**: On a live, healthy server the runtime checks pass; on a down server they all report the distinct runtime-not-running message (non-zero, but clearly "not running" not "corrupt"); ≥2 Portal entries on any managed event fails the hooks check as a duplicate; a `_portal-bootstrap`-only server (saver gone) still fails the saver check; a transient tmux read is reported honestly, not as a crash.

**Do**:
- Add a read-only helper to `internal/tmux`, e.g. `func PortalHookCountsByEvent(c *Client) (map[string]int, error)` (NEW), that iterates `managedEvents`, reads each event via `c.ShowGlobalHooksForEvent(event)` (the per-event seam — NOT the tmux-3.6b-blind no-arg `show-hooks -g`), parses with `parseEventEntries`, and counts entries whose command matches that event's `fingerprints` via `containsAny` — exactly the classification `convergeEvent` uses. Return the per-event count map; a read failure on any event returns the wrapped error (transient path).
- Extend `DoctorDeps` with tmux probe seams (function-typed, cmd-package idiom): `ServerRunning func() bool`, `SaverPresent func() (present bool, err error)` (production wraps `tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)`, discarding the pid), and `HookCounts func() (map[string]int, error)` (production wraps `tmux.PortalHookCountsByEvent(client)`). Build the production tmux client once via `tmux.DefaultClient()` (doctor is bootstrap-exempt so no client is injected into `cmd.Context()`).
- Add a **ServerRunning gate** in `runDoctorDiagnosis`: read `serverUp := deps.ServerRunning()` once. When `!serverUp`, emit the daemon / saver / hooks checks all as `checkFail` carrying the exact detail `Portal runtime not running — run \`portal open\` to start`. When up, run the real probes (the daemon-alive check from task 4.1 stays state-based; saver and hooks run their tmux probes).
- **Saver-up check** (server up): `deps.SaverPresent()` → present → `checkPass` `_portal-saver up`; absent (`present=false, err=nil`) → `checkFail` `_portal-saver not running`; transient `err != nil` → `checkNotEvaluable` `could not read saver (transient tmux error)` (honest, never a crash).
- **Hooks-no-duplicates check** (server up): `deps.HookCounts()`. On `err != nil` → `checkNotEvaluable` `could not read hooks (transient tmux error)`. Else: any event with count `>= 2` → `checkFail` naming the event(s) and count (`duplicate hook entries on <event> (N)`); any event with count `0` → `checkFail` `hooks not registered on <event>`; all counts `== 1` → `checkPass` `hooks registered (one per event)`.

**Acceptance Criteria**:
- [ ] With the server down, the daemon, saver, and hooks checks all report `Portal runtime not running — run \`portal open\` to start` (byte-exact) and drive a non-zero exit, distinct from any corruption message.
- [ ] With the server up and exactly one Portal entry per managed event, the hooks check passes; ≥2 entries on ANY managed event fails as a duplicate; a `0`-count event fails as not-registered.
- [ ] The hooks probe reads per-event via `ShowGlobalHooksForEvent` over `managedEvents` — it never relies on the no-arg `show-hooks -g` (so `pane-focus-out` / `window-layout-changed` duplicates are detected).
- [ ] `_portal-saver` absent (even when `_portal-bootstrap` is present) fails the saver check; `_portal-saver` present passes it.
- [ ] A transient tmux read (saver or hooks) is reported as `checkNotEvaluable`, never a crash, and does not spuriously fail the exit code.
- [ ] All probes are read-only (no `set-hook`, no `kill-session`, no writes).

**Tests**:
- `"it reports runtime-not-running for daemon, saver, and hooks when the server is down"` — hermetic: `ServerRunning` seam returns false; assert all three checks carry the exact distinct message and the run is unhealthy.
- `"it passes the hooks check with exactly one entry per managed event"` — `HookCounts` seam returns 1 for every `managedEvents` name.
- `"it fails the hooks check on a duplicated event"` — seam returns 2 for `pane-focus-out`; assert duplicate fail naming the event.
- `"it fails the hooks check when an event has zero Portal entries"`.
- `"it fails the saver check when _portal-saver is absent"` and `"...passes when present"`.
- `"it reports not-evaluable on a transient saver/hooks read error"` — seam returns a non-nil error; assert `checkNotEvaluable` and that it alone does not make the exit non-zero.
- `internal/tmux` unit: `"PortalHookCountsByEvent counts only Portal-fingerprint entries per managed event"` — drive a fake `Commander` returning `show-hooks -g <event>` output with a Portal entry plus a foreign entry; assert per-event count = 1 (foreign ignored) and a stacked event = 2. A per-event tmux read failure propagates as an error.
- Integration-tagged (`//go:build integration`, `IsolateStateForTest`, `tmuxtest` socket, `portalbintest`): `"it passes runtime checks against a real bootstrapped server"` and `"it reports runtime-not-running when the test server is stopped"`.

**Edge Cases**:
- Server down → saver/hooks/daemon all report the distinct not-running message, never "corrupt".
- ≥2 Portal-fingerprint entries on any managed event → duplicate fail, via per-event `ShowGlobalHooksForEvent` + `managedEvents` (never the tmux-3.6b-blind no-arg read).
- `_portal-bootstrap` present but `_portal-saver` absent → saver check still fails (exact-match `HasSession`/`SaverPanePIDOrAbsent` never resolves the wrong session).
- Transient tmux read → `checkNotEvaluable`, honest, not a crash, does not drive the exit code.

**Context**:
> Spec § `doctor` catalog: "daemon alive; global tmux hooks registered without duplicates (exactly one Portal entry per managed event); `_portal-saver` session up". Spec § Exit-code contract: "A down server counts as unhealthy → non-zero … reported honestly and distinctly — 'Portal runtime not running — run `portal open` to start' vs. actual corruption".
>
> `managedEvents` is the single source of truth for the Portal-managed event set (`internal/tmux/hooks_register.go`); `convergeEvent` already classifies a Portal-authored entry by `containsAny(entry.Command, fingerprints)`. The new `PortalHookCountsByEvent` REUSES this machinery read-only so the fingerprint knowledge never leaks into `cmd`. `SaverPanePIDOrAbsent` (`internal/tmux/saver_pane_pid.go`) collapses `ErrNoSuchSession`/`ErrEmptyPaneList` to `present=false` and surfaces other errors — exactly the tri-state the saver check needs. Doctor builds its own `tmux.DefaultClient()` because it is bootstrap-exempt (no client in `cmd.Context()`).

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — Diagnostics & Repair; Exit-code contract.

## cli-verb-surface-redesign-4-3 | approved

### Task 4.3: Read-only stale-entry checks — dead-pane hooks + gone-dir projects

**Problem**: The catalog's "no stale entries" check covers two independent kinds of cruft — dead-pane hook entries (a `hooks.json` key with no live pane) and gone-dir projects (a `projects.json` path that no longer exists on disk). Detecting dead-pane hooks requires enumerating live panes on a running server; with the server down (or a degenerate zero-panes state) that enumeration is empty and every hook would falsely look orphaned — the exact mass-deletion hazard `runHookStaleCleanup` guards against. The doctor version must be strictly read-only and must never report "all stale" in that state.

**Solution**: Add two read-only checks to `cmd/doctor.go` — **stale hooks** and **stale projects** — rendered under the catalog's "no stale entries" item. The stale-hook check mirrors `runHookStaleCleanup`'s mass-deletion hazard guard exactly but computes the stale set without deleting; when live-pane enumeration is empty-or-errored while hooks are present, it reports `checkNotEvaluable` (never "all stale"). The stale-project check stats each project path (`os.Stat`) and counts only `ErrNotExist` as stale, retaining permission-denied paths.

**Outcome**: On a healthy server with live panes, genuine dead-pane hooks and genuine gone-dir projects are reported as failures (non-zero); on a down server the dead-pane-hook check reports not-evaluable (neutral, never a false failure) while the filesystem-only gone-dir-project check still evaluates; nothing is pruned.

**Do**:
- Extend `DoctorDeps` with the read seams the checks need: `HookLister AllPaneLister` (production = the doctor tmux client, which implements `ListAllPaneHookKeys`), `HookStore *hooks.Store` (production = `loadHookStore()`), and `ProjectStore *project.Store` (production = `loadProjectStore()`).
- **Stale-hooks check** (read-only, mirrors the `runHookStaleCleanup` hazard guard in `cmd/run_hook_stale_cleanup.go`): load persisted hooks via `HookStore.Load()`; enumerate live keys via `HookLister.ListAllPaneHookKeys()`. Guard, in this exact order: (1) `Load` error → `checkNotEvaluable` `could not read hooks.json`; (2) `ListAllPaneHookKeys` error → `checkNotEvaluable` `could not enumerate live panes` (the server-down / transient case — never "all stale"); (3) `len(live) == 0 && len(persisted) > 0` → `checkNotEvaluable` `zero live panes with hooks present (not evaluable)` (mirrors the hazard guard's deferral — this is the "zero live panes" not-evaluable case); (4) `len(live) == 0 && len(persisted) == 0` → `checkPass` `no hooks`; (5) otherwise compute `stale = persisted keys ∉ live` — `len(stale) > 0` → `checkFail` `N stale hook entries`, else `checkPass` `no stale hooks`. **Never** call `CleanStale` here (that is `--fix`, task 4.5).
- **Stale-projects check** (read-only, filesystem-only — mirrors `project.Store.CleanStale`'s classification without saving): load via `ProjectStore.Load()`; for each project `os.Stat(p.Path)` — `nil` → live; `errors.Is(err, os.ErrNotExist)` → stale; any other error (permission-denied, etc.) → retained (NOT counted stale). `len(stale) > 0` → `checkFail` `N stale projects`, else `checkPass` `no stale projects`. A `Load` error → `checkNotEvaluable` `could not read projects.json`.
- Render both under the "no stale entries" heading (two rows) so the not-evaluable state is clearly attributed to hooks only while projects still evaluate.

**Acceptance Criteria**:
- [ ] With live panes present, a `hooks.json` key with no matching live key is reported as a stale-hook failure → non-zero; with no stale keys, the check passes.
- [ ] With the server down / `ListAllPaneHookKeys` erroring, OR zero live panes parsed while hooks are present, the stale-hook check reports `checkNotEvaluable` — never "all stale", never a false failure, and it alone does not drive the exit code.
- [ ] A `projects.json` entry whose path is gone (`os.Stat` → `ErrNotExist`) is a stale-project failure → non-zero; a permission-denied path is retained (not counted stale).
- [ ] The stale-project check evaluates even when the server is down (it touches only the filesystem).
- [ ] Neither check mutates `hooks.json` or `projects.json` (strictly read-only — no pruning).
- [ ] The stale-hook guard order and semantics match `runHookStaleCleanup`'s hazard guard exactly (zero-live-with-hooks → deferral/not-evaluable; both-empty → clean).

**Tests**:
- `"it fails the stale-hook check when a persisted key has no live pane"` — hermetic: `HookStore` seeded with keys `a:0.0`, `b:0.0`; `HookLister` returns only `a:0.0`; assert one stale, `checkFail`.
- `"it reports not-evaluable when zero live panes are enumerated but hooks are present"` — `HookLister` returns empty slice, `HookStore` non-empty; assert `checkNotEvaluable`, exit not driven by it.
- `"it reports not-evaluable when live-pane enumeration errors"` — `HookLister` returns an error (down-server surrogate).
- `"it passes the stale-hook check when both live and persisted are empty"`.
- `"it fails the stale-project check for a gone directory but retains a permission-denied path"` — seed one project at a deleted temp path (stale) and one at a path whose parent denies stat (retained).
- `"it evaluates stale projects even with the server down"` — no tmux involved; assert the check runs.
- `"neither stale check mutates its store"` — assert `hooks.json` / `projects.json` bytes unchanged after the run.

**Edge Cases**:
- Server down → dead-pane-hook staleness `checkNotEvaluable` (never "all stale", never a false failure).
- Zero live panes with hooks present is exactly the not-evaluable case (mirrors `runHookStaleCleanup`'s hazard guard deferral).
- Genuine stale hook / project → `checkFail` → non-zero.
- Gone-dir detection is `os.Stat`-based; permission-denied paths are retained, not counted stale (matches `project.Store.CleanStale`).
- Strictly read-only — no pruning here; that is `--fix` (task 4.5).

**Context**:
> Spec § `doctor` catalog: "no stale entries (dead-pane hooks, gone-dir projects)". Spec § Down-server guard: "with the server **down**, that enumeration is empty and *every* hook would falsely look orphaned. So … the 'no stale entries' check reports dead-pane-hook staleness as **not-evaluable** (never 'all stale') … The stale-**project** prune is filesystem-only (directory existence) and may still run."
>
> The load-bearing mechanism is `runHookStaleCleanup`'s hazard guard (`cmd/run_hook_stale_cleanup.go`, steps 4–5): `len(livePanes) == 0` with persisted entries → Warn-and-defer (not-evaluable); both-empty → clean. This doctor check mirrors that guard read-only. `project.Store.CleanStale` (`internal/project/store.go`) is the model for the gone-dir classification (`os.Stat` → `ErrNotExist` stale, other errors retained). `AllPaneLister` / `ListAllPaneHookKeys` derive live keys the same way registration does, so freshly-registered `@portal-id`-keyed entries are not mis-flagged.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — Diagnostics & Repair (catalog); Down-server guard on the stale-hook prune.

## cli-verb-surface-redesign-4-4 | approved

### Task 4.4: Host-terminal informational line (`Detect()` + resolver)

**Problem**: `spawn --detect` (a dry-run printing the detected host terminal's identity) is retired with `spawn` (Phase 5). Its job folds into `doctor`: the report must show the host terminal and whether it is a supported multi-window spawn target — but this is an environmental fact, not a Portal-health defect, so it must never drive the exit code.

**Solution**: Add a host-terminal informational line to `cmd/doctor.go`, computed from the same `Detect()` the picker/burst use and the same config-aware `Resolve` (loaded from `terminals.json`). Classify the identity into supported / unsupported / unsupported-remote and render a single `host terminal: …` line as a `checkInfo` result — outside the pass/fail set.

**Outcome**: `portal doctor` prints e.g. `host terminal: Ghostty (supported)`, `host terminal: Warp (unsupported)`, or `host terminal: unsupported (remote session)`; the line reflects the true detection+resolution and never affects whether doctor exits `0` or `1`.

**Do**:
- Extend `DoctorDeps` with `Detector TerminalDetector` (the `Detect() spawn.Identity` seam already defined in `cmd/spawn.go`; production = `spawn.NewDetector(client)` over the doctor tmux client) and `Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` (production = `buildResolver().Resolve` — the config-aware resolver loaded once from `terminals.json`, identical to the burst's seam).
- Compute the line in `runDoctorDiagnosis`: `id := deps.Detector.Detect()`; `_, resolution := deps.Resolve(id)`. Classify: `id.IsNull()` → `host terminal: unsupported (remote session)` (covers remote/mosh / no-local-client / a transient detect failure, which `Detect` already folds to the NULL identity); non-null and `resolution == spawn.ResolutionUnsupported` → `host terminal: <id.Name> (unsupported)` (recognised terminal, no adapter); otherwise (`resolution != Unsupported`) → `host terminal: <id.Name> (supported)`.
- Emit this as a `checkResult{status: checkInfo}` so it renders without a pass/fail marker and is excluded from `doctorUnhealthy`. Confirm `doctorUnhealthy` counts only `checkFail` (task 4.1) so this line can never push the exit non-zero.

**Acceptance Criteria**:
- [ ] The report includes a single `host terminal:` line reflecting `Detect()` + `Resolve`.
- [ ] A NULL identity (remote/mosh, no local client, or a transient detect failure folded to NULL) renders `unsupported (remote session)`.
- [ ] A recognised-but-undriven terminal (non-null identity, `Resolution == Unsupported`) renders `<Name> (unsupported)`.
- [ ] A supported terminal (`Resolution != Unsupported`) renders `<Name> (supported)`.
- [ ] The line is `checkInfo` — it never makes `doctor` (or `doctor --fix`) non-zero, regardless of its content.
- [ ] Detection reuses the same `Detect()` / `Resolve` seams the picker/burst use (no bespoke detection path).

**Tests**:
- `"it prints supported for a driven terminal"` — `Detector` seam returns a Ghostty identity, `Resolve` returns `ResolutionNative`; assert `host terminal: Ghostty (supported)`.
- `"it prints unsupported (remote session) for a NULL identity"` — `Detector` returns `spawn.Identity{}`.
- `"it prints <Name> (unsupported) for a recognised-but-undriven terminal"` — non-null identity, `Resolve` returns `ResolutionUnsupported`.
- `"the host-terminal line never drives the exit code"` — a doctor run where every real check passes but the terminal is unsupported still exits `0`; a run where a real check fails exits `1` regardless of a supported terminal.
- Integration-tagged: `"it prints a host-terminal line against the real detector"` — smoke test that the line is present and well-formed.

**Edge Cases**:
- NULL / remote / mosh identity → `unsupported (remote session)`.
- Recognised-but-undriven terminal → `<Name> (unsupported)`.
- Transient detect failure folds (inside `Detect`) to the NULL identity → renders as `unsupported (remote session)`; never a crash.
- `supported` iff resolver `Resolution != Unsupported`.
- The line is outside the pass/fail set and never drives the exit code.

**Context**:
> Spec § Host-terminal detection folded in: "`spawn --detect` … is retired with `spawn`. Its job folds into `doctor`: the picker keeps calling `Detect()` in-process; `doctor` calls the same function and prints a line such as `host terminal: Ghostty (supported)` / `unsupported (remote session)`." Spec § Exit-code contract: "The **host-terminal check is informational only** — it is *outside* the pass/fail set … reported honestly but never makes `doctor` (or `doctor --fix`) non-zero."
>
> `spawn.Detector.Detect()` (`internal/spawn/detect.go`) already folds a transient failure to the NULL identity internally, so doctor gets a clean tri-state. `Resolver.Resolve` returns `ResolutionUnsupported` for a NULL identity, a known-but-undriven terminal, or an unknown one (`internal/spawn/resolver.go`). `TerminalDetector` and `buildResolver().Resolve` are the exact seams `cmd/spawn.go`'s `--detect` and burst use — REUSED, not reimplemented.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — Host-terminal detection folded in; Exit-code contract.

## cli-verb-surface-redesign-4-5 | approved

### Task 4.5: `doctor --fix` — repairs + unconditional log-sweep side-action + re-diagnose

**Problem**: `clean` is being deleted, but its one genuinely-useful action (prune stale projects) plus the daemon's already-automated stale-hook prune need a manual trigger. `doctor` is the natural home for the paired verb: diagnose, then optionally repair the diagnosis. The repairs must be low-stakes and reversible-by-reconstruction — critically, hook pruning must NOT run on a down server (it would wipe user-authored on-resume commands that Portal cannot reconstruct).

**Solution**: Add a `--fix` flag to `cmd/doctor.go`. When set, run the diagnosis, then apply the reversible repairs — prune stale hooks (via `runHookStaleCleanup`, whose hazard guard already refuses to prune when live-pane enumeration is empty/errored), prune stale projects (via `project.Store.CleanStale`, filesystem-only), and sweep logs (via `log.SweepLogsForClean`, an unconditional maintenance side-action outside the diagnose→repair loop) — then re-run the full diagnosis and set the exit code from the post-repair state.

**Outcome**: `portal doctor --fix` prunes stale hooks (only when live panes are enumerable) and stale projects, sweeps rotated logs unconditionally, re-diagnoses, and exits `0` iff everything is healthy post-repair; on a down server it prunes NO hooks (protecting user-authored commands) but still prunes stale projects and still sweeps logs; the log-sweep never affects the exit code.

**Do**:
- Add the `--fix` bool flag to `doctorCmd` (`doctorCmd.Flags().Bool("fix", false, "apply low-stakes reversible repairs, then re-diagnose")`).
- In `RunE`: run `runDoctorDiagnosis` and render as today. If `--fix` is set, call a new `runDoctorFix(deps)` after the initial render, then re-run `runDoctorDiagnosis`, render the post-repair report, and drive the exit from the post-repair results.
- `runDoctorFix` performs, in order:
  1. **Prune stale hooks** — `runHookStaleCleanup(deps.HookLister, deps.HookStore, bootstrapLogger, onRemoved)` where `onRemoved` prints `Pruned stale hook: <key>`. This reuses the hazard guard verbatim: a `ListAllPaneHookKeys` error or zero-live-panes-with-hooks defers (no prune), so on a down server NO hook is pruned. Do not add a separate down-server branch — the guard IS the protection.
  2. **Prune stale projects** — `removed, err := deps.ProjectStore.CleanStale()`; print `Pruned stale project: <name> (<path>)` per removed. Filesystem-only, so it runs regardless of server state.
  3. **Sweep logs** — resolve the state dir via `state.Dir()` and call `log.SweepLogsForClean(stateDir)`; best-effort, errors logged under `bootstrapLogger` and swallowed. This is OUTSIDE the diagnose→repair loop and its outcome NEVER touches the exit code.
- Keep `runDoctorFix`'s repairs off the exit code directly — only the post-repair re-diagnosis (`doctorUnhealthy`) drives the exit. The log-sweep participates in neither the catalog nor the exit code.
- Wire `deps.HookLister` for `--fix` from the doctor's own tmux client (the one built in task 4.2) so it does not depend on the deleted `buildCleanPaneLister` (task 4.7).
- Ensure `doctor --fix` stays bootstrap-exempt (the `skipTmuxCheck["doctor"]` entry from task 4.1 covers it — the flag does not change the command name).

**Acceptance Criteria**:
- [ ] `doctor --fix` prunes stale hooks via `runHookStaleCleanup`, stale projects via `project.Store.CleanStale`, and sweeps logs via `log.SweepLogsForClean`, then re-runs the diagnosis.
- [ ] With the server down, `--fix` performs NO hook pruning (the `runHookStaleCleanup` hazard guard defers) — user-authored on-resume commands survive — while the filesystem-only stale-project prune still runs.
- [ ] The log-sweep is outside the diagnose→repair loop and never affects the exit code (a stale-log state can never make doctor non-zero).
- [ ] After repairs, `doctor --fix` exits `0` iff everything is healthy post-repair, non-zero if anything remains unhealthy or unfixable (e.g. a still-down server).
- [ ] `--fix` stays bootstrap-exempt (starts nothing).

**Tests**:
- `"it prunes stale hooks and stale projects then re-diagnoses clean"` — hermetic: seed a stale hook (live keys minus one) and a gone-dir project; assert both pruned, post-repair report clean for those checks.
- `"it prunes no hooks when live-pane enumeration is empty/errored"` — `HookLister` returns an error (down-server surrogate) with hooks present; assert `hooks.json` unchanged after `--fix` (user commands protected).
- `"it still prunes stale projects on a down server"` — same down-server surrogate; assert the gone-dir project is removed.
- `"the log-sweep runs but never changes the exit code"` — a run where a real check still fails post-repair exits `1`; a run where everything is healthy exits `0`, regardless of what the sweep did.
- `"re-diagnosis drives the post-repair exit code"` — a server-down run: `--fix` prunes stale projects, sweeps logs, but the re-diagnosis still fails daemon/saver/hooks → exit `1`.
- Integration-tagged (`IsolateStateForTest`, `tmuxtest`, `portalbintest`): `"doctor --fix prunes a genuinely stale hook against a real server and re-reads clean"`.

**Edge Cases**:
- Server down → NO hook pruning (reuse `runHookStaleCleanup` hazard guard; protects user-authored on-resume commands); filesystem-only stale-project prune still runs.
- Log-sweep outside the diagnose→repair loop — never touches the exit code (no "logs" catalog check).
- Re-diagnosis exits non-zero if anything remains unhealthy/unfixable.
- Repairs reuse `runHookStaleCleanup` + `project.Store.CleanStale` + `log.SweepLogsForClean` (no new algorithms).
- Stays bootstrap-exempt (starts nothing).

**Context**:
> Spec § `doctor --fix`: "performs the low-stakes, reversible-by-reconstruction repairs it diagnoses: prune stale hooks, prune stale projects, sweep logs." Spec § Log-sweep: "outside the diagnose→repair loop … a deliberate unconditional maintenance side-action … does **not** participate in the exit-code contract." Spec § Down-server guard: "when the server is down … `--fix` performs **no hook pruning** … This protects the 'reversible-by-reconstruction' guarantee — a user-authored on-resume command is *not* reconstructable by Portal … The stale-**project** prune is filesystem-only … and may still run."
>
> `runHookStaleCleanup` (`cmd/run_hook_stale_cleanup.go`) is the load-bearing reuse: its steps 1 and 4 (list error → Warn+return nil; zero-live-with-persisted → defer) mean a down server yields no prune with no extra branch. `project.Store.CleanStale` and `log.SweepLogsForClean` are the exact functions the old `clean` used (`cmd/clean.go`'s `cleanRotatedLogs`/`cleanStaleHooks`), reused directly here so `clean` can be deleted (task 4.7).

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor --fix`; Log-sweep; Down-server guard on the stale-hook prune.

## cli-verb-surface-redesign-4-6 | approved

### Task 4.6: `portal uninstall` — runtime-only teardown (+ delete `state cleanup`)

**Problem**: `state cleanup` hid its meaningful destructive action (`--purge`, which deleted the state dir) behind a flag — the exact inconsistency this redesign removes. It is replaced by a public `portal uninstall` that is runtime-only, fully recoverable, and touches no files. The teardown work that earns the command (locating the daemon, unregistering the exact hook entries) is preserved; the file-deleting purge is dropped entirely.

**Solution**: Add a new public `portal uninstall` command (`cmd/uninstall.go`) that removes only Portal's tmux-server footprint — kill `_portal-saver` (SIGHUP-flushing the daemon), then unregister the global hooks — and prints the fixed completion/recovery message. It touches no state-dir or config files and leaves all sessions (including `_portal-bootstrap`) running. Delete `cmd/state_cleanup.go` entirely, relocating the two still-used helpers (`killSaver`, `isSessionAbsentError`) into `uninstall.go`.

**Outcome**: `portal uninstall` deactivates Portal's tmux machinery, prints the exact recovery message, is a graceful idempotent no-op on already-clean or server-down state, and is fully recoverable (`portal open` re-bootstraps). `state cleanup` and its `--purge` are gone.

**Do**:
- Create `cmd/uninstall.go` with `uninstallCmd` (`Use: "uninstall"`, `Args: cobra.NoArgs`, `SilenceErrors`/`SilenceUsage` as siblings), registered on `rootCmd` in `init()`.
- Relocate from the deleted `state_cleanup.go` (verbatim, still in package `cmd`): `killSaver(c *tmux.Client, logger *slog.Logger) error`, `isSessionAbsentError(err error) bool`, and `killSaverInfoMessage`. Their existing callers (`cmd/abridged_integration_test.go`, `cmd/state_daemon_hysteresis_measurement_test.go`) keep compiling since they stay package-level in `cmd`.
- Introduce an injection seam mirroring the removed `StateCleanupDeps`: `type UninstallDeps struct { Client *tmux.Client; Unregister func(*tmux.Client) error; Logger *slog.Logger }` and `var uninstallDeps *UninstallDeps`, with a `buildUninstallDeps()` that defaults `Client` to `tmux.DefaultClient()`, `Unregister` to `tmux.UnregisterPortalHooks`, and `Logger` to `daemonLogger` (no new log component — the taxonomy is closed).
- `RunE`: build deps; if `client.ServerRunning()` → `killSaver(client, logger)` FIRST, then `unregister(client)` (kill-before-unregister ordering is load-bearing: the daemon's final SIGHUP flush must observe hooks still registered and `_portal-saver` alive at flush start). Accumulate errors via `errors.Join` so a partial failure never short-circuits. If the server is down, skip both (graceful no-op). Then ALWAYS print the completion message.
- Print the exact two-line message verbatim from the spec:
  ```
  Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/.
  To remove Portal completely, uninstall the binary and delete that directory.
  ```
- Delete `cmd/state_cleanup.go` entirely: `stateCleanupCmd`, `StateCleanupDeps`, `stateCleanupDeps`, `buildStateCleanupDeps`, `runPurge`, `purgeStateDir`, the `--purge` flag, and the `init()` that registered `stateCleanupCmd` on `stateCmd`.
- Add `"uninstall": true` to `skipTmuxCheck` (`cmd/root.go`) so uninstall does not bootstrap-then-teardown (EnsureServer/RegisterHooks/EnsureSaver/Restore followed immediately by teardown — circular, wasteful, racy). Update the comment.
- Migrate `cmd/state_cleanup_test.go`: move the `killSaver` / `isSessionAbsentError` behavioural tests to `cmd/uninstall_test.go`; delete the `--purge` / `purgeStateDir` / symlink-refusal tests (that behaviour is gone). Update `cmd/state_test.go` assertions that expect `cleanup` as a visible `state` child (it is removed; Phase 6 hides `state` wholesale).

**Acceptance Criteria**:
- [ ] `portal uninstall` kills `_portal-saver` and unregisters the global hooks when the server is running, in kill-before-unregister order.
- [ ] On a down server it skips kill/unregister and is a graceful no-op, still printing the completion message.
- [ ] Saver already absent / no hooks registered → idempotent success (no error), completion message still printed.
- [ ] It leaves all user sessions AND the `_portal-bootstrap` anchor running (touches only the daemon + hooks).
- [ ] It touches no state-dir or config files — fully recoverable, `portal open` re-bootstraps.
- [ ] No `--yes` gate, no prompt, no `--purge` flag; the printed message is byte-exact to the spec's two lines.
- [ ] `uninstall` is in `skipTmuxCheck`; `state cleanup` (and `--purge`) are removed with no dangling references; `killSaver`/`isSessionAbsentError` are relocated (not deleted).

**Tests**:
- `"it kills the saver then unregisters hooks in order when the server is up"` — inject `UninstallDeps` with a fake client recording call order; assert kill precedes unregister.
- `"it is a graceful no-op printing the message when the server is down"` — `ServerRunning` false; assert no kill/unregister, completion message printed, exit 0.
- `"it succeeds idempotently when the saver is already absent"` — `HasSession(_portal-saver)` false; assert `killSaver` returns nil and the message prints.
- `"it prints the exact completion message"` — assert byte-exact two-line output.
- `"it accumulates a hook-removal failure without skipping the kill"` — `Unregister` returns an error; assert the kill still ran and the joined error surfaces.
- `"uninstall is registered in skipTmuxCheck and state cleanup is gone"` — assert `skipTmuxCheck["uninstall"]` and that `state` has no `cleanup` child.
- Integration-tagged (`//go:build integration`, `IsolateStateForTest`, `SpawnIsolatedDaemon`, `tmuxtest`, `portalbintest`): `"portal uninstall against a real bootstrapped server removes the saver + hooks and leaves _portal-bootstrap and user sessions"` — assert `_portal-saver` gone, managed-event hooks empty, `_portal-bootstrap` and a seeded user session still present.

**Edge Cases**:
- Server down → graceful no-op (skip kill/unregister) still prints the completion message.
- Saver absent / no hooks → idempotent success.
- Leaves all user sessions AND the load-bearing `_portal-bootstrap` anchor running.
- Touches no state-dir or config files (fully recoverable — `open` re-bootstraps).
- No `--yes` gate or prompt.
- Kill-before-unregister ordering preserved for the daemon SIGHUP flush.
- `killSaver`/`isSessionAbsentError` relocated out of the deleted `state_cleanup.go`.

**Context**:
> Spec § `uninstall`: "runtime-only and fully recoverable … touches **no files at all** … Removes only Portal's tmux-server footprint: kills the `_portal-saver` daemon and unregisters the global tmux hooks … Idempotent / nothing-to-remove … Leaves all sessions in place … user sessions **and** the load-bearing `_portal-bootstrap` anchor session are left running." Completion message (verbatim): `Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/.` / `To remove Portal completely, uninstall the binary and delete that directory.` Spec § Bootstrap Exemption: "`uninstall` must be exempt — otherwise it would EnsureServer / RegisterHooks / EnsureSaver / Restore and then immediately tear all of it down (circular, wasteful, racy)."
>
> The existing `stateCleanupCmd` (`cmd/state_cleanup.go`) already implements the kill-before-unregister ordering and the server-down skip; `uninstall` is that logic minus purge. `killSaver` handles the two idempotent-success shapes (`_portal-saver` absent at probe; auto-destroyed between probe and kill → `isSessionAbsentError`). `tmux.UnregisterPortalHooks` (`internal/tmux/hooks_unregister.go`) is consumed as a function value — the relocation keeps its signature.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `uninstall` — Runtime-Only Teardown; Bootstrap Exemption.

## cli-verb-surface-redesign-4-7 | approved

### Task 4.7: Delete `clean` + `state status`, relocate housed helpers, drop `clean` from exempt set

**Problem**: With `doctor` and `uninstall` in place, the grab-bag `clean` command and the subsumed `state status` are dead surface. They must be deleted — but several helpers they house are still used elsewhere (`open` / TUI / `doctor` / the daemon), so those must be relocated rather than deleted, and `clean` must leave the bootstrap-exempt set it currently sits in.

**Solution**: Delete `cmd/clean.go`'s clean-only surface and `cmd/state_status.go` entirely. Relocate the still-consumed helpers (`loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` and the `AllPaneLister` interface) into surviving homes. Remove `clean` from `skipTmuxCheck` and remove the `ErrStatusUnhealthy` silent-exit reference. Keep `internal/state.CollectStatus`/`StatusReport` (doctor reuses them) and `runHookStaleCleanup` (the daemon still calls it) green.

**Outcome**: `portal clean` and `portal state status` no longer exist; the build is green with the daemon's throttled hook cleanup and `open`/TUI/`doctor` still wired to the relocated helpers; `skipTmuxCheck` no longer lists `clean`.

**Do**:
- Relocate config-store helpers out of `cmd/clean.go` into `cmd/config.go` (the `configFilePath` authority) verbatim: `loadProjectStore`, `projectsFilePath`, `loadPrefsStore`, `prefsFilePath` — still called by `cmd/open.go` (lines ~297, ~490, ~523), the TUI wiring, and `doctor` (tasks 4.3/4.5).
- Relocate the `AllPaneLister` interface out of `cmd/clean.go` into `cmd/run_hook_stale_cleanup.go` (its primary consumer) — still referenced by `runHookStaleCleanup` and `cmd/state_daemon.go`.
- Delete the clean-only surface from `cmd/clean.go` (then remove the now-empty file): `cleanCmd`, its `init()` (`--logs` flag + `rootCmd.AddCommand(cleanCmd)`), `CleanDeps`, `cleanDeps`, `buildCleanPaneLister`, `cleanStaleHooks`, `cleanRotatedLogs`.
- Delete `cmd/state_status.go` entirely: `stateStatusCmd`, `ErrStatusUnhealthy`, `staleSaveThreshold`, `renderStatus`, `printWriter`, `daemonLine`/`lastSaveLine`/`warningsLine`/`renderVersion`, `isUnhealthy`, `formatDuration`, `formatBytes`, and its `init()`. **Keep** `internal/state.CollectStatus` / `state.StatusReport` (`internal/state/status.go`) — doctor reuses them.
- Update `IsSilentExitError` (`cmd/state_commit_now.go`) to drop the `ErrStatusUnhealthy` term (leaving `errCommitNowFailed` and `ErrDoctorUnhealthy` from task 4.1). Update the `main.go` `classify` doc comment that names `cmd.ErrStatusUnhealthy` to name `cmd.ErrDoctorUnhealthy` instead.
- Remove `"clean": true` from `skipTmuxCheck` (`cmd/root.go`) and update the comment (drop the "clean" and "Stale hook-entry cleanup … `portal clean` is the manual home" mentions; the manual home is now `doctor --fix`).
- Update the `runHookStaleCleanup` doc comment (`cmd/run_hook_stale_cleanup.go`) to stop naming `cmd/clean.go` / `cleanCmd.RunE → cleanStaleHooks` as a caller — after this task the sole remaining caller is the daemon's `maybeRunHookCleanup` (`cmd/state_daemon.go`). Confirm the daemon path still compiles and is green.
- Migrate/delete the orphaned tests: delete `cmd/clean_test.go`, `cmd/clean_logs_test.go`, `cmd/state_status_test.go`; fold the clean-path hook-cleanup coverage in `cmd/cleanstale_transient_listpanes_clean_integration_test.go` into the `doctor --fix` path (task 4.5) or the daemon-cleanup integration test (both exercise `runHookStaleCleanup`). Update `cmd/version_guard_test.go` (drop the `portal clean` / `portal state status` / `portal state cleanup` exempt-command cases and the `cleanDeps` injection) and `cmd/state_test.go` (drop `status`/`cleanup` visibility assertions).

**Acceptance Criteria**:
- [ ] `portal clean` (and `--logs`) and `portal state status` no longer exist; no dangling references to `cleanCmd` / `stateStatusCmd` / `CleanDeps` / `buildCleanPaneLister` / `cleanStaleHooks` / `cleanRotatedLogs` / `ErrStatusUnhealthy` anywhere.
- [ ] `loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` are relocated (still resolvable by `open`/TUI/`doctor`) and `AllPaneLister` is relocated (still consumed by `runHookStaleCleanup` + the daemon) — none deleted.
- [ ] `internal/state.CollectStatus`/`StatusReport` survive and remain used by `doctor`.
- [ ] `"clean"` is removed from `skipTmuxCheck`; `state` stays exempt; the comment is updated.
- [ ] The daemon's throttled hook cleanup (`maybeRunHookCleanup` → `runHookStaleCleanup`, its sole remaining caller) compiles and is green; the `run_hook_stale_cleanup.go` doc comment no longer names `clean.go`.
- [ ] `IsSilentExitError` no longer references `ErrStatusUnhealthy`; `main.go`'s classify comment is updated; the whole module builds and `go test ./...` (unit lane) is green.

**Tests**:
- `"portal clean and portal state status are not registered"` — assert neither command resolves on `rootCmd` / `stateCmd`.
- `"the relocated config-store helpers still resolve for open/doctor"` — call `loadProjectStore` / `loadPrefsStore` from their new home under `PORTAL_*_FILE` overrides.
- `"AllPaneLister still satisfies runHookStaleCleanup and the daemon"` — compile-time + a unit test driving `runHookStaleCleanup` with a fake `AllPaneLister` from its new home.
- `"skipTmuxCheck no longer contains clean and still contains state/doctor/uninstall"`.
- `"the daemon hook-cleanup path is unchanged"` — reuse/keep the daemon-cleanup unit test (`cmd/state_daemon_hook_cleanup_test.go`) green against the relocated `AllPaneLister`.
- Build/vet gate: `go build ./...` and `go test ./...` (unit lane) green; `golangci-lint run` clean of unused-symbol warnings.

**Edge Cases**:
- Relocate `loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` (still used by open/TUI/doctor) and `AllPaneLister` (still consumed by `runHookStaleCleanup` + daemon) rather than delete.
- `internal/state.CollectStatus`/`StatusReport` survive (doctor reuses them).
- Remove clean-only `cleanStaleHooks`/`cleanRotatedLogs`/`CleanDeps`/`buildCleanPaneLister` and status-only `ErrStatusUnhealthy`/render helpers.
- No dangling `cleanCmd`/`stateStatusCmd` references.
- Daemon's throttled hook cleanup (the sole remaining `runHookStaleCleanup` caller) still compiles + green.
- Update `run_hook_stale_cleanup.go` doc comment naming `clean.go`.

**Context**:
> Spec § `clean` deleted: "`portal clean` and its `--logs` flag are **removed** … No `logs`/`hooks` maintenance namespaces are created." Spec § `doctor` (subsumes `state status`) and Command Surface Summary → Removed public commands: `portal clean [--logs]` → `portal doctor --fix` + automatic daemon pruning; `portal state status` → `portal doctor`. Spec § Bootstrap Exemption: "`clean` **leaves** the exempt set (deleted); `state` **stays**." Spec § Rationale: "**Nothing internal calls `clean` or `state cleanup`** — both were purely manual backstops."
>
> Confirmed non-test callers of the relocated helpers: `loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` — `cmd/open.go`; `AllPaneLister` — `cmd/run_hook_stale_cleanup.go` + `cmd/state_daemon.go`. `runHookStaleCleanup`'s current doc comment names two live callers (the daemon's `maybeRunHookCleanup` and `cleanCmd.RunE → cleanStaleHooks`); after this task only the daemon remains. `IsSilentExitError` currently ORs `errCommitNowFailed || ErrStatusUnhealthy` — task 4.1 added `ErrDoctorUnhealthy`; this task removes the `ErrStatusUnhealthy` term.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` §§ `doctor` — `clean` deleted; Command Surface Summary — Removed public commands; Bootstrap Exemption; Rationale.
