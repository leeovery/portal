package spawn

import "testing"

func TestNewIdentity(t *testing.T) {
	tests := []struct {
		name         string
		bundleID     string
		appName      string
		wantNull     bool
		wantBundleID string
		wantName     string
	}{
		{
			name:         "it builds a passthrough identity from a channel-suffixed bundle id with a derived name",
			bundleID:     "dev.warp.Warp-Stable",
			appName:      "",
			wantNull:     false,
			wantBundleID: "dev.warp.Warp-Stable",
			wantName:     "Warp",
		},
		{
			name:         "it returns a NULL identity for an empty bundle id even with an app name",
			bundleID:     "",
			appName:      "Ghostty",
			wantNull:     true,
			wantBundleID: "",
			wantName:     "",
		},
		{
			name:         "it returns a NULL identity for a whitespace-only bundle id",
			bundleID:     "   ",
			appName:      "",
			wantNull:     true,
			wantBundleID: "",
			wantName:     "",
		},
		{
			name:         "it keeps an unknown bundle id as a passthrough identity, never NULL",
			bundleID:     "com.example.MyTerm",
			appName:      "",
			wantNull:     false,
			wantBundleID: "com.example.MyTerm",
			wantName:     "MyTerm",
		},
		{
			name:         "it prefers a supplied app name over the derived name",
			bundleID:     "com.mitchellh.ghostty",
			appName:      "Ghostty",
			wantNull:     false,
			wantBundleID: "com.mitchellh.ghostty",
			wantName:     "Ghostty",
		},
		{
			name:         "it derives Terminal from the apple terminal bundle id",
			bundleID:     "com.apple.Terminal",
			appName:      "",
			wantNull:     false,
			wantBundleID: "com.apple.Terminal",
			wantName:     "Terminal",
		},
		{
			name:         "it trims the bundle id before stamping it on the identity",
			bundleID:     "  com.apple.Terminal  ",
			appName:      "",
			wantNull:     false,
			wantBundleID: "com.apple.Terminal",
			wantName:     "Terminal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIdentity(tt.bundleID, tt.appName)
			if got.IsNull() != tt.wantNull {
				t.Errorf("NewIdentity(%q, %q).IsNull() = %v, want %v", tt.bundleID, tt.appName, got.IsNull(), tt.wantNull)
			}
			if got.BundleID != tt.wantBundleID {
				t.Errorf("NewIdentity(%q, %q).BundleID = %q, want %q", tt.bundleID, tt.appName, got.BundleID, tt.wantBundleID)
			}
			if got.Name != tt.wantName {
				t.Errorf("NewIdentity(%q, %q).Name = %q, want %q", tt.bundleID, tt.appName, got.Name, tt.wantName)
			}
		})
	}
}

func TestNewIdentity_NeverEmptyNameForNonEmptyBundleID(t *testing.T) {
	// A bundle id whose last segment collapses to empty once the channel
	// suffix is trimmed must still yield a non-empty, human-readable Name.
	got := NewIdentity("com.example.-Stable", "")
	if got.IsNull() {
		t.Fatalf("NewIdentity(%q, \"\") is NULL, want passthrough", "com.example.-Stable")
	}
	if got.Name == "" {
		t.Errorf("NewIdentity(%q, \"\").Name is empty, want a non-empty derived name", "com.example.-Stable")
	}
}

func TestMatchesFamily(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		pattern  string
		want     bool
	}{
		{
			name:     "it matches a channel-suffixed bundle id against its family glob",
			bundleID: "dev.warp.Warp-Stable",
			pattern:  "dev.warp.Warp-*",
			want:     true,
		},
		{
			name:     "it matches an exact bundle id against its literal pattern",
			bundleID: "com.apple.Terminal",
			pattern:  "com.apple.Terminal",
			want:     true,
		},
		{
			name:     "it rejects a channel-suffixed id against an exact literal pattern",
			bundleID: "com.apple.Terminal-Beta",
			pattern:  "com.apple.Terminal",
			want:     false,
		},
		{
			name:     "it matches any bundle id against a bare star catch-all",
			bundleID: "anything",
			pattern:  "*",
			want:     true,
		},
		{
			name:     "it rejects a bundle id that belongs to a different family",
			bundleID: "com.apple.Terminal",
			pattern:  "dev.warp.Warp-*",
			want:     false,
		},
		{
			name:     "it treats a malformed glob pattern as a non-match, not a failure",
			bundleID: "com.apple.Terminal",
			pattern:  "[", // path.ErrBadPattern
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesFamily(tt.bundleID, tt.pattern)
			if got != tt.want {
				t.Errorf("MatchesFamily(%q, %q) = %v, want %v", tt.bundleID, tt.pattern, got, tt.want)
			}
		})
	}
}
