// Package upgrade self-updates the binary from GitHub releases.
// It refuses dev builds, verifies nothing beyond HTTPS transport, and
// always keeps a .bak copy until the new binary is in place.
package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	repo        = "RndmCodeGuy20/pretty-hooks"
	apiLatest   = "https://api.github.com/repos/" + repo + "/releases/latest"
	httpTimeout = 30 * time.Second
)

// release is the subset of the GitHub release API used here.
type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Upgrade fetches the latest release and replaces the running binary.
// It returns a human-readable outcome ("already current" or the new tag).
func Upgrade(current string) (string, error) {
	if current == "dev" || current == "" {
		return "", fmt.Errorf("cannot upgrade a dev build (version is %q)", current)
	}
	rel, err := latestRelease()
	if err != nil {
		return "", err
	}
	if compareVersions(current, rel.TagName) >= 0 {
		return fmt.Sprintf("already current (%s)", current), nil
	}
	name, err := assetName(rel.TagName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	url := ""
	for _, a := range rel.Assets {
		if a.Name == name {
			url = a.URL
		}
	}
	if url == "" {
		return "", fmt.Errorf("release %s has no asset %q", rel.TagName, name)
	}
	exe, err := executable()
	if err != nil {
		return "", err
	}
	if err := installFromURL(url, exe); err != nil {
		return "", err
	}
	return fmt.Sprintf("upgraded %s to %s", current, rel.TagName), nil
}

// latestRelease queries the GitHub releases API.
func latestRelease() (*release, error) {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(apiLatest)
	if err != nil {
		return nil, fmt.Errorf("query releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("query releases: HTTP %d", resp.StatusCode)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release has no tag")
	}
	return &rel, nil
}

// assetName maps a tag and platform to the goreleaser archive name.
func assetName(tag, goos, goarch string) (string, error) {
	var osName string
	switch goos {
	case "darwin":
		osName = "Darwin"
	case "linux":
		osName = "Linux"
	default:
		return "", fmt.Errorf("unsupported OS %q", goos)
	}
	return fmt.Sprintf("pretty-hooks_%s_%s_%s.tar.gz", tag, osName, goarch), nil
}

// compareVersions compares dotted versions with optional leading v.
// It returns -1, 0, or 1.
func compareVersions(have, latest string) int {
	nums := func(s string) []int {
		s = strings.TrimPrefix(s, "v")
		var out []int
		for _, p := range strings.Split(s, ".") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	a, b := nums(have), nums(latest)
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// executable resolves the running binary, following symlinks so an
// aliased install (ph) still replaces the real file.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// installFromURL downloads the archive, extracts the pretty-hooks binary,
// and swaps it into exe with .bak rollback.
func installFromURL(url, exe string) error {
	dir, err := os.MkdirTemp("", "ph-upgrade")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	archive := filepath.Join(dir, "rel.tar.gz")
	if err := download(url, archive); err != nil {
		return err
	}
	bin, err := extractBinary(archive, dir)
	if err != nil {
		return err
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		return err
	}
	bak := exe + ".bak"
	os.Remove(bak)
	if err := os.Rename(exe, bak); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(bin, exe); err != nil {
		_ = os.Rename(bak, exe)
		return fmt.Errorf("install new binary: %w", err)
	}
	_ = os.Remove(bak)
	return nil
}

// download fetches url into dest.
func download(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractBinary unpacks the archive and returns the pretty-hooks path.
func extractBinary(archive, dir string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("unpack: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("unpack: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "pretty-hooks" {
			continue
		}
		out := filepath.Join(dir, "pretty-hooks")
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(w, tr)
		w.Close()
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("archive has no pretty-hooks binary")
}
