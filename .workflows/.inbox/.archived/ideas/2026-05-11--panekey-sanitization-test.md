# Pin sanitization invariant in panekey_test.go

`buildHydrateCommand` in `internal/restore/session.go:408-436` emits a bare `portal state hydrate ...` string with raw `%s` value-arg interpolation (no shell quoting — `shellQuoteSingle` was deleted in task 8-1). The docstring documents that this is safe because Portal's `sanitizeSessionName` (in `internal/state/panekey.go`) filters `/`, `\`, and `\0` from session names, and pane keys derive from the sanitized session name plus integer indices — so apostrophes never reach the interpolation point.

If a future change to `sanitizeSessionName` ever relaxes that filter (e.g., to support quoted session names), the docstring becomes the only signpost that hydrate-command interpolation depends on the invariant. The bare form would silently break shell parsing in `tmux respawn-pane -k` for any pane whose key contains a literal `'`.

Add a test assertion in `internal/state/panekey_test.go` pinning the filtered character set so the invariant is enforceable by CI rather than load-bearing comment.

Source: review of killed-sessions-resurrect-on-restart/killed-sessions-resurrect-on-restart
