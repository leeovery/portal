# Discovery Session 001

Date: 2026-07-16
Work unit: ghostty-spawn-zero-windows

## Description (as of session)

Native Ghostty spawn adapter AppleScript uses commands absent from Ghostty 1.3.1's scripting dictionary, so multi-window spawn compile-fails and opens zero windows; includes two riders (DEBUG-only failure logging, misleading total-failure banner copy).

## Seed

- seeds/2026-07-16-ghostty-spawn-zero-windows.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work originated from an inbox bug report captured during the first live test of the picker's multi-window multi-select spawn on a real Mac (portal 0.9.1, Ghostty 1.3.1): three sessions selected, Enter pressed, zero windows opened, notice band reported the sessions failed to open. portal.log showed `spawn: opened 0/3` for two consecutive batches, no `portal attach --spawn-ack` process ever starting, and no permission-required line.

A prior same-day analysis session diagnosed the likely root cause (to be validated by investigation, not taken as fact): the AppleScript template in `internal/spawn/ghostty.go` uses `make new surface configuration …` / `make new window …`, but Ghostty 1.3.1's scripting dictionary has no `make` command — `surface configuration` is a record-type and windows are created via a custom `new window` command taking a `with configuration` parameter. The malformed script fails to compile (osascript -2741, reportedly reproduced locally), so osascript exits non-zero in milliseconds, the adapter maps it to SpawnFailed, and every external window fails instantly. The sdef-correct form (`new window with configuration {command:"…", wait after command:true}`) was said to compile against the installed Ghostty. The in-code "validated (Ghostty 1.3.1)" claim appears never to have been exercised; the `-tags manual` test would have caught it.

Two secondary defects surfaced in the same diagnosis and are in scope as riders on this fix: (1) per-window spawn failure detail (the osascript error text) is emitted at DEBUG only (`internal/spawn/logemit.go`), so at production-default INFO the log records that windows failed but never why — and the `spawn` log catalog is spec-governed, so surfacing failures at WARN is a spec amendment; (2) the partial-failure banner suffix "— others left open" (`internal/spawn/message.go`) is static and renders even when opened=0 and nothing was left open — golden-spec-governed copy, parity-tested across CLI and picker.

Shaping decision: the user brought two spawn-area bugs from the inbox together (this one plus a persistent "no host-local terminal" banner / multi-select-gating bug). We weighed doing them as one bugfix vs. separately. Reasons to split surfaced: the two are at different pipeline stages (this one has genuine root-cause-to-validate work; the banner one reads as already-decided-behaviour wanting spec, not investigation), and the banner bug's natural sibling is a *third* inbox bug (`remote-trigger-spawns-on-local-terminal`), not this one — the Ghostty adapter is the self-contained, independently-shippable fix. The user chose to split the Ghostty fix out and take it first; the banner bug stays in the inbox. Work type confirmed as bugfix (something that worked in principle is broken in practice, with a root cause to confirm) → routes to investigation.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
