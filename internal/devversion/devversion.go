// Package devversion is the version of a dev build: one with no release
// version injected. Each rebuilt binary reports its own, so the bootstrap
// latch and the saver's version check see a rebuild as they see an upgrade.
package devversion

import (
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Unidentified is the version of a build that says nothing of itself: one
// with no build info and no executable to stat. It names no one build, so
// nothing may take two binaries reporting it for one.
const Unidentified = "dev"

const (
	revisionWidth = 12
	writtenLayout = "20060102T150405.000000000Z"
)

// String is the running binary's version, worked out once. A build of a
// module at a release, as go install module@version makes, has no VCS stamp
// and reports that release as an injected version reads. Any other is a dev
// version: Unidentified, then the VCS revision it was built from and
// "modified" when that tree had changes, then when its executable file was
// written, each part left out when the build or the file doesn't say. The
// revision and modified bit are the same for every rebuild of an edited tree;
// every rebuild rewrites the executable, so its write time tells them apart
// without hashing the binary.
func String() string { return current() }

var current = sync.OnceValue(func() string {
	info, _ := debug.ReadBuildInfo()
	return compose(info, executableWritten())
})

func compose(info *debug.BuildInfo, written time.Time) string {
	if release := moduleRelease(info); release != "" {
		return release
	}
	parts := []string{Unidentified}
	if rev := setting(info, "vcs.revision"); rev != "" {
		parts = append(parts, rev[:min(len(rev), revisionWidth)])
		if setting(info, "vcs.modified") == "true" {
			parts = append(parts, "modified")
		}
	}
	if !written.IsZero() {
		parts = append(parts, written.UTC().Format(writtenLayout))
	}
	return strings.Join(parts, "-")
}

// moduleRelease is the module version a build without a VCS stamp was made
// at, less its "v", and empty for any other build. Since Go 1.24 a build in a
// checkout carries a module version too, worked out from the checkout, so
// only one with no VCS stamp, built from the module's own download, names a
// release.
func moduleRelease(info *debug.BuildInfo) string {
	if info == nil || setting(info, "vcs.revision") != "" {
		return ""
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return strings.TrimPrefix(v, "v")
	}
	return ""
}

func setting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// executableWritten stats the file at the executable's path, which a rebuild
// replaces under a running process. It is read once, at the first ask for the
// version, which main makes before anything else, so the file is all but
// surely the one the process started from.
func executableWritten() time.Time {
	exe, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
