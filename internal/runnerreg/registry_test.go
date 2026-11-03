package runnerreg_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/runnerreg"
)

func TestRegister(t *testing.T) {
	t.Run("it lists the runner under its token: its pid and session in an entry, its socket beside it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if g.Entry() != r.EntryPath("tok123") || g.Socket() != r.SocketPath("tok123") || filepath.Dir(g.Socket()) != r.Dir() {
			t.Errorf("entry %s, socket %s; want tok123.json and tok123.sock in %s", g.Entry(), g.Socket(), r.Dir())
		}
		data, err := os.ReadFile(g.Entry())
		if err != nil {
			t.Fatalf("read the entry: %v", err)
		}
		var e struct {
			PID     int    `json:"pid"`
			Session string `json:"session"`
		}
		if err := json.Unmarshal(data, &e); err != nil || e.PID != os.Getpid() || e.Session != "api-x7Kd9a" {
			t.Errorf("entry = %s, %v; want this pid and session api-x7Kd9a", data, err)
		}
		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Errorf("a switch through the socket = %v, want it served", err)
		}
	})

	t.Run("only this user can reach it: the directory 0700, the socket 0600", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := os.MkdirAll(r.Dir(), 0o755); err != nil {
			t.Fatal(err)
		}
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		dir, err := os.Stat(r.Dir())
		if err != nil || dir.Mode().Perm() != 0o700 {
			t.Errorf("directory mode = %v, %v; want 0700, even made wider before", dir.Mode().Perm(), err)
		}
		socket, err := os.Stat(g.Socket())
		if err != nil || socket.Mode()&fs.ModeSocket == 0 || socket.Mode().Perm() != 0o600 {
			t.Errorf("socket mode = %v, %v; want a socket, 0600", socket.Mode(), err)
		}
		entry, err := os.Stat(g.Entry())
		if err != nil || entry.Mode().Perm() != 0o600 {
			t.Errorf("entry mode = %v, %v; want 0600", entry.Mode().Perm(), err)
		}
	})

	t.Run("SetSession moves the entry to the session switched to", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if err := g.SetSession("blog-c3d4"); err != nil {
			t.Fatalf("SetSession: %v", err)
		}

		if data, err := os.ReadFile(g.Entry()); err != nil || !strings.Contains(string(data), `"session":"blog-c3d4"`) {
			t.Errorf("entry = %s, %v; want it on blog-c3d4", data, err)
		}
	})

	t.Run("Close removes the entry and the socket, and may be called again", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		for range 2 {
			if err := g.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}

		if names := files(t, r.Dir()); len(names) != 0 {
			t.Errorf("the registry holds %q after Close, want nothing", names)
		}
		if err := g.SetSession("blog-c3d4"); err == nil {
			t.Error("SetSession after Close = nil, want a refusal")
		}
	})

	t.Run("a token no runner could have is refused, making nothing", func(t *testing.T) {
		for _, token := range []string{"", "../tok", "a/b", "tok.12", "tók123"} {
			r := runnerreg.New(stateDir(t))

			if g, err := r.Register(token, "api-x7Kd9a", &handler{}); err == nil {
				_ = g.Close()
				t.Errorf("Register(%q) = nil, want a refusal", token)
			}
			if exists(t, r.Dir()) {
				t.Errorf("Register(%q) made %s", token, r.Dir())
			}
		}
	})
}

func TestRegister_SocketPathLength(t *testing.T) {
	// Every socket path is <dir>/<token>.sock, the token six characters.
	const pathBeyondDir = len("/tok123.sock")
	stateDirWithSocketPath := func(t *testing.T, length int) string {
		t.Helper()
		dir := stateDir(t)
		pad := length - pathBeyondDir - len(dir) - len("/runners") - 1
		if pad < 1 {
			t.Fatalf("the state dir %s is too long to make a socket path of %d bytes", dir, length)
		}
		state := filepath.Join(dir, strings.Repeat("s", pad))
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
		return state
	}

	t.Run("a path filling a socket address binds whole", func(t *testing.T) {
		r := runnerreg.New(stateDirWithSocketPath(t, runnerreg.MaxSocketPath()))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if len(g.Socket()) != runnerreg.MaxSocketPath() {
			t.Fatalf("socket path is %d bytes, want %d; the case is unarmed", len(g.Socket()), runnerreg.MaxSocketPath())
		}
		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Errorf("a switch through the socket = %v, want it served", err)
		}
	})

	t.Run("a path one byte longer is refused, never cut short, and makes nothing", func(t *testing.T) {
		state := stateDirWithSocketPath(t, runnerreg.MaxSocketPath()+1)
		r := runnerreg.New(state)

		g, err := r.Register("tok123", "api-x7Kd9a", &handler{})

		if !errors.Is(err, runnerreg.ErrSocketPathTooLong) {
			t.Fatalf("Register = %v, %v; want ErrSocketPathTooLong", g, err)
		}
		for _, want := range []string{r.Dir(), strconv.Itoa(runnerreg.MaxSocketPath()+1) + " bytes", strconv.Itoa(runnerreg.MaxSocketPath())} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not say %q", err, want)
			}
		}
		if exists(t, r.Dir()) {
			t.Errorf("the refused registration made %s", r.Dir())
		}
	})
}
