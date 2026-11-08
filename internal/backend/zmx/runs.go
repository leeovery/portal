package zmx

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// FirstRelease is the first zmx release whose daemons speak ProtocolVersion.
// Portal supports zmx's releases alone: a build of zmx's main from between
// the protocol's freeze and this release's version bump speaks
// ProtocolVersion but prints an older release, and is refused with it.
const FirstRelease = "1.0.0"

// firstRelease is FirstRelease as releaseOf reads one.
var firstRelease, _ = releaseOf(FirstRelease)

// OlderReleaseError is a zmx whose version line reads a release older than
// FirstRelease: a release whose daemons speak no protocol Portal does, or a
// build of zmx's main that prints one, whatever protocol that build speaks.
type OlderReleaseError struct {
	// Release is the release the version line reads.
	Release string
}

func (e *OlderReleaseError) Error() string {
	return fmt.Sprintf("zmx %s is older than %s, the first release that speaks protocol %d", e.Release, FirstRelease, ProtocolVersion)
}

// Runs reports whether the Backend's zmx binary runs: zmx version exits 0,
// bound on the Backend's directory, within the ceiling Create's run of the
// binary has. A failure carries what zmx printed. A version line that reads a
// release older than FirstRelease is an *OlderReleaseError, refused before
// that zmx makes anything; any other passes, a version line Runs can't read
// included, and the protocol its daemons speak judges it.
func (b *Backend) Runs(ctx context.Context) error {
	ctx, cancel := b.bound(ctx, b.limits.create, "")
	defer cancel()
	cmd := exec.CommandContext(ctx, b.binary, "version")
	cmd.Env = b.env(envSession, envPrefix)
	cmd.WaitDelay = pipeGrace
	out, err := cmd.CombinedOutput()
	switch {
	case err == nil:
		return refuseOlderRelease(string(out))
	case ctx.Err() != nil:
		return fmt.Errorf("zmx version: %w", context.Cause(ctx))
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("zmx version: %s: %w", msg, err)
	}
	return fmt.Errorf("zmx version: %w", err)
}

// refuseOlderRelease refuses the release zmx's version line, the first line
// of out, names after "zmx", when it reads older than FirstRelease. It judges
// the number alone, never the protocol the build speaks.
func refuseOlderRelease(out string) error {
	line, _, _ := strings.Cut(out, "\n")
	fields := strings.Fields(line)
	if len(fields) != 2 || fields[0] != "zmx" {
		return nil
	}
	if release, ok := releaseOf(fields[1]); ok && slices.Compare(release, firstRelease) < 0 {
		return &OlderReleaseError{Release: fields[1]}
	}
	return nil
}

// releaseOf reads s as major.minor.patch, disregarding a pre-release or build
// suffix: the release's three numbers, or false for anything else.
func releaseOf(s string) ([]int, bool) {
	core, _, _ := strings.Cut(s, "+")
	core, _, _ = strings.Cut(core, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return nil, false
	}
	numbers := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || part != strconv.Itoa(n) {
			return nil, false
		}
		numbers[i] = n
	}
	return numbers, true
}
