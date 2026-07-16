# Investigation: Ghostty Spawn Opens Zero Windows

## Symptoms

### Problem Description

**Expected behavior:**
Picker multi-select multi-window spawn opens N host-terminal windows (one per selected session) on a real Mac running native Ghostty. The trigger window self-attaches to the Nth session; the N−1 others are externally spawned as new Ghostty windows. On success the notice band / log reports `opened N/N`.

**Actual behavior:**
Three sessions selected, Enter pressed, **zero** windows opened. The notice band showed:

```
'portal-EfVRkk', 'portal-agent-first-3' failed to open — others left open
```

`portal.log` shows `spawn: opened 0/3` for two consecutive batches (17:37:28 and 17:37:33 on 2026-07-16), **no** `portal attach --spawn-ack` process ever starting, and **no** permission-required line. A retry seconds later failed identically.

### Manifestation

- Every external window fails instantly (osascript exits non-zero in milliseconds).
- Suspected error: osascript compile error `-2741` (reproduced locally per discovery).
- The banner suffix "— others left open" renders even though `opened=0` and nothing was actually left open (misleading copy — rider defect #2).
- At production-default INFO log level, the log records *that* windows failed but never *why* — the per-window osascript error detail is emitted at DEBUG only (rider defect #1).

### Reproduction Steps

1. Real Mac, portal 0.9.1, native Ghostty 1.3.1 as host terminal.
2. Launch picker, enter multi-select (`m`), mark ≥2 sessions.
3. Press Enter to dispatch the spawn burst.
4. Observe: zero windows open; notice band reports failure.

**Reproducibility:** Always (feature entirely non-functional on native Ghostty adapter).

### Environment

- **Affected environments:** Local (production binary on developer's Mac).
- **Browser/platform:** macOS, native Ghostty 1.3.1, portal 0.9.1.
- **User conditions:** Host terminal detected as native Ghostty (the config-`terminals.json`-less path → native Ghostty adapter).

### Impact

- **Severity:** High — multi-window spawn is entirely non-functional on the primary supported terminal (native Ghostty). Every burst fails; nothing self-attaches.
- **Scope:** All users spawning via the native Ghostty adapter (no `terminals.json` override).
- **Business impact:** Core feature of a shipped release broken in practice; shipped without live validation.

### References

- Seed: `.workflows/ghostty-spawn-zero-windows/seeds/2026-07-16-ghostty-spawn-zero-windows.md` (inbox:bug)
- Discovery session: `.workflows/ghostty-spawn-zero-windows/discovery/sessions/session-001.md`
- `portal.log` batches at 17:37:28 / 17:37:33 on 2026-07-16 (`spawn: opened 0/3`)

---

## Analysis

### Initial Hypotheses

Seed / discovery diagnosis (to validate, not assume):

1. **Primary:** The AppleScript template in `internal/spawn/ghostty.go` uses `make new surface configuration with properties {…}` and `make new window with properties {configuration:…}`, but Ghostty 1.3.1's scripting dictionary (`Ghostty.sdef`) has **no `make` command** — `surface configuration` is a record-type, and windows are created via a custom `new window` command taking a `with configuration` parameter. The script fails to *compile* (osascript `-2741`), so osascript exits non-zero instantly, the adapter maps it to `SpawnFailed`, and every external window fails in milliseconds. The sdef-correct form is `new window with configuration {command:"…", wait after command:true}`. The in-code "validated (Ghostty 1.3.1)" claim appears never to have been exercised; the `-tags manual` test `TestManual_OpenWindow_OpensRealGhosttyWindow` would have failed with exactly this error.

2. **Rider #1:** Per-window spawn failure detail (`Result.Detail`, the osascript error text) is emitted at DEBUG only in `internal/spawn/logemit.go`, so at production-default INFO the log records failure but not cause. Surfacing at WARN is a spec amendment (the `spawn` log catalog is spec-governed).

3. **Rider #2:** The partial-failure banner suffix "— others left open" in `internal/spawn/message.go` is static and renders even when `opened=0`. The copy is golden-spec-governed and parity-tested across CLI and picker.

### Code Trace

_(to be filled during Code Analysis)_

### Root Cause

_(to be filled during Root Cause Synthesis)_

---

## Fix Direction

_(to be filled during Findings Review & Fix Discussion)_

---

## Notes

- Verification of any fix MUST include running the `-tags manual` Ghostty test plus a live end-to-end multi-select burst confirming `opened N/N` — the absence of live validation is what let this ship.
