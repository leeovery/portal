//go:build darwin

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

	t.Run("a pane about to wait has signal generation alone turned off", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		var got hydrateConfig
		withFuncSeam(t, &hydrateRunFunc, func(cfg hydrateConfig) error {
			got = cfg
			return nil
		})
		executeHydrateCommand(t)
		withStdin(t, slave)
		before := *localModes(t, fd)
		if before.Lflag&unix.ISIG == 0 {
			t.Fatalf("local modes = %#x, want signal generation on before the hand-off", before.Lflag)
		}

		if err := got.DisableTTYSignals(); err != nil {
			t.Fatalf("DisableTTYSignals() error = %v", err)
		}

		after := localModes(t, fd)
		if after.Lflag != before.Lflag&^unix.ISIG {
			t.Errorf("local modes = %#x, want %#x", after.Lflag, before.Lflag&^unix.ISIG)
		}
		if after.Iflag != before.Iflag || after.Oflag != before.Oflag {
			t.Errorf("input/output modes = %#x/%#x, want untouched %#x/%#x", after.Iflag, after.Oflag, before.Iflag, before.Oflag)
		}
	})
}

func executeHydrateCommand(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	resetRootCmd()
	rootCmd.SetOut(new(bytes.Buffer))
	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "hydrate", "--fifo", filepath.Join(dir, "hydrate.fifo"), "--file", filepath.Join(dir, "scrollback.bin")})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("executing hydrate: %v\nstderr: %s", err, errBuf)
	}
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

func assertModesEqual(t *testing.T, got, want *unix.Termios) {
	t.Helper()
	if got.Lflag != want.Lflag {
		t.Errorf("local modes = %#x, want %#x", got.Lflag, want.Lflag)
	}
	if got.Iflag != want.Iflag || got.Oflag != want.Oflag || got.Cflag != want.Cflag {
		t.Errorf("input/output/control modes = %#x/%#x/%#x, want %#x/%#x/%#x",
			got.Iflag, got.Oflag, got.Cflag, want.Iflag, want.Oflag, want.Cflag)
	}
	if got.Cc != want.Cc {
		t.Errorf("control characters = %v, want %v", got.Cc, want.Cc)
	}
}

// screenAfterTyping types typed on the terminal's side, lets the reader take it
// off the input queue, then has the reader print a marker row, returning what
// the terminal displayed once a row ended: the marker row alone when nothing
// echoed.
func screenAfterTyping(t *testing.T, master, slave *os.File, typed string) string {
	t.Helper()
	if _, err := master.Write([]byte(typed)); err != nil {
		t.Fatalf("type into the master: %v", err)
	}
	if got := readWithin(t, slave); len(got) == 0 {
		t.Fatalf("the reader saw nothing of %q", typed)
	}
	go func() { _, _ = slave.Write([]byte("END\n")) }()
	return string(readRows(t, master, 1))
}

func TestTTYEcho_RealPTY(t *testing.T) {
	t.Run("clearing echo turns echo alone off", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		before := *localModes(t, fd)
		if before.Lflag&unix.ECHO == 0 {
			t.Fatalf("local modes = %#x, want echo on before the step", before.Lflag)
		}

		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}

		want := before
		want.Lflag &^= unix.ECHO
		assertModesEqual(t, localModes(t, fd), &want)
	})

	t.Run("clearing echo on a terminal already without it changes nothing", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}
		before := *localModes(t, fd)

		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}

		assertModesEqual(t, localModes(t, fd), &before)
	})

	t.Run("setting echo turns echo alone back on", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}
		before := *localModes(t, fd)

		if err := setTTYEcho(fd); err != nil {
			t.Fatalf("setTTYEcho() error = %v", err)
		}

		want := before
		want.Lflag |= unix.ECHO
		assertModesEqual(t, localModes(t, fd), &want)
	})

	t.Run("input typed with echo cleared puts nothing on screen", func(t *testing.T) {
		master, slave := openPTY(t)
		if err := clearTTYEcho(descriptorOf(t, slave)); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}

		if got := screenAfterTyping(t, master, slave, "echo 0123456789\r"); got != "END\r\n" {
			t.Errorf("the terminal displayed %q, want only the reader's own row", got)
		}
	})

	t.Run("a raw-mode round trip puts echo back off", func(t *testing.T) {
		master, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}
		prior, err := term.MakeRaw(uintptr(fd))
		if err != nil {
			t.Fatalf("make the pty raw: %v", err)
		}
		if err := term.Restore(uintptr(fd), prior); err != nil {
			t.Fatalf("restore the pty: %v", err)
		}

		if got := screenAfterTyping(t, master, slave, "echo 0123456789\r"); got != "END\r\n" {
			t.Errorf("the terminal displayed %q after a raw-mode round trip, want only the reader's own row", got)
		}
	})
}

func TestResumeChain_EchoAcrossTheWait_RealPTY(t *testing.T) {
	t.Run("a draw has echo alone turned off", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		var got resumeDrawConfig
		withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
			got = cfg
			return nil
		})
		executeResumeCommand(t, resumeDrawSubcommand)
		withStdin(t, slave)
		before := *localModes(t, fd)

		if err := got.DisableEcho(); err != nil {
			t.Fatalf("DisableEcho() error = %v", err)
		}

		want := before
		want.Lflag &^= unix.ECHO
		assertModesEqual(t, localModes(t, fd), &want)
	})

	t.Run("an answered pane has echo alone turned back on", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYEcho(fd); err != nil {
			t.Fatalf("clearTTYEcho() error = %v", err)
		}
		var got resumeWaitConfig
		withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
			got = cfg
			return nil
		})
		executeResumeCommand(t, resumeWaitSubcommand)
		withStdin(t, slave)
		before := *localModes(t, fd)

		if err := got.EnableEcho(); err != nil {
			t.Fatalf("EnableEcho() error = %v", err)
		}

		want := before
		want.Lflag |= unix.ECHO
		assertModesEqual(t, localModes(t, fd), &want)
	})

	t.Run("an answer hands on the modes the pane held before its first draw, with signal generation on", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		var hydrate hydrateConfig
		withFuncSeam(t, &hydrateRunFunc, func(cfg hydrateConfig) error {
			hydrate = cfg
			return nil
		})
		executeHydrateCommand(t)
		var draw resumeDrawConfig
		withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
			draw = cfg
			return nil
		})
		executeResumeCommand(t, resumeDrawSubcommand)
		var wait resumeWaitConfig
		withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
			wait = cfg
			return nil
		})
		executeResumeCommand(t, resumeWaitSubcommand)
		withStdin(t, slave)
		original := *localModes(t, fd)

		steps := []struct {
			name string
			run  func() error
		}{
			{"hydrate's signal step", hydrate.DisableTTYSignals},
			{"the draw's echo step", draw.DisableEcho},
		}
		for _, step := range steps {
			if err := step.run(); err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
		}
		restore, err := wait.MakeRaw()
		if err != nil {
			t.Fatalf("MakeRaw() error = %v", err)
		}
		restore()
		if err := wait.EnableTTYSignals(); err != nil {
			t.Fatalf("EnableTTYSignals() error = %v", err)
		}
		if err := wait.EnableEcho(); err != nil {
			t.Fatalf("EnableEcho() error = %v", err)
		}

		want := original
		want.Lflag |= unix.ISIG
		assertModesEqual(t, localModes(t, fd), &want)
	})
}
