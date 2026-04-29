// T6 — opt-in update check via GitHub Releases API.
//
// The handler queries https://api.github.com/repos/ssallem/local-fx/releases/latest
// at most once per 24h, caches the result on disk with the response ETag, and
// returns a comparison against the compiled-in version.Version.
//
// Privacy posture (see docs/PRIVACY.md):
//   - Default OFF on the extension side. The extension only invokes this op
//     after the user has explicitly opted in via the settings panel.
//   - Host-side hard-disable: env var LOCALFX_DISABLE_UPDATE_CHECK=1 short-
//     circuits the handler with E_DISABLED. No network call, no cache read.
//   - The only outbound data is the User-Agent header (host version + repo
//     URL) and the source IP — the request body is empty.
//   - ETag caching collapses repeat checks into 304 Not Modified responses
//     so even users who poll aggressively don't generate body traffic.
//
// Cache file lives at <UserCacheDir>/LocalFx/update-cache.json (Windows:
// %LOCALAPPDATA%\LocalFx\update-cache.json).
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"local-fx-host/internal/protocol"
	"local-fx-host/internal/version"
)

// ErrCodeDisabled is returned when LOCALFX_DISABLE_UPDATE_CHECK is set to
// "1". Declared inline here (rather than in protocol/errors.go) because it
// is op-local and not part of the formal §8 catalog — the extension treats
// it as an informational state, not a transport failure.
const ErrCodeDisabled = "E_DISABLED"

// updateCacheRelative is the path joined onto os.UserCacheDir() to produce
// the cache file location. Exposed as a constant so tests can compute the
// expected path without re-deriving it.
const updateCacheRelative = "LocalFx" + string(filepath.Separator) + "update-cache.json"

// updateCheckInterval is the minimum elapsed time between live HTTP calls.
// Cache hits within this window are returned with Cached=true and no network.
const updateCheckInterval = 24 * time.Hour

// releaseNotesMaxLen caps the Markdown body forwarded to the UI so a release
// with a long changelog cannot blow up the response frame size.
const releaseNotesMaxLen = 500

// httpTimeout bounds the GitHub API call. 10s is comfortably above observed
// 95p latency yet short enough to surface a real outage to the UI instead
// of stalling the whole IPC channel.
const httpTimeout = 10 * time.Second

// defaultReleasesURL is the upstream endpoint. Hard-coded to the canonical
// repo so a fork that doesn't change this constant can't accidentally point
// users at a different update channel. Tests override it via releasesURL.
const defaultReleasesURL = "https://api.github.com/repos/ssallem/local-fx/releases/latest"

// assetPattern is the prefix every Windows installer asset shares. We pick
// the first asset whose name matches localfx-host-setup-*.exe.
const assetPattern = "localfx-host-setup-"

// releasesURL is the in-memory endpoint used by fetchLatestRelease. It is
// package-level (not const) so tests can swap in a httptest.NewServer URL
// for the duration of a single test case.
var releasesURL = defaultReleasesURL

// updateCache is the JSON shape persisted to disk. Field names are
// deliberately snake_case (not Go's PascalCase via tags) so the file is
// readable for someone debugging cache state outside the binary.
type updateCache struct {
	CheckedAtUnixMs int64  `json:"checked_at_unix_ms"`
	LatestTag       string `json:"latest_tag"`
	ETag            string `json:"etag"`
	LastStatusCode  int    `json:"last_status_code"`
	DownloadURL     string `json:"download_url,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
}

// CheckUpdateData is the response payload returned to the extension. Field
// tags mirror the TypeScript interface in extension/src/types/shared.ts
// (CheckUpdateData) — keep both in lock-step.
type CheckUpdateData struct {
	HasUpdate      bool   `json:"hasUpdate"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	DownloadURL    string `json:"downloadUrl,omitempty"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
	Cached         bool   `json:"cached"`
	CheckedAtMs    int64  `json:"checkedAtMs"`
}

// CheckUpdateArgs intentionally has zero user-facing fields right now;
// declared as a struct (rather than skipping the unmarshal) so future
// additions (e.g. `force: true` to bypass the cache) can land without
// changing the op signature. Fields MUST stay user-facing only — anything
// that influences host filesystem paths or network targets must be a
// compiled-in constant, env var, or test-only package-level setter (see
// SetTestCacheDir below) so a malicious extension page cannot flip them.
type CheckUpdateArgs struct{}

// testCacheDirOverride is a package-level test hook. When non-empty it
// replaces the platform os.UserCacheDir() base used by resolveCachePath.
// MUST NEVER be settable from the IPC wire — that would let any caller
// redirect cache writes to an arbitrary path. Default empty = use OS user
// cache dir (Windows: %LOCALAPPDATA%, etc.).
var testCacheDirOverride string

// SetTestCacheDir is a TEST-ONLY hook. Production code paths must NEVER
// call this. Tests should pair it with t.Cleanup(func() { SetTestCacheDir("") })
// so a panic mid-test doesn't leak the override into the next test.
func SetTestCacheDir(path string) { testCacheDirOverride = path }

// githubRelease is the subset of the GitHub Releases API JSON we care
// about. Anything we don't decode is silently dropped by encoding/json.
//
// Draft and Prerelease are decoded so fetchLatestRelease can refuse to
// surface anything other than a stable, published release: the
// /releases/latest endpoint is supposed to skip these, but a
// misconfiguration upstream (or a manual tag flip from stable → draft)
// could expose users to a half-baked installer. Belt-and-braces.
type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Body       string        `json:"body"`
	Assets     []githubAsset `json:"assets"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// httpClient is package-level so tests can swap the transport. Using a
// dedicated client (rather than http.DefaultClient) means we can attach a
// 10s timeout without bleeding into other network code in this binary.
var httpClient = &http.Client{Timeout: httpTimeout}

// CheckUpdate is the registered op handler. It returns Response{OK:false}
// only on programmer errors (bad args, env-var disable). Network failures
// and rate-limit responses degrade to a cached response with a warning
// rather than aborting — the caller benefits more from "your last-known
// latest is X" than from a hard error.
func CheckUpdate(_ context.Context, req protocol.Request) protocol.Response {
	if os.Getenv("LOCALFX_DISABLE_UPDATE_CHECK") == "1" {
		return protocol.Response{
			ID: req.ID,
			OK: false,
			Error: protocol.NewError(
				ErrCodeDisabled,
				"Update checks are disabled by environment",
				false,
			),
		}
	}

	var args CheckUpdateArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return errResp(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}
	_ = args // currently no user-facing fields; keep var to lock in the unmarshal contract.

	cachePath, err := resolveCachePath()
	if err != nil {
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}
	}

	cache, _ := loadCache(cachePath)
	now := time.Now()

	// Within-window cache hit — no network, no header roundtrip.
	if cache != nil && cache.CheckedAtUnixMs > 0 {
		age := now.Sub(time.UnixMilli(cache.CheckedAtUnixMs))
		if age >= 0 && age < updateCheckInterval && cache.LatestTag != "" {
			return protocol.Response{
				ID:   req.ID,
				OK:   true,
				Data: buildData(cache, true),
			}
		}
	}

	// Cache stale (or absent) — issue the live request. We pass the cached
	// ETag so GitHub can answer 304 Not Modified for the common no-new-
	// release case.
	resp, status, err := fetchLatestRelease(cache)
	switch {
	case err != nil:
		// Hard transport failure (DNS, TLS, refused connection). If we have
		// any cached value at all, surface that with a stale flag instead
		// of failing — users prefer slightly outdated info to "?".
		if cache != nil && cache.LatestTag != "" {
			return protocol.Response{
				ID:   req.ID,
				OK:   true,
				Data: buildData(cache, true),
			}
		}
		return protocol.Response{ID: req.ID, OK: false, Error: mapFSError(err)}

	case status == http.StatusNotModified:
		// 304 — keep the cached tag, refresh the timestamp so we don't
		// re-poll on the next call within the window.
		if cache == nil {
			cache = &updateCache{}
		}
		cache.CheckedAtUnixMs = now.UnixMilli()
		cache.LastStatusCode = status
		_ = saveCache(cachePath, cache)
		return protocol.Response{
			ID:   req.ID,
			OK:   true,
			Data: buildData(cache, false),
		}

	case status == http.StatusForbidden:
		// 403 typically means the IP hit the unauthenticated rate limit.
		// Don't update the cache timestamp — we want to retry sooner once
		// the limit resets — but return whatever we already had so the UI
		// can keep functioning.
		if cache != nil && cache.LatestTag != "" {
			return protocol.Response{
				ID:   req.ID,
				OK:   true,
				Data: buildData(cache, true),
			}
		}
		return protocol.Response{
			ID: req.ID,
			OK: false,
			Error: protocol.NewError(
				protocol.ErrCodeEIO,
				"github rate limit reached and no cached release available",
				true,
			),
		}

	case status >= 200 && status < 300:
		// 200 — happy path. Replace the cache with fresh fields.
		next := &updateCache{
			CheckedAtUnixMs: now.UnixMilli(),
			LatestTag:       strings.TrimSpace(resp.tagName),
			ETag:            resp.etag,
			LastStatusCode:  status,
			DownloadURL:     resp.downloadURL,
			ReleaseNotes:    truncateNotes(resp.body),
		}
		_ = saveCache(cachePath, next)
		return protocol.Response{
			ID:   req.ID,
			OK:   true,
			Data: buildData(next, false),
		}

	default:
		// Other non-2xx (5xx, weird upstream). Same fallback chain as 403.
		if cache != nil && cache.LatestTag != "" {
			return protocol.Response{
				ID:   req.ID,
				OK:   true,
				Data: buildData(cache, true),
			}
		}
		return protocol.Response{
			ID: req.ID,
			OK: false,
			Error: protocol.NewErrorWithDetails(
				protocol.ErrCodeEIO,
				"github responded with status "+strconv.Itoa(status),
				true,
				map[string]interface{}{"status": status},
			),
		}
	}
}

// buildData converts a cache record + the "is this stale" flag into the
// wire response struct. Centralised so the four code paths (within-window,
// 304, 200, fallback) all assemble exactly the same shape.
func buildData(c *updateCache, cached bool) CheckUpdateData {
	current := version.Version
	latest := strings.TrimPrefix(c.LatestTag, "v")
	hasUpdate := compareSemver(current, latest) < 0
	d := CheckUpdateData{
		HasUpdate:      hasUpdate,
		CurrentVersion: current,
		LatestVersion:  latest,
		Cached:         cached,
		CheckedAtMs:    c.CheckedAtUnixMs,
	}
	if hasUpdate {
		// Only expose the download URL/notes when there's actually an
		// update. Saves the UI from having to second-guess whether to show
		// the download button on an up-to-date install.
		if c.DownloadURL != "" {
			d.DownloadURL = c.DownloadURL
		}
		if c.ReleaseNotes != "" {
			d.ReleaseNotes = c.ReleaseNotes
		}
	}
	return d
}

// fetchLatestReleaseResult bundles the fields we extract from a successful
// GitHub response so fetchLatestRelease can return them alongside the HTTP
// status without exporting the githubRelease struct.
type fetchLatestReleaseResult struct {
	tagName     string
	etag        string
	downloadURL string
	body        string
}

// fetchLatestRelease sends the GET to GitHub. The caller may pass a non-nil
// cache to enable If-None-Match (etag); a nil cache means "no prior etag,
// always send a body-bearing GET".
func fetchLatestRelease(cache *updateCache) (fetchLatestReleaseResult, int, error) {
	req, err := http.NewRequest(http.MethodGet, releasesURL, nil)
	if err != nil {
		return fetchLatestReleaseResult{}, 0, err
	}
	req.Header.Set("User-Agent",
		"local-fx/"+version.Version+" (+https://github.com/ssallem/local-fx)")
	req.Header.Set("Accept", "application/vnd.github+json")
	if cache != nil && cache.ETag != "" {
		req.Header.Set("If-None-Match", cache.ETag)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fetchLatestReleaseResult{}, 0, err
	}
	defer resp.Body.Close()

	// 304 has no body; the only thing we want is the status itself plus
	// the (possibly refreshed) ETag header. Treat it as a thin success.
	if resp.StatusCode == http.StatusNotModified {
		return fetchLatestReleaseResult{etag: resp.Header.Get("ETag")}, resp.StatusCode, nil
	}

	// Read at most 1 MiB of body — defence against an upstream that
	// streams gigabytes of release notes. The releases endpoint reliably
	// fits in tens of KB; 1 MiB is a generous ceiling.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fetchLatestReleaseResult{}, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Surface the non-2xx without parsing — the caller decides
		// whether to fall back to cache or error out.
		return fetchLatestReleaseResult{etag: resp.Header.Get("ETag")}, resp.StatusCode, nil
	}

	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return fetchLatestReleaseResult{}, resp.StatusCode,
			fmt.Errorf("decode release JSON: %w", err)
	}

	// Refuse to surface drafts or pre-releases. /releases/latest is supposed
	// to filter these out server-side, but if something flips upstream we'd
	// otherwise quietly point users at an unstable installer. Returning an
	// error here makes the caller fall through to the "stale cache or hard
	// fail" branch — the UI shows last-known-good or nothing.
	if rel.Draft || rel.Prerelease {
		return fetchLatestReleaseResult{}, resp.StatusCode,
			fmt.Errorf("github returned draft or prerelease (tag=%q draft=%v prerelease=%v) — refusing to surface",
				rel.TagName, rel.Draft, rel.Prerelease)
	}

	return fetchLatestReleaseResult{
		tagName:     rel.TagName,
		etag:        resp.Header.Get("ETag"),
		downloadURL: pickDownloadURL(rel.Assets),
		body:        rel.Body,
	}, resp.StatusCode, nil
}

// pickDownloadURL returns the first asset whose Name starts with the
// expected installer prefix. Empty when no asset matches — the UI then
// degrades to a "release page" link rather than a direct download.
func pickDownloadURL(assets []githubAsset) string {
	for _, a := range assets {
		if strings.HasPrefix(a.Name, assetPattern) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// truncateNotes caps the release Markdown to releaseNotesMaxLen bytes so a
// chatty changelog can't blow up the IPC frame size. We slice on bytes
// because the response is wire-bound and unicode-aware truncation isn't
// required for a tooltip preview.
func truncateNotes(s string) string {
	if len(s) <= releaseNotesMaxLen {
		return s
	}
	return s[:releaseNotesMaxLen]
}

// resolveCachePath produces the absolute path to update-cache.json,
// honouring the package-level testCacheDirOverride when set. The override
// is intentionally NOT a function parameter — that would invite a future
// caller to plumb a wire field into it, re-introducing the arbitrary-write
// vulnerability the test hook was carved out to avoid. Side effect:
// ensures the parent directory exists so a subsequent saveCache won't
// ENOENT.
func resolveCachePath() (string, error) {
	base := testCacheDirOverride
	if base == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("user cache dir: %w", err)
		}
		base = dir
	}
	full := filepath.Join(base, updateCacheRelative)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("mkdir cache parent: %w", err)
	}
	return full, nil
}

// loadCache reads and decodes the on-disk cache. Returns (nil, nil) when
// the file is absent so callers don't need to distinguish "no cache" from
// "decode failed" — either way the next step is a fresh HTTP call.
func loadCache(path string) (*updateCache, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c updateCache
	if err := json.Unmarshal(raw, &c); err != nil {
		// Corrupt cache shouldn't permanently brick update checks; treat
		// as missing so the next fetch repopulates it.
		return nil, nil
	}
	return &c, nil
}

// saveCache atomically writes the cache via temp-file + rename so a crash
// mid-write can't leave a half-formed JSON file that loadCache can't parse.
func saveCache(path string, c *updateCache) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-cache-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// compareSemver returns -1, 0, or +1 for a<b, a==b, a>b.
//
// Both inputs are treated as dot-separated numeric components with an
// optional `-` pre-release suffix. A bare version is considered NEWER than
// the same version with any pre-release suffix (so 0.3.0 > 0.3.0-rc1),
// matching SemVer 2.0.0 §11.
//
// Non-numeric components fall back to strings.Compare so unusual tags like
// `v1.0.x` don't panic; the comparison is still deterministic, just not
// semver-perfect for those shapes.
func compareSemver(a, b string) int {
	aBase, aPre := splitPreRelease(a)
	bBase, bPre := splitPreRelease(b)

	aParts := strings.Split(aBase, ".")
	bParts := strings.Split(bBase, ".")
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		// Missing components are treated as "0" so 0.3 == 0.3.0 == 0.3.0.0.
		// SemVer 2.0.0 mandates 3-component versions but real-world tags
		// often drop trailing zero segments; padding here makes comparison
		// robust without changing the meaning of explicit zeros.
		ap := "0"
		bp := "0"
		if i < len(aParts) {
			ap = aParts[i]
		}
		if i < len(bParts) {
			bp = bParts[i]
		}
		ai, aerr := strconv.Atoi(ap)
		bi, berr := strconv.Atoi(bp)
		if aerr == nil && berr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
			continue
		}
		if cmp := strings.Compare(ap, bp); cmp != 0 {
			return cmp
		}
	}

	// Bases equal. Per SemVer §11: a version with a pre-release suffix
	// has LOWER precedence than the same version without one.
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "" && bPre != "":
		return 1
	case aPre != "" && bPre == "":
		return -1
	default:
		return strings.Compare(aPre, bPre)
	}
}

// splitPreRelease splits "0.3.0-rc1" into ("0.3.0", "rc1"). A bare version
// returns ("0.3.0", "").
func splitPreRelease(v string) (string, string) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}
