# Plan: Restore Host Terminal Windows

## Overview

Adds a multi-select mode to the Sessions page of the Portal picker: mark N sessions, press `Enter`, and each springs open attached in its own host terminal window (net **N** windows, never N+1 — the trigger window is reused as one). Spawn logic lives in a shared `internal/spawn` package reached two ways: **in-process by the picker** and as a thin **`portal spawn <sessions…>` CLI** — the CLI mirrors the picker's commit exactly and is the primary test seam. Ghostty-first, cross-terminal via built-in Go adapters + a user-config `terminals.json` escape hatch. The hard dependency (`warm-command-bootstrap-latch`) is done and merged.

**Spec:** `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md`

---

## Phases

### Phase 1: Terminal Detection & the `portal spawn --detect` Dry-Run
status: approved
approved_at: 2026-07-12

**Goal**: Stand up the `internal/spawn` package with detect-self identity resolution — the process-tree/env/`list-clients` walk to a macOS bundle id — and expose it through a `portal spawn --detect` dry-run and the new `spawn` log component.

**Why this order**: Terminal identity is the root dependency of adapter resolution, the unsupported banner, and the whole spawn path; the spec makes detection a deliberately standalone operation. A self-contained, fully unit-testable detect surface is the strongest foundation and the first shippable, user-visible increment (`--detect`), with no forward dependency on anything else in the feature.

**Acceptance**:
- [ ] `portal spawn --detect` prints the friendly `.app` name + exact bundle id for a supported local terminal, and prints an unsupported/NULL result (both forms shown) for a remote/mosh client or no local client
- [ ] Detection resolution is unit-tested against fabricated seam data: outside-tmux process-tree walk + env fast-path (`GHOSTTY_*`/`__CFBundleIdentifier`); inside-tmux local-client NULL-filter + local-only `client_activity` tiebreak; bundle-id family match; clean-NULL vs transient-error
- [ ] A transient detection error (`ps`/`defaults read` failure) folds into the unsupported/no-op path and additionally emits a `spawn`-component WARN
- [ ] `portal spawn` with no session args and no `--detect` exits `2` (usage error)
- [ ] The `spawn` log component is registered in the closed taxonomy and emits the detection-outcome event (identity / unsupported / NULL-bundle) with the spec's attr keys

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-1-1 | Package scaffold + Identity model + bundle-id family matching | channel-suffixed bundle id matches its family glob (dev.warp.Warp-Stable → dev.warp.Warp-*); unknown bundle id → passthrough identity (raw id + derived name, not NULL); empty/absent bundle id → NULL identity |
| restore-host-terminal-windows-1-2 | Process-tree walk to bundle id | ancestry reaches ppid-1/mosh-server with no .app → NULL; multi-hop walk (picker → zsh → ghostty); ps or `defaults read` failure → typed transient error distinct from clean NULL |
| restore-host-terminal-windows-1-3 | Outside-tmux detection — env fast-path + walk fallback | __CFBundleIdentifier present → bundle id direct, no walk; GHOSTTY_* present without __CFBundleIdentifier; both env vars absent → walk fallback; empty/malformed env value → walk fallback |
| restore-host-terminal-windows-1-4 | Inside-tmux detection — list-clients NULL-filter + local-only activity tiebreak | only remote/mosh clients → NULL (no host-local terminal); single local client → no tiebreak; 2+ local clients → highest client_activity wins; list-clients failure → typed transient error |
| restore-host-terminal-windows-1-5 | Detect orchestrator + spawn log component | transient error folds to unsupported and emits a spawn WARN; clean NULL emits NULL-bundle outcome with no WARN; resolved identity emits terminal + bundle_id (+ opaque detail) |
| restore-host-terminal-windows-1-6 | portal spawn command — --detect dry-run + usage-error gate | resolved terminal prints friendly .app name + exact bundle id; NULL (remote/mosh / no local client) prints the honest "no host-local terminal" line; no sessions and no --detect → UsageError exit 2; unknown flag → exit 2 |

### Phase 2: Spawn Execution Core — `portal spawn` Opens Windows & Self-Attaches
status: approved
approved_at: 2026-07-12

**Goal**: Deliver a working `portal spawn <sessions…>` on Ghostty: resolve identity → native adapter, compose the env-self-sufficient attach command, sequentially spawn the N−1 external windows, and self-attach the calling window to the Nth.

**Why this order**: This is the fundamental new capability that both callers (CLI and picker) reuse, built directly on Phase 1's resolved identity. It comes before the confirmation contract and the config escape hatch, which harden and extend it. The CLI is the spec-designated test seam, so a runnable happy-path `portal spawn` establishes the foundation the picker later drives in-process.

**Acceptance**:
- [ ] `portal spawn s1 s2 s3` on Ghostty opens N−1 host windows each running `<os.Executable()> attach <session>` and self-attaches the calling window to the Nth (via `AttachConnector` outside tmux / `SwitchConnector` inside)
- [ ] The composed command is an env-self-sufficient argv (`/usr/bin/env PATH=<picker's full PATH> … attach <session>`) with `TMUX`/`TMUX_PANE` stripped, and uses `os.Executable()` (not a PATH lookup) — asserted in unit tests
- [ ] The resolver returns the native Ghostty adapter for a Ghostty identity and `unsupported` for NULL; the Ghostty driver's built `osascript` command (surface-config `command` + `wait after command` + new window) is unit-tested via the pure command-construction split
- [ ] N≥2 on an unsupported/NULL terminal is an atomic no-op exiting `1` with the one-line message on stderr; N=1 self-attaches regardless of terminal (no adapter needed)
- [ ] The full pipeline is exercised through the `Adapter` fake (records "would open command X") with no real terminal; the typed result taxonomy (`unsupported`/`spawn-failed`/`permission-required`) is defined and quarantines all OS-specific detail

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-2-1 | Adapter interface, typed result taxonomy & fake adapter seam | OS-specific detail carried as opaque `detail` never leaked to general code; all four outcomes (success + unsupported/spawn-failed/permission-required) distinguishable; fake records the exact composed argv handed to it |
| restore-host-terminal-windows-2-2 | Adapter resolver — identity → native Ghostty adapter / unsupported | channel-suffixed Ghostty bundle id resolves via family match; NULL identity → unsupported; known-but-no-native-adapter identity (e.g. com.apple.Terminal) → unsupported; passthrough/unknown identity → unsupported |
| restore-host-terminal-windows-2-3 | Env-self-sufficient attach command composition | TMUX/TMUX_PANE stripped even when composed from inside tmux; only PATH injected (no whole-env snapshot); session name with spaces stays a discrete argv element (no shell quoting); os.Executable() error surfaced; --spawn-ack deferred to Phase 3 |
| restore-host-terminal-windows-2-4 | Ghostty driver — pure osascript command construction | `wait after command` present (normal-detach window lifecycle); composed argv embedded with correct AppleScript-string escaping; asserts the built command without running osascript |
| restore-host-terminal-windows-2-5 | Ghostty driver — thin exec boundary + outcome mapping (manual) | non-zero osascript exit → spawn-failed; success → success with opaque `detail`; real window actually opens (live-Mac manual, not automated); permission-code mapping (-1712/-1743 → permission-required) deferred to Phase 3 |
| restore-host-terminal-windows-2-6 | portal spawn <sessions…> pipeline — sequential spawn N−1 + self-attach Nth | N=1 → zero spawns, direct self-attach regardless of terminal; inside-tmux self-attach via SwitchConnector vs outside via AttachConnector; sessions opened in list/arg order sequentially (one completes before the next); any adapter OpenWindow non-success → skip self-attach + exit 1 (detailed leave-what-opened deferred to Phase 3); success self-execs away (no success exit code) |
| restore-host-terminal-windows-2-7 | N≥2 unsupported/NULL atomic no-op exit 1 | N≥2 unsupported → nothing spawns, exit 1, one-line message on stderr, no self-attach; check happens before any adapter call (atomic); N=1 on unsupported still self-attaches (only external-window spawning needs the adapter) |

### Phase 3: Confirmation & Partial-Failure Contract
status: approved
approved_at: 2026-07-12

**Goal**: Complete the full `portal spawn` contract — pre-flight `has-session` gate, `@portal-spawn-*` token-ack confirmation (with `--spawn-ack` on `portal attach`), per-window timeout, leave-what-opened failure handling, and the permission burst-stop.

**Why this order**: This hardens the Phase 2 happy path into the spec's pre-flight + all-or-nothing / leave-what-opened contract, and adds the `--spawn-ack` write point to the existing `portal attach`. It must be locked before the picker (Phase 6) can rely on it, and it finalises the test seam the picker reuses. It follows Phase 2 because it upgrades the self-attach gate from "adapter returned success" to "token ack confirmed."

**Acceptance**:
- [ ] `portal attach --spawn-ack <batch>:<token>` writes `@portal-spawn-<batch>-<token>` immediately before the exec handoff and still execs if the write fails (best-effort), verified in unit tests
- [ ] Pre-flight `has-session` runs before any window opens; if any selected session is gone, nothing spawns, the process exits `1` naming the gone session(s) on stderr, and no self-attach occurs
- [ ] Self-attach happens only after all N−1 windows write their tokens within the per-window `spawnAckTimeout` (default ~8s, timer starts when each spawn fires); a missing token at timeout classifies that window failed → self-attach skipped, opened windows left in place, exit `1` naming the failed window
- [ ] A `permission-required` result stops the burst, surfaces the permission guidance once for the batch (target terminal + Automation-settings hint) on stderr, exits `1`, and leaves opened windows in place
- [ ] Batch/token ids are option-name-safe nanoids (not the session name); the batch markers self-clean on success, pre-flight abort, and reported failure; a test proves `ListSkeletonMarkers` is blind to the `@portal-spawn-` prefix

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-3-1 | Option-safe batch/token ids + `@portal-spawn` marker-name derivation | ids restricted to tmux-option-name-safe charset (why session name is rejected); batch+token independent, collision-resistant across calls; marker name round-trips to (batch, token) with unambiguous delimiter; generator error propagates, never an empty/malformed id |
| restore-host-terminal-windows-3-2 | `@portal-spawn` ack channel seam — write / collect / clean over tmux server options | collect returns only the given batch's tokens (ignores other batches + all `@portal-skeleton-` markers, both directions); `ListSkeletonMarkers` proven blind to the `@portal-spawn-` prefix; clean unsets every batch marker idempotently (already-absent not an error); enumerate/read failure surfaces as error, not false-empty collect |
| restore-host-terminal-windows-3-3 | `--spawn-ack <batch>:<token>` flag on the existing `portal attach` | marker write failure still execs the attach (best-effort); malformed flag value (missing colon / empty batch or token) → usage error exit 2; session-not-found writes no marker, takes existing no-session path; flag absent leaves normal attach unchanged |
| restore-host-terminal-windows-3-4 | Pre-flight `has-session` gate in the spawn orchestrator | multiple gone sessions all named in the one-line message; all-present proceeds unchanged; gone session with N=1 still aborts with no self-attach; `has-session` probe error handled conservatively (abort rather than risk a false open) |
| restore-host-terminal-windows-3-5 | Token-ack self-attach gate + per-window `spawnAckTimeout` | per-window timer starts at its own spawn (cumulative sequential delay never eats a later window's budget — not one global clock); token arriving late but within timeout counts as confirmed; all-confirm self-attaches and cleans markers before the exec handoff; N=1 self-attaches immediately, no ack wait; `spawnAckTimeout` a named/documented/tunable constant (~8s default) |
| restore-host-terminal-windows-3-6 | Leave-what-opened partial-failure handling | one window times out among many → named while others stay open (no teardown); ack-timeout and adapter `spawn-failed` both map to failed classification; self-attach skipped so trigger stays in its calling context; markers self-cleaned on the failure path; exit 1 with the one-line failed-window message on stderr |
| restore-host-terminal-windows-3-7 | `permission-required` burst-stop | permission-required on window k stops windows k+1…N−1 (each hits the same per-(source,target) wall); guidance shown once for the batch (target terminal + Automation-settings hint), not the generic spawn-failed one-liner; windows opened before k left in place; markers self-cleaned |

### Phase 4: Config Escape Hatch — `terminals.json`
status: approved
approved_at: 2026-07-12

**Goal**: Add the user-authored `terminals.json` config-override tier to the resolver — identity-matcher → `commands.open` recipes (`argv` / `script`), `{command}` substitution, within-config most-specific precedence, and tolerant validation with `spawn`-component WARNs.

**Why this order**: This extends the now-complete resolver and failure taxonomy additively — config override → native → unsupported. It is independently valuable (custom/unknown terminals) and cleanly deferrable behind the native path, so it follows the locked core contract without being a prerequisite for it. It comes before the picker so the picker's resolution is complete when the burst wires in.

**Acceptance**:
- [ ] A matching `terminals.json` entry (argv or script) resolves ahead of the native adapter; `{command}` substitutes to the composed env-self-sufficient attach command (dropped in as a single already-resolved string / `$1`)
- [ ] Within-config precedence is most-specific-first (exact raw bundle id → exact `.app`/friendly alias → longer/more-specific `*`-glob → bare `*`), verified for an identity matching several entries
- [ ] A recipe with neither or both of `argv`/`script`, a recipe omitting `{command}`, or a malformed/unreadable file is skipped/ignored (falls through to native → unsupported) with a `spawn` WARN naming the entry; unknown capability sub-keys are ignored (forward-compat)
- [ ] `script` recipes execute the file directly with `{command}` as `$1`, expanding a leading `~`; a missing/non-executable script is an invalid entry (skipped + WARN); a non-zero recipe exit maps to `spawn-failed` and config recipes never produce `permission-required`
- [ ] `terminals.json` resolves via the existing `configFilePath` XDG chain and is read-only at spawn time (no writes, no `sessions.json`/daemon interaction)

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-4-1 | terminals.json store — load + tolerant decode | missing file → empty result, no WARN; malformed/unreadable file → whole file ignored + spawn-component WARN; unknown capability sub-keys (introspect/place) ignored; read-only (load never writes the file) |
| restore-host-terminal-windows-4-2 | Recipe structural validation — exactly-one-of argv/script + {command} presence | neither argv nor script → invalid + WARN; both present → invalid + WARN; recipe template omits {command} → invalid + WARN; valid argv-only / valid script-only accepted |
| restore-host-terminal-windows-4-3 | Identity match + within-config most-specific precedence | identity matches several entries → most-specific wins; friendly-alias key matches via bundle-id family; longer/more-specific *-glob beats broader; bare * catch-all lowest; no match → none (fall through) |
| restore-host-terminal-windows-4-4 | Config argv recipe adapter | {command} dropped in as one literal string (never shell-split); surrounding argv elements pass through verbatim; non-zero recipe exit → spawn-failed; config recipe never returns permission-required |
| restore-host-terminal-windows-4-5 | Config script recipe adapter | leading ~ expanded in script path; {command} delivered as $1; missing script → invalid + WARN (falls through); non-executable / no exec bit → invalid + WARN; non-zero exit → spawn-failed; never permission-required |
| restore-host-terminal-windows-4-6 | Wire config tier into the resolver + resolution observability | config match returns config adapter ahead of a would-match native; no/invalid config entry falls through to native then unsupported; resolution=config vs native logged; no sessions.json/daemon interaction |

### Phase 5: Multi-Select TUI Mode
status: approved
approved_at: 2026-07-12

**Goal**: Add the explicit multi-select mode to the Sessions page — `m` enter/toggle, `Enter` commit, `Esc` exit, session-identity selection, sticky selection, the violet banner + `●` markers, keymap coexistence, filter-as-inner-sub-state, and the N=0/N=1 commit boundary — without the N≥2 burst.

**Why this order**: The mode's state machine is unit-testable as a Bubble Tea model entirely independent of spawning, so building it before the burst integration isolates its risk (keymap, selection stickiness, notice-band arbitration) and delivers the first visual-gate frame. It depends on nothing from Phase 6; Phase 6 depends on it.

**Acceptance**:
- [ ] `m` enters multi-select mode from the Sessions list (mode is enterable with zero selected), `m` again toggles the cursor row, `Enter` commits the marked set, `Esc` exits and clears; `k`/`x`/`r` are suppressed in mode while `Space`/`/`/`s` stay live (and `s`/`m` are literal filter chars while the filter is focused); `HeaderItem` rows stay non-selectable
- [ ] Selection is keyed on session identity: marking any one row of a multi-tag (By-Tag Pattern B) session shows `●` on all of that session's rows and the `N selected` banner counts distinct sessions once
- [ ] Selection is sticky across filtering, paging, regrouping, and the `Space`-preview round-trip, pruning only a selection whose session was externally killed during preview; filter is an inner sub-state where the focused input owns `Enter`/`Esc`
- [ ] The violet multi-select banner owns the notice-band single slot in mode with correct precedence; selected rows carry the `●` marker + mode colour (glyph-backed under NO_COLOR); the footer reads the delivered copy; the `sessions-multi-select-active` frame matches the Paper reference at the visual gate
- [ ] N=0 Enter is a no-op that exits the mode (same effect as `Esc`); N=1 Enter degenerates to a plain single attach in the current window via the existing connector; both verified as Bubble Tea model tests

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-5-1 | Multi-select mode state machine + session-identity selection set | enter mode with zero selected; toggle same row twice (idempotent pair); multi-tag By-Tag session toggled via one row marks the underlying session once; `m` on a HeaderItem row is a no-op; `Esc` exits and clears the whole set; uppercase `M` stays retired |
| restore-host-terminal-windows-5-2 | `●` selection markers on session rows | `●` on every row of a multi-tag By-Tag session; NO_COLOR (glyph survives, violet hue drops); cursor row also marked shows band + `●`; HeaderItem rows never carry `●`; no `●` when not in mode |
| restore-host-terminal-windows-5-3 | Multi-select banner + notice-band single-slot precedence | counts distinct sessions once (multi-tag marked = 1 selected); transient flash outranks the banner; banner outranks the no-tags signpost; filter-focused → filter line owns the slot (banner steps aside); N=0 in-mode still shows "0 selected"; count updates live on toggle |
| restore-host-terminal-windows-5-4 | Multi-select footer copy | exact delivered copy from the Paper design; filter-focused within mode renders the filter footer instead; narrow-width ellipsis degrade; NO_COLOR (glyphs survive, hue drops) |
| restore-host-terminal-windows-5-5 | Keymap coexistence + filter-focus key routing | `k`/`x`/`r` no-op in mode; `Space`/`/`/`s` stay live in mode; `s`/`m` literal while the filter is focused; filter-focused Enter = commit-to-browse and Esc = clear-filter (multi-select Enter/Esc do not fire); `q`/`Ctrl+C` still quit; out-of-mode `k`/`x`/`r` unchanged |
| restore-host-terminal-windows-5-6 | Sticky selection across filter/paging/regroup + Space-preview prune | marks survive `s`-regroup and paging; a filtered-out row stays selected and reappears on clear; preview round-trip returns in-mode with selection intact; externally-killed session pruned during preview (survivors kept); marked session that moves buckets on regroup stays marked |
| restore-host-terminal-windows-5-7 | N=0 / N=1 Enter commit boundary (N≥2 no-op stub) | N=0 Enter exits mode with nothing opened; N=1 Enter attaches that one session via the existing connector (no special-casing); N≥2 Enter is a no-op leaving the mode intact (Phase 6 wires the burst); highlighted-but-unmarked cursor row irrelevant at Enter |
| restore-host-terminal-windows-5-8 | Visual gate — `sessions-multi-select-active` capture fixture + reference | fixture matches the Paper frame (cursor row also marked); NO_COLOR variant renders glyph-backed without crashing; dark appearance only (light-mode variant deferred per spec) |

### Phase 6: Picker Burst Integration
status: approved
approved_at: 2026-07-12

**Goal**: Wire the picker's N≥2 Enter to the in-process spawn service — the async burst `tea.Cmd`, the once-per-session cached detection lifecycle, the proactive unsupported banner + N≥2 gate, leave-what-opened selection mutation, in-burst feedback, and cancellation — completing the feature and its visual gates.

**Why this order**: This is the integration climax that depends on every prior phase: the complete spawn service (Phases 1–4) and the multi-select mode (Phase 5). It can only be built once the service is contract-complete and the mode exists to trigger it. It carries the highest integration risk and delivers the end-to-end feature plus the remaining visual-gate frames.

**Acceptance**:
- [ ] Host-terminal detection runs once asynchronously on Sessions-page entry (off the ~50ms first-paint appearance gate), is cached, and `rebuildSessionList` never re-walks; an unsupported/NULL identity surfaces the proactive banner (friendly name + bundle id + `see docs`) over the normal list, and an in-flight identity at Enter is awaited rather than treated as unsupported
- [ ] N≥2 Enter on a supported terminal runs the in-process pre-flight → sequential spawn → per-window token-ack → self-attach-last burst as an async `tea.Cmd` streaming progress/ack results as `tea.Msg`; the picker is input-locked to row actions while pending (only `Ctrl-C`/`Esc` live) and shows the `Opening n/N…` affordance
- [ ] Full success self-attaches silently (net N windows, never N+1); a post-pre-flight partial failure leaves opened windows in place, skips the self-attach, unmarks the sessions whose windows opened, and keeps the failed/un-acked ones marked so a second Enter retries exactly the missing set
- [ ] Pre-flight abort shows `⚠ '<session>' is gone — nothing opened` and prunes the gone session(s) keeping survivors marked; N≥2 on an unsupported/NULL terminal is an atomic no-op re-asserting the unsupported banner; `Ctrl-C`/`Esc` mid-burst returns to multi-select mode, aborts remaining spawns, leaves opened windows, and self-cleans the batch markers
- [ ] The picker emits the `spawn` batch summary from the chokepoint (one INFO `opened N/N` with `batch`/`terminal`/`bundle_id`/`resolution`/`opened`/`total`; DEBUG per-window with `session`/`ack`); the `sessions-multi-select-preflight-abort` and `sessions-unsupported-terminal` frames match the Paper references at the visual gate

#### Tasks
status: approved
approved_at: 2026-07-12

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-6-1 | Async terminal-detection lifecycle + caching | rebuild (s-toggle/refresh/filter/projects-edit return) never re-dispatches detection; transient detection error caches as unsupported (Phase-1 WARN already emitted); in-flight state distinct from resolved-NULL; direct warm Sessions entry (no loading page) also dispatches exactly once |
| restore-host-terminal-windows-6-2 | Proactive unsupported/NULL banner + notice-band slot | detection in-flight → no banner; supported terminal → no banner; in multi-select mode the multi-select banner owns the slot (unsupported steps aside); NO_COLOR glyph-backed, never colour-only |
| restore-host-terminal-windows-6-3 | N≥2 burst dispatch + async spawn tea.Cmd + streaming message protocol | Enter while detection in-flight → defer decision until resolved then branch; open in list order (selection is a set); trigger session excluded from the N−1 spawn set (net-N); cursor-but-unmarked row never opened; N=0/N=1 still handled by Phase 5 (untouched) |
| restore-host-terminal-windows-6-4 | Full-success self-attach (net N) + marker self-clean | includes-self selection (trigger becomes one marked session); session already attached elsewhere (iPhone) confirmed via ack; no "N/N ✓" nag; trigger-window reuse via existing AttachConnector/SwitchConnector |
| restore-host-terminal-windows-6-5 | Input-lock while pending + Opening n/N… feedback band | second Enter mid-burst ignored (no double-dispatch); m/nav/Space/`/`/`s` all ignored while pending; Ctrl-C/Esc stay live; counter advances with each per-window progress msg; Opening band precedence just below the filter line |
| restore-host-terminal-windows-6-6 | Partial-failure leave-what-opened + selection mutation | permission-required stop (burst-stopped, guidance flash once for the batch naming target terminal + Automation-settings hint, affected session stays marked); retry re-opens only the still-marked missing set; opened windows never torn down; stays in multi-select mode |
| restore-host-terminal-windows-6-7 | Pre-flight abort UI — gone flash + prune keeping survivors | multiple gone sessions named; zero windows opened → nothing to undo; survivor marks intact; same prune rule as the sticky-selection preview round-trip |
| restore-host-terminal-windows-6-8 | Cancellation — Ctrl-C/Esc mid-burst | cancel before first spawn (nothing opened, all stay marked); cancel after some opened (opened unmarked, rest stay marked); Ctrl-C live even while input-locked; after self-exec there is nothing to cancel |
| restore-host-terminal-windows-6-9 | N≥2 on unsupported/NULL — atomic no-op + re-asserted banner | detection in-flight at Enter → awaited then resolves NULL → no-op; transient-error identity treated as unsupported; N=1 self-attach unaffected (no adapter needed); stays in multi-select mode with selection intact |
| restore-host-terminal-windows-6-10 | Spawn batch-summary observability from the chokepoint | full success → opened N/N (trigger self-attach counted); partial/permission failure → trigger self-attach skipped and not counted; unsupported no-op → resolution=unsupported; total=N includes the trigger self-attach target |
| restore-host-terminal-windows-6-11 | Visual gates — capture + wire the remaining frames | the Opening n/N… frame is a new design residual (absent from the delivered Paper set); dark-mode only (light deferred); NO_COLOR glyph-backed variants; move references to testdata/vhs/reference when wiring |

### Phase 7: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-7-1 | Hoist result classification into internal/spawn and add the missing picker permission event | permission-required picker burst emits the dedicated `spawn: permission required` INFO, not the generic `opened 0/N` summary; `PartitionResults` preserves list order and folds `AckFailed`+`AckTimeout` into failed; `FirstPermission` returns the first permission window / false when none; `Confirmed()` truth table; non-permission outcomes + CLI/picker parity unchanged (byte-identical) |
| restore-host-terminal-windows-7-2 | Extract shared gone-session / unsupported-terminal message renderers into internal/spawn/message.go | `GoneMessage` one vs ≥2 names (is/are verb agreement); `UnsupportedNoopMessage` IsNull vs named identity (name + bundle-id U+00B7 middot); `spawn:` prefix applied at CLI sites only; ⚠ glyph still added once by the notice band, not the returned body; output byte-identical at all seven sites |
| restore-host-terminal-windows-7-3 | Extract the shared exec-boundary and failure-detail helpers for the two spawn adapters | `runArgvCombined` over clean exit (out,0,nil) / non-zero exit (combined out + code, nil err) / missing-binary non-exit failure (err surfaced); `execFailureDetail` never-empty fallback per label; the two runner interfaces + Adapter types stay distinct (no seam merge) |
| restore-host-terminal-windows-7-4 | Remove or unexport the dead spawn.AttachCommand public API | no production caller (only a doc-comment + spawntest comment reference); removal preferred, unexport only if a real caller remains; `ExecutableResolver` + `composeAttachArgv` unchanged; go build + spawn tests green, no dangling reference to the removed symbol |
| restore-host-terminal-windows-7-5 | Re-derive the marked set at burst decision time so a deferred N≥2 Enter cannot open a stale selection | mark toggle between a deferred Enter and `terminalDetectedMsg` honoured (unmarked NOT opened, newly marked IS opened); already-resolved non-deferred path unchanged; `pendingBurstOrdered` removed or no longer the source of the spawned set |
| restore-host-terminal-windows-7-6 | Resolve the spawn-failure/permission flash vs multi-select banner notice-slot precedence | decision-first: two-row (document) vs strict single-slot (suppress banner); if suppression, flash presents alone with the retry set still marked + mode intact; if documentation, no behavioural change + a seam comment referencing the spec precedence clause and the pre-flight-abort sibling |

### Phase 8: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-8-1 | Extract the spawn log-emission shapes into internal/spawn shared helpers | four emission shapes (summary/permission/unsupported/gone) exist once in internal/spawn; opened/total derived from `spawn.PartitionResults` on BOTH CLI and picker (no residual inline confirmed-count loop); rendered message + closed attr keys byte-identical at every site; mixed AckConfirmed/AckTimeout/AckFailed count parity; only closed `spawn` attr keys emitted |
| restore-host-terminal-windows-8-2 | Suppress n (new-session-in-cwd) while in multi-select mode | `n` in mode is a no-op (no session created, picker does not quit, marked set preserved); `n` outside mode still creates in cwd and quits; live-set (Space, /, s) and suppressed set (k, x, r, n) match the spec's key-coexistence rule; no other entry point reaches handleNewInCWD in mode |
| restore-host-terminal-windows-8-3 | Run pre-flight before the unsupported gate on the picker burst path | N≥2 + unsupported terminal + one marked session killed → gone message + prune survivors (not the unsupported banner), matching the CLI; no gone session → unsupported atomic no-op fires unchanged; supported path unchanged; ordering holds for both already-resolved and deferred-detection entry points into decideBurst |
| restore-host-terminal-windows-8-4 | Extract the shared partial-failure leave-what-opened message renderer into internal/spawn/message.go | one name vs ≥2 names (QuoteJoin quoting + verb agreement); bare body with no `spawn:` prefix and no ⚠ glyph; CLI adds prefix only at its call site, picker adds ⚠ only via the notice band; CLI exit-1 body and picker flash body identical for the same failed set |
| restore-host-terminal-windows-8-5 | Extract the shared burst test-model construction prefix helper | `Windows: i + 1` convention defined in exactly one place; the four constructors keep only their distinct tail (force burstPending / all-false Confirm / precondition check); all four burst suites (input-lock, cancel, self-attach, partial-failure) pass unchanged; no dead helper |
| restore-host-terminal-windows-8-6 | Promote the nanoid alphabet to a single shared constant referenced by spawn and session | alphabet literal appears exactly once; both naming.go and ackid.go reference the shared constant; no new import cycle (`go build ./...` green — verify dependency direction so internal/spawn gains no cycle); `isOptionSafeID` post-generation check retained; name + ack-id generation unchanged |
| restore-host-terminal-windows-8-7 | Remove (or document) the unreachable OutcomeUnsupported Result taxonomy member | preferred: remove `OutcomeUnsupported` + `Unsupported()` constructor; else document at declaration that `OpenWindow` must never return it (unsupported is a resolution-tier outcome); `ResolutionUnsupported` → atomic no-op unchanged; no dangling reference; `go build ./...` and `go test ./internal/spawn/...` green |

### Phase 9: Analysis (Cycle 3)

**Goal**: Address findings from Analysis (Cycle 3).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-9-1 | Extract the section-header line-0 splice into one shared helper | multi-line `listView` splices header + tail from first newline; single-line no-`\n` returns header bare; empty `listView`; all eight `applySectionHeader`/`applyProjectsSectionHeader` branches preserve conditions/precedence/args and stay byte-identical; `replaceListBodyWithNoMatches` (keep-first-line/replace-body) left untouched; one-row-per-delegate pagination invariant holds |
| restore-host-terminal-windows-9-2 | Delete the four dead burst-outcome fields on Model | `burstBatch`/`burstResults`/`burstIdentity`/`burstResolution` + their `resetBurstState`/`dispatchBurst` assignments removed; no dangling reference (`go build ./...` + `go test ./internal/tui/...` green); burst terminal outcome unchanged (reads from `spawnCompleteMsg`, not the fields); live burst fields untouched; doc comment corrected |
| restore-host-terminal-windows-9-3 | Extract the left-bar single-glyph column renderer | `renderLeftBarGlyphColumn` for `●`/`⚠`/`▌` yields the same 2-cell column (glyph + correct pad width); marked/gone/selected rows byte-identical incl fixed 2-cell width + name left edge; gone → marked → selector precedence in `renderSessionRow` unchanged; unselected branch left as-is |
| restore-host-terminal-windows-9-4 | Unify the footer narrow-degrade fitter across the standard and multi-select footers | shared try-full-then-greedy-prefix-with-ellipsis loop exists once; wide (full cluster) / narrow (prefix + `· …`) / ellipsis-only / sub-ellipsis (empty) all byte-identical with width ≤ budget; per-type renderers (`renderFilterCluster`/`renderFooterCluster`) + each fitter's budget (full vs right-anchor-reserved) stay caller-owned |
| restore-host-terminal-windows-9-5 | Give Outcome a zero sentinel so a zero-value Result is not silently a success | `OutcomeUnknown` at iota 0 → `Result{}.OK()` false; `Success().OK()` true, `SpawnFailed()`/`PermissionRequired()` `.OK()` false; no code depends on prior numeric value (classification via `OK()`/`Confirmed()`/`FirstPermission`); self-attach gate (`Burster.Run` on `result.OK()`) unchanged; `go build`/`go test ./internal/spawn/...` green |
| restore-host-terminal-windows-9-6 | Derive burstAllConfirmed from the shared PartitionResults chokepoint | derived from `spawn.PartitionResults` `failed == empty` (no residual `!r.Confirmed()` loop); true only for error-free full-length all-`AckConfirmed` `spawnCompleteMsg`; false on any `AckTimeout`/`AckFailed`, `msg.Err`, or length mismatch; CLI/picker cross-path parity against `PartitionResults`/`FirstPermission` |
| restore-host-terminal-windows-9-7 | Route the spawn-ack write-failure DEBUG through the enumerated detail attr | DEBUG carries `detail` (= error text) + `session`/`batch`, no `error` attr on the `spawn`-component line; stays DEBUG, best-effort, non-fatal (falls through to `Connect`); message string unchanged; any test asserting old `error` key updated to `detail` |
| restore-host-terminal-windows-9-8 | Fix the --spawn-ack flag help text delimiter label | help names the marker `@portal-spawn-<batch>-<token>` (hyphen, matching `SpawnMarkerName`), no longer implies a colon in the marker name; flag-value delimiter `<batch>:<token>` (colon) distinguished; flag name, default (`""`), and `FormatSpawnAckFlag` parsing unchanged; text-only, no behavioural assertion |

### Phase 10: Analysis (Cycle 4)

**Goal**: Address findings from Analysis (Cycle 4).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| restore-host-terminal-windows-10-1 | Cache the burst Adapter at detection time so dispatchBurst cannot re-resolve to a nil adapter and panic | config-script terminal whose script is deleted / exec-bit-cleared between detection and Enter → no panic (cached adapter fails cleanly through partial-failure, or nil-adapter guard routes to the unsupported no-op); `m.resolve` invoked exactly once per detection; nil `detectAdapter` (undriven capture-harness model) routes to unsupported no-op; native/argv/config-script bursts unchanged for the un-mutated case; redundant second `os.Stat` eliminated |
| restore-host-terminal-windows-10-2 | Extract one shared production spawn-seam builder for the CLI and picker | seven shared seams (detector/resolve/ack/exe/getenv/exists/logger) built in one helper read by both `buildSpawnDeps` and `openConfig`; CLI test-injection via `spawnDeps` still overrides every shared field (builder consulted only for unset); `--detect` dry-run detector resolution unchanged; CLI-only `Connector` + lazy `NewBurster` and picker-only non-spawn fields unchanged; no extra client / terminals.json resolution; byte-for-byte seam equivalence on both paths |
| restore-host-terminal-windows-10-3 | Centralize the net-N split behind a shared spawn.SplitNetN helper | single computation of the external/trigger split used by `runSpawn` and `dispatchBurst`; single-element slice → empty external + that element as trigger; empty-set / N=1 guards preserved (no zero-length slice passed in); byte-identical to the two prior inline slice expressions; sync/async control-flow ordering untouched |
