package zmxtest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// OlderBinaryEnv names a build of zmx 0.8.1, the release before the protocol
// Portal speaks, for the tests of how Portal treats one.
const OlderBinaryEnv = "PORTAL_TEST_ZMX_OLDER_BINARY"

// homebrewZmx is where Homebrew installs zmx, whose tap shipped 0.8.1.
const homebrewZmx = "/opt/homebrew/bin/zmx"

// olderVersion is the release OlderBinary finds.
const olderVersion = "0.8.1"

// errNoOlder is no zmx 0.8.1 found where a test may do without one.
var errNoOlder = errors.New("no zmx " + olderVersion)

// OlderBinary is the path of a zmx 0.8.1 build: the one OlderBinaryEnv names,
// or else Homebrew's zmx when it reports 0.8.1. It skips the test when
// neither is one, as on the weekly zmx check, which tests zmx's newest builds
// only. It fails the test when the build OlderBinaryEnv names isn't one,
// outside the integration lane, when ZMX_DIR or TMUX_TMPDIR is not poisoned,
// and when the weekly zmx check doesn't run the test.
func OlderBinary(t *testing.T) string {
	t.Helper()
	if !integrationLane {
		t.Fatal("zmxtest: a real zmx runs only in the integration lane, the one the weekly zmx check runs: tag the test's file //go:build integration")
	}
	if err := checkPoisoned(os.Getenv); err != nil {
		t.Fatal(err)
	}
	requireWeeklyCheck(t)
	binary, err := findOlder(os.Getenv, homebrewZmx, t.TempDir())
	switch {
	case errors.Is(err, errNoOlder):
		t.Skip(err)
	case err != nil:
		t.Fatal(err)
	}
	return binary
}

// findOlder is OlderBinary's zmx 0.8.1 in the environment getenv reads, with
// homebrew the path Homebrew's zmx would have, each checked with dir as its
// directory and home. Finding none fails with errNoOlder.
func findOlder(getenv func(string) string, homebrew, dir string) (string, error) {
	if binary := getenv(OlderBinaryEnv); binary != "" {
		if err := olderBuild(binary, dir); err != nil {
			return "", fmt.Errorf("%s=%q: %w", OlderBinaryEnv, binary, err)
		}
		return binary, nil
	}
	if err := olderBuild(homebrew, dir); err != nil {
		return "", fmt.Errorf("%w: %s is unset, and Homebrew's zmx is none: %w", errNoOlder, OlderBinaryEnv, err)
	}
	return homebrew, nil
}

// olderBuild checks that binary, an absolute path to a file named zmx,
// reports itself zmx 0.8.1, run with dir as its directory and home.
func olderBuild(binary, dir string) error {
	if !filepath.IsAbs(binary) || filepath.Base(binary) != "zmx" {
		return fmt.Errorf("want an absolute path to a file named zmx")
	}
	cmd := exec.Command(binary, "version")
	cmd.Env = []string{"ZMX_DIR=" + dir, "HOME=" + dir, "PATH=/usr/bin:/bin"}
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("zmx version: %w", err)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	if fields := strings.Fields(first); len(fields) != 2 || fields[0] != "zmx" || fields[1] != olderVersion {
		return fmt.Errorf("it reports %q, not zmx %s", first, olderVersion)
	}
	return nil
}
