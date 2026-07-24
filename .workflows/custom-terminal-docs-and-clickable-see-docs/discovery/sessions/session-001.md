# Discovery Session 001

Date: 2026-07-24
Work unit: custom-terminal-docs-and-clickable-see-docs

## Description (as of session)

Write terminals.json custom-terminal setup docs and make the picker's
unsupported-terminal "see docs" banner hint resolve to them — an OSC 8
clickable link if feasible, otherwise leaving the bare "see docs" word; the
banner never prints a URL or path.

## Seed

- seeds/2026-07-22-custom-terminal-docs-and-clickable-see-docs.md (inbox:quickfix)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Picked up from the inbox quick-fix. The sessions-picker unsupported-terminal
banner (named-unsupported case) renders a blue "see docs" hint that points
nowhere: no URL or path is shown, and no `terminals.json` setup documentation
exists in the repo. The hint is a promise pointing at nothing. It surfaced
while specifying the `persistent-no-host-terminal-banner` bugfix, which
deliberately leaves the named banner unchanged — so this is picked up
separately as additive docs + link work.

Two parts: (1) a docs page explaining how to set up a custom/unknown terminal
via `terminals.json`, using the bundle id shown in the banner as the
copy-paste key for the config entry; (2) making the "see docs" hint actually
actionable.

The user settled the banner-degradation behaviour during shaping: the banner
must **never** print a URL or a path — there isn't enough horizontal room. So
if a clickable link is feasible (OSC 8 escape sequences — known possible;
Claude Code renders links this way), the "see docs" word upgrades in place to
a clickable link pointing at the docs page. If OSC 8 isn't viable across the
supported terminals, it degrades cleanly to the bare "see docs" word as it is
today (which is self-explanatory enough), with no printed URL/path fallback.
Better if it's a link, acceptable if it's just the word.

The banner copy lives in `internal/tui/section_header.go`
(`unsupportedDocsHint = "see docs"`, rendered in `renderUnsupportedHeader`).
The docs page location is TBD — a shape/feasibility question (repo Markdown
vs hosted URL, and OSC 8 link-target implications) deferred to scoping.

Shape converged quickly: a small, additive, mostly-mechanical change with one
coherent scope, no behaviour to debate beyond the degradation rule above, and
nothing broken to diagnose — a quick-fix. Confirmed by the user.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to scoping.
