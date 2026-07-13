package spawn

import (
	"errors"
	"strings"
	"testing"
)

// TestRunArgvCombined exercises the shared exec boundary directly (real exec of
// trivial hermetic programs — no tmux, no daemon, no built binary), pinning the
// three-way outcome contract both production runner seams delegate to.
func TestRunArgvCombined(t *testing.T) {
	t.Run("clean exit returns the stdout, a zero code, and a nil err", func(t *testing.T) {
		out, code, err := runArgvCombined([]string{"sh", "-c", "echo ok"})

		if err != nil {
			t.Fatalf("err = %v, want nil on a clean exit", err)
		}
		if code != 0 {
			t.Errorf("exitCode = %d, want 0 on a clean exit", code)
		}
		if strings.TrimSpace(out) != "ok" {
			t.Errorf("out = %q, want it to carry the clean stdout %q", out, "ok")
		}
	})

	t.Run("non-zero exit returns the combined output plus the exit code with a nil err", func(t *testing.T) {
		out, code, err := runArgvCombined([]string{"sh", "-c", "echo stdout-line; echo stderr-line >&2; exit 3"})

		if err != nil {
			t.Fatalf("err = %v, want nil (it ran but failed — the exit code carries the failure)", err)
		}
		if code != 3 {
			t.Errorf("exitCode = %d, want 3", code)
		}
		if !strings.Contains(out, "stdout-line") {
			t.Errorf("out = %q, want it to carry the captured stdout", out)
		}
		if !strings.Contains(out, "stderr-line") {
			t.Errorf("out = %q, want it to carry the child stderr through the combined output", out)
		}
	})

	t.Run("missing binary surfaces the execution error", func(t *testing.T) {
		out, code, err := runArgvCombined([]string{"portal-no-such-binary-xyz"})

		if err == nil {
			t.Fatalf("err = nil, want a non-exit execution error for a missing binary (out=%q)", out)
		}
		if code != 0 {
			t.Errorf("exitCode = %d, want 0 on the non-exit failure path", code)
		}
	})
}

// TestExecFailureDetail pins the shared failure-detail formatter across every
// branch, including the never-empty fallback rendered per fallback label.
func TestExecFailureDetail(t *testing.T) {
	const ghosttyLabel = "ghostty osascript exit %d"
	const recipeLabel = "recipe exit %d"

	t.Run("non-empty output with no error returns the trimmed output", func(t *testing.T) {
		if got := execFailureDetail("  boom  ", 1, nil, ghosttyLabel); got != "boom" {
			t.Errorf("got %q, want %q", got, "boom")
		}
	})

	t.Run("empty output with an error returns the error text", func(t *testing.T) {
		if got := execFailureDetail("   ", 0, errors.New("exec: not found"), ghosttyLabel); got != "exec: not found" {
			t.Errorf("got %q, want %q", got, "exec: not found")
		}
	})

	t.Run("non-empty output with an error joins the detail and the error", func(t *testing.T) {
		if got := execFailureDetail("boom", 1, errors.New("kaboom"), ghosttyLabel); got != "boom: kaboom" {
			t.Errorf("got %q, want %q", got, "boom: kaboom")
		}
	})

	t.Run("empty output and no error falls back to the never-empty label per fallback string", func(t *testing.T) {
		if got := execFailureDetail("", 7, nil, ghosttyLabel); got != "ghostty osascript exit 7" {
			t.Errorf("ghostty fallback = %q, want %q", got, "ghostty osascript exit 7")
		}
		if got := execFailureDetail("", 7, nil, recipeLabel); got != "recipe exit 7" {
			t.Errorf("recipe fallback = %q, want %q", got, "recipe exit 7")
		}
	})
}

// TestFailureDetailWrappersDelegate proves both wrappers return exactly what
// execFailureDetail returns for their own fallback label across representative
// inputs — so the extraction preserves each side's behaviour and only the
// fallback label differs.
func TestFailureDetailWrappersDelegate(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		exitCode int
		err      error
	}{
		{name: "trimmed output wins", out: "  detail  ", exitCode: 2, err: nil},
		{name: "error only", out: "", exitCode: 0, err: errors.New("boom")},
		{name: "detail and error joined", out: "detail", exitCode: 4, err: errors.New("boom")},
		{name: "empty falls back to the label", out: "", exitCode: 9, err: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := failureDetail(tc.out, tc.exitCode, tc.err)
			want := execFailureDetail(tc.out, tc.exitCode, tc.err, "ghostty osascript exit %d")
			if got != want {
				t.Errorf("failureDetail = %q, want %q (execFailureDetail with the ghostty label)", got, want)
			}

			got = recipeFailureDetail(tc.out, tc.exitCode, tc.err)
			want = execFailureDetail(tc.out, tc.exitCode, tc.err, "recipe exit %d")
			if got != want {
				t.Errorf("recipeFailureDetail = %q, want %q (execFailureDetail with the recipe label)", got, want)
			}
		})
	}
}
