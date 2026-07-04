//go:build !integration

package state

// Production / unit build: the daemon-pgrep test sandbox DOES NOT EXIST.
//
// sandboxFilterPgrep is the identity function and the sandbox controls are
// no-ops, so PgrepPortalDaemons behaves in the shipped binary exactly as it
// always has — no test-only filtering logic can run in production because it is
// not compiled in. The real implementation lives in pgrep_sandbox.go under
// //go:build integration.
//
// EnableDaemonSandbox / RegisterSandboxDaemon / ResetDaemonSandbox are exported
// so that internal/portaltest (imported by BOTH unit and integration daemon
// tests) compiles in either build mode; here they do nothing.

func sandboxFilterPgrep(pids []int) []int { return pids }

func EnableDaemonSandbox()                           {}
func RegisterSandboxStateDir(string)                 {}
func RegisterSandboxDaemon(int)                      {}
func RegisterSandboxDaemonSource(func() (int, bool)) {}
func ResetDaemonSandbox()                            {}
