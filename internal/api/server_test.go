package api_test

import (
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/insidegreen/rpc-operator-claude/internal/api"
)

func TestServer_UnknownPathReturns404(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/no-such-path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestStaticServing verifies that GET / serves the embedded placeholder index.html.
func TestStaticServing(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: want text/html, got %q", ct)
	}
	// The HTML entrypoint must be revalidated every load so a redeploy's new
	// content-hashed chunks are picked up (stale index.html → 404 lazy chunks).
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("GET /: Cache-Control = %q, want no-cache", cc)
	}
}

// TestStaticAssetsImmutable verifies hashed asset paths advertise long-lived
// immutable caching (the counterpart to the no-cache HTML entrypoint).
func TestStaticAssetsImmutable(t *testing.T) {
	entries, err := fs.ReadDir(api.StaticFiles, "static/assets")
	if err != nil {
		t.Skipf("no embedded assets dir: %v", err)
	}
	var asset string
	for _, e := range entries {
		if !e.IsDir() {
			asset = e.Name()
			break
		}
	}
	if asset == "" {
		t.Skip("no embedded asset file to probe")
	}

	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/" + asset)
	if err != nil {
		t.Fatalf("GET /assets/%s: %v", asset, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/%s: want 200, got %d", asset, resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("GET /assets/%s: Cache-Control = %q, want immutable", asset, cc)
	}
}

// TestStaticDoesNotShadowAPI verifies that /api/v1/... routes are served by the
// API handlers, not by the static file catch-all.
func TestStaticDoesNotShadowAPI(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/catalog")
	if err != nil {
		t.Fatalf("GET /api/v1/catalog: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/catalog: want 200, got %d", resp.StatusCode)
	}
}

// TestServer_WrongMethodOnValidate documents that with the SPA catch-all handler
// in place, a GET to the validate path returns 404 (file not in static FS).
// The POST /api/v1/pipelines/validate handler remains reachable for POST requests.
func TestServer_WrongMethodOnValidate(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// GET to the validate path hits the static catch-all → 404 (file not found)
	resp, err := http.Get(ts.URL + "/api/v1/pipelines/validate")
	if err != nil {
		t.Fatalf("GET validate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 (static catch-all), got %d", resp.StatusCode)
	}
}

func TestServer_WrongMethodOnListAll(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// DELETE /api/v1/pipelines has no registered handler; returns 404.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/pipelines", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /pipelines: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
