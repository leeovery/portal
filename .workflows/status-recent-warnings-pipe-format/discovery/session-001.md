# Discovery Session 001

Date: 2026-06-08
Work unit: status-recent-warnings-pipe-format

## Description (as of session)

portal state status reports zero recent warnings because its log scanner
still parses the legacy pipe-delimited format while logs are now written in
slog text format.

## Seed

- seeds/2026-06-02-status-recent-warnings-pipe-format.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work originated from an inbox bug that Claude surfaced at the end of the
`portal-observability-layer` implementation session — the user has no prior
first-hand awareness of it, so the shape was settled from the seed plus a
quick code check rather than from user knowledge.

During shaping the seed's central claim was verified against current code:
`internal/state/status.go` (`scanRecentWarnings` / `logEntryQualifies`) splits
each log line on the legacy `" | "` field separator, while `internal/log`'s
handler now writes slog **text** format (`<RFC3339Nano> <LEVEL> <component>:
<msg> <attrs>`). The two formats no longer agree, so `portal state status`
"Recent warnings" silently matches nothing regardless of what the daemon
logged. This is a latent functional regression introduced when the
observability layer changed the log format out from under the status reader,
and it sits outside the scope of every observability-feature plan task.

Settled as a single, self-contained **bugfix**: one coherent scope (the status
reader's format assumption), no new behaviour to design, located root cause.
The open question of *how deep* the mismatch goes — whether only the warnings
scanner is affected or other `portal state status` fields too, and whether the
status tests still seed the old pipe format — is explicitly deferred to the
investigation phase, not a discovery shape question.

User directive carried into investigation: investigate first; if the root
cause is not easily identifiable, stop the work; if it is, proceed to the fix.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
