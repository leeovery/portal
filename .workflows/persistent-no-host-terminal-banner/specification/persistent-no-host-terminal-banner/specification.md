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

## Working Notes
