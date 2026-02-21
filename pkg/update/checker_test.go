package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.0.2", "0.0.1", 1},
		{"1.2.3", "1.2.3", 0},
		{"1.2.2", "1.2.3", -1},
		{"v1.3.0", "1.2.9", 1},
		{"1.2", "1.2.0", 0},
	}

	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Fatalf("compareVersions(%q,%q): got %d want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCheckLatestAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.0.2","html_url":"https://example.com/release"}`))
	}))
	defer server.Close()

	withTempCacheHome(t)

	res, err := checkLatest(context.Background(), "0.0.1", server.Client(), server.URL, time.Minute)
	if err != nil {
		t.Fatalf("checkLatest returned error: %v", err)
	}
	if !res.UpdateAvailable {
		t.Fatal("expected update to be available")
	}
	if res.LatestVersion != "0.0.2" {
		t.Fatalf("expected latest version 0.0.2, got %q", res.LatestVersion)
	}
	if res.LatestReleaseURL != "https://example.com/release" {
		t.Fatalf("unexpected release URL: %q", res.LatestReleaseURL)
	}
}

func TestCheckLatestSkippedForDev(t *testing.T) {
	withTempCacheHome(t)

	res, err := checkLatest(context.Background(), "dev", nil, defaultLatestReleaseAPIURL, time.Minute)
	if err != nil {
		t.Fatalf("checkLatest returned error: %v", err)
	}
	if !res.Skipped {
		t.Fatal("expected check to be skipped for dev version")
	}
}

func TestCheckLatestUsesCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.0.5","html_url":"https://example.com/release"}`))
	}))
	defer server.Close()

	withTempCacheHome(t)

	_, err := checkLatest(context.Background(), "0.0.1", server.Client(), server.URL, time.Hour)
	if err != nil {
		t.Fatalf("first checkLatest returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 API call after first check, got %d", calls)
	}

	res, err := checkLatest(context.Background(), "0.0.1", server.Client(), server.URL, time.Hour)
	if err != nil {
		t.Fatalf("second checkLatest returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected no additional API calls due to cache, got %d", calls)
	}
	if !res.Cached {
		t.Fatal("expected cached result on second check")
	}
}

func withTempCacheHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cacheRoot := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatalf("mkdir cache root: %v", err)
	}
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("HOME", dir)
}
