AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:

- FINDING: Repeated got/want assertion block across buildHydrateCommand sub-tests
  SEVERITY: low
  FILES: internal/restore/session_build_hydrate_test.go:17-19, 25-27, 35-39, 52-54, 64-66, 75-77, 86-88
  DESCRIPTION: Six sub-tests in TestBuildHydrateCommand each repeat the same three-line `if got != want { t.Errorf("buildHydrateCommand <label>:\n got %q\nwant %q", got, want) }` pattern, varying only the leading label string. The body is mechanical; the only signal per site is the label and the (got, want) pair. Mild test-helper extraction territory — a small `assertHydrateCmd(t, label, got, want)` would collapse ~18 lines to ~6. Not a correctness risk and within the Go convention of explicit per-case assertions, hence low severity.
  RECOMMENDATION: Optional. If the file grows further sub-tests, introduce a local helper such as `func assertHydrateCmd(t *testing.T, label, got, want string)` calling `t.Helper()` and the got/want comparison. Otherwise leave as-is — six call sites is borderline.

SUMMARY: One low-severity finding — a small repeated assertion block across six sub-tests in session_build_hydrate_test.go that could optionally be collapsed into a local helper. No production-code duplication; shellQuoteSingle, sanitizeSessionName, and isAllowedByte each have a single home, and the cycle-1 collision-suffix assertion repetition is fully resolved via assertSanitizedStem.
