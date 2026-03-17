---
topic: portal
cycle: 4
total_proposed: 1
---
# Analysis Tasks: Portal (Cycle 4)

## Task 1: Align GoReleaser config and release workflow with tick reference pattern
status: approved
severity: high
sources: standards

**Problem**: The `.goreleaser.yaml` contains a `homebrew_casks` section which is semantically wrong (portal is a CLI binary, not a macOS GUI .app bundle) and diverges from the tick reference project that shares the same `leeovery/homebrew-tools` tap. The archives use `format_overrides` with zip for darwin instead of the simpler `formats: [tar.gz]`. The `files: [none*]` exclusion is missing. The changelog is set to `sort: asc` instead of `disable: true`. The release workflow passes `HOMEBREW_TAP_GITHUB_TOKEN` and relies on GoReleaser's Homebrew integration instead of the correct pattern: extract checksums from GoReleaser output and dispatch to the homebrew-tools repo via GitHub API.

**Solution**: Rewrite `.goreleaser.yaml` and `.github/workflows/release.yml` to match the tick reference pattern exactly (adapted for portal).

**Outcome**: Portal's release pipeline matches the established pattern used by tick. GoReleaser produces tar.gz archives only. Homebrew formula updates are triggered via repository dispatch to `leeovery/homebrew-tools` with checksums, using the `CICD_PAT` secret.

**Do**:

1. Replace `.goreleaser.yaml` with:
   ```yaml
   version: 2

   project_name: portal

   builds:
     - id: portal
       main: .
       binary: portal
       env:
         - CGO_ENABLED=0
       goos:
         - darwin
         - linux
       goarch:
         - amd64
         - arm64
       ldflags:
         - -s -w -X github.com/leeovery/portal/cmd.version={{.Version}}

   archives:
     - id: portal
       formats:
         - tar.gz
       name_template: "portal_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
       files:
         - none*

   changelog:
     disable: true

   release:
     draft: false
   ```

2. Replace `.github/workflows/release.yml` with:
   ```yaml
   name: Release

   on:
     push:
       tags:
         - 'v[0-9]*.[0-9]*.[0-9]*'
         - '!v*-*'

   permissions:
     contents: write

   jobs:
     release:
       runs-on: ubuntu-latest
       steps:
         - name: Checkout
           uses: actions/checkout@v4
           with:
             fetch-depth: '0'

         - name: Setup Go
           uses: actions/setup-go@v5
           with:
             go-version-file: go.mod

         - name: Run GoReleaser
           uses: goreleaser/goreleaser-action@v6
           with:
             args: release --clean
           env:
             GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

         - name: Extract checksums
           id: checksums
           run: |
             VERSION="${GITHUB_REF_NAME#v}"
             CHECKSUMS_FILE="dist/portal_${VERSION}_checksums.txt"
             SHA256_ARM64=$(grep "portal_${VERSION}_darwin_arm64.tar.gz" "$CHECKSUMS_FILE" | awk '{print $1}')
             SHA256_AMD64=$(grep "portal_${VERSION}_darwin_amd64.tar.gz" "$CHECKSUMS_FILE" | awk '{print $1}')
             echo "version=$VERSION" >> "$GITHUB_OUTPUT"
             echo "sha256_arm64=$SHA256_ARM64" >> "$GITHUB_OUTPUT"
             echo "sha256_amd64=$SHA256_AMD64" >> "$GITHUB_OUTPUT"

         - name: Dispatch to homebrew-tools
           run: |
             curl -fsSL -X POST \
               -H "Accept: application/vnd.github.v3+json" \
               -H "Authorization: token ${{ secrets.CICD_PAT }}" \
               https://api.github.com/repos/leeovery/homebrew-tools/dispatches \
               -d '{
                 "event_type": "update-formula",
                 "client_payload": {
                   "tool": "portal",
                   "version": "${{ steps.checksums.outputs.version }}",
                   "sha256_arm64": "${{ steps.checksums.outputs.sha256_arm64 }}",
                   "sha256_amd64": "${{ steps.checksums.outputs.sha256_amd64 }}"
                 }
               }'
   ```

3. Verify no other files reference `HOMEBREW_TAP_TOKEN` or `homebrew_casks` -- if found, remove those references.

**Acceptance Criteria**:
- `.goreleaser.yaml` has no `homebrew_casks` section
- `.goreleaser.yaml` archives use `formats: [tar.gz]` with no `format_overrides`
- `.goreleaser.yaml` archives include `files: [none*]`
- `.goreleaser.yaml` has `changelog: disable: true`
- `.goreleaser.yaml` has `release: draft: false` (no explicit github owner/name)
- `.github/workflows/release.yml` has no `HOMEBREW_TAP_GITHUB_TOKEN` env var
- `.github/workflows/release.yml` extracts checksums from `dist/portal_*_checksums.txt`
- `.github/workflows/release.yml` dispatches to `leeovery/homebrew-tools` with tool, version, sha256_arm64, sha256_amd64
- `.github/workflows/release.yml` uses `CICD_PAT` secret for the dispatch step
- `.github/workflows/release.yml` tag pattern is `v[0-9]*.[0-9]*.[0-9]*` with `!v*-*` exclusion
- `goreleaser check` passes (run locally or verify YAML is valid)

**Tests**:
- Run `goreleaser check` against the new `.goreleaser.yaml` to validate syntax
- Confirm no references to `HOMEBREW_TAP_TOKEN` or `homebrew_casks` remain in the repo
