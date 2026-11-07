//go:build integration

package vtcapture_test

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/vtcapture"
	"github.com/leeovery/portal/internal/zmxtest"
)

// settle bounds each wait for a session's screen to show what its program
// drew.
const settle = 10 * time.Second

// TestCaptureAgainstZmx holds the package's reading of zmx to the build: each
// capture form it reads is the one zmx answers with. A zmx that leads or
// closes a History another way fails here, rather than leaving a restored
// prompt among the rows it replays or a program's modes on in its shell.
func TestCaptureAgainstZmx(t *testing.T) {
	b := zmxtest.New(t)
	for i, tc := range captureForms {
		if _, err := b.Create(t.Context(), "form-"+strconv.Itoa(i), t.TempDir(), []string{"sh", "-c", tc.script + "; exec sleep 1000"}); err != nil {
			t.Fatalf("create the session for %s: %v", tc.name, err)
		}
	}
	for i, tc := range captureForms {
		t.Run("it captures "+tc.name+" as the package reads it", func(t *testing.T) {
			want := tc.lead + tc.rows + tc.closer
			var got []byte
			if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
				var err error
				got, err = b.History(t.Context(), "form-"+strconv.Itoa(i), 0)
				return err == nil && string(got) == want
			}) {
				t.Errorf("History = %q, want %q", got, want)
			}
		})
	}
}

// TestAltScreenAgainstZmx pins AlternateScreen to the captures real zmx
// gives: it fires for a session on the alternate screen, as vim and less put
// one, and for none on its main screen, whatever modes lead its capture.
func TestAltScreenAgainstZmx(t *testing.T) {
	b := zmxtest.New(t)
	const enter = `printf 'main line\n'; printf '\033[?%sh\033[H\033[2Jalt line\nready'; exec sleep 600`
	for i, tc := range []struct {
		name, program string
		// ready is the text the capture holds once the program has drawn.
		ready string
		// needs is a program the case runs, skipped where it isn't installed.
		needs string
		want  bool
	}{
		{
			name:    "the modes vim sets, the alternate screen among them",
			program: `printf 'main line\n'; printf '\033[?1049h\033[22;0;0t\033[>4;2m\033[?1h\033=\033[?2004h\033[?1004h\033[?1000h\033[?1006h\033[H\033[2Jalt line\nready'; exec sleep 600`,
			ready:   "ready",
			want:    true,
		},
		{name: "mode 1049", program: fmt.Sprintf(enter, "1049"), ready: "ready", want: true},
		// zmx writes the ANSI modes off their defaults ahead of the DEC ones.
		{name: "1049 after IRM", program: `printf '\033[4h'; ` + fmt.Sprintf(enter, "1049"), ready: "ready", want: true},
		{name: "1049 after LNM", program: `printf '\033[20h'; ` + fmt.Sprintf(enter, "1049"), ready: "ready", want: true},
		{name: "1049 after KAM", program: `printf '\033[2h'; ` + fmt.Sprintf(enter, "1049"), ready: "ready", want: true},
		{name: "1049 after SRM reset", program: `printf '\033[12l'; ` + fmt.Sprintf(enter, "1049"), ready: "ready", want: true},
		{
			name:    "1049 after IRM, LNM, KAM and SRM",
			program: `printf '\033[4h\033[20h\033[2h\033[12l'; ` + fmt.Sprintf(enter, "1049"),
			ready:   "ready",
			want:    true,
		},
		{
			name:    "the main screen led by IRM, LNM, KAM and SRM",
			program: `printf '\033[4h\033[20h\033[2h\033[12l\033[?2004hmain line\nready'; exec sleep 600`,
			ready:   "ready",
		},
		{name: "mode 1047", program: fmt.Sprintf(enter, "1047"), ready: "ready", want: true},
		{name: "mode 47", program: fmt.Sprintf(enter, "47"), ready: "ready", want: true},
		{
			name:    "vim",
			program: `exec vim -u NONE -i NONE -N -n -c 'call setline(1, "alt line ready")'`,
			ready:   "alt line ready",
			needs:   "vim",
			want:    true,
		},
		{
			name:    "less",
			program: `printf 'one\nready less\n' > "$HOME/page"; exec less "$HOME/page"`,
			ready:   "ready less",
			needs:   "less",
			want:    true,
		},
		{name: "the main screen", program: `printf 'main line\nready'; exec sleep 600`, ready: "ready"},
		{
			name:    "the main screen led by modes a shell sets",
			program: `printf '\033[?1h\033=\033[?25l\033[?2004h\033[?1000hmain line\nready'; exec sleep 600`,
			ready:   "ready",
		},
		{
			name:    "the main screen, back from the alternate screen",
			program: `printf 'main line\n'; printf '\033[?1049h\033[H\033[2Jalt line'; printf '\033[?1049lready'; exec sleep 600`,
			ready:   "ready",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needs != "" {
				if _, err := exec.LookPath(tc.needs); err != nil {
					t.Skipf("%s isn't installed", tc.needs)
				}
			}
			name := fmt.Sprintf("screen-%d", i)
			if _, err := b.Create(t.Context(), name, t.TempDir(), []string{"/bin/sh", "-c", tc.program}); err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			var vt []byte
			if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
				got, err := b.History(t.Context(), name, 0)
				vt = got
				return err == nil && strings.Contains(string(got), tc.ready)
			}) {
				t.Fatalf("the capture never showed %q: %q", tc.ready, vt)
			}

			if got := vtcapture.AlternateScreen(vt); got != tc.want {
				t.Errorf("AlternateScreen(%q) = %v, want %v", vt, got, tc.want)
			}
		})
	}
}
