package spawn

import (
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// ProcessWalker reads one hop of a process tree: given a pid it returns that
// process's parent pid and its executable command (a full path on macOS, e.g.
// "/Applications/Ghostty.app/Contents/MacOS/ghostty"). It is a 1-method DI seam
// so the walk resolution is unit-testable with fabricated ancestry.
type ProcessWalker interface {
	ProcessInfo(pid int) (ppid int, command string, err error)
}

// BundleReader reads a macOS `.app` bundle's identity: given the bundle
// directory (e.g. "/Applications/Ghostty.app") it returns the bundle's
// CFBundleIdentifier and a friendly display name. It is a 1-method DI seam so
// the walk resolution is unit-testable without touching `defaults`.
type BundleReader interface {
	Read(appPath string) (bundleID, name string, err error)
}

// ErrDetectTransient marks a transient terminal-detection failure — a `ps` or
// `defaults read` error encountered mid-walk. It is deliberately distinct from
// the clean NULL outcome (no host-local terminal), which is signalled by a NULL
// Identity and a nil error: callers branch on errors.Is(err, ErrDetectTransient)
// to emit a `spawn`-component WARN for a transient error while staying silent on
// a clean NULL. Transient errors always wrap the underlying `ps`/`defaults`
// cause, so that cause remains reachable through the chain.
var ErrDetectTransient = errors.New("terminal detection transient failure")

// maxWalkHops bounds the ancestry walk so a cyclic or pathologically long
// process tree cannot hang detection. Hitting the bound is a clean NULL, not an
// error. Real terminal ancestries are only a handful of hops deep; 32 is a
// generous ceiling.
const maxWalkHops = 32

// appBundleSuffix is the ".app" directory marker the walk searches for inside a
// process command path.
const appBundleSuffix = ".app"

// walkToBundle walks the process tree upward from startPID until it reaches a
// macOS `.app` bundle, then resolves that bundle's identity. It has a three-shape
// return contract:
//
//   - resolved Identity, nil error: an ancestor's command lives inside a `.app`
//     bundle whose Info.plist read succeeded.
//   - NULL Identity (Identity{}), nil error: the ancestry exhausted at ppid <= 1
//     (or a repeated pid, or the hop bound) without ever reaching a `.app` — the
//     honest "no host-local terminal" outcome (a remote/mosh client walks here).
//   - NULL Identity, ErrDetectTransient-wrapped error: a `ps` read failed on some
//     hop, or the `defaults` read failed on a found `.app`.
func walkToBundle(startPID int, walker ProcessWalker, reader BundleReader) (Identity, error) {
	seen := make(map[int]bool)
	pid := startPID

	for range maxWalkHops {
		if seen[pid] {
			// Repeated pid: a cycle. Exhausted with no .app -> clean NULL.
			return Identity{}, nil
		}
		seen[pid] = true

		ppid, command, err := walker.ProcessInfo(pid)
		if err != nil {
			return Identity{}, transient(fmt.Sprintf("read process info for pid %d", pid), err)
		}

		if appPath, ok := appBundlePath(command); ok {
			bundleID, name, rerr := reader.Read(appPath)
			if rerr != nil {
				return Identity{}, transient(fmt.Sprintf("read bundle info for %s", appPath), rerr)
			}
			return NewIdentity(bundleID, name), nil
		}

		if ppid <= 1 {
			// Reached the process-tree root with no .app -> clean NULL.
			return Identity{}, nil
		}
		pid = ppid
	}

	// Hit the hop bound on a runaway ancestry -> clean NULL, not an error.
	return Identity{}, nil
}

// transient tags cause as an ErrDetectTransient failure while preserving it in
// the chain, so callers can errors.Is either the sentinel or the underlying
// `ps`/`defaults` cause.
func transient(context string, cause error) error {
	return fmt.Errorf("%s: %w: %w", context, ErrDetectTransient, cause)
}

// appBundlePath reports whether command lives inside a macOS `.app` bundle and,
// if so, returns the bundle directory (the prefix up to and including `.app`).
// For "/Applications/Ghostty.app/Contents/MacOS/ghostty" it returns
// "/Applications/Ghostty.app". A command with no `.app/` segment is not inside a
// bundle.
func appBundlePath(command string) (string, bool) {
	marker := appBundleSuffix + "/"
	idx := strings.Index(command, marker)
	if idx < 0 {
		return "", false
	}
	return command[:idx+len(appBundleSuffix)], true
}

// realProcessWalker is the production ProcessWalker backed by `ps`. The real
// `ps` boundary is manual/integration only — no automated test executes it.
type realProcessWalker struct{}

var _ ProcessWalker = realProcessWalker{}

// ProcessInfo runs `ps -o ppid=,comm= -p <pid>` and parses the parent pid and
// command. On this Mac `ps -o comm=` returns the executable's full path (which
// may itself contain spaces), so only the first field is split off as the ppid.
func (realProcessWalker) ProcessInfo(pid int) (int, string, error) {
	cmd := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid))
	out, err := log.CombinedOutputWithContext(cmd)
	if err != nil {
		return 0, "", err
	}
	return parsePSProcessInfo(string(out))
}

// parsePSProcessInfo parses one `ps -o ppid=,comm=` line into (ppid, command).
// The ppid field is right-justified (leading spaces) and the command is a path
// that may contain spaces, so the line is split on its first whitespace run
// only: the first token is the ppid, the trimmed remainder is the command.
func parsePSProcessInfo(out string) (int, string, error) {
	line := strings.TrimSpace(out)
	if line == "" {
		return 0, "", errors.New("empty ps output")
	}

	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return 0, "", fmt.Errorf("malformed ps output %q", line)
	}

	ppid, err := strconv.Atoi(line[:idx])
	if err != nil {
		return 0, "", fmt.Errorf("parse ppid from %q: %w", line, err)
	}
	command := strings.TrimSpace(line[idx:])
	if command == "" {
		return 0, "", fmt.Errorf("empty command in ps output %q", line)
	}
	return ppid, command, nil
}

// realBundleReader is the production BundleReader backed by `defaults read`. The
// real `defaults` boundary is manual/integration only — no automated test
// executes it. This is the clean `lsappinfo`-free route the spec chose.
type realBundleReader struct{}

var _ BundleReader = realBundleReader{}

// Read reads a `.app` bundle's identity via `defaults read <app>/Contents/
// Info.plist`. CFBundleIdentifier is required (its failure is an error);
// CFBundleName is best-effort, falling back to the `.app` basename with `.app`
// stripped when absent.
func (realBundleReader) Read(appPath string) (string, string, error) {
	plist := path.Join(appPath, "Contents", "Info.plist")

	bundleID, err := readDefault(plist, "CFBundleIdentifier")
	if err != nil {
		return "", "", err
	}

	name, err := readDefault(plist, "CFBundleName")
	if err != nil || name == "" {
		name = appBasename(appPath)
	}
	return bundleID, name, nil
}

// readDefault runs `defaults read <plist> <key>` and returns the trimmed value.
func readDefault(plist, key string) (string, error) {
	cmd := exec.Command("defaults", "read", plist, key)
	out, err := log.CombinedOutputWithContext(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// appBasename derives a display name from a `.app` bundle path: the basename
// with the trailing `.app` stripped (e.g. "/Applications/Ghostty.app" ->
// "Ghostty").
func appBasename(appPath string) string {
	return strings.TrimSuffix(path.Base(appPath), appBundleSuffix)
}
