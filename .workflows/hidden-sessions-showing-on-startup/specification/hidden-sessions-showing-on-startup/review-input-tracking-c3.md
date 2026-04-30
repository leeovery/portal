---
status: complete
created: 2026-04-30
cycle: 3
phase: Input Review
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Input Review

## Findings

### 1. Stale `tmux-continuum` reference in `StartServer` doc-comment not addressed

**Source**: investigation/hidden-sessions-showing-on-startup.md lines 148-153 (full text of the existing stale doc-comment)
**Category**: Enhancement to existing topic
**Affects**: "Doc-Comment Cleanup → `tmux.StartServer`"

**Details**:
The investigation quotes the full existing `StartServer` doc-comment, which contains two stale third-party-plugin references, not one:

1. *"preventing tmux's default `exit-empty on` from terminating the server before plugins like **tmux-continuum** can restore saved sessions"* (justification line)
2. *"The unnamed session defaults to '0', which **tmux-resurrect** recognizes and cleans up"* (cleanup line)

The spec's "Doc-Comment Cleanup → `tmux.StartServer`" section instructs the implementer to "Drop the tmux-resurrect cleanup claim entirely" and to "Retain the `exit-empty on` rationale for using `new-session -d` rather than `start-server`". The retain-clause is ambiguous about whether the "plugins like tmux-continuum can restore saved sessions" framing should remain, be reframed, or be dropped. With Portal's own resurrection now authoritative (the same justification used to drop the tmux-resurrect line), the tmux-continuum framing is equally stale — Portal's `Restore` step is what now needs the server kept alive, not external plugins.

A reviewer reading the cleanup instructions could reasonably leave the tmux-continuum wording in place, which would result in a comment that is technically less wrong than today but still references a third-party plugin Portal no longer relies on.

**Current**:
> After Fix B, the comment MUST:
>
> - Drop the tmux-resurrect cleanup claim entirely.
> - Document that the session is created with the reserved name
>   `PortalBootstrapName` (`_portal-bootstrap`).
> - Document that the session is hidden from user-facing listings by
>   the underscore-prefix filter in `Client.ListSessions`.
> - Retain the `exit-empty on` rationale for using `new-session -d`
>   rather than `start-server` (this is still load-bearing — commit
>   `bd659a3`).

**Proposed Addition**:
Added a new bullet directing the implementer to drop or reframe the "plugins like tmux-continuum" wording so the `exit-empty on` rationale references Portal's own `Restore` step, not external plugins. Final retain-bullet also updated to require Portal-framing.

**Resolution**: Approved
**Notes**: Auto-approved.

---
