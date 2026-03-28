# Add Unit Test for configFilePath os.UserHomeDir Failure Path

The `configFilePath` function in `cmd/config.go` has an error path around line 48-49 that handles the case where `os.UserHomeDir()` fails. This path is currently untested. The code is trivially correct — a standard two-line `if err != nil { return "", fmt.Errorf(...) }` pattern — but it's the only untested branch in the function.

This came up during review of the config-dir-wrong-path-macos bugfix. The plan acknowledged it as infeasible without mocking `os.UserHomeDir`, since the stdlib function reads from the environment and OS state with no injection point. The existing test pattern in `cmd/config_test.go` uses `t.Setenv` for env var manipulation but has no mechanism to force `os.UserHomeDir` to fail.

One approach would be extracting a `homeDirFunc` dependency that `configFilePath` calls instead of `os.UserHomeDir` directly. This follows the same DI pattern the project already uses for external dependencies (small interfaces, package-level dep structs). The test could then inject a failing implementation. Whether the coverage gain justifies the added indirection is a judgment call — the code is trivially correct and the failure mode (no HOME set, broken OS state) is exotic.

Located in `cmd/config.go`, the `configFilePath` function, specifically the `os.UserHomeDir()` call and its error check.
