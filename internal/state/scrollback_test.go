package state_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

type captureFunc func(target string) (string, error)

func (f captureFunc) CapturePane(target string) (string, error) { return f(target) }

func openTempLogger(t *testing.T) (*slog.Logger, *logtest.Sink) {
	t.Helper()
	return logtest.NewCaptureLogger(t)
}

func TestSeedHashMap(t *testing.T) {
	t.Run("returns empty HashMap when scrollback directory is missing", func(t *testing.T) {
		dir := t.TempDir()
		// Deliberately no EnsureDir: the scrollback subdir must be absent.
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
		// chmod 0o000 cannot make a file unreadable for root.
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
		// Restore mode so t.TempDir's cleanup can remove the file.
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

		logger, sink := openTempLogger(t)

		hm := state.SeedHashMap(dir, logger)

		want := xxhash.Sum64([]byte("readable"))
		if got, ok := hm["good__0.0"]; !ok || got != want {
			t.Errorf("hm[good__0.0] = (%d, %v), want (%d, true)", got, ok, want)
		}
		if _, ok := hm["bad__0.0"]; ok {
			t.Errorf("hm contains entry for unreadable file: %v", hm)
		}

		log := sink.Body()
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
		if hm[paneA] != hashA {
			t.Errorf("hm[%s] mutated to %d, want %d", paneA, hm[paneA], hashA)
		}
		if hm[paneB] != hashB2 {
			t.Errorf("hm[%s] = %d, want %d", paneB, hm[paneB], hashB2)
		}
	})

	t.Run("propagates AtomicWrite errors with paneKey context", func(t *testing.T) {
		// An existing regular file as a parent component makes AtomicWrite fail.
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
		if _, ok := hm[paneKey]; ok {
			t.Errorf("hm updated despite write failure: %v", hm)
		}
	})
}

const waitingPaneToken = "ab12cd"

func waitingIndex(token, scrollbackFile string) state.Index {
	return state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{{
			Name:        "work",
			Environment: map[string]string{},
			Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "tiled", Active: true,
				Panes: []state.Pane{{
					Index:          1,
					CWD:            "/tmp",
					CurrentCommand: "zsh",
					ScrollbackFile: scrollbackFile,
					PortalPaneID:   token,
				}},
			}},
		}},
	}
}

func waitingPaneOf(t *testing.T, idx state.Index) state.Pane {
	t.Helper()
	return idx.Sessions[0].Windows[0].Panes[0]
}

func waitingSet() map[string]struct{} {
	return map[string]struct{}{state.SanitizePaneKey("work", 0, 1): {}}
}

func seedScrollback(t *testing.T, dir, name, body string) string {
	t.Helper()
	sb := state.ScrollbackDir(dir)
	if err := os.MkdirAll(sb, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(sb, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func readScrollback(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state.ScrollbackDir(dir), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestRefilePendingScrollback(t *testing.T) {
	t.Run("it re-files a waiting pane's scrollback under its token and points the record at that path", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		idx := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		hm := state.HashMap{"work__0.1": 42}
		logger, sink := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, waitingSet(), hm, logger)

		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, idx).ScrollbackFile; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "frozen-body" {
			t.Errorf("token-named file = %q, want %q", got, "frozen-body")
		}
		if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "work__0.1.bin")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("positional file stat err = %v, want not-exist", err)
		}
		if _, held := hm["work__0.1"]; held {
			t.Errorf("hash map still holds the vacated key: %v", hm)
		}
		if got := sink.Records(); len(got) != 0 {
			t.Errorf("records on the success path = %v, want none", sink.Lines())
		}
	})

	t.Run("it renames nothing on a second tick for a pane already filed under its token", func(t *testing.T) {
		dir := t.TempDir()
		tokenName := "pane-" + waitingPaneToken + ".bin"
		seedScrollback(t, dir, tokenName, "frozen-body")
		idx := waitingIndex(waitingPaneToken, "scrollback/"+tokenName)
		hm := state.HashMap{"pane-" + waitingPaneToken: 42}
		logger, sink := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, waitingSet(), hm, logger)

		if got, want := waitingPaneOf(t, idx).ScrollbackFile, "scrollback/"+tokenName; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, tokenName); got != "frozen-body" {
			t.Errorf("token-named file = %q, want %q", got, "frozen-body")
		}
		if got, held := hm["pane-"+waitingPaneToken]; !held || got != 42 {
			t.Errorf("hm[pane-%s] = (%d, %v), want (42, true)", waitingPaneToken, got, held)
		}
		if got := sink.Records(); len(got) != 0 {
			t.Errorf("records = %v, want none", sink.Lines())
		}
	})

	t.Run("it adopts the token path when the record still names a positional file that is gone", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		idx := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		hm := state.HashMap{"work__0.1": 42}
		logger, sink := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, waitingSet(), hm, logger)

		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, idx).ScrollbackFile; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if _, held := hm["work__0.1"]; held {
			t.Errorf("hash map still holds the vacated key: %v", hm)
		}
		if got := sink.Records(); len(got) != 0 {
			t.Errorf("records = %v, want none", sink.Lines())
		}
	})

	t.Run("it keeps the positional path for a waiting pane whose token is absent or not token-shaped", func(t *testing.T) {
		tokens := map[string]string{
			"absent":           "",
			"too short":        "ab12",
			"too long":         "ab12cdef",
			"outside alphabet": "ab-2cd",
		}
		for name, token := range tokens {
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
				idx := waitingIndex(token, "scrollback/work__0.1.bin")
				hm := state.HashMap{"work__0.1": 42}
				logger, sink := openTempLogger(t)

				state.RefilePendingScrollback(dir, &idx, waitingSet(), hm, logger)

				if got, want := waitingPaneOf(t, idx).ScrollbackFile, "scrollback/work__0.1.bin"; got != want {
					t.Errorf("ScrollbackFile = %q, want %q", got, want)
				}
				if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
					t.Errorf("positional file = %q, want %q", got, "frozen-body")
				}
				if got, held := hm["work__0.1"]; !held || got != 42 {
					t.Errorf("hm[work__0.1] = (%d, %v), want (42, true)", got, held)
				}
				if got := sink.Records(); len(got) != 0 {
					t.Errorf("records = %v, want none", sink.Lines())
				}
			})
		}
	})

	t.Run("it warns once and leaves the record and the dedup entry alone when the rename fails", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		blocked := filepath.Join(state.ScrollbackDir(dir), "pane-"+waitingPaneToken+".bin")
		if err := os.MkdirAll(filepath.Join(blocked, "occupant"), 0o700); err != nil {
			t.Fatalf("seed blocking directory: %v", err)
		}
		idx := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		hm := state.HashMap{"work__0.1": 42}
		logger, sink := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, waitingSet(), hm, logger)

		if got, want := waitingPaneOf(t, idx).ScrollbackFile, "scrollback/work__0.1.bin"; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "frozen-body" {
			t.Errorf("positional file = %q, want %q", got, "frozen-body")
		}
		if got, held := hm["work__0.1"]; !held || got != 42 {
			t.Errorf("hm[work__0.1] = (%d, %v), want (42, true)", got, held)
		}
		rec := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "refile failure warning")
		if got, want := rec.AttrOrEmpty("pane_key"), "work__0.1"; got != want {
			t.Errorf("pane_key = %q, want %q", got, want)
		}
		if got, want := rec.AttrOrEmpty("path"), "scrollback/work__0.1.bin"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if !rec.HasAttr("error") {
			t.Errorf("warning carries no error attr: %v", rec.Keys)
		}
		if got, want := len(sink.Records()), 1; got != want {
			t.Errorf("records = %d, want %d: %v", got, want, sink.Lines())
		}
		for _, key := range rec.Keys {
			switch key {
			case "pane_key", "path", "error":
			default:
				t.Errorf("warning carries unexpected attr key %q", key)
			}
		}
	})

	t.Run("it tolerates a nil hash map and a nil logger", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "frozen-body")
		idx := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")

		state.RefilePendingScrollback(dir, &idx, waitingSet(), nil, nil)

		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, idx).ScrollbackFile; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, "pane-"+waitingPaneToken+".bin"); got != "frozen-body" {
			t.Errorf("token-named file = %q, want %q", got, "frozen-body")
		}
	})

	t.Run("it renames nothing when the record names no scrollback file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		idx := waitingIndex(waitingPaneToken, "")
		logger, sink := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, waitingSet(), state.HashMap{}, logger)

		want := "scrollback/pane-" + waitingPaneToken + ".bin"
		if got := waitingPaneOf(t, idx).ScrollbackFile; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("state dir stat: %v", err)
		}
		if got := sink.Records(); len(got) != 0 {
			t.Errorf("records = %v, want none", sink.Lines())
		}
	})

	t.Run("it leaves a pane the live enumeration did not mark pending alone", func(t *testing.T) {
		dir := t.TempDir()
		seedScrollback(t, dir, "work__0.1.bin", "live-body")
		idx := waitingIndex(waitingPaneToken, "scrollback/work__0.1.bin")
		hm := state.HashMap{"work__0.1": 42}
		logger, _ := openTempLogger(t)

		state.RefilePendingScrollback(dir, &idx, map[string]struct{}{}, hm, logger)

		if got, want := waitingPaneOf(t, idx).ScrollbackFile, "scrollback/work__0.1.bin"; got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
		if got := readScrollback(t, dir, "work__0.1.bin"); got != "live-body" {
			t.Errorf("positional file = %q, want %q", got, "live-body")
		}
	})
}
