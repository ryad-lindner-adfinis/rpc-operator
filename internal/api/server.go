// Package api serves an HTTP REST layer over the Pipeline CRD plus a static
// component catalog. v0.2 ships inside the operator binary; later milestones
// may split it into a dedicated process — keep this package strictly
// independent of internal/controller.
//
// SECURITY: v0.2 listens plain HTTP and performs no authn/authz. Front with
// an Ingress that terminates TLS and integrates with your OIDC provider until
// v0.6 ships in-process auth (docs/prd.md F20).
package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

// Server is an HTTP REST server that integrates with the controller-runtime Manager.
type Server struct {
	Addr                string
	Client              client.Client
	Clientset           *kubernetes.Clientset // for pod log streaming; nil in tests
	Catalog             *catalog.Catalog
	PrometheusURL       string          // empty = Prometheus not configured
	WatchNamespaces     []string        // F21: nil/empty = cluster-wide; otherwise only listed namespaces are accessible
	AuthEnabled         bool            // F43: false = Mode A (Operator-SA serves everything); true = Mode B (token-forwarding)
	AnonymousRead       bool            // F42: when true (and AuthEnabled), GETs on pipelines/catalog/namespaces pass without a token
	AnonymousLogs       bool            // F42: when true (and AuthEnabled), WS /logs passes without a token; separate from AnonymousRead because log content can leak payloads
	Scheme              *runtime.Scheme // F20a: scheme for per-request controller-runtime clients
	RestConfig          *rest.Config    // F20a: base config (host + CA) for per-request clients; never mutated directly
	OIDC                *OIDCConfig     // F20b: when nil, OIDC routes are not registered and Whoami reports oidcEnabled=false
	VisualEditorEnabled bool            // F49: when false (default), UI routes all editors to RawPipelineEditor
	oidcRT              oidcRuntime     // F20b: lazy-initialized provider + verifier + oauth2 config
	oidcStore           *sessionStore   // F20b: in-memory session store; nil when OIDC is disabled
	srv                 *http.Server
}

// Compile-time check that Server implements manager.Runnable.
var _ manager.Runnable = (*Server)(nil)

// New constructs a Server. Returns an error if the embedded catalog fails to load.
// oidcCfg may be nil — in that case F20b OIDC routes are not registered and
// Whoami reports oidcEnabled=false.
func New(
	addr string, c client.Client, restCfg *rest.Config, scheme *runtime.Scheme,
	prometheusURL string, watchNamespaces []string,
	authEnabled, anonymousRead, anonymousLogs, visualEditorEnabled bool,
	oidcCfg *OIDCConfig,
) (*Server, error) {
	cat, err := catalog.Default()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	s := &Server{
		Addr:                addr,
		Client:              c,
		Clientset:           cs,
		Catalog:             cat,
		PrometheusURL:       prometheusURL,
		WatchNamespaces:     watchNamespaces,
		AuthEnabled:         authEnabled,
		AnonymousRead:       anonymousRead,
		AnonymousLogs:       anonymousLogs,
		VisualEditorEnabled: visualEditorEnabled,
		Scheme:              scheme,
		RestConfig:          restCfg,
		OIDC:                oidcCfg,
	}
	if oidcCfg != nil {
		s.oidcStore = newSessionStore(oidcSessionTTL)
	}
	return s, nil
}

// Start implements manager.Runnable. Called by the manager once the cache is synced.
func (s *Server) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("api")
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.startSessionStoreGC(ctx, 2*time.Minute)
	s.srv = &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("API server listening", "addr", s.Addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			log.Error(err, "API server shutdown error")
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// RegisterRoutesForTest exposes route registration for use in tests.
func (s *Server) RegisterRoutesForTest(mux *http.ServeMux) {
	s.registerRoutes(mux)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// F20a + F42: whoami is anonymous-eligible so the UI can detect Mode C
	// (read-only) state without a token.
	mux.HandleFunc("GET /api/v1/auth/whoami", s.authOrAnonymous(s.handleWhoami))

	// F20b: public, token-free capabilities probe. Registered unconditionally
	// (reports oidcEnabled=false when OIDC is off) so the login screen can show
	// the SSO button in Mode B strict, where whoami 401s before login.
	mux.HandleFunc("GET /api/v1/auth/config", s.handleAuthConfig)

	// F21 + F42: allowlist endpoint; anonymous-eligible in Mode C.
	mux.HandleFunc("GET /api/v1/namespaces", s.authOrAnonymous(s.handleListNamespaces))

	// Pipelines — Reads anonymous-eligible (F42), Writes stay token-required.
	mux.HandleFunc("GET /api/v1/pipelines",
		s.authOrAnonymous(s.handleListAll))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines",
		s.authOrAnonymous(s.allowlist(s.handleListNamespaced)))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}",
		s.authOrAnonymous(s.allowlist(s.handleGet)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelines",
		s.authIfEnabled(s.allowlist(s.handleCreate)))
	mux.HandleFunc("PUT /api/v1/namespaces/{namespace}/pipelines/{name}",
		s.authIfEnabled(s.allowlist(s.handleUpdate)))
	mux.HandleFunc("DELETE /api/v1/namespaces/{namespace}/pipelines/{name}",
		s.authIfEnabled(s.allowlist(s.handleDelete)))

	// F45: stop/run subresources — write actions, require auth in Modes B/C-with-token.
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelines/{name}/stop",
		s.authIfEnabled(s.allowlist(s.handleStop)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelines/{name}/run",
		s.authIfEnabled(s.allowlist(s.handleRun)))

	// F47 Phase 3b: PipelineCluster management. Reads anonymous-eligible (F42),
	// writes token-required — mirrors the pipeline routes above.
	mux.HandleFunc("GET /api/v1/pipelineclusters",
		s.authOrAnonymous(s.handleListAllClusters))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineclusters",
		s.authOrAnonymous(s.allowlist(s.handleListNamespacedClusters)))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineclusters/{name}",
		s.authOrAnonymous(s.allowlist(s.handleGetCluster)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelineclusters",
		s.authIfEnabled(s.allowlist(s.handleCreateCluster)))
	mux.HandleFunc("PUT /api/v1/namespaces/{namespace}/pipelineclusters/{name}",
		s.authIfEnabled(s.allowlist(s.handleUpdateCluster)))
	mux.HandleFunc("DELETE /api/v1/namespaces/{namespace}/pipelineclusters/{name}",
		s.authIfEnabled(s.allowlist(s.handleDeleteCluster)))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineclusters/{name}/instances",
		s.authOrAnonymous(s.allowlist(s.handleClusterInstances)))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineclusters/{name}/metrics",
		s.authOrAnonymous(s.allowlist(s.handleClusterMetrics)))

	// F50.3: PipelineProject management. Reads anonymous-eligible (F42),
	// writes token-required — mirrors the pipeline + cluster routes above.
	mux.HandleFunc("GET /api/v1/pipelineprojects",
		s.authOrAnonymous(s.handleListAllProjects))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineprojects",
		s.authOrAnonymous(s.allowlist(s.handleListNamespacedProjects)))
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelineprojects/{name}",
		s.authOrAnonymous(s.allowlist(s.handleGetProject)))
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelineprojects",
		s.authIfEnabled(s.allowlist(s.handleCreateProject)))
	mux.HandleFunc("PUT /api/v1/namespaces/{namespace}/pipelineprojects/{name}",
		s.authIfEnabled(s.allowlist(s.handleUpdateProject)))
	mux.HandleFunc("DELETE /api/v1/namespaces/{namespace}/pipelineprojects/{name}",
		s.authIfEnabled(s.allowlist(s.handleDeleteProject)))

	// Spec-only — no K8s touch, no auth, no allowlist. F42 anonymous-read keeps these open.
	mux.HandleFunc("POST /api/v1/pipelines/validate", s.handleValidate)
	mux.HandleFunc("POST /api/v1/pipelines/render", s.handleRender)

	// Catalog — anonymous-eligible (F42).
	mux.HandleFunc("GET /api/v1/catalog", s.authOrAnonymous(s.handleCatalogList))
	mux.HandleFunc("GET /api/v1/catalog/{category}/{name}", s.authOrAnonymous(s.handleCatalogGet))

	// Logs WS: token check is inline in handleLogStream (browsers cannot set
	// headers on `new WebSocket(...)`, so authMiddleware in front would always
	// 401 the WS upgrade). Logs have a SEPARATE F42 flag (s.AnonymousLogs)
	// because log content can contain payloads/secrets. Allowlist still
	// wraps it — path-param check is orthogonal to the WS mechanism.
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}/logs",
		s.allowlist(s.handleLogStream))

	// Metrics — anonymous-eligible (F42).
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}/metrics",
		s.authOrAnonymous(s.allowlist(s.handleMetrics)))

	// F20b: OIDC PKCE login. Routes are only registered when OIDC is configured;
	// when disabled, the paths return Go's default 404 (cleaner than 503/500).
	// All four endpoints are intentionally unauthenticated — the IdP and the
	// cookie-bound session are the trust anchors, not the F20a Bearer token.
	if s.OIDC != nil {
		mux.HandleFunc("GET /api/v1/auth/login", s.handleOIDCLogin)
		mux.HandleFunc("GET /api/v1/auth/callback", s.handleOIDCCallback)
		mux.HandleFunc("POST /api/v1/auth/refresh", s.handleOIDCRefresh)
		mux.HandleFunc("POST /api/v1/auth/logout", s.handleOIDCLogout)
	}

	// Serve the embedded SPA. Must come after all /api/v1/ routes (catch-all).
	sub, err := fs.Sub(StaticFiles, "static")
	if err != nil {
		panic("static embed broken: " + err.Error())
	}
	mux.Handle("/", spaCacheControl(http.FileServerFS(sub)))
}

// spaCacheControl wraps the static file server with SPA-appropriate caching.
// Vite emits content-hashed asset filenames, so /assets/* are immutable and can
// be cached forever. The HTML entrypoint, however, must always be revalidated:
// otherwise a browser keeps a stale index.html across a redeploy that references
// chunk hashes the new server no longer has, and a lazy-loaded chunk (e.g. the
// Monaco editor) 404s — surfacing as a blank page when opening the editor.
func spaCacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// index.html and any non-asset path: cache but force revalidation.
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
