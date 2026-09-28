//go:build darwin

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// shellModes are the modes a shell reading the terminal needs and a terminal
// left raw lacks.
var shellModes = struct{ lflag, iflag, oflag uint64 }{
	lflag: unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ISIG | unix.IEXTEN,
	iflag: unix.ICRNL | unix.IXON,
	oflag: unix.OPOST | unix.ONLCR,
}

// rawPTY is a pty slave left raw, as a waiter that never ran its restore leaves
// the pane's terminal.
func rawPTY(t *testing.T) (master, slave *os.File, fd int) {
	t.Helper()
	master, slave = openPTY(t)
	fd = descriptorOf(t, slave)
	if _, err := term.MakeRaw(uintptr(fd)); err != nil {
		t.Fatalf("make the pty raw: %v", err)
	}
	return master, slave, fd
}

func assertShellModes(t *testing.T, modes *unix.Termios) {
	t.Helper()
	if missing := shellModes.lflag &^ modes.Lflag; missing != 0 {
		t.Errorf("local modes missing %#x", missing)
	}
	if missing := shellModes.iflag &^ modes.Iflag; missing != 0 {
		t.Errorf("input modes missing %#x", missing)
	}
	if missing := shellModes.oflag &^ modes.Oflag; missing != 0 {
		t.Errorf("output modes missing %#x", missing)
	}
}

// withStdin points os.Stdin at f for the test, so a stdin-form seam acts on it.
func withStdin(t *testing.T, f *os.File) {
	t.Helper()
	prior := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = prior })
}

func TestCookTTY_RealPTY(t *testing.T) {
	t.Run("a terminal left raw gets every mode a shell needs back", func(t *testing.T) {
		_, _, fd := rawPTY(t)

		if err := cookTTY(fd); err != nil {
			t.Fatalf("cookTTY() error = %v", err)
		}

		assertShellModes(t, localModes(t, fd))
	})

	t.Run("typed input echoes once the terminal is cooked", func(t *testing.T) {
		master, _, fd := rawPTY(t)
		if err := cookTTY(fd); err != nil {
			t.Fatalf("cookTTY() error = %v", err)
		}

		if _, err := master.Write([]byte("ls\r")); err != nil {
			t.Fatalf("type into the master: %v", err)
		}

		if got := readRows(t, master, 1); !bytes.Contains(got, []byte("ls\r\n")) {
			t.Errorf("the terminal echoed %q, want the typed line back with its line start", got)
		}
	})
}

func TestResumeHandOffs_TerminalModes_RealPTY(t *testing.T) {
	t.Run("the recovery tail hands the user's shell a cooked terminal", func(t *testing.T) {
		_, slave, fd := rawPTY(t)
		var got resumeRecoverConfig
		withFuncSeam(t, &resumeRecoverRunFunc, func(cfg resumeRecoverConfig) error {
			got = cfg
			return nil
		})
		executeResumeCommand(t, resumeRecoverSubcommand)
		withStdin(t, slave)

		if err := got.EnableTTYSignals(); err != nil {
			t.Fatalf("EnableTTYSignals() error = %v", err)
		}

		assertShellModes(t, localModes(t, fd))
	})

	t.Run("an answered pane has signal generation alone turned back on", func(t *testing.T) {
		_, slave, fd := rawPTY(t)
		var got resumeWaitConfig
		withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
			got = cfg
			return nil
		})
		executeResumeCommand(t, resumeWaitSubcommand)
		withStdin(t, slave)
		before := *localModes(t, fd)

		if err := got.EnableTTYSignals(); err != nil {
			t.Fatalf("EnableTTYSignals() error = %v", err)
		}

		after := localModes(t, fd)
		if after.Lflag != before.Lflag|unix.ISIG {
			t.Errorf("local modes = %#x, want %#x", after.Lflag, before.Lflag|unix.ISIG)
		}
		if after.Iflag != before.Iflag || after.Oflag != before.Oflag {
			t.Errorf("input/output modes = %#x/%#x, want untouched %#x/%#x", after.Iflag, after.Oflag, before.Iflag, before.Oflag)
		}
	})
}

func executeResumeCommand(t *testing.T, subcommand string) {
	t.Helper()
	resetRootCmd()
	rootCmd.SetOut(new(bytes.Buffer))
	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(resumeChainArgv("portal", subcommand, samplePayload())[1:])
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("executing %s: %v\nstderr: %s", subcommand, err, errBuf)
	}
}
