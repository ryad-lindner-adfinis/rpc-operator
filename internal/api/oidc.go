package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig holds operator-level OIDC settings. nil-able on Server: when nil,
// all OIDC routes are unregistered and Whoami reports oidcEnabled=false.
// F20b uses a public OAuth 2.0 client with PKCE — no client_secret is needed
// or supported (see PRD Z.111).
type OIDCConfig struct {
	Issuer        string   // e.g. https://accounts.google.com or https://keycloak.example.com/realms/x
	ClientID      string   // OAuth 2.0 client ID registered at the IdP
	Scopes        []string // default: ["openid", "email", "offline_access"]
	RedirectURL   string   // public-facing https://<host>/api/v1/auth/callback
	UIRedirectURL string   // public-facing UI root, where the browser lands after callback success
}

// oidcRuntime wires the live provider + oauth2 config. Built lazily on the first
// /auth/login or /auth/refresh call so an IdP outage at operator boot does not
// trigger a crash loop — F20a token-paste must keep working in that case.
type oidcRuntime struct {
	once     sync.Once
	err      error
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// init performs OIDC discovery + verifier construction exactly once per process.
// Returns the cached error on subsequent calls when the first attempt failed,
// so a misconfigured issuer surfaces consistently rather than retrying forever.
func (rt *oidcRuntime) init(ctx context.Context, cfg *OIDCConfig) error {
	rt.once.Do(func() {
		p, err := oidc.NewProvider(ctx, cfg.Issuer)
		if err != nil {
			rt.err = fmt.Errorf("oidc discovery: %w", err)
			return
		}
		rt.provider = p
		rt.verifier = p.Verifier(&oidc.Config{ClientID: cfg.ClientID})
		rt.oauth = &oauth2.Config{
			ClientID:    cfg.ClientID,
			Endpoint:    p.Endpoint(),
			RedirectURL: cfg.RedirectURL,
			Scopes:      cfg.Scopes,
		}
	})
	return rt.err
}

// oidcSession is the backend-side state for one in-flight or active OIDC login.
// state/codeVerifier/nonce are set during /auth/login and consumed at /auth/callback;
// refreshToken is set at callback success and consumed by /auth/refresh.
type oidcSession struct {
	state        string
	codeVerifier string
	nonce        string
	refreshToken string // populated after callback; empty during in-flight login
	createdAt    time.Time
	mu           sync.Mutex // serializes refresh against concurrent callers in the same tab
}

// sessionStore is an in-memory TTL map keyed by random session ID (cookie value).
// Single-replica only; HA-multi-replica needs a shared backend (see ADR-0003).
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*oidcSession
	ttl      time.Duration
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{
		sessions: make(map[string]*oidcSession),
		ttl:      ttl,
	}
}

func (s *sessionStore) put(id string, sess *oidcSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
}

func (s *sessionStore) get(id string) (*oidcSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	if time.Since(sess.createdAt) > s.ttl {
		delete(s.sessions, id)
		return nil, false
	}
	return sess, true
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// gc walks the store and drops expired sessions. Cheap enough to run on a
// time.Ticker without locking individual entries — the store mutex is fast.
func (s *sessionStore) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-s.ttl)
	for id, sess := range s.sessions {
		if sess.createdAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

// randB64 returns n cryptographically random bytes encoded as base64url.
// PKCE code-verifier needs 43-128 chars; passing n=64 gives 86 chars, well in spec.
func randB64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// rand.Read on a healthy host never fails; panic so we surface a misconfigured
		// CSPRNG rather than silently issuing weak state/verifier values.
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// s256Challenge implements the PKCE S256 transformation: base64url(sha256(verifier)).
func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

const (
	oidcCookieName   = "rpc-oidc-state"
	oidcSessionTTL   = 10 * time.Minute
	oidcCookiePath   = "/api/v1/auth/"
	oidcCookieMaxAge = int(10 * 60) // seconds; matches oidcSessionTTL
)

// setSessionCookie writes the session-id cookie with the standard attributes.
// HttpOnly so JS cannot read it; SameSite=Lax so it rides along on the IdP
// callback redirect (top-level navigation); Secure so it only travels over TLS.
func setSessionCookie(w http.ResponseWriter, sessID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcCookieName,
		Value:    sessID,
		Path:     oidcCookiePath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   oidcCookieMaxAge,
	})
}

// clearSessionCookie expires the session-id cookie at the same path.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcCookieName,
		Value:    "",
		Path:     oidcCookiePath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// uiRedirectTarget returns the OIDC.UIRedirectURL when set, falling back to "/"
// (relative-root) which lands on the embedded SPA index for same-origin setups.
func (cfg *OIDCConfig) uiRedirectTarget() string {
	if cfg.UIRedirectURL != "" {
		return cfg.UIRedirectURL
	}
	return "/"
}

// handleOIDCLogin starts the PKCE auth-code flow. It is intentionally
// unauthenticated — anyone can hit this endpoint, the IdP is the trust anchor.
func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if s.OIDC == nil {
		writeJSONError(w, http.StatusNotFound, "oidc_disabled", "OIDC is not configured")
		return
	}
	if err := s.oidcRT.init(r.Context(), s.OIDC); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "oidc_init_failed", err.Error())
		return
	}

	sessID := randB64(32)
	state := randB64(32)
	codeVerifier := randB64(64) // 86 base64url chars, within RFC 7636 [43, 128]
	nonce := randB64(16)

	s.oidcStore.put(sessID, &oidcSession{
		state:        state,
		codeVerifier: codeVerifier,
		nonce:        nonce,
		createdAt:    time.Now(),
	})
	setSessionCookie(w, sessID)

	challenge := s256Challenge(codeVerifier)
	authURL := s.oidcRT.oauth.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("nonce", nonce),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback finalizes the PKCE flow: validates state, exchanges the
// code for tokens, verifies the id_token (signature + audience + nonce), caches
// the refresh_token in the session, and redirects the browser back to the UI
// with the id_token in the URL fragment (never query — fragment is not sent to
// the server and is absent from access logs).
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.OIDC == nil {
		writeJSONError(w, http.StatusNotFound, "oidc_disabled", "OIDC is not configured")
		return
	}
	if err := s.oidcRT.init(r.Context(), s.OIDC); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "oidc_init_failed", err.Error())
		return
	}

	cookie, err := r.Cookie(oidcCookieName)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing_state_cookie", err.Error())
		return
	}
	sess, ok := s.oidcStore.get(cookie.Value)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "session_not_found", "state cookie expired or unknown")
		return
	}

	// CSRF defense: state from the URL must match the one we stashed.
	if r.URL.Query().Get("state") != sess.state {
		s.oidcStore.delete(cookie.Value)
		writeJSONError(w, http.StatusBadRequest, "state_mismatch", "state parameter does not match")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_code", "no auth code in callback")
		return
	}

	tok, err := s.oidcRT.oauth.Exchange(r.Context(), code,
		oauth2.SetAuthURLParam("code_verifier", sess.codeVerifier),
	)
	if err != nil {
		s.oidcStore.delete(cookie.Value)
		writeJSONError(w, http.StatusBadGateway, "token_exchange_failed", err.Error())
		return
	}

	rawIDToken, _ := tok.Extra("id_token").(string)
	if rawIDToken == "" {
		writeJSONError(w, http.StatusBadGateway, "no_id_token",
			"IdP response is missing id_token; verify scope=openid is requested")
		return
	}

	idTok, err := s.oidcRT.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "id_token_invalid", err.Error())
		return
	}

	// OIDC Core 1.0 §3.1.3.7 step 11: verify nonce. go-oidc's verifier does not
	// enforce this on its own — the expected value lives in our session, not
	// the verifier config.
	var claims struct {
		Nonce string `json:"nonce"`
	}
	if err := idTok.Claims(&claims); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "id_token_claims_unreadable", err.Error())
		return
	}
	if claims.Nonce != sess.nonce {
		s.oidcStore.delete(cookie.Value)
		writeJSONError(w, http.StatusUnauthorized, "nonce_mismatch", "id_token nonce does not match login request")
		return
	}

	// Stash the refresh_token in the session for /auth/refresh. May be empty
	// if the IdP did not return one (missing offline_access scope, etc.) — in
	// that case /auth/refresh will fail and the UI falls back to a fresh login.
	sess.refreshToken = tok.RefreshToken
	s.oidcStore.put(cookie.Value, sess)
	setSessionCookie(w, cookie.Value) // refresh TTL on the client side too

	// no-store prevents browser-Back from re-submitting the (now consumed) code.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	target := s.OIDC.uiRedirectTarget() + "#id_token=" + rawIDToken
	http.Redirect(w, r, target, http.StatusFound)
}

// handleOIDCRefresh exchanges the cached refresh_token for a fresh id_token.
// The response body is a small JSON containing only the new id_token; the
// refresh_token never leaves the backend. If the IdP rotates the refresh_token
// (e.g. Keycloak with reuse=0), we replace it in the session.
func (s *Server) handleOIDCRefresh(w http.ResponseWriter, r *http.Request) {
	if s.OIDC == nil {
		writeJSONError(w, http.StatusNotFound, "oidc_disabled", "OIDC is not configured")
		return
	}
	if err := s.oidcRT.init(r.Context(), s.OIDC); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "oidc_init_failed", err.Error())
		return
	}

	cookie, err := r.Cookie(oidcCookieName)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "no_session", "no oidc session cookie")
		return
	}
	sess, ok := s.oidcStore.get(cookie.Value)
	if !ok || sess.refreshToken == "" {
		writeJSONError(w, http.StatusUnauthorized, "session_expired", "session not found or has no refresh token")
		return
	}

	// Serialize refresh against parallel calls in the same session so a
	// rotating IdP does not invalidate the refresh_token mid-flight.
	sess.mu.Lock()
	defer sess.mu.Unlock()

	src := s.oidcRT.oauth.TokenSource(r.Context(), &oauth2.Token{RefreshToken: sess.refreshToken})
	tok, err := src.Token()
	if err != nil {
		s.oidcStore.delete(cookie.Value)
		clearSessionCookie(w)
		writeJSONError(w, http.StatusUnauthorized, "refresh_failed", err.Error())
		return
	}

	rawIDToken, _ := tok.Extra("id_token").(string)
	if rawIDToken == "" {
		writeJSONError(w, http.StatusBadGateway, "no_id_token", "refresh response is missing id_token")
		return
	}
	if _, err := s.oidcRT.verifier.Verify(r.Context(), rawIDToken); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "id_token_invalid", err.Error())
		return
	}

	// IdP may rotate the refresh_token (Keycloak reuse=0). Replace it so the
	// next refresh uses the new value.
	if tok.RefreshToken != "" && tok.RefreshToken != sess.refreshToken {
		sess.refreshToken = tok.RefreshToken
		s.oidcStore.put(cookie.Value, sess)
	}

	writeJSON(w, http.StatusOK, map[string]string{"id_token": rawIDToken})
}

// handleOIDCLogout removes the session and expires the cookie. Always returns
// 204 to keep the call idempotent — UIs can fire-and-forget on logout.
func (s *Server) handleOIDCLogout(w http.ResponseWriter, r *http.Request) {
	if s.OIDC == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if cookie, err := r.Cookie(oidcCookieName); err == nil {
		s.oidcStore.delete(cookie.Value)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// startSessionStoreGC runs a background ticker that drops expired sessions
// from the in-memory store. Stops when ctx is cancelled (manager shutdown).
func (s *Server) startSessionStoreGC(ctx context.Context, interval time.Duration) {
	if s.oidcStore == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.oidcStore.gc()
			}
		}
	}()
}

// PrepareOIDCStoreForTest initializes the in-memory session store on a
// hand-built Server. Tests that bypass New() (because they don't have a real
// RestConfig) use this to make the OIDC handlers usable. Not for production.
func PrepareOIDCStoreForTest(s *Server) {
	if s.OIDC != nil && s.oidcStore == nil {
		s.oidcStore = newSessionStore(oidcSessionTTL)
	}
}
