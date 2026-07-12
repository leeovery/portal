---
status: in-progress
created: 2026-07-12
cycle: 2
phase: Traceability Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Traceability

## Summary

Full fresh bidirectional pass over the specification and all six phase-detail files
(45 tasks). Direction 1 (spec → plan) and Direction 2 (plan → spec) were both re-run
from scratch, not narrowed to the cycle-1 deltas.

The plan remains a faithful, high-depth translation across nearly the whole spec. One
genuine fidelity defect surfaced this cycle (missed in cycle 1): the **picker classifies a
terminal as "unsupported" by `Identity.IsNull()` instead of by adapter resolution**. A
recognised-but-undriven terminal (Apple Terminal — the exact terminal in the delivered
`sessions-unsupported-terminal.png` design frame) produces a **non-NULL** identity
(`com.apple.Terminal`), so `IsNull()` is `false` for it. As authored, the proactive
unsupported banner (Task 6-2) never renders for it, and the N≥2 Enter atomic-no-op gate
(Tasks 6-3/6-9) does not classify it as unsupported — the two branches leave it unhandled
(and 6-3's loose "resolved supported → dispatch" would dispatch a burst against the `nil`
adapter the resolver returns for it). The CLI (Task 2-7) already gates the *identical*
scenario on `resolution == spawn.ResolutionUnsupported` (correct, and explicitly listing
`com.apple.Terminal`), and the spec requires the picker to mirror the CLI — so this is both
a spec-fidelity gap and a plan-internal CLI↔picker inconsistency.

Two coordinated findings capture the single root cause across its two surfaces (proactive
banner; N≥2 Enter gate).

## Findings

### 1. Picker's proactive unsupported banner is gated on `Identity.IsNull()`, so it never fires for a recognised-but-undriven terminal (the delivered Apple Terminal design frame)

**Type**: Incomplete coverage
**Spec Reference**: *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter)* ("the unsupported/unconfigured banner (naming the detected identity) surfaces **proactively** over the normal list … a recognised-but-undriven terminal like Apple Terminal"); *User-facing display: both* (design copy `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal`); *Adapter Contract → Resolution precedence* (config override → native → **unsupported**; "A NULL/unmatched identity → unsupported"); *Design References → Sessions — Unsupported terminal (banner)* (`design/sessions-unsupported-terminal.png`)
**Plan Reference**: Phase 6 — Task 6-2 (`Proactive unsupported/NULL banner`) and Task 6-1 (`Async terminal-detection lifecycle + caching`)
**Change Type**: update-task

**Details**:
Apple Terminal — the terminal named in the delivered design frame the banner must match —
produces a **non-NULL** `Identity` (`BundleID:"com.apple.Terminal"`, `Name:"Apple Terminal"`;
confirmed by Task 1-3 AC: `__CFBundleIdentifier=com.apple.Terminal ⇒ Identity{BundleID:"com.apple.Terminal", Name:"Terminal"}`,
and Task 2-2 AC: `NewIdentity("com.apple.Terminal", …)` resolves to `(nil, ResolutionUnsupported)`).
"Unsupported" is therefore an **adapter-resolution** property (no native adapter and no
`terminals.json` entry), not an `IsNull()` property — `IsNull()` is true only for the
remote/mosh/transient-error NULL cases.

Task 6-2 gates the banner on `m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode`
and renders `renderUnsupportedHeader(m.detectIdentity.Name, m.detectIdentity.BundleID, …)`.
For the design-frame case (Apple Terminal, non-NULL) the gate is **false**, so the banner
never renders — directly contradicting Task 6-2's own Problem statement ("a recognised-but-undriven
terminal like Apple Terminal … the user must learn this proactively") and the delivered frame.
Conversely, for a NULL identity (remote/mosh) the gate is true but the render prints the named
format with **empty** name/bundle (`⚠ unsupported terminal —  · `). The CLI (Task 2-7) gets
this right by gating on `resolution == spawn.ResolutionUnsupported` and by choosing a
`no host-local terminal` line for the NULL case vs the named line otherwise; the picker must
match it.

The correct signal — adapter resolution — is not available to the proactive banner today,
because Task 6-1 caches only the `Identity` and the config-aware `Resolve` seam is not injected
into the model until Task 6-3. The fix therefore (a) moves the config-aware `Resolve` seam
injection up to Task 6-1 and caches the resolution alongside the identity, and (b) re-gates and
re-renders the Task 6-2 banner on the cached resolution.

**Current** (Task 6-1 — Model detection state + `terminalDetectedMsg` handling; the resolve seam is currently injected only in Task 6-3):
> - In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `detectIdentity spawn.Identity`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`.
> …
> - Define `type terminalDetectedMsg struct { identity spawn.Identity }` and add an Update arm: `case terminalDetectedMsg: m.detectIdentity = msg.identity; m.detectResolved = true; return m, nil` (6-9 later extends this arm to resolve a deferred N≥2 Enter).

**Current** (Task 6-2 — Solution / Do step 3 / notice-band helper / AC):
> **Solution** … Insert it as a claimant in the Sessions-page section-header resolver (`applySectionHeader` …), … gated on `m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode`.
> **Do** … 3. **new:** `m.detectResolved && m.detectIdentity.IsNull()` → `renderUnsupportedHeader(m.detectIdentity.Name, m.detectIdentity.BundleID, m.contentWidth(), m.canvasMode, m.colourless)` (first-line replacement, same mechanism as the FilterApplied query header). Gated on `detectResolved` so an **in-flight** identity shows the standard header, not the banner.
> … add a small `func (m Model) unsupportedBannerActive() bool { return m.detectResolved && m.detectIdentity.IsNull() && !m.multiSelectMode }` helper …
> **Acceptance Criteria** … On a supported (non-NULL) resolved identity, no banner shows (standard header).

**Proposed** (Task 6-1 — cache the resolution alongside the identity; inject the config-aware resolver here):
> - In `internal/tui/model.go` add `Model` fields: `detector TerminalDetector`, `detectIdentity spawn.Identity`, `detectResolution spawn.Resolution`, `detectResolved bool`, `detectDispatched bool`. Add test accessors mirroring the existing convention: `func (m Model) DetectDispatched() bool`, `func (m Model) DetectResolved() bool`, `func (m Model) DetectedIdentity() spawn.Identity`, and `func (m Model) DetectedResolution() spawn.Resolution`.
> - Inject the config-aware `Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` seam into the model **here** (not first in Task 6-3): `tui.Deps.Resolve` + `WithResolve`, defaulting in `cmd/open.go` to the **same** `spawn.NewResolver(terminals.json).Resolve` the CLI builds in Task 4-6 (load `terminals.json` once at TUI construction; degrade to an empty native-only config on a `configFilePath` error, exactly as the CLI does). Task 6-3 reuses this already-injected seam rather than injecting its own.
> - Define `type terminalDetectedMsg struct { identity spawn.Identity }` and add an Update arm that caches **both** the identity and its resolution: `case terminalDetectedMsg: m.detectIdentity = msg.identity; if m.resolve != nil { _, m.detectResolution = m.resolve(msg.identity) } else { m.detectResolution = spawn.ResolutionUnsupported }; m.detectResolved = true; return m, nil` (6-9 later extends this arm to resolve a deferred N≥2 Enter). Caching the resolution once (detection is invariant for the picker's lifetime) keeps `rebuildSessionList` off the resolver, consistent with "detect once, cached."
> - Add an AC: "The resolved identity's adapter resolution is cached (`DetectedResolution()`) alongside the identity, computed once via the config-aware resolver; a rebuild never re-resolves."

**Proposed** (Task 6-2 — gate + render on the cached resolution, not `IsNull()`; name a non-NULL identity, honest no-host-local line for NULL):
> **Solution** … Insert it as a claimant in the Sessions-page section-header resolver (`applySectionHeader` …), … gated on `m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported && !m.multiSelectMode` (i.e. any terminal the resolver could not drive — a NULL remote/mosh identity **or** a non-NULL recognised-but-undriven terminal like Apple Terminal — surfaces the banner; a resolved **native**/**config** identity does not).
> **Do** … 3. **new:** `m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported` → `renderUnsupportedHeader(m.detectIdentity, m.contentWidth(), m.canvasMode, m.colourless)` (first-line replacement, same mechanism as the FilterApplied query header). Gated on `detectResolved` so an **in-flight** identity shows the standard header, not the banner. `renderUnsupportedHeader` renders `⚠ unsupported terminal — <Name> · <BundleID>` + right-anchored blue `see docs` for a **non-NULL** identity (the Apple Terminal design-frame case), and the honest `⚠ no host-local terminal` line (no `see docs`) for a **NULL** identity (remote/mosh / transient error), mirroring the CLI's `id.IsNull()` split in Task 2-7 and the Task 6-9 flash text.
> … add a small `func (m Model) unsupportedBannerActive() bool { return m.detectResolved && m.detectResolution == spawn.ResolutionUnsupported && !m.multiSelectMode }` helper …
> **Acceptance Criteria** (replace the "supported (non-NULL)" criterion and add the undriven case):
> - On a resolved-unsupported terminal outside multi-select mode — **whether NULL (remote/mosh) or a non-NULL recognised-but-undriven terminal (e.g. `com.apple.Terminal`)** — the section-header row renders the unsupported banner (the named `⚠ unsupported terminal — <name> · <bundleID>` + blue `see docs` for a non-NULL identity; the honest `⚠ no host-local terminal` line for a NULL identity), and the `Sessions ··· N` header is not shown.
> - On a resolved **supported** identity (resolver returns `native`/`config`), no banner shows (standard header). (Being non-NULL is **not** sufficient — a non-NULL identity the resolver cannot drive is unsupported.)
> - Add a test: `"it renders the named unsupported banner for a non-NULL recognised-but-undriven terminal (com.apple.Terminal)"`.

**Resolution**: Pending
**Notes**: The banner render/copy is spec-grounded (design frame `Apple Terminal · com.apple.Terminal`; CLI Task 2-7's identical NULL-vs-named split). Moving the `Resolve` seam injection to Task 6-1 is the minimal way to give the *proactive* (pre-Enter) banner the resolution it needs; Task 6-3 then reuses the same injected seam (see Finding 2).

---

### 2. Picker's N≥2 Enter atomic-no-op gate branches on `Identity.IsNull()`, so a recognised-but-undriven terminal is neither no-op'd nor safely dispatched

**Type**: Incomplete coverage
**Spec Reference**: *Terminal Identity & Detection → Unsupported-terminal behaviour (banner + Enter)* ("**`Enter` with N≥2** on an unsupported/NULL terminal is an **atomic no-op** … nothing opens (the N−1 external windows need an adapter that isn't available)"); *Spawn Architecture → `portal spawn` CLI behaviour* ("mirrors the picker's commit exactly"); *Reporting & exit codes* ("unsupported/NULL terminal with N≥2 → exit 1")
**Plan Reference**: Phase 6 — Task 6-9 (`N≥2 on unsupported/NULL — atomic no-op + re-asserted banner`) and Task 6-3 (`N≥2 burst dispatch`)
**Change Type**: update-task

**Details**:
The picker's N≥2 branch must take the atomic no-op for **every** terminal the resolver cannot
drive — remote/mosh NULL **and** a non-NULL recognised-but-undriven terminal (Apple Terminal),
exactly as the CLI's Task 2-7 gate does (`if len(external) >= 1 && resolution == spawn.ResolutionUnsupported`).
As authored the picker branches on `IsNull()`:

- Task 6-9's unsupported branch condition is `m.detectIdentity.IsNull()` — **false** for Apple
  Terminal, so it does not take the no-op.
- Task 6-9's supported branch is `!IsNull() && resolve(...) returns a non-unsupported resolution`
  — **also false** for Apple Terminal (its resolution *is* unsupported). So Apple Terminal
  matches **neither** branch and is left unhandled.
- Task 6-3's looser routing ("If resolved supported → dispatch now. If resolved NULL → the 6-9
  no-op.") would classify a non-NULL Apple Terminal as "supported," dispatch the burst, resolve
  to `(nil, ResolutionUnsupported)`, and construct a `burstProgressPipe` bound to a **nil**
  adapter — the burst goroutine then calls `nil.OpenWindow` (panic) or mis-handles the batch.

The fix removes `IsNull()` from the N≥2 branch entirely and branches on the resolution (which
the model can now read from the cache added in Finding 1, or resolve once at Enter): resolve →
if `resolution == spawn.ResolutionUnsupported`, take the Task 6-9 atomic no-op (covering NULL
**and** non-NULL-undriven); else dispatch the burst with the resolved adapter. Task 6-9's flash
text (`unsupportedFlashText`) already distinguishes a named non-NULL identity from the NULL
`no host-local terminal` line, so only the branch **condition** changes; N=1 self-attach
(Task 5-7) is untouched.

**Current** (Task 6-3 — detection gate / dispatch):
> - Detection gate: if `!m.detectResolved` → **defer**: stash a `pendingBurstEnter bool` … return `m, nil`; the `terminalDetectedMsg` arm (6-1) resolves it — supported → dispatch the burst below; NULL → the 6-9 no-op. If resolved supported → dispatch now. If resolved NULL → the 6-9 no-op. (6-3 owns the supported dispatch + the defer plumbing; 6-9 owns the NULL branch.)
> - Dispatch (supported): resolve the adapter `adapter, resolution := m.resolve(m.detectIdentity)`; construct the `burstProgressPipe` … set `m.burstPending = true` …

**Current** (Task 6-9 — Do branch conditions + AC):
> - resolved **supported** (`!IsNull()` and `resolve(...)` returns a non-unsupported resolution) → dispatch the burst (Task 6.3).
> - resolved **unsupported/NULL** (`m.detectIdentity.IsNull()`) → this task's atomic no-op:
> …
> **Acceptance Criteria** — N≥2 Enter on a resolved-unsupported/NULL terminal opens nothing (no `burstProgressPipe`, no adapter resolve/call) …

**Proposed** (Task 6-3 — resolve first, branch on resolution, never on `IsNull()`):
> - Detection gate: if `!m.detectResolved` → **defer**: stash a `pendingBurstEnter bool` … return `m, nil`; the `terminalDetectedMsg` arm (6-1) resolves it, then re-runs this same branch decision against the now-cached identity + resolution. If `m.detectResolved`, decide by **resolution** (read the cached `m.detectResolution` from Finding 1, or resolve once here via the config-aware `m.resolve`): `resolution == spawn.ResolutionUnsupported` → the Task 6-9 atomic no-op (covers a NULL remote/mosh identity **and** a non-NULL recognised-but-undriven terminal like Apple Terminal); otherwise → dispatch the burst below. `IsNull()` is **not** used as the discriminator — a non-NULL identity the resolver cannot drive must take the no-op, not the dispatch.
> - Dispatch (supported ⇒ `resolution ∈ {native, config}`): use the already-resolved non-nil `adapter`; construct the `burstProgressPipe` … set `m.burstPending = true` …

**Proposed** (Task 6-9 — branch on resolution; flash text unchanged):
> - resolved **supported** (`resolve(m.detectIdentity)` returns `native`/`config`, i.e. `resolution != spawn.ResolutionUnsupported`) → dispatch the burst (Task 6.3).
> - resolved **unsupported** (`resolution == spawn.ResolutionUnsupported` — this covers a non-NULL recognised-but-undriven terminal like Apple Terminal **and** a NULL remote/mosh/transient-error identity) → this task's atomic no-op:
>   - Construct no pipe, resolve no adapter beyond the classification resolve, call no adapter method, do not set `m.selected`, do not `tea.Quit`.
>   - Re-assert the unsupported warning: `m.setFlash(unsupportedFlashText(m.detectIdentity))` where `unsupportedFlashText` composes `⚠ unsupported terminal — <name> · <bundleID>` for a non-NULL identity (e.g. Apple Terminal) and folds a NULL identity to `⚠ no host-local terminal — nothing opened`. (Unchanged — the flash already handles both; only the branch condition above changed from `IsNull()` to resolution-based.)
>   - Stay in multi-select mode; leave `m.selectedSessions` intact (no prune).
> **Acceptance Criteria** (replace criterion 1; add the undriven case):
> - N≥2 Enter on a **resolved-unsupported** terminal — whether NULL (remote/mosh) or a non-NULL recognised-but-undriven terminal (e.g. `com.apple.Terminal`) — opens nothing (no `burstProgressPipe`, no adapter `OpenWindow` call), does not self-attach (`Selected()==""`, no `tea.Quit`), and stays in multi-select mode with the selection intact.
> - Add a test: `"it takes the atomic no-op on N>=2 Enter for a non-NULL recognised-but-undriven terminal (com.apple.Terminal), not a burst dispatch"`.

**Resolution**: Pending
**Notes**: Mirrors the CLI's already-correct Task 2-7 gate (`resolution == spawn.ResolutionUnsupported`, explicitly listing `com.apple.Terminal`), satisfying the spec's "portal spawn mirrors the picker's commit exactly." Depends on the cached `detectResolution` from Finding 1 (or an equivalent resolve-once-at-Enter).

---

## Direction 1 — Specification → Plan (completeness)

Every spec section re-verified as covered with implementer-grade depth. All rows below are
covered; the single completeness defect is captured in Findings 1–2 (unsupported classification
in the picker).

| Spec section | Plan coverage |
|---|---|
| Overview / Foundational shape (multi-select `m`, net-N never N+1, Ghostty-first + config, detection walk, no dup guard) | 1-1, 2-6, 4-x, 5-1, 6-3/6-4 |
| Spawn Architecture — one service two callers / `portal spawn` CLI + `--detect` + usage | 1-1, 1-6, 2-6, 6-3 |
| Reporting & exit codes (success self-exec / pre-flight abort / partial / unsupported N≥2 / permission / usage) | 2-6, 2-7, 3-3, 3-4, 3-6, 3-7 |
| N vs N−1 split / Order load-bearing / `os.Executable()` / env PATH-only + TMUX strip | 2-3, 2-6, 3-5, 6-3 |
| Multi-Select (trigger, N=0/N=1, key coexistence, sticky, filter sub-state, affordance, per-session) | 5-1..5-8, 6-5 |
| Burst & Partial-Failure (pre-flight all-or-nothing, token ack, per-window timeout, leave-what-opened, permission burst-stop, cleanup) | 3-1..3-7, 6-6, 6-7 |
| In-picker execution model (async tea.Cmd, in-burst feedback, input-lock, cancellation) | 6-3, 6-5, 6-8 |
| Trigger-Context Matrix (in/out tmux, attached-elsewhere, includes-self, vanished, list order) | 2-6, 3-4, 6-3, 6-4, 6-7 |
| Terminal Identity & Detection (standalone, outside/inside model, bundle-id family, display both, layered keys, no headless, lifecycle) | 1-1..1-6, 6-1, 6-2 |
| Unsupported-terminal behaviour (banner + Enter) | 2-7 (CLI, correct); **6-2 / 6-3 / 6-9 (picker — Findings 1–2)** |
| Adapter Contract (detection separate, precedence, `OpenWindow(command)`, two impls, capabilities) | 2-1, 2-2, 4-6 |
| Config Schema (location/format, structure, argv/script recipe, `{command}`, execution contract, validation, precedence) | 4-1..4-6 |
| Permissions & Error Quarantine (typed result boundary, TCC self-exempt, defensive net) | 2-1, 2-5, 3-7, 6-6 |
| Observability (spawn component, closed attr keys, count semantics) | 1-5, 2-6, 3-5, 6-10 |
| State footprint (reads terminals.json, transient markers only, no sessions.json/daemon/prefs/restore) | 3-2, 4-6 |
| Concurrency & Post-Reboot Safety (burst gated post-hydration; CLI runs own bootstrap) | 6-1 (context), 1-6 (context) |
| Testing Strategy (Adapter fake seam, detection seams, driver split, mode state machine, manual residue) | 2-1, 1-2/1-4, 2-4/2-5, Phase 5, 2-5/5-8/6-11 |
| Design References (3 frames, tokens violet/amber/red no-new, clean selected-only, visual-gate) | 5-2/5-3/5-4/5-8, 6-2/6-7/6-11 |
| Deferred scope + build-time residuals (iTerm2/Terminal.app, ps walk cross-version, Ghostty preview API) | correctly out-of-scope; residuals noted in 1-2/1-3/2-4/2-5 context |

## Direction 2 — Plan → Specification (fidelity / anti-hallucination)

No task content was found that cannot be traced to the specification. The most likely
hallucination candidates all resolved to spec-grounded content or explicitly-flagged
implementation mechanics (re-confirmed this cycle):

- `spawnAckTimeout = 8s` (3-5) → spec "default ~8s per window"; `Poll ≈ 75ms` / hop-bound 32 / env plausibility / friendly-name derivation → each flagged in-task as "not pinned by the spec," realizing spec'd behaviour.
- `total = N incl. trigger`, `opened = confirmed + trigger-on-success`, `Opening` denominator held at N with `burstDone` 0…N−1 (2-6, 6-5, 6-10) → spec Count semantics + "no N/N nag" (self-consistent after cycle-1 integrity fix).
- `friendlyAliases = {ghostty, warp}` and within-config precedence tiers (4-3) → spec's named examples + Precedence verbatim.
- `-1743`/`-1712` → `permission-required` + driver-composed guidance/deep-link (3-7) → spec Defensive net / Architectural boundary.
- section-header-row placement of the multi-select / unsupported / abort / Opening banners → flagged as design-anchored (delivered Paper frames govern placement over the spec's abstract "notice-band single slot") in 5-3, 6-2, 6-5, 6-7 context — reasoned realization, not silent invention.
- `● selected-only, no dim ○` (5-2) → spec "Open toss-up (settled): frames built clean".
- config-aware `Resolve` in the picker burst (6-3), `Burster.Run(ctx, external, progress)` call-site update, `spawn.AckChannelFull` declaration (3-2) → present and consistent (cycle-1 integrity fixes verified).

No task introduces a new colour token, a `--terminal` override, group-select, Spaces
placement, window introspection, the daemon-readable ack follow-on, the defensive
`@portal-spawn-*` sweep, parallel spawn, or the detect-and-wait hardening — all deferrals
are honoured. The `spawn` log component and closed attr set are introduced exactly as the
spec governs, with Phase-1 attr-key scoping guards (1-5).

**Resolution**: Pending
**Notes**: Two coordinated findings, one root cause (picker "unsupported" = `IsNull()` instead of
`resolution == unsupported`). The defect is a genuine miss from cycle 1 (which listed 6-1/6-2/6-9/2-7
as covering unsupported behaviour without catching that only 2-7 gates on resolution). Fixes are
fully spec-grounded from the delivered design frame + the CLI's own correct gate.
