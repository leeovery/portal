// Package spawn detects the host terminal a Portal process runs under, resolves
// it to a window-spawning adapter, and drives that adapter to open new windows.
// Detection yields an Identity — a macOS bundle id plus display name, or the
// NULL identity when there is no host-local terminal (a remote/mosh client, a
// session whose leading terminal can't be told, or a transient failure).
package spawn
