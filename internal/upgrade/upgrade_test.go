// Package upgrade self-updates the binary from GitHub releases.
package upgrade

import (
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		have, latest string
		want         int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"0.1.0", "v0.1.1", -1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "v2.0.0", -1},
	} {
		t.Run(tc.have+"_vs_"+tc.latest, func(t *testing.T) {
			if got := compareVersions(tc.have, tc.latest); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAssetName(t *testing.T) {
	got, err := assetName("v0.1.0", "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "pretty-hooks_v0.1.0_darwin_arm64.tar.gz" {
		t.Errorf("got %q", got)
	}
	if _, err := assetName("v0.1.0", "windows", "amd64"); err == nil {
		t.Error("windows must be unsupported")
	}
	if _, err := assetName("v0.1.0", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Errorf("current platform must map: %v", err)
	}
}

func TestAssetNamesCoverBothConventions(t *testing.T) {
	names, err := assetNames("v0.2.0", "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pretty-hooks_v0.2.0_darwin_arm64.tar.gz",
		"pretty-hooks_0.2.0_darwin_arm64.tar.gz",
	}
	if len(names) != len(want) {
		t.Fatalf("got %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("got %v, want %v", names, want)
		}
	}
}
