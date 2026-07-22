# Specification: Persistent No Host Terminal Banner

## Specification

## 1. Problem & Target Behaviour

### Context

On the sessions picker, spawn-detection resolves the host terminal identity into one of three shapes: **supported** (a driveable adapter — native Ghostty or a `terminals.json` match), **named-unsupported** (a recognised bundle id with no adapter, e.g. Apple Terminal), or **NULL/remote** (mosh/SSH — no host-local terminal attached to the tmux server). Two surfaces key off the coarse `DetectUnsupported()` resolution without distinguishing the NULL and named shapes, producing two defects on unsupported terminals:

1. **Persistent noise banner on remote clients.** The proactive unsupported-terminal banner *replaces* the `Sessions ··· N` section header (count + grouping-mode suffix) for the whole picker session. For a NULL/remote client the banner (`⚠ no host-local terminal`) carries nothing actionable — no bundle id, no `see docs` hint — yet permanently costs the user their session count and grouping indicator.
2. **Walkable dead-end multi-select.** On *any* unsupported terminal (NULL or named), pressing `m` enters multi-select mode and lets the user mark sessions, only to dead-end at the N≥2 Enter with a reactive no-op flash. The affordance is offered for a burst that can never fire, and `m` is listed unconditionally in the `?` help.

Neither is a regression — both surfaces work to the original `restore-host-terminal-windows` spec, which modelled "unsupported" as one flat state served by one banner + one reactive burst-time no-op. That is correct for the named-undriven case but wrong for NULL/remote and wrong as the *primary* multi-select gate.

### Target Behaviour

On a picker session where detection has resolved an unsupported terminal:

- **NULL/remote identity:** no proactive banner. The standard `Sessions ··· N` header (count + grouping-mode suffix) renders normally, and the By-Tag "no tags yet" signpost behaves as on any supported client.
- **Named-unsupported identity:** the proactive banner is **kept unchanged** — it is actionable (carries the bundle id, the copy-paste key for `terminals.json`, and the `see docs` hint).
- **Multi-select (`m`) on any unsupported resolution (NULL or named):** the mode does **not** open. Pressing `m` fails immediately with a transient flash (self-clearing on the next keypress) instead of walking the user to a guaranteed dead-end. `m` is omitted from the `?` help while unavailable.

### Solution Shape

Four coordinated, independently-testable TUI-side sub-fixes (banner split, proactive `m`-entry block, help-modal `m`-suppression, blocked-entry flash copy). No CLI change; no state/daemon/`sessions.json`/`prefs.json` footprint — spawn's near-zero state footprint is unchanged.

---

## 2. Sub-fix 1 — Banner Split by Identity Shape

### Change

Add an `IsNull()` identity-shape discriminator to `unsupportedBannerActive()` (`internal/tui/model.go`) so the predicate is true **only for a named-unsupported identity**, false for NULL/remote. Currently it is `DetectUnsupported() && !multiSelectMode`, which fires for *any* unsupported resolution; the new form additionally requires `!m.detectIdentity.IsNull()`.

### Behaviour

- **Named-unsupported:** predicate stays true → the banner still replaces the section header, unchanged (`⚠ unsupported terminal — <name> · <bundleID>` + right-anchored `see docs`).
- **NULL/remote:** predicate now false → the banner never activates; the standard `Sessions ··· N` header (count + grouping-mode suffix) renders normally.

### Why one gate covers both surfaces

`unsupportedBannerActive()` is the single predicate read by **two** consumers, so the discriminator fixes both coherently in one place:

1. **`applySectionHeader`** — swaps in the unsupported banner in place of the title row. With the split, NULL no longer claims the header row.
2. **`activeNoticeBand`** — reads the same predicate to *suppress* the By-Tag "no tags yet" signpost while the banner is active. With the split, a NULL/remote client that has no tags now shows the signpost again (correct — there is no banner competing for the slot).

The renderer already knows the NULL/named split (`renderUnsupportedHeader` / `unsupportedLeftCluster` branch on `bundleID == ""`); only the *gate* was blind to it. This sub-fix adds the missing discriminator at the gate — it does not change the renderers (the fate of the now-unreachable NULL render branch is Topic 6).

---

## 3. Sub-fix 2 — Proactive Multi-Select Entry Block

### Change

Gate the entry branch of `handleMultiSelectToggle` (`internal/tui/model.go`) on `DetectUnsupported()`. Today the entry branch (`if !m.multiSelectMode { multiSelectMode = true; …mark-on-entry… }`) has **no** detection read; the only unsupported gate is downstream at `decideBurst`'s N≥2 Enter. The fix adds a proactive check: when `DetectUnsupported()` is true, pressing `m` does **not** open the mode — it sets a transient blocked-entry flash instead (copy defined in Topic 5) and returns.

Applies to **both** unsupported shapes (NULL and named) — `DetectUnsupported()` is the coarse resolution predicate; the entry block is deliberately identity-blind (only the *flash copy* differs by shape, Topic 5).

### Retain the reactive backstop (Fork A → A1)

`decideBurst`'s reactive unsupported no-op (`internal/tui/burst_progress.go`, the N≥2-Enter arm that emits `spawn.UnsupportedNoopMessage` and flashes) is **retained**. It is not redundant: detection is asynchronous, so the entry block cannot fully replace it.

### Async in-flight window (why the backstop is load-bearing)

- Detection dispatches on Bubble Tea's command goroutine on reaching the Sessions page and resolves later via `terminalDetectedMsg`. Until it resolves, `detectResolved == false` → `DetectUnsupported() == false` → the entry block does **not** fire, so a user *can* enter multi-select during the in-flight window.
- **Fork A resolved to A1 (leave the reactive backstop; no mid-mode eject).** If the user entered multi-select during the in-flight window and detection then resolves unsupported, the mode is **not** ejected. The `terminalDetectedMsg` arm continues to only cache identity/adapter/resolution (and resolve a `pendingBurstEnter` deferral) — it does not close an open multi-select mode. The reactive `decideBurst` no-op remains the sole backstop for the "entered-before-resolve → Enter" path.
- Rationale: the in-flight window is tiny and ejecting a user mid-interaction is jarring. A2 (eject on resolve) was explored and rejected for that reason.

### Net effect

Once detection has resolved unsupported, `m` is proactively blocked at entry. Before resolution, the mode is enterable but the burst is still caught reactively at Enter. Supported terminals are unaffected — `m` enters and dispatches as today.

---

## 4. Sub-fix 3 — Help-Modal `m`-Suppression

### Change

When `DetectUnsupported()` is true, filter the `m` (multi-select) entry out of the keymap descriptor slice passed to the help modal **at the call site** (`renderHelpModalOnClearedCanvas`, `internal/tui/model.go`). `sessionsKeymap()` itself stays a pure static constant — the filter is applied to the copy fed to the modal, not baked into the descriptor function.

### Behaviour

- **Unsupported (NULL or named):** the `?` help body omits the `m` row (consistent with `m` being blocked at entry).
- **Supported:** `?` help lists `m` as today.
- **Footer unchanged either way.** `m` is a non-`Core` descriptor entry, so `renderCondensedFooter` never lists it — the footer needs no change under any resolution.

### Why call-site filter, not a parameterised keymap

A parameterised `sessionsKeymap()` (dropping `m` inside the descriptor function) is **rejected**. `keymap_dispatch_guard_test.go` probes the *static* descriptor against an unwired-detection model (where `DetectUnsupported()` is false → `m` supported → dispatch enters the mode) to guard descriptor↔dispatch parity. A call-site filter leaves that static descriptor — and therefore the guard — green; parameterising the descriptor would break it. The descriptor is meant to remain the single static source for dispatch parity.

### Latent guard-coupling note (carry into implementation)

Sub-fix 3's guard-safety depends on `sessionsGuardModel` (`NewModelWithSessions`) keeping detection **unwired**, so `DetectUnsupported()` is false and the `m` dispatch probe still enters the mode. This is true today. Sub-fix 2's entry block makes `keymap_dispatch_guard_test` newly sensitive to that seed state: a future change that wires detection into `NewModelWithSessions` (or defaults `DetectUnsupported()` true) would make the `m` probe hit the block and fail. This coupling is **not introduced** by this fix, but an inline source note near the entry-block gate / the guard probe should record it so a later reader understands the dependency.

---

## Working Notes
