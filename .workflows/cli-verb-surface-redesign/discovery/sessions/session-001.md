# Discovery Session 001

Date: 2026-07-15
Work unit: cli-verb-surface-redesign

## Description (as of session)

Intentional one-pass redesign of Portal's full CLI verb surface — reconciling open/attach/spawn and auditing every command, with a back-compat/deprecation story.

## Seed

- seeds/2026-07-09-cli-verb-surface-redesign.md (inbox:idea)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work originates from the promoted inbox idea captured while naming the window-spawn command during the `restore-host-terminal-windows` discussion. Portal's CLI grew by accretion — commands added as needed with no holistic design pass — and the verb surface has drifted past coherent: even the author can no longer cleanly recall the difference between `open` and `attach`. The current shape is `portal open` (no args → TUI picker; one arg → path/query resolution through path → alias → zoxide → session, then attach in place), `portal attach <session>` (attach in place to a named session), the `x` alias, the provisionally-named `portal spawn <sessions…>` (open host-terminal windows for N sessions), and the utility commands `hooks`, `clean`, `init`, `state`, `alias`, `version`.

The core problem: overlapping, blurry verbs with illegible input domains (path/query vs single session name vs multi-session). A live design question carried from the seed: should the window-spawn operation stay a distinct `spawn`, or fold into a variadic `attach foo bar baz` where argument count decides attach-in-place vs spawn-new-windows? The author likes variadic-attach (it matches the session-name input domain) but notes the count-dependent behaviour split. The redesign must also settle where the picker sits in the mental model and carry a compatibility/deprecation story (back-compat aliases), since existing commands live in user muscle memory and scripts.

Shaping settled one scope question explicitly: the user chose a **full audit** of the whole command list — `hooks`, `clean`, `state`, `alias`, `init` and friends included — not just reconciling the three overlapping verbs. The work is one coherent, ship-able design pass (rename/restructure commands plus back-compat aliases), with the substantive verb-design debate deferred to the discussion phase. Confirmed as a feature.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
