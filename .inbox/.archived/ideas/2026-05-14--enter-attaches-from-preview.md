# Enter attaches from preview

Close the natural Space → look → Enter → attach loop on the Sessions page by wiring Enter from the scrollback-preview page to attach to the previewed session. Today the preview's `Update` in `internal/tui/pagepreview.go:257-317` handles Esc/Home/End/Tab/`]`/`[` but has no `tea.KeyEnter` case; Enter falls through to `viewport.Update(msg)` (a silent no-op for viewport navigation), so the user has to press Esc to dismiss and then Enter on the highlighted session to do something the affordance suggests should be a single keystroke.

The current behaviour matches the existing spec, so this is a spec amendment rather than a bug fix. `session-scrollback-preview/specification.md:60-72` lists preview's owned keymap as `]`, `[`, `Tab`, `Esc`, and the keymap-policy paragraph explicitly says "Everything else either passes through to the embedded bubbles/viewport (scroll keys) or is unbound/no-op". The user's mental model — that Enter attaches — is reinforced by spec line 17 ("Attach. `Enter` continues to attach as today (unchanged).") which reads that way in isolation, even though in surrounding context it is scoped to Sessions-page behaviour, not preview. Worth pinning down through discussion before implementing.

Threads to draw out in discussion:

- Does Enter attach to the **session** (matching the "preview is per-session" framing) or to the **focused pane** (Tab/`]`/`[` make pane focus a real concept in preview — does it carry over to the attach action)?
- Should the transition be instantaneous, or two beats (dismiss then attach) with a perceptible state change?
- What is the behaviour when the preview is mid-load and the placeholder is showing — still attach, or block until content resolves so the user has confirmed what they are attaching to?
- What is the behaviour when the filter is committed and the list is in an unusual state (zero matches, etc.) — same as Sessions-page Enter, or no-op?
- If we lift Enter from the "everything else is no-op" rule, where does the line sit for other keys with obvious analogues (e.g. `r` to rename in place, `k` to kill from preview)? Defining the line once is cheaper than re-litigating per key.

Surfaced from a user reporting that pressing Enter while previewing did not attach as they expected.
