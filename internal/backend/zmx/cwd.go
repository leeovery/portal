package zmx

import (
	"net/url"
	"strings"
)

// localDir reads a session's cwd as zmx reports it, as a directory on this
// machine, or "" when it names none here. zmx keeps the cwd as
// file://<host><percent-encoded path>, as a raw OSC 7 the shell sent (which
// may be kitty-shell-cwd://), or as a plain path. OSC 7 crosses SSH, so a
// cwd on another host names no directory here.
func localDir(cwd, hostname string) string {
	if strings.HasPrefix(cwd, "/") {
		return cwd
	}
	u, err := url.Parse(cwd)
	if err != nil || (u.Scheme != "file" && u.Scheme != "kitty-shell-cwd") || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	if !isLocalHost(u.Hostname(), hostname) {
		return ""
	}
	return u.Path
}

// isLocalHost is zmx's rule: no host, localhost, this machine's name, or the
// same first DNS label, since gethostname may give the short name where OSC 7
// carries the full one, or the reverse.
func isLocalHost(host, hostname string) bool {
	if host == "" || strings.EqualFold(host, "localhost") || strings.EqualFold(host, hostname) {
		return true
	}
	label, _, _ := strings.Cut(hostname, ".")
	hostLabel, _, _ := strings.Cut(host, ".")
	return label != "" && strings.EqualFold(hostLabel, label)
}
