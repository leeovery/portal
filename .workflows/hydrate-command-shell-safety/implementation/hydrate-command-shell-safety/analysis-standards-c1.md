AGENT: standards
STATUS: clean
FINDINGS_COUNT: 0

FINDINGS: none

SUMMARY: Implementation conforms to specification and project conventions.

- Atom 1 (shell quoting) — internal/restore/session.go: shellQuoteSingle restored using canonical close-escape-reopen idiom; applied to fifo/file/hookKey; docstring rewritten; no sh -c envelope reintroduced.
- Atom 2 (allowlist sanitisation) — internal/state/panekey.go: sanitizeSessionName rewritten with isAllowedByte covering exactly [A-Za-z0-9._-]; leading-`.` step preserved; isUnsafeByte fully removed; SanitizePaneKey docstring step 1 updated; collisionSuffix logic untouched.
- Test coverage: hydrate tests pin typical/empty/whitespace/embedded-quote/shell-meta inputs plus negative regression guard against sh -c. Panekey tests retain the original five and add whitespace, per-byte shell-meta, mixed, and all-non-allowlist coverage.
- Exclusions respected: no changes to cmd/state_hydrate.go, daemon, bootstrap, TUI, schema, FIFO format, marker format, or per-caller code. hookKey remains preserved-raw.
- Go convention compliance: godoc on exports, errors propagated with %w, table-driven sub-tests via t.Run, no t.Parallel() introduced.
