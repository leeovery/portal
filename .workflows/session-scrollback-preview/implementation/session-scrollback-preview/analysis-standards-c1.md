AGENT: standards
FINDINGS:

- FINDING: Home/End intercepted at preview level rather than passed to viewport — defensible bridge but worth pinning to spec
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:253-258
  DESCRIPTION: The acceptance criteria (spec § Acceptance Criteria > Within-preview navigation) lists `Home`/`End` among "the viewport's native scroll keys" that should pass through. However, `bubbles/viewport@v1.0.0`'s `DefaultKeyMap` (`bubbles@v1.0.0/viewport/keymap.go`) does NOT bind `Home` or `End` at all — only `PageDown`/`PageUp`/`HalfPage*`/`Up`/`Down`/`Left`/`Right`. A literal "passthrough" would therefore silently no-op those keys, contradicting the user-visible acceptance criterion. The implementation correctly compensates by intercepting `tea.KeyHome`/`tea.KeyEnd` in `Update` and calling `viewport.GotoTop()`/`GotoBottom()`. This is the right outcome but introduces a subtle drift from § Within-preview Key Bindings > Keymap policy ("Preview owns `]` `[` `Tab` `Esc`. Everything else either passes through to the embedded `bubbles/viewport` ... or is unbound/no-op."). `Home`/`End` are now a fifth/sixth preview-owned key, not pure passthroughs.
  RECOMMENDATION: No code change required — behaviour matches the acceptance criteria, which take precedence. Extend the existing in-code comment with one line noting that viewport's `DefaultKeyMap` does not bind Home/End so preview must own them to satisfy the acceptance criterion.

- FINDING: previewSessionsRefreshedMsg.Err is silently swallowed on the post-dismiss re-fetch
  SEVERITY: low
  FILES: internal/tui/model.go:886-903
  DESCRIPTION: Spec § Cross-cutting Seams > Externally-Killed Session During Preview mandates a re-fetch on `Esc`-from-preview so a killed session does not linger in the post-dismiss view. The implementation correctly performs the re-fetch, but on lister error it silently drops `msg.Err` and keeps the pre-refresh snapshot intact. The spec is silent on the error path; this means a session killed externally between Space and Esc could remain visible if the post-dismiss ListSessions call fails. Build-phase decision filling a spec gap, well-documented inline.
  RECOMMENDATION: No change required — the chosen behaviour is reasonable. If desired, surface a transient error indicator at a future point; not required for v1.

- FINDING: Chrome layout pinned to header — spec leaves placement open, implementation pins it without an accurate rationale anchor
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:300-306, internal/tui/pagepreview.go:12-16
  DESCRIPTION: Spec § Multi-pane Rendering Shape > Chrome Floor lists "Exact chrome wording, header vs footer, single-line vs two-line" as an open item. § Interaction Shape > Layout says "chrome on a single line (header **or** footer; final placement is a build-phase decision per *Open Items*)". The implementation pins header-on-top in `View()`, which is a legitimate build-phase choice. However, the doc-comment on `View()` claims "The orientation (header on top) is fixed in v1 per § Interaction Shape > Layout, and pinned by tests" — but that section explicitly defers the choice rather than fixing it.
  RECOMMENDATION: Tighten the `View()` doc-comment to reflect the spec's actual wording — header was a build-phase choice, not a fixed spec choice. E.g. "Header-on-top is the build-phase choice (spec § Open Items defers placement); only `previewChromeHeight` and this orientation change if footer is later preferred."

- FINDING: Chrome embeds tmux's internal `#W` format code as user-facing label
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:165-168
  DESCRIPTION: Spec § Multi-pane Rendering Shape > Chrome Floor mandates "Window name — tmux's `#W` / window name" as part of the chrome floor; the `#W` reference is the spec's way of identifying which tmux format code supplies the value, not user-facing text. The implementation's chrome line embeds the literal string `#W:` verbatim: `"Window %d of %d · Pane %d of %d · #W: %s    ] [ Tab Esc"`. Users unfamiliar with tmux format codes will read `#W:` as a label that needs decoding. Spec's "Exact chrome wording" is an open item, so this is a build-phase choice rather than a spec violation — but the `#W:` prefix appears to be a literal copy of the spec's internal source-code reference rather than a deliberate UX decision.
  RECOMMENDATION: Drop the `#W:` prefix or replace with a user-facing label. E.g. `"Window %d of %d · Pane %d of %d · %s    ] [ Tab Esc"` (no label — name speaks for itself given the preceding counter).

STATUS: findings
