AGENT: standards
FINDINGS:
- FINDING: GoReleaser config uses homebrew_casks instead of brews (carried from c1)
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/.goreleaser.yaml:34
  DESCRIPTION: The spec says "Distributed via Homebrew tap" with `brew tap leeovery/tools && brew install portal`. GoReleaser v2 uses the `brews` key for generating Homebrew formulae for CLI tools. The config uses `homebrew_casks`, which is for macOS .app cask definitions, not CLI formulae. This means the release pipeline will not generate or push a Homebrew formula to the leeovery/homebrew-tools tap.
  RECOMMENDATION: Change `homebrew_casks` to `brews` in .goreleaser.yaml and adjust the inner structure to match the GoReleaser v2 brews schema.

- FINDING: Config directory resolves to wrong location on macOS
  SEVERITY: high
  FILES: /Users/leeovery/Code/portal/cmd/config.go:17
  DESCRIPTION: The spec states "All Portal data is stored in ~/.config/portal/" for projects.json, aliases, and config files. The implementation uses os.UserConfigDir() which on macOS (Darwin) returns ~/Library/Application Support, not ~/.config. This means on macOS the actual storage path will be ~/Library/Application Support/portal/projects.json instead of the spec's ~/.config/portal/projects.json. Since Portal's primary target is macOS (the use case is SSH from phone to Mac), this is a meaningful divergence from the spec's documented behavior.
  RECOMMENDATION: Replace os.UserConfigDir() with a direct path construction using os.UserHomeDir() + "/.config/portal/" to match the spec's explicit ~/.config/portal/ location, or use os.Getenv("XDG_CONFIG_HOME") with fallback to ~/.config as the XDG spec prescribes on all platforms.

- FINDING: TUI outside-tmux session creation uses attach-session instead of new-session -A
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/open.go:57-63, /Users/leeovery/Code/portal/internal/tui/model.go:281-288
  DESCRIPTION: The spec says outside-tmux session attachment uses "exec tmux new-session -A -s <name>" for atomic create-or-attach. The openPath CLI path correctly uses QuickStart which builds exec args with -A. However, the TUI path (openTUI) creates sessions detached via SessionCreator.CreateFromDir (which calls tmux new-session -d), then exec's "tmux attach-session -t <name>" via AttachConnector. This is functionally equivalent since the session was just created, but uses a different command sequence than specified. The QuickStart -A pattern from openPath is not used in the TUI flow.
  RECOMMENDATION: Acceptable as functionally equivalent. The detached-then-attach pattern works correctly. If strict spec alignment is desired, the TUI outside-tmux flow could be refactored to use QuickStart's exec args instead of the attach-session path.

SUMMARY: The most impactful finding is that config file storage uses os.UserConfigDir() which on macOS resolves to ~/Library/Application Support rather than the spec's documented ~/.config/portal/ location. The GoReleaser homebrew_casks issue from cycle 1 remains unfixed. The cycle 1 high-severity finding (missing TUI dependencies) and the package-level parsedCommand variable have been properly resolved.
