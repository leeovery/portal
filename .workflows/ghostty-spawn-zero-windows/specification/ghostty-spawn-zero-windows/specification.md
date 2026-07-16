# Specification: Ghostty Spawn Zero Windows

## Specification

## Context: Defect & Root Cause

**Defect.** Multi-window spawn via the **native Ghostty adapter** is entirely non-functional. Selecting ≥2 sessions in the picker and pressing Enter (or running `portal spawn <sessions…>`) opens **zero** host-terminal windows: the log reports `spawn: opened 0/N`, no `portal attach --spawn-ack` process ever starts, no permission-required line appears, and the trigger window never self-attaches. Reproduces 100% on native Ghostty 1.3.1 (the path taken when no `terminals.json` override exists).

**Root cause.** The adapter's AppleScript template (`internal/spawn/ghostty.go`) is written against a **non-existent scripting API**:

```applescript
tell application "Ghostty"
	set surfaceConfig to make new surface configuration with properties {command:"%s", wait after command:true}
	make new window with properties {configuration:surfaceConfig}
end tell
```

Ghostty 1.3.1's scripting dictionary has **no `make` command** and **no `with properties`** terminology (its Standard Suite is trimmed to `count`/`exists`/`quit`); `surface configuration` is a **record-type**, not a makeable class; and windows are created via the custom `new window` command's **`with configuration`** parameter. The script therefore **fails to compile** (osascript `-2741`), exits non-zero in milliseconds without opening anything, and `mapGhosttyResult` maps a non-zero exit whose output contains neither `-1743` nor `-1712` to **`SpawnFailed`**. Every external window in every burst fails identically → `opened 0/N`.

**Why it shipped.** This is a **regression away from a researched-and-recorded API**, not a guess at an unknown one — earlier feasibility research had already resolved the correct `new window with configuration` form, and the implementation drifted to the invalid `make new … with properties` idiom without re-validation. The in-code comment claiming the template was "validated (Ghostty 1.3.1)" is false. The only test exercising real osascript (`TestManual_OpenWindow_OpensRealGhosttyWindow`) is `//go:build manual` — compiled into neither the unit nor integration lane — so no automatable lane could catch a wrong template, and the feature reached tagged release 0.9.1 without live validation.

**Two rider defects** surfaced by (not caused by) the template bug, both in scope:
- **Rider #1:** the per-window osascript failure detail logs at DEBUG only, so at production-default INFO the log records *that* windows failed but never *why*.
- **Rider #2:** the partial-failure banner suffix "— others left open" is static and renders even when `opened=0` (nothing was left open), producing misleading copy.

---

## Working Notes
