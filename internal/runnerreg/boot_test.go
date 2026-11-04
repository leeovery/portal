package runnerreg_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

// inBoot has the registry read id as the current boot's for the rest of t.
func inBoot(t *testing.T, id string) {
	runnerreg.SetBootID(t, func() (string, error) { return id, nil })
}

// bootUnknown has the registry fail to read the current boot's id for the
// rest of t.
func bootUnknown(t *testing.T) {
	runnerreg.SetBootID(t, func() (string, error) { return "", errors.New("read boot id: unsupported") })
}

// entryBoot is the boot id the entry at path names, empty for none.
func entryBoot(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the entry: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("decode the entry %s: %v", data, err)
	}
	id, ok := fields["boot_id"].(string)
	return id, ok
}

func TestLiveness_AnEntryFromAnotherBootIsStale(t *testing.T) {
	for reason, body := range map[string]string{
		"another boot": entryIn(os.Getpid(), "api-x7Kd9a", "boot-a"),
		"no boot id":   entryIn(os.Getpid(), "api-x7Kd9a", ""),
	} {
		t.Run(reason, func(t *testing.T) {
			inBoot(t, "boot-b")
			sink := logtest.Install(t)
			r := runnerreg.New(stateDir(t))
			current := &handler{}
			register(t, r, "kept01", "api-x7Kd9a", current)
			// The stale entry names a running pid, and an answer waits
			// behind its socket, as when a reboot hands its pid on.
			writeEntry(t, r, "old001", body)
			requests := make(chan string, 1)
			answeringSocket(t, r.SocketPath("old001"), ourAnswer(`"ok":true`), requests)

			accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

			if err != nil || accepted != 1 {
				t.Errorf("DetachToPicker = %d, %v; want this boot's runner alone", accepted, err)
			}
			if _, detaches := current.asked(); detaches != 1 {
				t.Errorf("this boot's runner detached %d times, want once", detaches)
			}
			select {
			case req := <-requests:
				t.Errorf("the stale runner was asked %s", req)
			default:
			}
			if exists(t, r.EntryPath("old001")) || exists(t, r.SocketPath("old001")) {
				t.Error("the stale runner's entry or socket was left")
			}
			rec := sink.Records().Matching("attach", "stale runner").AtExactLevel(slog.LevelDebug).Only(t, "stale runner record")
			if got := rec.AttrString(t, "reason"); got != reason {
				t.Errorf("stale runner reason = %q, want %q", got, reason)
			}
			if got := rec.AttrString(t, "path"); got != r.EntryPath("old001") {
				t.Errorf("stale runner path = %q, want the stale entry", got)
			}
		})
	}
}

func TestLiveness_AnUnreadableBootJudgesByPidAlone(t *testing.T) {
	bootUnknown(t)
	sink := logtest.Install(t)
	r := runnerreg.New(stateDir(t))
	for token, boot := range map[string]string{"other1": "boot-a", "none01": ""} {
		writeEntry(t, r, token, entryIn(os.Getpid(), "api-x7Kd9a", boot))
		answeringSocket(t, r.SocketPath(token), ourAnswer(`"ok":true`), nil)
	}
	writeEntry(t, r, "gone01", entryIn(deadPID(t), "api-x7Kd9a", "boot-a"))

	accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

	if err != nil || accepted != 2 {
		t.Errorf("DetachToPicker = %d, %v; want both runners whose pids are running", accepted, err)
	}
	rec := sink.Records().Matching("attach", "stale runner").Only(t, "stale runner record")
	if got := rec.AttrString(t, "reason"); got != "dead" || rec.AttrString(t, "path") != r.EntryPath("gone01") {
		t.Errorf("stale runner = %+v, want the dead runner's entry, reason dead", rec)
	}
	sink.Records().Matching("attach", "boot id unknown").AtExactLevel(slog.LevelDebug).Only(t, "one boot id record for the scan")
}

func TestRegister_RecordsTheBoot(t *testing.T) {
	t.Run("an entry names the boot its runner registered in", func(t *testing.T) {
		inBoot(t, "boot-a")
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if id, _ := entryBoot(t, g.Entry()); id != "boot-a" {
			t.Errorf("the entry's boot_id = %q, want boot-a", id)
		}
		if err := g.SetSession("blog-c3d4"); err != nil {
			t.Fatalf("SetSession: %v", err)
		}
		if id, _ := entryBoot(t, g.Entry()); id != "boot-a" {
			t.Errorf("the moved entry's boot_id = %q, want boot-a", id)
		}
	})

	t.Run("a runner that can't read its boot names none", func(t *testing.T) {
		bootUnknown(t)
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if id, ok := entryBoot(t, g.Entry()); ok {
			t.Errorf("the entry's boot_id = %q, want none", id)
		}
	})
}

func TestRegister_AnEntryLandsThoughTheDirectorySyncFails(t *testing.T) {
	fileutil.FailDirSyncForTest(t, syscall.EIO)
	sink := logtest.Install(t)
	r := runnerreg.New(stateDir(t))
	g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

	if err := g.SetSession("blog-c3d4"); err != nil {
		t.Fatalf("SetSession = %v, want the landed write to succeed", err)
	}

	if data, err := os.ReadFile(g.Entry()); err != nil || !json.Valid(data) {
		t.Fatalf("the entry = %s, %v; want it written", data, err)
	}
	recs := sink.Records().Matching("attach", "unsynced")
	if len(recs) != 2 {
		t.Fatalf("unsynced records = %d, want one for the register and one for the move", len(recs))
	}
	for _, rec := range recs {
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-fsync" {
			t.Errorf("error_class = %q, want write-failed-fsync", got)
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, syscall.EIO) {
			t.Errorf("error = %v, want the sync's EIO", err)
		}
		if got := rec.AttrString(t, "path"); got != g.Entry() {
			t.Errorf("path = %q, want the entry", got)
		}
	}
}
