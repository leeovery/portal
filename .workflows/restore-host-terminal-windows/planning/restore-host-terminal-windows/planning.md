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
status: draft

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
