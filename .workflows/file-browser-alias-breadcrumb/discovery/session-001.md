# Discovery Session 001

Date: 2026-06-09
Work unit: file-browser-alias-breadcrumb

## Description (as of session)

File-browser alias-save flow (handleAliasSave) emits no audit breadcrumb
because it uses the un-audited Set+Save two-step instead of the audited
SetAndSave seam.

## Seed

- seeds/2026-06-02-file-browser-alias-breadcrumb.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Promoted from the inbox as a bug captured during the
portal-observability-layer review. The shared file-browser "save alias for
highlighted dir" flow — `handleAliasSave` in `internal/ui/browser.go`,
wired into `cmd/open.go` via the `a`-key — is a third production
alias-mutation site that still uses the un-audited two-step
`store.Set(...)` + `store.Save()`. Its `AliasSaver` interface exposes only
`Load`/`Set`/`Save`, so aliases created this way leave no `aliases: set`
breadcrumb in `portal.log`, defeating the observability spec's "single
place per file where the breadcrumb can't be forgotten" guarantee.

The user confirmed the read with nothing to add. Shape is a clear bugfix:
working code is missing the audit breadcrumb every other alias-mutation
site emits — a specific, present-broken behaviour at an identified site,
with no new behaviour to design. The suspected fix (thread this caller onto
the audited `SetAndSave(name, path, "cli")` seam, extending the
`AliasSaver` interface and the file-browser mock accordingly) is named but
left for investigation to confirm; the two callers that Task 3-5 already
instrumented (`cmd/alias.go`, `internal/tui/model.go`) are the precedent.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to investigation.
