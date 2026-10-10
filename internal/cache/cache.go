// Package cache skips steps whose inputs are unchanged since the last
// passing run. Entries live under .git/ph-cache and never get committed.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// entry is one stored result.
type entry struct {
	Hash string `json:"hash"`
	At   string `json:"at"`
}

// Key hashes the command, workdir, env, and file contents into a
// deterministic cache key. files maps repo-relative path to content hash.
func Key(run, workdir string, env map[string]string, files map[string]string) string {
	h := sha256.New()
	fmt.Fprintf(h, "run\x00%s\x00workdir\x00%s\x00", run, workdir)
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "env\x00%s=%s\x00", k, env[k])
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(h, "file\x00%s=%s\x00", n, files[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// entryPath resolves the storage file. Step and hook names are sanitized so
// hostile names cannot escape the cache directory.
func entryPath(root, hook, step string) string {
	clean := func(s string) string {
		s = strings.ReplaceAll(s, string(filepath.Separator), "_")
		s = strings.ReplaceAll(s, "..", "_")
		return s
	}
	return filepath.Join(root, ".git", "ph-cache", clean(hook), clean(step)+".json")
}

// Store records a passing hash. Writes are atomic.
func Store(root, hook, step, hash string) error {
	p := entryPath(root, hook, step)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry{Hash: hash, At: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Load returns the stored hash. Missing or corrupt entries miss.
func Load(root, hook, step string) (string, bool) {
	data, err := os.ReadFile(entryPath(root, hook, step))
	if err != nil {
		return "", false
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil || e.Hash == "" {
		return "", false
	}
	return e.Hash, true
}

// HashFile returns the sha256 hex of a file's content.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
