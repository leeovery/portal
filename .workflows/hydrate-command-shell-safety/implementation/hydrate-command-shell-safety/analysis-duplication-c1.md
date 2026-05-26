AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:

- FINDING: Repeated collision-suffix length/charset assertion across new panekey sub-tests
  SEVERITY: low
  FILES: internal/state/panekey_test.go:117-119, internal/state/panekey_test.go:147-149, internal/state/panekey_test.go:161-163, internal/state/panekey_test.go:174-176
  DESCRIPTION: Four new sub-tests in TestSanitizePaneKey ("replaces whitespace bytes...", each "replaces shell-meta bytes..." case via tc loop, "replaces mixed whitespace and shell-meta...", and "collapses an all-non-allowlist input...") repeat the same three-step shape: `strings.TrimSuffix(got, "__0.0")`, `strings.HasPrefix(stem, "<expected>-")`, then `strings.TrimPrefix` + `len(hashPart) != 8`. The pre-existing collision sub-test (lines 56-79) additionally validates hashPart is lowercase hex; the four new copies drop that check, so the assertion has also diverged (weaker than the original). Three near-identical 4-6 line blocks inlined where one helper would centralise the invariant.
  RECOMMENDATION: Extract a test-local helper in panekey_test.go, e.g. `assertSanitizedStem(t, got, wantStem string, w, p int)` that strips `__<w>.<p>`, asserts `HasPrefix(stem, wantStem+"-")`, and asserts the trailing 8 chars are lowercase hex. Replace the four inline blocks with one-line calls. Restores the lowercase-hex assertion to every collision-bearing case and removes ~20 lines of repeated structure.

SUMMARY: One low-severity duplication on the test side — four new panekey_test.go sub-tests inline the same TrimSuffix/HasPrefix/8-char-len triple. Extractable into a single helper that would also restore the lowercase-hex assertion the inline copies dropped. No production-code duplication; `shellQuoteSingle` is the sole shell-quoting helper in the repo and `sanitizeSessionName`/`isAllowedByte` have no near-duplicates elsewhere.
