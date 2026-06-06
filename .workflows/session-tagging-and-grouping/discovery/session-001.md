# Discovery Session 001

Date: 2026-06-06
Work unit: session-tagging-and-grouping

## Description (as of session)

Tag-based organization for the session list: projects carry tags inherited by
their sessions, with optional per-session tags, and the picker can
group/aggregate sessions by tag via a toggle.

## Seed

(none)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The user runs many tmux sessions at once (currently ~15–20), which the Portal
Open session list presents in flat alphabetical order — past a certain count,
that flat list stops being legible. The desire is to slice the list different
ways on demand and aggregate sessions into logical groups, flipping between
views with a toggle.

The initial framing was three fixed grouping modes — by directory, by project,
or by hand-rolled custom buckets (e.g. work / personal). The user then reframed
toward a more general primitive: **tags**. Tag a project and its sessions
inherit that tag; optionally tag individual sessions directly too. Grouping then
becomes "aggregate by tag", with directory/project either derived facets or
just built-in tags over the same machinery.

The user explicitly confirmed this hangs together as **one cohesive feature**,
not several independently-shippable pieces — the tag model/persistence, the
project→session inheritance rule, the aggregated/grouped TUI view, and
assigning/managing tags only make sense delivered together. Confirmed work type:
feature.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to discussion.
