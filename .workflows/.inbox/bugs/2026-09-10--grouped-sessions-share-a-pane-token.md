# Grouped sessions and linked windows share one pane token

A window that belongs to more than one session — a grouped session created with
`new-session -t <session>`, or a window shared with `link-window` — is listed once per
session by `list-panes -a`, so one live pane produces two enumeration rows carrying the
same `@portal-pane-id` token. Measured on tmux 3.7c against the real capture format:

```
$ tmux -L s -f /dev/null new-session -d -s work
$ tmux -L s set-option -p -t work:0.0 @portal-pane-id TOKEN1
$ tmux -L s new-session -d -s work-clone -t work
$ tmux -L s list-panes -a -F '[#{@portal-pane-id}]|#{session_name}:#{window_index}.#{pane_index} pane=#{pane_id}'
[TOKEN1]|work:0.0 pane=%0
[TOKEN1]|work-clone:0.0 pane=%0
```

The chain from there:

- `internal/state/capture.go` reads the token through that same enumeration and lifts it
  onto every row, so `sessions.json` holds two `Pane` records under two sessions with one
  `portal_pane_id`.
- `internal/restore` rebuilds each saved session independently (`new-session`,
  `new-window`, `split-window`); the grouped relationship is not reconstructed, so the two
  records become two genuinely distinct live panes.
- `internal/restore/session.go` re-stamps the saved token onto each and bakes it into
  `--hook-key`, so both panes are stamped `TOKEN1` and both are armed with the same
  `hooks.json` entry. The `on-resume` command runs in both after the reboot.

It does not end at the boot: the two panes then share one durable identity for good.
`hook set` in either overwrites the same entry, `hook rm` in either removes it for both,
and `hook list` renders one location for two panes.

This is a regression in that corner rather than a pre-existing condition. The double
capture predates the token — but under the old positional key the two rows yielded two
distinct keys, and registration resolved exactly one of them, so exactly one restored pane
was armed. What is new is that the two records carry the same identity. The
specification's §2.1 rejects tmux's `%N` pane id because a recycled id "fires a hook on the
wrong pane rather than losing it"; the token scheme reintroduces that through a different
door, and §2.1's only duplication argument ("a split cannot duplicate an id") addresses
pane creation, not enumeration.

Portal already models linked windows elsewhere (`internal/tmux/hooks_register.go` registers
managed hooks on `window-linked` and `window-unlinked`), and grouped sessions are a
mainstream tmux workflow for driving one window set from two clients.

No coverage exists: nothing in the tree builds a grouped or linked window and runs capture,
restore, or the hook key over it.

The fix needs a decision before code: whether restore should reconstruct a grouped session
as a group (one window set, two sessions), or whether capture should collapse duplicate
rows on `pane_id` so one pane is saved once and the clone is dropped or rebuilt as a group
member. Either way the token stays one-per-pane; the question is what "the pane" means
across the reboot gap when two sessions showed it.

Relevant files: `internal/state/capture.go` (`captureFormat`, `buildPanes`),
`internal/restore/restore.go` and `internal/restore/session.go` (skeleton creation,
`restampPaneToken`, `buildHydrateCommand`), `internal/tmux/tmux.go`
(`ListAllPaneHookKeys`), `cmd/hooks.go` (`paneLocationsByToken`, whose duplicate-token
branch now documents this cause).
