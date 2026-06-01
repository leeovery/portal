package state_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

// captureFunc is a tiny PaneCapturer impl used for CaptureAndHashPane tests.
type captureFunc func(target string) (string, error)

func (f captureFunc) CapturePane(target string) (string, error) { return f(target) }

// openTempLogger returns a capturing *slog.Logger plus the captureSink so
// callers can inspect the rendered log body after the call under test.
func openTempLogger(t *testing.T) (*slog.Logger, *captureSink) {
	t.Helper()
	return newCaptureLogger(t)
}

func TestSeedHashMap(t *testing.T) {
	t.Run("returns empty HashMap when scrollback directory is missing", func(t *testing.T) {
		dir := t.TempDir()
		// Note: do NOT call EnsureDir — the scrollback subdir must be absent.
		logger, _ := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		if len(hm) != 0 {
			t.Errorf("SeedHashMap on missing dir = %v, want empty map", hm)
		}
	})

	t.Run("returns empty HashMap for an empty scrollback directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		logger, _ := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		if len(hm) != 0 {
			t.Errorf("SeedHashMap on empty dir = %v, want empty map", hm)
		}
	})

	t.Run("hashes every .bin file during seed", func(t *testing.T) {
		dir := t.TempDir()
		sb := state.ScrollbackDir(dir)
		if err := os.MkdirAll(sb, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		fixtures := map[string][]byte{
			"work__0.0":    []byte("alpha"),
			"work__0.1":    []byte("beta-with-trailing-newline\n"),
			"side__1.2":    []byte(""),
			"deep__nested": []byte("\x1b[31mred\x1b[0m"),
			"binary__0.0":  {0x00, 0x01, 0x02, 0xff},
		}
		for k, v := range fixtures {
			if err := os.WriteFile(filepath.Join(sb, k+".bin"), v, 0o600); err != nil {
				t.Fatalf("write fixture %s: %v", k, err)
			}
		}
		logger, _ := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		if len(hm) != len(fixtures) {
			t.Fatalf("hm has %d entries, want %d", len(hm), len(fixtures))
		}
		for k, v := range fixtures {
			want := xxhash.Sum64(v)
			got, ok := hm[k]
			if !ok {
				t.Errorf("missing entry for %q", k)
				continue
			}
			if got != want {
				t.Errorf("hm[%q] = %d, want %d", k, got, want)
			}
		}
	})

	t.Run("skips non-bin files during seed", func(t *testing.T) {
		dir := t.TempDir()
		sb := state.ScrollbackDir(dir)
		if err := os.MkdirAll(sb, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sb, "stray.txt"), []byte("ignored"), 0o600); err != nil {
			t.Fatalf("write stray: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sb, "no-extension"), []byte("ignored"), 0o600); err != nil {
			t.Fatalf("write no-extension: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sb, "work__0.0.bin"), []byte("kept"), 0o600); err != nil {
			t.Fatalf("write kept: %v", err)
		}
		// Subdirectory should also be skipped (it's a directory, not a .bin file).
		if err := os.MkdirAll(filepath.Join(sb, "subdir"), 0o700); err != nil {
			t.Fatalf("MkdirAll subdir: %v", err)
		}
		logger, _ := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		if len(hm) != 1 {
			t.Fatalf("hm has %d entries, want 1; got %v", len(hm), hm)
		}
		want := xxhash.Sum64([]byte("kept"))
		if hm["work__0.0"] != want {
			t.Errorf("hm[work__0.0] = %d, want %d", hm["work__0.0"], want)
		}
	})

	t.Run("logs a warning and continues when a .bin file is unreadable", func(t *testing.T) {
		// Skip when running as root (chmod 0o000 cannot make a file unreadable for root).
		if os.Geteuid() == 0 {
			t.Skip("cannot test unreadable file as root")
		}
		dir := t.TempDir()
		sb := state.ScrollbackDir(dir)
		if err := os.MkdirAll(sb, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		readable := filepath.Join(sb, "good__0.0.bin")
		if err := os.WriteFile(readable, []byte("readable"), 0o600); err != nil {
			t.Fatalf("write readable: %v", err)
		}
		unreadable := filepath.Join(sb, "bad__0.0.bin")
		if err := os.WriteFile(unreadable, []byte("nope"), 0o600); err != nil {
			t.Fatalf("write unreadable: %v", err)
		}
		if err := os.Chmod(unreadable, 0o000); err != nil {
			t.Fatalf("chmod 0o000: %v", err)
		}
		// Restore mode so t.TempDir cleanup can remove the file.
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

		logger, sink := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		// Readable file must still be hashed.
		want := xxhash.Sum64([]byte("readable"))
		if got, ok := hm["good__0.0"]; !ok || got != want {
			t.Errorf("hm[good__0.0] = (%d, %v), want (%d, true)", got, ok, want)
		}
		if _, ok := hm["bad__0.0"]; ok {
			t.Errorf("hm contains entry for unreadable file: %v", hm)
		}

		// Logger must have produced a warning mentioning the file (via the
		// path attr).
		log := sink.body()
		if !strings.Contains(log, "WARN") {
			t.Errorf("log does not contain WARN: %q", log)
		}
		if !strings.Contains(log, "bad__0.0.bin") {
			t.Errorf("log does not mention unreadable file: %q", log)
		}
	})

	t.Run("survives a nil logger", func(t *testing.T) {
		dir := t.TempDir()
		sb := state.ScrollbackDir(dir)
		if err := os.MkdirAll(sb, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sb, "work__0.0.bin"), []byte("data"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		// Must not panic with a nil logger (the Logger doc promises nil-safe methods).
		hm := state.SeedHashMap(dir, nil)

		if len(hm) != 1 {
			t.Errorf("hm has %d entries, want 1", len(hm))
		}
	})
}

func TestCaptureAndHashPane(t *testing.T) {
	t.Run("returns both bytes and hash from CaptureAndHashPane", func(t *testing.T) {
		raw := "abc\n  \x1b[31mred"
		c := captureFunc(func(target string) (string, error) {
			if target != "work:0.0" {
				t.Errorf("target = %q, want %q", target, "work:0.0")
			}
			return raw, nil
		})

		bytes, hash, err := state.CaptureAndHashPane(c, "work:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(bytes) != raw {
			t.Errorf("bytes = %q, want %q", string(bytes), raw)
		}
		if hash != xxhash.Sum64([]byte(raw)) {
			t.Errorf("hash = %d, want %d", hash, xxhash.Sum64([]byte(raw)))
		}
	})

	t.Run("returns the underlying error when capture fails", func(t *testing.T) {
		want := errors.New("can't find pane")
		c := captureFunc(func(target string) (string, error) { return "", want })

		bytes, hash, err := state.CaptureAndHashPane(c, "missing:0.0")
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want wraps %v", err, want)
		}
		if bytes != nil {
			t.Errorf("bytes = %q, want nil on error", string(bytes))
		}
		if hash != 0 {
			t.Errorf("hash = %d, want 0 on error", hash)
		}
	})
}

func TestWriteScrollbackIfChanged(t *testing.T) {
	t.Run("skips the write when the new hash matches the stored hash", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "work__0.0"
		data := []byte("unchanged")
		hash := xxhash.Sum64(data)
		hm := state.HashMap{paneKey: hash}

		wrote, err := state.WriteScrollbackIfChanged(dir, paneKey, data, hash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wrote {
			t.Error("wrote = true, want false (hash matched)")
		}
		if _, err := os.Stat(state.ScrollbackFile(dir, paneKey)); !os.IsNotExist(err) {
			t.Errorf("file was written despite identical hash; stat err = %v", err)
		}
	})

	t.Run("writes and updates the hash when content has changed", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "work__0.0"
		oldHash := xxhash.Sum64([]byte("old"))
		newData := []byte("new content")
		newHash := xxhash.Sum64(newData)
		hm := state.HashMap{paneKey: oldHash}

		wrote, err := state.WriteScrollbackIfChanged(dir, paneKey, newData, newHash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !wrote {
			t.Error("wrote = false, want true")
		}
		if hm[paneKey] != newHash {
			t.Errorf("hm[%s] = %d, want %d", paneKey, hm[paneKey], newHash)
		}
		got, err := os.ReadFile(state.ScrollbackFile(dir, paneKey))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != string(newData) {
			t.Errorf("file contents = %q, want %q", got, newData)
		}
	})

	t.Run("writes and inserts the hash when paneKey is absent", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "fresh__0.0"
		data := []byte("first capture")
		hash := xxhash.Sum64(data)
		hm := state.HashMap{}

		wrote, err := state.WriteScrollbackIfChanged(dir, paneKey, data, hash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !wrote {
			t.Error("wrote = false, want true (paneKey absent)")
		}
		if hm[paneKey] != hash {
			t.Errorf("hm[%s] = %d, want %d", paneKey, hm[paneKey], hash)
		}
	})

	t.Run("writes scrollback files with mode 0600", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "work__0.0"
		data := []byte("private bytes")
		hash := xxhash.Sum64(data)
		hm := state.HashMap{}

		_, err := state.WriteScrollbackIfChanged(dir, paneKey, data, hash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(state.ScrollbackFile(dir, paneKey))
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("scrollback file mode = %o, want 0o600", got)
		}
	})

	t.Run("writes a zero-byte file for empty scrollback on first capture", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "empty__0.0"
		data := []byte{}
		hash := xxhash.Sum64(data)
		hm := state.HashMap{}

		wrote, err := state.WriteScrollbackIfChanged(dir, paneKey, data, hash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !wrote {
			t.Error("wrote = false, want true (first capture even when empty)")
		}
		got, err := os.ReadFile(state.ScrollbackFile(dir, paneKey))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("file contents = %q, want zero bytes", got)
		}
	})

	t.Run("skips zero-byte writes on subsequent captures of identical empty scrollback", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		paneKey := "empty__0.0"
		data := []byte{}
		hash := xxhash.Sum64(data)
		hm := state.HashMap{paneKey: hash}

		wrote, err := state.WriteScrollbackIfChanged(dir, paneKey, data, hash, hm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wrote {
			t.Error("wrote = true on identical empty scrollback, want false")
		}
		if _, err := os.Stat(state.ScrollbackFile(dir, paneKey)); !os.IsNotExist(err) {
			t.Errorf("file was written; stat err = %v", err)
		}
	})

	t.Run("maintains independent hash entries per paneKey", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		hm := state.HashMap{}

		paneA := "work__0.0"
		dataA := []byte("alpha pane bytes")
		hashA := xxhash.Sum64(dataA)
		paneB := "work__0.1"
		dataB := []byte("beta pane bytes")
		hashB := xxhash.Sum64(dataB)

		if _, err := state.WriteScrollbackIfChanged(dir, paneA, dataA, hashA, hm); err != nil {
			t.Fatalf("write A: %v", err)
		}
		if _, err := state.WriteScrollbackIfChanged(dir, paneB, dataB, hashB, hm); err != nil {
			t.Fatalf("write B: %v", err)
		}
		if hm[paneA] != hashA {
			t.Errorf("hm[%s] = %d, want %d", paneA, hm[paneA], hashA)
		}
		if hm[paneB] != hashB {
			t.Errorf("hm[%s] = %d, want %d", paneB, hm[paneB], hashB)
		}

		// A second identical write to paneA must skip; an updated paneB must write.
		wrote, err := state.WriteScrollbackIfChanged(dir, paneA, dataA, hashA, hm)
		if err != nil {
			t.Fatalf("re-write A: %v", err)
		}
		if wrote {
			t.Error("re-write A wrote = true, want false")
		}
		dataB2 := []byte("beta updated")
		hashB2 := xxhash.Sum64(dataB2)
		wrote, err = state.WriteScrollbackIfChanged(dir, paneB, dataB2, hashB2, hm)
		if err != nil {
			t.Fatalf("update B: %v", err)
		}
		if !wrote {
			t.Error("update B wrote = false, want true")
		}
		// paneA hash unchanged; paneB hash updated.
		if hm[paneA] != hashA {
			t.Errorf("hm[%s] mutated to %d, want %d", paneA, hm[paneA], hashA)
		}
		if hm[paneB] != hashB2 {
			t.Errorf("hm[%s] = %d, want %d", paneB, hm[paneB], hashB2)
		}
	})

	t.Run("propagates AtomicWrite errors with paneKey context", func(t *testing.T) {
		// Force an AtomicWrite failure by pointing at a path whose parent
		// cannot be created (parent component is an existing regular file).
		root := t.TempDir()
		blocker := filepath.Join(root, "scrollback")
		if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		paneKey := "work__0.0"
		data := []byte("anything")
		hash := xxhash.Sum64(data)
		hm := state.HashMap{}

		_, err := state.WriteScrollbackIfChanged(root, paneKey, data, hash, hm)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), paneKey) {
			t.Errorf("error %q does not contain paneKey %q", err.Error(), paneKey)
		}
		// Hash must NOT be updated when the write fails.
		if _, ok := hm[paneKey]; ok {
			t.Errorf("hm updated despite write failure: %v", hm)
		}
	})
}
