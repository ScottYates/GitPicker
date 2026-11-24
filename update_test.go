package main

// Tests for the self update: the version comparison, the release page and
// the swap of the running build for the new one.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		tag   string
		major int
		minor int
		ok    bool
	}{
		{"v1.9", 1, 9, true},
		{"v1.10", 1, 10, true},
		{"1.10", 1, 10, true},
		{"v2.0", 2, 0, true},
		{"v1", 1, 0, true},
		{"v1.9.3", 1, 9, true},
		{"", 0, 0, false},
		{"vX.Y", 0, 0, false},
	}
	for _, c := range cases {
		major, minor, ok := parseVersion(c.tag)
		if ok != c.ok {
			t.Fatalf("%q: got ok %v, want %v", c.tag, ok, c.ok)
		}
		if ok && (major != c.major || minor != c.minor) {
			t.Fatalf("%q: got %d.%d, want %d.%d", c.tag, major, minor, c.major, c.minor)
		}
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		tag     string
		current string
		newer   bool
	}{
		{"v1.10", "v1.9", true},
		{"v1.9", "v1.9", false},
		{"v1.8", "v1.9", false},
		{"v2.0", "v1.9", true},
		{"v1.10", "v1.10", false},
		{"nonsense", "v1.9", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.tag, c.current); got != c.newer {
			t.Fatalf("%q against %q: got %v, want %v", c.tag, c.current, got, c.newer)
		}
	}
}

func TestLatestReleaseReadsThePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("the request should say who it is")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v1.10","assets":[{"name":"git-picker.exe","browser_download_url":"https://example.invalid/git-picker.exe"}]}`))
	}))
	defer server.Close()

	rel, err := latestRelease(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("the release page should be read: %v", err)
	}
	if rel.TagName != "v1.10" {
		t.Fatalf("got the tag %q, want v1.10", rel.TagName)
	}
	asset, ok := rel.asset(updateAsset)
	if !ok {
		t.Fatalf("the release should have the %s in it", updateAsset)
	}
	if asset.BrowserDownloadURL != "https://example.invalid/git-picker.exe" {
		t.Fatalf("got the address %q", asset.BrowserDownloadURL)
	}
}

func TestLatestReleaseFailsOnABadAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := latestRelease(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("a failed request should be an error")
	}
}

func TestInstallUpdateSwapsTheBuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-picker.exe")
	if err := os.WriteFile(path, []byte("the old build"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installUpdate(path, []byte("the new build")); err != nil {
		t.Fatalf("the build should be swapped: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "the new build" {
		t.Fatalf("the new build is not in place: %q", string(data))
	}

	// The old one is moved aside rather than removed, because Windows
	// will not remove a program that is running.
	old := path + updateOld
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("the old build should have been kept aside: %v", err)
	}
}

func TestRemoveOldBuildClearsItAway(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-picker.exe")
	if err := os.WriteFile(path+updateOld, []byte("the old build"), 0o755); err != nil {
		t.Fatal(err)
	}

	removeOldBuild(path)

	if _, err := os.Stat(path + updateOld); !os.IsNotExist(err) {
		t.Fatal("the old build should be gone")
	}
}
