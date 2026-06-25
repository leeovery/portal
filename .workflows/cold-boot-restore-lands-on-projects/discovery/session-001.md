# Discovery Session 001

Date: 2026-06-25
Work unit: cold-boot-restore-lands-on-projects

## Description (as of session)

Cold-boot TUI restore lands on the Projects page instead of Sessions despite N sessions being restored.

## Seed

- seeds/2026-06-25-cold-boot-restore-lands-on-projects.md (inbox:bug)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox bug captured 2026-06-25 while building the cold-boot resurrection demo (`demo/`, a sandboxed Linux container with a baked 12-session restore seed). On a **cold** start (no tmux server yet) through the TUI picker, the concurrent bootstrap restores every saved session correctly — the loading screen reports `Restoring sessions N/N` accurately — but when the picker appears it opens on the **Projects** page rather than **Sessions**, so the user must press `x` to reach the sessions they just resurrected. The **warm** path (tmux server already running) opens on Sessions as expected, so the defect is specific to the cold concurrent-bootstrap landing decision; restore itself is fully correct, only the initial page selection is wrong.

The user has not reproduced it personally — an agent running portal + tmux in a container environment observed it — and wants the bugfix to start by exploring the code to confirm it's a real, recognisable defect (the investigation phase is free to conclude it is not). The seed's own (unverified) hypothesis points at the Loading → page transition choosing Sessions-vs-Projects from a session count captured before the restored sessions are visible to `ListSessions` — an ordering/race between restore completion on the `BootstrapCompleteMsg` path and the "no sessions yet → fall back to Projects" landing rule — and suggests checking the cold-path landing decision in `internal/tui/model.go` against the warm path. Treated as a hypothesis to verify in investigation, not an asserted cause.

Shape settled quickly: a concrete reproducible symptom, a clear violated expectation (warm path lands on Sessions), an isolated suspect, no new behaviour to build — a bugfix routing to investigation. No tangential concerns surfaced for the inbox.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
