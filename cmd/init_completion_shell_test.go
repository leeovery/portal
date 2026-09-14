package cmd

// Drives the scripts `portal init` emits through the real shells, over a recording
// `portal` stub on PATH: no portal binary is built or run, and no tmux server is
// contacted. A shell the machine does not have skips its own case.

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const shellStubCandidate = "/portal-a1b2"

// The recording stub joins the argv it was handed with this, so an empty trailing
// word — the one the shells append for a complete word followed by a space — stays
// visible in the recorded request.
const requestFieldSeparator = "|"

const portalStubScript = `#!/bin/sh
( IFS='` + requestFieldSeparator + `'; printf '%s\n' "$*" ) >> "$PORTAL_STUB_RECORD"
printf '%s\n' "$PORTAL_STUB_CANDIDATES"
printf ':4\n'
`

// Fills cur/prev/words/cword the way the bash-completion package's own
// _init_completion does: the generated script prefers it when it is declared, and
// its fallback needs _get_comp_words_by_ref, which is absent for the same reason.
// cur comes from COMP_LINE and COMP_POINT rather than from COMP_WORDS[COMP_CWORD],
// as the package's own cursor walk does — a cursor left mid-line takes the current
// word no further than itself.
const bashCompletionDriver = `_init_completion() {
    local index start=0
    for (( index = 0; index <= COMP_CWORD; index++ )); do
        while [[ ${COMP_LINE:start:1} == " " ]]; do (( start++ )); done
        (( index == COMP_CWORD )) && break
        (( start += ${#COMP_WORDS[index]} ))
    done
    cur=${COMP_LINE:start:COMP_POINT-start}
    prev=${COMP_WORDS[COMP_CWORD-1]}
    words=("${COMP_WORDS[@]}")
    cword=$COMP_CWORD
    return 0
}

source "$PORTAL_INIT_SCRIPT"

COMP_WORDS=("$@")
COMP_CWORD=$PORTAL_COMPLETION_CWORD
COMP_LINE=$PORTAL_COMPLETION_LINE
COMP_POINT=$PORTAL_COMPLETION_POINT

fn=$(complete -p "${COMP_WORDS[0]}" | sed -n 's/.* -F \([^ ]*\) .*/\1/p')
"$fn" "${COMP_WORDS[0]}" "${COMP_WORDS[COMP_CWORD]}" "${COMP_WORDS[COMP_CWORD-1]}"
printf 'REPLY %s\n' "${COMPREPLY[@]}"
printf 'LINE %s\n' "$COMP_LINE"
printf 'POINT %s\n' "$COMP_POINT"
`

// compdef and the compsys functions the generated script calls are stand-ins, so
// the completion runs without compinit: compdef records which function the script
// registered for a word, and _describe reports what it was offered while matching
// on the current word the way zsh's own does. _files and _arguments report that
// file completion was reached at all.
const zshCompletionDriver = `typeset -A registered
compdef() { registered[$2]=$1 }
_describe() {
    local name=$2 current=${words[CURRENT]} candidate found=1
    for candidate in ${(P)name}; do
        if [[ $candidate == ${current}* ]]; then
            print -r -- "REPLY $candidate"
            found=0
        fi
    done
    return $found
}
compadd() { print -rl -- "REPLY $@" }
_files() { print -r -- "FILECOMP _files" }
_arguments() { print -r -- "FILECOMP $@" }
_message() { : }

source "$PORTAL_INIT_SCRIPT"

words=("$@")
CURRENT=$#
${registered[$words[1]]}
# A completion function reports "no candidates" with a non-zero status, which is not
# the driver's verdict — that is the request and the reply it printed.
exit 0
`

const fishCompletionDriver = `source "$PORTAL_INIT_SCRIPT"
for candidate in (complete -C "$PORTAL_COMPLETION_LINE")
    printf 'REPLY %s\n' $candidate
end
`

// completionRun is what one Tab press produced: the request the shell made of
// Portal, the candidates it offered back, every file-completion call the compsys
// stand-ins caught, cobra's own debug trace of the run, and the command line the
// completion was left holding with the cursor offset into it (bash alone rewrites
// either, so the other shells report both empty).
type completionRun struct {
	request  string
	reply    []string
	fileComp []string
	debug    string
	line     string
	point    string
}

// completionLine is the command line one Tab press is taken on: the words the shell
// splits it into, the line as the user typed it, and the index of the word their
// cursor sits at the end of.
type completionLine struct {
	words      []string
	typed      string
	cursorWord int
}

// completionOption states a departure from a line whose words are separated by a
// single space with the cursor at its end.
type completionOption func(*completionLine)

// cursorAfterWord puts the cursor at the end of the word at the given index rather
// than at the end of the line.
func cursorAfterWord(index int) completionOption {
	return func(line *completionLine) { line.cursorWord = index }
}

// typedLine states the line exactly as the user typed it, for spacing that joining
// the words with a single space cannot spell.
func typedLine(typed string) completionOption {
	return func(line *completionLine) { line.typed = typed }
}

func newCompletionLine(words []string, opts ...completionOption) completionLine {
	line := completionLine{
		words:      words,
		typed:      strings.Join(words, " "),
		cursorWord: len(words) - 1,
	}
	for _, opt := range opts {
		opt(&line)
	}
	return line
}

// point reports the cursor's byte offset into the typed line, found by walking the
// words in order across the spacing between them.
func (l completionLine) point() int {
	offset := 0
	for index := 0; index < l.cursorWord; index++ {
		for offset < len(l.typed) && l.typed[offset] == ' ' {
			offset++
		}
		offset += len(l.words[index])
	}
	for offset < len(l.typed) && l.typed[offset] == ' ' {
		offset++
	}
	return offset + len(l.words[l.cursorWord])
}

// requireShell resolves a shell binary, skipping the test when the machine has none.
func requireShell(t *testing.T, shell string) string {
	t.Helper()

	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s is not installed on this machine", shell)
	}
	return path
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// driveCompletion sources what `portal init <shell>` emits, completes the given
// command line in that shell, and reports what Portal was asked and what the shell
// offered. The words are the whole line, the last one being the word under the
// cursor — empty for a line ending in a space — unless an option says otherwise.
func driveCompletion(t *testing.T, shell string, initArgs []string, words []string, candidates []string, opts ...completionOption) completionRun {
	t.Helper()

	line := newCompletionLine(words, opts...)

	shellPath := requireShell(t, shell)

	dir := t.TempDir()
	initPath := filepath.Join(dir, "init."+shell)
	if err := os.WriteFile(initPath, []byte(emitInitScript(t, shell, initArgs...)), 0o644); err != nil {
		t.Fatalf("writing the emitted init script: %v", err)
	}

	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("creating the stub bin directory: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "portal"), portalStubScript)

	recordPath := filepath.Join(dir, "request")
	debugPath := filepath.Join(dir, "debug")
	env := append(os.Environ(),
		"BASH_COMP_DEBUG_FILE="+debugPath,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_STUB_RECORD="+recordPath,
		"PORTAL_STUB_CANDIDATES="+strings.Join(candidates, "\n"),
		"PORTAL_INIT_SCRIPT="+initPath,
		"PORTAL_COMPLETION_LINE="+line.typed,
		"PORTAL_COMPLETION_POINT="+strconv.Itoa(line.point()),
		"PORTAL_COMPLETION_CWORD="+strconv.Itoa(line.cursorWord),
	)

	driverPath := filepath.Join(dir, "driver."+shell)
	var cmd *exec.Cmd
	switch shell {
	case "bash":
		writeExecutable(t, driverPath, bashCompletionDriver)
		cmd = exec.Command(shellPath, append([]string{"--noprofile", "--norc", driverPath}, words...)...)
	case "zsh":
		writeExecutable(t, driverPath, zshCompletionDriver)
		cmd = exec.Command(shellPath, append([]string{"-f", driverPath}, words...)...)
	case "fish":
		writeExecutable(t, driverPath, fishCompletionDriver)
		cmd = exec.Command(shellPath, "--no-config", driverPath)
	default:
		t.Fatalf("no completion driver for shell %q", shell)
	}
	cmd.Env = env

	// Nothing in the verdict comes from stderr: compopt outside a live completion
	// context prints a diagnostic there, and Output captures it unread.
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s completion driver: %v\noutput:\n%s", shell, err, out)
	}

	return completionRun{
		request:  readRecordedRequest(t, recordPath),
		reply:    linesWithPrefix(string(out), "REPLY "),
		fileComp: linesWithPrefix(string(out), "FILECOMP "),
		debug:    readDebugTrace(t, debugPath),
		line:     readingWithPrefix(string(out), "LINE "),
		point:    readingWithPrefix(string(out), "POINT "),
	}
}

// readDebugTrace reports what the generated script wrote to cobra's own debug
// channel, which is where bash records that it switched file completion off — the
// compopt call itself is gated on compopt being a builtin, so a stub cannot see it.
func readDebugTrace(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("reading the completion debug trace: %v", err)
	}
	return string(body)
}

// readRecordedRequest reports the one request the stub was handed, failing when the
// shell asked Portal more or less than once.
func readRecordedRequest(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the shell made no request of portal: %v", err)
	}

	requests := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(requests) != 1 {
		t.Fatalf("recorded %d requests, want exactly 1:\n%s", len(requests), body)
	}
	return requests[0]
}

func linesWithPrefix(out, prefix string) []string {
	var found []string
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if after, ok := strings.CutPrefix(line, prefix); ok {
			found = append(found, after)
		}
	}
	return found
}

// readingWithPrefix reports the one reading a driver printed under the prefix, and
// the empty string for a driver that prints none.
func readingWithPrefix(out, prefix string) string {
	found := linesWithPrefix(out, prefix)
	if len(found) == 0 {
		return ""
	}
	return found[0]
}

func request(fields ...string) string {
	return strings.Join(fields, requestFieldSeparator)
}

var completionShells = []string{"bash", "zsh", "fish"}

func TestInitCompletion_AsksPortalForOpenCompletions(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  string
	}{
		{
			name:  "a search-form word",
			words: []string{"x", "/po"},
			want:  request("__complete", "open", "/po"),
		},
		{
			name:  "an empty word",
			words: []string{"x", ""},
			want:  request("__complete", "open", ""),
		},
		{
			name:  "an empty word after a flag",
			words: []string{"x", "-s", ""},
			want:  request("__complete", "open", "-s", ""),
		},
	}

	for _, shell := range completionShells {
		for _, tt := range tests {
			t.Run(shell+"/"+tt.name, func(t *testing.T) {
				run := driveCompletion(t, shell, nil, tt.words, []string{shellStubCandidate})

				if run.request != tt.want {
					t.Errorf("request = %q, want %q", run.request, tt.want)
				}
			})
		}
	}
}

func TestInitCompletion_LeavesControlFunctionRequestUnchanged(t *testing.T) {
	for _, shell := range completionShells {
		t.Run(shell, func(t *testing.T) {
			run := driveCompletion(t, shell, nil, []string{"xctl", ""}, []string{shellStubCandidate})

			want := request("__complete", "")
			if run.request != want {
				t.Errorf("request = %q, want %q", run.request, want)
			}
		})
	}
}

func TestInitCompletion_CarriesSlashPrefixedCandidateIntoReply(t *testing.T) {
	for _, shell := range completionShells {
		t.Run(shell, func(t *testing.T) {
			run := driveCompletion(t, shell, nil, []string{"x", "/po"}, []string{shellStubCandidate})

			if !slices.Contains(run.reply, shellStubCandidate) {
				t.Errorf("reply = %q, want it to offer %q", run.reply, shellStubCandidate)
			}
		})
	}
}

func TestInitCompletion_OffersNoFilenameForPartialAbsoluteDirectory(t *testing.T) {
	for _, shell := range completionShells {
		t.Run(shell, func(t *testing.T) {
			run := driveCompletion(t, shell, nil, []string{"x", "/tm"}, []string{shellStubCandidate})

			for _, candidate := range run.reply {
				if strings.HasPrefix(candidate, "/tm") {
					t.Errorf("reply = %q, want no filename under /tm", run.reply)
				}
			}
			if len(run.fileComp) != 0 {
				t.Errorf("the completion reached file completion: %q", run.fileComp)
			}
			// Each shell reports the switch-off on cobra's debug channel in its own
			// words; a run that reports neither never switched it off.
			if shell == "bash" && !strings.Contains(run.debug, "Activating no file completion") {
				t.Errorf("bash did not switch file completion off\ndebug trace:\n%s", run.debug)
			}
			if shell == "zsh" && !strings.Contains(run.debug, "deactivating file completion") {
				t.Errorf("zsh did not switch file completion off\ndebug trace:\n%s", run.debug)
			}
		})
	}
}

func TestInitCompletion_FollowsTheConfiguredFunctionName(t *testing.T) {
	for _, shell := range completionShells {
		t.Run(shell, func(t *testing.T) {
			run := driveCompletion(t, shell, []string{"--cmd", "p"}, []string{"p", "/po"}, []string{shellStubCandidate})

			want := request("__complete", "open", "/po")
			if run.request != want {
				t.Errorf("request = %q, want %q", run.request, want)
			}
		})
	}
}

func TestInitCompletion_SkipsAnAbsentShell(t *testing.T) {
	const absent = "portal-no-such-shell"
	if _, err := exec.LookPath(absent); err == nil {
		t.Fatalf("%q resolves on this machine, so it cannot stand in for an absent shell", absent)
	}

	t.Run("absent shell", func(t *testing.T) {
		requireShell(t, absent)
		t.Error("requireShell returned for an absent shell instead of skipping")
	})
}

// The cursor cases are bash's alone: zsh derives its own PREFIX/SUFFIX from the real
// cursor and fish's wrap never touches the line, so neither shim moves it.
func TestInitCompletion_CompletesTheWordUnderAMidLineCursor(t *testing.T) {
	run := driveCompletion(t, "bash", nil, []string{"x", "/po", "extra"}, []string{shellStubCandidate}, cursorAfterWord(1))

	want := request("__complete", "open", "/po")
	if run.request != want {
		t.Errorf("request = %q, want %q", run.request, want)
	}
	if !slices.Contains(run.reply, shellStubCandidate) {
		t.Errorf("reply = %q, want it to offer %q", run.reply, shellStubCandidate)
	}
}

func TestInitCompletion_ShiftsTheCursorRatherThanPinningItToTheEndOfTheLine(t *testing.T) {
	run := driveCompletion(t, "bash", nil, []string{"x", "/po", "extra"}, []string{shellStubCandidate}, cursorAfterWord(1))

	// `x /po` is 5 characters and `portal open /po` is 15.
	const want = "15"
	if run.point != want {
		t.Errorf("COMP_POINT = %q, want %q (the line is %q)", run.point, want, run.line)
	}
}

func TestInitCompletion_PreservesTypedSpacingInTheRewrittenLine(t *testing.T) {
	run := driveCompletion(t, "bash", nil, []string{"x", "api"}, []string{shellStubCandidate}, typedLine("x   api"))

	const want = "portal open   api"
	if run.line != want {
		t.Errorf("COMP_LINE = %q, want %q", run.line, want)
	}
}

func TestInitCompletion_CompletesOnALineWithLeadingWhitespace(t *testing.T) {
	run := driveCompletion(t, "bash", nil, []string{"x", "/po"}, []string{shellStubCandidate}, typedLine("   x /po"))

	want := request("__complete", "open", "/po")
	if run.request != want {
		t.Errorf("request = %q, want %q", run.request, want)
	}
	if !slices.Contains(run.reply, shellStubCandidate) {
		t.Errorf("reply = %q, want it to offer %q", run.reply, shellStubCandidate)
	}
}
