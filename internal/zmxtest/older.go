package zmxtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// OlderBinaryEnv names a build of zmx 0.8.1, the release before the protocol
// Portal speaks, for the tests of how Portal treats one. The weekly zmx check
// builds it and names it here.
const OlderBinaryEnv = "PORTAL_TEST_ZMX_OLDER_BINARY"

// homebrewZmx is where Homebrew installs zmx, whose tap shipped 0.8.1.
const homebrewZmx = "/opt/homebrew/bin/zmx"

// olderVersion is the release OlderBinary finds.
const olderVersion = "0.8.1"

// OlderBinary is the path of a zmx 0.8.1 build: the one OlderBinaryEnv names,
// or else Homebrew's zmx when it reports 0.8.1. It skips the test when
// neither is one, and fails it when the build OlderBinaryEnv names isn't, as
// it does outside the integration lane and when the weekly zmx check doesn't
// run the test.
func OlderBinary(t *testing.T) string {
	t.Helper()
	if !integrationLane {
		t.Fatal("zmxtest: a real zmx runs only in the integration lane, the one the weekly zmx check runs: tag the test's file //go:build integration")
	}
	requireWeeklyCheck(t)
	if binary := os.Getenv(OlderBinaryEnv); binary != "" {
		if err := olderBuild(binary, t.TempDir()); err != nil {
			t.Fatalf("%s=%q: %v", OlderBinaryEnv, binary, err)
		}
		return binary
	}
	if err := olderBuild(homebrewZmx, t.TempDir()); err != nil {
		t.Skipf("no zmx %s: %s is unset, and Homebrew's zmx is none: %v", olderVersion, OlderBinaryEnv, err)
	}
	return homebrewZmx
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
