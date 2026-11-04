package devversion

import (
	"runtime/debug"
	"testing"
	"time"
)

func buildInfo(settings ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	for i := 0; i+1 < len(settings); i += 2 {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return info
}

func atModuleVersion(version string, settings ...string) *debug.BuildInfo {
	info := buildInfo(settings...)
	info.Main.Version = version
	return info
}

func TestCompose(t *testing.T) {
	const rev = "754e31b498e0d53ade03fb715056f2ec87a05190"
	written := time.Date(2026, 11, 4, 0, 16, 32, 195584591, time.FixedZone("BST", 3600))

	for _, tc := range []struct {
		name    string
		info    *debug.BuildInfo
		written time.Time
		want    string
	}{
		{"a build saying nothing is dev", nil, time.Time{}, "dev"},
		{"a build with no VCS stamp is dev and when it was written", buildInfo("-compiler", "gc"), written, "dev-20261103T231632.195584591Z"},
		{"a clean tree's build carries its revision", buildInfo("vcs.revision", rev, "vcs.modified", "false"), written, "dev-754e31b498e0-20261103T231632.195584591Z"},
		{"a modified tree's build says so", buildInfo("vcs.revision", rev, "vcs.modified", "true"), written, "dev-754e31b498e0-modified-20261103T231632.195584591Z"},
		{"a short revision is kept whole", buildInfo("vcs.revision", "abc123"), time.Time{}, "dev-abc123"},
		{"a modified bit with no revision says nothing", buildInfo("vcs.modified", "true"), time.Time{}, "dev"},
		{"a module built at a release reports it, less its v", atModuleVersion("v1.4.0"), written, "1.4.0"},
		{"a module built at a pseudo-version reports it", atModuleVersion("v0.0.0-20261104001632-754e31b498e0"), written, "0.0.0-20261104001632-754e31b498e0"},
		{"a module version of (devel) is no release", atModuleVersion("(devel)"), written, "dev-20261103T231632.195584591Z"},
		{
			"a checkout's build is dev whatever module version it carries",
			atModuleVersion("v1.4.1-0.20261104001632-754e31b498e0+dirty", "vcs.revision", rev, "vcs.modified", "true"),
			written,
			"dev-754e31b498e0-modified-20261103T231632.195584591Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compose(tc.info, tc.written); got != tc.want {
				t.Errorf("compose = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("two builds of one modified tree differ by when each was written", func(t *testing.T) {
		info := buildInfo("vcs.revision", rev, "vcs.modified", "true")

		if compose(info, written) == compose(info, written.Add(time.Nanosecond)) {
			t.Error("two rebuilds of the same modified revision report one version")
		}
	})
}
