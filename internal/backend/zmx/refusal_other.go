//go:build !linux

package zmx

// refusalProvesNoListener is false off Linux: macOS refuses a connect to a
// unix socket whose listen backlog is full, as it refuses one to a socket no
// process listens on.
const refusalProvesNoListener = false
