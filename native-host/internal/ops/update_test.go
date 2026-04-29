package ops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/version"
)

// withReleasesURL points the package-level releasesURL at the supplied
// httptest server for the duration of t. Restoring on cleanup keeps tests
// independent — without it a prior test's server URL would leak into a
// later test's "no server provided" path.
func withReleasesURL(t *testing.T, url string) {
	t.Helper()
	prev := releasesURL
	releasesURL = url
	t.Cleanup(func() { releasesURL = prev })
}

// failingTransport panics if any HTTP request is made. Installed for the
// "cache-hit must not call network" assertion so the test fails loudly
// rather than silently round-tripping to GitHub during CI.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatalf("HTTP call made when cache should have been used")
	return nil, nil
}

// withFailingClient installs a client whose transport panics on any
// outbound request, restoring the original on cleanup. Use in tests that
// must prove the network was NOT touched.
func withFailingClient(t *testing.T) {
	t.Helper()
	prev := httpClient
	httpClient = &http.Client{Transport: failingTransport{t: t}}
	t.Cleanup(func() { httpClient = prev })
}

// useTestCacheDir installs the package-level test cache dir override and
// schedules its restoration on cleanup. Callers MUST use this rather than
// trying to plumb a path through CheckUpdateArgs — the wire struct no
// longer carries an override field (T6 CRITICAL #1 fix), because letting
// arbitrary JSON pick the host's cache directory would let any code that
// can reach the IPC port redirect cache writes to an attacker-chosen path.
func useTestCacheDir(t *testing.T, dir string) {
	t.Helper()
	prev := testCacheDirOverride
	SetTestCacheDir(dir)
	t.Cleanup(func() { SetTestCacheDir(prev) })
}

// mustCheckUpdateRequest builds a Request envelope for CheckUpdate. Args
// is intentionally an empty struct — there are no user-facing fields. The
// cache directory is configured separately via useTestCacheDir.
func mustCheckUpdateRequest(t *testing.T, id string) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(CheckUpdateArgs{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return protocol.Request{ID: id, Op: "checkUpdate", Args: raw}
}

// parseCheckUpdateData round-trips the response Data through JSON so the
// assertions can read concrete struct fields without resorting to map
// indexing — same pattern as parseStatData in stat_test.go.
func parseCheckUpdateData(t *testing.T, resp protocol.Response) CheckUpdateData {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var d CheckUpdateData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d
}

// seedCache writes a synthetic cache file to disk so tests can simulate
// "previous check happened N ago" without driving an end-to-end round.
// Caller MUST have already installed a useTestCacheDir override — without
// it the seed lands in the real %LOCALAPPDATA%\LocalFx and pollutes the
// developer's machine.
func seedCache(t *testing.T, c updateCache) {
	t.Helper()
	path, err := resolveCachePath()
	if err != nil {
		t.Fatalf("resolveCachePath: %v", err)
	}
	if err := saveCache(path, &c); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
}

// TestCheckUpdate_CacheHitWithinWindow exercises the fast path: a cache
// entry younger than 24h MUST be returned without any network call.
// failingTransport guarantees this — any HTTP egress fails the test.
func TestCheckUpdate_CacheHitWithinWindow(t *testing.T) {
	useTestCacheDir(t, t.TempDir())
	withFailingClient(t)
	seedCache(t, updateCache{
		CheckedAtUnixMs: time.Now().Add(-1 * time.Hour).UnixMilli(),
		LatestTag:       "v0.4.0",
		ETag:            `W/"abc"`,
		LastStatusCode:  200,
		DownloadURL:     "https://example.invalid/installer.exe",
	})

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r1"))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseCheckUpdateData(t, resp)
	if !d.Cached {
		t.Errorf("Cached: got false, want true")
	}
	if d.LatestVersion != "0.4.0" {
		t.Errorf("LatestVersion: got %q, want %q", d.LatestVersion, "0.4.0")
	}
}

// TestCheckUpdate_CacheMiss200 — no cache file, server returns 200 with a
// matching asset. We expect HasUpdate=true (because version.Version is
// older than the test response 0.4.0) and the asset URL to flow through.
func TestCheckUpdate_CacheMiss200(t *testing.T) {
	useTestCacheDir(t, t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sanity-check the headers we promise in PRIVACY.md.
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Errorf("User-Agent header missing")
		}
		if ac := r.Header.Get("Accept"); ac != "application/vnd.github+json" {
			t.Errorf("Accept: got %q, want application/vnd.github+json", ac)
		}
		w.Header().Set("ETag", `W/"fresh-etag"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "tag_name": "v0.4.0",
		  "body": "release notes here",
		  "assets": [
		    {"name": "localfx-host-setup-0.4.0.exe", "browser_download_url": "https://example.invalid/dl.exe"},
		    {"name": "checksums.txt", "browser_download_url": "https://example.invalid/c.txt"}
		  ]
		}`))
	}))
	t.Cleanup(srv.Close)
	withReleasesURL(t, srv.URL)

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r2"))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseCheckUpdateData(t, resp)
	if d.Cached {
		t.Errorf("Cached: got true, want false (live response)")
	}
	if d.LatestVersion != "0.4.0" {
		t.Errorf("LatestVersion: got %q, want %q", d.LatestVersion, "0.4.0")
	}
	if !d.HasUpdate {
		t.Errorf("HasUpdate: got false, want true (current=%s, latest=0.4.0)", version.Version)
	}
	if d.DownloadURL != "https://example.invalid/dl.exe" {
		t.Errorf("DownloadURL: got %q, want %q", d.DownloadURL, "https://example.invalid/dl.exe")
	}
	if d.ReleaseNotes != "release notes here" {
		t.Errorf("ReleaseNotes: got %q, want %q", d.ReleaseNotes, "release notes here")
	}
	if d.CurrentVersion != version.Version {
		t.Errorf("CurrentVersion: got %q, want %q", d.CurrentVersion, version.Version)
	}
	if d.CheckedAtMs <= 0 {
		t.Errorf("CheckedAtMs: got %d, want positive unix millis", d.CheckedAtMs)
	}
}

// TestCheckUpdate_CacheMiss304 — cache exists but is stale (>24h). Server
// returns 304 Not Modified. The cached tag must survive and the timestamp
// must refresh so the next call is a within-window hit.
func TestCheckUpdate_CacheMiss304(t *testing.T) {
	useTestCacheDir(t, t.TempDir())
	seedCache(t, updateCache{
		CheckedAtUnixMs: time.Now().Add(-48 * time.Hour).UnixMilli(),
		LatestTag:       "v0.4.0",
		ETag:            `W/"prev-etag"`,
		LastStatusCode:  200,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inm := r.Header.Get("If-None-Match"); inm != `W/"prev-etag"` {
			t.Errorf("If-None-Match: got %q, want W/\"prev-etag\"", inm)
		}
		w.Header().Set("ETag", `W/"prev-etag"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)
	withReleasesURL(t, srv.URL)

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r3"))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseCheckUpdateData(t, resp)
	if d.Cached {
		t.Errorf("Cached: got true, want false (304 with refreshed timestamp counts as fresh)")
	}
	if d.LatestVersion != "0.4.0" {
		t.Errorf("LatestVersion: got %q, want %q", d.LatestVersion, "0.4.0")
	}
	// Verify the on-disk timestamp moved forward by reading back the cache.
	path, _ := resolveCachePath()
	c, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache: %v", err)
	}
	if c == nil {
		t.Fatal("expected cache to persist after 304")
	}
	age := time.Since(time.UnixMilli(c.CheckedAtUnixMs))
	if age > 5*time.Minute {
		t.Errorf("cache timestamp not refreshed: age=%s", age)
	}
}

// TestCheckUpdate_CacheMiss403 — cache exists but is stale, server returns
// 403 (rate limit). We expect the cached value to surface with Cached=true,
// and the on-disk timestamp must NOT advance (we want to retry sooner).
func TestCheckUpdate_CacheMiss403(t *testing.T) {
	useTestCacheDir(t, t.TempDir())
	staleTs := time.Now().Add(-48 * time.Hour).UnixMilli()
	seedCache(t, updateCache{
		CheckedAtUnixMs: staleTs,
		LatestTag:       "v0.4.0",
		ETag:            `W/"prev"`,
		LastStatusCode:  200,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	t.Cleanup(srv.Close)
	withReleasesURL(t, srv.URL)

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r4"))
	if !resp.OK {
		t.Fatalf("OK=false: %+v", resp.Error)
	}
	d := parseCheckUpdateData(t, resp)
	if !d.Cached {
		t.Errorf("Cached: got false, want true (403 fallback)")
	}
	if d.LatestVersion != "0.4.0" {
		t.Errorf("LatestVersion: got %q, want %q", d.LatestVersion, "0.4.0")
	}
	// Critical: timestamp on disk must NOT have advanced — otherwise the
	// next call would be a within-window hit and we'd never retry past
	// the rate-limit window.
	path, _ := resolveCachePath()
	c, err := loadCache(path)
	if err != nil || c == nil {
		t.Fatalf("loadCache after 403: c=%v err=%v", c, err)
	}
	if c.CheckedAtUnixMs != staleTs {
		t.Errorf("CheckedAtUnixMs advanced after 403: got %d, want %d", c.CheckedAtUnixMs, staleTs)
	}
}

// TestCheckUpdate_EnvDisabled — LOCALFX_DISABLE_UPDATE_CHECK=1 short-
// circuits the handler with E_DISABLED before any cache read or HTTP call.
// withFailingClient enforces the no-network requirement.
func TestCheckUpdate_EnvDisabled(t *testing.T) {
	t.Setenv("LOCALFX_DISABLE_UPDATE_CHECK", "1")
	withFailingClient(t)
	useTestCacheDir(t, t.TempDir())

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r5"))
	if resp.OK {
		t.Fatalf("OK=true, want false")
	}
	if resp.Error == nil {
		t.Fatal("Error: nil, want populated")
	}
	if resp.Error.Code != ErrCodeDisabled {
		t.Errorf("Error.Code: got %q, want %q", resp.Error.Code, ErrCodeDisabled)
	}
}

// TestCheckUpdate_DraftRejected — even if /releases/latest somehow returns
// a release marked draft:true (server-side filter misconfiguration, or a
// test endpoint), the host MUST refuse to surface it. With no prior cache
// the response degrades to E_IO rather than fabricating a hasUpdate=true
// claim that points at an unfinished installer.
func TestCheckUpdate_DraftRejected(t *testing.T) {
	useTestCacheDir(t, t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"draft-etag"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "tag_name": "v9.9.9",
		  "body": "WIP — do not ship",
		  "draft": true,
		  "prerelease": false,
		  "assets": [
		    {"name": "localfx-host-setup-9.9.9.exe", "browser_download_url": "https://example.invalid/draft.exe"}
		  ]
		}`))
	}))
	t.Cleanup(srv.Close)
	withReleasesURL(t, srv.URL)

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r-draft"))
	if resp.OK {
		d := parseCheckUpdateData(t, resp)
		t.Fatalf("draft release surfaced as OK; got HasUpdate=%v LatestVersion=%q", d.HasUpdate, d.LatestVersion)
	}
	if resp.Error == nil {
		t.Fatal("Error: nil, want populated")
	}
	if resp.Error.Code != protocol.ErrCodeEIO {
		t.Errorf("Error.Code: got %q, want %q", resp.Error.Code, protocol.ErrCodeEIO)
	}
}

// TestCheckUpdate_PrereleaseRejected — same shape as the draft test for
// the prerelease:true path. A pre-release tag (rc/beta/alpha) must never
// reach the user via the opt-in update toast — that surface is for
// stable-track installs only.
func TestCheckUpdate_PrereleaseRejected(t *testing.T) {
	useTestCacheDir(t, t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"pre-etag"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "tag_name": "v0.4.0-rc1",
		  "body": "release candidate",
		  "draft": false,
		  "prerelease": true,
		  "assets": [
		    {"name": "localfx-host-setup-0.4.0-rc1.exe", "browser_download_url": "https://example.invalid/rc.exe"}
		  ]
		}`))
	}))
	t.Cleanup(srv.Close)
	withReleasesURL(t, srv.URL)

	resp := CheckUpdate(context.Background(), mustCheckUpdateRequest(t, "r-prerelease"))
	if resp.OK {
		d := parseCheckUpdateData(t, resp)
		t.Fatalf("prerelease surfaced as OK; got HasUpdate=%v LatestVersion=%q", d.HasUpdate, d.LatestVersion)
	}
	if resp.Error == nil {
		t.Fatal("Error: nil, want populated")
	}
	if resp.Error.Code != protocol.ErrCodeEIO {
		t.Errorf("Error.Code: got %q, want %q", resp.Error.Code, protocol.ErrCodeEIO)
	}
}

// TestCheckUpdate_RegisteredInRegistry locks in that init() wires the op
// up; without this a regression that drops the Register call would
// silently fall through to E_UNKNOWN_OP at runtime.
func TestCheckUpdate_RegisteredInRegistry(t *testing.T) {
	if Lookup("checkUpdate") == nil {
		t.Fatal("checkUpdate handler not registered; registry init() did not run")
	}
}

// TestCompareSemver covers the documented edge cases. Treats the helper as
// part of the contract because the wire response's HasUpdate field hinges
// on it — a regression here is a regression in the user-visible toast.
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Numeric ordering.
		{"0.2.1", "0.4.0", -1},
		{"0.4.0", "0.2.1", 1},
		{"0.4.0", "0.4.0", 0},
		// Per SemVer §11: pre-release < release.
		{"0.4.0-rc1", "0.4.0", -1},
		{"0.4.0", "0.4.0-rc1", 1},
		// Pre-release vs pre-release falls back to lexical.
		{"0.4.0-rc1", "0.4.0-rc2", -1},
		// Leading "v" is stripped.
		{"v0.4.0", "0.4.0", 0},
		// Different component counts still compare numerically.
		{"0.4", "0.4.0", 0},
		{"1.0", "0.9.9", 1},
	}
	for _, tc := range cases {
		got := compareSemver(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareSemver(%q, %q): got %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
