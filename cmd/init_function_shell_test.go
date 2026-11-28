package cmd

// Runs the session-opening function `portal init` emits in the real shells, with no
// rc files, HOME and TMPDIR in temp dirs, and PATH holding only a stub `portal`
// beside what the shells need from /usr/bin and /bin: no portal binary is built or
// run, and no zmx session is contacted. A shell the machine does not have skips its
// own case.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The stub records its argv a word a line, writes
// PORTAL_STUB_DIR into the file --cwd-file names, as Portal does once it has left
// a session, records which file that was, removes it when PORTAL_STUB_REMOVE is
// set, as a TMPDIR cleaner might, and exits PORTAL_STUB_EXIT.
const cwdStubScript = `#!/bin/sh
printf '%s\n' "$@" > "$PORTAL_STUB_ARGS"
while [ $# -gt 0 ]; do
    if [ "$1" = --cwd-file ]; then
        printf '%s' "$2" > "$PORTAL_STUB_FILE"
        if [ "${PORTAL_STUB_DIR+set}" = set ]; then printf '%s' "$PORTAL_STUB_DIR" > "$2"; fi
        if [ -n "${PORTAL_STUB_REMOVE-}" ]; then /bin/rm -f -- "$2"; fi
        break
    fi
    shift
done
exit "${PORTAL_STUB_EXIT:-0}"
`

// Each driver starts in PORTAL_START, runs PORTAL_PREAMBLE, then x with its own
// arguments, and reports, as the shell exits however
// it does, where the shell ended, its last status and whether the function's
// variables leaked into it. bash's and zsh's define cd, rm, cat and mktemp
// functions that would break the function were it to call them rather than the
// builtin and the commands.
const bashZshFunctionDriver = `source "$PORTAL_INIT_SCRIPT"
builtin cd -- "$PORTAL_START" || exit 99
cd() { return 1; }
rm() { return 1; }
cat() { return 1; }
mktemp() { return 1; }
report() { printf 'PWD %s\nSTATUS %s\nLEAK %s\n' "$PWD" "$1" "${__portal_file-}${__portal_status-}${__portal_dir-}"; }
trap 'report $?' EXIT
eval "$PORTAL_PREAMBLE"
x "$@"
`

// fish's own cd is the one its function calls, to keep dirprev, so only rm,
// cat and mktemp are made hostile there.
const fishFunctionDriver = `source "$PORTAL_INIT_SCRIPT"
cd "$PORTAL_START"; or exit 99
function rm; return 1; end
function cat; return 1; end
function mktemp; return 1; end
eval $PORTAL_PREAMBLE
x $argv
set -l status_ $status
printf 'PWD %s\n' "$PWD"
printf 'STATUS %s\n' $status_
printf 'LEAK %s\n' "$__portal_file$__portal_status$__portal_dir$__portal_tmp"
`

// functionShell is a shell to run the function in: kind is the shell portal
// init is asked for, bin the binary run.
type functionShell struct {
	label, kind, bin string
}

// The bash first on PATH and macOS's own /bin/bash, 3.2, each where it exists.
var functionShells = []functionShell{
	{label: "bash", kind: "bash", bin: "bash"},
	{label: "bin-bash", kind: "bash", bin: "/bin/bash"},
	{label: "zsh", kind: "zsh", bin: "zsh"},
	{label: "fish", kind: "fish", bin: "fish"},
}

// functionRun is one call of the function: where the shell ended, the status it
// returned, what its variables left behind, the argv the stub was handed, and
// the cwd file it was handed, "" for none.
type functionRun struct {
	pwd, status, leak string
	args              []string
	cwdFile           string
}

type functionStage struct {
	start    string
	tmpDir   string
	exit     string
	stubDir  *string
	preamble string
	remove   bool
}

type functionOption func(*functionStage)

// writesDir has the stub write dir into the cwd file; without it the file is
// left as mktemp made it, empty.
func writesDir(dir string) functionOption {
	return func(s *functionStage) { s.stubDir = &dir }
}

func exitsWith(code string) functionOption {
	return func(s *functionStage) { s.exit = code }
}

// tmpDirAt points TMPDIR at dir.
func tmpDirAt(dir string) functionOption {
	return func(s *functionStage) { s.tmpDir = dir }
}

// removesFile has the stub remove the cwd file before Portal's exit.
func removesFile() functionOption {
	return func(s *functionStage) { s.remove = true }
}

// withPreamble runs the shell code line before the function is called.
func withPreamble(line string) functionOption {
	return func(s *functionStage) { s.preamble = line }
}

// runOpenFunction sources what `portal init <shell>` emits and calls its
// session-opening function with args, from a directory of its own.
func runOpenFunction(t *testing.T, shell functionShell, args []string, opts ...functionOption) (functionRun, functionStage) {
	t.Helper()
	shellPath := requireShell(t, shell.bin)

	dir := t.TempDir()
	stage := functionStage{start: t.TempDir(), tmpDir: t.TempDir(), exit: "0"}
	for _, opt := range opts {
		opt(&stage)
	}
	initPath := filepath.Join(dir, "init."+shell.kind)
	if err := os.WriteFile(initPath, []byte(emitInitScript(t, shell.kind)), 0o644); err != nil {
		t.Fatalf("writing the emitted init script: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("creating the stub bin directory: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "portal"), cwdStubScript)

	argsPath, filePath := filepath.Join(dir, "args"), filepath.Join(dir, "file")
	env := []string{
		"HOME=" + t.TempDir(),
		"TMPDIR=" + stage.tmpDir,
		"PATH=" + binDir + ":/usr/bin:/bin",
		"PORTAL_INIT_SCRIPT=" + initPath,
		"PORTAL_START=" + stage.start,
		"PORTAL_PREAMBLE=" + stage.preamble,
		"PORTAL_STUB_ARGS=" + argsPath,
		"PORTAL_STUB_FILE=" + filePath,
		"PORTAL_STUB_EXIT=" + stage.exit,
	}
	if stage.stubDir != nil {
		env = append(env, "PORTAL_STUB_DIR="+*stage.stubDir)
	}
	if stage.remove {
		env = append(env, "PORTAL_STUB_REMOVE=1")
	}

	driverPath := filepath.Join(dir, "driver."+shell.kind)
	// Bounds a function that never returns, so the test fails rather than
	// hanging.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch shell.kind {
	case "bash":
		writeExecutable(t, driverPath, bashZshFunctionDriver)
		cmd = exec.CommandContext(ctx, shellPath, append([]string{"--noprofile", "--norc", driverPath}, args...)...)
	case "zsh":
		// compdef comes with compinit, which zsh -f never runs.
		writeExecutable(t, driverPath, "compdef() { :; }\n"+bashZshFunctionDriver)
		cmd = exec.CommandContext(ctx, shellPath, append([]string{"-f", driverPath}, args...)...)
	case "fish":
		writeExecutable(t, driverPath, fishFunctionDriver)
		cmd = exec.CommandContext(ctx, shellPath, append([]string{"--no-config", driverPath}, args...)...)
	default:
		t.Fatalf("no function driver for shell %q", shell.kind)
	}
	cmd.Env = env
	// A grandchild still holding the output can't keep the read open past the
	// kill.
	cmd.WaitDelay = 5 * time.Second
	// A shell under errexit exits with the function's status, which its
	// report carries; only one that reports nothing failed to run.
	out, err := cmd.Output()
	if _, exited := errors.AsType[*exec.ExitError](err); err != nil && !exited {
		t.Fatalf("%s function driver: %v\noutput:\n%s", shell.label, err, out)
	}
	status := readingWithPrefix(string(out), "STATUS ")
	if status == "" {
		t.Fatalf("%s function driver reported nothing: %v\noutput:\n%s", shell.label, err, out)
	}

	run := functionRun{
		pwd:     readingWithPrefix(string(out), "PWD "),
		status:  status,
		leak:    readingWithPrefix(string(out), "LEAK "),
		args:    readLines(t, argsPath),
		cwdFile: readOptional(t, filePath),
	}
	return run, stage
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the function never ran portal: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
}

func readOptional(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestInitFunction_ChangesToTheDirectoryPortalLeftAndKeepsItsStatus(t *testing.T) {
	for _, shell := range functionShells {
		t.Run(shell.label, func(t *testing.T) {
			target := t.TempDir()

			run, _ := runOpenFunction(t, shell, []string{"api"}, writesDir(target), exitsWith("7"))

			if run.pwd != target {
				t.Errorf("PWD = %q, want %q", run.pwd, target)
			}
			if run.status != "7" {
				t.Errorf("status = %q, want portal's 7", run.status)
			}
			if run.leak != "" {
				t.Errorf("the function's variables leaked into the shell: %q", run.leak)
			}
		})
	}
}

func TestInitFunction_RemovesTheCwdFileItMadeUnderTMPDIR(t *testing.T) {
	for _, shell := range functionShells {
		t.Run(shell.label, func(t *testing.T) {
			run, stage := runOpenFunction(t, shell, nil, writesDir(t.TempDir()))

			if run.cwdFile == "" {
				t.Fatal("portal was handed no --cwd-file")
			}
			if got := filepath.Dir(filepath.Clean(run.cwdFile)); got != stage.tmpDir {
				t.Errorf("cwd file %q is in %q, want TMPDIR %q", run.cwdFile, got, stage.tmpDir)
			}
			if _, err := os.Stat(run.cwdFile); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("cwd file %q stat = %v, want it removed", run.cwdFile, err)
			}
		})
	}
}

func TestInitFunction_StaysPutWhenTheFileNamesNoDirectory(t *testing.T) {
	tests := []struct {
		name string
		opts []functionOption
	}{
		{name: "left empty", opts: nil},
		{name: "naming a directory that is missing", opts: []functionOption{writesDir("/nonexistent/portal-gone")}},
	}
	for _, shell := range functionShells {
		for _, tt := range tests {
			t.Run(shell.label+"/"+tt.name, func(t *testing.T) {
				run, stage := runOpenFunction(t, shell, nil, append(tt.opts, exitsWith("3"))...)

				if run.pwd != stage.start {
					t.Errorf("PWD = %q, want it unchanged at %q", run.pwd, stage.start)
				}
				if run.status != "3" {
					t.Errorf("status = %q, want portal's 3", run.status)
				}
			})
		}
	}
}

func TestInitFunction_PassesTheArgumentsThrough(t *testing.T) {
	user := []string{"-s", "api x", "--", "claude", "--resume"}
	for _, shell := range functionShells {
		t.Run(shell.label, func(t *testing.T) {
			run, _ := runOpenFunction(t, shell, user)

			want := append([]string{"open", "--cwd-file", run.cwdFile}, user...)
			if !slices.Equal(run.args, want) {
				t.Errorf("portal argv = %q, want %q", run.args, want)
			}
		})
	}
}

func TestInitFunction_RunsPortalAsBeforeWithNoTempFile(t *testing.T) {
	for _, shell := range functionShells {
		t.Run(shell.label, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "missing")

			run, stage := runOpenFunction(t, shell, []string{"api"}, tmpDirAt(missing), exitsWith("5"))

			if want := []string{"open", "api"}; !slices.Equal(run.args, want) {
				t.Errorf("portal argv = %q, want %q", run.args, want)
			}
			if run.status != "5" {
				t.Errorf("status = %q, want portal's 5", run.status)
			}
			if run.pwd != stage.start {
				t.Errorf("PWD = %q, want it unchanged at %q", run.pwd, stage.start)
			}
		})
	}
}

func TestInitFunction_ReadsAndRemovesTheFileUnderErrexit(t *testing.T) {
	// zsh runs no EXIT trap on an errexit exit, so its err_exit row reports
	// from ZERR, at the first command to fail outside the function's exempt
	// places: one inside it would report the shell where it started, and the
	// status of what failed rather than portal's.
	preambles := map[string][]string{
		"bash": {"set -e"},
		"zsh":  {"setopt err_return", "trap 'report $?' ZERR; setopt err_exit"},
	}
	tests := []struct {
		name string
		// stage builds the case's options and the directory the shell should
		// end in, "" for where it started.
		stage func(t *testing.T) ([]functionOption, string)
	}{
		{
			name: "a directory left",
			stage: func(t *testing.T) ([]functionOption, string) {
				target := t.TempDir()
				return []functionOption{writesDir(target)}, target
			},
		},
		{
			name: "a file gone before it is read",
			stage: func(t *testing.T) ([]functionOption, string) {
				return []functionOption{writesDir(t.TempDir()), removesFile()}, ""
			},
		},
		{
			name: "a directory that can't be searched",
			stage: func(t *testing.T) ([]functionOption, string) {
				locked := t.TempDir()
				if err := os.Chmod(locked, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
				return []functionOption{writesDir(locked)}, ""
			},
		},
	}
	for _, shell := range functionShells {
		for _, preamble := range preambles[shell.kind] {
			for _, tt := range tests {
				t.Run(shell.label+"/"+preamble+"/"+tt.name, func(t *testing.T) {
					opts, want := tt.stage(t)

					run, stage := runOpenFunction(t, shell, nil, append(opts, exitsWith("7"), withPreamble(preamble))...)

					if want == "" {
						want = stage.start
					}
					if run.pwd != want {
						t.Errorf("PWD = %q, want %q", run.pwd, want)
					}
					if run.status != "7" {
						t.Errorf("status = %q, want portal's 7", run.status)
					}
					if _, err := os.Stat(run.cwdFile); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("cwd file %q stat = %v, want it removed", run.cwdFile, err)
					}
				})
			}
		}
	}
}
