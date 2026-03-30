AGENT: standards
FINDINGS: none
SUMMARY: Implementation conforms to specification and project conventions. The os.Stat(newPath) guard clause correctly uses os.IsNotExist(err) to gate migration, non-not-found errors cause early return as specified, the os.Stat(oldPath) check is unchanged, and configFilePath/xdgConfigBase are untouched. The new test covers the non-not-found error path. No t.Parallel() usage, consistent with project rules.
