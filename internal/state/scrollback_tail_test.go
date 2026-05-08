package state_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// writeTailFixture writes data to a fresh .bin path inside a fresh temp dir
// and returns the path. Centralises the boilerplate used by the tail tests.
func writeTailFixture(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// buildLines returns the bytes for `count` newline-terminated lines whose
// content is "line-<i>\n" for i in [0, count). The exact line text matters
// for byte-identity assertions against a naive whole-file tail.
func buildLines(count int) []byte {
	var buf bytes.Buffer
	for i := 0; i < count; i++ {
		fmt.Fprintf(&buf, "line-%d\n", i)
	}
	return buf.Bytes()
}

// naiveTail returns the last n newline-terminated lines from data using the
// straightforward whole-file approach. Used as the byte-identity oracle for
// the chunked reverse-scan implementation under test.
func naiveTail(data []byte, n int) []byte {
	if len(data) == 0 {
		return nil
	}
	// Count newlines walking backwards; track the cut point.
	seen := 0
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != '\n' {
			continue
		}
		// Skip the trailing newline of the file itself first.
		if i == len(data)-1 && seen == 0 {
			seen++
			continue
		}
		seen++
		if seen == n+1 {
			// data[i] is the newline before the (n+1)th-from-last line; the
			// returned slice starts at the byte after it.
			return data[i+1:]
		}
	}
	// Fewer than n lines available: return everything up to and including the
	// last \n, which (for fully-terminated input) is the whole buffer.
	return data
}

func TestTailScrollback(t *testing.T) {
	t.Run("returns the last N terminated lines when the file has more than N lines", func(t *testing.T) {
		data := buildLines(1500)
		path := writeTailFixture(t, data)

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		want := naiveTail(data, 1000)
		if !bytes.Equal(got, want) {
			t.Fatalf("tail mismatch: got %d bytes, want %d bytes", len(got), len(want))
		}
		if got := bytes.Count(got, []byte{'\n'}); got != 1000 {
			t.Errorf("newline count = %d, want 1000", got)
		}
	})

	t.Run("returns all lines when the file has fewer than N", func(t *testing.T) {
		data := buildLines(5)
		path := writeTailFixture(t, data)

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("expected all content returned; got %d bytes, want %d bytes", len(got), len(data))
		}
		if got := bytes.Count(got, []byte{'\n'}); got != 5 {
			t.Errorf("newline count = %d, want 5", got)
		}
	})

	t.Run("returns exactly N lines when the file has exactly N lines", func(t *testing.T) {
		data := buildLines(1000)
		path := writeTailFixture(t, data)

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("expected all content returned; got %d bytes, want %d bytes", len(got), len(data))
		}
		if got := bytes.Count(got, []byte{'\n'}); got != 1000 {
			t.Errorf("newline count = %d, want 1000", got)
		}
	})

	t.Run("assembles the tail correctly when N lines span multiple chunk boundaries", func(t *testing.T) {
		// Lines wider than the chunk stride: 2 KiB per line × 200 lines >>
		// any sane chunk constant (8/64 KiB). Asking for the last 50 forces
		// the (51)th-from-last \n to live many chunks back from EOF.
		const lineCount = 200
		const tailN = 50
		const lineWidth = 2048
		var buf bytes.Buffer
		filler := strings.Repeat("x", lineWidth-1) // -1 leaves room for \n
		for i := 0; i < lineCount; i++ {
			fmt.Fprintf(&buf, "%05d-%s\n", i, filler[6:]) // keep total = lineWidth
		}
		data := buf.Bytes()
		path := writeTailFixture(t, data)

		got, err := state.TailScrollback(path, tailN)
		if err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		want := naiveTail(data, tailN)
		if !bytes.Equal(got, want) {
			t.Fatalf("tail mismatch: got %d bytes, want %d bytes", len(got), len(want))
		}
		if got := bytes.Count(got, []byte{'\n'}); got != tailN {
			t.Errorf("newline count = %d, want %d", got, tailN)
		}
	})

	t.Run("preserves the trailing newline on the returned bytes", func(t *testing.T) {
		data := buildLines(10)
		path := writeTailFixture(t, data)

		got, err := state.TailScrollback(path, 3)
		if err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		if len(got) == 0 || got[len(got)-1] != '\n' {
			start := len(got) - 8
			if start < 0 {
				start = 0
			}
			t.Fatalf("expected trailing \\n; got tail ending in %q", string(got[start:]))
		}
	})

	t.Run("returns (nil, nil) for a missing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "does-not-exist.bin")

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: unexpected error %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil bytes, got %d bytes", len(got))
		}
	})

	t.Run("returns (nil, nil) for a zero-byte file", func(t *testing.T) {
		path := writeTailFixture(t, nil)

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: unexpected error %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil bytes, got %d bytes", len(got))
		}
	})

	t.Run("returns (nil, nil) for a file with only an unterminated partial line", func(t *testing.T) {
		path := writeTailFixture(t, []byte("partial line without newline"))

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: unexpected error %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil bytes, got %q", string(got))
		}
	})

	t.Run("excludes a trailing partial line from the returned tail", func(t *testing.T) {
		path := writeTailFixture(t, []byte("line1\nline2\npartial"))

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: unexpected error %v", err)
		}
		want := []byte("line1\nline2\n")
		if !bytes.Equal(got, want) {
			t.Fatalf("tail mismatch: got %q, want %q", string(got), string(want))
		}
	})

	t.Run("preserves a single empty terminated line as content", func(t *testing.T) {
		path := writeTailFixture(t, []byte("\n"))

		got, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("TailScrollback: unexpected error %v", err)
		}
		want := []byte("\n")
		if !bytes.Equal(got, want) {
			t.Fatalf("tail mismatch: got %q, want %q", string(got), string(want))
		}
	})

	t.Run("does not surface ENOENT as an error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "missing.bin")

		_, err := state.TailScrollback(path, 1000)
		if err != nil {
			t.Fatalf("expected literal nil error, got %v (is fs.ErrNotExist = %v)", err, errors.Is(err, fs.ErrNotExist))
		}
	})

	t.Run("holds a single file descriptor across the reverse scan", func(t *testing.T) {
		// Force a wide span so the reverse-scan must call Read multiple
		// times. Any close-and-reopen between chunk reads would show up as
		// > 1 open against the seam.
		const lineCount = 4000
		data := buildLines(lineCount)
		path := writeTailFixture(t, data)

		var opens int
		restore := state.SetOpenFileForTest(func(name string) (*os.File, error) {
			opens++
			return os.Open(name)
		})
		t.Cleanup(restore)

		if _, err := state.TailScrollback(path, 1000); err != nil {
			t.Fatalf("TailScrollback: %v", err)
		}
		if opens != 1 {
			t.Errorf("file opened %d times, want exactly 1", opens)
		}
	})
}
