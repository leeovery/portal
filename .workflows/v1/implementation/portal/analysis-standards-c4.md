AGENT: standards
FINDINGS:
- FINDING: GoReleaser config contains incorrect homebrew_casks section and wrong archive format
  SEVERITY: high
  FILES: /Users/leeovery/Code/portal/.goreleaser.yaml:34-44, /Users/leeovery/Code/portal/.goreleaser.yaml:19-24
  DESCRIPTION: The GoReleaser config diverges from the established project pattern used by tick (the reference project sharing the same leeovery/homebrew-tools tap). Three issues:
    1. **homebrew_casks section should not exist** (lines 34-44). The reference tick project has no homebrew_casks section. Homebrew formula management is handled separately. The homebrew_casks section is also semantically wrong -- portal is a CLI binary, not a macOS GUI app (casks are for .app bundles). GoReleaser's correct section for CLI formulas would be `brews`, but the reference project uses neither, indicating formulas are managed outside GoReleaser.
    2. **Archives use format_overrides with zip for darwin** (lines 19-23) instead of the simpler `formats: [tar.gz]` used by tick. The format_overrides approach produces zip files for macOS and tar.gz for Linux, which is inconsistent and diverges from the reference.
    3. **Missing `files: [none*]`** in archives. The reference includes `files: [none*]` to exclude extra files from archives. Portal omits this, which means GoReleaser may bundle default files (README, LICENSE, CHANGELOG) into the archive.
  RECOMMENDATION: Replace the archives and remove homebrew_casks to match the tick reference pattern:
    ```yaml
    archives:
      - id: portal
        formats:
          - tar.gz
        name_template: "portal_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
        files:
          - none*
    ```
    Remove the entire `homebrew_casks:` block (lines 34-44). Also consider adding `changelog: disable: true` and simplifying the release section to `release: draft: false` to match the reference, though these are lower priority.

- FINDING: GoReleaser changelog config diverges from reference pattern
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/.goreleaser.yaml:26-27
  DESCRIPTION: Portal uses `changelog: sort: asc` while the reference tick project uses `changelog: disable: true`. This is a minor inconsistency with the established project pattern. With changelogs enabled, GoReleaser auto-generates a changelog from git commits in the GitHub release, which may not be desired given the reference explicitly disables it.
  RECOMMENDATION: Change to `changelog: disable: true` to match the reference project convention.

SUMMARY: One high-severity finding: the GoReleaser config contains a homebrew_casks section (which is for macOS GUI apps, not CLI binaries), uses zip format overrides instead of the simpler tar.gz-only approach, and omits the files exclusion -- all diverging from the established reference pattern in the tick project that shares the same Homebrew tap. One low-severity finding on changelog config divergence.
