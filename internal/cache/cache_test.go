package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyDeterministic(t *testing.T) {
	files := map[string]string{"a.go": "aaa", "b.go": "bbb"}
	k1 := Key("go vet ./...", "", nil, files)
	k2 := Key("go vet ./...", "", nil, map[string]string{"b.go": "bbb", "a.go": "aaa"})
	if k1 != k2 {
		t.Error("key depends on map order")
	}
	if k1 == Key("go vet ./... ", "", nil, files) {
		t.Error("command change did not invalidate the key")
	}
	if k1 == Key("go vet ./...", "", nil, map[string]string{"a.go": "aaa", "b.go": "ccc"}) {
		t.Error("content change did not invalidate the key")
	}
	if k1 == Key("go vet ./...", "", map[string]string{"X": "1"}, files) {
		t.Error("env change did not invalidate the key")
	}
	if len(k1) != 64 {
		t.Errorf("expected sha256 hex, got %q", k1)
	}
}

func TestStoreLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "pre-push", "vet", "abc123"); err != nil {
		t.Fatal(err)
	}
	hash, ok := Load(dir, "pre-push", "vet")
	if !ok || hash != "abc123" {
		t.Errorf("got %q %v, want abc123 true", hash, ok)
	}
}

func TestLoadMissingAndCorruptIsMiss(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Load(dir, "pre-push", "nope"); ok {
		t.Error("missing entry must miss")
	}
	p := entryPath(dir, "pre-push", "vet")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(dir, "pre-push", "vet"); ok {
		t.Error("corrupt entry must miss")
	}

	// Hostile step names must stay inside the cache dir.
	evil := entryPath(dir, "pre-push", "../../escape")
	rel, err := filepath.Rel(dir, evil)
	if err != nil || len(rel) < 3 || rel[:3] == ".."+string(filepath.Separator) {
		t.Errorf("step name escaped cache dir: %q", evil)
	}
}
