---
status: complete
created: 2026-06-01
cycle: 2
phase: Input Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Input Review

## Findings

### 1. Default-level-flip user-visible-impact decision (G7/G10) dropped from spec

**Source**: discussion `### Considered and Rejected / Closed by Prior Decisions`, the **G7, G10** entry (lines 1277): "closed. Resolution: release notes only, no in-band breadcrumb. `portal.log` is a forensic artifact users only look at after the fact, so an in-band INFO line announcing the default change is invisible at the moment it would matter. Existing users who explicitly set `PORTAL_LOG_LEVEL=warn` continue to work unchanged."
**Category**: Enhancement to existing topic
**Affects**: *Log-level discipline* → `### Decision` (spec lines 254-255, where the WARN→INFO default flip is stated) and/or `### Default and invalid-value handling` (spec lines 293-300)

**Details**:
The spec states the production default flips from the historical WARN to INFO (line 255: "changed from the historical WARN — WARN-only was the posture that left no evidence on 2026-05-28") but is silent on the two decided consequences the discussion explicitly resolved:

1. **No in-band breadcrumb announces the change.** The discussion considered and *rejected* emitting an in-band INFO line announcing the default change, on the reasoning that `portal.log` is read after-the-fact so an in-band notice is invisible at the moment it matters; the change is communicated via release notes only. This is a logging-content decision (a deliberate choice NOT to emit a particular line), squarely inside the spec's scope — it is the natural sibling of the other unconditional `process:` lines that the spec *does* enumerate, and a spec writer/implementer might otherwise reasonably add a "default changed" breadcrumb that the discussion deliberately rejected.

2. **Existing explicit-`warn` users are unaffected.** A user who has set `PORTAL_LOG_LEVEL=warn` continues to resolve to `warn` (`source=env`) unchanged — only users with *no* explicit value get the new INFO baseline. The spec's `### Default and invalid-value handling` and the `source` resolution table (`env`/`default`/`fallback`) imply this mechanically, but the decided backward-compat statement ("the flip only affects the unset case; explicit settings are honoured verbatim") is never stated as the user-facing contract it was resolved to be.

Both are decided items, not open questions, and neither is rollout/PR-phasing — they are properties of the level-resolution behavior the spec already documents. The "release notes" delivery mechanism itself is comms (out of scope), but the "no in-band breadcrumb" decision and the "explicit `warn` honoured unchanged" contract are not.

**Current**:
Spec `### Decision` (Log-level discipline), lines 254-255:
```
slog four-level model (Debug / Info / Warn / Error) with the semantic contract below. **Production default `PORTAL_LOG_LEVEL=info`** (changed from the historical WARN — WARN-only was the posture that left no evidence on 2026-05-28). Custom sublevels (Trace/Notice) were considered and rejected — four levels is the standard.
```

**Proposed Addition**:
A short clause under the Log-level discipline Decision (or under Default and invalid-value handling) recording: the WARN→INFO flip affects only the unset case — a user who has explicitly set `PORTAL_LOG_LEVEL` (including `warn`) continues to resolve to that value unchanged; and the default change is deliberately NOT announced via an in-band log line (a `portal.log` notice would be invisible at the after-the-fact moment it would matter), so no "default changed" breadcrumb is emitted. *(Leave blank pending discussion.)*

**Resolution**: Approved
**Notes**: Logged the spec-relevant behaviour into *Log-level discipline* § Default and invalid-value handling (explicit levels honoured unchanged; no in-band "default changed" breadcrumb). The release-notes comms mechanism is explicitly scoped out as a delivery concern.

---
