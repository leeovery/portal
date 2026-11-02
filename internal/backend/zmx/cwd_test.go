package zmx

import "testing"

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
