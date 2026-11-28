package zmx

import (
	"strings"
	"testing"
)

func TestOSC7ReadsBackAsItsDirectory(t *testing.T) {
	for _, dir := range []string{"/", "/srv/app", "/srv/a dir·x", "/tmp/100%", "/srv/a;b#c?d", "/srv/tab\there", "/srv/\x1b\\x", "/srv/a:b@c~d", "//srv"} {
		t.Run(dir, func(t *testing.T) {
			uri, ok := oscURI(osc7("lees-mac.local", dir))
			if !ok {
				t.Fatalf("osc7(%q) = %q, want one OSC 7 with no control byte inside", dir, osc7("lees-mac.local", dir))
			}
			if got := localDir(uri, "lees-mac.local"); got != dir {
				t.Errorf("localDir(osc7(%q)) = %q, want it back", dir, got)
			}
		})
	}
}

func FuzzOSC7ReadsBackAsItsDirectory(f *testing.F) {
	f.Add("srv/a dir·x/100%")
	f.Fuzz(func(t *testing.T, path string) {
		dir := "/" + path
		uri, ok := oscURI(osc7("host", dir))
		if !ok {
			t.Fatalf("osc7(%q) = %q, want one OSC 7 with no control byte inside", dir, osc7("host", dir))
		}
		if got := localDir(uri, "host"); got != dir {
			t.Errorf("localDir(osc7(%q)) = %q, want it back", dir, got)
		}
	})
}

// oscURI is the URI an OSC 7 carries, and whether osc is one OSC 7 whose URI
// holds no control byte to end it early.
func oscURI(osc []byte) (string, bool) {
	uri, opened := strings.CutPrefix(string(osc), "\x1b]7;")
	uri, closed := strings.CutSuffix(uri, "\x1b\\")
	return uri, opened && closed && !strings.ContainsFunc(uri, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

func TestLocalDir(t *testing.T) {
	const host = "lees-mac.local"
	for _, tt := range []struct {
		name string
		cwd  string
		want string
	}{
		{"a plain path", "/srv/app", "/srv/app"},
		{"a file URI on this host, as zmx writes it", "file://lees-mac.local/srv/my%20app", "/srv/my app"},
		{"a file URI with no host", "file:///srv/app", "/srv/app"},
		{"a file URI on localhost", "file://localhost/srv/app", "/srv/app"},
		{"the host's short name where gethostname gives the full one", "file://lees-mac/srv/app", "/srv/app"},
		{"the host's name in another case", "file://LEES-MAC.LOCAL/srv/app", "/srv/app"},
		{"the host's full name under another domain", "file://lees-mac.lan/srv/app", "/srv/app"},
		{"kitty's scheme", "kitty-shell-cwd://lees-mac.local/srv/app", "/srv/app"},
		{"a percent-encoded percent", "file://lees-mac.local/tmp/100%25", "/tmp/100%"},
		{"another host", "file://build-box/srv/app", ""},
		{"another host sharing a prefix", "file://lees-mac-2.local/srv/app", ""},
		{"an unsupported scheme", "http://lees-mac.local/srv/app", ""},
		{"a URI with no path", "file://lees-mac.local", ""},
		{"a relative path", "srv/app", ""},
		{"nothing", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := localDir(tt.cwd, host); got != tt.want {
				t.Errorf("localDir(%q, %q) = %q, want %q", tt.cwd, host, got, tt.want)
			}
		})
	}

	t.Run("it takes only a host-less or localhost cwd as local when the hostname is unknown", func(t *testing.T) {
		for cwd, want := range map[string]string{
			"file:///srv/app":          "/srv/app",
			"file://localhost/srv/app": "/srv/app",
			"file://lees-mac/srv/app":  "",
		} {
			if got := localDir(cwd, ""); got != want {
				t.Errorf("localDir(%q, \"\") = %q, want %q", cwd, got, want)
			}
		}
	})
}
