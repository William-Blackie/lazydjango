package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLatestReleaseAPIURL = "https://api.github.com/repos/William-Blackie/lazydjango/releases/latest"
	defaultLatestReleaseURL    = "https://github.com/William-Blackie/lazydjango/releases/latest"
	defaultCacheTTL            = 24 * time.Hour
)

type Result struct {
	CurrentVersion   string
	LatestVersion    string
	LatestReleaseURL string
	UpdateAvailable  bool
	Cached           bool
	Skipped          bool
	SkipReason       string
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

type cacheEntry struct {
	CheckedAt        time.Time `json:"checked_at"`
	CurrentVersion   string    `json:"current_version"`
	LatestVersion    string    `json:"latest_version"`
	LatestReleaseURL string    `json:"latest_release_url"`
}

// CheckLatest checks for a newer release. Errors are returned only for fetch/parse failures.
func CheckLatest(ctx context.Context, currentVersion string) (Result, error) {
	return checkLatest(ctx, currentVersion, nil, defaultLatestReleaseAPIURL, defaultCacheTTL)
}

func checkLatest(ctx context.Context, currentVersion string, client *http.Client, apiURL string, cacheTTL time.Duration) (Result, error) {
	result := Result{
		CurrentVersion:   strings.TrimSpace(currentVersion),
		LatestReleaseURL: defaultLatestReleaseURL,
	}
	if !isCheckableVersion(result.CurrentVersion) {
		result.Skipped = true
		result.SkipReason = "non-release build"
		return result, nil
	}

	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}

	cache, err := loadCache()
	if err == nil && cache.CurrentVersion == result.CurrentVersion && time.Since(cache.CheckedAt) < cacheTTL && cache.LatestVersion != "" {
		result.LatestVersion = cache.LatestVersion
		if cache.LatestReleaseURL != "" {
			result.LatestReleaseURL = cache.LatestReleaseURL
		}
		result.UpdateAvailable = compareVersions(cache.LatestVersion, result.CurrentVersion) > 0
		result.Cached = true
		return result, nil
	}

	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	latest, latestURL, err := fetchLatestRelease(ctx, client, apiURL, result.CurrentVersion)
	if err != nil {
		return result, err
	}

	result.LatestVersion = latest
	if latestURL != "" {
		result.LatestReleaseURL = latestURL
	}
	result.UpdateAvailable = compareVersions(result.LatestVersion, result.CurrentVersion) > 0

	_ = saveCache(cacheEntry{
		CheckedAt:        time.Now().UTC(),
		CurrentVersion:   result.CurrentVersion,
		LatestVersion:    result.LatestVersion,
		LatestReleaseURL: result.LatestReleaseURL,
	})

	return result, nil
}

func fetchLatestRelease(ctx context.Context, client *http.Client, apiURL, currentVersion string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("lazy-django/%s", currentVersion))

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("update check failed: status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decode latest release: %w", err)
	}

	latest := normalizeVersion(payload.TagName)
	if !isCheckableVersion(latest) {
		return "", "", fmt.Errorf("invalid latest release version: %q", payload.TagName)
	}

	return latest, strings.TrimSpace(payload.HTMLURL), nil
}

func compareVersions(a, b string) int {
	ap, okA := parseVersionParts(a)
	bp, okB := parseVersionParts(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if ap[i] > bp[i] {
			return 1
		}
		if ap[i] < bp[i] {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) ([3]int, bool) {
	var parts [3]int
	norm := normalizeVersion(v)
	if norm == "" {
		return parts, false
	}
	fields := strings.Split(norm, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return parts, false
	}
	for i := 0; i < len(fields); i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		parts[i] = n
	}
	return parts, true
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	if idx := strings.Index(v, "+"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

func isCheckableVersion(v string) bool {
	_, ok := parseVersionParts(v)
	return ok
}

func cacheFilePath() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "lazy-django", "update-check.json"), nil
}

func loadCache() (cacheEntry, error) {
	path, err := cacheFilePath()
	if err != nil {
		return cacheEntry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, err
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, err
	}
	return entry, nil
}

func saveCache(entry cacheEntry) error {
	path, err := cacheFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
