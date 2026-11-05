package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/migrate"
)

// tmuxShim is a tmux of the test's own, first on PATH, which answers each
// subcommand it is given codes for with the next of them, its last once they
// run out, and refuses every other: no real tmux is reached. Having answered
// a subcommand marked as vanishing, it removes itself, so the next run finds
// no tmux to run.
const tmuxShim = `#!/bin/sh
dir=$(dirname "$0")
sub=$3
[ -f "$dir/$sub.codes" ] || { echo "the shim answers no $sub" >&2; exit 2; }
n=$(( $(cat "$dir/$sub.count" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$dir/$sub.count"
code=$(sed -n "${n}p" "$dir/$sub.codes")
[ -n "$code" ] || code=$(tail -n 1 "$dir/$sub.codes")
[ "$code" = 0 ] || echo "$sub refused by the shim" >&2
[ ! -f "$dir/$sub.vanishes" ] || rm -f "$0"
exit "$code"
`

// shimTmux puts tmuxShim first on PATH, answering each subcommand of codes
// with its exit codes in turn and vanishing once it has answered any of
// vanishing, and returns a Tmux reaching it.
func shimTmux(t *testing.T, codes map[string][]int, vanishing ...string) migrate.Tmux {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(tmuxShim), 0o700); err != nil {
		t.Fatal(err)
	}
	for sub, exits := range codes {
		var lines []string
		for _, code := range exits {
			lines = append(lines, strconv.Itoa(code))
		}
		writeFile(t, filepath.Join(dir, sub+".codes"), strings.Join(lines, "\n")+"\n")
	}
	for _, sub := range vanishing {
		writeFile(t, filepath.Join(dir, sub+".vanishes"), "")
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	return migrate.Tmux{Socket: filepath.Join(dir, "no-server")}
}

func TestTmux_KillSaverFailing(t *testing.T) {
	t.Run("it fails on a kill the saver outlasts", func(t *testing.T) {
		tmux := shimTmux(t, map[string][]int{"has-session": {0}, "kill-session": {1}})

		killed, err := tmux.KillSaver(context.Background())

		if killed || err == nil || !strings.Contains(err.Error(), "kill-session refused by the shim") {
			t.Errorf("KillSaver = %v, %v; want false and the kill's failure, saying what tmux said", killed, err)
		}
	})

	t.Run("it fails on a failed kill it can't check, tmux gone without an answer", func(t *testing.T) {
		tmux := shimTmux(t, map[string][]int{"has-session": {0}, "kill-session": {1}}, "kill-session")

		killed, err := tmux.KillSaver(context.Background())

		if killed || err == nil || !strings.Contains(err.Error(), "kill-session refused by the shim") {
			t.Errorf("KillSaver = %v, %v; want false and the kill's failure: a check that can't tell isn't the saver gone", killed, err)
		}
	})

	t.Run("it counts a saver gone when a failed kill is checked as killed", func(t *testing.T) {
		tmux := shimTmux(t, map[string][]int{"has-session": {0, 1}, "kill-session": {1}})

		killed, err := tmux.KillSaver(context.Background())

		if !killed || err != nil {
			t.Errorf("KillSaver = %v, %v; want true, nil: the saver went all the same", killed, err)
		}
	})
}
