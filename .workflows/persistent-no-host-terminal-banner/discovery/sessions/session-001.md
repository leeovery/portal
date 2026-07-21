# Discovery Session 001

Date: 2026-07-21
Work unit: persistent-no-host-terminal-banner

## Description (as of session)

On remote/unsupported terminals the picker's proactive "no host-local terminal" banner behaves poorly and multi-select is a dead-end affordance. Split the banner by identity shape (drop it entirely for the NULL/remote identity, keep it for named unsupported terminals where it's actionable) and disable multi-select outright on any unsupported resolution — surfacing a transient, self-clearing flash on `m` rather than a permanent band, and hiding `m` from the help keymap when unavailable.

## Seed

- seeds/2026-07-16-persistent-no-host-terminal-banner.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox bug logged 2026-07-16. The user confirmed the picked-up shape matches: on a pure-remote client (mosh/SSH, NULL terminal identity resolved by spawn detection) the §6.2 proactive unsupported-terminal banner shows permanently for the whole picker session and, because it *replaces* the standard `Sessions ··· N` section header (`internal/tui/model.go`, `unsupportedBannerActive`), the session count and grouping-mode suffix are lost for the session. On a remote client this is pure noise — nothing the user can act on — even though it is behaving exactly as spec'd. Separately, multi-select `m` still enters the mode on any unsupported terminal and lets the user mark sessions, only to dead-end at the N≥2 Enter with a reactive no-op flash (verified against `burst_unsupported_noop_test.go`): a misleading affordance for a burst that can never fire.

The decided direction, carried in from the report and re-affirmed: (1) **banner split by identity** — drop the proactive banner entirely for the NULL/remote identity so the normal section header renders, but keep it for *named* unsupported terminals (e.g. Apple Terminal) where it is actionable, carrying the bundle id (the copy-paste key for `terminals.json`) and the `see docs` hint; (2) **multi-select fully disabled on any unsupported resolution** (named and NULL alike) — `m` fails immediately rather than entering the mode, not deferred to the N≥2 Enter and not gated on selection count.

The user added refinements during shaping: `m` should also **not appear in the `?` help keymap** on unsupported terminals, and the blocked `m` keypress should surface a **transient error banner that self-clears on the next keypress** (a flash) rather than a silent no-op or a permanent band. Rationale: the transient error can be genuinely helpful — it can point the user at `terminals.json` config to enable the functionality — without nagging the whole session. The exact flash copy (a variant suited to a blocked mode-entry vs the current `— nothing opened` burst wording) and the help-suppression mechanics are design detail deferred to investigation/spec.

Classification was discussed explicitly: strictly this is not a regression — the banner and reactive-only `m` gating both work as designed — so it's a *decided feature that turns out wrong in a case it didn't cover*. Settled as a **bugfix** on the basis that bugfix here means correcting wrong existing behaviour rather than building new capability or defining a pattern; the outcome is understood and touches existing surfaces. Investigation is expected to be a light confirm pass over the code loci (`unsupportedBannerActive` in `model.go`, the `m`-entry handler, `burst_unsupported_noop_test.go`, the help modal) before the spec does the real design work.

Scope boundary noted and kept: the CLI multi-target `portal open <a> <b> …` (N≥2) burst block for unsupported/remote is owned by the in-flight `cli-verb-surface-redesign` feature (which introduces the CLI burst in the first place), not by this bug. That feature is expected to land before this bug is implemented. This bug stays TUI-side (banner split + `m`-entry block); when actioned, keep its copy/UX consistent with the CLI's unsupported message. Also related but separate: `2026-07-15--remote-trigger-spawns-on-local-terminal` — once its trigger-locality gate is fixed, every remote login resolves NULL, so this banner would otherwise appear on all remote use.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
