AGENT: standards
FINDINGS: none
COMMENT_CORRECTIONS:
- cmd/state_resume_wait.go:103-104 — Ctrl-Z stops the foreground process rather than killing it. The specification corrected this wording on 2026-09-28, and the comment still carries the old version.
  OLD: // answer the panel, and the three keys that would kill a foreground process are
// bytes like any other under raw mode. No signal is declined: tmux tearing the
  NEW: // answer the panel, and the three keys that would signal a foreground process are
// bytes like any other under raw mode. No signal is declined: tmux tearing the
- internal/capture/resume_surfaces.go:141-143 — makes a claim about tests ("joins every enumerating guard"), which goes false as soon as a guard is renamed or changes how it enumerates
  OLD: // the contrast swatch and the resume surfaces. Declared once so a surface added
// later joins every enumerating guard at this edit rather than fataling each of
// them on a name it cannot resolve.
  NEW: // the contrast swatch and the resume surfaces.
SUMMARY: This is a full pass over all 231 changed files against the whole specification and its corrigenda. The implementation conforms. That covers:
- both stored registration shapes, including byte-for-byte preservation of entries a write did not name;
- `hook set --resume-mode`, including the refusals, and the fifth column of `hook list`;
- the two informational lines in doctor;
- the tolerant `prefs.json` default, including an unreadable file;
- the helper resolving the pane's mode once and marking the pane pending before the mid-restore marker clears, on all three tails;
- the draw/wait/recover chain: the per-screen hand-off, the input drop after the appearance probe, the kill keys held as bytes across the whole chain, the recovery tail that tolerates version skew, and its shell backstop;
- Enter and discard clearing the marker only after leaving the panel's screen, and holding the answer when the clear fails;
- the capture freeze, with the frozen pane's record matched on its token and its scrollback re-filed under a token name;
- both panel screens and how they degrade on a small pane;
- the picker's second dot and the help-modal legend.

No divergence clears the floor. There are two comment corrections.
